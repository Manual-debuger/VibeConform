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
	// optArg lists the short options whose argument is optional and only
	// attached (sed -i.bak). Only the writers read it.
	optArg string
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

// Writers are the commands whose operands name a file they write,
// delete, move or rename (spec 0058, Desired Behavior 2). The tables are
// a closed list, as the wrappers above are: a command not in them writes
// no operand, whatever it does.

// writtenOperands returns the operands c writes, in the order the
// writer's reading gives them. A PowerShell command is read as a cmdlet
// and, when its name is also a program here (rm, cp, tee, …), as that
// program too, since the alias is Windows-only.
func writtenOperands(c Command) []Word {
	if len(c.Words) == 0 {
		return nil
	}
	var out []Word
	if c.Shell == ShellPowerShell {
		cmdlet := c.Cmdlet()
		if full, ok := psWriterAliases[cmdlet]; ok {
			cmdlet = full
		}
		if w, ok := psWriters[cmdlet]; ok {
			out = append(out, w.written(c.Words[1:])...)
		}
	}
	if w, ok := writers[c.Name()]; ok {
		for _, p := range w(c.Words[1:]) {
			if !slices.ContainsFunc(out, func(o Word) bool { return o.Text == p.Text }) {
				out = append(out, p)
			}
		}
	}
	return out
}

// writers reads a program's arguments (without its name) by getopt's
// conventions and returns the operands it writes.
var writers = map[string]func(args []Word) []Word{
	"tee": func(args []Word) []Word { return scanArgs(args, options{}).operands },
	"sed": writtenBySed,
	"truncate": func(args []Word) []Word {
		return scanArgs(args, options{withArg: "sr", long: []string{"--size", "--reference"}}).operands
	},
	"cp": copyWriter("cp", copyOptions),
	"mv": copyWriter("mv", copyOptions),
	"ln": copyWriter("ln", copyOptions),
	"install": copyWriter("install", options{withArg: "gmoSt", long: []string{
		"--group", "--mode", "--owner", "--suffix", "--target-directory", "--strip-program"}}),
	"rm":     func(args []Word) []Word { return scanArgs(args, options{}).operands },
	"unlink": func(args []Word) []Word { return scanArgs(args, options{}).operands },
	"dd":     writtenByDd,
}

var copyOptions = options{withArg: "St", long: []string{"--suffix", "--target-directory"}}

// parsedArgs is a program's argument list read by getopt's conventions.
type parsedArgs struct {
	operands []Word
	// opts are the options given, short ones without "-" ("t"), long ones
	// with "--" and without a value ("--target-directory"); values holds
	// each one's argument, or "" for none.
	opts, values []string
}

// has reports whether one of names was given. A long name also matches
// an abbreviation of at least two letters, as getopt_long accepts one.
func (a parsedArgs) has(names ...string) bool { _, ok := a.value(names...); return ok }

func (a parsedArgs) value(names ...string) (string, bool) {
	for i, o := range a.opts {
		for _, n := range names {
			if o == n || strings.HasPrefix(n, "--") && len(o) >= 4 && strings.HasPrefix(n, o) {
				return a.values[i], true
			}
		}
	}
	return "", false
}

// scanArgs reads args with o's options taking an argument; sed's -i
// takes an optional one, attached. Options may follow operands, and "--"
// ends them. A word with an expansion is an operand.
func scanArgs(args []Word, o options) parsedArgs {
	var a parsedArgs
	add := func(opt, value string) { a.opts, a.values = append(a.opts, opt), append(a.values, value) }
	for i := 0; i < len(args); i++ {
		w := args[i]
		t := w.Text
		switch {
		case !w.Literal || t == "-" || !strings.HasPrefix(t, "-"):
			a.operands = append(a.operands, w)
		case t == "--":
			a.operands = append(a.operands, args[i+1:]...)
			return a
		case strings.HasPrefix(t, "--"):
			name, value, hasValue := strings.Cut(t, "=")
			if !hasValue && slices.ContainsFunc(o.long, func(l string) bool {
				return name == l || len(name) >= 4 && strings.HasPrefix(l, name)
			}) && i+1 < len(args) {
				i++
				value = args[i].Text
			}
			add(name, value)
		default:
			for j := 1; j < len(t); j++ {
				ch := t[j : j+1]
				switch {
				case strings.Contains(o.withArg, ch):
					value := t[j+1:]
					if value == "" && i+1 < len(args) {
						i++
						value = args[i].Text
					}
					add(ch, value)
				case strings.Contains(o.optArg, ch):
					add(ch, t[j+1:])
				default:
					add(ch, "")
					continue
				}
				break
			}
		}
	}
	return a
}

