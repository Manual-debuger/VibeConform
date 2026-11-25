package contract_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The CI rules (C-01 to C-05), D-03 and R-01's static part, through
// contract.Audit (spec 0052, "Recognition rules", GitHub workflows; ADR
// 0028). The baseline has .github/workflows/ci.yml (verify on a Linux and
// Windows matrix, a govulncheck scan) and audit.yml (vibe audit in its
// own job); each case changes one thing.

const (
	ciFile      = ".github/workflows/ci.yml"
	ciAuditFile = ".github/workflows/audit.yml"
	// ciPinned is a pinned checkout, as the baseline uses it.
	ciPinned = "actions/checkout@08eba0b27e820071cde6df949e0beb9ba4906955"
)

// ciWorkflow is a workflow with the baseline's permissions, the given on:
// block (written out, "on: ..." included) and jobs.
func ciWorkflow(on, jobs string) string {
	return on + "permissions:\n  contents: read\njobs:\n" + jobs
}

// ciWith replaces ci.yml with a workflow of the given on: block and jobs.
func ciWith(on, jobs string) contracttest.Edit {
	return contracttest.WithFile(ciFile, ciWorkflow(on, jobs))
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
	perms := func(p string) contracttest.Edit {
		return contracttest.WithFile(".github/workflows/extra.yml", "on: push\n"+p+"jobs:\n"+jobs)
	}
	ciRuleCases(t, goldenGitHub, "C-03", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "read-all", Want: "pass", Edits: []contracttest.Edit{perms("permissions: read-all\n")}},
		{Name: "empty map", Want: "pass", Edits: []contracttest.Edit{perms("permissions: {}\n")}},
		{Name: "read and none", Want: "pass",
			Edits: []contracttest.Edit{perms("permissions:\n  contents: read\n  issues: none\n")}},
		{Name: "a job widens them", Want: "pass", Edits: []contracttest.Edit{contracttest.WithFile(".github/workflows/extra.yml",
			"on: push\npermissions:\n  contents: read\n"+
				"jobs:\n  w:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n")}},
		{Name: "missing", Want: "fail", Edits: []contracttest.Edit{perms("")},
			Findings: []string{".github/workflows/extra.yml: ", "no top-level permissions", "contents: read"}},
		{Name: "a write scope", Want: "fail",
			Edits:    []contracttest.Edit{perms("permissions:\n  contents: read\n  security-events: write\n")},
			Findings: []string{".github/workflows/extra.yml: ", "security-events: write", "job"}},
		{Name: "write-all", Want: "fail", Edits: []contracttest.Edit{perms("permissions: write-all\n")},
			Findings: []string{".github/workflows/extra.yml: ", "write-all"}},
		{Name: "a workflow that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/workflows/broken.yml", "on: [push\n")},
			Findings: []string{".github/workflows/broken.yml: ", "does not parse"}},
	})
}

// ciListVerify declares verify as a list of two commands.
var ciListVerify = contracttest.Entrypoints("  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")

// ciNoVerify declares every entrypoint but verify.
var ciNoVerify = contracttest.Entrypoints("  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")

