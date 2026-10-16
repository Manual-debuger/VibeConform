package cli

import (
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

// A-07 (static part): a SessionStart hook exists.
func TestRuleA07Static(t *testing.T) {
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