// writtenBySed: with -i or --in-place, the files sed edits. The first
// operand is the script unless -e or -f gave one.
func writtenBySed(args []Word) []Word {
	a := scanArgs(args, options{withArg: "efl", optArg: "i", long: []string{"--expression", "--file", "--line-length"}})
	if !a.has("i", "--in-place") {
		return nil
	}
	files := a.operands
	if !a.has("e", "f", "--expression", "--file") && len(files) > 0 {
		files = files[1:]
	}
	return files
}

// copyWriter reads cp, mv, ln and install: each writes its destination,
// the last operand or the directory after -t. A destination may be a
// directory, so it also writes each source's name inside it, as
// "cp settings.json .claude" writes .claude/settings.json. mv writes its
// sources too, since it removes them. ln with one operand links it into
// the current directory under its own name; install -d creates every
// operand.
func copyWriter(name string, o options) func([]Word) []Word {
	return func(args []Word) []Word {
		a := scanArgs(args, o)
		if name == "install" && a.has("d", "--directory") {
			return a.operands
		}
		var dest Word
		sources := a.operands
		switch target, ok := a.value("t", "--target-directory"); {
		case ok:
			dest = Word{Text: target, Literal: !strings.Contains(target, ExpansionMark)}
		case len(sources) >= 2:
			dest, sources = sources[len(sources)-1], sources[:len(sources)-1]
		case len(sources) == 1 && name == "ln":
			s := sources[0]
			return []Word{{Text: base(s.Text), Literal: s.Literal, Quoted: s.Quoted}}
		default:
			return nil
		}
		out := []Word{dest}
		if !a.has("T", "--no-target-directory") {
			for _, s := range sources {
				out = append(out, inside(dest, s))
			}
		}
		if name == "mv" {
			out = append(out, sources...)
		}
		return out
	}
}

