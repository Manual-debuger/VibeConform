package cli

import (
	"strings"
	"testing"
)

// The CI rules (C-01 to C-05), D-03 and R-01's static part, through the
// vibe audit seam (spec 0052, "Recognition rules", GitHub workflows).
// The baseline has .github/workflows/ci.yml (verify on a Linux and
// Windows matrix, a govulncheck scan) and audit.yml (vibe audit in its
// own job); each case changes one thing.

const (
	ciFile    = ".github/workflows/ci.yml"
	auditFile = ".github/workflows/audit.yml"
	// ciPinned is a pinned checkout, as the baseline uses it.
	ciPinned = "actions/checkout@08eba0b27e820071cde6df949e0beb9ba4906955"
)

// ciWorkflow is a workflow with the baseline's permissions, the given on:
// block (written out, "on: ..." included) and jobs.
func ciWorkflow(on, jobs string) string {
	return on + "permissions:\n  contents: read\njobs:\n" + jobs
}

// ciWith replaces ci.yml with a workflow of the given on: block and jobs.
func ciWith(on, jobs string) fixtureEdit {
	return withFile(ciFile, ciWorkflow(on, jobs))
}

// ciPRAndPush is the baseline's on: block.
const ciPRAndPush = "on:\n  pull_request:\n  push:\n    branches: [main]\n"

// ciJob is a job on runner (a runs-on value) with a pinned checkout and
// one run: step per script.
func ciJob(id, runner string, runs ...string) string {
	s := "  " + id + ":\n    runs-on: " + runner + "\n    steps:\n      - uses: " + ciPinned + "\n"
	for _, r := range runs {
		s += "      - run: " + r + "\n"
	}
	return s
}

// ciVerifyJob is a job on runner that runs task verify.
func ciVerifyJob(id, runner string) string { return ciJob(id, runner, "task verify") }

// C-03: every workflow's top-level permissions grant no write scope.
func TestRuleC03(t *testing.T) {
	t.Parallel()
	jobs := ciVerifyJob("v", "ubuntu-latest")
	perms := func(p string) fixtureEdit {
		return withFile(".github/workflows/extra.yml", "on: push\n"+p+"jobs:\n"+jobs)
	}
	runRuleCases(t, "C-03", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "read-all", want: "pass", edits: []fixtureEdit{perms("permissions: read-all\n")}},
		{name: "empty map", want: "pass", edits: []fixtureEdit{perms("permissions: {}\n")}},
		{name: "read and none", want: "pass",
			edits: []fixtureEdit{perms("permissions:\n  contents: read\n  issues: none\n")}},
		{name: "a job widens them", want: "pass", edits: []fixtureEdit{withFile(".github/workflows/extra.yml",
			"on: push\npermissions:\n  contents: read\n"+
				"jobs:\n  w:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n")}},
		{name: "missing", want: "fail", edits: []fixtureEdit{perms("")},
			findings: []string{".github/workflows/extra.yml: ", "no top-level permissions", "contents: read"}},
		{name: "a write scope", want: "fail",
			edits:    []fixtureEdit{perms("permissions:\n  contents: read\n  security-events: write\n")},
			findings: []string{".github/workflows/extra.yml: ", "security-events: write", "job"}},
		{name: "write-all", want: "fail", edits: []fixtureEdit{perms("permissions: write-all\n")},
			findings: []string{".github/workflows/extra.yml: ", "write-all"}},
		{name: "a workflow that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(".github/workflows/broken.yml", "on: [push\n")},
			findings: []string{".github/workflows/broken.yml: ", "does not parse"}},
	})
}

// listVerify declares verify as a list of two commands.
var listVerify = entrypoints("  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")

// noVerify declares every entrypoint but verify.
var noVerify = entrypoints("  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")

