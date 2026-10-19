package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The rules that read hook managers, Claude Code settings, the guard
// policy and update bots (spec 0052, "Recognition rules"). Each case is
// the baseline plus one change.

// lefthookWith is a lefthook.yml whose pre-commit and pre-push hooks run
// one command each.
func lefthookWith(preCommit, prePush string) fixtureEdit {
	return withFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: "+preCommit+
		"\npre-push:\n  commands:\n    b:\n      run: "+prePush+"\n")
}

// preCommitConfig is a .pre-commit-config.yaml whose local hooks run
// task fmt:check at commit and task verify:fast at push.
const preCommitConfig = `repos:
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
	runRuleCases(t, "H-01", []ruleCase{
		{name: "baseline lefthook", want: "pass"},
		{name: ".lefthook.yml", want: "pass", edits: []fixtureEdit{
			withoutFile("lefthook.yml"),
			withFile(".lefthook.yml", "pre-push:\n  commands:\n    v:\n      run: task verify:fast\n")}},
		{name: "pre-commit", want: "pass", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".pre-commit-config.yaml", preCommitConfig)}},
		{name: "husky", want: "pass", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".husky/pre-push", "task verify:fast\n")}},
		{name: "no hook manager", want: "fail", edits: []fixtureEdit{withoutFile("lefthook.yml")},
			findings: []string{"lefthook.yml: no recognized hook manager (lefthook, pre-commit, husky)", "Fix: "}},
		{name: "untracked lefthook.yml", want: "fail", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withUntracked("lefthook.yml", "pre-push:\n  commands: {}\n")},
			findings: []string{"no recognized hook manager"}},
		{name: "lefthook-local.yml alone is personal", want: "fail", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile("lefthook-local.yml", "pre-push:\n  commands: {}\n")},
			findings: []string{"no recognized hook manager"}},
		{name: "lefthook.yml does not parse", want: "fail",
			edits:    []fixtureEdit{withFile("lefthook.yml", "pre-commit: [\n")},
			findings: []string{"lefthook.yml: does not parse"}},
		{name: ".pre-commit-config.yaml does not parse", want: "fail", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".pre-commit-config.yaml", "repos: {\n")},
			findings: []string{".pre-commit-config.yaml: does not parse"}},
	})
}

// H-02: the pre-commit hook calls fmt-check or fmt; the pre-push hook
// calls verify-fast or verify.
func TestRuleH02(t *testing.T) {
	t.Parallel()
	runRuleCases(t, "H-02", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "fmt and verify", want: "pass", edits: []fixtureEdit{lefthookWith("task fmt", "task verify")}},
		{name: "inside a list with redirection", want: "pass",
			edits: []fixtureEdit{lefthookWith(`"go vet ./... && task fmt:check >/dev/null"`, `"task verify:fast || exit 1"`)}},
		{name: "pre-push calls neither", want: "fail",
			edits: []fixtureEdit{lefthookWith("task fmt:check", "go test ./...")},
			findings: []string{"lefthook.yml: pre-push calls neither verify-fast (task verify:fast) nor verify (task verify)",
				"Fix: make pre-push run `task verify:fast`"}},
		{name: "pre-commit calls neither", want: "fail",
			edits: []fixtureEdit{lefthookWith("gofmt -l .", "task verify:fast")},
			findings: []string{"lefthook.yml: pre-commit calls neither fmt-check (task fmt:check) nor fmt (task fmt)",
				"Fix: make pre-commit run `task fmt:check`"}},
		{name: "an argument too many is not called", want: "fail",
			edits:    []fixtureEdit{lefthookWith("task fmt:check -v", "task verify:fast")},
			findings: []string{"pre-commit calls neither"}},
		{name: "no pre-push hook", want: "fail",
			edits:    []fixtureEdit{withFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\n")},
			findings: []string{"pre-push calls neither"}},
		{name: "lefthook jobs and nested groups", want: "pass", edits: []fixtureEdit{withFile("lefthook.yml", `pre-commit:
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
		{name: "lefthook scripts", want: "pass", edits: []fixtureEdit{
			withFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  scripts:\n    \"verify.sh\":\n      runner: bash\n"),
			withFile(".lefthook/pre-push/verify.sh", "#!/bin/bash\nset -e\ntask verify:fast\n")}},
		{name: "lefthook script in a job", want: "pass", edits: []fixtureEdit{
			withFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  jobs:\n    - script: verify.sh\n      runner: sh\n"),
			withFile(".lefthook/pre-push/verify.sh", "task verify:fast\n")}},
		{name: "untracked lefthook script calls nothing", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  scripts:\n    \"verify.sh\":\n      runner: bash\n"),
			withUntracked(".lefthook/pre-push/verify.sh", "task verify:fast\n")},
			findings: []string{"pre-push calls neither"}},
		{name: "lefthook extends a tracked file", want: "pass", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends:\n  - hooks/push.yml\npre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			withFile("hooks/push.yml", "pre-push:\n  commands:\n    v:\n      run: task verify\n")}},
		{name: "lefthook extends an untracked file", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends:\n  - hooks/push.yml\npre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			withUntracked("hooks/push.yml", "pre-push:\n  commands:\n    v:\n      run: task verify\n")},
			findings: []string{"pre-push calls neither"}},
		{name: "lefthook extends overrides a command", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends: [hooks/more.yml]\n"+
				"pre-commit:\n  commands:\n    a:\n      run: task fmt\npre-push:\n  commands:\n    v:\n      run: task verify\n"),
			withFile("hooks/more.yml", "pre-push:\n  commands:\n    v:\n      run: go test ./...\n")},
			findings: []string{"pre-push calls neither"}},
		{name: "extended file does not parse", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends: [hooks/more.yml]\npre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			withFile("hooks/more.yml", "pre-push: [\n")},
			findings: []string{"hooks/more.yml: does not parse"}},
		{name: "lefthook-local.yml is not read", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "pre-commit:\n  commands:\n    a:\n      run: task fmt\n"),
			withFile("lefthook-local.yml", "pre-push:\n  commands:\n    v:\n      run: task verify\n")},
			findings: []string{"pre-push calls neither"}},
		{name: "pre-commit local hooks", want: "pass", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".pre-commit-config.yaml", preCommitConfig)}},
		{name: "pre-commit default_stages and legacy stage names", want: "pass", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".pre-commit-config.yaml", `default_stages: [push]
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
		{name: "pre-commit remote hooks call nothing", want: "fail", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".pre-commit-config.yaml", `repos:
  - repo: https://example.com/hooks
    rev: v1.0.0
    hooks:
      - id: fmt
        entry: task fmt:check
`)},
			findings: []string{".pre-commit-config.yaml: pre-commit calls neither"}},
		{name: "pre-commit hook in the pre-commit stage only", want: "fail", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".pre-commit-config.yaml", `repos:
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
			findings: []string{".pre-commit-config.yaml: pre-push calls neither"}},
		{name: "husky", want: "pass", edits: []fixtureEdit{
			withoutFile("lefthook.yml"),
			withFile(".husky/pre-commit", "#!/usr/bin/env sh\nif ! task fmt:check; then\n  exit 1\nfi\n"),
			withFile(".husky/pre-push", "task verify:fast\n")}},
		{name: "husky without pre-push", want: "fail", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".husky/pre-commit", "task fmt\n")},
			findings: []string{".husky/pre-push: pre-push calls neither", "create .husky/pre-push"}},
		{name: "husky script does not parse", want: "fail", edits: []fixtureEdit{
			withoutFile("lefthook.yml"), withFile(".husky/pre-commit", "task fmt\n"), withFile(".husky/pre-push", "if task verify; then\n")},
			findings: []string{".husky/pre-push: does not parse"}},
		{name: "no hook manager", want: "undecided", edits: []fixtureEdit{withoutFile("lefthook.yml")},
			findings: []string{"lefthook.yml: ", "no recognized hook manager"}},
		{name: "lefthook.yml does not parse", want: "fail",
			edits:    []fixtureEdit{withFile("lefthook.yml", "pre-commit: [\n")},
			findings: []string{"lefthook.yml: does not parse"}},
		{name: "fmt entrypoints missing point at V-04", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n")},
			findings: []string{"vibe.yaml: ", "V-04"}},
		{name: "one of two entrypoints missing is enough", want: "pass",
			edits: []fixtureEdit{entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n")}},
	})
}

