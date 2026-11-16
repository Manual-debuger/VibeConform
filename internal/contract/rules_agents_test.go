package contract_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The rules that read hook managers, Claude Code settings, the guard
// policy and update bots (spec 0052, "Recognition rules"), through
// contract.Audit (ADR 0028). Each case is the baseline plus one change.

// agentsLefthookWith is a lefthook.yml whose pre-commit and pre-push hooks run
// one command each.
func agentsLefthookWith(preCommit, prePush string) contracttest.Edit {
	return contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: "+preCommit+
		"\npre-push:\n  commands:\n    b:\n      run: "+prePush+"\n")
}

// agentsPreCommitConfig is a .pre-commit-config.yaml whose local hooks run
// task fmt:check at commit and task verify:fast at push.
const agentsPreCommitConfig = `repos:
  - repo: local
    hooks:
      - id: fmt
        name: format
        entry: task
        args: [fmt:check]
        language: system
        pass_filenames: false
      - id: verify
        name: verify
        entry: task verify:fast
        language: system
        stages: [pre-push]
        pass_filenames: false
`

// H-01: a recognized hook manager's configuration is committed.
func TestRuleH01(t *testing.T) {
	t.Parallel()
	contracttest.RunRuleCases(t, "H-01", []contracttest.RuleCase{
		{Name: "baseline lefthook", Want: "pass"},
		{Name: ".lefthook.yml", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"),
			contracttest.WithFile(".lefthook.yml", "pre-push:\n  commands:\n    v:\n      run: task verify:fast\n")}},
		{Name: "pre-commit", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".pre-commit-config.yaml", agentsPreCommitConfig)}},
		{Name: "husky", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".husky/pre-push", "task verify:fast\n")}},
		{Name: "no hook manager", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile("lefthook.yml")},
			Findings: []string{"lefthook.yml: no recognized hook manager (lefthook, pre-commit, husky)", "Fix: "}},
		{Name: "untracked lefthook.yml", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithUntracked("lefthook.yml", "pre-push:\n  commands: {}\n")},
			Findings: []string{"no recognized hook manager"}},
		{Name: "lefthook-local.yml alone is personal", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile("lefthook-local.yml", "pre-push:\n  commands: {}\n")},
			Findings: []string{"no recognized hook manager"}},
		{Name: "lefthook.yml does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("lefthook.yml", "pre-commit: [\n")},
			Findings: []string{"lefthook.yml: does not parse"}},
		{Name: ".pre-commit-config.yaml does not parse", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".pre-commit-config.yaml", "repos: {\n")},
			Findings: []string{".pre-commit-config.yaml: does not parse"}},
	})
}

// H-02: the pre-commit hook calls fmt-check or fmt; the pre-push hook
// calls verify-fast or verify.
func TestRuleH02(t *testing.T) {
	t.Parallel()
	contracttest.RunRuleCases(t, "H-02", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "fmt and verify", Want: "pass", Edits: []contracttest.Edit{agentsLefthookWith("task fmt", "task verify")}},
		{Name: "inside a list with redirection", Want: "pass",
			Edits: []contracttest.Edit{agentsLefthookWith(`"go vet ./... && task fmt:check >/dev/null"`, `"task verify:fast || exit 1"`)}},
		{Name: "pre-push calls neither", Want: "fail",
			Edits: []contracttest.Edit{agentsLefthookWith("task fmt:check", "go test ./...")},
			Findings: []string{"lefthook.yml: pre-push calls neither verify-fast (task verify:fast) nor verify (task verify)",
				"Fix: make pre-push run `task verify:fast`"}},
		{Name: "pre-commit calls neither", Want: "fail",
			Edits: []contracttest.Edit{agentsLefthookWith("gofmt -l .", "task verify:fast")},
			Findings: []string{"lefthook.yml: pre-commit calls neither fmt-check (task fmt:check) nor fmt (task fmt)",
				"Fix: make pre-commit run `task fmt:check`"}},
		{Name: "an argument too many is not called", Want: "fail",
			Edits:    []contracttest.Edit{agentsLefthookWith("task fmt:check -v", "task verify:fast")},
			Findings: []string{"pre-commit calls neither"}},
		{Name: "no pre-push hook", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\n")},
			Findings: []string{"pre-push calls neither"}},
		{Name: "lefthook jobs and nested groups", Want: "pass", Edits: []contracttest.Edit{contracttest.WithFile("lefthook.yml", `pre-commit:
  jobs:
    - name: vet
      run: go vet ./...
    - group:
        parallel: true
        jobs:
          - run: task fmt:check
pre-push:
  jobs:
    - group:
        jobs:
          - group:
              jobs:
                - run: task verify:fast
`)}},
		{Name: "lefthook scripts", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  scripts:\n    \"verify.sh\":\n      runner: bash\n"),
			contracttest.WithFile(".lefthook/pre-push/verify.sh", "#!/bin/bash\nset -e\ntask verify:fast\n")}},
		{Name: "lefthook script in a job", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  jobs:\n    - script: verify.sh\n      runner: sh\n"),
			contracttest.WithFile(".lefthook/pre-push/verify.sh", "task verify:fast\n")}},
		{Name: "untracked lefthook script calls nothing", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  scripts:\n    \"verify.sh\":\n      runner: bash\n"),
			contracttest.WithUntracked(".lefthook/pre-push/verify.sh", "task verify:fast\n")},
			Findings: []string{"pre-push calls neither"}},
		{Name: "lefthook extends a tracked file", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends:\n  - hooks/push.yml\npre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			contracttest.WithFile("hooks/push.yml", "pre-push:\n  commands:\n    v:\n      run: task verify\n")}},
		{Name: "lefthook extends an untracked file", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends:\n  - hooks/push.yml\npre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			contracttest.WithUntracked("hooks/push.yml", "pre-push:\n  commands:\n    v:\n      run: task verify\n")},
			Findings: []string{"pre-push calls neither"}},
		{Name: "lefthook extends overrides a command", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends: [hooks/more.yml]\n"+
				"pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  commands:\n    v:\n      run: task verify\n"),
			contracttest.WithFile("hooks/more.yml", "pre-push:\n  commands:\n    v:\n      run: go test ./...\n")},
			Findings: []string{"pre-push calls neither"}},
		{Name: "extended file does not parse", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends: [hooks/more.yml]\npre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			contracttest.WithFile("hooks/more.yml", "pre-push: [\n")},
			Findings: []string{"hooks/more.yml: does not parse"}},
		{Name: "lefthook-local.yml is not read", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			contracttest.WithFile("lefthook-local.yml", "pre-push:\n  commands:\n    v:\n      run: task verify\n")},
			Findings: []string{"pre-push calls neither"}},
		{Name: "pre-commit local hooks", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".pre-commit-config.yaml", agentsPreCommitConfig)}},
		{Name: "pre-commit default_stages and legacy stage names", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".pre-commit-config.yaml", `default_stages: [push]
repos:
  - repo: local
    hooks:
      - id: verify
        name: verify
        entry: task verify
        language: system
      - id: fmt
        name: fmt
        entry: task fmt
        language: system
        stages: [commit]
`)}},
		{Name: "pre-commit remote hooks call nothing", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".pre-commit-config.yaml", `repos:
  - repo: https://example.com/hooks
    rev: v1.0.0
    hooks:
      - id: fmt
        entry: task fmt:check
`)},
			Findings: []string{".pre-commit-config.yaml: pre-commit calls neither"}},
		{Name: "pre-commit hook in the pre-commit stage only", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".pre-commit-config.yaml", `repos:
  - repo: local
    hooks:
      - id: all
        name: all
        entry: task verify:fast
        language: system
      - id: fmt
        name: fmt
        entry: task fmt
        language: system
`)},
			Findings: []string{".pre-commit-config.yaml: pre-push calls neither"}},
		{Name: "husky", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"),
			contracttest.WithFile(".husky/pre-commit", "#!/usr/bin/env sh\nif ! task fmt:check; then\n  exit 1\nfi\n"),
			contracttest.WithFile(".husky/pre-push", "task verify:fast\n")}},
		{Name: "husky without pre-push", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".husky/pre-commit", "task fmt\n")},
			Findings: []string{".husky/pre-push: pre-push calls neither", "create .husky/pre-push"}},
		{Name: "husky script does not parse", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile("lefthook.yml"), contracttest.WithFile(".husky/pre-commit", "task fmt\n"), contracttest.WithFile(".husky/pre-push", "if task verify; then\n")},
			Findings: []string{".husky/pre-push: does not parse"}},
		{Name: "no hook manager", Want: "undecided", Edits: []contracttest.Edit{contracttest.WithoutFile("lefthook.yml")},
			Findings: []string{"lefthook.yml: ", "no recognized hook manager"}},
		{Name: "lefthook.yml does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("lefthook.yml", "pre-commit: [\n")},
			Findings: []string{"lefthook.yml: does not parse"}},
		{Name: "fmt entrypoints missing point at V-04", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n")},
			Findings: []string{"vibe.yaml: ", "V-04"}},
		{Name: "one of two entrypoints missing is enough", Want: "pass",
			Edits: []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n")}},
	})
}