// C-01: workflows triggered by pull requests and by pushes to the default
// branch call verify. These fixtures have no origin, so the default
// branch is main, or master when only master is admitted.
func TestRuleC01(t *testing.T) {
	t.Parallel()
	linux := ciVerifyJob("v", "ubuntu-latest")
	onPush := func(filters string) string { return "on:\n  pull_request:\n  push:\n" + filters }
	onPR := func(filters string) string { return "on:\n  push:\n    branches: [main]\n  pull_request:\n" + filters }
	runRuleCases(t, "C-01", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "on as a list", want: "pass", edits: []fixtureEdit{ciWith("on: [pull_request, push]\n", linux)}},
		{name: "on as one event per workflow", want: "pass", edits: []fixtureEdit{
			ciWith("on: pull_request\n", linux),
			withFile(".github/workflows/push.yml", ciWorkflow("on: push\n", linux))}},
		{name: "list verify, commands in different jobs", want: "pass", edits: []fixtureEdit{listVerify,
			ciWith(ciPRAndPush, ciJob("lint", "ubuntu-latest", "task lint")+ciJob("test", "ubuntu-latest", "go mod download && task test"))}},
		{name: "list verify, one command not called", want: "fail", edits: []fixtureEdit{listVerify,
			ciWith(ciPRAndPush, ciJob("lint", "ubuntu-latest", "task lint"))},
			findings: []string{".github/workflows/: ", "`task test`", "pull requests", "pushes to main"}},
		{name: "verify inside a condition", want: "pass", edits: []fixtureEdit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "|\n          if true; then task verify; fi"))}},
		{name: "verify with an extra argument is not called", want: "fail", edits: []fixtureEdit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "task verify --force"))}},
		{name: "a step that does not parse calls nothing", want: "fail", edits: []fixtureEdit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "task verify 'unclosed"))}},
		{name: "a GitHub expression is not a word", want: "fail", edits: []fixtureEdit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "task ${{ matrix.task }}"))}},
		{name: "no push", want: "fail", edits: []fixtureEdit{ciWith("on: pull_request\n", linux)},
			findings: []string{".github/workflows/: ", "pushes to main"}},
		{name: "no pull request", want: "fail", edits: []fixtureEdit{ciWith("on: push\n", linux)},
			findings: []string{".github/workflows/: ", "pull requests"}},
		{name: "push to another branch", want: "fail", edits: []fixtureEdit{ciWith(onPush("    branches: [develop]\n"), linux)},
			findings: []string{".github/workflows/: ", "pushes to main"}},
		{name: "push branches double star", want: "pass", edits: []fixtureEdit{ciWith(onPush("    branches: ['**']\n"), linux)}},
		{name: "push branches single star", want: "pass", edits: []fixtureEdit{ciWith(onPush("    branches: ['ma*']\n"), linux)}},
		{name: "push branches negated", want: "fail", edits: []fixtureEdit{ciWith(onPush("    branches: ['*', '!main']\n"), linux)}},
		{name: "push branches negated then included again", want: "pass",
			edits: []fixtureEdit{ciWith(onPush("    branches: ['*', '!ma*', 'main']\n"), linux)}},
		{name: "push branches as one string", want: "pass", edits: []fixtureEdit{ciWith(onPush("    branches: main\n"), linux)}},
		{name: "push branches-ignore others", want: "pass",
			edits: []fixtureEdit{ciWith(onPush("    branches-ignore: ['release/**']\n"), linux)}},
		{name: "push branches-ignore main", want: "fail", edits: []fixtureEdit{ciWith(onPush("    branches-ignore: [main]\n"), linux)}},
		{name: "push tags without branches", want: "fail", edits: []fixtureEdit{ciWith(onPush("    tags: ['v*']\n"), linux)}},
		{name: "push tags with branches", want: "pass",
			edits: []fixtureEdit{ciWith(onPush("    branches: [main]\n    tags: ['v*']\n"), linux)}},
		{name: "push paths filter", want: "fail",
			edits: []fixtureEdit{ciWith(onPush("    branches: [main]\n    paths: ['**.go']\n"), linux)}},
		{name: "pull request paths-ignore filter", want: "fail",
			edits:    []fixtureEdit{ciWith(onPR("    paths-ignore: ['docs/**']\n"), linux)},
			findings: []string{"pull requests"}},
		{name: "pull request branches admit main", want: "pass", edits: []fixtureEdit{ciWith(onPR("    branches: [main]\n"), linux)}},
		{name: "pull request branches exclude main", want: "fail", edits: []fixtureEdit{ciWith(onPR("    branches: [develop]\n"), linux)}},
		{name: "pull request types keep the defaults", want: "pass",
			edits: []fixtureEdit{ciWith(onPR("    types: [opened, synchronize, reopened, labeled]\n"), linux)}},
		{name: "pull request types drop synchronize", want: "fail",
			edits: []fixtureEdit{ciWith(onPR("    types: [opened, reopened]\n"), linux)}},
		{name: "pull_request_target does not count", want: "fail",
			edits: []fixtureEdit{ciWith("on:\n  pull_request_target:\n  push:\n    branches: [main]\n", linux)}},
		{name: "job if: conditions are ignored", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush,
			"  v:\n    if: false\n    runs-on: ubuntu-latest\n    steps:\n      - run: task verify\n")}},
		{name: "local reusable workflow", want: "pass", edits: []fixtureEdit{
			ciWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/reusable.yml\n"),
			withFile(".github/workflows/reusable.yml", ciWorkflow("on: workflow_call\n", linux))}},
		{name: "local reusable workflow, nested", want: "pass", edits: []fixtureEdit{
			ciWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/outer.yml\n"),
			withFile(".github/workflows/outer.yml", ciWorkflow("on: workflow_call\n", "  inner:\n    uses: ./.github/workflows/inner.yml\n")),
			withFile(".github/workflows/inner.yml", ciWorkflow("on: workflow_call\n", linux))}},
		{name: "a reusable workflow's own triggers do not count", want: "fail", edits: []fixtureEdit{
			ciWith("on: workflow_dispatch\n", "  call:\n    uses: ./.github/workflows/reusable.yml\n"),
			withFile(".github/workflows/reusable.yml", ciWorkflow("on: workflow_call\n", linux))}},
		{name: "remote reusable workflow is not followed", want: "fail", edits: []fixtureEdit{
			ciWith(ciPRAndPush, "  call:\n    uses: octo/ci/.github/workflows/verify.yml@08eba0b27e820071cde6df949e0beb9ba4906955\n")}},
		{name: "master is assumed when the filters favor it", want: "pass",
			edits: []fixtureEdit{ciWith(onPush("    branches: [master]\n"), linux), withoutFile(auditFile)}},
		{name: "main is assumed on a tie", want: "fail",
			edits:    []fixtureEdit{ciWith(onPush("    branches: [master]\n"), linux)},
			findings: []string{"pushes to main"}},
		{name: "verify missing points at V-04", want: "fail",
			edits:    []fixtureEdit{noVerify},
			findings: []string{"vibe.yaml: ", "V-04"}},
		{name: "a workflow that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(".github/workflows/broken.yml", "jobs: [\n")},
			findings: []string{".github/workflows/broken.yml: ", "does not parse"}},
		{name: "a workflow whose jobs are not a map", want: "fail",
			edits:    []fixtureEdit{withFile(".github/workflows/broken.yml", "on: push\njobs: [a, b]\n")},
			findings: []string{".github/workflows/broken.yml: ", "jobs must be a map"}},
		{name: "no CI", want: "fail",
			edits:    []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile)},
			findings: []string{".github/workflows/: ", "no CI configuration"}},
		{name: "an untracked workflow does not count", want: "fail",
			edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile),
				withUntracked(ciFile, ciWorkflow(ciPRAndPush, linux))},
			findings: []string{"no CI configuration"}},
		{name: "another CI provider", want: "undecided",
			edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile),
				withFile("azure-pipelines.yml", "steps:\n  - script: task verify\n")},
			findings: []string{"azure-pipelines.yml: ", "GitHub Actions"}},
		{name: "another CI provider beside GitHub workflows", want: "pass",
			edits: []fixtureEdit{withFile("azure-pipelines.yml", "steps:\n  - script: task verify\n")}},
	})
}

