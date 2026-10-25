package guardrule

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// A small PowerShell tokenizer (spec 0053, "Parsing"). It is not a
// PowerShell parser: it finds the commands of a PowerShell command line
// and their words. It reads single and double quotes, backtick escapes,
// here-strings, comments, ; | && || and newlines, script blocks, the &
// and . invocation operators, $(…), @(…) and (…) subexpressions,
// hashtables, splatting, redirections and the --% stop-parsing token.
// A statement that starts with a variable, a string, a number or a
// keyword is an expression; the tokenizer looks inside it only for
// commands (in parentheses, blocks and after an assignment).

type psParser struct {
	src []rune
	i   int
	out []Command
}

func parsePowerShell(src string) ([]Command, error) {
	p := &psParser{src: []rune(src)}
	if err := p.statements(0, false); err != nil {
		return nil, err
	}
	return p.out, nil
}

func (p *psParser) errorf(format string, args ...any) error {
	return &ParseError{Shell: ShellPowerShell, Msg: fmt.Sprintf(format, args...)}
}

func (p *psParser) eof() bool { return p.i >= len(p.src) }

func (p *psParser) peek(k int) rune {
	if p.i+k < len(p.src) {
		return p.src[p.i+k]
	}
	return 0
}

func (p *psParser) at(s string) bool {
	r := []rune(s)
	if p.i+len(r) > len(p.src) {
		return false
	}
	return slices.Equal(p.src[p.i:p.i+len(r)], r)
}