// settings is a .claude/settings.json with the given hooks object body.
func settings(hooks string) fixtureEdit {
	return withFile(".claude/settings.json", `{"hooks": {`+hooks+`}}`)
}

// hookEntry is one hooks.<Event>[] entry: a matcher (omitted when "-")
// and one command hook.
func hookEntry(matcher, command string) string {
	m := `"matcher": "` + matcher + `", `
	if matcher == "-" {
		m = ""
	}
	return `{` + m + `"hooks": [{"type": "command", "command": "` + command + `"}]}`
}

const (
	settingsFile    = ".claude/settings.json"
	baseSessionHook = `"SessionStart": [{"hooks": [{"type": "command", "command": "task -x hook:context"}]}]`
	basePreHook     = `"PreToolUse": [{"matcher": "Bash|PowerShell|Write|Edit|MultiEdit", "hooks": [{"type": "command", "command": "task -x hook:guard"}]}]`
	baseStopHook    = `"Stop": [{"hooks": [{"type": "command", "command": "task verify:fast"}]}]`
)

// partCase is ruleCase for a rule with several parts: want is the result
// of one part.
type partCase struct {
	name     string
	edits    []fixtureEdit
	want     string
	findings []string
}

// runPartCases runs --static cases for one part of a rule. A failing part
// fails the rule; a passing static part leaves it not run (its other
// parts did not run).
func runPartCases(t *testing.T, id, part string, cases []partCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep, _ := auditJSON(t, contractFixture(t, tc.edits...), "--static")
			r := rep.rule(t, id)
			got := ""
			for _, p := range r.Parts {
				if p.Check == part {
					got = p.Result
				}
			}
			if got != tc.want {
				t.Errorf("%s %s part = %q, want %s\nfindings:\n%s", id, part, got, tc.want, r.findingText())
			}
			if tc.want == "fail" && r.Result != "fail" {
				t.Errorf("%s = %s, want fail", id, r.Result)
			}
			text := r.findingText()
			for _, f := range tc.findings {
				if !strings.Contains(text, f) {
					t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
				}
			}
		})
	}
}