// The default branch comes from refs/remotes/origin/HEAD when the clone
// has one; otherwise the report says which branch it assumed.
func TestRuleC01DefaultBranch(t *testing.T) {
	t.Parallel()
	onPush := func(branch string) fixtureEdit {
		return ciWith("on:\n  pull_request:\n  push:\n    branches: ["+branch+"]\n", ciVerifyJob("v", "ubuntu-latest"))
	}
	withOrigin := func(t *testing.T, branch string, edits ...fixtureEdit) string {
		t.Helper()
		dir := contractFixture(t, edits...)
		fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+branch)
		return dir
	}
	t.Run("origin/HEAD names trunk, push to trunk", func(t *testing.T) {
		rep, _ := auditJSON(t, withOrigin(t, "trunk", onPush("trunk")), "--static")
		assertRule(t, rep, "C-01", "pass")
	})
	t.Run("origin/HEAD names trunk, push to main", func(t *testing.T) {
		rep, _ := auditJSON(t, withOrigin(t, "trunk", onPush("main")), "--static")
		assertRule(t, rep, "C-01", "fail", "pushes to trunk")
	})
	t.Run("origin/HEAD names master, nothing assumed", func(t *testing.T) {
		r := runVibeAudit(t, "--repo-root", withOrigin(t, "master", onPush("master")), "--static")
		if strings.Contains(r.stdout, "assume") {
			t.Errorf("report notes an assumed branch although origin/HEAD names it:\n%s", r.stdout)
		}
	})
	t.Run("no origin/HEAD, the report says main was assumed", func(t *testing.T) {
		r := runVibeAudit(t, "--repo-root", contractFixture(t), "--static")
		if !strings.Contains(r.stdout, "assume the default branch is main") {
			t.Errorf("report does not say main was assumed:\n%s", r.stdout)
		}
	})
	t.Run("no origin/HEAD, the report says master was assumed", func(t *testing.T) {
		r := runVibeAudit(t, "--repo-root", contractFixture(t, onPush("master"), withoutFile(auditFile)), "--static")
		if !strings.Contains(r.stdout, "assume the default branch is master") {
			t.Errorf("report does not say master was assumed:\n%s", r.stdout)
		}
	})
}

