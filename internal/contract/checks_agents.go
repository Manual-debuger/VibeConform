package contract

import (
	"fmt"
	"strings"
)

// The static checks that read hook managers, Claude Code settings, the
// guard policy and update bots: H-01, H-02, A-01, A-04, A-05's and
// A-07's static parts, A-06 and D-02 (spec 0052, "Recognition rules").
// Hook commands are parsed with ParseCommands, and "called" is Called /
// CallsAny against env.Entrypoint(...).

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
		return Undecided(f)
	}
	var broken []Finding
	cmds := map[string][]Command{}
	for _, m := range ms {
		broken = append(broken, m.Broken...)
		broken = append(broken, m.ScriptBroken...)
		for _, h := range gitHooks {
			cmds[h] = append(cmds[h], m.Commands[h]...)
		}
	}
	if len(broken) > 0 {
		return Fail(broken...)
	}
	return Merge(
		hookCalls(env, ms[0], hookPreCommit, cmds[hookPreCommit], EntrypointFmtCheck, EntrypointFmt),
		hookCalls(env, ms[0], hookPrePush, cmds[hookPrePush], EntrypointVerifyFast, EntrypointVerify),
	)
}

// hookCalls passes when a Git hook's commands call either of two
// entrypoints. When neither is usable it fails with V-04's finding.
func hookCalls(env *Env, m *hookManager, hook string, cmds []Command, first, second string) PartResult {
	var usable []string
	var missing []Finding
	for _, name := range []string{first, second} {
		ep, f := env.Entrypoint(name)
		if f != nil {
			missing = append(missing, *f)
			continue
		}
		if Called(ep, cmds) {
			return Pass()
		}
		usable = append(usable, name)
	}
	if len(usable) == 0 {
		return Fail(missing...)
	}
	file := m.fileFor(hook)
	fix := fmt.Sprintf("make %s run %s", hook, quoteCommands(env.EntrypointSource(usable[0])))
	if !env.Repo.HasFile(file) {
		fix = fmt.Sprintf("create %s that runs %s", file, quoteCommands(env.EntrypointSource(usable[0])))
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
	src := env.EntrypointSource(name)
	if len(src) == 0 {
		return name
	}
	trimmed := make([]string, len(src))
	for i, s := range src {
		trimmed[i] = strings.TrimSpace(s)
	}
	return name + " (" + strings.Join(trimmed, "; ") + ")"
}

func checkA01(_ *Env) PartResult { return notImplementedYet() }

func checkA04(_ *Env) PartResult { return notImplementedYet() }

func checkA05(_ *Env) PartResult { return notImplementedYet() }

func checkA06(_ *Env) PartResult { return notImplementedYet() }

func checkA07(_ *Env) PartResult { return notImplementedYet() }

func checkD02(_ *Env) PartResult { return notImplementedYet() }

// guardListings returns what the report lists for review: the guard
// policy's exceptions (A-04) and the exceptions and extra rules of a
// tracked .claude/hooks/policy.local.json (spec 0050). It reads, and
// decides nothing.
func guardListings(_ *Env) ([]GuardException, []LocalAdjustment) { return nil, nil }