// C-01: workflows triggered by pull requests and by pushes to the default
// branch call verify. These fixtures have no origin, so the default
// branch is main, or master when only master is admitted.
func TestRuleC01(t *testing.T) {
	t.Parallel()
	linux := ciVerifyJob("v", "ubuntu-latest")
	onPush := func(filters string) string { return "on:\n  pull_request:\n  push:\n" + filters }
	onPR := func(filters string) string { return "on:\n  push:\n    branches: [main]\n  pull_request:\n" + filters }
	ciRuleCases(t, goldenGitHub, "C-01", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "on as a list", Want: "pass", Edits: []contracttest.Edit{ciWith("on: [pull_request, push]\n", linux)}},
		{Name: "on as one event per workflow", Want: "pass", Edits: []contracttest.Edit{
			ciWith("on: pull_request\n", linux),
			contracttest.WithFile(".github/workflows/push.yml", ciWorkflow("on: push\n", linux))}},
		{Name: "list verify, commands in different jobs", Want: "pass", Edits: []contracttest.Edit{ciListVerify,
			ciWith(ciPRAndPush, ciJob("lint", "ubuntu-latest", "task lint")+ciJob("test", "ubuntu-latest", "go mod download && task test"))}},
		{Name: "list verify, one command not called", Want: "fail", Edits: []contracttest.Edit{ciListVerify,
			ciWith(ciPRAndPush, ciJob("lint", "ubuntu-latest", "task lint"))},
			Findings: []string{".github/workflows/: ", "`task test`", "pull requests", "pushes to main"}},
		{Name: "verify inside a condition", Want: "pass", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "|\n          if true; then task verify; fi"))}},
		{Name: "verify with an extra argument is not called", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "task verify --force"))}},
		{Name: "a step that does not parse calls nothing", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "task verify 'unclosed"))}},
		{Name: "a GitHub expression is not a word", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "task ${{ matrix.task }}"))}},
		{Name: "no push", Want: "fail", Edits: []contracttest.Edit{ciWith("on: pull_request\n", linux)},
			Findings: []string{".github/workflows/: ", "pushes to main"}},
		{Name: "no pull request", Want: "fail", Edits: []contracttest.Edit{ciWith("on: push\n", linux)},
			Findings: []string{".github/workflows/: ", "pull requests"}},
		{Name: "push to another branch", Want: "fail", Edits: []contracttest.Edit{ciWith(onPush("    branches: [develop]\n"), linux)},
			Findings: []string{".github/workflows/: ", "pushes to main"}},
		{Name: "push branches double star", Want: "pass", Edits: []contracttest.Edit{ciWith(onPush("    branches: ['**']\n"), linux)}},
		{Name: "push branches single star", Want: "pass", Edits: []contracttest.Edit{ciWith(onPush("    branches: ['ma*']\n"), linux)}},
		{Name: "push branches negated", Want: "fail", Edits: []contracttest.Edit{ciWith(onPush("    branches: ['*', '!main']\n"), linux)}},
		{Name: "push branches negated then included again", Want: "pass",
			Edits: []contracttest.Edit{ciWith(onPush("    branches: ['*', '!ma*', 'main']\n"), linux)}},
		{Name: "push branches as one string", Want: "pass", Edits: []contracttest.Edit{ciWith(onPush("    branches: main\n"), linux)}},
		{Name: "push branches-ignore others", Want: "pass",
			Edits: []contracttest.Edit{ciWith(onPush("    branches-ignore: ['release/**']\n"), linux)}},
		{Name: "push branches-ignore main", Want: "fail", Edits: []contracttest.Edit{ciWith(onPush("    branches-ignore: [main]\n"), linux)}},
		{Name: "push tags without branches", Want: "fail", Edits: []contracttest.Edit{ciWith(onPush("    tags: ['v*']\n"), linux)}},
		{Name: "push tags with branches", Want: "pass",
			Edits: []contracttest.Edit{ciWith(onPush("    branches: [main]\n    tags: ['v*']\n"), linux)}},
		{Name: "push paths filter", Want: "fail",
			Edits: []contracttest.Edit{ciWith(onPush("    branches: [main]\n    paths: ['**.go']\n"), linux)}},
		{Name: "pull request paths-ignore filter", Want: "fail",
			Edits:    []contracttest.Edit{ciWith(onPR("    paths-ignore: ['docs/**']\n"), linux)},
			Findings: []string{"pull requests"}},
		{Name: "pull request branches admit main", Want: "pass", Edits: []contracttest.Edit{ciWith(onPR("    branches: [main]\n"), linux)}},
		{Name: "pull request branches exclude main", Want: "fail", Edits: []contracttest.Edit{ciWith(onPR("    branches: [develop]\n"), linux)}},
		{Name: "pull request types keep the defaults", Want: "pass",
			Edits: []contracttest.Edit{ciWith(onPR("    types: [opened, synchronize, reopened, labeled]\n"), linux)}},
		{Name: "pull request types drop synchronize", Want: "fail",
			Edits: []contracttest.Edit{ciWith(onPR("    types: [opened, reopened]\n"), linux)}},
		{Name: "pull_request_target does not count", Want: "fail",
			Edits: []contracttest.Edit{ciWith("on:\n  pull_request_target:\n  push:\n    branches: [main]\n", linux)}},
		{Name: "job if: conditions are ignored", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			"  v:\n    if: false\n    runs-on: ubuntu-latest\n    steps:\n      - run: task verify\n")}},
		{Name: "local reusable workflow", Want: "pass", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/reusable.yml\n"),
			contracttest.WithFile(".github/workflows/reusable.yml", ciWorkflow("on: workflow_call\n", linux))}},
		{Name: "local reusable workflow, nested", Want: "pass", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/outer.yml\n"),
			contracttest.WithFile(".github/workflows/outer.yml", ciWorkflow("on: workflow_call\n", "  inner:\n    uses: ./.github/workflows/inner.yml\n")),
			contracttest.WithFile(".github/workflows/inner.yml", ciWorkflow("on: workflow_call\n", linux))}},
		{Name: "a reusable workflow's own triggers do not count", Want: "fail", Edits: []contracttest.Edit{
			ciWith("on: workflow_dispatch\n", "  call:\n    uses: ./.github/workflows/reusable.yml\n"),
			contracttest.WithFile(".github/workflows/reusable.yml", ciWorkflow("on: workflow_call\n", linux))}},
		{Name: "remote reusable workflow is not followed", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, "  call:\n    uses: octo/ci/.github/workflows/verify.yml@08eba0b27e820071cde6df949e0beb9ba4906955\n")}},
		{Name: "master is assumed when the filters favor it", Want: "pass",
			Edits: []contracttest.Edit{ciWith(onPush("    branches: [master]\n"), linux), contracttest.WithoutFile(ciAuditFile)}},
		{Name: "main is assumed on a tie", Want: "fail",
			Edits:    []contracttest.Edit{ciWith(onPush("    branches: [master]\n"), linux)},
			Findings: []string{"pushes to main"}},
		{Name: "verify missing points at V-04", Want: "fail",
			Edits:    []contracttest.Edit{ciNoVerify},
			Findings: []string{"vibe.yaml: ", "V-04"}},
		{Name: "a workflow that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/workflows/broken.yml", "jobs: [\n")},
			Findings: []string{".github/workflows/broken.yml: ", "does not parse"}},
		{Name: "a workflow whose jobs are not a map", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/workflows/broken.yml", "on: push\njobs: [a, b]\n")},
			Findings: []string{".github/workflows/broken.yml: ", "jobs must be a map"}},
		{Name: "no CI", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile)},
			Findings: []string{".github/workflows/: ", "no CI configuration"}},
		{Name: "an untracked workflow does not count", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile),
				contracttest.WithUntracked(ciFile, ciWorkflow(ciPRAndPush, linux))},
			Findings: []string{"no CI configuration"}},
		{Name: "another CI provider", Want: "undecided",
			Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile),
				contracttest.WithFile("azure-pipelines.yml", "steps:\n  - script: task verify\n")},
			Findings: []string{"azure-pipelines.yml: ", "GitHub Actions"}},
		{Name: "another CI provider beside GitHub workflows", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithFile("azure-pipelines.yml", "steps:\n  - script: task verify\n")}},
	})
}