// C-02: every uses: but a local path is pinned to a 40-character commit
// SHA, and no run: step has a word ending in @latest. Every workflow
// counts, whatever its triggers.
func TestRuleC02(t *testing.T) {
	t.Parallel()
	extra := func(jobs string) fixtureEdit {
		return withFile(".github/workflows/extra.yml", ciWorkflow("on: workflow_dispatch\n", jobs))
	}
	step := func(uses string) fixtureEdit {
		return extra("  x:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n")
	}
	run := func(script string) fixtureEdit { return extra(ciJob("x", "ubuntu-latest", script)) }
	const sha = "@08eba0b27e820071cde6df949e0beb9ba4906955"
	runRuleCases(t, "C-02", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "local action", want: "pass", edits: []fixtureEdit{step("./.github/actions/setup")}},
		{name: "action in a subdirectory, pinned", want: "pass", edits: []fixtureEdit{step("github/codeql-action/init" + sha)}},
		{name: "upper-case SHA", want: "pass", edits: []fixtureEdit{step("actions/setup-go@08EBA0B27E820071CDE6DF949E0BEB9BA4906955")}},
		{name: "local reusable workflow", want: "pass",
			edits: []fixtureEdit{extra("  call:\n    uses: ./.github/workflows/ci.yml\n")}},
		{name: "a version tag", want: "fail", edits: []fixtureEdit{step("actions/setup-go@v5")},
			findings: []string{".github/workflows/extra.yml: ", "actions/setup-go@v5", "line ", "40-character"}},
		{name: "a short SHA", want: "fail", edits: []fixtureEdit{step("actions/setup-go@08eba0b")}},
		{name: "no ref", want: "fail", edits: []fixtureEdit{step("actions/setup-go")}},
		{name: "a docker reference", want: "fail", edits: []fixtureEdit{step("docker://alpine:3.20")},
			findings: []string{"docker://alpine:3.20"}},
		{name: "a remote reusable workflow by tag", want: "fail",
			edits:    []fixtureEdit{extra("  call:\n    uses: octo/ci/.github/workflows/verify.yml@v1\n")},
			findings: []string{".github/workflows/extra.yml: ", "job call", "octo/ci/.github/workflows/verify.yml@v1"}},
		{name: "a remote reusable workflow pinned", want: "pass",
			edits: []fixtureEdit{extra("  call:\n    uses: octo/ci/.github/workflows/verify.yml" + sha + "\n")}},
		{name: "a pinned go install", want: "pass", edits: []fixtureEdit{run("go install golang.org/x/tools/cmd/goimports@v0.30.0")}},
		{name: "go install @latest", want: "fail", edits: []fixtureEdit{run("go install golang.org/x/tools/cmd/goimports@latest")},
			findings: []string{".github/workflows/extra.yml: ", "goimports@latest", "pin"}},
		{name: "@latest quoted, in a list", want: "fail",
			edits: []fixtureEdit{run("|\n          npm ci && npx 'prettier@latest' --check .")}},
		{name: "@latest in a step that does not parse", want: "fail",
			edits:    []fixtureEdit{run("npx prettier@latest 'unclosed")},
			findings: []string{"prettier@latest"}},
		{name: "@latest inside a word is not a version", want: "pass", edits: []fixtureEdit{run("echo user@latest.example")}},
		{name: "a GitHub expression version is not @latest", want: "pass",
			edits: []fixtureEdit{run("go install github.com/go-task/task/v3/cmd/task@${{ env.TASK_VERSION }}")}},
		{name: "a workflow that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(".github/workflows/broken.yml", "on: push\njobs:\n  x:\n    steps: {}\n")},
			findings: []string{".github/workflows/broken.yml: ", "steps must be a list"}},
		{name: "no CI", want: "fail", edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile)}},
	})
}

// auditWith replaces audit.yml with a workflow of the given on: block and
// jobs.
func auditWith(on, jobs string) fixtureEdit {
	return withFile(auditFile, ciWorkflow(on, jobs))
}