// agentsSettings is a .claude/settings.json with the given hooks object body.
func agentsSettings(hooks string) contracttest.Edit {
	return contracttest.WithFile(".claude/settings.json", `{"hooks": {`+hooks+`}}`)
}

// agentsHookEntry is one hooks.<Event>[] entry: a matcher (omitted when "-")
// and one command hook.
func agentsHookEntry(matcher, command string) string {
	m := `"matcher": "` + matcher + `", `
	if matcher == "-" {
		m = ""
	}
	return `{` + m + `"hooks": [{"type": "command", "command": "` + command + `"}]}`
}

const (
	agentsSettingsFile = ".claude/settings.json"
	agentsSessionHook  = `"SessionStart": [{"hooks": [{"type": "command", "command": "task -x hook:context"}]}]`
	agentsPreHook      = `"PreToolUse": [{"matcher": "Bash|PowerShell|Write|Edit|MultiEdit", "hooks": [{"type": "command", "command": "task -x hook:guard"}]}]`
	agentsStopHook     = `"Stop": [{"hooks": [{"type": "command", "command": "task verify:fast"}]}]`
)

// agentsRunPartCases runs static audits for one part of a rule: each
// case's Want is the result of that part. A failing part fails the rule;
// a passing static part leaves it not run (its other parts did not run).
func agentsRunPartCases(t *testing.T, id string, kind contract.PartKind, cases []contracttest.RuleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline, tc.Edits...), contract.Options{Static: true})
			contracttest.AssertPart(t, rep, id, kind, tc.Want, tc.Findings...)
			if r := contracttest.RuleResult(t, rep, id); tc.Want == contract.ResultFail && r.Result != contract.ResultFail {
				t.Errorf("%s = %s, want fail", id, r.Result)
			}
		})
	}
}

