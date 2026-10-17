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

// ciVibeCalls reads every CI step of every parsed workflow, except the
// jobs in exempt.
func ciVibeCalls(parsed []*Workflow, exempt []ciJob) []vibeCall {
	var out []vibeCall
	for _, wf := range parsed {
		for _, j := range wf.Jobs {
			if slices.ContainsFunc(exempt, func(e ciJob) bool { return e.job == j }) {
				continue
			}
			for _, s := range j.Steps {
				out = append(out, callsVibe(wf.File, fmt.Sprintf("line %d: job %s, step %d", s.Line, j.ID, s.Index+1), s.Run.Commands)...)
			}
		}
	}
	return out
}