// C-04: a job triggered on pull requests runs vibe audit and calls no
// verify command.
func TestRuleC04(t *testing.T) {
	t.Parallel()
	runRuleCases(t, "C-04", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "vibe audit after building vibe, in one step", want: "pass", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "go build -o \"$HOME/bin/vibe\" ./cmd/vibe && vibe audit --json"))}},
		{name: "vibe audit with leading assignments", want: "pass", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "NO_COLOR=1 vibe audit"))}},
		{name: "only pull requests", want: "pass", edits: []fixtureEdit{
			auditWith("on: pull_request\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{name: "in a local reusable workflow", want: "pass", edits: []fixtureEdit{
			auditWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/contract.yml\n"),
			withFile(".github/workflows/contract.yml", ciWorkflow("on: workflow_call\n", ciJob("audit", "ubuntu-latest", "vibe audit")))}},
		{name: "beside a verify step in other jobs", want: "pass", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{name: "the audit job also calls verify", want: "fail", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe audit", "task verify"))},
			findings: []string{auditFile + ": ", "job audit", "`task verify`", "own"}},
		{name: "the audit job calls one command of a list verify", want: "fail", edits: []fixtureEdit{listVerify,
			ciWith(ciPRAndPush, ciJob("lint", "ubuntu-latest", "task lint")+ciJob("test", "ubuntu-latest", "task test")),
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe audit", "task test"))},
			findings: []string{"`task test`"}},
		{name: "no audit job", want: "fail", edits: []fixtureEdit{withoutFile(auditFile)},
			findings: []string{".github/workflows/: ", "vibe audit", "pull_request"}},
		{name: "the audit job runs only on push", want: "fail", edits: []fixtureEdit{
			auditWith("on: push\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{name: "a paths filter makes it not count", want: "fail", edits: []fixtureEdit{
			auditWith("on:\n  pull_request:\n    paths: ['vibe.yaml']\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{name: "vibe without audit", want: "fail", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe doctor"))}},
		{name: "vibe audit by path is not vibe audit", want: "fail", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "./bin/vibe audit"))}},
		{name: "verify missing points at V-04", want: "fail", edits: []fixtureEdit{noVerify},
			findings: []string{"vibe.yaml: ", "V-04"}},
		{name: "another CI provider", want: "undecided", edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile),
			withFile("Jenkinsfile", "pipeline {}\n")}, findings: []string{"Jenkinsfile: "}},
	})
}

