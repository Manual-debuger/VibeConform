package contract

import (
	"fmt"
	"strings"
)

// The static checks that read hook managers, Claude Code settings, the
// guard policy and update bots: H-01, H-02, A-01, A-04, A-05's and
// A-07's static parts, A-06 and D-02 (spec 0052, "Recognition rules").
// Hook commands are parsed with parseCommands, and "called" is called /
// callsAny against env.entrypointCommands(...).

// checkH01: a recognized hook manager's configuration is committed, and
// it parses.
func checkH01(env *Env) PartResult {
	ms := env.hookManagers()
	if len(ms) == 0 {
		return Fail(noHookManager())
	}
	var broken []Finding
	for _, m := range ms {
		broken = append(broken, m.Broken...)
	}
	if len(broken) > 0 {
		return Fail(broken...)
	}
	return Pass()
}

// checkH02: the pre-commit hook calls fmt-check or fmt; the pre-push hook
// calls verify-fast or verify. The hooks of every recognized manager
// count together. With no recognized manager it cannot be decided.
func checkH02(env *Env) PartResult {
	ms := env.hookManagers()
	if len(ms) == 0 {
		f := noHookManager()
		f.Message += ", so the hooks cannot be read"
		return undecided(f)
	}
	var broken []Finding
	cmds := map[string][]shellCommand{}
	for _, m := range ms {
		broken = append(broken, m.Broken...)
		for _, h := range gitHooks {
			broken = append(broken, m.ScriptBroken[h]...)
			cmds[h] = append(cmds[h], m.commands(h)...)
		}
	}
	if len(broken) > 0 {
		return Fail(broken...)
	}
	return merge(
		hookCalls(env, ms[0], hookPreCommit, cmds[hookPreCommit], entrypointFmtCheck, entrypointFmt),
		hookCalls(env, ms[0], hookPrePush, cmds[hookPrePush], entrypointVerifyFast, entrypointVerify),
	)
}

// hookCalls passes when a Git hook's commands call either of two
// entrypoints. When neither is usable it fails with V-04's finding.
func hookCalls(env *Env, m *hookManager, hook string, cmds []shellCommand, first, second string) PartResult {
	var usable []string
	var missing []Finding
	for _, name := range []string{first, second} {
		ep, f := env.entrypointCommands(name)
		if f != nil {
			missing = append(missing, *f)
			continue
		}
		if called(ep, cmds) {
			return Pass()
		}
		usable = append(usable, name)
	}
	if len(usable) == 0 {
		return Fail(missing...)
	}
	file := m.fileFor(hook)
	fix := fmt.Sprintf("make %s run %s", hook, quoteCommands(env.entrypointSource(usable[0])))
	if !env.repo.HasFile(file) {
		fix = fmt.Sprintf("create %s that runs %s", file, quoteCommands(env.entrypointSource(usable[0])))
	}
	return Fail(Finding{
		File: file,
		Message: fmt.Sprintf("%s calls neither %s nor %s", hook,
			describeEntrypoint(env, first), describeEntrypoint(env, second)),
		Fix: fix,
	})
}

// describeEntrypoint names an entrypoint with its commands, such as
// "verify-fast (task verify:fast)".
func describeEntrypoint(env *Env, name string) string {
	src := env.entrypointSource(name)
	if len(src) == 0 {
		return name
	}
	trimmed := make([]string, len(src))
	for i, s := range src {
		trimmed[i] = strings.TrimSpace(s)
	}
	return name + " (" + strings.Join(trimmed, "; ") + ")"
}

// guardedTools are the tools A-01 requires a PreToolUse hook for.
var guardedTools = []string{"Bash", "PowerShell", "Write", "Edit", "MultiEdit"}

// checkA01: for each of Bash, PowerShell, Write, Edit and MultiEdit, a
// PreToolUse entry with a command hook matches it.
func checkA01(env *Env) PartResult {
	s, res, ok := env.readClaudeSettings("add a PreToolUse hook with matcher " +
		strings.Join(guardedTools, "|") + " that runs the guard")
	if !ok {
		return res
	}
	var unguarded []string
	for _, tool := range guardedTools {
		if len(s.matching(eventPreToolUse, tool)) == 0 {
			unguarded = append(unguarded, tool)
		}
	}
	if len(unguarded) == 0 {
		return Pass()
	}
	fix := "add " + joinWords(unguarded, " and ") + " to the matcher of the hook that runs the guard"
	if len(s.commandEntries(eventPreToolUse)) == 0 {
		fix = "add a PreToolUse hook with matcher " + strings.Join(guardedTools, "|") + " that runs the guard"
	}
	return Fail(Finding{
		File:    claudeSettingsFile,
		Message: "no PreToolUse command hook matches " + joinWords(unguarded, " and "),
		Fix:     fix,
	})
}

// checkA05 (static part): a Stop hook calls verify-fast.
func checkA05(env *Env) PartResult {
	verifyFast, missing := env.entrypointCommands(entrypointVerifyFast)
	if missing != nil {
		return Fail(*missing)
	}
	run := quoteCommands(env.entrypointSource(entrypointVerifyFast))
	s, res, ok := env.readClaudeSettings("add a Stop hook that runs " + run)
	if !ok {
		return res
	}
	var cmds []shellCommand
	for _, h := range s.commandEntries(eventStop) {
		cmds = append(cmds, h.calls()...)
	}
	if called(verifyFast, cmds) {
		return Pass()
	}
	return Fail(Finding{
		File:    claudeSettingsFile,
		Message: "no Stop hook calls " + describeEntrypoint(env, entrypointVerifyFast),
		Fix:     "make a Stop hook run " + run + " as a command of its own",
	})
}

// checkA06: a PostToolUse entry that matches both Write and Edit calls
// fmt.
func checkA06(env *Env) PartResult {
	format, missing := env.entrypointCommands(entrypointFmt)
	if missing != nil {
		return Fail(*missing)
	}
	fix := "add a PostToolUse hook with matcher Write|Edit that runs " + quoteCommands(env.entrypointSource(entrypointFmt))
	s, res, ok := env.readClaudeSettings(fix)
	if !ok {
		return res
	}
	for _, h := range s.matching(eventPostToolUse, "Write", "Edit") {
		if called(format, h.calls()) {
			return Pass()
		}
	}
	return Fail(Finding{
		File:    claudeSettingsFile,
		Message: "no PostToolUse hook that matches Write and Edit calls " + describeEntrypoint(env, entrypointFmt),
		Fix:     fix,
	})
}

// checkA07 (static part): a SessionStart entry with a command hook
// exists.
func checkA07(env *Env) PartResult {
	const fix = "add a SessionStart hook that runs a command which prints facts for the agent and changes no file"
	s, res, ok := env.readClaudeSettings(fix)
	if !ok {
		return res
	}
	if len(s.commandEntries(eventSessionStart)) > 0 {
		return Pass()
	}
	return Fail(Finding{
		File:    claudeSettingsFile,
		Message: "has no SessionStart command hook",
		Fix:     fix,
	})
}