// The default branch comes from refs/remotes/origin/HEAD when the clone
// has one; otherwise the report says which branch it assumed.
func TestRuleC01DefaultBranch(t *testing.T) {
	t.Parallel()
	onPush := func(branch string) contracttest.Edit {
		return ciWith("on:\n  pull_request:\n  push:\n    branches: ["+branch+"]\n", ciVerifyJob("v", "ubuntu-latest"))
	}
	withOrigin := func(t *testing.T, branch string, edits ...contracttest.Edit) string {
		t.Helper()
		dir := contracttest.GitFixture(t, contracttest.Baseline, edits...)
		contracttest.Git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+branch)
		return dir
	}
	// The report's notes; the CLI prints them (TestContractTextReportEverySection).
	static := func(t *testing.T, dir string) (*contract.Report, string) {
		t.Helper()
		rep := contracttest.Audit(t, dir, contract.Options{Static: true})
		return rep, strings.Join(rep.Notes, "\n")
	}
	t.Run("origin/HEAD names trunk, push to trunk", func(t *testing.T) {
		rep, _ := static(t, withOrigin(t, "trunk", onPush("trunk")))
		contracttest.AssertRule(t, rep, "C-01", contract.ResultPass)
	})
	t.Run("origin/HEAD names trunk, push to main", func(t *testing.T) {
		rep, _ := static(t, withOrigin(t, "trunk", onPush("main")))
		contracttest.AssertRule(t, rep, "C-01", contract.ResultFail, "pushes to trunk")
	})
	t.Run("origin/HEAD names master, nothing assumed", func(t *testing.T) {
		_, notes := static(t, withOrigin(t, "master", onPush("master")))
		if strings.Contains(notes, "assume") {
			t.Errorf("report notes an assumed branch although origin/HEAD names it:\n%s", notes)
		}
	})
	t.Run("no origin/HEAD, the report says main was assumed", func(t *testing.T) {
		_, notes := static(t, contracttest.GitFixture(t, contracttest.Baseline))
		if !strings.Contains(notes, "assume the default branch is main") {
			t.Errorf("report does not say main was assumed:\n%s", notes)
		}
	})
	t.Run("no origin/HEAD, the report says master was assumed", func(t *testing.T) {
		_, notes := static(t, contracttest.GitFixture(t, contracttest.Baseline, onPush("master"), contracttest.WithoutFile(ciAuditFile)))
		if !strings.Contains(notes, "assume the default branch is master") {
			t.Errorf("report does not say master was assumed:\n%s", notes)
		}
	})
}

// When the report notes the assumed default branch (contract 1.0.1): on
// GitHub Actions only when C-01, C-04 or R-01 reads the runs, so an
// audit that runs none of them has no note; on GitLab CI whenever the
// configuration is read. Regression: spec 0067 first noted it whenever
// the GitHub workflows were read, which every audit does.
func TestRuleCIDefaultBranchNote(t *testing.T) {
	t.Parallel()
	assumed := func(rep *contract.Report) bool {
		return slices.ContainsFunc(rep.Notes, func(n string) bool { return strings.Contains(n, "assume the default branch") })
	}
	for _, tc := range []struct {
		name  string
		edits []contracttest.Edit
		rules []string
		want  bool
	}{
		{name: "GitHub, C-02 and C-03 only", rules: []string{"C-02", "C-03"}},
		{name: "GitHub, C-01", rules: []string{"C-01"}, want: true},
		{name: "GitHub, C-04", rules: []string{"C-04"}, want: true},
		{name: "GitHub, R-01", rules: []string{"A-02", "R-01"}, want: true},
		{name: "GitLab, C-02 only", edits: gitlab(glOnly(glVerify("verify", ""))), rules: []string{"C-02"}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep := contracttest.Audit(t, contracttest.GitFixture(t, contracttest.Baseline, tc.edits...),
				contract.Options{Static: true, Rules: contracttest.Rules(t, tc.rules...)})
			if got := assumed(rep); got != tc.want {
				t.Errorf("the report notes an assumed default branch: %t, want %t; notes %q", got, tc.want, rep.Notes)
			}
		})
	}
}

