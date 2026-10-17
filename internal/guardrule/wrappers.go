package guardrule

import (
	"strings"
)

// Wrappers run another command given as their arguments (spec 0053,
// "Parsing"): sudo, env with its assignments, command, builtin, exec,
// nohup, time, nice and xargs; and, beyond the spec's list, timeout and
// stdbuf. find -exec is handled apart, since find is also a command of
// its own.

// unwrap returns the command a wrapper runs, and the wrapper's label.
// ok is false when c is no wrapper, or a wrapper that runs nothing.
func unwrap(c Command) (inner Command, label string, ok bool) {
	name := c.Name()
	if len(c.Words) < 2 {
		return Command{}, "", false
	}
	args := c.Words[1:]
	var rest []Word
	switch name {
	case "sudo", "doas":
		rest = skipOptions(args, options{withArg: "ugCDhpRrtTU", long: []string{
			"--user", "--group", "--host", "--close-from", "--chdir", "--prompt", "--chroot",
			"--role", "--type", "--command-timeout", "--other-user"}})
		rest = skipAssignments(rest)
	case "env":
		if hasSplitString(args) {
			return Command{}, "", false
		}
		rest = skipOptions(args, options{withArg: "uCP", long: []string{"--unset", "--chdir"}})
		rest = skipAssignments(rest)
	case "command":
		for _, w := range args {
			if !w.Literal || !strings.HasPrefix(w.Text, "-") || w.Text == "--" {
				break
			}
			if strings.ContainsAny(w.Text, "vV") {
				return Command{}, "", false // prints what the name is, runs nothing
			}
		}
		rest = skipOptions(args, options{})
	case "builtin", "nohup":
		rest = skipOptions(args, options{})
	case "exec":
		rest = skipOptions(args, options{withArg: "a"})
	case "time":
		rest = skipOptions(args, options{withArg: "fo", long: []string{"--format", "--output"}})
	case "nice":
		rest = skipOptions(args, options{withArg: "n", long: []string{"--adjustment"}})
	case "timeout":
		rest = skipOptions(args, options{withArg: "sk", long: []string{"--signal", "--kill-after"}})
		if len(rest) > 0 {
			rest = rest[1:] // the duration
		}
	case "stdbuf":
		rest = skipOptions(args, options{withArg: "ioe", long: []string{"--input", "--output", "--error"}})
	case "xargs":
		return unwrapXargs(c)
	default:
		return Command{}, "", false
	}
	if len(rest) == 0 {
		return Command{}, "", false
	}
	return Command{Words: rest}, name, true
}

type options struct {
	// withArg lists the short options that take an argument.
	withArg string
	// long lists the long options that take an argument (as --opt arg or
	// --opt=arg).
	long []string
}

// skipOptions returns the words after a wrapper's options: the command
// it runs. A non-literal word ends the options, so a dynamic option list
// makes the command name dynamic.
func skipOptions(args []Word, o options) []Word {
	for i := 0; i < len(args); i++ {
		w := args[i]
		if !w.Literal {
			return args[i:]
		}
		t := w.Text
		switch {
		case t == "--":
			return args[i+1:]
		case strings.HasPrefix(t, "--"):
			if !strings.Contains(t, "=") {
				for _, l := range o.long {
					if t == l {
						i++
					}
				}
			}
		case len(t) > 1 && t[0] == '-':
			for j := 1; j < len(t); j++ {
				if strings.IndexByte(o.withArg, t[j]) >= 0 {
					if j == len(t)-1 {
						i++
					}
					break
				}
			}
		default:
			return args[i:]
		}
	}
	return nil
}

// skipAssignments drops leading NAME=value words.
func skipAssignments(args []Word) []Word {
	for len(args) > 0 && args[0].Literal && isAssignment(args[0].Text) {
		args = args[1:]
	}
	return args
}