// A-01: PreToolUse hooks match each of Bash, PowerShell, Write, Edit and
// MultiEdit. A matcher is missing, empty, * or an anchored regular
// expression.
func TestRuleA01(t *testing.T) {
	t.Parallel()
	pre := func(entries ...string) fixtureEdit {
		return settings(`"PreToolUse": [` + strings.Join(entries, ", ") + `]`)
	}
	const guard = "task -x hook:guard"
	runRuleCases(t, "A-01", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "no matcher matches every tool", want: "pass", edits: []fixtureEdit{pre(hookEntry("-", guard))}},
		{name: "empty matcher", want: "pass", edits: []fixtureEdit{pre(hookEntry("", guard))}},
		{name: "star matcher", want: "pass", edits: []fixtureEdit{pre(hookEntry("*", guard))}},
		{name: "a regular expression", want: "pass", edits: []fixtureEdit{pre(hookEntry("Bash|PowerShell|(Multi)?Edit|Wr.*", guard))}},
		{name: "one entry per tool", want: "pass", edits: []fixtureEdit{pre(
			hookEntry("Bash", guard), hookEntry("PowerShell", guard), hookEntry("Write", guard),
			hookEntry("Edit", guard), hookEntry("MultiEdit", guard))}},
		{name: "Write|Edit does not cover MultiEdit", want: "fail",
			edits: []fixtureEdit{pre(hookEntry("Bash|PowerShell|Write|Edit", guard))},
			findings: []string{settingsFile + ": no PreToolUse command hook matches MultiEdit",
				"Fix: add MultiEdit to the matcher of the hook that runs the guard"}},
		{name: "a prefix is not a match", want: "fail",
			edits:    []fixtureEdit{pre(hookEntry("Bas|PowerShel|Writ|Edit|MultiEdit", guard))},
			findings: []string{"matches Bash, PowerShell and Write"}},
		{name: "an entry without a command hook", want: "fail",
			edits:    []fixtureEdit{pre(`{"matcher": "*", "hooks": [{"type": "prompt", "prompt": "is this safe?"}]}`)},
			findings: []string{"matches Bash, PowerShell, Write, Edit and MultiEdit"}},
		{name: "no PreToolUse hook", want: "fail",
			edits:    []fixtureEdit{settings(baseStopHook)},
			findings: []string{settingsFile + ": no PreToolUse command hook matches"}},
		{name: "settings.json missing", want: "fail",
			edits:    []fixtureEdit{withoutFile(settingsFile)},
			findings: []string{settingsFile + ": ", "create " + settingsFile}},
		{name: "settings.json untracked", want: "fail",
			edits: []fixtureEdit{withoutFile(settingsFile), withUntracked(settingsFile, `{"hooks": {`+basePreHook+`}}`)}},
		{name: "settings.local.json is never read", want: "fail", edits: []fixtureEdit{
			withoutFile(settingsFile), withFile(".claude/settings.local.json", `{"hooks": {`+basePreHook+`}}`)},
			findings: []string{settingsFile + ": "}},
		{name: "settings.json does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(settingsFile, `{"hooks": `)},
			findings: []string{settingsFile + ": does not parse"}},
		{name: "hooks of the wrong shape", want: "fail",
			edits:    []fixtureEdit{withFile(settingsFile, `{"hooks": {"PreToolUse": {"matcher": "*"}}}`)},
			findings: []string{settingsFile + ": does not parse", "hooks.PreToolUse"}},
	})
}

// A-05 (static part): a Stop hook calls verify-fast.
func TestRuleA05Static(t *testing.T) {
	t.Parallel()
	runPartCases(t, "A-05", "static", []partCase{
		{name: "baseline, inside a list with redirection", want: "pass"},
		{name: "plain", want: "pass", edits: []fixtureEdit{settings(baseStopHook)}},
		{name: "a wrapper task is not verify-fast", want: "fail",
			edits: []fixtureEdit{settings(`"Stop": [` + hookEntry("-", "task -x hook:done") + `]`)},
			findings: []string{settingsFile + ": no Stop hook calls verify-fast (task verify:fast)",
				"Fix: make a Stop hook run `task verify:fast`"}},
		{name: "sh -c is not parsed again", want: "fail",
			edits:    []fixtureEdit{settings(`"Stop": [` + hookEntry("-", `sh -c 'task verify:fast'`) + `]`)},
			findings: []string{"no Stop hook calls verify-fast"}},
		{name: "no Stop hook", want: "fail",
			edits:    []fixtureEdit{settings(basePreHook)},
			findings: []string{"no Stop hook calls verify-fast"}},
		{name: "settings.json missing", want: "fail",
			edits:    []fixtureEdit{withoutFile(settingsFile)},
			findings: []string{settingsFile + ": ", "create " + settingsFile}},
		{name: "settings.json does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(settingsFile, `[`)},
			findings: []string{settingsFile + ": does not parse"}},
		{name: "verify-fast missing points at V-04", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task verify\n  fmt: task fmt\n  fmt-check: task fmt:check\n")},
			findings: []string{"vibe.yaml: ", "V-04"}},
	})
}

