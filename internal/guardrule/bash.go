package guardrule

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// parseBash parses src as Bash (the Bash tool runs bash) and returns its
// simple commands in source order, outer before inner.
func parseBash(src string) ([]Command, error) {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
	if err != nil {
		return nil, &ParseError{Shell: ShellBash, Msg: err.Error()}
	}
	var out []Command
	syntax.Walk(f, func(n syntax.Node) bool {
		st, ok := n.(*syntax.Stmt)
		if !ok {
			return true
		}
		c := Command{Shell: ShellBash}
		for _, r := range st.Redirs {
			switch r.Op {
			case syntax.Hdoc, syntax.DashHdoc:
				if r.Hdoc != nil {
					w := bashWord(r.Hdoc)
					c.stdin = &w
				}
			case syntax.WordHdoc:
				w := bashWord(r.Word)
				c.stdin = &w
			case syntax.DplIn, syntax.DplOut:
			default:
				c.Redirects = append(c.Redirects, bashWord(r.Word))
			}
		}
		if call, ok := st.Cmd.(*syntax.CallExpr); ok && len(call.Args) > 0 {
			for _, a := range call.Args {
				c.Words = append(c.Words, bashWord(a))
			}
			out = append(out, c)
		} else if len(c.Redirects) > 0 {
			c.stdin = nil
			out = append(out, c)
		}
		return true
	})
	return out, nil
}

// bashWord performs quote removal on a word. An unquoted expansion, or a
// brace expansion, makes the word Unknown.
func bashWord(w *syntax.Word) Word {
	out := Word{Literal: true}
	braces := &syntax.Word{Parts: slices.Clone(w.Parts)}
	if syntax.SplitBraces(braces) && expandsBraces(braces) {
		out.Literal, out.Unknown = false, true
	}
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(Unescape(p.Value, false))
		case *syntax.SglQuoted:
			out.Quoted = true
			if p.Dollar {
				b.WriteString(ansiC(p.Value))
			} else {
				b.WriteString(p.Value)
			}
		case *syntax.DblQuoted:
			out.Quoted = true
			for _, q := range p.Parts {
				if l, ok := q.(*syntax.Lit); ok {
					b.WriteString(Unescape(l.Value, true))
					continue
				}
				out.Literal = false
				b.WriteString(ExpansionMark)
			}
		default:
			out.Literal, out.Unknown = false, true
			b.WriteString(ExpansionMark)
		}
	}
	out.Text = b.String()
	return out
}

// expandsBraces reports whether a word split by syntax.SplitBraces holds
// a brace expansion bash expands: a list of two or more elements or a
// sequence. "{}" and "{x}" stay literal.
func expandsBraces(w *syntax.Word) bool {
	for _, part := range w.Parts {
		be, ok := part.(*syntax.BraceExp)
		if !ok {
			continue
		}
		if be.Sequence || len(be.Elems) > 1 {
			return true
		}
		for _, e := range be.Elems {
			if expandsBraces(e) {
				return true
			}
		}
	}
	return false
}

// Unescape removes backslash quoting from literal shell text. Outside
// double quotes a backslash quotes any character; inside them only $ ` "
// \ and newline. A backslash before a newline joins the lines. POSIX and
// Bash agree on it, so the contract's POSIX parsing uses it too.
func Unescape(s string, inDouble bool) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		next := s[i+1]
		switch {
		case next == '\n':
			i++
		case !inDouble || strings.IndexByte("$`\"\\", next) >= 0:
			b.WriteByte(next)
			i++
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// ansiC decodes the body of a $'…' string, as bash does.
func ansiC(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'e', 'E':
			b.WriteByte(0x1b)
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\', '\'', '"', '?':
			b.WriteByte(c)
		case 'c':
			if i+1 < len(s) {
				i++
				b.WriteByte(s[i] & 0x1f)
			}
		case 'x', 'u', 'U':
			n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[c]
			j := i + 1
			for j < len(s) && j < i+1+n && isHex(s[j]) {
				j++
			}
			if j == i+1 {
				b.WriteByte('\\')
				b.WriteByte(c)
				continue
			}
			v := digitsValue(s[i+1:j], 16)
			if c == 'x' {
				writeLowByte(&b, v)
			} else {
				b.WriteRune(v)
			}
			i = j - 1
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j := i
			for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
				j++
			}
			writeLowByte(&b, digitsValue(s[i:j], 8))
			i = j - 1
		default:
			b.WriteByte('\\')
			b.WriteByte(c)
		}
	}
	return b.String()
}

// digitsValue is the value of a run of at most eight hex or octal
// digits, so it fits a rune.
func digitsValue(s string, base rune) rune {
	var v rune
	for _, d := range s {
		switch {
		case d >= '0' && d <= '9':
			v = v*base + d - '0'
		case d >= 'a' && d <= 'f':
			v = v*base + d - 'a' + 10
		case d >= 'A' && d <= 'F':
			v = v*base + d - 'A' + 10
		}
	}
	return v
}

// writeLowByte writes the low byte of v, as bash does for \xHH and \nnn.
func writeLowByte(b *strings.Builder, v rune) {
	b.WriteByte(byte(v & 0xff))
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