// R-01 (static part): no entrypoint, hook command or CI step calls vibe,
// except the C-04 audit job.
func TestRuleR01(t *testing.T) {
	t.Parallel()
	const others = "  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"
	runStaticPartCases(t, "R-01", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "vibe by path is not a call of vibe", want: "pass", edits: []fixtureEdit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("sync", "ubuntu-latest", "|\n          \"$RUNNER_TEMP/bin/vibe\" sync"))}},
		{name: "a verify entrypoint that calls vibe", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: vibe verify\n" + others)},
			findings: []string{"vibe.yaml: ", "entrypoints.verify", "`vibe verify`"}},
		{name: "a setup entrypoint that calls vibe", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task verify\n  setup: vibe sync\n" + others)},
			findings: []string{"vibe.yaml: ", "entrypoints.setup"}},
		{name: "a CI step outside the audit job", want: "fail", edits: []fixtureEdit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("sync", "ubuntu-latest", "vibe sync && git diff --exit-code"))},
			findings: []string{ciFile + ": ", "job sync", "`vibe sync`", "line "}},
		{name: "vibe audit in a job that also calls verify", want: "fail", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe audit", "task verify"))},
			findings: []string{auditFile + ": ", "`vibe audit`"}},
		{name: "vibe audit in a push-only workflow", want: "fail", edits: []fixtureEdit{
			auditWith("on: push\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{name: "other vibe commands in the audit job", want: "pass", edits: []fixtureEdit{
			auditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe doctor", "vibe audit"))}},
		{name: "the audit job in a local reusable workflow", want: "pass", edits: []fixtureEdit{
			auditWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/contract.yml\n"),
			withFile(".github/workflows/contract.yml", ciWorkflow("on: workflow_call\n", ciJob("audit", "ubuntu-latest", "vibe audit")))}},
		{name: "a lefthook command", want: "fail",
			edits:    []fixtureEdit{withFile("lefthook.yml", "pre-commit:\n  commands:\n    sync:\n      run: vibe sync\n")},
			findings: []string{"lefthook.yml: ", "pre-commit.commands.sync", "`vibe sync`"}},
		{name: "a lefthook job in a group", want: "fail",
			edits: []fixtureEdit{withoutFile("lefthook.yml"),
				withFile(".lefthook.yml", "pre-push:\n  jobs:\n    - group:\n        jobs:\n          - run: task lint && vibe audit\n")},
			findings: []string{".lefthook.yml: ", "vibe audit"}},
		{name: "a lefthook file it extends", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends:\n  - hooks/extra.yml\npre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			withFile("hooks/extra.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")},
			findings: []string{"hooks/extra.yml: "}},
		{name: "an untracked lefthook file it extends", want: "pass", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends:\n  - lefthook.local.yml\npre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			withUntracked("lefthook.local.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")}},
		// R-01 reads the hooks as the H rules do (regressions: it read
		// lefthook-local.yml, missed ./-prefixed extends, read
		// .lefthook.yml beside lefthook.yml, and decoded settings.json
		// loosely).
		{name: "a tracked lefthook-local.yml it extends is personal", want: "pass", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends:\n  - lefthook-local.yml\npre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			withFile("lefthook-local.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")}},
		{name: "a ./ path it extends", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends:\n  - ./hooks/extra.yml\n"),
			withFile("hooks/extra.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")},
			findings: []string{"hooks/extra.yml: ", "pre-push.commands.s", "`vibe sync`"}},
		{name: "only the first lefthook file is read", want: "pass", edits: []fixtureEdit{
			withFile("lefthook.yml", "pre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			withFile(".lefthook.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")}},
		{name: "an extending file replaces a command of the same name", want: "pass", edits: []fixtureEdit{
			withFile("lefthook.yml", "extends:\n  - hooks/extra.yml\npre-push:\n  commands:\n    s:\n      run: vibe sync\n"),
			withFile("hooks/extra.yml", "pre-push:\n  commands:\n    s:\n      run: task verify:fast\n")}},
		{name: "a lefthook script", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "pre-commit:\n  scripts:\n    sync.sh:\n      runner: bash\n"),
			withFile(".lefthook/pre-commit/sync.sh", "vibe sync\n")},
			findings: []string{".lefthook/pre-commit/sync.sh: ", "`vibe sync`"}},
		{name: "a lefthook hook other than pre-commit and pre-push", want: "fail", edits: []fixtureEdit{
			withFile("lefthook.yml", "post-checkout:\n  commands:\n    s:\n      run: vibe sync\n")},
			findings: []string{"lefthook.yml: ", "post-checkout.commands.s"}},
		{name: "a pre-commit entry with its args", want: "fail", edits: []fixtureEdit{withFile(".pre-commit-config.yaml",
			"repos:\n  - repo: local\n    hooks:\n      - id: sync\n        name: sync\n        entry: vibe\n        args: [sync]\n        language: system\n")},
			findings: []string{".pre-commit-config.yaml: ", "hook sync", "`vibe sync`"}},
		{name: "a husky hook other than pre-commit and pre-push", want: "fail",
			edits:    []fixtureEdit{withFile(".husky/commit-msg", "vibe sync\n")},
			findings: []string{".husky/commit-msg: "}},
		{name: "a Claude Code settings file the A rules do not accept", want: "fail", edits: []fixtureEdit{withFile(".claude/settings.json",
			`{"hooks": {"Stop": [{"matcher": 1, "hooks": [{"type": "command", "command": "task verify:fast"}]}]}}`)},
			findings: []string{".claude/settings.json: ", "does not parse"}},
		{name: "lefthook commands without vibe", want: "pass",
			edits: []fixtureEdit{withFile("lefthook.yml", "pre-commit:\n  commands:\n    fmt:\n      run: task fmt:check {staged_files}\n")}},
		{name: "a pre-commit local hook", want: "fail", edits: []fixtureEdit{withFile(".pre-commit-config.yaml",
			"repos:\n  - repo: local\n    hooks:\n      - id: sync\n        name: sync\n        entry: vibe sync\n        language: system\n")},
			findings: []string{".pre-commit-config.yaml: ", "hook sync"}},
		{name: "a husky script", want: "fail",
			edits:    []fixtureEdit{withFile(".husky/pre-push", "task verify\nvibe audit\n")},
			findings: []string{".husky/pre-push: "}},
		{name: "a Claude Code hook", want: "fail", edits: []fixtureEdit{withFile(".claude/settings.json",
			`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "vibe sync"}]}]}}`)},
			findings: []string{".claude/settings.json: ", "hooks.Stop"}},
		{name: "a Claude Code settings file that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(".claude/settings.json", `{"hooks": `)},
			findings: []string{".claude/settings.json: ", "does not parse"}},
		{name: "a lefthook file that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile("lefthook.yml", "pre-commit: [\n")},
			findings: []string{"lefthook.yml: ", "does not parse"}},
		{name: "a workflow that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(".github/workflows/broken.yml", "on: [\n")},
			findings: []string{".github/workflows/broken.yml: ", "does not parse"}},
		{name: "no CI: entrypoints and hooks still read", want: "pass",
			edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile)}},
		{name: "no CI, a hook calls vibe", want: "fail", edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile),
			withFile(".husky/pre-commit", "vibe sync\n")}},
	})
}

