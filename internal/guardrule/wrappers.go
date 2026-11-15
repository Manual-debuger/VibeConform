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
	"tee": func(args []Word) []Word { return scanArgs(args, writerTable("", "")).operands },
	"sed": writtenBySed,
	"truncate": func(args []Word) []Word {
		return scanArgs(args, writerTable("sr", "", "size", "reference")).operands
	},
	"cp": copyWriter("cp", copyOptions),
	"mv": copyWriter("mv", copyOptions),
	"ln": copyWriter("ln", copyOptions),
	"install": copyWriter("install", writerTable("gmoSt", "",
		"group", "mode", "owner", "suffix", "target-directory", "strip-program")),
	"rm":     func(args []Word) []Word { return scanArgs(args, writerTable("", "")).operands },
	"unlink": func(args []Word) []Word { return scanArgs(args, writerTable("", "")).operands },
	"dd":     writtenByDd,
}

var copyOptions = writerTable("St", "", "suffix", "target-directory")

// writerTable is a writer's option table: the short options that take an
// argument, those whose argument is optional and attached, and the long
// options that take one. Any other long option takes none. Options may
// follow operands, and a long option may be abbreviated to two letters
// or more ("--ta", four characters with its dashes).
func writerTable(shortArg, shortOptArg string, long ...string) optionTable {
	return optionTable{
		shortArg: shortArg, shortOptArg: shortOptArg, long: longOpts(argRequired, long...),
		abbrev: true, minAbbrev: 2, unknownLong: argNone, permute: true,
	}
}

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

// scanArgs reads args by a writer's table t. "--" ends the options, and
// a word with an expansion is an operand.
func scanArgs(args []Word, t optionTable) parsedArgs {
	var a parsedArgs
	// A writer's table gives one reading: it reads no option both ways.
	readings, _ := readOptions(args, t)
	for _, r := range readings[:min(1, len(readings))] {
		a.operands = r.operands
		for _, o := range r.opts {
			a.opts, a.values = append(a.opts, o.name), append(a.values, o.value.Text)
		}
	}
	return a
}

// writtenBySed: with -i or --in-place, the files sed edits. The first
// operand is the script unless -e or -f gave one.
func writtenBySed(args []Word) []Word {
	a := scanArgs(args, writerTable("efl", "i", "expression", "file", "line-length"))
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
func copyWriter(name string, o optionTable) func([]Word) []Word {
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
func (p psWriter) written(args []Word) []Word { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
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