// C-02: every uses: but a local path is pinned to a 40-character commit
// SHA, and no run: step has a word ending in @latest. Every workflow
// counts, whatever its triggers.
func TestRuleC02(t *testing.T) {
	t.Parallel()
	extra := func(jobs string) contracttest.Edit {
		return contracttest.WithFile(".github/workflows/extra.yml", ciWorkflow("on: workflow_dispatch\n", jobs))
	}
	step := func(uses string) contracttest.Edit {
		return extra("  x:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n")
	}
	run := func(script string) contracttest.Edit { return extra(ciJob("x", "ubuntu-latest", script)) }
	const sha = "@08eba0b27e820071cde6df949e0beb9ba4906955"
	ciRuleCases(t, goldenGitHub, "C-02", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "local action", Want: "pass", Edits: []contracttest.Edit{step("./.github/actions/setup")}},
		{Name: "action in a subdirectory, pinned", Want: "pass", Edits: []contracttest.Edit{step("github/codeql-action/init" + sha)}},
		{Name: "upper-case SHA", Want: "pass", Edits: []contracttest.Edit{step("actions/setup-go@08EBA0B27E820071CDE6DF949E0BEB9BA4906955")}},
		{Name: "local reusable workflow", Want: "pass",
			Edits: []contracttest.Edit{extra("  call:\n    uses: ./.github/workflows/ci.yml\n")}},
		{Name: "a version tag", Want: "fail", Edits: []contracttest.Edit{step("actions/setup-go@v5")},
			Findings: []string{".github/workflows/extra.yml: ", "actions/setup-go@v5", "line ", "40-character"}},
		{Name: "a short SHA", Want: "fail", Edits: []contracttest.Edit{step("actions/setup-go@08eba0b")}},
		{Name: "no ref", Want: "fail", Edits: []contracttest.Edit{step("actions/setup-go")}},
		{Name: "a docker reference", Want: "fail", Edits: []contracttest.Edit{step("docker://alpine:3.20")},
			Findings: []string{"docker://alpine:3.20"}},
		{Name: "a remote reusable workflow by tag", Want: "fail",
			Edits:    []contracttest.Edit{extra("  call:\n    uses: octo/ci/.github/workflows/verify.yml@v1\n")},
			Findings: []string{".github/workflows/extra.yml: ", "job call", "octo/ci/.github/workflows/verify.yml@v1"}},
		{Name: "a remote reusable workflow pinned", Want: "pass",
			Edits: []contracttest.Edit{extra("  call:\n    uses: octo/ci/.github/workflows/verify.yml" + sha + "\n")}},
		{Name: "a pinned go install", Want: "pass", Edits: []contracttest.Edit{run("go install golang.org/x/tools/cmd/goimports@v0.30.0")}},
		{Name: "go install @latest", Want: "fail", Edits: []contracttest.Edit{run("go install golang.org/x/tools/cmd/goimports@latest")},
			Findings: []string{".github/workflows/extra.yml: ", "goimports@latest", "pin"}},
		{Name: "@latest quoted, in a list", Want: "fail",
			Edits: []contracttest.Edit{run("|\n          npm ci && npx 'prettier@latest' --check .")}},
		{Name: "@latest in a step that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{run("npx prettier@latest 'unclosed")},
			Findings: []string{"prettier@latest"}},
		{Name: "@latest inside a word is not a version", Want: "pass", Edits: []contracttest.Edit{run("echo user@latest.example")}},
		{Name: "a GitHub expression version is not @latest", Want: "pass",
			Edits: []contracttest.Edit{run("go install github.com/go-task/task/v3/cmd/task@${{ env.TASK_VERSION }}")}},
		// Spec 0067, "C-02": the unpinned uses: findings come before the
		// @latest findings, and a word repeated in one run: step gives one
		// finding, as on GitLab CI.
		{Name: "an @latest run: step before an unpinned uses: step", Want: "fail", Edits: []contracttest.Edit{extra(
			"  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npx prettier@latest --check .\n      - uses: actions/setup-go@v5\n")},
			Findings: []string{"prettier@latest", "actions/setup-go@v5"}},
		{Name: "a run: step that repeats one @latest word", Want: "fail",
			Edits:    []contracttest.Edit{run("npx prettier@latest --check . && npx prettier@latest --write .")},
			Findings: []string{"prettier@latest"}},
		{Name: "a workflow that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/workflows/broken.yml", "on: push\njobs:\n  x:\n    steps: {}\n")},
			Findings: []string{".github/workflows/broken.yml: ", "steps must be a list"}},
		{Name: "no CI", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile)}},
	})
}

