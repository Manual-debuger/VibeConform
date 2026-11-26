package guardrule

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// TargetKind is what a tool call acts on: nothing, a command or a path.
type TargetKind int

const (
	// NoTarget is the kind of a call of a tool that is neither a command
	// tool nor a file-edit tool, and of the zero Target.
	NoTarget TargetKind = iota
	// CommandTarget is the kind of a command tool's call: its text is the
	// command.
	CommandTarget
	// PathTarget is the kind of a file-edit tool's call: its text is the
	// file's path.
	PathTarget
)

// String is "none", "command" or "path".
func (k TargetKind) String() string {
	switch k {
	case CommandTarget:
		return "command"
	case PathTarget:
		return "path"
	}
	return "none"
}

// Target is what a tool call acts on: the command of a command tool, or
// the file of a file-edit tool. Its kind says which; ParseTarget,
// NewCommandTarget and NewPathTarget build it.
type Target struct {
	tool string
	kind TargetKind
	text string
}

// NewCommandTarget is the target of a call of the command tool tool that
// runs command.
func NewCommandTarget(tool, command string) Target {
	return Target{tool: tool, kind: CommandTarget, text: command}
}

// NewPathTarget is the target of a call of the file-edit tool tool that
// edits the file at path.
func NewPathTarget(tool, path string) Target {
	return Target{tool: tool, kind: PathTarget, text: path}
}

// Tool is the name of the tool the call is of.
func (t Target) Tool() string { return t.tool }

// Kind says whether the target is a command, a path or neither.
func (t Target) Kind() TargetKind { return t.kind }

// Text is the command of a command target, the path of a path target,
// and empty for no target.
func (t Target) Text() string { return t.text }

// PayloadError is a tool call whose input lacks what its tool needs. A
// guard refuses it under payload-error.
type PayloadError struct{ Msg string }

func (e *PayloadError) Error() string { return e.Msg }

