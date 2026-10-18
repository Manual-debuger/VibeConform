// Package guard is vibe-guard, the reference agent guard (spec 0053,
// "vibe-guard"). It decides Claude Code tool calls against the baseline
// rules of contract 1.0.0 and the repository's own policy.json version 2.
// It imports internal/guardrule, which it shares with the contract audit,
// and nothing from internal/contract or the old template system. It
// opens no network connection.
package guard

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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

// Guard decides tool calls in one repository.
type Guard struct {
	Repo Repo
	// Policy is the repository's policy.json, or the default (enforce,
	// no rules, no exceptions) when it has none.
	Policy *gr.Policy
	// PolicyErr is set when policy.json cannot be read or is invalid.
	// Every call is then refused under policy-error.
	PolicyErr error
	// Now is the time an unlock marker is compared with.
	Now time.Time
}

// New loads the guard of the repository at repo.
func New(repo Repo, now time.Time) *Guard {
	g := &Guard{Repo: repo, Now: now, Policy: &gr.Policy{Version: 2, Mode: gr.Enforce}}
	data, err := os.ReadFile(filepath.Join(repo.Root, filepath.FromSlash(gr.PolicyFile)))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		g.PolicyErr = err
	default:
		g.Policy, g.PolicyErr = gr.ParsePolicy(data)
	}
	return g
}

func deny(rule, why string) Decision {
	return Decision{Outcome: Deny, Finding: &Finding{Rule: rule, Why: why}}
}

// Decide decides a tool call whose target was read without error.
func (g *Guard) Decide(t gr.Target) Decision {
	if g.PolicyErr != nil {
		return deny(gr.IDPolicyError, gr.PolicyFile+" cannot be used: "+g.PolicyErr.Error())
	}
	var d Decision
	switch {
	case gr.IsCommandTool(t.Tool):
		d = g.decideCommand(t)
	case gr.IsEditTool(t.Tool):
		d = g.decidePath(t)
	}
	d.Observe = g.Policy.Mode == gr.Observe
	return d
}

// DecideError decides a call whose payload could not be read.
func (g *Guard) DecideError(rule, why string) Decision {
	if g.PolicyErr != nil {
		return deny(gr.IDPolicyError, gr.PolicyFile+" cannot be used: "+g.PolicyErr.Error())
	}
	d := deny(rule, why)
	d.Observe = g.Policy.Mode == gr.Observe
	return d
}

func (g *Guard) decideCommand(t gr.Target) Decision {
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
		if readsScriptFromStdin(cmds, i) {
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
		for _, r := range g.Policy.Rules {
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
	for _, r := range pathRules {
		if !r.match(g, t.Path) {
			continue
		}
		f := Finding{Rule: r.id, Why: r.why, Path: t.Path}
		if e := g.exception(r.id, func(e gr.Exception) bool { return e.MatchesPath(t.Path, g.Repo.Root) }); e != nil && !r.fixed {
			f.Exception = e
			d.Lifted = append(d.Lifted, f)
			continue
		}
		d.Outcome, d.Finding = Deny, &f
		return d
	}
	var ask *Finding
	for _, r := range g.Policy.Rules {
		if r.Path == "" || !r.AppliesTo(t.Tool) || !r.MatchesPath(t.Path, g.Repo.Root) {
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

// exception returns the first exception for rule that matches.
func (g *Guard) exception(rule string, matches func(gr.Exception) bool) *gr.Exception {
	for i, e := range g.Policy.Exceptions {
		if e.Rule == rule && matches(e) {
			return &g.Policy.Exceptions[i]
		}
	}
	return nil
}

var posixShells = []string{"bash", "sh", "zsh", "dash", "ksh", "ash", "mksh"}

// readsScriptFromStdin reports whether cmds[i] is a POSIX shell given no
// -c string, no script file and no here-document: it runs whatever its
// standard input holds (curl … | bash), which the guard cannot see.
func readsScriptFromStdin(cmds []gr.Command, i int) bool {
	c := cmds[i]
	name := c.Name()
	if !slices.Contains(posixShells, name) {
		return false
	}
	for _, w := range c.Words[1:] {
		if !w.Literal || !strings.HasPrefix(w.Text, "-") && !strings.HasPrefix(w.Text, "+") {
			return false // a script file, or something the guard reads as one
		}
		switch {
		case w.Text == "--version" || w.Text == "--help":
			return false
		case !strings.HasPrefix(w.Text, "--") && strings.Contains(w.Text, "c"):
			return false // -c: guardrule parses its string
		}
	}
	// A here-document fed to the shell is parsed by guardrule: its
	// commands follow, one level deeper, found through "<name> <<".
	if i+1 < len(cmds) {
		next := cmds[i+1]
		if next.Depth == c.Depth+1 && len(next.Via) > 0 && next.Via[len(next.Via)-1] == name+" <<" {
			return false
		}
	}
	return true
}