// ciAuditWith replaces audit.yml with a workflow of the given on: block and
// jobs.
func ciAuditWith(on, jobs string) contracttest.Edit {
	return contracttest.WithFile(ciAuditFile, ciWorkflow(on, jobs))
}

// C-04: a job triggered on pull requests runs vibe audit and calls no
// verify command.
func TestRuleC04(t *testing.T) {
	t.Parallel()
	ciRuleCases(t, goldenGitHub, "C-04", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "vibe audit after building vibe, in one step", Want: "pass", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "go build -o \"$HOME/bin/vibe\" ./cmd/vibe && vibe audit --json"))}},
		{Name: "vibe audit with leading assignments", Want: "pass", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "NO_COLOR=1 vibe audit"))}},
		{Name: "only pull requests", Want: "pass", Edits: []contracttest.Edit{
			ciAuditWith("on: pull_request\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{Name: "in a local reusable workflow", Want: "pass", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/contract.yml\n"),
			contracttest.WithFile(".github/workflows/contract.yml", ciWorkflow("on: workflow_call\n", ciJob("audit", "ubuntu-latest", "vibe audit")))}},
		{Name: "beside a verify step in other jobs", Want: "pass", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{Name: "the audit job also calls verify", Want: "fail", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe audit", "task verify"))},
			Findings: []string{ciAuditFile + ": ", "job audit", "`task verify`", "own"}},
		{Name: "the audit job calls one command of a list verify", Want: "fail", Edits: []contracttest.Edit{ciListVerify,
			ciWith(ciPRAndPush, ciJob("lint", "ubuntu-latest", "task lint")+ciJob("test", "ubuntu-latest", "task test")),
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe audit", "task test"))},
			Findings: []string{"`task test`"}},
		{Name: "no audit job", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(ciAuditFile)},
			Findings: []string{".github/workflows/: ", "vibe audit", "pull_request"}},
		{Name: "the audit job runs only on push", Want: "fail", Edits: []contracttest.Edit{
			ciAuditWith("on: push\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{Name: "a paths filter makes it not count", Want: "fail", Edits: []contracttest.Edit{
			ciAuditWith("on:\n  pull_request:\n    paths: ['vibe.yaml']\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{Name: "vibe without audit", Want: "fail", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe doctor"))}},
		{Name: "vibe audit by path is not vibe audit", Want: "fail", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "./bin/vibe audit"))}},
		{Name: "verify missing points at V-04", Want: "fail", Edits: []contracttest.Edit{ciNoVerify},
			Findings: []string{"vibe.yaml: ", "V-04"}},
		{Name: "another CI provider", Want: "undecided", Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile),
			contracttest.WithFile("Jenkinsfile", "pipeline {}\n")}, Findings: []string{"Jenkinsfile: "}},
	})
}

