// Package guard is vibe-guard, the reference agent guard (spec 0053,
// "vibe-guard"). It decides Claude Code tool calls against the baseline
// rules of contract 1.0.0 and the repository's own policy.json version 2.
// It imports internal/guardrule, which it shares with the contract audit,
// and nothing from internal/contract or the old template system. It
// opens no network connection.
package guard

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// Outcome is what the guard does with a tool call.
type Outcome int

const (
	// Allow lets the call run.
	Allow Outcome = iota
	// Ask lets Claude Code ask the person (a repository rule of level
	// ask).
	Ask
	// Deny refuses the call.
	Deny
)

func (o Outcome) String() string {
	switch o {
	case Ask:
		return "ask"
	case Deny:
		return "deny"
	}
	return "allow"
}

// Finding is one rule that matched.
type Finding struct {
	// Rule is a guard rule ID, a guard-internal ID or a repository rule's
	// id.
	Rule string
	Why  string
	// Command is the simple command the rule matched, for a command rule.
	Command *gr.Command
	// Path is the file, for a path rule.
	Path string
	// Exception lifted the rule, when one did.
	Exception *gr.Exception
}

// Decision is the guard's answer to one tool call.
type Decision struct {
	Outcome Outcome
	// Finding is the rule that decided a Deny or an Ask.
	Finding *Finding
	// Lifted are baseline rules that matched and that an exception
	// lifted.
	Lifted []Finding
	// Observe is set when policy.json's mode is observe: a Deny is only
	// reported.
	Observe bool
}

// Refuses reports whether the call is refused: a Deny that observe mode
// does not soften. An Ask is not a refusal.
func (d Decision) Refuses() bool { return d.Outcome == Deny && !d.Observe }

// Guard decides tool calls in one repository. It is complete when New
// returns, and Decide is the only way to use it.
type Guard struct {
	repo Repo
	// policy is the repository's policy.json, or the default (enforce,
	// no rules, no exceptions) when it has none or cannot be used.
	policy *gr.Policy
	// policyErr is set when policy.json cannot be read or is invalid.
	// Every call is then refused under policy-error.
	policyErr error
	// now is the time an unlock marker is compared with.
	now time.Time
	// dir is the directory a relative path, edited or written by a
	// command, is resolved against. When it is "", the root is.
	dir string
}

// Options are what New needs besides the repository.
type Options struct {
	// Now is the time an unlock marker is compared with.
	Now time.Time
	// Dir is the directory a relative path is read against, such as the
	// payload's cwd. When it is "", the project root is.
	Dir string
}

// New loads the guard of the repository at repo. No policy.json is the
// default policy (enforce, no rules, no exceptions); one that cannot be
// read or parsed is a policy error, which refuses every call.
func New(repo Repo, opts Options) *Guard {
	g := &Guard{repo: repo, now: opts.Now, dir: opts.Dir, policy: &gr.Policy{Version: 2, Mode: gr.Enforce}}
	data, err := os.ReadFile(filepath.Join(repo.Root, filepath.FromSlash(gr.PolicyFile)))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		g.policyErr = err
	default:
		if p, err := gr.ParsePolicy(data); err != nil {
			g.policyErr = err
		} else {
			g.policy = p
		}
	}
	return g
}

// Call is one tool call to decide: its target, or why its payload could
// not be read.
type Call struct {
	Target gr.Target
	// Err is set when the payload could not be read; Target is then
	// unset.
	Err *CallError
}

// CallError is why a call's payload could not be read, and the rule
// that refuses the call.
type CallError struct {
	Rule string
	Why  string
}

func deny(rule, why string) Decision {
	return Decision{Outcome: Deny, Finding: &Finding{Rule: rule, Why: why}}
}

// Decide decides a call, in this order: a policy error refuses it under
// policy-error, in observe mode too; a call error refuses it under its
// rule; a target is decided by the command or path rules. Observe is
// set from the policy's mode on every decision but a policy error.
func (g *Guard) Decide(c Call) Decision {
	if g.policyErr != nil {
		return deny(gr.IDPolicyError, gr.PolicyFile+" cannot be used: "+g.policyErr.Error())
	}
	var d Decision
	switch t := c.Target; {
	case c.Err != nil:
		d = deny(c.Err.Rule, c.Err.Why)
	case gr.IsCommandTool(t.Tool):
		d = g.decideCommand(t)
	case gr.IsEditTool(t.Tool):
		d = g.decidePath(t)
	}
	d.Observe = g.policy.Mode == gr.Observe
	return d
}

