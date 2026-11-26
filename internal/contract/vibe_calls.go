package contract

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// The places R-01's static part reads for a call of vibe (spec 0052,
// "Recognition rules", R-01): every declared entrypoint, every hook
// command, and every CI step but those of a job that satisfies C-04.
//
// Hook commands are read through the models the H and A rules read, so
// the three agree on which files count and how they parse: the hook
// managers (hookManagers) and the Claude Code settings
// (readClaudeSettings). R-01 reads every Git hook those managers
// configure, not only pre-commit and pre-push, and every Claude Code hook
// event.

// vibeReading is how the audited contract recognizes a call of vibe, the
// one reading R-01's three places and C-04's audit job share (spec 0072,
// "R-01 and C-04"). Under 1.2.0 a command's first word must be the
// literal vibe. Under 2.0.0 its last path element, after / or \, must be
// the literal vibe or vibe.exe, compared case-sensitively; the directory
// part may hold an expansion, the last element may not.
type vibeReading struct {
	byPath bool
}

// vibeReadingOf is the reading of the contract env audits.
func vibeReadingOf(env *Env) vibeReading {
	return vibeReading{byPath: env.auditedContract() != Contract1}
}

// isVibe reports whether w, a command's first word, names vibe.
func (r vibeReading) isVibe(w shellWord) bool {
	if !r.byPath {
		return w.Literal && w.Text == "vibe"
	}
	tail := w.Tail()
	i := strings.LastIndexAny(tail, `/\`)
	if i < 0 && !w.Literal {
		// The last element holds an expansion.
		return false
	}
	last := tail[i+1:]
	return last == "vibe" || last == "vibe.exe"
}

// runs reports whether c calls vibe with args as its next words, each
// literal; more words may follow them.
func (r vibeReading) runs(c shellCommand, args ...string) bool {
	if len(c.Words) <= len(args) || !r.isVibe(c.Words[0]) {
		return false
	}
	for i, a := range args {
		if w := c.Words[i+1]; !w.Literal || w.Text != a {
			return false
		}
	}
	return true
}

// runsAudit reports whether some command in cmds runs vibe audit.
func (r vibeReading) runsAudit(cmds []shellCommand) bool {
	return slices.ContainsFunc(cmds, func(c shellCommand) bool { return r.runs(c, "audit") })
}

// vibeCall is one simple command that calls vibe.
type vibeCall struct {
	file  string
	where string
	cmd   shellCommand
}

// callsVibe returns the commands in cmds that call vibe.
func (r vibeReading) callsVibe(file, where string, cmds []shellCommand) []vibeCall {
	var out []vibeCall
	for _, c := range cmds {
		if r.runs(c) {
			out = append(out, vibeCall{file, where, c})
		}
	}
	return out
}

// entrypointVibeCalls reads the declared entrypoints. An entrypoint V-04
// finds malformed is skipped: V-04 fails it.
func entrypointVibeCalls(env *Env) []vibeCall {
	var out []vibeCall
	r := vibeReadingOf(env)
	for _, name := range entrypointNames {
		if _, ok := env.config.Entrypoints[name]; !ok {
			continue
		}
		out = append(out, r.callsVibe(env.config.file, "entrypoints."+name, env.entrypoint(name).commands)...)
	}
	return out
}

// hookVibeCalls reads every hook command: each Git hook of each
// recognized hook manager, and each command hook of .claude/settings.json.
// A configuration or hook script that does not parse fails R-01, which
// cannot hold for it; settings that cannot be read leave it undecided.
func hookVibeCalls(env *Env) ([]vibeCall, PartResult) { //nolint:gocognit // baseline: pre-existing complexity
	var calls []vibeCall
	var broken []Finding
	seen := map[string]bool{}
	r := vibeReadingOf(env)
	for _, m := range env.hookManagers() {
		broken = append(broken, m.Broken...)
		for _, hook := range slices.Sorted(maps.Keys(m.ScriptBroken)) {
			broken = append(broken, m.ScriptBroken[hook]...)
		}
		for _, hook := range slices.Sorted(maps.Keys(m.Entries)) {
			for _, en := range m.Entries[hook] {
				for _, c := range r.callsVibe(en.File, en.Where, en.Commands) {
					// A pre-commit hook in several stages is one entry.
					key := c.file + "\x00" + c.where + "\x00" + c.cmd.String()
					if !seen[key] {
						seen[key] = true
						calls = append(calls, c)
					}
				}
			}
		}
	}
	res := failIf(broken)

	if env.repo.HasFile(claudeSettingsFile) {
		s, sres, ok := env.readClaudeSettings("")
		if !ok {
			return calls, merge(res, sres)
		}
		for _, event := range slices.Sorted(maps.Keys(s.Events)) {
			for _, entry := range s.Events[event] {
				calls = append(calls, r.callsVibe(claudeSettingsFile, "hooks."+event, entry.calls())...)
			}
		}
	}
	return calls, res
}

// ciVibeCalls reads every script of every CI job (GitHub run: steps;
// GitLab before_script, script and after_script), except the jobs in
// exempt. In a job that calls no declared entrypoint, a command that is
// exactly vibe --version tests the installed binary and is not a call of
// vibe (spec 0055, "R-01 and tool-installation tests").
func ciVibeCalls(env *Env, ci *ciModel, exempt []*ciJob) []vibeCall {
	var out []vibeCall
	r := vibeReadingOf(env)
	for _, j := range ci.Jobs {
		if slices.ContainsFunc(exempt, func(e *ciJob) bool { return e.Key() == j.Key() }) {
			continue
		}
		var all []shellCommand
		for _, s := range j.Scripts {
			all = append(all, s.Commands...)
		}
		testsTools := !callsAnEntrypoint(env, all)
		for _, s := range j.Scripts {
			for _, c := range r.callsVibe(s.File, fmt.Sprintf("line %d: %s", s.Line, s.Where), s.Commands) {
				if testsTools && len(c.cmd.Words) == 2 && r.runs(c.cmd, "--version") {
					continue
				}
				out = append(out, c)
			}
		}
	}
	return out
}

// callsAnEntrypoint reports whether some command in cmds calls a command
// of a declared entrypoint. Calling one command of a list entrypoint is
// enough: the job then runs what the entrypoint runs, and vibe there is
// not only a tool-installation test.
func callsAnEntrypoint(env *Env, cmds []shellCommand) bool {
	for _, name := range entrypointNames {
		if _, ok := env.config.Entrypoints[name]; !ok {
			continue
		}
		for _, want := range env.entrypoint(name).commands {
			if callsAny(cmds, want) {
				return true
			}
		}
	}
	return false
}