// A-06: a PostToolUse hook that matches Write and Edit calls fmt.
func TestRuleA06(t *testing.T) {
	t.Parallel()
	post := func(entries ...string) fixtureEdit {
		return settings(`"PostToolUse": [` + strings.Join(entries, ", ") + `]`)
	}
	runRuleCases(t, "A-06", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "no matcher", want: "pass", edits: []fixtureEdit{post(hookEntry("-", "task fmt"))}},
		{name: "Write|Edit", want: "pass", edits: []fixtureEdit{post(hookEntry("Write|Edit", "task fmt || true"))}},
		{name: "a formatter that is not fmt", want: "fail",
			edits: []fixtureEdit{post(hookEntry("Write|Edit|MultiEdit", "task -x hook:format"))},
			findings: []string{settingsFile + ": no PostToolUse hook that matches Write and Edit calls fmt (task fmt)",
				"Fix: add a PostToolUse hook with matcher Write|Edit that runs `task fmt`"}},
		{name: "Write only", want: "fail", edits: []fixtureEdit{post(hookEntry("Write", "task fmt"))},
			findings: []string{"no PostToolUse hook that matches Write and Edit"}},
		{name: "Write and Edit in separate entries", want: "fail",
			edits:    []fixtureEdit{post(hookEntry("Write", "task fmt"), hookEntry("Edit", "task fmt"))},
			findings: []string{"no PostToolUse hook that matches Write and Edit"}},
		{name: "no PostToolUse hook", want: "fail", edits: []fixtureEdit{settings(basePreHook)},
			findings: []string{"no PostToolUse hook"}},
		{name: "settings.json missing", want: "fail", edits: []fixtureEdit{withoutFile(settingsFile)},
			findings: []string{settingsFile + ": "}},
		{name: "fmt missing points at V-04", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n")},
			findings: []string{"vibe.yaml: ", "V-04"}},
	})
}

const (
	policyFile      = ".claude/hooks/policy.json"
	localPolicyFile = ".claude/hooks/policy.local.json"
)

// policy is a guard policy.json with the given version and exceptions
// (JSON list body).
func policy(version, exceptions string) fixtureEdit {
	return withFile(policyFile, `{"version": `+version+`, "rules": [], "exceptions": [`+exceptions+`]}`)
}

