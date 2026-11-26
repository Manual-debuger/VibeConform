package guardrule

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// RuleKind is what a repository rule reads: a command or a path.
type RuleKind int

const (
	// CommandRule reads the simple commands of a command tool's call. Its
	// pattern is a word prefix, matched as exceptions match it.
	CommandRule RuleKind = iota + 1
	// PathRule reads the file of a file-edit tool's call. Its pattern is
	// a path glob (*, ?, **), matched against the repository relative
	// path.
	PathRule
)

// String is the policy.json key that gives a rule of this kind its
// pattern: "command" or "path".
func (k RuleKind) String() string {
	switch k {
	case CommandRule:
		return "command"
	case PathRule:
		return "path"
	}
	return ""
}

// RuleSpec is what NewRule builds a rule from: the values of a rule in
// policy.json.
type RuleSpec struct {
	ID     string
	Level  Level
	Reason string
	// Tools is the list of tools the rule reads, or nil for every tool of
	// its kind.
	Tools []string
	Kind  RuleKind
	// Pattern is a command rule's word prefix or a path rule's glob.
	Pattern string
	// Args is a regular expression that some argument after a command
	// rule's prefix must match, or empty for none.
	Args string
}

// Rule is one of the repository's own rules (spec 0053, "policy.json
// version 2"). NewRule builds it, so a Rule is valid: its command is one
// simple command with no expansion, its args compiles, and its glob is
// compiled. The zero Rule matches nothing.
type Rule struct {
	id      string
	level   Level
	reason  string
	tools   []string
	kind    RuleKind
	pattern string
	args    string

	command commandMatch
	argsRe  *regexp.Regexp
	glob    *regexp.Regexp
}

// Exception lifts one contract rule where its match matches (spec 0051,
// "Guard policy and exceptions"; spec 0053 decides what match means).
// NewException builds it, so its rule is a contract rule and a command
// match is parsed. The zero Exception matches nothing.
type Exception struct {
	rule   string
	match  string
	reason string

	command commandMatch
	glob    *regexp.Regexp
}

// FieldError is a value that NewRule or NewException refuses. Field
// names the value as policy.json names it ("level", "command", "args",
// "match"), or is empty for the rule as a whole; Msg is the message
// ParsePolicy gives after the key.
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string {
	if e.Field == "" {
		return e.Msg
	}
	return e.Field + " " + e.Msg
}

