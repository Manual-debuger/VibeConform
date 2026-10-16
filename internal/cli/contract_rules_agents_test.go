package cli

import "testing"

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