// A-04: policy.json is valid version 2; every exception names a guard
// rule ID and has a non-empty match and reason.
func TestRuleA04(t *testing.T) {
	t.Parallel()
	runRuleCases(t, "A-04", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "exceptions omitted", want: "pass", edits: []fixtureEdit{withFile(policyFile, `{"version": 2}`)}},
		{name: "valid exceptions", want: "pass", edits: []fixtureEdit{policy("2",
			`{"rule": "git-push-force", "match": "^git push --force origin feat/", "reason": "rebasing feature branches"},
			 {"rule": "guardrail-settings", "match": "x", "reason": "y", "note": "other keys are the guard's"}`)}},
		{name: "version 1", want: "fail", edits: []fixtureEdit{policy("1", "")},
			findings: []string{policyFile + ": has version 1; contract 1.1.0 needs version 2", "Fix: "}},
		{name: "no version", want: "fail", edits: []fixtureEdit{withFile(policyFile, `{"rules": []}`)},
			findings: []string{policyFile + ": has no version"}},
		{name: "version is a string", want: "fail", edits: []fixtureEdit{policy(`"2"`, "")},
			findings: []string{policyFile + ": has version \"2\""}},
		{name: "missing", want: "fail", edits: []fixtureEdit{withoutFile(policyFile)},
			findings: []string{policyFile + ": ", "create " + policyFile}},
		{name: "untracked", want: "fail",
			edits: []fixtureEdit{withoutFile(policyFile), withUntracked(policyFile, `{"version": 2}`)}},
		{name: "does not parse", want: "fail", edits: []fixtureEdit{withFile(policyFile, `{"version": 2,`)},
			findings: []string{policyFile + ": does not parse"}},
		{name: "not an object", want: "fail", edits: []fixtureEdit{withFile(policyFile, `[]`)},
			findings: []string{policyFile + ": does not parse", "object"}},
		{name: "exceptions not a list", want: "fail", edits: []fixtureEdit{withFile(policyFile, `{"version": 2, "exceptions": {}}`)},
			findings: []string{policyFile + ": exceptions is not a list"}},
		{name: "exception not an object", want: "fail", edits: []fixtureEdit{policy("2", `"rm-rf"`)},
			findings: []string{policyFile + ": exceptions[0] is not an object"}},
		{name: "unknown guard rule", want: "fail", edits: []fixtureEdit{policy("2",
			`{"rule": "git-reset-hard", "match": "a", "reason": "b"}, {"rule": "no-such-rule", "match": "a", "reason": "b"}`)},
			findings: []string{policyFile + `: exceptions[1].rule "no-such-rule" is not a guard rule ID`, "git-reset-hard"}},
		{name: "empty match", want: "fail", edits: []fixtureEdit{policy("2", `{"rule": "rm-rf", "match": "", "reason": "b"}`)},
			findings: []string{policyFile + ": exceptions[0].match is empty"}},
		{name: "blank reason", want: "fail", edits: []fixtureEdit{policy("2", `{"rule": "rm-rf", "match": "a", "reason": "  "}`)},
			findings: []string{policyFile + ": exceptions[0].reason is empty"}},
		{name: "no reason", want: "fail", edits: []fixtureEdit{policy("2", `{"rule": "rm-rf", "match": "a"}`)},
			findings: []string{"exceptions[0].reason is empty"}},
		{name: "a policy.local.json is not judged", want: "pass",
			edits: []fixtureEdit{withFile(localPolicyFile, `{"version": 7, "exceptions": [{"rule": "x"}]}`)}},
		// Contract 1.0.1 (pilot P2): when the PreToolUse hooks run
		// vibe-guard, policy.json must be one vibe-guard loads. Another
		// guard's keys stay the guard's (the cases above).
		{name: "vibe-guard cannot load it", want: "fail",
			edits: []fixtureEdit{vibeGuardSettings(), withFile(policyFile, `{"version": 2, "tools": [], "rules": [], "exceptions": []}`)},
			findings: []string{policyFile + ": vibe-guard cannot load it: policy.json: tools is not a known key",
				"Fix: make " + policyFile + " a policy vibe-guard loads"}},
		{name: "vibe-guard cannot load an exception", want: "fail",
			edits:    []fixtureEdit{vibeGuardSettings(), policy("2", `{"rule": "rm-rf", "match": "a", "reason": "b", "note": "c"}`)},
			findings: []string{policyFile + ": vibe-guard cannot load it: policy.json: exceptions[0].note is not a known key"}},
		{name: "vibe-guard loads it", want: "pass",
			edits: []fixtureEdit{vibeGuardSettings(), withFile(policyFile, `{"version": 2, "mode": "enforce", "rules": [], "exceptions": []}`)}},
		{name: "another guard's keys are not read", want: "pass",
			edits: []fixtureEdit{withFile(policyFile, `{"version": 2, "tools": [], "rules": [], "exceptions": []}`)}},
		{name: "vibe-guard and a missing policy", want: "fail",
			edits:    []fixtureEdit{vibeGuardSettings(), withoutFile(policyFile)},
			findings: []string{"create " + policyFile}},
		// vibe-guard is found by the base name of the hook command's
		// first word, with .exe dropped.
		{name: "vibe-guard by absolute path", want: "fail",
			edits:    []fixtureEdit{guardHookSettings(`/usr/local/bin/vibe-guard hook`), unloadablePolicy()},
			findings: []string{policyFile + ": vibe-guard cannot load it"}},
		{name: "vibe-guard.exe", want: "fail",
			edits:    []fixtureEdit{guardHookSettings(`vibe-guard.exe hook`), unloadablePolicy()},
			findings: []string{policyFile + ": vibe-guard cannot load it"}},
		{name: "vibe-guard by quoted Windows path", want: "fail",
			edits:    []fixtureEdit{guardHookSettings(`'C:\\Program Files\\vibe\\vibe-guard.exe' hook`), unloadablePolicy()},
			findings: []string{policyFile + ": vibe-guard cannot load it"}},
		// A known limit: a wrapper that runs vibe-guard is not followed.
		{name: "a wrapper is not followed", want: "pass",
			edits: []fixtureEdit{guardHookSettings(`task -x hook:guard`), unloadablePolicy()}},
		{name: "vibe-guard without hook is not the guard", want: "pass",
			edits: []fixtureEdit{guardHookSettings(`vibe-guard explain x`), unloadablePolicy()}},
	})
}

// A policy with defects of its own is not also reported as one vibe-guard
// cannot load: version 1 is one finding, not two.
func TestRuleA04OneFindingPerDefect(t *testing.T) {
	t.Parallel()
	rep, _ := auditJSON(t, contractFixture(t, vibeGuardSettings(), policy("1", "")), "--static")
	r := rep.rule(t, "A-04")
	if r.Result != "fail" || len(r.Findings) != 1 {
		t.Errorf("A-04 = %s with %d findings, want fail with 1\nfindings:\n%s", r.Result, len(r.Findings), r.findingText())
	}
}