// runStaticPartCases is runRuleCases for a rule with a static part and
// other parts: under --static the others are not run, so want is the
// static part's result, and the findings are the rule's.
func runStaticPartCases(t *testing.T, id string, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep, _ := auditJSON(t, contractFixture(t, tc.edits...), "--static")
			r := rep.rule(t, id)
			got := ""
			for _, p := range r.Parts {
				if p.Check == "static" {
					got = p.Result
				}
			}
			if got != tc.want {
				t.Errorf("%s static part = %q, want %s\nfindings:\n%s", id, got, tc.want, r.findingText())
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

// matrixJob is a job that runs task verify on runs-on ${{ matrix.os }}
// with the given strategy.matrix body.
func matrixJob(matrix string) string {
	return "  v:\n    strategy:\n      matrix:\n" + matrix +
		"    runs-on: ${{ matrix.os }}\n    steps:\n      - run: task verify\n"
}

// C-05: the steps that call verify run on a Linux and on a Windows runner.
func TestRuleC05(t *testing.T) {
	t.Parallel()
	ubuntu, windows := ciVerifyJob("l", "ubuntu-22.04"), ciVerifyJob("w", "windows-2022")
	runRuleCases(t, "C-05", []ruleCase{
		{name: "baseline matrix", want: "pass"},
		{name: "two jobs", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush, ubuntu+windows)}},
		{name: "label lists", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush,
			ciVerifyJob("l", "[self-hosted, linux, x64]")+ciVerifyJob("w", "[self-hosted, windows]"))}},
		{name: "a runner group with labels", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush,
			ubuntu+ciVerifyJob("w", "\n      group: big\n      labels: [windows-latest]"))}},
		{name: "matrix include adds Windows", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush,
			matrixJob("        os: [ubuntu-latest]\n        include:\n          - os: windows-latest\n"))}},
		{name: "matrix of includes only", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush,
			matrixJob("        include:\n          - os: ubuntu-latest\n          - os: windows-latest\n"))}},
		{name: "matrix exclude removes one combination only", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush,
			matrixJob("        os: [ubuntu-latest, windows-latest]\n        go: ['1.24', '1.25']\n        exclude:\n          - os: windows-latest\n            go: '1.24'\n"))}},
		{name: "matrix exclude removes Windows", want: "fail", edits: []fixtureEdit{ciWith(ciPRAndPush,
			matrixJob("        os: [ubuntu-latest, windows-latest]\n        exclude:\n          - os: windows-latest\n"))},
			findings: []string{ciFile + ": ", "`task verify`", "Windows"}},
		{name: "matrix include merged into a combination", want: "pass", edits: []fixtureEdit{ciWith(ciPRAndPush,
			matrixJob("        os: [ubuntu-latest, windows-latest]\n        include:\n          - os: windows-latest\n            shell: pwsh\n"))}},
		{name: "an expression that is not one matrix value", want: "fail", edits: []fixtureEdit{ciWith(ciPRAndPush,
			"  v:\n    strategy:\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n"+
				"    runs-on: ${{ matrix.runner || matrix.os }}\n    steps:\n      - run: task verify\n")},
			findings: []string{"unknown platform"}},
		{name: "a matrix from an expression", want: "fail", edits: []fixtureEdit{ciWith(ciPRAndPush,
			"  v:\n    strategy:\n      matrix: ${{ fromJSON(needs.plan.outputs.matrix) }}\n"+
				"    runs-on: ${{ matrix.os }}\n    steps:\n      - run: task verify\n")},
			findings: []string{"unknown platform", "${{ matrix.os }}"}},
		{name: "runs-on from an input", want: "fail", edits: []fixtureEdit{ciWith(ciPRAndPush,
			ubuntu+ciVerifyJob("w", "${{ inputs.os }}"))},
			findings: []string{"Windows", "${{ inputs.os }}"}},
		{name: "Linux only", want: "fail", edits: []fixtureEdit{ciWith(ciPRAndPush, ubuntu)},
			findings: []string{ciFile + ": ", "no Windows runner", "windows-latest"}},
		{name: "macOS and Windows", want: "fail", edits: []fixtureEdit{ciWith(ciPRAndPush,
			ciVerifyJob("m", "macos-14")+windows)}, findings: []string{"no Linux runner"}},
		{name: "list verify, one command on Linux only", want: "fail", edits: []fixtureEdit{listVerify,
			ciWith(ciPRAndPush, "  lint:\n    runs-on: ubuntu-latest\n    steps:\n      - run: task lint\n"+
				"  test:\n    strategy:\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n"+
				"    runs-on: ${{ matrix.os }}\n    steps:\n      - run: task test\n")},
			findings: []string{"`task lint`"}},
		{name: "in a local reusable workflow", want: "pass", edits: []fixtureEdit{
			ciWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/reusable.yml\n"),
			withFile(".github/workflows/reusable.yml", ciWorkflow("on: workflow_call\n", ubuntu+windows))}},
		{name: "verify called nowhere", want: "fail",
			edits:    []fixtureEdit{ciWith(ciPRAndPush, ciJob("x", "ubuntu-latest", "echo hi"))},
			findings: []string{".github/workflows/: ", "no workflow step calls"}},
		{name: "not applicable without windows", want: "not_applicable",
			edits: []fixtureEdit{withFile("vibe.yaml", configWith(section{"platforms", "platforms: [linux]\n"})),
				ciWith(ciPRAndPush, ubuntu)}},
		{name: "verify missing points at V-04", want: "fail", edits: []fixtureEdit{noVerify},
			findings: []string{"vibe.yaml: ", "V-04"}},
		{name: "a workflow that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(".github/workflows/broken.yml", "on: push\njobs:\n  x: 3\n")},
			findings: []string{".github/workflows/broken.yml: ", "job x must be a map"}},
		{name: "another CI provider", want: "undecided", edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile),
			withFile(".circleci/config.yml", "version: 2.1\n")}, findings: []string{".circleci/config.yml: "}},
	})
}