// A-01: PreToolUse hooks match each of Bash, PowerShell, Write, Edit and
// MultiEdit. A matcher is missing, empty, * or an anchored regular
// expression.
func TestRuleA01(t *testing.T) {
	t.Parallel()
	pre := func(entries ...string) contracttest.Edit {
		return agentsSettings(`"PreToolUse": [` + strings.Join(entries, ", ") + `]`)
	}
	const guard = "task -x hook:guard"
	contracttest.RunRuleCases(t, "A-01", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "no matcher matches every tool", Want: "pass", Edits: []contracttest.Edit{pre(agentsHookEntry("-", guard))}},
		{Name: "empty matcher", Want: "pass", Edits: []contracttest.Edit{pre(agentsHookEntry("", guard))}},
		{Name: "star matcher", Want: "pass", Edits: []contracttest.Edit{pre(agentsHookEntry("*", guard))}},
		{Name: "a regular expression", Want: "pass", Edits: []contracttest.Edit{pre(agentsHookEntry("Bash|PowerShell|(Multi)?Edit|Wr.*", guard))}},
		{Name: "one entry per tool", Want: "pass", Edits: []contracttest.Edit{pre(
			agentsHookEntry("Bash", guard), agentsHookEntry("PowerShell", guard), agentsHookEntry("Write", guard),
			agentsHookEntry("Edit", guard), agentsHookEntry("MultiEdit", guard))}},
		{Name: "Write|Edit does not cover MultiEdit", Want: "fail",
			Edits: []contracttest.Edit{pre(agentsHookEntry("Bash|PowerShell|Write|Edit", guard))},
			Findings: []string{agentsSettingsFile + ": no PreToolUse command hook matches MultiEdit",
				"Fix: add MultiEdit to the matcher of the hook that runs the guard"}},
		{Name: "a prefix is not a match", Want: "fail",
			Edits:    []contracttest.Edit{pre(agentsHookEntry("Bas|PowerShel|Writ|Edit|MultiEdit", guard))},
			Findings: []string{"matches Bash, PowerShell and Write"}},
		{Name: "an entry without a command hook", Want: "fail",
			Edits:    []contracttest.Edit{pre(`{"matcher": "*", "hooks": [{"type": "prompt", "prompt": "is this safe?"}]}`)},
			Findings: []string{"matches Bash, PowerShell, Write, Edit and MultiEdit"}},
		{Name: "no PreToolUse hook", Want: "fail",
			Edits:    []contracttest.Edit{agentsSettings(agentsStopHook)},
			Findings: []string{agentsSettingsFile + ": no PreToolUse command hook matches"}},
		{Name: "settings.json missing", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile(agentsSettingsFile)},
			Findings: []string{agentsSettingsFile + ": ", "create " + agentsSettingsFile}},
		{Name: "settings.json untracked", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithoutFile(agentsSettingsFile), contracttest.WithUntracked(agentsSettingsFile, `{"hooks": {`+agentsPreHook+`}}`)}},
		{Name: "settings.local.json is never read", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithoutFile(agentsSettingsFile), contracttest.WithFile(".claude/settings.local.json", `{"hooks": {`+agentsPreHook+`}}`)},
			Findings: []string{agentsSettingsFile + ": "}},
		{Name: "settings.json does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(agentsSettingsFile, `{"hooks": `)},
			Findings: []string{agentsSettingsFile + ": does not parse"}},
		{Name: "hooks of the wrong shape", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(agentsSettingsFile, `{"hooks": {"PreToolUse": {"matcher": "*"}}}`)},
			Findings: []string{agentsSettingsFile + ": does not parse", "hooks.PreToolUse"}},
	})
}

// A-05 (static part): a Stop hook calls verify-fast.
func TestRuleA05Static(t *testing.T) {
	t.Parallel()
	agentsRunPartCases(t, "A-05", "static", []contracttest.RuleCase{
		{Name: "baseline, inside a list with redirection", Want: "pass"},
		{Name: "plain", Want: "pass", Edits: []contracttest.Edit{agentsSettings(agentsStopHook)}},
		{Name: "a wrapper task is not verify-fast", Want: "fail",
			Edits: []contracttest.Edit{agentsSettings(`"Stop": [` + agentsHookEntry("-", "task -x hook:done") + `]`)},
			Findings: []string{agentsSettingsFile + ": no Stop hook calls verify-fast (task verify:fast)",
				"Fix: make a Stop hook run `task verify:fast`"}},
		{Name: "sh -c is not parsed again", Want: "fail",
			Edits:    []contracttest.Edit{agentsSettings(`"Stop": [` + agentsHookEntry("-", `sh -c 'task verify:fast'`) + `]`)},
			Findings: []string{"no Stop hook calls verify-fast"}},
		{Name: "no Stop hook", Want: "fail",
			Edits:    []contracttest.Edit{agentsSettings(agentsPreHook)},
			Findings: []string{"no Stop hook calls verify-fast"}},
		{Name: "settings.json missing", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile(agentsSettingsFile)},
			Findings: []string{agentsSettingsFile + ": ", "create " + agentsSettingsFile}},
		{Name: "settings.json does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(agentsSettingsFile, `[`)},
			Findings: []string{agentsSettingsFile + ": does not parse"}},
		{Name: "verify-fast missing points at V-04", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  fmt: task fmt\n  fmt-check: task fmt:check\n")},
			Findings: []string{"vibe.yaml: ", "V-04"}},
	})
}

// A-06: a PostToolUse hook that matches Write and Edit calls fmt.
func TestRuleA06(t *testing.T) {
	t.Parallel()
	post := func(entries ...string) contracttest.Edit {
		return agentsSettings(`"PostToolUse": [` + strings.Join(entries, ", ") + `]`)
	}
	contracttest.RunRuleCases(t, "A-06", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "no matcher", Want: "pass", Edits: []contracttest.Edit{post(agentsHookEntry("-", "task fmt"))}},
		{Name: "Write|Edit", Want: "pass", Edits: []contracttest.Edit{post(agentsHookEntry("Write|Edit", "task fmt || true"))}},
		{Name: "a formatter that is not fmt", Want: "fail",
			Edits: []contracttest.Edit{post(agentsHookEntry("Write|Edit|MultiEdit", "task -x hook:format"))},
			Findings: []string{agentsSettingsFile + ": no PostToolUse hook that matches Write and Edit calls fmt (task fmt)",
				"Fix: add a PostToolUse hook with matcher Write|Edit that runs `task fmt`"}},
		{Name: "Write only", Want: "fail", Edits: []contracttest.Edit{post(agentsHookEntry("Write", "task fmt"))},
			Findings: []string{"no PostToolUse hook that matches Write and Edit"}},
		{Name: "Write and Edit in separate entries", Want: "fail",
			Edits:    []contracttest.Edit{post(agentsHookEntry("Write", "task fmt"), agentsHookEntry("Edit", "task fmt"))},
			Findings: []string{"no PostToolUse hook that matches Write and Edit"}},
		{Name: "no PostToolUse hook", Want: "fail", Edits: []contracttest.Edit{agentsSettings(agentsPreHook)},
			Findings: []string{"no PostToolUse hook"}},
		{Name: "settings.json missing", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(agentsSettingsFile)},
			Findings: []string{agentsSettingsFile + ": "}},
		{Name: "fmt missing points at V-04", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n")},
			Findings: []string{"vibe.yaml: ", "V-04"}},
	})
}