// vibeGuardSettings is the baseline settings with the reference
// PreToolUse command, which runs vibe-guard.
func vibeGuardSettings() fixtureEdit {
	return guardHookSettings(`command -v vibe-guard >/dev/null 2>&1 || exit 2; vibe-guard hook`)
}

// guardHookSettings is the baseline settings with command as the
// PreToolUse hook (a JSON string body).
func guardHookSettings(command string) fixtureEdit {
	return settings(baseSessionHook + `, "PreToolUse": [` +
		hookEntry("Bash|PowerShell|Write|Edit|MultiEdit", command) +
		`], ` + baseStopHook)
}

// unloadablePolicy is a policy.json the contract accepts and vibe-guard
// does not load.
func unloadablePolicy() fixtureEdit {
	return withFile(policyFile, `{"version": 2, "tools": [], "rules": [], "exceptions": []}`)
}

// listingsReport is the part of the --json report that lists the guard's
// loosenings.
type listingsReport struct {
	GuardExceptions []struct {
		Rule   string `json:"rule"`
		Match  string `json:"match"`
		Reason string `json:"reason"`
	} `json:"guard_exceptions"`
	LocalAdjustments []struct {
		File    string `json:"file"`
		Kind    string `json:"kind"`
		Rule    string `json:"rule"`
		Pattern string `json:"pattern"`
		Reason  string `json:"reason"`
	} `json:"local_adjustments"`
}

func auditListings(t *testing.T, edits ...fixtureEdit) listingsReport {
	t.Helper()
	r := runVibeAudit(t, "--repo-root", contractFixture(t, edits...), "--json", "--static")
	var rep listingsReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\n%s", err, r.stdout)
	}
	return rep
}

// The report lists every guard exception and every local guardrail
// adjustment (spec 0052, "Guard policy", "Local guardrail adjustments").
func TestContractGuardListings(t *testing.T) {
	t.Parallel()
	t.Run("none", func(t *testing.T) {
		rep := auditListings(t)
		if len(rep.GuardExceptions) != 0 || len(rep.LocalAdjustments) != 0 {
			t.Errorf("listings = %+v, want none", rep)
		}
	})
	t.Run("every guard exception, in order", func(t *testing.T) {
		rep := auditListings(t, policy("2",
			`{"rule": "git-push-force", "match": "^git push --force origin feat/", "reason": "rebasing"},
			 {"rule": "secret-file", "match": "\\.env\\.example$", "reason": "a template"}`))
		got := fmt.Sprintf("%+v", rep.GuardExceptions)
		want := `[{Rule:git-push-force Match:^git push --force origin feat/ Reason:rebasing} {Rule:secret-file Match:\.env\.example$ Reason:a template}]`
		if got != want {
			t.Errorf("guard_exceptions = %s\nwant %s", got, want)
		}
	})
	t.Run("an invalid exception is listed too", func(t *testing.T) {
		rep := auditListings(t, policy("1", `{"rule": "nope", "match": "x"}`))
		if len(rep.GuardExceptions) != 1 || rep.GuardExceptions[0].Rule != "nope" || rep.GuardExceptions[0].Match != "x" {
			t.Errorf("guard_exceptions = %+v, want the one exception", rep.GuardExceptions)
		}
	})
	t.Run("policy.local.json exceptions and extra rules", func(t *testing.T) {
		rep := auditListings(t, withFile(localPolicyFile, `{
  "version": 1,
  "rules": [{"id": "no-curl", "field": "command", "pattern": "curl ", "message": "no network"}],
  "exceptions": [
    {"rule": "git-push-force", "pattern": "^git push --force origin feat/", "reason": "rebasing feature branches"},
    {"rule": "rm-rf", "pattern": "^rm -r build$"}
  ]
}`))
		got := fmt.Sprintf("%+v", rep.LocalAdjustments)
		want := "[{File:" + localPolicyFile + " Kind:exception Rule:git-push-force Pattern:^git push --force origin feat/ Reason:rebasing feature branches}" +
			" {File:" + localPolicyFile + " Kind:exception Rule:rm-rf Pattern:^rm -r build$ Reason:}" +
			" {File:" + localPolicyFile + " Kind:rule Rule:no-curl Pattern:curl  Reason:}]"
		if got != want {
			t.Errorf("local_adjustments = %s\nwant %s", got, want)
		}
	})
	t.Run("an untracked policy.local.json is not listed", func(t *testing.T) {
		rep := auditListings(t, withUntracked(localPolicyFile,
			`{"version": 1, "exceptions": [{"rule": "rm-rf", "pattern": "x"}]}`))
		if len(rep.LocalAdjustments) != 0 {
			t.Errorf("local_adjustments = %+v, want none", rep.LocalAdjustments)
		}
	})
	t.Run("a policy.local.json that does not parse is noted", func(t *testing.T) {
		dir := contractFixture(t, withFile(localPolicyFile, `{"version": 1,`))
		r := runVibeAudit(t, "--repo-root", dir, "--static")
		if !strings.Contains(r.stdout, localPolicyFile+" does not parse") {
			t.Errorf("text report does not note the unreadable policy.local.json:\n%s", r.stdout)
		}
	})
}

