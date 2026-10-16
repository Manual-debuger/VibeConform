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
				withFile(".gitlab-ci.yml", "test:\n  script: task verify\n")},
			findings: []string{".gitlab-ci.yml: ", "GitHub Actions"}},
		{name: "another CI provider beside GitHub workflows", want: "pass",
			edits: []fixtureEdit{withFile(".gitlab-ci.yml", "test:\n  script: task verify\n")}},
	})
}

// The default branch comes from refs/remotes/origin/HEAD when the clone
// has one; otherwise the report says which branch it assumed.
func TestRuleC01DefaultBranch(t *testing.T) {
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
