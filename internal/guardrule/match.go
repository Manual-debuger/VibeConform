package guardrule

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Target is what a tool call acts on: the command of a command tool, or
// the file of a file-edit tool. Both are empty for any other tool.
type Target struct {
	Tool    string
	Command string
	Path    string
}

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
		return Target{Tool: tool}, nil
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
		return Target{Tool: tool, Command: v}, nil
	}
	if v == "" {
		return Target{}, &PayloadError{fmt.Sprintf("%s tool_input.%s is empty", tool, key)}
	}
	return Target{Tool: tool, Path: v}, nil
}

// Matches reports whether the exception matches the target: for a path
// rule, its glob matches the path made relative to root (RelPath); for a
// command rule, its words are a prefix of the words of some simple
// command of the target's command (MatchesCommand). A command that does
// not parse matches no exception.
//
// This is the reading the audit's A-02 needs. A guard, which knows the
// simple command a rule refused, calls MatchesCommand on that command.
func (e Exception) Matches(t Target, root string) bool {
	if IsPathRule(e.Rule) {
		return t.Path != "" && IsEditTool(t.Tool) && e.MatchesPath(t.Path, root)
	}
	shell := ShellFor(t.Tool)
	if shell == 0 {
		return false
	}
	cmds, err := Commands(shell, t.Command)
	if err != nil {
		return false
	}
	for _, c := range cmds {
		if e.MatchesCommand(c) {
			return true
		}
	}
	return false
}

// MatchesPath reports whether the exception's glob matches p, made
// relative to root.
func (e Exception) MatchesPath(p, root string) bool {
	return MatchGlob(e.Match, RelPath(root, p))
}

// MatchesCommand reports whether the exception's words are a prefix of
// c's words. The match is parsed in c's language. Words compare after
// quote removal; the first by Name (and, in PowerShell, by Cmdlet, so
// "Remove-Item" matches "ri"); in PowerShell without case. Every
// compared word of c must be literal: an exception never matches an
// expansion.
func (e Exception) MatchesCommand(c Command) bool {
	m, err := simpleCommand(c.Shell, e.Match)
	if err != nil {
		return false
	}
	return prefixMatch(m, c, false)
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
		if i >= len(c.Words) {
			return false
		}
		cw := c.Words[i]
		if !mw.Literal {
			return false
		}
		if !cw.Literal {
			if !unknown {
				return false
			}
			if cw.Unknown {
				return true
			}
			continue
		}
		if i == 0 {
			if !sameName(m, c) {
				return false
			}
			continue
		}
		if c.Shell == ShellPowerShell {
			if !strings.EqualFold(mw.Text, cw.Text) {
				return false
			}
		} else if mw.Text != cw.Text {
			return false
		}
	}
	return true
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

// simpleCommand parses s, in shell, as one simple command.
func simpleCommand(shell Shell, s string) (Command, error) {
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

// checkMatch checks that s is one simple command, as Bash or as
// PowerShell, with no expansion: the shape of a command match.
func checkMatch(s string) error {
	c, err := simpleCommand(ShellBash, s)
	if err != nil {
		var err2 error
		if c, err2 = simpleCommand(ShellPowerShell, s); err2 != nil {
			return err
		}
	}
	for _, w := range c.Words {
		if !w.Literal {
			return errors.New("holds an expansion; a match is compared word for word")
		}
	}
	return nil
}

// MatchesCommand reports whether a repository command rule matches c:
// its words are a prefix of c's, and, with args, some later word
// matches the regular expression. A word of c that holds an expansion
// counts as matching, unless a known word already rules the match out
// (spec 0053, "Unknown words").
func (r Rule) MatchesCommand(c Command) bool {
	if r.Command == "" {
		return false
	}
	m, err := simpleCommand(c.Shell, r.Command)
	if err != nil || !prefixMatch(m, c, true) {
		return false
	}
	if r.Args == "" {
		return true
	}
	re := r.args
	if re == nil {
		if re, err = regexp.Compile(r.Args); err != nil {
			return false
		}
	}
	for i, w := range c.Words {
		switch {
		case w.Unknown:
			return true // it can become the prefix's last words and any argument
		case i < len(m.Words):
		case !w.Literal || re.MatchString(w.Text):
			return true
		}
	}
	return false
}

// MatchesPath reports whether a repository path rule matches p, made
// relative to root.
func (r Rule) MatchesPath(p, root string) bool {
	return r.Path != "" && MatchGlob(r.Path, RelPath(root, p))
}

// RelPath is the path a glob is matched against (spec 0053): p made
// relative to root and slash separated when it is inside root, and p as
// given otherwise. Backslashes count as separators; a root with a drive
// letter is compared without case. A relative p is taken as relative to
// root. The result is cleaned (no "." or ".." elements where they can
// be removed).
func RelPath(root, p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	root = strings.TrimRight(strings.ReplaceAll(root, `\`, "/"), "/")
	if root != "" && isAbs(p) {
		if rel, ok := cutPrefixFold(p, root+"/", hasDrive(root)); ok {
			return path.Clean(rel)
		}
	}
	return path.Clean(p)
}

func isAbs(p string) bool { return strings.HasPrefix(p, "/") || hasDrive(p) }

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
func MatchGlob(pattern, p string) bool {
	return globRegexp(pattern).MatchString(p)
}

func globRegexp(pattern string) *regexp.Regexp {
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