const dependabotFile = ".github/dependabot.yml"

// dependabot is a dependabot.yml with the given updates entries, each
// "ecosystem directory-key directory-value".
func dependabot(entries ...string) fixtureEdit {
	var b strings.Builder
	b.WriteString("version: 2\nupdates:\n")
	for _, e := range entries {
		f := strings.SplitN(e, " ", 3)
		b.WriteString("  - package-ecosystem: " + f[0] + "\n    " + f[1] + ": " + f[2] + "\n    schedule:\n      interval: weekly\n")
	}
	return withFile(dependabotFile, b.String())
}

// monorepo declares a Go component at apps/api, a TypeScript one at
// web and a Python one at tools/py.
func monorepo() []fixtureEdit {
	return []fixtureEdit{
		withFile("vibe.yaml", configWith(section{"components", "components:\n" +
			"  - {path: apps/api, language: go}\n  - {path: web, language: typescript}\n  - {path: tools/py, language: python}\n"})),
		withFile("apps/api/go.mod", "module example.com/api\n"),
		withFile("web/package.json", "{}\n"),
		withFile("tools/py/pyproject.toml", "[project]\nname = \"py\"\n"),
	}
}

func renovate(file, content string) []fixtureEdit {
	return []fixtureEdit{withoutFile(dependabotFile), withFile(file, content)}
}