const (
	agentsPolicyFile      = ".claude/hooks/policy.json"
	agentsLocalPolicyFile = ".claude/hooks/policy.local.json"
)

// agentsPolicy is a guard policy.json with the given version and exceptions
// (JSON list body).
func agentsPolicy(version, exceptions string) contracttest.Edit {
	return contracttest.WithFile(agentsPolicyFile, `{"version": `+version+`, "rules": [], "exceptions": [`+exceptions+`]}`)
}

// A-04: policy.json is valid version 2; every exception names a guard
// rule ID and has a non-empty match and reason.
func TestRuleA04(t *testing.T) {
	t.Parallel()
	contracttest.RunRuleCases(t, "A-04", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "exceptions omitted", Want: "pass", Edits: []contracttest.Edit{contracttest.WithFile(agentsPolicyFile, `{"version": 2}`)}},
		{Name: "valid exceptions", Want: "pass", Edits: []contracttest.Edit{agentsPolicy("2",
			`{"rule": "git-push-force", "match": "^git push --force origin feat/", "reason": "rebasing feature branches"},
			 {"rule": "guardrail-settings", "match": "x", "reason": "y", "note": "other keys are the guard's"}`)}},
		{Name: "version 1", Want: "fail", Edits: []contracttest.Edit{agentsPolicy("1", "")},
			Findings: []string{agentsPolicyFile + ": has version 1; contract 1.2.0 needs version 2", "Fix: "}},
		{Name: "no version", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(agentsPolicyFile, `{"rules": []}`)},
			Findings: []string{agentsPolicyFile + ": has no version"}},
		{Name: "version is a string", Want: "fail", Edits: []contracttest.Edit{agentsPolicy(`"2"`, "")},
			Findings: []string{agentsPolicyFile + ": has version \"2\""}},
		{Name: "missing", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(agentsPolicyFile)},
			Findings: []string{agentsPolicyFile + ": ", "create " + agentsPolicyFile}},
		{Name: "untracked", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithoutFile(agentsPolicyFile), contracttest.WithUntracked(agentsPolicyFile, `{"version": 2}`)}},
		{Name: "does not parse", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(agentsPolicyFile, `{"version": 2,`)},
			Findings: []string{agentsPolicyFile + ": does not parse"}},
		{Name: "not an object", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(agentsPolicyFile, `[]`)},
			Findings: []string{agentsPolicyFile + ": does not parse", "object"}},
		{Name: "exceptions not a list", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(agentsPolicyFile, `{"version": 2, "exceptions": {}}`)},
			Findings: []string{agentsPolicyFile + ": exceptions is not a list"}},
		{Name: "exception not an object", Want: "fail", Edits: []contracttest.Edit{agentsPolicy("2", `"rm-rf"`)},
			Findings: []string{agentsPolicyFile + ": exceptions[0] is not an object"}},
		{Name: "unknown guard rule", Want: "fail", Edits: []contracttest.Edit{agentsPolicy("2",
			`{"rule": "git-reset-hard", "match": "a", "reason": "b"}, {"rule": "no-such-rule", "match": "a", "reason": "b"}`)},
			Findings: []string{agentsPolicyFile + `: exceptions[1].rule "no-such-rule" is not a guard rule ID`, "git-reset-hard"}},
		{Name: "empty match", Want: "fail", Edits: []contracttest.Edit{agentsPolicy("2", `{"rule": "rm-rf", "match": "", "reason": "b"}`)},
			Findings: []string{agentsPolicyFile + ": exceptions[0].match is empty"}},
		{Name: "blank reason", Want: "fail", Edits: []contracttest.Edit{agentsPolicy("2", `{"rule": "rm-rf", "match": "a", "reason": "  "}`)},
			Findings: []string{agentsPolicyFile + ": exceptions[0].reason is empty"}},
		{Name: "no reason", Want: "fail", Edits: []contracttest.Edit{agentsPolicy("2", `{"rule": "rm-rf", "match": "a"}`)},
			Findings: []string{"exceptions[0].reason is empty"}},
		{Name: "a policy.local.json is not judged", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithFile(agentsLocalPolicyFile, `{"version": 7, "exceptions": [{"rule": "x"}]}`)}},
		// Contract 1.0.1 (pilot P2): when the PreToolUse hooks run
		// vibe-guard, policy.json must be one vibe-guard loads. Another
		// guard's keys stay the guard's (the cases above).
		{Name: "vibe-guard cannot load it", Want: "fail",
			Edits: []contracttest.Edit{agentsVibeGuardSettings(), contracttest.WithFile(agentsPolicyFile, `{"version": 2, "tools": [], "rules": [], "exceptions": []}`)},
			Findings: []string{agentsPolicyFile + ": vibe-guard cannot load it: policy.json: tools is not a known key",
				"Fix: make " + agentsPolicyFile + " a policy vibe-guard loads"}},
		{Name: "vibe-guard cannot load an exception", Want: "fail",
			Edits:    []contracttest.Edit{agentsVibeGuardSettings(), agentsPolicy("2", `{"rule": "rm-rf", "match": "a", "reason": "b", "note": "c"}`)},
			Findings: []string{agentsPolicyFile + ": vibe-guard cannot load it: policy.json: exceptions[0].note is not a known key"}},
		{Name: "vibe-guard loads it", Want: "pass",
			Edits: []contracttest.Edit{agentsVibeGuardSettings(), contracttest.WithFile(agentsPolicyFile, `{"version": 2, "mode": "enforce", "rules": [], "exceptions": []}`)}},
		{Name: "another guard's keys are not read", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithFile(agentsPolicyFile, `{"version": 2, "tools": [], "rules": [], "exceptions": []}`)}},
		{Name: "vibe-guard and a missing policy", Want: "fail",
			Edits:    []contracttest.Edit{agentsVibeGuardSettings(), contracttest.WithoutFile(agentsPolicyFile)},
			Findings: []string{"create " + agentsPolicyFile}},
		// vibe-guard is found by the base name of the hook command's
		// first word, with .exe dropped.
		{Name: "vibe-guard by absolute path", Want: "fail",
			Edits:    []contracttest.Edit{agentsGuardHookSettings(`/usr/local/bin/vibe-guard hook`), agentsUnloadablePolicy()},
			Findings: []string{agentsPolicyFile + ": vibe-guard cannot load it"}},
		{Name: "vibe-guard.exe", Want: "fail",
			Edits:    []contracttest.Edit{agentsGuardHookSettings(`vibe-guard.exe hook`), agentsUnloadablePolicy()},
			Findings: []string{agentsPolicyFile + ": vibe-guard cannot load it"}},
		{Name: "vibe-guard by quoted Windows path", Want: "fail",
			Edits:    []contracttest.Edit{agentsGuardHookSettings(`'C:\\Program Files\\vibe\\vibe-guard.exe' hook`), agentsUnloadablePolicy()},
			Findings: []string{agentsPolicyFile + ": vibe-guard cannot load it"}},
		// A known limit: a wrapper that runs vibe-guard is not followed.
		{Name: "a wrapper is not followed", Want: "pass",
			Edits: []contracttest.Edit{agentsGuardHookSettings(`task -x hook:guard`), agentsUnloadablePolicy()}},
		{Name: "vibe-guard without hook is not the guard", Want: "pass",
			Edits: []contracttest.Edit{agentsGuardHookSettings(`vibe-guard explain x`), agentsUnloadablePolicy()}},
	})
}