func isAssignment(s string) bool {
	name, _, ok := strings.Cut(s, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func hasSplitString(args []Word) bool {
	for _, w := range args {
		if !w.Literal || !strings.HasPrefix(w.Text, "-") || w.Text == "--" {
			return false
		}
		if strings.HasPrefix(w.Text, "--split-string") ||
			(!strings.HasPrefix(w.Text, "--") && strings.Contains(w.Text, "S")) {
			return true
		}
	}
	return false
}

// envSplitString finds env -S's string, which env splits into the
// command it runs, followed by env's other operands.
func envSplitString(args []Word) (nestedScript, bool) {
	for i := 0; i < len(args); i++ {
		w := args[i]
		if !w.Literal || !strings.HasPrefix(w.Text, "-") || w.Text == "--" {
			return nestedScript{}, false
		}
		var s []Word
		switch {
		case strings.HasPrefix(w.Text, "--split-string="):
			s = append([]Word{{Text: strings.TrimPrefix(w.Text, "--split-string="), Literal: true}}, args[i+1:]...)
		case w.Text == "--split-string":
			s = args[i+1:]
		case !strings.HasPrefix(w.Text, "--") && strings.Contains(w.Text, "S"):
			if j := strings.IndexByte(w.Text, 'S'); j < len(w.Text)-1 {
				s = append([]Word{{Text: w.Text[j+1:], Literal: true}}, args[i+1:]...)
			} else {
				s = args[i+1:]
			}
		default:
			continue
		}
		if len(s) == 0 {
			return nestedScript{}, false
		}
		// The string is split into words and then followed by the other
		// operands, which are already single words.
		parts := []string{scriptOf(ShellBash, s[:1])}
		for _, o := range s[1:] {
			parts = append(parts, bashQuoteWord(o))
		}
		return nestedScript{ShellBash, strings.Join(parts, " "), "env -S"}, true
	}
	return nestedScript{}, false
}

func bashQuoteWord(w Word) string {
	if !w.Literal {
		return bashUnknown
	}
	return "'" + strings.ReplaceAll(w.Text, "'", `'\''`) + "'"
}

// unwrapXargs returns the command xargs runs. xargs appends words read
// from its input, which can be anything, so an Unknown word is added at
// the end. With a replacement string (-I), each word holding it is
// Unknown instead.
func unwrapXargs(c Command) (Command, string, bool) {
	args := c.Words[1:]
	replace := ""
	i := 0
loop:
	for ; i < len(args); i++ {
		w := args[i]
		if !w.Literal {
			break
		}
		t := w.Text
		switch {
		case t == "--":
			i++
			break loop
		case strings.HasPrefix(t, "--replace"):
			replace = "{}"
			if v, ok := strings.CutPrefix(t, "--replace="); ok {
				replace = v
			}
		case strings.HasPrefix(t, "--"):
			for _, l := range []string{"--arg-file", "--delimiter", "--eof", "--max-lines", "--max-args",
				"--max-procs", "--max-chars", "--process-slot-var"} {
				if t == l {
					i++
				}
			}
		case len(t) > 1 && t[0] == '-':
			for j := 1; j < len(t); j++ {
				ch := t[j]
				if ch == 'i' { // -i[R]: optional argument, attached only
					replace = "{}"
					if j < len(t)-1 {
						replace = t[j+1:]
					}
					break
				}
				if ch == 'e' || ch == 'l' { // optional argument, attached only
					break
				}
				if strings.IndexByte("adEILnPs", ch) >= 0 {
					arg := t[j+1:]
					if arg == "" && i+1 < len(args) {
						i++
						arg = args[i].Text
					}
					if ch == 'I' {
						replace = arg
					}
					break
				}
			}
		default:
			break loop
		}
	}
	if i >= len(args) {
		return Command{}, "", false // runs echo
	}
	words := append([]Word(nil), args[i:]...)
	if replace == "" {
		words = append(words, Word{Text: ExpansionMark, Unknown: true})
	} else {
		for k := range words {
			if words[k].Literal && strings.Contains(words[k].Text, replace) {
				words[k] = Word{Text: ExpansionMark, Unknown: true}
			}
		}
	}
	return Command{Words: words}, "xargs", true
}

func endsExec(w Word) bool { return w.Literal && (w.Text == ";" || w.Text == "+") }

// findExecs returns the commands that find's -exec, -execdir, -ok and
// -okdir actions run. Each ends at ";" or "+".
func findExecs(c Command) []Command {
	var out []Command
	words := c.Words[1:]
	for i := 0; i < len(words); i++ {
		w := words[i]
		if !w.Literal {
			continue
		}
		switch w.Text {
		case "-exec", "-execdir", "-ok", "-okdir":
		default:
			continue
		}
		j := i + 1
		for j < len(words) && !endsExec(words[j]) {
			j++
		}
		if j > i+1 {
			out = append(out, Command{
				Words: append([]Word(nil), words[i+1:j]...), Shell: ShellBash,
				Depth: c.Depth, Via: appendVia(c.Via, "find "+w.Text),
			})
		}
		i = j
	}
	return out
}
