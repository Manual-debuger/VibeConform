package guardrule

import (
	"fmt"
	"slices"
	"strings"
)

// Command decomposition (spec 0053, "vibe-guard", "Parsing"). A Bash
// command is parsed with mvdan.cc/sh (syntax.LangBash), a PowerShell
// command with the tokenizer in powershell.go. Every simple command is
// found, wherever it sits; wrappers such as sudo are stripped; and the
// strings that bash -c, eval, pwsh -Command and their kin run are parsed
// again, up to MaxDepth. Anything that cannot be parsed is an error, so
// that a guard refuses it (fails closed).
//
// The decomposition lives here, not in internal/guard, so that the audit
// (exception matching for A-02) and vibe-guard find the same words.

// Shell is the language a command is written in.
type Shell int

const (
	// ShellBash is the Bash tool's language. It is also the convention
	// for the words of an external program started by a wrapper.
	ShellBash Shell = iota + 1
	// ShellPowerShell is the PowerShell tool's language.
	ShellPowerShell
)

func (s Shell) String() string {
	switch s {
	case ShellBash:
		return "Bash"
	case ShellPowerShell:
		return "PowerShell"
	}
	return fmt.Sprintf("Shell(%d)", int(s))
}

// ShellFor returns the language of a command tool's command, or 0 for a
// tool that runs no command.
func ShellFor(tool string) Shell {
	switch tool {
	case Bash:
		return ShellBash
	case PowerShell:
		return ShellPowerShell
	}
	return 0
}

// MaxDepth is how deep nested shells are parsed again. A command nested
// deeper is a parse error.
const MaxDepth = 8

// ExpansionMark stands for an expansion (a variable, a command
// substitution, …) inside Word.Text.
const ExpansionMark = "\x00"

// ShowExpansions writes text with each ExpansionMark shown as $…, for a
// message.
func ShowExpansions(text string) string {
	return strings.ReplaceAll(text, ExpansionMark, "$…")
}

// Word is one word of a simple command after quote removal.
type Word struct {
	// Text is the word after quote removal, with each expansion replaced
	// by ExpansionMark.
	Text string
	// Literal is true when the word holds no expansion: Text is exactly
	// what the program receives. A PowerShell $true, $false or $null is
	// a constant, not an expansion.
	Literal bool
	// Unknown is true when the word holds an unquoted expansion (or a
	// Bash brace expansion). Such a word can become any number of words,
	// flags included. A word with only quoted expansions is one word, an
	// operand whose value is unknown: Literal and Unknown are both false.
	Unknown bool
	// Quoted is true when some part of the word was quoted. A quoted
	// PowerShell word is never a parameter.
	Quoted bool

	// block is a PowerShell script block ({ … }). Its commands are
	// already found; it is not a string to parse again.
	block bool
}

// Command is one simple command.
type Command struct {
	// Words are the command's words; Words[0] is the command name.
	// Leading assignments and redirections are not words. Words is empty
	// only for a redirection with no command (">f", "{ …; } >f"), which
	// is kept so that its target is seen.
	Words []Word
	// Assigns are the environment assignments the command runs with, as
	// NAME=value words: those before its name, then those of the env or
	// sudo wrappers it was found through. The contract audit does not
	// read Assigns.
	Assigns []Word
	// Redirects are the targets of the command's file redirections
	// (">f", ">>f", "<f", "2>f"). Here-documents and descriptor
	// duplications ("2>&1") are not targets.
	Redirects []Word
	// Writes are the paths the command writes, deletes, moves or renames
	// (spec 0058, "Write targets come from the parser"): its output
	// redirections, then the operands that the writers in writes.go
	// write. A word with an expansion is listed as it is; whether it
	// counts is the reader's to decide. The contract audit does not read
	// Writes.
	Writes []Word
	// Shell is the language whose conventions apply to Words. A command
	// started by a wrapper (sudo, env, xargs, find -exec, …) is an
	// external program, so it is ShellBash even inside PowerShell.
	Shell Shell
	// Depth is how many nested parses the command is inside: 0 for the
	// command as given, 1 inside bash -c '…', and so on.
	Depth int
	// Via lists the wrappers and nested shells the command was found
	// through, outermost first: "sudo", "env", "bash -c", "eval",
	// "pwsh -c", "iex", "find -exec", "bash <<", "env -S".
	Via []string
	// ScriptSource is where a POSIX shell (bash, sh, zsh, …) reads the
	// script it runs, as its options say (spec 0062, "A shell's script").
	// It is ScriptNone for any other command.
	ScriptSource ScriptSource

	// stdin is a here-document or here-string fed to the command.
	stdin *Word
	// envArgs marks env reading the words of an outer env's split string
	// as its arguments again. The command it runs is found through that
	// env -S alone.
	envArgs bool
}