// A policy with defects of its own is not also reported as one vibe-guard
// cannot load: version 1 is one finding, not two.
func TestRuleA04OneFindingPerDefect(t *testing.T) {
	t.Parallel()
	rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline,
		agentsVibeGuardSettings(), agentsPolicy("1", "")), contract.Options{Static: true})
	r := contracttest.RuleResult(t, rep, "A-04")
	if n := len(r.Findings()); r.Result != contract.ResultFail || n != 1 {
		t.Errorf("A-04 = %s with %d findings, want fail with 1\nfindings:\n%s", r.Result, n, contracttest.FindingText(r))
	}
}

// agentsVibeGuardSettings is the baseline settings with the reference
// PreToolUse command, which runs vibe-guard.
func agentsVibeGuardSettings() contracttest.Edit {
	return agentsGuardHookSettings(`command -v vibe-guard >/dev/null 2>&1 || exit 2; vibe-guard hook`)
}

// agentsGuardHookSettings is the baseline settings with command as the
// PreToolUse hook (a JSON string body).
func agentsGuardHookSettings(command string) contracttest.Edit {
	return agentsSettings(agentsSessionHook + `, "PreToolUse": [` +
		agentsHookEntry("Bash|PowerShell|Write|Edit|MultiEdit", command) +
		`], ` + agentsStopHook)
}

// agentsUnloadablePolicy is a policy.json the contract accepts and vibe-guard
// does not load.
func agentsUnloadablePolicy() contracttest.Edit {
	return contracttest.WithFile(agentsPolicyFile, `{"version": 2, "tools": [], "rules": [], "exceptions": []}`)
}

// agentsListings is the static report of the baseline with edits, whose
// GuardExceptions and LocalAdjustments list the guard's loosenings.
func agentsListings(t *testing.T, edits ...contracttest.Edit) *contract.Report {
	t.Helper()
	return contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline, edits...), contract.Options{Static: true})
}

// The report lists every guard exception and every local guardrail
// adjustment (spec 0052, "Guard policy", "Local guardrail adjustments").
func TestContractGuardListings(t *testing.T) {
	t.Parallel()
	t.Run("none", func(t *testing.T) {
		rep := agentsListings(t)
		if len(rep.GuardExceptions) != 0 || len(rep.LocalAdjustments) != 0 {
			t.Errorf("listings = %+v %+v, want none", rep.GuardExceptions, rep.LocalAdjustments)
		}
	})
	t.Run("every guard exception, in order", func(t *testing.T) {
		rep := agentsListings(t, agentsPolicy("2",
			`{"rule": "git-push-force", "match": "^git push --force origin feat/", "reason": "rebasing"},
			 {"rule": "secret-file", "match": "\\.env\\.example$", "reason": "a template"}`))
		got := fmt.Sprintf("%+v", rep.GuardExceptions)
		want := `[{Rule:git-push-force Match:^git push --force origin feat/ Reason:rebasing} {Rule:secret-file Match:\.env\.example$ Reason:a template}]`
		if got != want {
			t.Errorf("guard_exceptions = %s\nwant %s", got, want)
		}
	})
	t.Run("an invalid exception is listed too", func(t *testing.T) {
		rep := agentsListings(t, agentsPolicy("1", `{"rule": "nope", "match": "x"}`))
		if len(rep.GuardExceptions) != 1 || rep.GuardExceptions[0].Rule != "nope" || rep.GuardExceptions[0].Match != "x" {
			t.Errorf("guard_exceptions = %+v, want the one exception", rep.GuardExceptions)
		}
	})
	t.Run("policy.local.json exceptions and extra rules", func(t *testing.T) {
		rep := agentsListings(t, contracttest.WithFile(agentsLocalPolicyFile, `{
  "version": 1,
  "rules": [{"id": "no-curl", "field": "command", "pattern": "curl ", "message": "no network"}],
  "exceptions": [
    {"rule": "git-push-force", "pattern": "^git push --force origin feat/", "reason": "rebasing feature branches"},
    {"rule": "rm-rf", "pattern": "^rm -r build$"}
  ]
}`))
		got := fmt.Sprintf("%+v", rep.LocalAdjustments)
		want := "[{File:" + agentsLocalPolicyFile + " Kind:exception Rule:git-push-force Pattern:^git push --force origin feat/ Reason:rebasing feature branches}" +
			" {File:" + agentsLocalPolicyFile + " Kind:exception Rule:rm-rf Pattern:^rm -r build$ Reason:}" +
			" {File:" + agentsLocalPolicyFile + " Kind:rule Rule:no-curl Pattern:curl  Reason:}]"
		if got != want {
			t.Errorf("local_adjustments = %s\nwant %s", got, want)
		}
	})
	t.Run("an untracked policy.local.json is not listed", func(t *testing.T) {
		rep := agentsListings(t, contracttest.WithUntracked(agentsLocalPolicyFile,
			`{"version": 1, "exceptions": [{"rule": "rm-rf", "pattern": "x"}]}`))
		if len(rep.LocalAdjustments) != 0 {
			t.Errorf("local_adjustments = %+v, want none", rep.LocalAdjustments)
		}
	})
	t.Run("a policy.local.json that does not parse is noted", func(t *testing.T) {
		rep := agentsListings(t, contracttest.WithFile(agentsLocalPolicyFile, `{"version": 1,`))
		if !slices.ContainsFunc(rep.Notes, func(n string) bool {
			return strings.HasPrefix(n, agentsLocalPolicyFile+" does not parse")
		}) {
			t.Errorf("the report does not note the unreadable policy.local.json:\n%s", strings.Join(rep.Notes, "\n"))
		}
	})
}