func edits(groups ...[]fixtureEdit) []fixtureEdit {
	var out []fixtureEdit
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

const automergeWorkflow = `name: automerge
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
	runRuleCases(t, "D-02", []ruleCase{
		{name: "baseline dependabot", want: "pass"},
		{name: "dependabot.yaml", want: "pass", edits: []fixtureEdit{
			withoutFile(dependabotFile), withFile(".github/dependabot.yaml", "version: 2\nupdates:\n"+
				"  - {package-ecosystem: gomod, directory: /}\n  - {package-ecosystem: github-actions, directory: /}\n")}},
		{name: "monorepo, directories and globs", want: "pass", edits: append(monorepo(), dependabot(
			"gomod directories [\"/apps/*\"]", "npm directory /web/", "uv directories [\"/**\"]", "github-actions directory /"))},
		{name: "monorepo, a component left out", want: "fail", edits: append(monorepo(), dependabot(
			"gomod directories [\"/apps/*\"]", "pip directory /tools", "npm directory /web", "github-actions directory /")),
			findings: []string{dependabotFile + ": no updates entry for pip or uv covers component tools/py",
				"Fix: add an updates entry with package-ecosystem: pip and directory: /tools/py"}},
		{name: "a glob in directory is not expanded", want: "fail",
			edits:    []fixtureEdit{dependabot("gomod directory /*", "github-actions directory /")},
			findings: []string{"no updates entry for gomod covers component ."}},
		{name: "wrong ecosystem", want: "fail",
			edits:    []fixtureEdit{dependabot("npm directory /", "github-actions directory /")},
			findings: []string{dependabotFile + ": no updates entry for gomod covers component .", "directory: /"}},
		{name: "no github-actions", want: "fail",
			edits: []fixtureEdit{dependabot("gomod directory /")},
			findings: []string{dependabotFile + ": no updates entry has package-ecosystem github-actions",
				"Fix: add an updates entry with package-ecosystem: github-actions and directory: /"}},
		{name: "a workflow automerges", want: "fail",
			edits: []fixtureEdit{withFile(".github/workflows/automerge.yml", automergeWorkflow)},
			findings: []string{".github/workflows/automerge.yml: job merge runs gh pr merge --auto",
				"Fix: remove --auto"}},
		{name: "gh pr merge without --auto", want: "pass",
			edits: []fixtureEdit{withFile(".github/workflows/automerge.yml", strings.ReplaceAll(automergeWorkflow, "--auto ", ""))}},
		{name: "no update bot", want: "fail", edits: []fixtureEdit{withoutFile(dependabotFile)},
			findings: []string{dependabotFile + ": no update bot (Dependabot or Renovate)", "Fix: add " + dependabotFile}},
		{name: "untracked dependabot.yml", want: "fail",
			edits: []fixtureEdit{withoutFile(dependabotFile), withUntracked(dependabotFile, "version: 2\nupdates: []\n")}},
		{name: "dependabot.yml does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(dependabotFile, "updates: [\n")},
			findings: []string{dependabotFile + ": does not parse"}},
		{name: "updates is not a list", want: "fail",
			edits:    []fixtureEdit{withFile(dependabotFile, "version: 2\nupdates: {}\n")},
			findings: []string{dependabotFile + ": does not parse", "updates"}},

		{name: "renovate.json", want: "pass", edits: renovate("renovate.json", `{"extends": ["config:recommended"]}`)},
		{name: ".github/renovate.json", want: "pass", edits: renovate(".github/renovate.json", `{}`)},
		{name: ".renovaterc", want: "pass", edits: renovate(".renovaterc", `{}`)},
		{name: ".renovaterc.json", want: "pass", edits: renovate(".renovaterc.json", `{}`)},
		{name: "renovate.json5 with comments and trailing commas", want: "pass", edits: renovate("renovate.json5",
			"{\n  // dependencies are reviewed\n  \"enabledManagers\": [\"gomod\", \"github-actions\",],\n}\n")},
		{name: "renovate.json5 beyond JWCC cannot be decided", want: "undecided", edits: renovate(".github/renovate.json5",
			"{ enabledManagers: ['gomod'] }\n"),
			findings: []string{".github/renovate.json5: "}},
		{name: "renovate.json does not parse", want: "fail", edits: renovate("renovate.json", `{"extends": [`),
			findings: []string{"renovate.json: does not parse"}},
		{name: "package.json renovate key", want: "pass", edits: edits(
			renovate("package.json", `{"name": "x", "renovate": {"extends": ["config:recommended"]}}`),
			[]fixtureEdit{withFile("vibe.yaml", configWith(section{"components", "components:\n  - {path: ., language: go}\n"}))})},
		{name: "package.json without a renovate key", want: "fail", edits: edits(
			renovate("package.json", `{"name": "x"}`),
			[]fixtureEdit{withFile("vibe.yaml", configWith(section{"components", "components:\n  - {path: ., language: go}\n"}))}),
			findings: []string{"no update bot"}},
		{name: "renovate enabledManagers covers the monorepo", want: "pass", edits: edits(monorepo(),
			renovate("renovate.json", `{"enabledManagers": ["gomod", "npm", "pep621", "github-actions"]}`))},
		{name: "renovate enabledManagers leaves out a component", want: "fail",
			edits: renovate("renovate.json", `{"enabledManagers": ["npm", "github-actions"]}`),
			findings: []string{"renovate.json: enabledManagers leaves out gomod, which component . needs",
				"Fix: add gomod to enabledManagers"}},
		{name: "renovate enabledManagers leaves out github-actions", want: "fail",
			edits:    renovate("renovate.json", `{"enabledManagers": ["gomod"]}`),
			findings: []string{"renovate.json: enabledManagers leaves out github-actions"}},
		{name: "renovate automerge in a package rule", want: "fail",
			edits: renovate("renovate.json", `{"packageRules": [{"matchUpdateTypes": ["patch"], "automerge": true}]}`),
			findings: []string{"renovate.json: enables automerge (packageRules[0].automerge)",
				"Fix: remove"}},
		{name: "renovate automerge false", want: "pass",
			edits: renovate("renovate.json", `{"automerge": false}`)},
		{name: "renovate automerge preset", want: "fail",
			edits:    renovate("renovate.json", `{"extends": ["config:recommended", ":automergeMinor"]}`),
			findings: []string{`renovate.json: extends the automerge preset ":automergeMinor"`}},
		{name: "renovate preset that turns automerge off", want: "pass",
			edits: renovate("renovate.json", `{"extends": ["config:recommended", ":automergeDisabled"]}`)},
		{name: "renovate presets that only choose how automerge works", want: "pass",
			edits: renovate("renovate.json", `{"extends": [":automergeBranch", "default:automergePr", ":automergeRequireAllStatusChecks"]}`)},
		{name: "renovate automerge off beside automerge true still fails", want: "fail",
			edits:    renovate("renovate.json", `{"extends": [":automergeDisabled"], "packageRules": [{"automerge": true}]}`),
			findings: []string{"renovate.json: enables automerge (packageRules[0].automerge)"}},
		{name: "renovate default: spelling of an enabling preset", want: "fail",
			edits:    renovate("renovate.json", `{"extends": ["default:automergeAll"]}`),
			findings: []string{`renovate.json: extends the automerge preset "default:automergeAll"`}},
		{name: "renovate third-party automerge preset counts", want: "fail",
			edits:    renovate("renovate.json", `{"extends": ["github>acme/renovate-config:automergeDisabled"]}`),
			findings: []string{`extends the automerge preset "github>acme/renovate-config:automergeDisabled"`}},
	})
}

// A-07 (static part): a SessionStart hook exists.
func TestRuleA07Static(t *testing.T) {
	t.Parallel()
	runPartCases(t, "A-07", "static", []partCase{
		{name: "baseline", want: "pass"},
		{name: "any matcher", want: "pass", edits: []fixtureEdit{settings(baseSessionHook)}},
		{name: "no SessionStart hook", want: "fail", edits: []fixtureEdit{settings(basePreHook + ", " + baseStopHook)},
			findings: []string{settingsFile + ": has no SessionStart command hook", "Fix: add a SessionStart hook"}},
		{name: "a SessionStart entry with no command hook", want: "fail",
			edits:    []fixtureEdit{settings(`"SessionStart": [{"hooks": []}]`)},
			findings: []string{"has no SessionStart command hook"}},
		{name: "settings.json missing", want: "fail", edits: []fixtureEdit{withoutFile(settingsFile)},
			findings: []string{settingsFile + ": "}},
	})
}