// R-01 (static part): no entrypoint, hook command or CI step calls vibe,
// except the C-04 audit job.
func TestRuleR01(t *testing.T) {
	t.Parallel()
	const others = "  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"
	ciPartCases(t, goldenGitHub, "R-01", contract.Static, []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "vibe by path is not a call of vibe", Want: "pass", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("sync", "ubuntu-latest", "|\n          \"$RUNNER_TEMP/bin/vibe\" sync"))}},
		// vibe --version tests an installed binary (spec 0055, #120 Part
		// A): exempt only in a CI job that calls no entrypoint.
		{Name: "vibe --version in a job that calls no entrypoint", Want: "pass", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("install", "ubuntu-latest", "go install ./cmd/vibe", "vibe --version | tee version.txt"))}},
		{Name: "vibe --version in a job that calls verify", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "vibe --version", "task verify"))},
			Findings: []string{ciFile + ": ", "job v", "`vibe --version`"}},
		{Name: "vibe --version in a job that calls one command of a list verify", Want: "fail", Edits: []contracttest.Edit{ciListVerify,
			ciWith(ciPRAndPush, ciJob("lint", "ubuntu-latest", "task lint")+ciJob("test", "ubuntu-latest", "vibe --version", "task test"))},
			Findings: []string{ciFile + ": ", "job test", "`vibe --version`"}},
		{Name: "vibe --version in a job that calls fmt-check", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("fmt", "ubuntu-latest", "vibe --version", "task fmt:check"))},
			Findings: []string{"job fmt", "`vibe --version`"}},
		{Name: "vibe --version with more words is a call", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("install", "ubuntu-latest", "vibe --version --json"))},
			Findings: []string{"`vibe --version --json`"}},
		{Name: "vibe version is a call", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("install", "ubuntu-latest", "vibe version"))},
			Findings: []string{"`vibe version`"}},
		{Name: "vibe --version in an entrypoint", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  setup: vibe --version\n" + others)},
			Findings: []string{"vibe.yaml: ", "entrypoints.setup", "`vibe --version`"}},
		{Name: "vibe --version in a hook", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".husky/pre-commit", "vibe --version\n")},
			Findings: []string{".husky/pre-commit: ", "`vibe --version`"}},
		{Name: "a verify entrypoint that calls vibe", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: vibe verify\n" + others)},
			Findings: []string{"vibe.yaml: ", "entrypoints.verify", "`vibe verify`"}},
		{Name: "a setup entrypoint that calls vibe", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  setup: vibe sync\n" + others)},
			Findings: []string{"vibe.yaml: ", "entrypoints.setup"}},
		{Name: "a CI step outside the audit job", Want: "fail", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("sync", "ubuntu-latest", "vibe sync && git diff --exit-code"))},
			Findings: []string{ciFile + ": ", "job sync", "`vibe sync`", "line "}},
		{Name: "vibe audit in a job that also calls verify", Want: "fail", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe audit", "task verify"))},
			Findings: []string{ciAuditFile + ": ", "`vibe audit`"}},
		{Name: "vibe audit in a push-only workflow", Want: "fail", Edits: []contracttest.Edit{
			ciAuditWith("on: push\n", ciJob("audit", "ubuntu-latest", "vibe audit"))}},
		{Name: "other vibe commands in the audit job", Want: "pass", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "vibe doctor", "vibe audit"))}},
		{Name: "the audit job in a local reusable workflow", Want: "pass", Edits: []contracttest.Edit{
			ciAuditWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/contract.yml\n"),
			contracttest.WithFile(".github/workflows/contract.yml", ciWorkflow("on: workflow_call\n", ciJob("audit", "ubuntu-latest", "vibe audit")))}},
		{Name: "a lefthook command", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    sync:\n      run: vibe sync\n")},
			Findings: []string{"lefthook.yml: ", "pre-commit.commands.sync", "`vibe sync`"}},
		{Name: "a lefthook job in a group", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithoutFile("lefthook.yml"),
				contracttest.WithFile(".lefthook.yml", "pre-push:\n  jobs:\n    - group:\n        jobs:\n          - run: task lint && vibe audit\n")},
			Findings: []string{".lefthook.yml: ", "vibe audit"}},
		{Name: "a lefthook file it extends", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends:\n  - hooks/extra.yml\npre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			contracttest.WithFile("hooks/extra.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")},
			Findings: []string{"hooks/extra.yml: "}},
		{Name: "an untracked lefthook file it extends", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends:\n  - lefthook.local.yml\npre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			contracttest.WithUntracked("lefthook.local.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")}},
		// R-01 reads the hooks as the H rules do (regressions: it read
		// lefthook-local.yml, missed ./-prefixed extends, read
		// .lefthook.yml beside lefthook.yml, and decoded settings.json
		// loosely).
		{Name: "a tracked lefthook-local.yml it extends is personal", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends:\n  - lefthook-local.yml\npre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			contracttest.WithFile("lefthook-local.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")}},
		{Name: "a ./ path it extends", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends:\n  - ./hooks/extra.yml\n"),
			contracttest.WithFile("hooks/extra.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")},
			Findings: []string{"hooks/extra.yml: ", "pre-push.commands.s", "`vibe sync`"}},
		{Name: "only the first lefthook file is read", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    fmt:\n      run: task fmt:check\n"),
			contracttest.WithFile(".lefthook.yml", "pre-push:\n  commands:\n    s:\n      run: vibe sync\n")}},
		{Name: "an extending file replaces a command of the same name", Want: "pass", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "extends:\n  - hooks/extra.yml\npre-push:\n  commands:\n    s:\n      run: vibe sync\n"),
			contracttest.WithFile("hooks/extra.yml", "pre-push:\n  commands:\n    s:\n      run: task verify:fast\n")}},
		{Name: "a lefthook script", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "pre-commit:\n  scripts:\n    sync.sh:\n      runner: bash\n"),
			contracttest.WithFile(".lefthook/pre-commit/sync.sh", "vibe sync\n")},
			Findings: []string{".lefthook/pre-commit/sync.sh: ", "`vibe sync`"}},
		{Name: "a lefthook hook other than pre-commit and pre-push", Want: "fail", Edits: []contracttest.Edit{
			contracttest.WithFile("lefthook.yml", "post-checkout:\n  commands:\n    s:\n      run: vibe sync\n")},
			Findings: []string{"lefthook.yml: ", "post-checkout.commands.s"}},
		{Name: "a pre-commit entry with its args", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(".pre-commit-config.yaml",
			"repos:\n  - repo: local\n    hooks:\n      - id: sync\n        name: sync\n        entry: vibe\n        args: [sync]\n        language: system\n")},
			Findings: []string{".pre-commit-config.yaml: ", "hook sync", "`vibe sync`"}},
		{Name: "a husky hook other than pre-commit and pre-push", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".husky/commit-msg", "vibe sync\n")},
			Findings: []string{".husky/commit-msg: "}},
		{Name: "a Claude Code settings file the A rules do not accept", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(".claude/settings.json",
			`{"hooks": {"Stop": [{"matcher": 1, "hooks": [{"type": "command", "command": "task verify:fast"}]}]}}`)},
			Findings: []string{".claude/settings.json: ", "does not parse"}},
		{Name: "lefthook commands without vibe", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithFile("lefthook.yml", "pre-commit:\n  commands:\n    fmt:\n      run: task fmt:check {staged_files}\n")}},
		{Name: "a pre-commit local hook", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(".pre-commit-config.yaml",
			"repos:\n  - repo: local\n    hooks:\n      - id: sync\n        name: sync\n        entry: vibe sync\n        language: system\n")},
			Findings: []string{".pre-commit-config.yaml: ", "hook sync"}},
		{Name: "a husky script", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".husky/pre-push", "task verify\nvibe audit\n")},
			Findings: []string{".husky/pre-push: "}},
		{Name: "a Claude Code hook", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(".claude/settings.json",
			`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "vibe sync"}]}]}}`)},
			Findings: []string{".claude/settings.json: ", "hooks.Stop"}},
		{Name: "a Claude Code settings file that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".claude/settings.json", `{"hooks": `)},
			Findings: []string{".claude/settings.json: ", "does not parse"}},
		{Name: "a lefthook file that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("lefthook.yml", "pre-commit: [\n")},
			Findings: []string{"lefthook.yml: ", "does not parse"}},
		{Name: "a workflow that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/workflows/broken.yml", "on: [\n")},
			Findings: []string{".github/workflows/broken.yml: ", "does not parse"}},
		{Name: "no CI: entrypoints and hooks still read", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile)}},
		{Name: "no CI, a hook calls vibe", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile),
			contracttest.WithFile(".husky/pre-commit", "vibe sync\n")}},
	})
}