func isBlank(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\f' || r == '\v' }

// skipSpaces skips blanks and line continuations (a backtick before a
// newline), not newlines.
func (p *psParser) skipSpaces() {
	for !p.eof() {
		switch {
		case isBlank(p.peek(0)):
			p.i++
		case p.peek(0) == '`' && (p.peek(1) == '\n' || (p.peek(1) == '\r' && p.peek(2) == '\n')):
			p.i += 2
			if p.peek(0) == '\n' {
				p.i++
			}
		default:
			return
		}
	}
}

// skipComment skips a comment at the cursor, if there is one.
func (p *psParser) skipComment() (bool, error) {
	switch {
	case p.at("<#"):
		end := strings.Index(string(p.src[p.i+2:]), "#>")
		if end < 0 {
			return false, p.errorf("a block comment <# is not closed")
		}
		p.i += 2 + len([]rune(string(p.src[p.i+2:])[:end])) + 2
		return true, nil
	case p.peek(0) == '#':
		for !p.eof() && p.peek(0) != '\n' {
			p.i++
		}
		return true, nil
	}
	return false, nil
}

// statements reads statements until close (0 for the end of input),
// which it consumes. In a hashtable each statement is key = value.
func (p *psParser) statements(close rune, hashtable bool) error {
	for {
		for !p.eof() {
			if r := p.peek(0); isBlank(r) || r == '\n' || r == ';' {
				p.i++
				continue
			}
			if p.peek(0) == '`' && (p.peek(1) == '\n' || p.peek(1) == '\r') {
				p.skipSpaces()
				continue
			}
			skipped, err := p.skipComment()
			if err != nil {
				return err
			}
			if !skipped {
				break
			}
		}
		if p.eof() {
			if close != 0 {
				return p.errorf("%q is not closed", opener(close))
			}
			return nil
		}
		if r := p.peek(0); r == close {
			p.i++
			return nil
		} else if r == ')' || r == '}' {
			return p.errorf("unexpected %q", r)
		}
		start := p.i
		if err := p.pipeline(hashtable); err != nil {
			return err
		}
		if p.i == start {
			return p.errorf("unexpected %q", p.peek(0))
		}
	}
}

func opener(close rune) rune {
	if close == ')' {
		return '('
	}
	return '{'
}

// pipeline reads commands joined by |, && and ||.
func (p *psParser) pipeline(hashtable bool) error {
	first := true
	for {
		p.skipSpaces()
		if !first {
			for !p.eof() && (isBlank(p.peek(0)) || p.peek(0) == '\n') {
				p.i++
			}
		}
		if err := p.element(first && hashtable); err != nil {
			return err
		}
		first = false
		p.skipSpaces()
		switch {
		case p.at("&&") || p.at("||"):
			p.i += 2
		case p.peek(0) == '|':
			p.i++
		case p.peek(0) == '&':
			p.i++ // runs in the background
			return nil
		default:
			return nil
		}
	}
}

var psKeywords = []string{
	"if", "elseif", "else", "foreach", "for", "while", "do", "until", "switch", "try", "catch",
	"finally", "function", "filter", "workflow", "param", "begin", "process", "end", "trap",
	"data", "class", "enum", "using", "dynamicparam", "in",
}

// element reads one pipeline element: a command or an expression.
func (p *psParser) element(expressionOnly bool) error {
	p.skipSpaces()
	if expressionOnly {
		return p.expression()
	}
	r := p.peek(0)
	switch {
	case p.eof():
		return nil
	case r == '&' && p.peek(1) != '&':
		p.i++
		p.skipSpaces()
		return p.command(true)
	case r == '.' && (isBlank(p.peek(1)) || strings.ContainsRune(`'"$&(`, p.peek(1))):
		p.i++
		p.skipSpaces()
		return p.command(true)
	case strings.ContainsRune("$('\"@{[!-+,", r) || unicode.IsDigit(r):
		return p.expression()
	}
	word := strings.ToLower(p.bareword())
	switch {
	case word == "return" || word == "throw" || word == "exit" || word == "break" || word == "continue":
		p.i += len([]rune(word))
		return p.element(false)
	case slices.Contains(psKeywords, word):
		return p.expression()
	}
	return p.command(false)
}

// bareword returns the plain token at the cursor without consuming it.
func (p *psParser) bareword() string {
	j := p.i
	for j < len(p.src) && (unicode.IsLetter(p.src[j]) || unicode.IsDigit(p.src[j]) || p.src[j] == '-' || p.src[j] == '_') {
		j++
	}
	if j < len(p.src) && !isBlank(p.src[j]) && !strings.ContainsRune("\n;|&(){}", p.src[j]) {
		return ""
	}
	return string(p.src[p.i:j])
}

// expression skips an expression up to the end of its statement, reading
// the commands inside its parentheses and blocks, and the pipeline
// after an assignment.
func (p *psParser) expression() error {
	for !p.eof() {
		r := p.peek(0)
		switch {
		case r == '\n' || r == ';' || r == ')' || r == '}' || r == '|' || r == '&':
			return nil
		case isBlank(r):
			p.i++
		case r == '`':
			p.i += 2
		case r == '#' || p.at("<#"):
			if _, err := p.skipComment(); err != nil {
				return err
			}
		case r == '\'':
			if _, err := p.single(); err != nil {
				return err
			}
		case r == '"':
			var w Word
			var b strings.Builder
			if err := p.double(&w, &b); err != nil {
				return err
			}
		case p.at("@'") || p.at("@\""):
			var w Word
			if err := p.hereString(&w); err != nil {
				return err
			}
		case p.at("$(") || p.at("@("):
			p.i += 2
			if err := p.statements(')', false); err != nil {
				return err
			}
		case p.at("@{"):
			p.i += 2
			if err := p.statements('}', true); err != nil {
				return err
			}
		case r == '(':
			p.i++
			if err := p.statements(')', false); err != nil {
				return err
			}
		case r == '{':
			p.i++
			if err := p.statements('}', false); err != nil {
				return err
			}
		case r == '=':
			p.i++
			return p.element(false)
		case r == '>' || (r == '*' || r >= '1' && r <= '6') && p.peek(1) == '>':
			// The expression's output goes to a file: a redirect-only
			// command, as "> f" is in Bash.
			c := Command{Shell: ShellPowerShell}
			if _, err := p.redirect(&c); err != nil {
				return err
			}
			if len(c.Redirects) > 0 {
				p.out = append(p.out, c)
			}
		default:
			p.i++
		}
	}
	return nil
}

// command reads a command: its name, then its arguments and
// redirections. After & or . (invoked), the name may be any word.
func (p *psParser) command(invoked bool) error {
	at := len(p.out)
	c := Command{Shell: ShellPowerShell}
	for {
		p.skipSpaces()
		if p.eof() {
			break
		}
		r := p.peek(0)
		if r == '\n' || r == ';' || r == '|' || r == '&' || r == ')' || r == '}' {
			break
		}
		if skipped, err := p.skipComment(); err != nil {
			return err
		} else if skipped {
			continue
		}
		if r == ',' {
			p.i++
			continue
		}
		if r == '<' {
			return p.errorf("the '<' operator is reserved for future use")
		}
		if ok, err := p.redirect(&c); err != nil {
			return err
		} else if ok {
			continue
		}
		if len(c.Words) > 0 && p.at("--%") && (p.i+3 == len(p.src) || isBlank(p.peek(3)) || p.peek(3) == '\n') {
			p.i += 3
			for !p.eof() && p.peek(0) != '\n' {
				j := p.i
				for j < len(p.src) && !isBlank(p.src[j]) && p.src[j] != '\n' {
					j++
				}
				if j > p.i {
					c.Words = append(c.Words, Word{Text: string(p.src[p.i:j]), Literal: true})
				}
				p.i = j
				p.skipSpaces()
			}
			break
		}
		w, err := p.word()
		if err != nil {
			return err
		}
		c.Words = append(c.Words, w)
	}
	if len(c.Words) == 0 {
		if invoked {
			return p.errorf("no command after the invocation operator")
		}
		if len(c.Redirects) == 0 {
			return nil
		}
	}
	p.out = slices.Insert(p.out, at, c)
	return nil
}

// redirect reads a redirection at the cursor: >, >>, n>, n>>, *>, *>>,
// or a stream merge such as 2>&1, which has no target.
func (p *psParser) redirect(c *Command) (bool, error) {
	j := p.i
	if j < len(p.src) && (p.src[j] == '*' || (p.src[j] >= '1' && p.src[j] <= '6')) {
		j++
	}
	if j >= len(p.src) || p.src[j] != '>' {
		return false, nil
	}
	j++
	if j < len(p.src) && p.src[j] == '>' {
		j++
	} else if j+1 < len(p.src) && p.src[j] == '&' && p.src[j+1] >= '1' && p.src[j+1] <= '6' {
		p.i = j + 2
		return true, nil
	}
	p.i = j
	p.skipSpaces()
	if p.eof() || strings.ContainsRune("\n;|&)}", p.peek(0)) {
		return false, p.errorf("a redirection has no target")
	}
	w, err := p.word()
	if err != nil {
		return false, err
	}
	// PowerShell has no input redirection, so every target is written.
	c.Redirects = append(c.Redirects, w)
	c.Writes = append(c.Writes, w)
	return true, nil
}

// word reads one argument-mode word.
func (p *psParser) word() (Word, error) {
	w := Word{Literal: true}
	var b strings.Builder
	start := true
	for !p.eof() {
		r := p.peek(0)
		if isBlank(r) || r == '\n' || strings.ContainsRune(";|&)},", r) {
			break
		}
		switch {
		case r == '(' && start:
			p.i++
			if err := p.statements(')', false); err != nil {
				return w, err
			}
			w.Literal, w.Unknown = false, true
			b.WriteString(ExpansionMark)
		case r == '(':
			w.Text = b.String()
			return w, nil
		case r == '{' && start:
			p.i++
			if err := p.statements('}', false); err != nil {
				return w, err
			}
			w.Literal, w.block = false, true
			b.WriteString(ExpansionMark)
		case r == '\'':
			s, err := p.single()
			if err != nil {
				return w, err
			}
			w.Quoted = true
			b.WriteString(s)
		case r == '"':
			w.Quoted = true
			if err := p.double(&w, &b); err != nil {
				return w, err
			}
		case r == '`':
			if p.peek(1) == '\n' || p.peek(1) == '\r' {
				p.skipSpaces()
				w.Text = b.String()
				return w, nil
			}
			if p.i+1 < len(p.src) {
				b.WriteRune(p.src[p.i+1])
			}
			p.i += 2
		case r == '$':
			if err := p.variable(&w, &b, false); err != nil {
				return w, err
			}
		case r == '@' && start && (p.at("@'") || p.at("@\"")) && p.hereStart():
			w.Quoted = true
			var hw Word
			if err := p.hereString(&hw); err != nil {
				return w, err
			}
			if !hw.Literal {
				w.Literal = false
			}
			b.WriteString(hw.Text)
		case r == '@' && start && p.peek(1) == '(':
			p.i += 2
			if err := p.statements(')', false); err != nil {
				return w, err
			}
			w.Literal, w.Unknown = false, true
			b.WriteString(ExpansionMark)
		case r == '@' && start && p.peek(1) == '{':
			p.i += 2
			if err := p.statements('}', true); err != nil {
				return w, err
			}
			w.Literal = false
			b.WriteString(ExpansionMark)
		case r == '@' && start && (unicode.IsLetter(p.peek(1)) || p.peek(1) == '_'):
			p.i++
			for !p.eof() && (unicode.IsLetter(p.peek(0)) || unicode.IsDigit(p.peek(0)) || p.peek(0) == '_') {
				p.i++
			}
			w.Literal, w.Unknown = false, true
			b.WriteString(ExpansionMark)
		default:
			b.WriteRune(r)
			p.i++
		}
		start = false
	}
	w.Text = b.String()
	return w, nil
}

// single reads a single-quoted string; ” is a quote.
func (p *psParser) single() (string, error) {
	p.i++
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errorf("a single-quoted string is not closed")
		}
		r := p.peek(0)
		if r == '\'' {
			if p.peek(1) == '\'' {
				b.WriteRune('\'')
				p.i += 2
				continue
			}
			p.i++
			return b.String(), nil
		}
		b.WriteRune(r)
		p.i++
	}
}

