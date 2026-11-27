package guardrule

import (
	"slices"
	"strings"
)

// Wrappers run another command given as their arguments (spec 0053,
// "Parsing"): sudo, env with its assignments, command, builtin, exec,
// nohup, time, nice and xargs; and, beyond the spec's list, timeout and
// stdbuf. find -exec is handled apart, since find is also a command of
// its own.
//
// Each wrapper's options are read by its table, as the program reads
// them (spec 0062): short option clusters, long options written in full
// or as a unique prefix, --opt=value and "--". A long option the table
// does not know, or a prefix of options that differ in whether they take
// an argument, is read both ways, so a wrapper may run more than one
// inner command.

// longOpts lists long options of one argument kind, for a table.
func longOpts(kind argKind, names ...string) map[string]argKind {
	m := make(map[string]argKind, len(names))
	for _, n := range names {
		m[n] = kind
	}
	return m
}

// longOptions joins the lists of longOpts.
func longOptions(lists ...map[string]argKind) map[string]argKind {
	m := map[string]argKind{}
	for _, l := range lists {
		for n, k := range l {
			m[n] = k
		}
	}
	return m
}

// wrapperTable is a wrapper's option table. A wrapper stops reading
// options at its first operand, the command it runs. Its long options
// are all listed, so one it does not list is read both ways.
func wrapperTable(shortArg, shortOptArg string, long map[string]argKind) optionTable {
	return optionTable{
		shortArg: shortArg, shortOptArg: shortOptArg, long: long,
		abbrev: true, minAbbrev: 1, unknownLong: argUnknown,
	}
}

var helpVersion = longOpts(argNone, "help", "version")

// wrapperTables follow the programs' documentation: sudo 1.9, GNU
// coreutils (env, nice, nohup, stdbuf, timeout), GNU time and GNU
// findutils (xargs). doas has no long options; command, builtin and exec
// are shell builtins and have none.
var wrapperTables = map[string]optionTable{
	"sudo": wrapperTable("aCcDghpRrTtUu", "", longOptions(
		longOpts(argNone, "askpass", "background", "bell", "edit", "set-home", "help", "login",
			"remove-timestamp", "reset-timestamp", "list", "non-interactive", "no-update",
			"preserve-groups", "stdin", "shell", "version", "validate"),
		longOpts(argRequired, "auth-type", "close-from", "login-class", "chdir", "chroot", "group",
			"host", "prompt", "role", "type", "command-timeout", "user", "other-user", "apparmor-profile"),
		longOpts(argOptional, "preserve-env"))),
	"doas": wrapperTable("aCu", "", nil),
	"env": func() optionTable {
		t := wrapperTable("aCPSu", "", longOptions(helpVersion,
			longOpts(argNone, "ignore-environment", "null", "list-signal-handling", "debug"),
			longOpts(argRequired, "unset", "chdir", "split-string", "argv0"),
			longOpts(argOptional, "block-signal", "default-signal", "ignore-signal")))
		t.ends = []string{"S", "--split-string"} // the split string, read by splitString
		return t
	}(),
	"command": wrapperTable("", "", nil),
	"builtin": wrapperTable("", "", nil),
	"exec":    wrapperTable("a", "", nil),
	"nohup":   wrapperTable("", "", helpVersion),
	"time": wrapperTable("fo", "", longOptions(helpVersion,
		longOpts(argNone, "append", "portability", "quiet", "verbose"),
		longOpts(argRequired, "format", "output"))),
	"nice": wrapperTable("n", "", longOptions(helpVersion, longOpts(argRequired, "adjustment"))),
	"timeout": wrapperTable("ks", "", longOptions(helpVersion,
		longOpts(argNone, "foreground", "preserve-status", "verbose"),
		longOpts(argRequired, "kill-after", "signal"))),
	"stdbuf": wrapperTable("ioe", "", longOptions(helpVersion, longOpts(argRequired, "input", "output", "error"))),
	"xargs": wrapperTable("adEILnPs", "eil", longOptions(helpVersion,
		longOpts(argNone, "null", "open-tty", "interactive", "no-run-if-empty", "show-limits", "verbose", "exit"),
		longOpts(argRequired, "arg-file", "delimiter", "max-args", "max-chars", "max-procs", "process-slot-var"),
		longOpts(argOptional, "eof", "replace", "max-lines"))),
}

