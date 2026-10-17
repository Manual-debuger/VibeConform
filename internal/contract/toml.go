package contract

import (
	"fmt"
	"strings"
)

// parseTOML reads the subset of TOML that D-01 needs from
// pyproject.toml: tables, arrays of tables, dotted and quoted keys,
// strings, arrays and inline tables. Other scalars (numbers, booleans,
// dates) are kept as their text. It is not a validating parser: D-01
// only asks whether dependency lists are empty, and a new dependency for
// that would need an ADR.
func parseTOML(src string) (map[string]any, error) {
	p := &tomlParser{s: src, line: 1}
	root := map[string]any{}
	current := root
	for {
		p.skipSpaceAndComments(true)
		if p.eof() {
			return root, nil
		}
		if p.peek() == '[' {
			array := strings.HasPrefix(p.s[p.i:], "[[")
			if array {
				p.i += 2
			} else {
				p.i++
			}
			keys, err := p.key()
			if err != nil {
				return nil, err
			}
			p.skipSpace()
			closing := "]"
			if array {
				closing = "]]"
			}
			if !strings.HasPrefix(p.s[p.i:], closing) {
				return nil, p.errorf("table header does not end with %s", closing)
			}
			p.i += len(closing)
			if current, err = tomlTable(root, keys, array); err != nil {
				return nil, p.errorf("%v", err)
			}
			if err := p.endOfLine(); err != nil {
				return nil, err
			}
			continue
		}
		if err := p.keyValue(current); err != nil {
			return nil, err
		}
		if err := p.endOfLine(); err != nil {
			return nil, err
		}
	}
}

// tomlTable returns the table a header names, creating it. For an array
// of tables it appends a new table and returns it.
func tomlTable(root map[string]any, keys []string, array bool) (map[string]any, error) {
	t := root
	for i, k := range keys {
		last := i == len(keys)-1
		switch v := t[k].(type) {
		case nil:
			if last && array {
				n := map[string]any{}
				t[k] = []any{n}
				return n, nil
			}
			n := map[string]any{}
			t[k] = n
			t = n
		case map[string]any:
			t = v
		case []any:
			if len(v) == 0 {
				return nil, fmt.Errorf("%s is not a table", k)
			}
			if last && array {
				n := map[string]any{}
				t[k] = append(v, n)
				return n, nil
			}
			m, ok := v[len(v)-1].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s is not a table", k)
			}
			t = m
		default:
			return nil, fmt.Errorf("%s is not a table", k)
		}
	}
	return t, nil
}

type tomlParser struct {
	s    string
	i    int
	line int
}

func (p *tomlParser) eof() bool  { return p.i >= len(p.s) }
func (p *tomlParser) peek() byte { return p.s[p.i] }

func (p *tomlParser) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", p.line, fmt.Sprintf(format, args...))
}

func (p *tomlParser) skipSpace() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t') {
		p.i++
	}
}

// skipSpaceAndComments skips blanks and comments, and newlines when nl.
func (p *tomlParser) skipSpaceAndComments(nl bool) {
	for !p.eof() {
		switch p.peek() {
		case ' ', '\t', '\r':
			p.i++
		case '\n':
			if !nl {
				return
			}
			p.i++
			p.line++
		case '#':
			for !p.eof() && p.peek() != '\n' {
				p.i++
			}
		default:
			return
		}
	}
}

func (p *tomlParser) endOfLine() error {
	p.skipSpaceAndComments(false)
	if p.eof() {
		return nil
	}
	if p.peek() != '\n' {
		return p.errorf("unexpected %q after a value", p.peek())
	}
	return nil
}

// key reads a dotted key.
func (p *tomlParser) key() ([]string, error) {
	var keys []string
	for {
		p.skipSpace()
		if p.eof() {
			return nil, p.errorf("missing key")
		}
		var k string
		switch c := p.peek(); c {
		case '"', '\'':
			s, err := p.str()
			if err != nil {
				return nil, err
			}
			k = s
		default:
			start := p.i
			for !p.eof() && isBareKeyChar(p.peek()) {
				p.i++
			}
			if p.i == start {
				return nil, p.errorf("invalid key character %q", c)
			}
			k = p.s[start:p.i]
		}
		keys = append(keys, k)
		p.skipSpace()
		if p.eof() || p.peek() != '.' {
			return keys, nil
		}
		p.i++
	}
}

func isBareKeyChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// keyValue reads key = value into t.
func (p *tomlParser) keyValue(t map[string]any) error {
	keys, err := p.key()
	if err != nil {
		return err
	}
	p.skipSpace()
	if p.eof() || p.peek() != '=' {
		return p.errorf("expected = after %s", strings.Join(keys, "."))
	}
	p.i++
	p.skipSpace()
	v, err := p.value()
	if err != nil {
		return err
	}
	target, err := tomlTable(t, keys[:len(keys)-1], false)
	if err != nil {
		return p.errorf("%v", err)
	}
	target[keys[len(keys)-1]] = v
	return nil
}

func (p *tomlParser) value() (any, error) {
	if p.eof() {
		return nil, p.errorf("missing value")
	}
	switch p.peek() {
	case '"', '\'':
		return p.str()
	case '[':
		p.i++
		var list []any
		for {
			p.skipSpaceAndComments(true)
			if p.eof() {
				return nil, p.errorf("unterminated array")
			}
			if p.peek() == ']' {
				p.i++
				return list, nil
			}
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			list = append(list, v)
			p.skipSpaceAndComments(true)
			if !p.eof() && p.peek() == ',' {
				p.i++
			}
		}
	case '{':
		p.i++
		t := map[string]any{}
		for {
			p.skipSpaceAndComments(true)
			if p.eof() {
				return nil, p.errorf("unterminated inline table")
			}
			if p.peek() == '}' {
				p.i++
				return t, nil
			}
			if err := p.keyValue(t); err != nil {
				return nil, err
			}
			p.skipSpaceAndComments(true)
			if !p.eof() && p.peek() == ',' {
				p.i++
			}
		}
	default:
		start := p.i
		for !p.eof() && !strings.ContainsRune(",]}#\n\r", rune(p.peek())) {
			p.i++
		}
		text := strings.TrimSpace(p.s[start:p.i])
		if text == "" {
			return nil, p.errorf("missing value")
		}
		return text, nil
	}
}

// str reads a basic or literal string, single or multi-line.
func (p *tomlParser) str() (string, error) {
	q := p.peek()
	multi := strings.HasPrefix(p.s[p.i:], strings.Repeat(string(q), 3))
	delim := string(q)
	if multi {
		delim = strings.Repeat(string(q), 3)
	}
	p.i += len(delim)
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errorf("unterminated string")
		}
		if strings.HasPrefix(p.s[p.i:], delim) {
			p.i += len(delim)
			// A multi-line string may end with up to two more quotes.
			for multi && !p.eof() && p.peek() == q {
				b.WriteByte(q)
				p.i++
			}
			return b.String(), nil
		}
		c := p.peek()
		switch {
		case c == '\n' && !multi:
			return "", p.errorf("newline in a string")
		case c == '\n':
			p.line++
		case c == '\\' && q == '"' && p.i+1 < len(p.s):
			b.WriteByte(p.s[p.i+1])
			p.i += 2
			continue
		}
		b.WriteByte(c)
		p.i++
	}
}