// D-03: some workflow runs a recognized vulnerability scanner.
func TestRuleD03(t *testing.T) {
	t.Parallel()
	scan := func(script string) fixtureEdit {
		return withFile(ciFile, ciWorkflow(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("scan", "ubuntu-latest", script)))
	}
	runRuleCases(t, "D-03", []ruleCase{
		{name: "baseline govulncheck", want: "pass"},
		{name: "pip-audit", want: "pass", edits: []fixtureEdit{scan("pip-audit -r requirements.txt")}},
		{name: "osv-scanner", want: "pass", edits: []fixtureEdit{scan("osv-scanner scan -r .")}},
		{name: "trivy", want: "pass", edits: []fixtureEdit{scan("trivy fs .")}},
		{name: "pnpm audit", want: "pass", edits: []fixtureEdit{scan("pnpm audit --prod")}},
		{name: "npm audit inside a list", want: "pass", edits: []fixtureEdit{scan("npm ci && npm audit")}},
		{name: "in a scheduled workflow", want: "pass", edits: []fixtureEdit{
			scan("echo no scanner"),
			withFile(".github/workflows/nightly.yml", ciWorkflow("on:\n  schedule:\n    - cron: '0 3 * * *'\n",
				ciJob("scan", "ubuntu-latest", "govulncheck ./...")))}},
		{name: "a scanner run through another command", want: "fail", edits: []fixtureEdit{scan("uv run pip-audit")}},
		{name: "installing a scanner is not running it", want: "fail",
			edits: []fixtureEdit{scan("go install golang.org/x/vuln/cmd/govulncheck@v1.1.4")}},
		{name: "a scanner action is not recognized", want: "fail", edits: []fixtureEdit{withFile(ciFile, ciWorkflow(ciPRAndPush,
			ciVerifyJob("v", "ubuntu-latest")+"  scan:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: golang/govulncheck-action@08eba0b27e820071cde6df949e0beb9ba4906955\n"))}},
		{name: "no scanner", want: "fail", edits: []fixtureEdit{scan("echo no scanner")},
			findings: []string{".github/workflows/: ", "govulncheck"}},
		{name: "a workflow that does not parse", want: "fail",
			edits:    []fixtureEdit{withFile(".github/workflows/broken.yml", "on: push\njobs:\n  x:\n    steps:\n      - run: [a]\n")},
			findings: []string{".github/workflows/broken.yml: ", "run must be a string"}},
		{name: "no CI", want: "fail", edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile)},
			findings: []string{".github/workflows/: ", "no CI configuration"}},
		{name: "another CI provider", want: "undecided", edits: []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile),
			withFile(".buildkite/pipeline.yml", "steps: []\n")}, findings: []string{".buildkite/: "}},
	})
}