// ciMatrixJob is a job that runs task verify on runs-on ${{ matrix.os }}
// with the given strategy.matrix body.
func ciMatrixJob(matrix string) string {
	return "  v:\n    strategy:\n      matrix:\n" + matrix +
		"    runs-on: ${{ matrix.os }}\n    steps:\n      - run: task verify\n"
}

// C-05: the steps that call verify run on a Linux and on a Windows runner.
func TestRuleC05(t *testing.T) {
	t.Parallel()
	ubuntu, windows := ciVerifyJob("l", "ubuntu-22.04"), ciVerifyJob("w", "windows-2022")
	ciRuleCases(t, goldenGitHub, "C-05", []contracttest.RuleCase{
		{Name: "baseline matrix", Want: "pass"},
		{Name: "two jobs", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush, ubuntu+windows)}},
		{Name: "label lists", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ciVerifyJob("l", "[self-hosted, linux, x64]")+ciVerifyJob("w", "[self-hosted, windows]"))}},
		{Name: "a runner group with labels", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ubuntu+ciVerifyJob("w", "\n      group: big\n      labels: [windows-latest]"))}},
		{Name: "matrix include adds Windows", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ciMatrixJob("        os: [ubuntu-latest]\n        include:\n          - os: windows-latest\n"))}},
		{Name: "matrix of includes only", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ciMatrixJob("        include:\n          - os: ubuntu-latest\n          - os: windows-latest\n"))}},
		{Name: "matrix exclude removes one combination only", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ciMatrixJob("        os: [ubuntu-latest, windows-latest]\n        go: ['1.24', '1.25']\n        exclude:\n          - os: windows-latest\n            go: '1.24'\n"))}},
		{Name: "matrix exclude removes Windows", Want: "fail", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ciMatrixJob("        os: [ubuntu-latest, windows-latest]\n        exclude:\n          - os: windows-latest\n"))},
			Findings: []string{ciFile + ": ", "`task verify`", "Windows"}},
		{Name: "matrix include merged into a combination", Want: "pass", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ciMatrixJob("        os: [ubuntu-latest, windows-latest]\n        include:\n          - os: windows-latest\n            shell: pwsh\n"))}},
		{Name: "an expression that is not one matrix value", Want: "fail", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			"  v:\n    strategy:\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n"+
				"    runs-on: ${{ matrix.runner || matrix.os }}\n    steps:\n      - run: task verify\n")},
			Findings: []string{"unknown platform"}},
		{Name: "a matrix from an expression", Want: "fail", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			"  v:\n    strategy:\n      matrix: ${{ fromJSON(needs.plan.outputs.matrix) }}\n"+
				"    runs-on: ${{ matrix.os }}\n    steps:\n      - run: task verify\n")},
			Findings: []string{"unknown platform", "${{ matrix.os }}"}},
		{Name: "runs-on from an input", Want: "fail", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ubuntu+ciVerifyJob("w", "${{ inputs.os }}"))},
			Findings: []string{"Windows", "${{ inputs.os }}"}},
		{Name: "Linux only", Want: "fail", Edits: []contracttest.Edit{ciWith(ciPRAndPush, ubuntu)},
			Findings: []string{ciFile + ": ", "no Windows runner", "windows-latest"}},
		{Name: "macOS and Windows", Want: "fail", Edits: []contracttest.Edit{ciWith(ciPRAndPush,
			ciVerifyJob("m", "macos-14")+windows)}, Findings: []string{"no Linux runner"}},
		{Name: "list verify, one command on Linux only", Want: "fail", Edits: []contracttest.Edit{ciListVerify,
			ciWith(ciPRAndPush, "  lint:\n    runs-on: ubuntu-latest\n    steps:\n      - run: task lint\n"+
				"  test:\n    strategy:\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n"+
				"    runs-on: ${{ matrix.os }}\n    steps:\n      - run: task test\n")},
			Findings: []string{"`task lint`"}},
		{Name: "in a local reusable workflow", Want: "pass", Edits: []contracttest.Edit{
			ciWith(ciPRAndPush, "  call:\n    uses: ./.github/workflows/reusable.yml\n"),
			contracttest.WithFile(".github/workflows/reusable.yml", ciWorkflow("on: workflow_call\n", ubuntu+windows))}},
		{Name: "verify called nowhere", Want: "fail",
			Edits:    []contracttest.Edit{ciWith(ciPRAndPush, ciJob("x", "ubuntu-latest", "echo hi"))},
			Findings: []string{".github/workflows/: ", "no workflow step calls"}},
		{Name: "not applicable without windows", Want: "not_applicable",
			Edits: []contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: [linux]\n"})),
				ciWith(ciPRAndPush, ubuntu)}},
		{Name: "verify missing points at V-04", Want: "fail", Edits: []contracttest.Edit{ciNoVerify},
			Findings: []string{"vibe.yaml: ", "V-04"}},
		{Name: "a workflow that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/workflows/broken.yml", "on: push\njobs:\n  x: 3\n")},
			Findings: []string{".github/workflows/broken.yml: ", "job x must be a map"}},
		{Name: "another CI provider", Want: "undecided", Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile),
			contracttest.WithFile(".circleci/config.yml", "version: 2.1\n")}, Findings: []string{".circleci/config.yml: "}},
	})
}

