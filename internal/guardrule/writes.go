package guardrule

import (
	"slices"
	"strings"
)

// Writers are the commands whose operands name a file they write,
// delete, move or rename (spec 0058, Desired Behavior 2). The tables are
// a closed list, as the wrappers in wrappers.go are: a command not in
// them writes no operand, whatever it does.

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
	"tee": readWriter(writerTable("", "", nil), allOperands),
	"sed": readWriter(writerTable("efl", "i", longOptions(
		longOpts(argRequired, "expression", "file", "line-length"),
		longOpts(argOptional, "in-place"))), writtenBySed),
	"truncate": readWriter(writerTable("sr", "", longOpts(argRequired, "size", "reference")), allOperands),
	"cp":       readWriter(copyOptions, copyWriter("cp")),
	"mv":       readWriter(copyOptions, copyWriter("mv")),
	"ln":       readWriter(copyOptions, copyWriter("ln")),
	"install": readWriter(writerTable("gmoSt", "", longOptions(copyLong,
		longOpts(argRequired, "group", "mode", "owner", "strip-program"),
		longOpts(argNone, "directory"))), copyWriter("install")),
	"rm":     readWriter(writerTable("", "", nil), allOperands),
	"unlink": readWriter(writerTable("", "", nil), allOperands),
	"dd":     writtenByDd,
}

var (
	copyLong = longOptions(
		longOpts(argRequired, "suffix", "target-directory"),
		longOpts(argNone, "no-target-directory"))
	copyOptions = writerTable("St", "", copyLong)
)

// writerTable is a writer's option table: the short options that take an
// argument, those whose argument is optional and attached, and the long
// options the writer reads. Any other long option takes no argument.
// Options may follow operands, and a long option may be abbreviated to
// two letters or more ("--ta", four characters with its dashes).
func writerTable(shortArg, shortOptArg string, long map[string]argKind) optionTable {
	return optionTable{
		shortArg: shortArg, shortOptArg: shortOptArg, long: long,
		abbrev: true, minAbbrev: 2, unknownLong: argNone, permute: true,
	}
}

// readWriter reads a writer's arguments by its table t and returns what
// written writes in each reading. The scan fails closed: when the
// options read more than one way, the writer writes what any reading
// writes; when they give too many readings to read, every argument is
// written. writtenOperands drops the repeats.
func readWriter(t optionTable, written func(optionReading) []Word) func([]Word) []Word {
	return func(args []Word) []Word {
		readings, ok := readOptions(args, t)
		if !ok {
			return args
		}
		var out []Word
		for _, r := range readings {
			out = append(out, written(r)...)
		}
		return out
	}
}

// allOperands: the writer writes every operand.
func allOperands(r optionReading) []Word { return r.operands }

// writtenBySed: with -i or --in-place, the files sed edits. The first
// operand is the script unless -e or -f gave one.
func writtenBySed(r optionReading) []Word {
	if !r.has("i", "--in-place") {
		return nil
	}
	files := r.operands
	if !r.has("e", "f", "--expression", "--file") && len(files) > 0 {
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
func copyWriter(name string) func(optionReading) []Word {
	return func(r optionReading) []Word {
		if name == "install" && r.has("d", "--directory") {
			return r.operands
		}
		var dest Word
		sources := r.operands
		switch target, ok := r.value("t", "--target-directory"); {
		case ok:
			dest = Word{Text: target.Text, Literal: !strings.Contains(target.Text, ExpansionMark)}
		case len(sources) >= 2:
			dest, sources = sources[len(sources)-1], sources[:len(sources)-1]
		case len(sources) == 1 && name == "ln":
			s := sources[0]
			return []Word{{Text: base(s.Text), Literal: s.Literal, Quoted: s.Quoted}}
		default:
			return nil
		}
		out := []Word{dest}
		if !r.has("T", "--no-target-directory") {
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