// ParseTarget reads a Claude Code tool call's target from its tool_name
// and tool_input: tool_input.command for Bash and PowerShell,
// tool_input.file_path for Write, Edit and MultiEdit, and
// tool_input.notebook_path for NotebookEdit. Any other tool has no
// target and no error. The error is a *PayloadError.
func ParseTarget(tool string, input json.RawMessage) (Target, error) {
	key := ""
	switch {
	case IsCommandTool(tool):
		key = "command"
	case tool == NotebookEdit:
		key = "notebook_path"
	case IsEditTool(tool):
		key = "file_path"
	default:
		return Target{tool: tool}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil || m == nil {
		return Target{}, &PayloadError{fmt.Sprintf("%s tool_input is not a JSON object", tool)}
	}
	var v string
	if raw, ok := m[key]; !ok || json.Unmarshal(raw, &v) != nil {
		return Target{}, &PayloadError{fmt.Sprintf("%s tool_input has no string %s", tool, key)}
	}
	if IsCommandTool(tool) {
		return NewCommandTarget(tool, v), nil
	}
	if v == "" {
		return Target{}, &PayloadError{fmt.Sprintf("%s tool_input.%s is empty", tool, key)}
	}
	return NewPathTarget(tool, v), nil
}

// Matches reports whether the exception matches the target: for a path
// rule, its glob matches the path made relative to root (RelPath); for a
// command rule, its words are a prefix of the words of some simple
// command of the target's command (MatchesCommand). A command that does
// not parse matches no exception.
//
// This is the reading the audit's A-02 needs under contract 1.x. A
// guard, which knows the simple command a rule refused, calls
// MatchesCommand on that command.
func (e Exception) Matches(t Target, root string) bool {
	return e.matches(t, root, func(Command) bool { return true })
}

// MatchesRefused is Matches with only the simple commands a rule refused
// counted, each given as its words (Command.String): how a guard reads an
// exception, and the audit's reading under contract 2.0.0 (spec 0072,
// "The audit's reading under 2.0.0"). With no refused, every simple
// command counts, as for a command that is one simple command. refused
// does not change a path rule's match.
func (e Exception) MatchesRefused(t Target, root string, refused []string) bool {
	return e.matches(t, root, func(c Command) bool { return len(refused) == 0 || slices.Contains(refused, c.String()) })
}

// matches is Matches with only the simple commands that counted accepts.
func (e Exception) matches(t Target, root string, counted func(Command) bool) bool {
	if IsPathRule(e.rule) {
		return t.kind == PathTarget && t.text != "" && e.MatchesPath(t.text, root)
	}
	shell := ShellFor(t.tool)
	if t.kind != CommandTarget || shell == 0 {
		return false
	}
	cmds, err := Commands(shell, t.text)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(cmds, func(c Command) bool { return counted(c) && e.MatchesCommand(c) })
}

// MatchesPath reports whether the exception's glob matches p, made
// relative to root. An exception for a command rule matches no path.
func (e Exception) MatchesPath(p, root string) bool {
	return e.glob != nil && e.glob.MatchString(RelPath(root, p))
}

// MatchesCommand reports whether the exception's words are a prefix of
// c's words. The match is compared in c's language, and matches nothing
// in a language it does not parse in. Words compare after quote removal;
// the first by Name (and, in PowerShell, by Cmdlet, so "Remove-Item"
// matches "ri"); in PowerShell without case. Every compared word of c
// must be literal: an exception never matches an expansion.
func (e Exception) MatchesCommand(c Command) bool {
	m, ok := e.command.in(c.Shell)
	return ok && prefixMatch(m, c, false)
}

// prefixMatch reports whether m's words are a prefix of c's. With
// unknown, an Unknown word of c matches the rest of the prefix, and an
// operand matches one word: the reading a deny rule uses (spec 0053,
// "Unknown words"). Without it, every compared word must be literal.
func prefixMatch(m, c Command, unknown bool) bool {
	if len(m.Words) == 0 || len(c.Words) == 0 {
		return false
	}
	for i, mw := range m.Words {
		if i >= len(c.Words) || !mw.Literal {
			return false
		}
		if cw := c.Words[i]; !cw.Literal {
			if !unknown {
				return false
			}
			if cw.Unknown {
				return true
			}
			continue
		}
		if !sameWord(m, c, i) {
			return false
		}
	}
	return true
}

// sameWord reports whether the literal word i of m and of c are the
// same: the first by name, any other by text, in PowerShell without
// case.
func sameWord(m, c Command, i int) bool {
	switch {
	case i == 0:
		return sameName(m, c)
	case c.Shell == ShellPowerShell:
		return strings.EqualFold(m.Words[i].Text, c.Words[i].Text)
	}
	return m.Words[i].Text == c.Words[i].Text
}

func sameName(m, c Command) bool {
	if c.Shell == ShellPowerShell {
		m.Shell = ShellPowerShell
		if mc, cc := m.Cmdlet(), c.Cmdlet(); mc != "" && mc == cc {
			return true
		}
	}
	return m.Name() == c.Name()
}

// matchParses counts the parses of a match, for the test that a match
// is parsed once per shell, when it is built.
var matchParses atomic.Int64

// simpleCommand parses s, in shell, as one simple command.
func simpleCommand(shell Shell, s string) (Command, error) {
	matchParses.Add(1)
	cmds, err := parse(shell, s)
	if err != nil {
		return Command{}, err
	}
	if len(cmds) != 1 || len(cmds[0].Words) == 0 || len(cmds[0].Redirects) > 0 {
		return Command{}, errors.New("is not one simple command")
	}
	c := cmds[0]
	c.Shell = shell
	return c, nil
}

// MatchesCommand reports whether a repository command rule matches c:
// its words are a prefix of c's, and, with args, some later word
// matches the regular expression. A word of c that holds an expansion
// counts as matching, unless a known word already rules the match out
// (spec 0053, "Unknown words"). A rule's args always compiles: NewRule
// refuses one that does not (spec 0069, as amended by spec 0073).
func (r Rule) MatchesCommand(c Command) bool {
	m, ok := r.command.in(c.Shell)
	if !ok || !prefixMatch(m, c, true) {
		return false
	}
	if r.argsRe == nil {
		return true
	}
	for i, w := range c.Words {
		switch {
		case w.Unknown:
			return true // it can become the prefix's last words and any argument
		case i < len(m.Words):
		case !w.Literal || r.argsRe.MatchString(w.Text):
			return true
		}
	}
	return false
}

// MatchesPath reports whether a repository path rule matches p, made
// relative to root.
func (r Rule) MatchesPath(p, root string) bool {
	return r.glob != nil && r.glob.MatchString(RelPath(root, p))
}

// RelPath is the path a glob is matched against (spec 0053): p made
// relative to root and slash separated when it is inside root, and p as
// given otherwise. Backslashes count as separators; a root with a drive
// letter is compared without case. A relative p is taken as relative to
// root. The path is cleaned before it is compared (spec 0069), so a ".."
// that leaves root and comes back is inside root. The result is cleaned
// (no "." or ".." elements where they can be removed).
func RelPath(root, p string) string {
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	root = strings.ReplaceAll(root, `\`, "/")
	if strings.TrimRight(root, "/") == "" {
		return p
	}
	// path.Clean leaves a trailing slash only on "/", which becomes "" so
	// that root+"/" is the prefix of every absolute path.
	root = strings.TrimSuffix(path.Clean(root), "/")
	fold := hasDrive(root)
	full := p
	if !IsAbs(p) {
		full = path.Clean(root + "/" + p)
	}
	if rel, ok := cutPrefixFold(full, root+"/", fold); ok {
		return rel
	}
	if !IsAbs(p) && (full == root || fold && strings.EqualFold(full, root)) {
		return "." // a relative p that names root itself, as path.Clean gives it
	}
	return p
}

// IsAbs reports whether p is absolute, with backslashes counted as
// separators: it starts with a slash or a backslash, or with a drive
// letter and a colon. This is how RelPath reads p on every platform.
func IsAbs(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || hasDrive(p)
}

func hasDrive(p string) bool {
	return len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

func cutPrefixFold(s, prefix string, fold bool) (string, bool) {
	if len(s) < len(prefix) {
		return "", false
	}
	if fold && strings.EqualFold(s[:len(prefix)], prefix) || s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return "", false
}

// MatchGlob reports whether the slash-separated path p matches pattern:
// * matches within one path element, ? one character other than /, and
// ** any number of whole elements, none included ("a/**/b" matches
// "a/b"; a trailing "dir/**" matches everything below dir, not dir
// itself). Matching is case sensitive.
//
// A caller with a Rule or an Exception uses its compiled glob. MatchGlob
// is for callers without one: it keeps the globs it compiles, up to
// globCacheSize of them, because the guard calls it with the same few
// patterns.
func MatchGlob(pattern, p string) bool {
	if re, ok := globCache.Load(pattern); ok {
		return re.(*regexp.Regexp).MatchString(p)
	}
	re := compileGlob(pattern)
	if globCacheLen.Load() < globCacheSize {
		if _, loaded := globCache.LoadOrStore(pattern, re); !loaded {
			globCacheLen.Add(1)
		}
	}
	return re.MatchString(p)
}

const globCacheSize = 256

var (
	globCache    sync.Map // pattern → *regexp.Regexp
	globCacheLen atomic.Int64
	// globCompiles counts the globs compiled, for the test that a glob is
	// compiled once per rule or exception.
	globCompiles atomic.Int64
)

// compileGlob compiles pattern as MatchGlob reads it.
func compileGlob(pattern string) *regexp.Regexp {
	globCompiles.Add(1)
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case strings.HasPrefix(pattern[i:], "**/") && (i == 0 || pattern[i-1] == '/'):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**") && (i == 0 || pattern[i-1] == '/') && i+2 == len(pattern):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			j := i
			for j < len(pattern) && pattern[j] != '*' && pattern[j] != '?' {
				j++
			}
			b.WriteString(regexp.QuoteMeta(pattern[i:j]))
			i = j - 1
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