// double reads a double-quoted string into b. "" is a quote, a backtick
// escapes, and variables and $(…) are expansions (quoted, so not
// Unknown).
func (p *psParser) double(w *Word, b *strings.Builder) error {
	p.i++
	for {
		if p.eof() {
			return p.errorf("a double-quoted string is not closed")
		}
		switch r := p.peek(0); r {
		case '"':
			if p.peek(1) == '"' {
				b.WriteRune('"')
				p.i += 2
				continue
			}
			p.i++
			return nil
		case '`':
			p.escape(b)
		case '$':
			if err := p.variable(w, b, true); err != nil {
				return err
			}
		default:
			b.WriteRune(r)
			p.i++
		}
	}
}

// escape reads a backtick escape inside a double-quoted string.
func (p *psParser) escape(b *strings.Builder) {
	if p.i+1 >= len(p.src) {
		p.i++
		return
	}
	c := p.src[p.i+1]
	p.i += 2
	switch c {
	case '0':
	case 'a':
		b.WriteRune('\a')
	case 'b':
		b.WriteRune('\b')
	case 'e':
		b.WriteRune(0x1b)
	case 'f':
		b.WriteRune('\f')
	case 'n':
		b.WriteRune('\n')
	case 'r':
		b.WriteRune('\r')
	case 't':
		b.WriteRune('\t')
	case 'v':
		b.WriteRune('\v')
	case 'u':
		if p.peek(0) == '{' {
			end := slices.Index(p.src[p.i:], '}')
			if end > 0 {
				if hex := string(p.src[p.i+1 : p.i+end]); len(hex) <= 6 && strings.Trim(hex, "0123456789abcdefABCDEF") == "" {
					b.WriteRune(digitsValue(hex, 16))
					p.i += end + 1
					return
				}
			}
		}
		b.WriteRune(c)
	default:
		b.WriteRune(c)
	}
}