// inside is the word for source's file name inside the directory dir.
func inside(dir, source Word) Word {
	return Word{
		Text:    strings.TrimRight(dir.Text, `/\`) + "/" + base(source.Text),
		Literal: dir.Literal && source.Literal,
		Quoted:  dir.Quoted || source.Quoted,
	}
}

// writtenByDd: the file after of=.
func writtenByDd(args []Word) []Word {
	var out []Word
	for _, w := range args {
		if p, ok := strings.CutPrefix(w.Text, "of="); ok {
			w.Text = p
			out = append(out, w)
		}
	}
	return out
}

// psWriter is a cmdlet that writes, deletes, moves or renames the item
// its path parameters name.
type psWriter struct {
	// params are the parameters that take a value; any other parameter is
	// a switch. The first of them is the first positional parameter, the
	// second the second when positional is 2.
	params     []string
	positional int
	// paths are the parameters that name a written item.
	paths []string
	// kind is "" for a cmdlet that writes its paths, or copy, move,
	// rename or new for the cmdlets that also write a second name.
	kind string
}

var psPathParams = []string{"Path", "LiteralPath"}

// psWriters are keyed by the cmdlet's lower-case name; psWriterAliases
// adds the aliases that Command.Cmdlet does not resolve.
var psWriters = map[string]psWriter{
	"set-content":   {params: []string{"Path", "Value", "LiteralPath", "Encoding", "Filter", "Include", "Exclude", "Stream", "Delimiter", "Credential"}, positional: 2, paths: psPathParams},
	"add-content":   {params: []string{"Path", "Value", "LiteralPath", "Encoding", "Filter", "Include", "Exclude", "Stream", "Delimiter", "Credential"}, positional: 2, paths: psPathParams},
	"out-file":      {params: []string{"FilePath", "Encoding", "LiteralPath", "Path", "Width", "InputObject"}, positional: 2, paths: []string{"FilePath", "LiteralPath", "Path"}},
	"clear-content": {params: []string{"Path", "LiteralPath", "Filter", "Include", "Exclude", "Stream", "Credential"}, positional: 1, paths: psPathParams},
	"remove-item":   {params: []string{"Path", "LiteralPath", "Filter", "Include", "Exclude", "Stream", "Credential"}, positional: 1, paths: psPathParams},
	"tee-object":    {params: []string{"FilePath", "LiteralPath", "Path", "Variable", "InputObject", "Encoding"}, positional: 1, paths: []string{"FilePath", "LiteralPath", "Path"}},
	"rename-item":   {params: []string{"Path", "NewName", "LiteralPath", "Credential"}, positional: 2, paths: psPathParams, kind: "rename"},
	"new-item":      {params: []string{"Path", "Name", "ItemType", "Type", "Value", "Target", "Credential"}, positional: 1, paths: psPathParams, kind: "new"},
	"copy-item":     {params: []string{"Path", "Destination", "LiteralPath", "Filter", "Include", "Exclude", "Credential", "ToSession", "FromSession"}, positional: 2, paths: psPathParams, kind: "copy"},
	"move-item":     {params: []string{"Path", "Destination", "LiteralPath", "Filter", "Include", "Exclude", "Credential"}, positional: 2, paths: psPathParams, kind: "move"},
}

var psWriterAliases = map[string]string{
	"sc": "set-content", "ac": "add-content", "clc": "clear-content", "tee": "tee-object",
	"rni": "rename-item", "ren": "rename-item", "ni": "new-item",
	"cp": "copy-item", "copy": "copy-item", "cpi": "copy-item", "mv": "move-item", "move": "move-item", "mi": "move-item",
}

// written reads a cmdlet's arguments (without its name) and returns the
// items it writes.
func (p psWriter) written(args []Word) []Word {
	values := map[string][]Word{}
	var positional []Word
	for i := 0; i < len(args); i++ {
		w := args[i]
		if w.Literal && !w.Quoted && len(w.Text) > 1 && w.Text[0] == '-' {
			param := ""
			for _, name := range p.params {
				if ok, _ := PSParam(w, name); ok {
					param = name
					break
				}
			}
			if param == "" {
				continue // a switch
			}
			if _, value, attached := strings.Cut(w.Text, ":"); attached {
				values[param] = append(values[param], Word{Text: value, Literal: true})
			} else if i+1 < len(args) {
				i++
				values[param] = append(values[param], args[i])
			}
			continue
		}
		positional = append(positional, w)
	}
	// Positional words go to the first positional parameters; with one
	// positional parameter, every such word is part of its list (a, b).
	for i, w := range positional {
		k := min(i, p.positional-1)
		if p.positional == 2 && i > 1 {
			break
		}
		values[p.params[k]] = append(values[p.params[k]], w)
	}
	var paths []Word
	for _, name := range p.paths {
		paths = append(paths, values[name]...)
	}
	switch p.kind {
	case "copy", "move":
		var out []Word
		for _, dest := range values["Destination"] {
			out = append(out, dest)
			for _, s := range paths {
				out = append(out, inside(dest, s))
			}
		}
		if p.kind == "move" {
			out = append(out, paths...)
		}
		return out
	case "rename":
		out := paths
		for _, name := range values["NewName"] {
			for _, s := range paths {
				dir := Word{Text: ".", Literal: true}
				if i := strings.LastIndexAny(s.Text, `/\`); i >= 0 {
					dir = Word{Text: s.Text[:i], Literal: s.Literal, Quoted: s.Quoted}
				}
				out = append(out, inside(dir, name))
			}
		}
		return out
	case "new":
		out := paths
		for _, name := range values["Name"] {
			if len(paths) == 0 {
				out = append(out, name)
			}
			for _, s := range paths {
				out = append(out, inside(s, name))
			}
		}
		return out
	}
	return paths
}