// unwrap returns the commands a wrapper runs, one for each reading of
// its options that runs one, and the wrapper's label. inners is empty
// when c is no wrapper, or a wrapper that runs nothing. A wrapper whose
// options give too many readings runs a command whose name is not known.
func unwrap(c Command) (inners []Command, label string) {
	name := c.Name()
	t, ok := wrapperTables[name]
	if !ok || len(c.Words) < 2 {
		return nil, ""
	}
	readings, ok := readOptions(c.Words[1:], t)
	if !ok {
		return []Command{{Words: []Word{{Text: ExpansionMark, Unknown: true}}}}, name
	}
	for _, r := range readings {
		inner, runs := wrapped(name, r)
		if runs && !slices.ContainsFunc(inners, inner.sameWords) {
			inners = append(inners, inner)
		}
	}
	return inners, name
}

// wrapped returns the command that wrapper name runs in reading r.
func wrapped(name string, r optionReading) (Command, bool) {
	rest := r.operands
	var assigns []Word
	switch name {
	case "env":
		if splitString(r) != nil {
			return Command{}, false // scriptsOf reads the split string
		}
		assigns, rest = splitAssignments(rest)
	case "sudo", "doas":
		assigns, rest = splitAssignments(rest)
	case "command":
		if r.has("v", "V") {
			return Command{}, false // prints what the name is, runs nothing
		}
	case "timeout":
		if len(rest) > 0 {
			rest = rest[1:] // the duration
		}
	case "xargs":
		return xargsCommand(r)
	}
	if len(rest) == 0 {
		return Command{}, false
	}
	return Command{Words: rest, Assigns: assigns}, true
}

// sameWords reports whether c and d have the same words and assignments.
func (c Command) sameWords(d Command) bool {
	eq := func(a, b Word) bool { return a == b }
	return slices.EqualFunc(c.Words, d.Words, eq) && slices.EqualFunc(c.Assigns, d.Assigns, eq)
}

// splitAssignments splits leading NAME=value words from the words after
// them.
func splitAssignments(args []Word) (assigns, rest []Word) {
	n := 0
	for n < len(args) && args[n].Literal && isAssignment(args[n].Text) {
		n++
	}
	return args[:n], args[n:]
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

// splitString returns the words env splits into the command it runs, in
// reading r: env -S's string, followed by env's other operands. It is
// nil when r has no split string.
func splitString(r optionReading) []Word {
	if len(r.opts) == 0 {
		return nil
	}
	// The options that end env's options are those that give its split
	// string.
	o := r.opts[len(r.opts)-1]
	if !slices.Contains(wrapperTables["env"].ends, o.name) || !o.hasValue {
		return nil
	}
	return append([]Word{o.value}, r.operands...)
}

// envSplitStrings returns the scripts env -S runs, one for each reading
// of env's options that has a split string.
func envSplitStrings(args []Word) []nestedScript {
	readings, ok := readOptions(args, wrapperTables["env"])
	if !ok {
		return nil // unwrap runs a command whose name is not known
	}
	var out []nestedScript
	for _, r := range readings {
		s := splitString(r)
		if s == nil {
			continue
		}
		// The string is split into words and then followed by the other
		// operands, which are already single words.
		parts := []string{scriptOf(ShellBash, s[:1])}
		for _, o := range s[1:] {
			parts = append(parts, bashQuoteWord(o))
		}
		n := nestedScript{ShellBash, strings.Join(parts, " "), "env -S"}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

func bashQuoteWord(w Word) string {
	if !w.Literal {
		return bashUnknown
	}
	return "'" + strings.ReplaceAll(w.Text, "'", `'\''`) + "'"
}

// xargsCommand returns the command xargs runs in reading r. xargs
// appends words read from its input, which can be anything, so an
// Unknown word is added at the end. With a replacement string (-I), each
// word holding it is Unknown instead.
func xargsCommand(r optionReading) (Command, bool) {
	replace := ""
	for _, o := range r.opts {
		switch o.name {
		case "I":
			replace = o.value.Text
		case "i", "--replace":
			replace = "{}"
			if o.hasValue {
				replace = o.value.Text
			}
		}
	}
	if len(r.operands) == 0 {
		return Command{}, false // runs echo
	}
	words := append([]Word(nil), r.operands...)
	if replace == "" {
		words = append(words, Word{Text: ExpansionMark, Unknown: true})
	} else {
		for k := range words {
			if words[k].Literal && strings.Contains(words[k].Text, replace) {
				words[k] = Word{Text: ExpansionMark, Unknown: true}
			}
		}
	}
	return Command{Words: words}, true
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