// D-03: some workflow runs a recognized vulnerability scanner.
func TestRuleD03(t *testing.T) {
	t.Parallel()
	scan := func(script string) contracttest.Edit {
		return contracttest.WithFile(ciFile, ciWorkflow(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("scan", "ubuntu-latest", script)))
	}
	ciRuleCases(t, goldenGitHub, "D-03", []contracttest.RuleCase{
		{Name: "baseline govulncheck", Want: "pass"},
		{Name: "pip-audit", Want: "pass", Edits: []contracttest.Edit{scan("pip-audit -r requirements.txt")}},
		{Name: "osv-scanner", Want: "pass", Edits: []contracttest.Edit{scan("osv-scanner scan -r .")}},
		{Name: "trivy", Want: "pass", Edits: []contracttest.Edit{scan("trivy fs .")}},
		{Name: "pnpm audit", Want: "pass", Edits: []contracttest.Edit{scan("pnpm audit --prod")}},
		{Name: "npm audit inside a list", Want: "pass", Edits: []contracttest.Edit{scan("npm ci && npm audit")}},
		{Name: "in a scheduled workflow", Want: "pass", Edits: []contracttest.Edit{
			scan("echo no scanner"),
			contracttest.WithFile(".github/workflows/nightly.yml", ciWorkflow("on:\n  schedule:\n    - cron: '0 3 * * *'\n",
				ciJob("scan", "ubuntu-latest", "govulncheck ./...")))}},
		{Name: "a scanner run through another command", Want: "fail", Edits: []contracttest.Edit{scan("uv run pip-audit")}},
		{Name: "installing a scanner is not running it", Want: "fail",
			Edits: []contracttest.Edit{scan("go install golang.org/x/vuln/cmd/govulncheck@v1.1.4")}},
		{Name: "a scanner action is not recognized", Want: "fail", Edits: []contracttest.Edit{contracttest.WithFile(ciFile, ciWorkflow(ciPRAndPush,
			ciVerifyJob("v", "ubuntu-latest")+"  scan:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: golang/govulncheck-action@08eba0b27e820071cde6df949e0beb9ba4906955\n"))}},
		{Name: "no scanner", Want: "fail", Edits: []contracttest.Edit{scan("echo no scanner")},
			Findings: []string{".github/workflows/: ", "govulncheck"}},
		{Name: "a workflow that does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/workflows/broken.yml", "on: push\njobs:\n  x:\n    steps:\n      - run: [a]\n")},
			Findings: []string{".github/workflows/broken.yml: ", "run must be a string"}},
		{Name: "no CI", Want: "fail", Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile)},
			Findings: []string{".github/workflows/: ", "no CI configuration"}},
		{Name: "another CI provider", Want: "undecided", Edits: []contracttest.Edit{contracttest.WithoutFile(ciFile), contracttest.WithoutFile(ciAuditFile),
			contracttest.WithFile(".buildkite/pipeline.yml", "steps: []\n")}, Findings: []string{".buildkite/: "}},
	})
}