// ScriptSource is where a POSIX shell reads the script it runs.
type ScriptSource int

const (
	// ScriptNone is for a command that is not a POSIX shell, or one that
	// runs no script (bash --version).
	ScriptNone ScriptSource = iota
	// ScriptFromString is a -c string. Its commands follow the shell's,
	// one level deeper, found through "<shell> -c".
	ScriptFromString
	// ScriptFromFile is a script file, the first operand.
	ScriptFromFile
	// ScriptFromStdin is standard input, piped or redirected from a file
	// (curl … | bash, bash -s x < x.sh). What runs is not seen.
	ScriptFromStdin
	// ScriptFromHereDoc is standard input fed by a here-document or
	// here-string. Its commands follow the shell's, one level deeper,
	// found through "<shell> <<".
	ScriptFromHereDoc
)

func (s ScriptSource) String() string {
	switch s {
	case ScriptNone:
		return "none"
	case ScriptFromString:
		return "-c string"
	case ScriptFromFile:
		return "script file"
	case ScriptFromStdin:
		return "standard input"
	case ScriptFromHereDoc:
		return "here-document"
	}
	return fmt.Sprintf("ScriptSource(%d)", int(s))
}

// Dynamic reports whether the command name holds an expansion, so that
// what runs is not known (guard-internal rule dynamic-command).
func (c Command) Dynamic() bool { return len(c.Words) > 0 && !c.Words[0].Literal }

// Name is the command name as a program name: its last path element,
// lower-cased, without a trailing ".exe". It is "" when there is no
// command or the name is dynamic. Lower-casing makes RM and rm one name,
// as they are on a case-insensitive file system.
func (c Command) Name() string {
	if len(c.Words) == 0 || c.Dynamic() {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(base(c.Words[0].Text)), ".exe")
}

// Cmdlet is the PowerShell command the name resolves to, lower-cased,
// with the aliases resolved (rm, ri, del, erase, rd and rmdir are
// remove-item; iex is invoke-expression). It is "" for a command that is
// not in PowerShell, and for a name with a program extension (rm.exe),
// which runs a program, not a cmdlet. On PowerShell for Linux and macOS,
// rm is the program, not the alias, so a rule may need to read a
// PowerShell rm both ways.
func (c Command) Cmdlet() string {
	if c.Shell != ShellPowerShell || c.Name() == "" {
		return ""
	}
	b := strings.ToLower(base(c.Words[0].Text))
	for _, ext := range []string{".exe", ".com", ".bat", ".cmd", ".ps1"} {
		if strings.HasSuffix(b, ext) {
			return ""
		}
	}
	if full, ok := psAliases[b]; ok {
		return full
	}
	return b
}

var psAliases = map[string]string{
	"rm": "remove-item", "ri": "remove-item", "del": "remove-item", "erase": "remove-item",
	"rd": "remove-item", "rmdir": "remove-item", "iex": "invoke-expression",
}