// variable reads $name, ${name}, $(…) or a lone $. $true, $false and
// $null are constants and stay literal.
func (p *psParser) variable(w *Word, b *strings.Builder, quoted bool) error {
	expansion := func() {
		w.Literal = false
		if !quoted {
			w.Unknown = true
		}
		b.WriteString(ExpansionMark)
	}
	switch next := p.peek(1); {
	case next == '(':
		p.i += 2
		if err := p.statements(')', false); err != nil {
			return err
		}
		expansion()
	case next == '{':
		end := slices.Index(p.src[p.i:], '}')
		if end < 0 {
			return p.errorf("a variable ${ is not closed")
		}
		p.i += end + 1
		expansion()
	case next == '$' || next == '?' || next == '^':
		p.i += 2
		expansion()
	case unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_':
		j := p.i + 1
		for j < len(p.src) && (unicode.IsLetter(p.src[j]) || unicode.IsDigit(p.src[j]) || p.src[j] == '_' ||
			(p.src[j] == ':' && j+1 < len(p.src) && (unicode.IsLetter(p.src[j+1]) || p.src[j+1] == '_'))) {
			j++
		}
		name := string(p.src[p.i+1 : j])
		p.i = j
		switch strings.ToLower(name) {
		case "true", "false", "null":
			b.WriteString("$" + name)
		default:
			expansion()
		}
	default:
		b.WriteRune('$')
		p.i++
	}
	return nil
}

// hereStart reports whether @' or @" at the cursor opens a here-string:
// the rest of its line is blank.
func (p *psParser) hereStart() bool {
	j := p.i + 2
	for j < len(p.src) && isBlank(p.src[j]) {
		j++
	}
	return j < len(p.src) && p.src[j] == '\n'
}

// hereString reads @'…'@ or @"…"@. Its body starts on the next line and
// ends at a line that starts with '@ or "@.
func (p *psParser) hereString(w *Word) error {
	quote := p.peek(1)
	if !p.hereStart() {
		return p.errorf("a here-string must end its first line after @%c", quote)
	}
	p.i += 2
	for p.peek(0) != '\n' {
		p.i++
	}
	p.i++
	*w = Word{Literal: true, Quoted: true}
	var b strings.Builder
	for {
		if p.eof() {
			return p.errorf("a here-string @%c is not closed", quote)
		}
		lineStart := p.i == 0 || p.src[p.i-1] == '\n'
		if lineStart && p.peek(0) == quote && p.peek(1) == '@' {
			p.i += 2
			w.Text = strings.TrimSuffix(strings.TrimSuffix(b.String(), "\n"), "\r")
			return nil
		}
		r := p.peek(0)
		switch {
		case quote == '"' && r == '$':
			if err := p.variable(w, &b, true); err != nil {
				return err
			}
		case quote == '"' && r == '`':
			p.escape(&b)
		default:
			b.WriteRune(r)
			p.i++
		}
	}
}
