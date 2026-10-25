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
	// Redirects are the targets of the command's file redirections
	// (">f", ">>f", "<f", "2>f"). Here-documents and descriptor
	// duplications ("2>&1") are not targets.
	Redirects []Word
	// Writes are the paths the command writes, deletes, moves or renames
	// (spec 0058, "Write targets come from the parser"): its output
	// redirections, then the operands that the writers in wrappers.go
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

	// stdin is a here-document or here-string fed to the command.
	stdin *Word
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
	if err := decompose(shell, src, 0, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func decompose(shell Shell, src string, depth int, via []string, out *[]Command) error {
	if depth > MaxDepth {
		return &ParseError{Shell: shell, Msg: fmt.Sprintf("commands are nested deeper than %d shells", MaxDepth)}
	}
	raw, err := parse(shell, src)
	if err != nil {
		return err
	}
	for _, c := range raw {
		c.Depth, c.Via = depth, via
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

// expand adds c to out, or the command it wraps, then the commands that
// c runs from a string.
func expand(c Command, out *[]Command) error {
	if inner, label, ok := unwrap(c); ok {
		inner.Shell = ShellBash
		inner.Depth = c.Depth
		inner.Via = appendVia(c.Via, label)
		inner.Redirects = c.Redirects
		inner.Writes = c.Writes
		return expand(inner, out)
	}
	c.Writes = append(slices.Clone(c.Writes), writtenOperands(c)...)
	*out = append(*out, c)
	if c.Name() == "find" {
		for _, seg := range findExecs(c) {
			if err := expand(seg, out); err != nil {
				return err
			}
		}
	}
	if n, ok := nested(c); ok {
		return decompose(n.shell, n.script, c.Depth+1, appendVia(c.Via, n.label), out)
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

func nested(c Command) (nestedScript, bool) {
	name := c.Name()
	args := c.Words[min(1, len(c.Words)):]
	switch {
	case slices.Contains(posixShells, name):
		return posixShellScript(name, args, c.stdin)
	case name == "pwsh" || name == "powershell":
		return powerShellScript(name, args)
	case name == "eval" && c.Shell == ShellBash && len(args) > 0:
		return nestedScript{ShellBash, scriptOf(ShellBash, args), "eval"}, true
	case name == "env":
		return envSplitString(args)
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

// posixShellScript finds the string "sh -c" runs: the first operand
// after the options, when some option cluster holds c. With no -c and no
// operand, the shell reads its script from a here-document or
// here-string, if one is given.
func posixShellScript(name string, args []Word, stdin *Word) (nestedScript, bool) {
	sawC := false
	for i := 0; i < len(args); i++ {
		w := args[i]
		if !w.Literal {
			if sawC {
				return nestedScript{ShellBash, scriptOf(ShellBash, args[i:i+1]), name + " -c"}, true
			}
			return nestedScript{}, false
		}
		t := w.Text
		switch {
		case t == "--" || t == "-":
			continue
		case strings.HasPrefix(t, "--"):
			if t == "--rcfile" || t == "--init-file" {
				i++
			}
			continue
		case len(t) > 1 && (t[0] == '-' || t[0] == '+'):
			for _, r := range t[1:] {
				switch r {
				case 'c':
					sawC = true
				case 'o', 'O':
					i++
				}
			}
			continue
		}
		if sawC {
			return nestedScript{ShellBash, scriptOf(ShellBash, args[i:i+1]), name + " -c"}, true
		}
		return nestedScript{}, false // runs a script file
	}
	if !sawC && stdin != nil {
		return nestedScript{ShellBash, scriptOf(ShellBash, []Word{*stdin}), name + " <<"}, true
	}
	return nestedScript{}, false
}

// powerShellScript finds the string pwsh -Command or powershell -Command
// runs: every word after -Command, joined. powershell also runs its
// first operand as a command; pwsh runs it as a file. An encoded command
// is unknown, so its nested command is dynamic.
func powerShellScript(name string, args []Word) (nestedScript, bool) {
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