// String writes the words separated by spaces, each expansion as $….
func (c Command) String() string {
	parts := make([]string, len(c.Words))
	for i, w := range c.Words {
		parts[i] = ShowExpansions(w.Text)
	}
	return strings.Join(parts, " ")
}

func base(s string) string {
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		return s[i+1:]
	}
	return s
}

// ParseError is a command that cannot be decomposed. A guard refuses it
// under parse-error.
type ParseError struct {
	Shell Shell
	Msg   string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("does not parse as %s: %s", e.Shell, e.Msg)
}

// Commands decomposes src, written in shell, into its simple commands, in
// source order, each nested command after the command that runs it.
func Commands(shell Shell, src string) ([]Command, error) {
	var out []Command
	if err := decompose(shell, src, 0, nil, false, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decompose parses src and expands its commands. With envArgs, src is
// env's split string: each command's words are read as env's arguments
// again, as GNU env reads them (options, assignments, then the command).
func decompose(shell Shell, src string, depth int, via []string, envArgs bool, out *[]Command) error {
	if depth > MaxDepth {
		return &ParseError{Shell: shell, Msg: fmt.Sprintf("commands are nested deeper than %d shells", MaxDepth)}
	}
	raw, err := parse(shell, src)
	if err != nil {
		return err
	}
	for _, c := range raw {
		c.Depth, c.Via = depth, via
		if envArgs && len(c.Words) > 0 {
			c.Words = append([]Word{literalWord("env")}, c.Words...)
			c.envArgs = true
		}
		if err := expand(c, out); err != nil {
			return err
		}
	}
	return nil
}

func parse(shell Shell, src string) ([]Command, error) {
	switch shell {
	case ShellBash:
		return parseBash(src)
	case ShellPowerShell:
		return parsePowerShell(src)
	}
	return nil, &ParseError{Shell: shell, Msg: "unknown language"}
}

// expand adds c to out, or the commands it wraps, then the commands that
// c runs from a string. A wrapper whose options are read two ways adds
// the command of each reading; one whose reading runs a string (env -S)
// is added itself too.
func expand(c Command, out *[]Command) error {
	inners, label := unwrap(c)
	var scripts []nestedScript
	c.ScriptSource, scripts = scriptsOf(c)
	if len(inners) == 0 || len(scripts) > 0 {
		c.Writes = append(slices.Clone(c.Writes), writtenOperands(c)...)
		*out = append(*out, c)
	}
	for _, inner := range inners {
		inner.Shell = ShellBash
		inner.Depth = c.Depth
		inner.Via = c.Via
		if !c.envArgs {
			inner.Via = appendVia(c.Via, label)
		}
		inner.Assigns = append(slices.Clone(c.Assigns), inner.Assigns...)
		inner.Redirects = c.Redirects
		inner.Writes = c.Writes
		if err := expand(inner, out); err != nil {
			return err
		}
	}
	if c.Name() == "find" {
		for _, seg := range findExecs(c) {
			if err := expand(seg, out); err != nil {
				return err
			}
		}
	}
	// env's only scripts are its split strings, whose words env reads as
	// its arguments again.
	envArgs := c.Name() == "env"
	for _, n := range scripts {
		if err := decompose(n.shell, n.script, c.Depth+1, appendVia(c.Via, n.label), envArgs, out); err != nil {
			return err
		}
	}
	return nil
}

func appendVia(via []string, label string) []string {
	return append(slices.Clone(via), label)
}

// nestedScript is a string that a command runs as a script.
type nestedScript struct {
	shell  Shell
	script string
	label  string
}

const (
	bashUnknown = "${__vibe_unknown}"
	psUnknown   = "$__vibe_unknown"
)

// scriptOf joins words into the text a shell parses again. An expansion
// becomes an unquoted variable, so the nested word is Unknown: its value
// is spliced into the program text, where it can become anything.
func scriptOf(shell Shell, words []Word) string {
	mark := bashUnknown
	if shell == ShellPowerShell {
		mark = psUnknown
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = strings.ReplaceAll(w.Text, ExpansionMark, mark)
	}
	script := strings.Join(parts, " ")
	if shell == ShellPowerShell && strings.HasPrefix(script, psUnknown) {
		// A PowerShell statement that starts with a variable is an
		// expression, not a command. The unknown text can be any
		// command, so it is invoked: & $x, a command whose name is
		// dynamic.
		script = psUnknownCommand + script[len(psUnknown):]
	}
	return script
}

// psUnknownCommand is a PowerShell command whose name is not known.
const psUnknownCommand = "& " + psUnknown

var posixShells = []string{"bash", "sh", "zsh", "dash", "ksh", "ash", "mksh"}

// scriptsOf returns where c reads its script, when c is a POSIX shell,
// and every string c runs as a script.
func scriptsOf(c Command) (ScriptSource, []nestedScript) {
	switch name := c.Name(); {
	case name == "env":
		return ScriptNone, envSplitStrings(c.Words[1:])
	case slices.Contains(posixShells, name):
		return posixShellScript(name, c.Words[1:], c.stdin)
	}
	if n, ok := argumentScript(c); ok {
		return ScriptNone, []nestedScript{n}
	}
	return ScriptNone, nil
}

// argumentScript returns the one script that pwsh, powershell, eval or
// Invoke-Expression runs from its arguments.
func argumentScript(c Command) (nestedScript, bool) {
	name := c.Name()
	args := c.Words[min(1, len(c.Words)):]
	switch {
	case name == "pwsh" || name == "powershell":
		return powerShellScript(name, args)
	case name == "eval" && c.Shell == ShellBash && len(args) > 0:
		return nestedScript{ShellBash, scriptOf(ShellBash, args), "eval"}, true
	case c.Cmdlet() == "invoke-expression":
		var rest []Word
		for i := 0; i < len(args); i++ {
			if ok, _ := PSParam(args[i], "Command"); ok {
				continue
			}
			rest = append(rest, args[i])
		}
		if len(rest) > 0 {
			return nestedScript{ShellPowerShell, scriptOf(ShellPowerShell, rest), "iex"}, true
		}
	}
	return nestedScript{}, false
}

// posixShellOptions is how a POSIX shell reads its options (bash's
// documentation; sh, dash, zsh and ksh read -c, -s, -o and -O the same
// way). -o and -O, written with "-" or "+", take the next word. Long
// options are matched exactly, as bash matches them; one the table does
// not hold takes no argument. The first operand ends the options, and a
// lone "-" ends them as "--" does.
var posixShellOptions = optionTable{
	shortArg: "oO",
	long: map[string]argKind{
		"rcfile": argRequired, "init-file": argRequired,
		"version": argNone, "help": argNone, "login": argNone, "noprofile": argNone,
		"norc": argNone, "posix": argNone, "restricted": argNone, "verbose": argNone,
		"noediting": argNone, "debugger": argNone, "dump-strings": argNone,
		"dump-po-strings": argNone, "pretty-print": argNone,
	},
	unknownLong: argNone,
	plus:        true,
	nextWordArg: true,
}

// posixShellScript reads a POSIX shell's options and returns where it
// reads its script, with the script to parse again: the first operand
// with -c; else, with no -s, a script file when there is an operand; else
// standard input, parsed again when a here-document or here-string feeds
// it. With -s the operands are positional parameters.
func posixShellScript(name string, args []Word, stdin *Word) (ScriptSource, []nestedScript) {
	readings, ok := readOptions(args, posixShellOptions)
	if !ok || len(readings) != 1 {
		// The table has no option read two ways, so this does not happen;
		// if it did, the script would not be known.
		return ScriptFromStdin, nil
	}
	r := readings[0]
	operands := r.operands
	if len(operands) > 0 && operands[0].Literal && operands[0].Text == "-" {
		operands = operands[1:]
	}
	switch {
	case r.has("c"):
		if len(operands) == 0 {
			return ScriptFromString, nil
		}
		return ScriptFromString, []nestedScript{{ShellBash, scriptOf(ShellBash, operands[:1]), name + " -c"}}
	case r.has("--version", "--help"):
		return ScriptNone, nil
	case len(operands) > 0 && !r.has("s"):
		return ScriptFromFile, nil
	case stdin != nil:
		return ScriptFromHereDoc, []nestedScript{{ShellBash, scriptOf(ShellBash, []Word{*stdin}), name + " <<"}}
	}
	return ScriptFromStdin, nil
}

// powerShellScript finds the string pwsh -Command or powershell -Command
// runs: every word after -Command, joined. powershell also runs its
// first operand as a command; pwsh runs it as a file. An encoded command
// is unknown, so its nested command is dynamic.
func powerShellScript(name string, args []Word) (nestedScript, bool) { //nolint:gocyclo // baseline: pre-existing complexity
	label := name + " -c"
	for i := 0; i < len(args); i++ {
		w := args[i]
		if !w.Literal || w.Quoted || !strings.HasPrefix(w.Text, "-") || len(w.Text) == 1 {
			if name == "powershell" || !w.Literal {
				return commandArgs(args[i:], label)
			}
			return nestedScript{}, false // pwsh runs its operand as a file
		}
		p := strings.ToLower(strings.TrimLeft(w.Text, "-"))
		p, _, _ = strings.Cut(p, ":")
		switch {
		case p == "c" || (len(p) >= 3 && abbreviates(p, "command")):
			if i+1 < len(args) && args[i+1].Literal && args[i+1].Text == "-" {
				return nestedScript{}, false // reads stdin
			}
			return commandArgs(args[i+1:], label)
		case p == "e" || p == "ec" || (len(p) >= 2 && abbreviates(p, "encodedcommand")):
			return nestedScript{ShellPowerShell, psUnknownCommand, label}, true
		case p == "f" || (len(p) >= 2 && abbreviates(p, "file")):
			return nestedScript{}, false
		case psTakesArg(p):
			i++
		}
	}
	return nestedScript{}, false
}

func commandArgs(args []Word, label string) (nestedScript, bool) {
	var rest []Word
	for _, w := range args {
		if !w.block {
			rest = append(rest, w)
		}
	}
	if len(rest) == 0 {
		return nestedScript{}, false
	}
	return nestedScript{ShellPowerShell, scriptOf(ShellPowerShell, rest), label}, true
}

var psArgParams = []string{
	"executionpolicy", "workingdirectory", "windowstyle", "configurationname", "custompipename",
	"outputformat", "inputformat", "settingsfile", "psconsolefile", "version", "configurationfile",
}

func psTakesArg(p string) bool {
	switch p {
	case "ex", "ep", "wd", "w", "o", "of", "if", "v":
		return true
	}
	if len(p) < 3 {
		return false
	}
	for _, full := range psArgParams {
		if abbreviates(p, full) {
			return true
		}
	}
	return false
}

// abbreviates reports whether short is a prefix of the parameter name
// full.
func abbreviates(short, full string) bool { return strings.HasPrefix(full, short) }

// PSParam reports whether w is PowerShell parameter param (given without
// its dash, as "Recurse") or an abbreviation of it, compared without
// case. A quoted word is never a parameter. value is the part after a
// colon, as in -Recurse:$false. Whether an abbreviation is ambiguous for
// a given cmdlet is the caller's to decide.
func PSParam(w Word, param string) (ok bool, value string) {
	if !w.Literal || w.Quoted || len(w.Text) < 2 || w.Text[0] != '-' {
		return false, ""
	}
	name, value, _ := strings.Cut(w.Text[1:], ":")
	if name == "" || len(name) > len(param) {
		return false, ""
	}
	if !strings.EqualFold(name, param[:len(name)]) {
		return false, ""
	}
	return true, value
}