const agentsDependabotFile = ".github/dependabot.yml"

// monorepo declares a Go component at apps/api, a TypeScript one at
// web and a Python one at tools/py.
func agentsMonorepo() []contracttest.Edit {
	return []contracttest.Edit{
		contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "components", Body: "components:\n" +
			"  - {path: apps/api, language: go}\n  - {path: web, language: typescript}\n  - {path: tools/py, language: python}\n"})),
		contracttest.WithFile("apps/api/go.mod", "module example.com/api\n"),
		contracttest.WithFile("web/package.json", "{}\n"),
		contracttest.WithFile("tools/py/pyproject.toml", "[project]\nname = \"py\"\n"),
	}
}

func agentsRenovate(file, content string) []contracttest.Edit {
	return []contracttest.Edit{contracttest.WithoutFile(agentsDependabotFile), contracttest.WithFile(file, content)}
}

const agentsAutomergeWorkflow = `name: automerge
on: pull_request
permissions: {}
jobs:
  merge:
    runs-on: ubuntu-latest
    steps:
      - run: gh pr merge --auto --squash "${{ github.event.pull_request.html_url }}"
`

// D-02: Dependabot or Renovate covers each component's ecosystem and
// github-actions, and enables no automerge.
func TestRuleD02(t *testing.T) {
	t.Parallel()
	contracttest.RunRuleCases(t, "D-02", []contracttest.RuleCase{
		{Name: "baseline dependabot", Want: "pass"},
		{Name: "dependabot.yaml", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithoutFile(agentsDependabotFile), contracttest.WithFile(".github/dependabot.yaml", "version: 2\nupdates:\n"+
				"  - {package-ecosystem: gomod, directory: /}\n  - {package-ecosystem: github-actions, directory: /}\n")}},
		{Name: "monorepo, directories and globs", Want: "pass", Edits: append(agentsMonorepo(), contracttest.Dependabot(
			"gomod directories [\"/apps/*\"]", "npm directory /web/", "uv directories [\"/**\"]", "github-actions directory /"))},
		{Name: "monorepo, a component left out", Want: "fail", Edits: append(agentsMonorepo(), contracttest.Dependabot(
			"gomod directories [\"/apps/*\"]", "pip directory /tools", "npm directory /web", "github-actions directory /")),
			Findings: []string{agentsDependabotFile + ": no updates entry for pip or uv covers component tools/py",
				"Fix: add an updates entry with package-ecosystem: pip and directory: /tools/py"}},
		{Name: "a glob in directory is not expanded", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Dependabot("gomod directory /*", "github-actions directory /")},
			Findings: []string{"no updates entry for gomod covers component ."}},
		{Name: "wrong ecosystem", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Dependabot("npm directory /", "github-actions directory /")},
			Findings: []string{agentsDependabotFile + ": no updates entry for gomod covers component .", "directory: /"}},
		{Name: "no github-actions", Want: "fail",
			Edits: []contracttest.Edit{contracttest.Dependabot("gomod directory /")},
			Findings: []string{agentsDependabotFile + ": no updates entry has package-ecosystem github-actions",
				"Fix: add an updates entry with package-ecosystem: github-actions and directory: /"}},
		{Name: "a workflow automerges", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithFile(".github/workflows/automerge.yml", agentsAutomergeWorkflow)},
			Findings: []string{".github/workflows/automerge.yml: job merge runs gh pr merge --auto",
				"Fix: remove --auto"}},
		{Name: "gh pr merge without --auto", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithFile(".github/workflows/automerge.yml", strings.ReplaceAll(agentsAutomergeWorkflow, "--auto ", ""))}},
		{Name: "no update bot", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(agentsDependabotFile)},
			Findings: []string{agentsDependabotFile + ": no update bot (Dependabot or Renovate)", "Fix: add " + agentsDependabotFile}},
		{Name: "untracked dependabot.yml", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithoutFile(agentsDependabotFile), contracttest.WithUntracked(agentsDependabotFile, "version: 2\nupdates: []\n")}},
		{Name: "dependabot.yml does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(agentsDependabotFile, "updates: [\n")},
			Findings: []string{agentsDependabotFile + ": does not parse"}},
		{Name: "updates is not a list", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(agentsDependabotFile, "version: 2\nupdates: {}\n")},
			Findings: []string{agentsDependabotFile + ": does not parse", "updates"}},

		{Name: "renovate.json", Want: "pass", Edits: agentsRenovate("renovate.json", `{"extends": ["config:recommended"]}`)},
		{Name: ".github/renovate.json", Want: "pass", Edits: agentsRenovate(".github/renovate.json", `{}`)},
		{Name: ".renovaterc", Want: "pass", Edits: agentsRenovate(".renovaterc", `{}`)},
		{Name: ".renovaterc.json", Want: "pass", Edits: agentsRenovate(".renovaterc.json", `{}`)},
		{Name: "renovate.json5 with comments and trailing commas", Want: "pass", Edits: agentsRenovate("renovate.json5",
			"{\n  // dependencies are reviewed\n  \"enabledManagers\": [\"gomod\", \"github-actions\",],\n}\n")},
		{Name: "renovate.json5 beyond JWCC cannot be decided", Want: "undecided", Edits: agentsRenovate(".github/renovate.json5",
			"{ enabledManagers: ['gomod'] }\n"),
			Findings: []string{".github/renovate.json5: "}},
		{Name: "renovate.json does not parse", Want: "fail", Edits: agentsRenovate("renovate.json", `{"extends": [`),
			Findings: []string{"renovate.json: does not parse"}},
		{Name: "package.json renovate key", Want: "pass", Edits: slices.Concat(
			agentsRenovate("package.json", `{"name": "x", "renovate": {"extends": ["config:recommended"]}}`),
			[]contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "components", Body: "components:\n  - {path: ., language: go}\n"}))})},
		{Name: "package.json without a renovate key", Want: "fail", Edits: slices.Concat(
			agentsRenovate("package.json", `{"name": "x"}`),
			[]contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "components", Body: "components:\n  - {path: ., language: go}\n"}))}),
			Findings: []string{"no update bot"}},
		{Name: "renovate enabledManagers covers the monorepo", Want: "pass", Edits: slices.Concat(agentsMonorepo(),
			agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "npm", "pep621", "github-actions"]}`))},
		{Name: "renovate enabledManagers leaves out a component", Want: "fail",
			Edits: agentsRenovate("renovate.json", `{"enabledManagers": ["npm", "github-actions"]}`),
			Findings: []string{"renovate.json: enabledManagers leaves out gomod, which component . needs",
				"Fix: add gomod to enabledManagers"}},
		{Name: "renovate enabledManagers leaves out github-actions", Want: "fail",
			Edits:    agentsRenovate("renovate.json", `{"enabledManagers": ["gomod"]}`),
			Findings: []string{"renovate.json: enabledManagers leaves out github-actions"}},
		{Name: "renovate automerge in a package rule", Want: "fail",
			Edits: agentsRenovate("renovate.json", `{"packageRules": [{"matchUpdateTypes": ["patch"], "automerge": true}]}`),
			Findings: []string{"renovate.json: enables automerge (packageRules[0].automerge)",
				"Fix: remove"}},
		{Name: "renovate automerge false", Want: "pass",
			Edits: agentsRenovate("renovate.json", `{"automerge": false}`)},
		{Name: "renovate automerge preset", Want: "fail",
			Edits:    agentsRenovate("renovate.json", `{"extends": ["config:recommended", ":automergeMinor"]}`),
			Findings: []string{`renovate.json: extends the automerge preset ":automergeMinor"`}},
		{Name: "renovate preset that turns automerge off", Want: "pass",
			Edits: agentsRenovate("renovate.json", `{"extends": ["config:recommended", ":automergeDisabled"]}`)},
		{Name: "renovate presets that only choose how automerge works", Want: "pass",
			Edits: agentsRenovate("renovate.json", `{"extends": [":automergeBranch", "default:automergePr", ":automergeRequireAllStatusChecks"]}`)},
		{Name: "renovate automerge off beside automerge true still fails", Want: "fail",
			Edits:    agentsRenovate("renovate.json", `{"extends": [":automergeDisabled"], "packageRules": [{"automerge": true}]}`),
			Findings: []string{"renovate.json: enables automerge (packageRules[0].automerge)"}},
		{Name: "renovate default: spelling of an enabling preset", Want: "fail",
			Edits:    agentsRenovate("renovate.json", `{"extends": ["default:automergeAll"]}`),
			Findings: []string{`renovate.json: extends the automerge preset "default:automergeAll"`}},
		{Name: "renovate third-party automerge preset counts", Want: "fail",
			Edits:    agentsRenovate("renovate.json", `{"extends": ["github>acme/renovate-config:automergeDisabled"]}`),
			Findings: []string{`extends the automerge preset "github>acme/renovate-config:automergeDisabled"`}},
	})
}

// agentsGLBase is the GitLab family's baseline pipeline (glBase), a copy
// for D-02's GitLab cases: verify on a Linux and a Windows runner, a scan
// and the audit in a job of its own.
const agentsGLBase = "default:\n  image: golang:1.25@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n\n" +
	"verify:linux:\n  tags: [linux]\n  script:\n    - task verify\n\n" +
	"verify:windows:\n  tags: [windows]\n  script:\n    - task verify\n\n" +
	"scan:\n  script:\n    - go install golang.org/x/vuln/cmd/govulncheck@v1.1.4\n    - govulncheck ./...\n\n" +
	"conformance:audit:\n  script:\n    - vibe audit --static\n"

// agentsGitlab turns the baseline into a GitLab-only repository whose
// .gitlab-ci.yml is pipeline, plus any further edits (the GitLab family's
// gitlab, a copy).
func agentsGitlab(pipeline string, more ...contracttest.Edit) []contracttest.Edit {
	return append([]contracttest.Edit{
		contracttest.WithoutFile(".github/workflows/ci.yml"),
		contracttest.WithoutFile(".github/workflows/audit.yml"),
		contracttest.WithFile(".gitlab-ci.yml", pipeline),
	}, more...)
}

// D-02 on GitLab CI (spec 0055, D-02; regression for pilot finding F-14):
// Renovate covers each component and gitlabci, and gitlabci-include when
// enabledManagers is set and the pipeline has includes it does not follow.
// Dependabot cannot satisfy D-02 there, and the fix text names Renovate.
func TestRuleD02GitLab(t *testing.T) {
	t.Parallel()
	const projectInclude = "include:\n  - project: acme/ci\n    ref: v1.2.0\n    file: /templates/go.yml\n\n"
	gl := func(more ...contracttest.Edit) []contracttest.Edit { return agentsGitlab(agentsGLBase, more...) }
	contracttest.RunRuleCases(t, "D-02", []contracttest.RuleCase{
		{Name: "renovate.json without enabledManagers", Want: "pass",
			Edits: gl(agentsRenovate("renovate.json", `{"extends": ["config:recommended"]}`)...)},
		{Name: "enabledManagers covers the component and gitlabci", Want: "pass",
			Edits: gl(agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`)...)},
		{Name: "enabledManagers covers the monorepo and gitlabci", Want: "pass",
			Edits: gl(slices.Concat(agentsMonorepo(), agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "npm", "pep621", "gitlabci"]}`))...)},
		{Name: "enabledManagers leaves out gitlabci", Want: "fail",
			Edits: gl(agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "github-actions"]}`)...),
			Findings: []string{"renovate.json: enabledManagers leaves out gitlabci",
				"Fix: add gitlabci to enabledManagers"}},
		{Name: "enabledManagers leaves out a component", Want: "fail",
			Edits:    gl(agentsRenovate("renovate.json", `{"enabledManagers": ["gitlabci"]}`)...),
			Findings: []string{"renovate.json: enabledManagers leaves out gomod, which component . needs"}},
		{Name: "dependabot only", Want: "fail", Edits: gl(),
			Findings: []string{agentsDependabotFile + ": Dependabot", "GitLab CI", "Fix: add renovate.json", "gitlabci"}},
		{Name: "dependabot does not cover what renovate leaves out", Want: "fail",
			Edits:    gl(contracttest.WithFile("renovate.json", `{"enabledManagers": ["gitlabci"]}`)),
			Findings: []string{"renovate.json: enabledManagers leaves out gomod"}},
		{Name: "dependabot beside a covering renovate", Want: "pass",
			Edits: gl(contracttest.WithFile("renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`))},
		{Name: "no update bot", Want: "fail", Edits: gl(contracttest.WithoutFile(agentsDependabotFile)),
			Findings: []string{"renovate.json: no update bot (Renovate) is configured", "Fix: add renovate.json", "gitlabci"}},
		{Name: ".gitlab/renovate.json", Want: "pass",
			Edits: gl(agentsRenovate(".gitlab/renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`)...)},
		{Name: ".gitlab/renovate.json5", Want: "pass",
			Edits: gl(agentsRenovate(".gitlab/renovate.json5", "{\n  // reviewed\n  \"enabledManagers\": [\"gomod\", \"gitlabci\",],\n}\n")...)},
		{Name: ".gitlab/renovate.json is judged", Want: "fail",
			Edits:    gl(agentsRenovate(".gitlab/renovate.json", `{"enabledManagers": ["gomod"]}`)...),
			Findings: []string{".gitlab/renovate.json: enabledManagers leaves out gitlabci"}},
		{Name: "includes and enabledManagers without gitlabci-include", Want: "fail",
			Edits: agentsGitlab(projectInclude+agentsGLBase, agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`)...),
			Findings: []string{"renovate.json: enabledManagers leaves out gitlabci-include", "line 2",
				"Fix: add gitlabci-include to enabledManagers"}},
		{Name: "includes and enabledManagers with gitlabci-include", Want: "pass",
			Edits: agentsGitlab(projectInclude+agentsGLBase, agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "gitlabci", "gitlabci-include"]}`)...)},
		{Name: "includes without enabledManagers", Want: "pass",
			Edits: agentsGitlab(projectInclude+agentsGLBase, agentsRenovate("renovate.json", `{}`)...)},
		{Name: "a local include needs no gitlabci-include", Want: "pass",
			Edits: agentsGitlab("include:\n  - local: ci/lint.yml\n\n"+agentsGLBase, slices.Concat(
				[]contracttest.Edit{contracttest.WithFile("ci/lint.yml", "lint:\n  script: echo lint\n")},
				agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`))...)},
		// gitlabci-include updates project: includes only.
		{Name: "component, template and remote includes need no gitlabci-include", Want: "pass",
			Edits: agentsGitlab("include:\n  - component: gitlab.com/acme/lint/lint@1.0.0\n  - template: Jobs/SAST.gitlab-ci.yml\n"+
				"  - remote: https://example.com/ci.yml\n\n"+agentsGLBase,
				agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`)...)},
		{Name: "a project include after other kinds is the one named", Want: "fail",
			Edits: agentsGitlab("include:\n  - template: Jobs/SAST.gitlab-ci.yml\n  - project: acme/ci\n    file: /go.yml\n\n"+agentsGLBase,
				agentsRenovate("renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`)...),
			Findings: []string{"enabledManagers leaves out gitlabci-include", "project include at .gitlab-ci.yml line 3"}},
	})
}

// D-02 on GitHub Actions does not read the GitLab Renovate location as
// anything but a Renovate location: it still needs github-actions.
func TestRuleD02GitLabRenovateOnGitHub(t *testing.T) {
	t.Parallel()
	contracttest.RunRuleCases(t, "D-02", []contracttest.RuleCase{
		{Name: ".gitlab/renovate.json on GitHub", Want: "pass", Edits: agentsRenovate(".gitlab/renovate.json", `{}`)},
		{Name: ".gitlab/renovate.json on GitHub needs github-actions", Want: "fail",
			Edits:    agentsRenovate(".gitlab/renovate.json", `{"enabledManagers": ["gomod", "gitlabci"]}`),
			Findings: []string{".gitlab/renovate.json: enabledManagers leaves out github-actions"}},
	})
}

// A-07 (static part): a SessionStart hook exists.
func TestRuleA07Static(t *testing.T) {
	t.Parallel()
	agentsRunPartCases(t, "A-07", "static", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "any matcher", Want: "pass", Edits: []contracttest.Edit{agentsSettings(agentsSessionHook)}},
		{Name: "no SessionStart hook", Want: "fail", Edits: []contracttest.Edit{agentsSettings(agentsPreHook + ", " + agentsStopHook)},
			Findings: []string{agentsSettingsFile + ": has no SessionStart command hook", "Fix: add a SessionStart hook"}},
		{Name: "a SessionStart entry with no command hook", Want: "fail",
			Edits:    []contracttest.Edit{agentsSettings(`"SessionStart": [{"hooks": []}]`)},
			Findings: []string{"has no SessionStart command hook"}},
		{Name: "settings.json missing", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(agentsSettingsFile)},
			Findings: []string{agentsSettingsFile + ": "}},
	})
}
