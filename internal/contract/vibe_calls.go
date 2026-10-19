package contract

import (
	"fmt"
	"maps"
	"slices"
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

// vibeCall is one simple command whose first word is vibe.
type vibeCall struct {
	file  string
	where string
	cmd   Command
}

// callsVibe returns the commands in cmds whose first word is vibe.
func callsVibe(file, where string, cmds []Command) []vibeCall {
	var out []vibeCall
	for _, c := range cmds {
		if c.HasPrefix("vibe") {
			out = append(out, vibeCall{file, where, c})
		}
	}
	return out
}

// entrypointVibeCalls reads the declared entrypoints. An entrypoint V-04
// finds malformed is skipped: V-04 fails it.
func entrypointVibeCalls(env *Env) []vibeCall {
	var out []vibeCall
	for _, name := range entrypointNames {
		if _, ok := env.Config.Entrypoints[name]; !ok {
			continue
		}
		out = append(out, callsVibe(env.Config.File, "entrypoints."+name, env.entrypoint(name).commands)...)
	}
	return out
}

// hookVibeCalls reads every hook command: each Git hook of each
// recognized hook manager, and each command hook of .claude/settings.json.
// A configuration or hook script that does not parse fails R-01, which
// cannot hold for it; settings that cannot be read leave it undecided.
func hookVibeCalls(env *Env) ([]vibeCall, PartResult) {
	var calls []vibeCall
	var broken []Finding
	seen := map[string]bool{}
	for _, m := range env.hookManagers() {
		broken = append(broken, m.Broken...)
		for _, hook := range slices.Sorted(maps.Keys(m.ScriptBroken)) {
			broken = append(broken, m.ScriptBroken[hook]...)
		}
		for _, hook := range slices.Sorted(maps.Keys(m.Entries)) {
			for _, en := range m.Entries[hook] {
				for _, c := range callsVibe(en.File, en.Where, en.Commands) {
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

	if env.Repo.HasFile(claudeSettingsFile) {
		s, sres, ok := env.readClaudeSettings("")
		if !ok {
			return calls, Merge(res, sres)
		}
		for _, event := range slices.Sorted(maps.Keys(s.Events)) {
			for _, entry := range s.Events[event] {
				calls = append(calls, callsVibe(claudeSettingsFile, "hooks."+event, entry.calls())...)
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
func ciVibeCalls(env *Env, ci *CI, exempt []*CIJob) []vibeCall {
	var out []vibeCall
	for _, j := range ci.Jobs {
		if slices.ContainsFunc(exempt, func(e *CIJob) bool { return e.Key() == j.Key() }) {
			continue
		}
		var all []Command
		for _, s := range j.Scripts {
			all = append(all, s.Commands...)
		}
		testsTools := !callsAnEntrypoint(env, all)
		for _, s := range j.Scripts {
			for _, c := range callsVibe(s.File, fmt.Sprintf("line %d: %s", s.Line, s.Where), s.Commands) {
				if testsTools && c.cmd.Calls(vibeVersion) {
					continue
				}
				out = append(out, c)
			}
		}
	}
	return out
}

// vibeVersion is the one vibe command a tool-installation test may run.
var vibeVersion = Command{Words: []Word{{Text: "vibe", Literal: true}, {Text: "--version", Literal: true}}}

// callsAnEntrypoint reports whether some command in cmds calls a command
// of a declared entrypoint. Calling one command of a list entrypoint is
// enough: the job then runs what the entrypoint runs, and vibe there is
// not only a tool-installation test.
func callsAnEntrypoint(env *Env, cmds []Command) bool {
	for _, name := range entrypointNames {
		if _, ok := env.Config.Entrypoints[name]; !ok {
			continue
		}
		for _, want := range env.entrypoint(name).commands {
			if CallsAny(cmds, want) {
				return true
			}
		}
	}
	return false
}