func fieldErr(field, format string, args ...any) *FieldError {
	return &FieldError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

const argsOnPathRule = "is only allowed with command"

// NewRule builds a repository rule from s. It refuses a rule that is not
// valid with a *FieldError, in the order ParsePolicy reads the values:
// an empty or built-in ID, a level other than deny and ask, an empty
// reason, no kind, an empty pattern, a command that is not one simple
// command without expansion, args on a path rule, an args that does not
// compile, and a tool that is not of the rule's kind. A command is
// parsed once in Bash and once in PowerShell; a glob is compiled once.
func NewRule(s RuleSpec) (Rule, error) {
	r := Rule{id: s.ID, level: s.Level, reason: s.Reason, tools: slices.Clone(s.Tools),
		kind: s.Kind, pattern: s.Pattern, args: s.Args}
	if err := r.checkHead(); err != nil {
		return Rule{}, err
	}
	switch r.kind {
	case CommandRule:
		m, err := parseMatch(r.pattern)
		if err != nil {
			return Rule{}, fieldErr("command", "does not parse as a command: %v", err)
		}
		r.command = m
	case PathRule:
		if r.args != "" {
			return Rule{}, fieldErr("args", argsOnPathRule)
		}
		r.glob = compileGlob(r.pattern)
	}
	if r.args != "" {
		re, err := regexp.Compile(r.args)
		if err != nil {
			return Rule{}, fieldErr("args", "is not a regular expression: %v", err)
		}
		r.argsRe = re
	}
	if err := checkTools(r.kind, r.tools); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// checkHead checks the values NewRule reads before the pattern.
func (r Rule) checkHead() error {
	switch {
	case isEmpty(r.id):
		return fieldErr("id", "is empty")
	case IsContractID(r.id) || IsInternalID(r.id):
		return fieldErr("id", "%q is a built-in guard rule ID; a repository rule needs its own", r.id)
	case r.level != Deny && r.level != Ask:
		return fieldErr("level", "is %q; it must be \"deny\" or \"ask\"", r.level)
	case isEmpty(r.reason):
		return fieldErr("reason", "is empty")
	case r.kind != CommandRule && r.kind != PathRule:
		return fieldErr("", "must have exactly one of command and path")
	case isEmpty(r.pattern):
		return fieldErr(r.kind.String(), "is empty")
	}
	return nil
}

func checkTools(kind RuleKind, tools []string) error {
	for _, tool := range tools {
		switch {
		case !IsCommandTool(tool) && !IsEditTool(tool):
			return fieldErr("tools", "names %q, which is not one of %s",
				tool, strings.Join(append(slices.Clone(CommandTools), EditTools...), ", "))
		case kind == CommandRule && !IsCommandTool(tool):
			return fieldErr("tools", "names %q, which runs no command, on a command rule", tool)
		case kind == PathRule && !IsEditTool(tool):
			return fieldErr("tools", "names %q, which edits no file, on a path rule", tool)
		}
	}
	return nil
}

// NewException builds an exception that lifts rule where match matches.
// It refuses, with a *FieldError, a rule that is not a contract rule, an
// empty match, and, for a command rule, a match that is not one simple
// command without expansion. A command match is parsed once in Bash and
// once in PowerShell; a path rule's glob is compiled once.
func NewException(rule, match, reason string) (Exception, error) {
	if !IsContractID(rule) {
		return Exception{}, fieldErr("rule", "%q is not a guard rule ID; it must be one of %s",
			rule, strings.Join(IDs, ", "))
	}
	if isEmpty(match) {
		return Exception{}, fieldErr("match", "is empty")
	}
	e := Exception{rule: rule, match: match, reason: reason}
	if IsPathRule(rule) {
		e.glob = compileGlob(match)
		return e, nil
	}
	m, err := parseMatch(match)
	if err != nil {
		return Exception{}, fieldErr("match", "does not parse as a command: %v", err)
	}
	e.command = m
	return e, nil
}

func isEmpty(s string) bool { return strings.TrimSpace(s) == "" }

// ID is the rule's ID.
func (r Rule) ID() string { return r.id }

// Level is what the rule does when it matches.
func (r Rule) Level() Level { return r.level }

// Reason is the rule's reason.
func (r Rule) Reason() string { return r.reason }

// Tools is the list of tools the rule reads, or nil for every tool of
// its kind.
func (r Rule) Tools() []string { return slices.Clone(r.tools) }

// Kind is whether the rule reads commands or paths.
func (r Rule) Kind() RuleKind { return r.kind }

// Pattern is the rule's word prefix (a command rule) or glob (a path
// rule).
func (r Rule) Pattern() string { return r.pattern }

// Args is the regular expression some argument after a command rule's
// prefix must match, or empty.
func (r Rule) Args() string { return r.args }

// AppliesTo reports whether the rule reads calls of tool.
func (r Rule) AppliesTo(tool string) bool {
	switch {
	case r.tools != nil:
		return slices.Contains(r.tools, tool)
	case r.kind == CommandRule:
		return IsCommandTool(tool)
	case r.kind == PathRule:
		return IsEditTool(tool)
	}
	return false
}

// Rule is the ID of the contract rule the exception lifts.
func (e Exception) Rule() string { return e.rule }

// Match is the exception's match: a word prefix or a glob.
func (e Exception) Match() string { return e.match }

// Reason is the exception's reason.
func (e Exception) Reason() string { return e.reason }

// commandMatch is a command match parsed in each shell it parses in.
type commandMatch struct {
	bash, powerShell *Command
}

// in returns the match as shell reads it, if it parses in shell.
func (m commandMatch) in(shell Shell) (Command, bool) {
	var c *Command
	switch shell {
	case ShellBash:
		c = m.bash
	case ShellPowerShell:
		c = m.powerShell
	}
	if c == nil {
		return Command{}, false
	}
	return *c, true
}

// parseMatch parses s once in Bash and once in PowerShell, and keeps
// each form that is one simple command. s must be one simple command in
// at least one of them, with no expansion in the form that is checked:
// Bash's, else PowerShell's. The error is Bash's when neither parses.
func parseMatch(s string) (commandMatch, error) {
	var m commandMatch
	bash, errBash := simpleCommand(ShellBash, s)
	if errBash == nil {
		m.bash = &bash
	}
	ps, errPS := simpleCommand(ShellPowerShell, s)
	if errPS == nil {
		m.powerShell = &ps
	}
	checked := m.bash
	if checked == nil {
		checked = m.powerShell
	}
	if checked == nil {
		return commandMatch{}, errBash
	}
	for _, w := range checked.Words {
		if !w.Literal {
			return commandMatch{}, errors.New("holds an expansion; a match is compared word for word")
		}
	}
	return m, nil
}
