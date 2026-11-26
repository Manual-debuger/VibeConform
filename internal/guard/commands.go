package guard

import (
	"fmt"
	"slices"
	"strings"
)

// Command is one vibe-guard command (spec 0073): what Main accepts and
// the usage text shows.
type Command struct {
	// Name is the command's name, the first argument.
	Name string
	// Aliases are other first arguments that run the command.
	Aliases []string
	// Flags are the command's flags.
	Flags []Flag
	// Operands is the usage text of the command's operands, if any.
	Operands string
	// Summary says what the command does, in the usage text, or is
	// empty.
	Summary string
	// Hidden keeps the command out of the usage text.
	Hidden bool
}

// Flag is a flag of a command.
type Flag struct {
	// Name is the flag as written, with its dashes: --tool.
	Name string
	// Value is the usage text of the flag's value, such as <tool>, or ""
	// for a flag that takes none.
	Value string
}

// TakesValue reports whether the flag takes a value.
func (f Flag) TakesValue() bool { return f.Value != "" }

// The command names Main dispatches on.
const (
	cmdHook    = "hook"
	cmdExplain = "explain"
	cmdRelock  = "relock"
	cmdVersion = "version"
	cmdHelp    = "help"
)

// Commands returns vibe-guard's command table, in usage order. Each call
// returns a new table.
func Commands() []Command {
	return []Command{
		{Name: cmdHook, Summary: "decide the PreToolUse payload on stdin"},
		{Name: cmdExplain, Flags: []Flag{{Name: "--tool", Value: "<tool>"}}, Operands: "'<command or path>'"},
		{Name: cmdRelock, Summary: "delete the unlock marker"},
		{Name: cmdVersion, Aliases: []string{"--version"}},
		{Name: cmdHelp, Aliases: []string{"-h", "--help"}, Hidden: true},
	}
}

// lookupCommand returns the name of the command that arg names, by its
// name or an alias, or "".
func lookupCommand(arg string) string {
	for _, c := range Commands() {
		if c.Name == arg || slices.Contains(c.Aliases, arg) {
			return c.Name
		}
	}
	return ""
}

// summaryColumn is where a command's summary starts in the usage text,
// after the two-space indent.
const summaryColumn = 49

// usage is the usage text, one line for each command that is not
// hidden.
var usage = func() string {
	lines := []string{"usage:"}
	for _, c := range Commands() {
		if c.Hidden {
			continue
		}
		words := []string{"vibe-guard", c.Name}
		for _, f := range c.Flags {
			fw := f.Name
			if f.TakesValue() {
				fw += " " + f.Value
			}
			words = append(words, "["+fw+"]")
		}
		if c.Operands != "" {
			words = append(words, c.Operands)
		}
		line := strings.Join(words, " ")
		if c.Summary != "" {
			line = fmt.Sprintf("%-*s%s", summaryColumn, line, c.Summary)
		}
		lines = append(lines, "  "+line)
	}
	return strings.Join(lines, "\n")
}()