func (g *Guard) decideCommand(t gr.Target) Decision { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	shell := gr.ShellFor(t.Tool)
	cmds, err := gr.Commands(shell, t.Command)
	if err != nil {
		return deny(gr.IDParseError, "the command "+err.Error())
	}
	var d Decision
	var ask *Finding
	for i := range cmds {
		c := &cmds[i]
		if c.Dynamic() {
			return Decision{Outcome: Deny, Lifted: d.Lifted, Finding: &Finding{Rule: gr.IDDynamicCommand,
				Why: "the command name holds an expansion, so what runs is not known", Command: c}}
		}
		// A shell that runs its standard input runs a script the guard
		// cannot see (curl … | bash). A here-document's script is parsed
		// by guardrule: its commands follow.
		if c.ScriptSource == gr.ScriptFromStdin {
			return Decision{Outcome: Deny, Lifted: d.Lifted, Finding: &Finding{Rule: gr.IDDynamicCommand,
				Why: "the shell runs a script it reads from standard input, so what runs is not known", Command: c}}
		}
		for _, r := range commandRulesFor(c.Shell) {
			if !r.match(*c) {
				continue
			}
			f := Finding{Rule: r.id, Why: r.why, Command: c}
			if e := g.exception(r.id, func(e gr.Exception) bool { return e.MatchesCommand(*c) }); e != nil && !r.fixed {
				f.Exception = e
				d.Lifted = append(d.Lifted, f)
				continue
			}
			d.Outcome, d.Finding = Deny, &f
			return d
		}
		// The paths the command writes get the guardrail path rules (spec
		// 0058). A word with an expansion is not a written path: what it
		// names is not known, and refusing it would refuse common scripts.
		for _, w := range c.Writes {
			if !w.Literal {
				continue
			}
			p := g.normalize(w.Text)
			if f := g.checkPath(shellWriteRules, p, gr.RelPath(g.repo.Root, p), c, &d); f != nil {
				d.Outcome, d.Finding = Deny, f
				return d
			}
		}
		for _, r := range g.policy.Rules {
			if r.Command == "" || !r.AppliesTo(t.Tool) || !r.MatchesCommand(*c) {
				continue
			}
			f := Finding{Rule: r.ID, Why: r.Reason, Command: c}
			if r.Level == gr.Deny {
				d.Outcome, d.Finding = Deny, &f
				return d
			}
			if ask == nil {
				ask = &f
			}
		}
	}
	if ask != nil {
		d.Outcome, d.Finding = Ask, ask
	}
	return d
}

func (g *Guard) decidePath(t gr.Target) Decision {
	var d Decision
	p := g.normalize(t.Path)
	if f := g.checkPath(pathRules, p, t.Path, nil, &d); f != nil {
		d.Outcome, d.Finding = Deny, f
		return d
	}
	var ask *Finding
	for _, r := range g.policy.Rules {
		if r.Path == "" || !r.AppliesTo(t.Tool) || !r.MatchesPath(p, g.repo.Root) {
			continue
		}
		f := Finding{Rule: r.ID, Why: r.Reason, Path: t.Path}
		if r.Level == gr.Deny {
			d.Outcome, d.Finding = Deny, &f
			return d
		}
		if ask == nil {
			ask = &f
		}
	}
	if ask != nil {
		d.Outcome, d.Finding = Ask, ask
	}
	return d
}

// checkPath checks the path p against rules, in order, and returns the
// first refusal. A rule an exception lifts is added to d.Lifted. shown is
// the path a finding names; c is the command that writes p, or nil for
// an edit.
func (g *Guard) checkPath(rules []pathRule, p, shown string, c *gr.Command, d *Decision) *Finding {
	for _, r := range rules {
		if !r.match(g, p) {
			continue
		}
		f := Finding{Rule: r.id, Why: r.why, Path: shown, Command: c}
		if e := g.exception(r.id, func(e gr.Exception) bool { return e.MatchesPath(p, g.repo.Root) }); e != nil && !r.fixed {
			f.Exception = e
			d.Lifted = append(d.Lifted, f)
			continue
		}
		return &f
	}
	return nil
}

// normalize is the one reading of a path the guard checks, from an edit
// tool or a command that writes it (spec 0058, "Path resolution"; spec
// 0069): backslashes become slashes; on a project root with a drive
// letter, Git Bash's /d/… is D:/…; a relative path is joined to g.dir,
// the payload's cwd, else to the project root; and the result is
// cleaned. The rules then see only absolute, clean, slash-separated
// paths.
func (g *Guard) normalize(p string) string {
	root := g.repo.Root
	p = strings.ReplaceAll(p, `\`, "/")
	if len(root) >= 2 && root[1] == ':' && isGitBashDrive(p) {
		p = strings.ToUpper(p[1:2]) + ":" + p[2:]
	}
	if !gr.IsAbs(p) {
		p = filepath.Join(cmp.Or(g.dir, root), filepath.FromSlash(p))
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
}

// isGitBashDrive reports whether p starts with a Git Bash drive, such as
// /d or /d/….
func isGitBashDrive(p string) bool {
	return len(p) >= 2 && p[0] == '/' && unicode.IsLetter(rune(p[1])) &&
		(len(p) == 2 || p[2] == '/')
}

// exception returns the first exception for rule that matches.
func (g *Guard) exception(rule string, matches func(gr.Exception) bool) *gr.Exception {
	for i, e := range g.policy.Exceptions {
		if e.Rule == rule && matches(e) {
			return &g.policy.Exceptions[i]
		}
	}
	return nil
}
