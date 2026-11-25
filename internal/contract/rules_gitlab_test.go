package contract_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The CI rules on GitLab CI, through contract.Audit (spec 0055, "The
// GitLab pipeline model" and "The rules on GitLab CI"). The GitLab
// baseline is the contract baseline with its GitHub workflows replaced by
// glBase; each case changes one thing.

const gitlabFile = ".gitlab-ci.yml"

// glGitHubCI and glGitHubAudit are the baseline's GitHub workflows, which
// gitlab removes.
const (
	glGitHubCI    = ".github/workflows/ci.yml"
	glGitHubAudit = ".github/workflows/audit.yml"
)

// glImage is an image pinned by digest, as the baseline uses it.
const glImage = "golang:1.25@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// glBase passes C-01 and C-04: verify on a Linux and a Windows runner, a
// scan, and the audit in a job of its own, none with rules.
const glBase = "default:\n  image: " + glImage + "\n\n" +
	"verify:linux:\n  tags: [linux]\n  script:\n    - task verify\n\n" +
	"verify:windows:\n  tags: [windows]\n  script:\n    - task verify\n\n" +
	"scan:\n  script:\n    - go install golang.org/x/vuln/cmd/govulncheck@v1.1.4\n    - govulncheck ./...\n\n" +
	"conformance:audit:\n  script:\n    - vibe audit --static\n"

// gitlab turns the baseline into a GitLab-only repository whose
// .gitlab-ci.yml is pipeline, plus any further edits.
func gitlab(pipeline string, more ...contracttest.Edit) []contracttest.Edit {
	return append([]contracttest.Edit{contracttest.WithoutFile(glGitHubCI), contracttest.WithoutFile(glGitHubAudit), contracttest.WithFile(gitlabFile, pipeline)}, more...)
}

// glVerify is a verify job with the given extra lines (indented two
// spaces, newline-terminated) before its script.
func glVerify(id, extra string) string {
	return id + ":\n" + extra + "  script:\n    - task verify\n"
}

// glAudit is the audit job with the given extra lines before its script.
func glAudit(extra string) string {
	return "conformance:audit:\n" + extra + "  script:\n    - vibe audit --static\n"
}

// glOnly is a pipeline of the given jobs and the baseline's audit job.
func glOnly(jobs string) string { return jobs + "\n" + glAudit("") }

// glRules is a rules: block of the given if: conditions, one rule each.
func glRules(conds ...string) string {
	s := "  rules:\n"
	for _, c := range conds {
		s += "    - if: '" + c + "'\n"
	}
	return s
}

const (
	glMR      = `$CI_PIPELINE_SOURCE == "merge_request_event"`
	glDefault = `$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH`
)

// The provider the C rules read (spec 0055, Q1): GitHub workflows win;
// .gitlab-ci.yml is read only without them; other providers cannot be
// decided. The report names the provider read.
func TestCIProvider(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name     string
		provider contract.CIProvider
		c01      contract.Result
		Edits    []contracttest.Edit
		Findings []string
	}{
		{Name: "GitHub only", provider: "github_actions", c01: "pass"},
		{Name: "GitLab only", provider: "gitlab_ci", c01: "pass", Edits: gitlab(glBase)},
		{Name: "both: GitHub wins over a GitLab file that would fail", provider: "github_actions", c01: "pass",
			Edits: []contracttest.Edit{contracttest.WithFile(gitlabFile, "test:\n  script: echo no verify\n")}},
		{Name: "both: GitHub wins over a GitLab file that does not parse", provider: "github_actions", c01: "pass",
			Edits: []contracttest.Edit{contracttest.WithFile(gitlabFile, "test: [\n")}},
		{Name: "an untracked .gitlab-ci.yml is not CI", c01: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile(glGitHubCI), contracttest.WithoutFile(glGitHubAudit), contracttest.WithUntracked(gitlabFile, glBase)},
			Findings: []string{"no CI configuration", "Fix: add a GitHub Actions workflow under .github/workflows/ or a GitLab CI configuration (.gitlab-ci.yml)"}},
		{Name: "another provider cannot be decided", c01: "undecided",
			Edits:    []contracttest.Edit{contracttest.WithoutFile(glGitHubCI), contracttest.WithoutFile(glGitHubAudit), contracttest.WithFile(".circleci/config.yml", "version: 2.1\n")},
			Findings: []string{".circleci/config.yml: ", "GitHub Actions", "GitLab CI"}},
		{Name: "another provider beside GitLab: GitLab is read", provider: "gitlab_ci", c01: "pass",
			Edits: gitlab(glBase, contracttest.WithFile("Jenkinsfile", "pipeline {}\n"))},
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline, tc.Edits...), contract.Options{Static: true})
			contracttest.AssertRule(t, rep, "C-01", tc.c01, tc.Findings...)
			if rep.CIProvider != tc.provider {
				t.Errorf("ci_provider = %q, want %q", rep.CIProvider, tc.provider)
			}
			// The name the text report prints after "CI provider read:";
			// the CLI's TestContractTextReportCIProvider prints it.
			want := map[contract.CIProvider]string{contract.CIGitHub: "GitHub Actions", contract.CIGitLab: "GitLab CI"}[tc.provider]
			if want != "" && rep.CIProvider.Name() != want {
				t.Errorf("provider name = %q, want %q", rep.CIProvider.Name(), want)
			}
		})
	}
}

// C-01 on GitLab CI: jobs that run in merge request pipelines, and jobs
// that run on pushes to the default branch, call verify.
func TestRuleC01GitLab(t *testing.T) {
	t.Parallel()
	v := func(extra string) []contracttest.Edit { return gitlab(glOnly(glVerify("verify", extra))) }
	rules := func(conds ...string) []contracttest.Edit { return v(glRules(conds...)) }
	ciRuleCases(t, goldenGitLab, "C-01", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass", Edits: gitlab(glBase)},
		{Name: "a job with no rules runs in both kinds", Want: "pass", Edits: v("")},
		{Name: "verify inside a condition", Want: "pass",
			Edits: gitlab(glOnly("verify:\n  script:\n    - if true; then task verify; fi\n"))},
		{Name: "a script as one string", Want: "pass", Edits: gitlab(glOnly("verify:\n  script: task verify\n"))},
		{Name: "a multi-line script", Want: "pass",
			Edits: gitlab(glOnly("verify:\n  script:\n    - |\n      go mod download\n      task verify\n"))},
		{Name: "verify with an extra argument is not called", Want: "fail",
			Edits: gitlab(glOnly("verify:\n  script:\n    - task verify --force\n"))},
		{Name: "a script line that does not parse calls nothing", Want: "fail",
			Edits: gitlab(glOnly("verify:\n  script:\n    - task verify 'unclosed\n"))},
		{Name: "no verify anywhere", Want: "fail", Edits: gitlab(glOnly("lint:\n  script: echo\n")),
			Findings: []string{".gitlab-ci.yml: ", "`task verify`", "merge request pipelines", "pushes to main"}},
		{Name: "list verify, commands in different jobs", Want: "pass", Edits: append(gitlab(glOnly(
			"lint:\n  script: task lint\ntest:\n  script:\n    - go mod download && task test\n")), ciListVerify)},
		{Name: "list verify, one command not called", Want: "fail",
			Edits:    append(gitlab(glOnly("lint:\n  script: task lint\n")), ciListVerify),
			Findings: []string{"`task test`"}},

		// Commands: before_script and script count, after_script does not.
		{Name: "before_script counts", Want: "pass",
			Edits: gitlab(glOnly("verify:\n  before_script: [task verify]\n  script: [echo done]\n"))},
		{Name: "after_script does not count", Want: "fail",
			Edits:    gitlab(glOnly("verify:\n  script: [echo start]\n  after_script: [task verify]\n")),
			Findings: []string{"merge request pipelines", "`task verify`"}},
		{Name: "allow_failure is ignored", Want: "pass", Edits: v("  allow_failure: true\n")},

		// Rules for the two pipeline kinds.
		{Name: "merge requests and the default branch", Want: "pass", Edits: rules(glMR, glDefault)},
		{Name: "merge requests only", Want: "fail", Edits: rules(glMR),
			Findings: []string{"pushes to main", "job verify does not count for pushes to main", "no rule admits it"}},
		{Name: "the default branch only", Want: "fail", Edits: rules(glDefault),
			Findings: []string{"merge request pipelines"}},
		{Name: "$CI_MERGE_REQUEST_IID and a literal default branch", Want: "pass",
			Edits: rules("$CI_MERGE_REQUEST_IID", `$CI_COMMIT_BRANCH == "main"`)},
		{Name: "$CI_COMMIT_REF_NAME against the default branch", Want: "pass",
			Edits: rules(glMR, `$CI_COMMIT_REF_NAME == $CI_DEFAULT_BRANCH`)},
		{Name: "the default branch written first", Want: "pass",
			Edits: rules(glMR, `$CI_DEFAULT_BRANCH == $CI_COMMIT_BRANCH`)},
		{Name: "another literal branch", Want: "fail", Edits: rules(glMR, `$CI_COMMIT_BRANCH == "develop"`),
			Findings: []string{"pushes to main"}},
		{Name: "push pipelines (branch pipelines show on the merge request)", Want: "pass",
			Edits: rules(`$CI_PIPELINE_SOURCE == "push"`)},
		{Name: "every branch", Want: "pass", Edits: rules("$CI_COMMIT_BRANCH")},
		{Name: "tags only", Want: "fail", Edits: rules("$CI_COMMIT_TAG"),
			Findings: []string{"merge request pipelines", "pushes to main"}},
		{Name: "schedules only", Want: "fail", Edits: rules(`$CI_PIPELINE_SOURCE == "schedule"`)},
		{Name: "not on schedules", Want: "pass", Edits: rules(`$CI_PIPELINE_SOURCE != "schedule"`)},
		{Name: "&&, || and parentheses", Want: "pass", Edits: rules(
			`$CI_PIPELINE_SOURCE == "merge_request_event" || ($CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH)`)},
		{Name: "an unread condition that the rest decides", Want: "pass", Edits: v(
			"  rules:\n    - if: '$CI_COMMIT_TAG && $DEPLOY == \"yes\"'\n      when: never\n    - when: on_success\n")},
		{Name: "when: never excludes tags", Want: "pass",
			Edits: v("  rules:\n    - if: $CI_COMMIT_TAG\n      when: never\n    - when: always\n")},
		{Name: "when: never excludes the default branch", Want: "fail",
			Edits:    v("  rules:\n    - if: '" + glDefault + "'\n      when: never\n    - when: always\n"),
			Findings: []string{"pushes to main", "when: never"}},
		{Name: "a condition the audit does not read is quoted with its line", Want: "fail",
			Edits: rules(`$CI_COMMIT_MESSAGE =~ /verify/`),
			Findings: []string{".gitlab-ci.yml: line 3: job verify does not count for merge request pipelines",
				"`$CI_COMMIT_MESSAGE =~ /verify/`", "does not read"}},
		{Name: "a variable the audit does not read", Want: "fail", Edits: rules(`$RUN_VERIFY == "true"`),
			Findings: []string{"`$RUN_VERIFY == \"true\"`"}},
		{Name: "changes: does not count", Want: "fail",
			Edits:    v("  rules:\n    - if: '" + glMR + "'\n      changes: [go.mod]\n    - if: '" + glDefault + "'\n"),
			Findings: []string{"line 4: job verify does not count for merge request pipelines", "changes:"}},
		{Name: "exists: does not count", Want: "fail",
			Edits:    v("  rules:\n    - exists: [go.mod]\n"),
			Findings: []string{"exists:"}},
		{Name: "changes: in a rule that does not match is not read", Want: "pass",
			Edits: v("  rules:\n    - if: $CI_COMMIT_TAG\n      changes: [go.mod]\n    - when: on_success\n")},

		// An unread rule without when: never is passed over: had it matched,
		// the job would run anyway. One with when: never might exclude it.
		{Name: "an unread rule before admitting rules is passed over", Want: "pass",
			Edits: v("  rules:\n    - if: $FOO\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n")},
		{Name: "an unread rule with when: never does not admit the job", Want: "fail",
			Edits:    v("  rules:\n    - if: $FOO\n      when: never\n    - when: always\n"),
			Findings: []string{"line 3: job verify does not count for merge request pipelines", "`$FOO`"}},
		{Name: "changes: followed by an admitting rule counts", Want: "pass",
			Edits: v("  rules:\n    - changes: [go.mod]\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n")},
		{Name: "workflow rules: an unread rule before an admitting one is passed over", Want: "pass", Edits: gitlab(
			"workflow:\n  rules:\n    - if: $FOO\n    - when: always\n" + glOnly(glVerify("verify", "")))},
		{Name: "workflow rules: an unread rule with when: never", Want: "fail", Edits: gitlab(
			"workflow:\n  rules:\n    - if: $FOO\n      when: never\n    - when: always\n" + glOnly(glVerify("verify", ""))),
			Findings: []string{"line 3: ", "workflow:rules: if: `$FOO`"}},
		{Name: "rules from a !reference", Want: "pass",
			Edits: gitlab(glOnly(".rules:\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" +
				glVerify("verify", "  rules:\n    - !reference [.rules, rules]\n")))},

		// workflow:rules decide whether the pipeline exists.
		{Name: "workflow rules for both kinds", Want: "pass", Edits: gitlab(
			"workflow:\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" + glVerify("verify", glRules(glMR, glDefault)))},
		{Name: "workflow rules for both kinds, a job without rules is not in merge request pipelines", Want: "fail", Edits: gitlab(
			"workflow:\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" + glVerify("verify", "")),
			Findings: []string{"line 6: job verify does not count for merge request pipelines", "branch and tag pipelines only"}},
		{Name: "workflow rules for merge requests only", Want: "fail", Edits: gitlab(
			"workflow:\n  rules:\n    - if: '" + glMR + "'\n" + glOnly(glVerify("verify", ""))),
			Findings: []string{"pushes to main", "workflow:rules: no rule admits it"}},
		{Name: "workflow rules with a condition the audit does not read", Want: "fail", Edits: gitlab(
			"workflow:\n  rules:\n    - if: $CI_OPEN_MERGE_REQUESTS\n" + glOnly(glVerify("verify", ""))),
			Findings: []string{"line 3: ", "workflow:rules: if: `$CI_OPEN_MERGE_REQUESTS`"}},

		// only and except.
		{Name: "only merge_requests", Want: "fail", Edits: v("  only: [merge_requests]\n"),
			Findings: []string{"pushes to main", "only: does not admit it"}},
		{Name: "only merge_requests and the default branch", Want: "pass", Edits: v("  only: [merge_requests, main]\n")},
		{Name: "only branches", Want: "pass", Edits: v("  only: [branches]\n")},
		{Name: "only pushes", Want: "pass", Edits: v("  only: [pushes]\n")},
		{Name: "only tags", Want: "fail", Edits: v("  only: [tags]\n")},
		{Name: "only schedules", Want: "fail", Edits: v("  only: [schedules]\n")},
		{Name: "except the default branch", Want: "fail", Edits: v("  except: [main]\n"),
			Findings: []string{"pushes to main", "except: excludes it"}},
		{Name: "except tags", Want: "pass", Edits: v("  except: [tags]\n")},
		{Name: "except merge_requests still runs in branch pipelines", Want: "pass", Edits: v("  except: [merge_requests]\n")},
		{Name: "only a regular expression", Want: "fail", Edits: v("  only: ['/^release/']\n"),
			Findings: []string{"`/^release/`"}},
		{Name: "only refs with changes", Want: "fail", Edits: v("  only:\n    refs: [branches]\n    changes: [go.mod]\n"),
			Findings: []string{"changes:"}},
		{Name: "only refs as a map", Want: "pass", Edits: v("  only:\n    refs: [branches, merge_requests]\n")},
		{Name: "a job that never runs", Want: "fail", Edits: v("  when: never\n")},

		// Anchors, merge keys, templates, extends, default and !reference.
		{Name: "a hidden job is a template, not a job", Want: "fail", Edits: gitlab(glOnly(glVerify(".verify", "")))},
		{Name: "extends a template", Want: "pass", Edits: gitlab(glOnly(glVerify(".verify", "") + "verify:\n  extends: .verify\n"))},
		{Name: "extends several templates, later ones win", Want: "fail", Edits: gitlab(glOnly(
			glVerify(".a", "") + ".b:\n  script: [echo b]\nverify:\n  extends: [.a, .b]\n"))},
		{Name: "extends deep-merges maps", Want: "pass", Edits: gitlab(glOnly(
			".base:\n  variables:\n    A: '1'\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" +
				"verify:\n  extends: .base\n  variables:\n    B: '2'\n  script: [task verify]\n"))},
		{Name: "extends in a chain", Want: "pass", Edits: gitlab(glOnly(
			glVerify(".a", "") + ".b:\n  extends: .a\n.c:\n  extends: .b\nverify:\n  extends: .c\n"))},
		{Name: "extends a job that is not defined", Want: "fail", Edits: gitlab(glOnly("verify:\n  extends: .missing\n  script: [task verify]\n")),
			Findings: []string{".gitlab-ci.yml: ", "line 2: job verify extends .missing, which is not defined"}},
		{Name: "an extends cycle", Want: "fail", Edits: gitlab(glOnly(
			".a:\n  extends: .b\n  script: [task verify]\n.b:\n  extends: .a\nverify:\n  extends: .a\n")),
			Findings: []string{".gitlab-ci.yml: ", "extends cycle"}},
		{Name: "extends nested within the limit", Want: "pass", Edits: gitlab(glOnly(glExtendsChain(11)))},
		{Name: "extends nested beyond the limit", Want: "fail", Edits: gitlab(glOnly(glExtendsChain(12))),
			Findings: []string{"extends nests more than 11 levels"}},
		{Name: "default before_script", Want: "pass",
			Edits: gitlab("default:\n  before_script: [task verify]\n" + glOnly("build:\n  script: [echo build]\n"))},
		{Name: "a job's own before_script replaces the default", Want: "fail",
			Edits: gitlab("default:\n  before_script: [task verify]\nbuild:\n  before_script: [echo]\n  script: [echo build]\n")},
		{Name: "inherit default false", Want: "fail",
			Edits: gitlab("default:\n  before_script: [task verify]\nbuild:\n  inherit:\n    default: false\n  script: [echo build]\n")},
		{Name: "inherit a list of default keys", Want: "pass",
			Edits: gitlab("default:\n  before_script: [task verify]\n" + glOnly("build:\n  inherit:\n    default: [before_script]\n  script: [echo build]\n"))},
		{Name: "a top-level before_script is a default", Want: "pass",
			Edits: gitlab("before_script: [task verify]\n" + glOnly("build:\n  script: [echo build]\n"))},
		{Name: "an anchor", Want: "pass",
			Edits: gitlab(glOnly(".scripts:\n  verify: &verify\n    - task verify\nverify:\n  script: *verify\n"))},
		{Name: "a merge key", Want: "pass",
			Edits: gitlab(glOnly(".base: &base\n  script: [task verify]\nverify:\n  <<: *base\n"))},
		{Name: "a merge key the job overrides", Want: "fail",
			Edits: gitlab(glOnly(".base: &base\n  script: [task verify]\nverify:\n  <<: *base\n  script: [echo]\n"))},
		{Name: "merge keys from a list", Want: "pass", Edits: gitlab(glOnly(
			".a: &a\n  script: [task verify]\n.b: &b\n  script: [echo]\n  tags: [linux]\nverify:\n  <<: [*a, *b]\n"))},
		{Name: "!reference to a script", Want: "pass",
			Edits: gitlab(glOnly(".tpl:\n  script: [task verify]\nverify:\n  script:\n    - echo start\n    - !reference [.tpl, script]\n"))},
		{Name: "!reference nested in a referenced key", Want: "pass", Edits: gitlab(glOnly(
			".a:\n  script: [task verify]\n.b:\n  script:\n    - !reference [.a, script]\nverify:\n  script:\n    - !reference [.b, script]\n"))},
		{Name: "!reference that points at nothing", Want: "fail",
			Edits:    gitlab(glOnly("verify:\n  script:\n    - !reference [.tpl, script]\n")),
			Findings: []string{".gitlab-ci.yml: ", "line 3: ", "!reference [.tpl, script] points at nothing"}},
		{Name: "!reference that refers to itself", Want: "fail",
			Edits:    gitlab(glOnly(".a:\n  script:\n    - !reference [.a, script]\nverify:\n  script: [task verify]\n")),
			Findings: []string{"refers to itself"}},

		// Local includes.
		{Name: "a local include", Want: "pass",
			Edits: gitlab("include:\n  - local: ci/verify.yml\n"+glOnly(""), contracttest.WithFile("ci/verify.yml", glVerify("verify", "")))},
		{Name: "a local include as a string", Want: "pass",
			Edits: gitlab("include: /ci/verify.yml\n"+glOnly(""), contracttest.WithFile("ci/verify.yml", glVerify("verify", "")))},
		{Name: "a nested local include", Want: "pass", Edits: gitlab("include: ci/a.yml\n"+glOnly(""),
			contracttest.WithFile("ci/a.yml", "include: ci/b.yml\n"), contracttest.WithFile("ci/b.yml", glVerify("verify", "")))},
		{Name: "a wildcard include", Want: "pass", Edits: gitlab("include: 'ci/*.yml'\n"+glOnly(""),
			contracttest.WithFile("ci/lint.yml", "lint:\n  script: [echo]\n"), contracttest.WithFile("ci/verify.yml", glVerify("verify", "")))},
		{Name: "a single star does not cross directories", Want: "fail", Edits: gitlab("include: 'ci/*.yml'\n"+glOnly(""),
			contracttest.WithFile("ci/lint.yml", "lint:\n  script: [echo]\n"), contracttest.WithFile("ci/sub/verify.yml", glVerify("verify", "")))},
		{Name: "a double star crosses directories", Want: "pass", Edits: gitlab("include: 'ci/**.yml'\n"+glOnly(""),
			contracttest.WithFile("ci/sub/verify.yml", glVerify("verify", "")))},
		{Name: "the including file wins over the included one", Want: "fail",
			Edits: gitlab("include: ci/verify.yml\n"+glOnly("verify:\n  script: [echo overridden]\n"),
				contracttest.WithFile("ci/verify.yml", glVerify("verify", "")))},
		{Name: "an included job extends a template of the including file", Want: "pass",
			Edits: gitlab("include: ci/verify.yml\n"+glOnly(glVerify(".verify", "")),
				contracttest.WithFile("ci/verify.yml", "verify:\n  extends: .verify\n"))},
		{Name: "a local include that names no file", Want: "fail",
			Edits:    gitlab("include: ci/missing.yml\n" + glBase),
			Findings: []string{".gitlab-ci.yml: ", "line 1: include local: ci/missing.yml names no tracked file"}},
		{Name: "an untracked include", Want: "fail",
			Edits:    gitlab("include: ci/verify.yml\n"+glBase, contracttest.WithUntracked("ci/verify.yml", "x:\n  script: [echo]\n")),
			Findings: []string{"names no tracked file"}},
		{Name: "an include cycle", Want: "fail", Edits: gitlab("include: ci/a.yml\n"+glBase,
			contracttest.WithFile("ci/a.yml", "include: ci/b.yml\n"), contracttest.WithFile("ci/b.yml", "include: ci/a.yml\n")),
			Findings: []string{"include cycle"}},
		{Name: "an included file that does not parse", Want: "fail",
			Edits:    gitlab("include: ci/a.yml\n"+glBase, contracttest.WithFile("ci/a.yml", "x: [\n")),
			Findings: []string{"ci/a.yml: ", "does not parse"}},
		{Name: "an include whose rules admit only merge requests", Want: "fail", Edits: gitlab(
			"include:\n  - local: ci/verify.yml\n    rules:\n      - if: '"+glMR+"'\n"+glOnly(""),
			contracttest.WithFile("ci/verify.yml", glVerify("verify", ""))),
			Findings: []string{"pushes to main"}},
		{Name: "an include whose rules admit both kinds", Want: "pass", Edits: gitlab(
			"include:\n  - local: ci/verify.yml\n    rules:\n      - if: '"+glMR+"'\n      - if: $CI_COMMIT_BRANCH\n"+glOnly(""),
			contracttest.WithFile("ci/verify.yml", glVerify("verify", "")))},
		{Name: "an include rule the audit does not read is quoted with its line", Want: "fail", Edits: gitlab(
			"include:\n  - local: ci/verify.yml\n    rules:\n      - if: $FOO\n"+glOnly(""),
			contracttest.WithFile("ci/verify.yml", glVerify("verify", ""))),
			Findings: []string{".gitlab-ci.yml: line 4: job verify does not count for merge request pipelines",
				"include rules: if: `$FOO`", "job verify does not count for pushes to main"}},
		{Name: "an include rule with changes: is quoted with its line", Want: "fail", Edits: gitlab(
			"include:\n  - local: ci/verify.yml\n    rules:\n      - changes: [go.mod]\n"+glOnly(""),
			contracttest.WithFile("ci/verify.yml", glVerify("verify", ""))),
			Findings: []string{".gitlab-ci.yml: line 4: job verify does not count for merge request pipelines", "changes:"}},
		{Name: "an unread include rule before an admitting one is passed over", Want: "pass", Edits: gitlab(
			"include:\n  - local: ci/verify.yml\n    rules:\n      - if: $FOO\n      - when: always\n"+glOnly(""),
			contracttest.WithFile("ci/verify.yml", glVerify("verify", "")))},
		{Name: "a project include is not followed", Want: "fail", Edits: gitlab(
			"include:\n  - project: group/ci\n    ref: 08eba0b27e820071cde6df949e0beb9ba4906955\n    file: verify.yml\n" + glOnly(""))},
		{Name: "a remote include is not followed", Want: "fail", Edits: gitlab(
			"include: https://example.com/verify.yml\n" + glOnly(""))},

		// Child pipelines.
		{Name: "a child pipeline the parent waits for", Want: "pass", Edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			contracttest.WithFile("ci/child.yml", glVerify("verify", "")))},
		{Name: "a child pipeline with strategy mirror", Want: "pass", Edits: gitlab(
			glOnly("child:\n  trigger:\n    include:\n      - local: ci/child.yml\n    strategy: mirror\n"),
			contracttest.WithFile("ci/child.yml", glVerify("verify", "")))},
		{Name: "a child pipeline the parent does not wait for", Want: "fail", Edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n"),
			contracttest.WithFile("ci/child.yml", glVerify("verify", "")))},
		{Name: "the trigger job's rules are the outer condition", Want: "fail", Edits: gitlab(
			glOnly("child:\n"+glRules(glMR)+"  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			contracttest.WithFile("ci/child.yml", glVerify("verify", ""))),
			Findings: []string{"ci/child.yml", "pushes to main", "its trigger job child does not run"}},
		{Name: "a child's workflow rules for parent pipelines", Want: "pass", Edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			contracttest.WithFile("ci/child.yml", "workflow:\n  rules:\n    - if: '$CI_PIPELINE_SOURCE == \"parent_pipeline\"'\n"+glVerify("verify", "")))},
		{Name: "a child's jobs gated to merge request events", Want: "fail", Edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			contracttest.WithFile("ci/child.yml", glVerify("verify", glRules(glMR))))},
		{Name: "a child pipeline file that names no file", Want: "fail", Edits: gitlab(
			glBase + "child:\n  trigger:\n    include: ci/missing.yml\n    strategy: depend\n"),
			Findings: []string{"names no tracked file"}},
		{Name: "a child pipeline from another project is not followed", Want: "fail", Edits: gitlab(
			glOnly("child:\n  trigger:\n    include:\n      - project: group/ci\n        file: verify.yml\n    strategy: depend\n"))},
		{Name: "a dynamic child pipeline is not followed", Want: "fail", Edits: gitlab(
			glOnly("child:\n  trigger:\n    include:\n      - artifact: generated.yml\n        job: generate\n    strategy: depend\n"))},

		// Failures.
		{Name: "a .gitlab-ci.yml that does not parse", Want: "fail", Edits: gitlab("verify: [\n"),
			Findings: []string{".gitlab-ci.yml: ", "does not parse"}},
		{Name: "a .gitlab-ci.yml that is not a map", Want: "fail", Edits: gitlab("- a\n- b\n"),
			Findings: []string{"must be a map"}},
		{Name: "an empty .gitlab-ci.yml", Want: "fail", Edits: gitlab(""),
			Findings: []string{"the file is empty"}},
		{Name: "a job that is not a map", Want: "fail", Edits: gitlab(glBase + "broken: 3\n"),
			Findings: []string{"job broken must be a map"}},
		{Name: "a script that is not a string or a list", Want: "fail", Edits: gitlab(glBase + "broken:\n  script: {a: b}\n"),
			Findings: []string{"job broken: script must be a string or a list of strings"}},
		{Name: "verify missing points at V-04", Want: "fail", Edits: append(gitlab(glBase), ciNoVerify),
			Findings: []string{"vibe.yaml: ", "V-04"}},
	})
}

// glExtendsChain is a verify job at the end of a chain of n extends.
func glExtendsChain(n int) string {
	s := glVerify(".t0", "")
	for i := 1; i < n; i++ {
		s += ".t" + strings.Repeat("x", i) + ":\n  extends: " + glLink(i-1) + "\n"
	}
	return s + "verify:\n  extends: " + glLink(n-1) + "\n"
}

func glLink(i int) string {
	if i == 0 {
		return ".t0"
	}
	return ".t" + strings.Repeat("x", i)
}

// The default branch on GitLab CI: origin/HEAD, or else main or master,
// the one the conditions name more often, main on a tie.
func TestRuleC01GitLabDefaultBranch(t *testing.T) {
	t.Parallel()
	onBranch := func(b string) []contracttest.Edit {
		return gitlab(glOnly(glVerify("verify", glRules(glMR, `$CI_COMMIT_BRANCH == "`+b+`"`))))
	}
	t.Run("origin/HEAD names trunk", func(t *testing.T) {
		dir := contracttest.GitFixture(t, contracttest.Baseline, onBranch("trunk")...)
		contracttest.Git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
		rep := contracttest.Audit(t, dir, contract.Options{Static: true})
		contracttest.AssertRule(t, rep, "C-01", contract.ResultPass)
	})
	t.Run("master is assumed when the conditions name it more often", func(t *testing.T) {
		dir := contracttest.GitFixture(t, contracttest.Baseline, gitlab(glOnly(glVerify("verify", glRules(glMR, `$CI_COMMIT_BRANCH == "master"`))+
			glVerify("v2", glRules(`$CI_COMMIT_BRANCH == "master"`))))...)
		rep := contracttest.Audit(t, dir, contract.Options{Static: true})
		if !slices.ContainsFunc(rep.Notes, func(n string) bool { return strings.Contains(n, "assume the default branch is master") }) {
			t.Errorf("report does not say master was assumed: notes %q", rep.Notes)
		}
	})
	t.Run("main on a tie", func(t *testing.T) {
		tie := gitlab(glVerify("verify", glRules(glMR, `$CI_COMMIT_BRANCH == "master"`)) +
			glAudit(glRules(glMR, `$CI_COMMIT_BRANCH == "main"`)))
		rep := contracttest.Audit(t, contracttest.GitFixture(t, contracttest.Baseline, tie...), contract.Options{Static: true})
		contracttest.AssertRule(t, rep, "C-01", contract.ResultFail, "pushes to main")
	})
}

// C-04 on GitLab CI: a job that runs in merge request pipelines runs vibe
// audit and calls no verify command.
func TestRuleC04GitLab(t *testing.T) {
	t.Parallel()
	verify := glVerify("verify:linux", "") + glVerify("verify:windows", "")
	audit := func(extra, script string) []contracttest.Edit {
		return gitlab(verify + "conformance:audit:\n" + extra + "  script:\n" + script)
	}
	ciRuleCases(t, goldenGitLab, "C-04", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass", Edits: gitlab(glBase)},
		{Name: "merge requests only", Want: "pass", Edits: audit(glRules(glMR), "    - vibe audit\n")},
		{Name: "after building vibe, in one line", Want: "pass",
			Edits: audit("", "    - go build -o \"$HOME/bin/vibe\" ./cmd/vibe && vibe audit --json\n")},
		{Name: "with leading assignments", Want: "pass", Edits: audit("", "    - NO_COLOR=1 vibe audit\n")},
		{Name: "the audit job also calls verify", Want: "fail", Edits: audit("", "    - vibe audit\n    - task verify\n"),
			Findings: []string{".gitlab-ci.yml: ", "job conformance:audit", "`task verify`", "own"}},
		{Name: "verify in the audit job's before_script", Want: "fail",
			Edits: audit("  before_script: [task verify]\n", "    - vibe audit\n")},
		{Name: "verify in the audit job's after_script does not count", Want: "pass",
			Edits: audit("  after_script: [task verify]\n", "    - vibe audit\n")},
		{Name: "vibe audit in after_script does not count", Want: "fail",
			Edits: audit("  after_script: [vibe audit]\n", "    - echo\n")},
		{Name: "no audit job", Want: "fail", Edits: gitlab(verify),
			Findings: []string{".gitlab-ci.yml: ", "no job that runs in merge request pipelines runs vibe audit"}},
		{Name: "the default branch only", Want: "fail", Edits: audit(glRules(glDefault), "    - vibe audit\n"),
			Findings: []string{"job conformance:audit does not count for merge request pipelines", "no rule admits it"}},
		{Name: "tags only", Want: "fail", Edits: audit("  only: [tags]\n", "    - vibe audit\n")},
		{Name: "a condition the audit does not read", Want: "fail",
			Edits:    audit(glRules(`$CI_MERGE_REQUEST_LABELS =~ /audit/`), "    - vibe audit\n"),
			Findings: []string{"`$CI_MERGE_REQUEST_LABELS =~ /audit/`", "line "}},
		{Name: "changes: does not count", Want: "fail",
			Edits:    audit("  rules:\n    - changes: [vibe.yaml]\n", "    - vibe audit\n"),
			Findings: []string{"changes:"}},
		{Name: "in an include whose rule the audit does not read", Want: "fail", Edits: gitlab(
			"include:\n  - local: ci/audit.yml\n    rules:\n      - if: $FOO\n"+verify,
			contracttest.WithFile("ci/audit.yml", glAudit(""))),
			Findings: []string{".gitlab-ci.yml: line 4: job conformance:audit does not count for merge request pipelines",
				"include rules: if: `$FOO`"}},
		{Name: "in a child pipeline the parent waits for", Want: "pass", Edits: gitlab(
			verify+"audit:\n  trigger:\n    include: ci/audit.yml\n    strategy: depend\n",
			contracttest.WithFile("ci/audit.yml", glAudit("")))},
		{Name: "in a child pipeline the parent does not wait for", Want: "fail", Edits: gitlab(
			verify+"audit:\n  trigger:\n    include: ci/audit.yml\n",
			contracttest.WithFile("ci/audit.yml", glAudit("")))},
		{Name: "from a template", Want: "pass", Edits: gitlab(verify + ".audit:\n  script: [vibe audit]\nconformance:audit:\n  extends: .audit\n")},
		{Name: "vibe without audit", Want: "fail", Edits: audit("", "    - vibe doctor\n")},
		{Name: "vibe audit by path is not vibe audit", Want: "fail", Edits: audit("", "    - ./bin/vibe audit\n")},
		{Name: "verify missing points at V-04", Want: "fail", Edits: append(gitlab(glBase), ciNoVerify),
			Findings: []string{"vibe.yaml: ", "V-04"}},
		{Name: "a .gitlab-ci.yml that does not parse", Want: "fail", Edits: gitlab("x: [\n"),
			Findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// glSHA is a full commit SHA, as a pinned include names it.
const glSHA = "08eba0b27e820071cde6df949e0beb9ba4906955"

// glUnpinned is an image without a digest.
const glUnpinned = "golang:1.25"

// glWithJob is the baseline with one more job.
func glWithJob(job string) []contracttest.Edit { return gitlab(glBase + "\n" + job) }

// glInclude is the baseline with a top-level include list.
func glInclude(entries string) []contracttest.Edit { return gitlab("include:\n" + entries + glBase) }

// C-02 on GitLab CI: images and services pinned by digest, unfollowed
// includes pinned, no template: include, no @latest in any script.
func TestRuleC02GitLab(t *testing.T) {
	t.Parallel()
	run := func(key, line string) []contracttest.Edit {
		return glWithJob("x:\n  " + key + ":\n    - " + line + "\n  script: [echo]\n")
	}
	ciRuleCases(t, goldenGitLab, "C-02", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass", Edits: gitlab(glBase)},

		// Images and services.
		{Name: "a job image by tag", Want: "fail", Edits: glWithJob("x:\n  image: " + glUnpinned + "\n  script: [echo]\n"),
			Findings: []string{".gitlab-ci.yml: ", "line 24: ", "job x", "image golang:1.25", "@sha256:"}},
		{Name: "a job image pinned", Want: "pass", Edits: glWithJob("x:\n  image: " + glImage + "\n  script: [echo]\n")},
		{Name: "a job image in name: form by tag", Want: "fail",
			Edits:    glWithJob("x:\n  image:\n    name: " + glUnpinned + "\n    entrypoint: ['']\n  script: [echo]\n"),
			Findings: []string{"image golang:1.25"}},
		{Name: "a job image in name: form pinned", Want: "pass",
			Edits: glWithJob("x:\n  image:\n    name: " + glImage + "\n  script: [echo]\n")},
		{Name: "a default image by tag", Want: "fail",
			Edits:    gitlab(strings.Replace(glBase, glImage, glUnpinned, 1)),
			Findings: []string{".gitlab-ci.yml: ", "line 2: ", "image golang:1.25"}},
		{Name: "a short digest", Want: "fail",
			Edits: glWithJob("x:\n  image: golang:1.25@sha256:0123abcd\n  script: [echo]\n")},
		{Name: "a job service by tag", Want: "fail",
			Edits:    glWithJob("x:\n  services:\n    - postgres:16\n  script: [echo]\n"),
			Findings: []string{"services postgres:16"}},
		{Name: "a job service in name: form by tag", Want: "fail",
			Edits:    glWithJob("x:\n  services:\n    - name: postgres:16\n      alias: db\n  script: [echo]\n"),
			Findings: []string{"services postgres:16"}},
		{Name: "a job service pinned", Want: "pass",
			Edits: glWithJob("x:\n  services:\n    - name: postgres:16@sha256:" + strings.Repeat("ab", 32) + "\n  script: [echo]\n")},
		{Name: "a default service by tag", Want: "fail",
			Edits:    gitlab("default:\n  services: [redis:7]\n" + glBase[len("default:\n"):]),
			Findings: []string{"services redis:7"}},
		{Name: "an image from a template", Want: "fail",
			Edits:    glWithJob(".tpl:\n  image: " + glUnpinned + "\nx:\n  extends: .tpl\n  script: [echo]\n"),
			Findings: []string{"image golang:1.25"}},
		{Name: "an unpinned image in a template no job uses", Want: "pass",
			Edits: glWithJob(".tpl:\n  image: " + glUnpinned + "\n")},
		{Name: "an image in a local include", Want: "fail", Edits: gitlab("include: ci/x.yml\n"+glBase,
			contracttest.WithFile("ci/x.yml", "x:\n  image: "+glUnpinned+"\n  script: [echo]\n")),
			Findings: []string{"ci/x.yml: ", "line 2: "}},
		{Name: "an image in a child pipeline", Want: "fail", Edits: gitlab(
			glBase+"child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n",
			contracttest.WithFile("ci/child.yml", "x:\n  image: "+glUnpinned+"\n  script: [echo]\n")),
			Findings: []string{"ci/child.yml: ", "image golang:1.25"}},

		// Includes that are not followed.
		{Name: "a project include pinned", Want: "pass",
			Edits: glInclude("  - project: group/ci\n    ref: " + glSHA + "\n    file: verify.yml\n")},
		{Name: "a project include by tag", Want: "fail",
			Edits:    glInclude("  - project: group/ci\n    ref: v1.2.3\n    file: verify.yml\n"),
			Findings: []string{".gitlab-ci.yml: ", "line 2: ", "project group/ci", "40-character"}},
		{Name: "a project include without ref", Want: "fail",
			Edits: glInclude("  - project: group/ci\n    file: verify.yml\n")},
		{Name: "a component pinned", Want: "pass",
			Edits: glInclude("  - component: $CI_SERVER_FQDN/group/lint/golangci@" + glSHA + "\n")},
		{Name: "a component by version", Want: "fail",
			Edits:    glInclude("  - component: $CI_SERVER_FQDN/group/lint/golangci@1.2.0\n"),
			Findings: []string{"component $CI_SERVER_FQDN/group/lint/golangci@1.2.0"}},
		{Name: "a component at ~latest", Want: "fail",
			Edits: glInclude("  - component: gitlab.com/group/lint/golangci@~latest\n")},
		{Name: "a remote include with integrity", Want: "pass",
			Edits: glInclude("  - remote: https://example.com/ci.yml\n    integrity: sha256-" + strings.Repeat("A", 43) + "=\n")},
		{Name: "a remote include without integrity", Want: "fail",
			Edits:    glInclude("  - remote: https://example.com/ci.yml\n"),
			Findings: []string{"remote https://example.com/ci.yml", "integrity:"}},
		{Name: "a remote include as a string", Want: "fail", Edits: glInclude("  - https://example.com/ci.yml\n")},
		{Name: "a template include cannot be pinned", Want: "fail",
			Edits:    glInclude("  - template: Jobs/SAST.gitlab-ci.yml\n"),
			Findings: []string{"template Jobs/SAST.gitlab-ci.yml", "cannot be pinned"}},
		{Name: "an unpinned include in a local include", Want: "fail", Edits: gitlab("include: ci/x.yml\n"+glBase,
			contracttest.WithFile("ci/x.yml", "include:\n  - project: group/ci\n    file: x.yml\n")),
			Findings: []string{"ci/x.yml: "}},
		{Name: "an unpinned include in a trigger", Want: "fail", Edits: glWithJob(
			"child:\n  trigger:\n    include:\n      - project: group/ci\n        file: child.yml\n    strategy: depend\n")},

		// @latest in scripts.
		{Name: "@latest in script", Want: "fail", Edits: run("script", "go install golang.org/x/tools/cmd/goimports@latest"),
			Findings: []string{".gitlab-ci.yml: ", "job x, script", "goimports@latest", "pin"}},
		{Name: "@latest in before_script", Want: "fail", Edits: run("before_script", "npx prettier@latest --check ."),
			Findings: []string{"job x, before_script", "prettier@latest"}},
		{Name: "@latest in after_script", Want: "fail", Edits: run("after_script", "npx prettier@latest --check ."),
			Findings: []string{"job x, after_script"}},
		{Name: "a pinned version", Want: "pass", Edits: run("script", "go install golang.org/x/tools/cmd/goimports@v0.30.0")},
		{Name: "@latest inside a word is not a version", Want: "pass", Edits: run("script", "echo user@latest.example")},
		{Name: "a .gitlab-ci.yml that does not parse", Want: "fail", Edits: gitlab("x: [\n"),
			Findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// C-03 on GitLab CI is not applicable: job tokens have no per-file
// permissions.
func TestRuleC03GitLab(t *testing.T) {
	t.Parallel()
	ciRuleCases(t, goldenGitLab, "C-03", []contracttest.RuleCase{
		{Name: "GitLab only", Want: "not_applicable", Edits: gitlab(glBase),
			Findings: []string{"not applicable", "GitLab", "job tokens have no per-file permissions", "project setting"}},
		{Name: "even when .gitlab-ci.yml does not parse", Want: "not_applicable", Edits: gitlab("x: [\n")},
		{Name: "both providers: GitHub is read", Want: "pass", Edits: []contracttest.Edit{contracttest.WithFile(gitlabFile, glBase)}},
	})
}

// C-05 on GitLab CI: platforms from tags after parallel: matrix expansion;
// no tags and an image is Linux.
func TestRuleC05GitLab(t *testing.T) {
	t.Parallel()
	only := func(jobs string) []contracttest.Edit { return gitlab(glOnly(jobs)) }
	matrix := func(tags, entries string) []contracttest.Edit {
		return only(glVerify("verify", "  parallel:\n    matrix:\n"+entries+"  tags: "+tags+"\n"))
	}
	ciRuleCases(t, goldenGitLab, "C-05", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass", Edits: gitlab(glBase)},
		{Name: "tags ignore case and match inside a tag", Want: "pass", Edits: only(
			glVerify("a", "  tags: [saas-Linux-small-amd64]\n") + glVerify("b", "  tags: [SAAS-WINDOWS-MEDIUM-AMD64]\n"))},
		{Name: "untagged with an image is Linux", Want: "pass", Edits: only(
			glVerify("a", "  image: "+glImage+"\n") + glVerify("b", "  tags: [windows]\n"))},
		{Name: "untagged with a default image is Linux", Want: "pass",
			Edits: gitlab("default:\n  image: " + glImage + "\n" + glOnly(glVerify("a", "")+glVerify("b", "  tags: [windows]\n")))},
		{Name: "untagged without an image is unknown", Want: "fail", Edits: only(
			glVerify("a", "") + glVerify("b", "  tags: [windows]\n")),
			Findings: []string{".gitlab-ci.yml: ", "`task verify`", "no Linux runner", "job a", "no tags and no image"}},
		{Name: "a tag that names no platform is unknown", Want: "fail", Edits: only(
			glVerify("a", "  tags: [docker]\n") + glVerify("b", "  tags: [windows]\n")),
			Findings: []string{"no Linux runner", "docker"}},
		{Name: "a tag beside a platform tag", Want: "pass", Edits: only(
			glVerify("a", "  tags: [docker, linux]\n") + glVerify("b", "  tags: [windows]\n"))},
		{Name: "Linux only", Want: "fail", Edits: only(glVerify("a", "  tags: [linux]\n")),
			Findings: []string{"no Windows runner", "tags: [windows]"}},
		{Name: "macOS and Windows", Want: "fail", Edits: only(
			glVerify("a", "  tags: [saas-macos-medium-m1]\n") + glVerify("b", "  tags: [windows]\n")),
			Findings: []string{"no Linux runner"}},
		{Name: "a matrix over tags", Want: "pass", Edits: matrix("['${RUNNER}']", "      - RUNNER: [linux, windows]\n")},
		{Name: "a matrix with $VAR", Want: "pass", Edits: matrix("[$RUNNER]", "      - RUNNER: [linux, windows]\n")},
		{Name: "a matrix of several entries", Want: "pass",
			Edits: matrix("[$OS]", "      - OS: linux\n        GO: ['1.24', '1.25']\n      - OS: windows\n")},
		{Name: "a matrix that leaves Windows out", Want: "fail",
			Edits: matrix("[$OS]", "      - OS: linux\n        GO: ['1.24', '1.25']\n")},
		{Name: "a tag variable the matrix does not set is unknown", Want: "fail",
			Edits:    matrix("[$RUNNER]", "      - OS: [linux, windows]\n"),
			Findings: []string{"$RUNNER"}},
		{Name: "after_script does not count", Want: "fail", Edits: only(glVerify("a", "  tags: [linux]\n") +
			"b:\n  tags: [windows]\n  script: [echo]\n  after_script: [task verify]\n")},
		{Name: "tags from default:", Want: "pass",
			Edits: gitlab("default:\n  tags: [windows]\n" + glOnly(glVerify("a", "  tags: [linux]\n")+glVerify("b", "")))},
		{Name: "list verify, one command on Linux only", Want: "fail", Edits: append(only(
			"lint:\n  tags: [linux]\n  script: [task lint]\n"+
				"test:\n  parallel:\n    matrix:\n      - T: [linux, windows]\n  tags: [$T]\n  script: [task test]\n"), ciListVerify),
			Findings: []string{"`task lint`"}},
		{Name: "verify called nowhere", Want: "fail", Edits: only("x:\n  tags: [linux]\n  script: [echo]\n"),
			Findings: []string{".gitlab-ci.yml: ", "no job calls"}},
		{Name: "not applicable without windows", Want: "not_applicable",
			Edits: gitlab(glOnly(glVerify("a", "  tags: [linux]\n")),
				contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: [linux]\n"})))},
		{Name: "a .gitlab-ci.yml that does not parse", Want: "fail", Edits: gitlab("x: [\n"),
			Findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// D-03 on GitLab CI: a scanner in any script, or GitLab's
// Dependency-Scanning template or component.
func TestRuleD03GitLab(t *testing.T) {
	t.Parallel()
	noScan := strings.Replace(glBase,
		"scan:\n  script:\n    - go install golang.org/x/vuln/cmd/govulncheck@v1.1.4\n    - govulncheck ./...\n\n", "", 1)
	with := func(more string) []contracttest.Edit { return gitlab(noScan + more) }
	include := func(entry string) []contracttest.Edit { return gitlab("include:\n" + entry + noScan) }
	ciRuleCases(t, goldenGitLab, "D-03", []contracttest.RuleCase{
		{Name: "baseline govulncheck", Want: "pass", Edits: gitlab(glBase)},
		{Name: "pip-audit in before_script", Want: "pass", Edits: with("scan:\n  before_script: [pip-audit]\n  script: [echo]\n")},
		{Name: "trivy in after_script", Want: "pass", Edits: with("scan:\n  script: [echo]\n  after_script: [trivy fs .]\n")},
		{Name: "a scanner in a child pipeline", Want: "pass", Edits: gitlab(
			noScan+"child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n",
			contracttest.WithFile("ci/child.yml", "scan:\n  script: [osv-scanner scan -r .]\n"))},
		{Name: "a scanner in a job with rules", Want: "pass",
			Edits: with("scan:\n  rules:\n    - if: '$CI_PIPELINE_SOURCE == \"schedule\"'\n  script: [npm audit]\n")},
		{Name: "the Dependency-Scanning template", Want: "pass",
			Edits: include("  - template: Jobs/Dependency-Scanning.gitlab-ci.yml\n")},
		{Name: "the Security Dependency-Scanning template", Want: "pass",
			Edits: include("  - template: Security/Dependency-Scanning.latest.gitlab-ci.yml\n")},
		{Name: "another template", Want: "fail", Edits: include("  - template: Jobs/SAST.gitlab-ci.yml\n")},
		{Name: "the dependency-scanning component", Want: "pass",
			Edits: include("  - component: $CI_SERVER_FQDN/components/dependency-scanning/main@" + glSHA + "\n")},
		{Name: "the dependency-scanning component unpinned is still recognized", Want: "pass",
			Edits: include("  - component: gitlab.com/components/dependency-scanning/main@0\n")},
		{Name: "another component", Want: "fail", Edits: include("  - component: gitlab.com/components/sast/main@" + glSHA + "\n")},
		{Name: "installing a scanner is not running it", Want: "fail",
			Edits: with("scan:\n  script: [go install golang.org/x/vuln/cmd/govulncheck@v1.1.4]\n")},
		{Name: "no scanner", Want: "fail", Edits: gitlab(noScan),
			Findings: []string{".gitlab-ci.yml: ", "govulncheck", "Dependency-Scanning"}},
		{Name: "a .gitlab-ci.yml that does not parse", Want: "fail", Edits: gitlab("x: [\n"),
			Findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// The Dependency-Scanning template is still a template: C-02 fails it.
func TestRuleD03GitLabTemplateFailsC02(t *testing.T) {
	t.Parallel()
	rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline,
		glInclude("  - template: Jobs/Dependency-Scanning.gitlab-ci.yml\n")...), contract.Options{Static: true})
	contracttest.AssertRule(t, rep, "D-03", contract.ResultPass)
	contracttest.AssertRule(t, rep, "C-02", contract.ResultFail, "template Jobs/Dependency-Scanning.gitlab-ci.yml")
}

// R-01 on GitLab CI: every job's scripts, after_script included, except the
// C-04 audit job's; vibe --version in a job that calls no entrypoint.
func TestRuleR01GitLab(t *testing.T) {
	t.Parallel()
	ciPartCases(t, goldenGitLab, "R-01", contract.Static, []contracttest.RuleCase{
		{Name: "baseline", Want: "pass", Edits: gitlab(glBase)},
		{Name: "vibe in a script", Want: "fail", Edits: glWithJob("sync:\n  script:\n    - vibe sync && git diff --exit-code\n"),
			Findings: []string{".gitlab-ci.yml: ", "line 25: job sync, script", "`vibe sync`"}},
		{Name: "vibe in before_script", Want: "fail", Edits: glWithJob("sync:\n  before_script: [vibe sync]\n  script: [echo]\n"),
			Findings: []string{"job sync, before_script"}},
		{Name: "vibe in after_script", Want: "fail", Edits: glWithJob("sync:\n  script: [echo]\n  after_script: [vibe doctor]\n"),
			Findings: []string{"job sync, after_script", "`vibe doctor`"}},
		{Name: "vibe in a default before_script", Want: "fail",
			Edits:    gitlab("default:\n  before_script: [vibe sync]\n" + glBase[len("default:\n"):]),
			Findings: []string{"`vibe sync`"}},
		{Name: "other vibe commands in the audit job", Want: "pass",
			Edits: gitlab(strings.Replace(glBase, "    - vibe audit --static\n", "    - vibe doctor\n    - vibe audit --static\n", 1))},
		{Name: "vibe in the audit job's after_script", Want: "pass",
			Edits: gitlab(glBase + "  after_script: [vibe doctor]\n")},
		{Name: "vibe audit in a job that also calls verify", Want: "fail",
			Edits:    gitlab(strings.Replace(glBase, "    - vibe audit --static\n", "    - vibe audit --static\n    - task verify\n", 1)),
			Findings: []string{"job conformance:audit, script", "`vibe audit --static`"}},
		{Name: "vibe audit in a job that does not run in merge requests", Want: "fail",
			Edits: gitlab(strings.Replace(glBase, "conformance:audit:\n", "conformance:audit:\n"+glRules(glDefault), 1))},
		{Name: "the audit job in a child pipeline", Want: "pass", Edits: gitlab(
			strings.Replace(glBase, glAudit(""), "", 1)+"audit:\n  trigger:\n    include: ci/audit.yml\n    strategy: depend\n",
			contracttest.WithFile("ci/audit.yml", glAudit("  after_script: [vibe doctor]\n")))},
		{Name: "vibe in a child pipeline", Want: "fail", Edits: gitlab(
			glBase+"child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n",
			contracttest.WithFile("ci/child.yml", "sync:\n  script: [vibe sync]\n")),
			Findings: []string{"ci/child.yml: ", "`vibe sync`"}},
		{Name: "vibe by path is not a call of vibe", Want: "pass",
			Edits: glWithJob("sync:\n  script: ['\"$CI_PROJECT_DIR/bin/vibe\" sync']\n")},
		// vibe --version tests an installed binary (#120 Part A).
		{Name: "vibe --version in a job that calls no entrypoint", Want: "pass",
			Edits: glWithJob("install:\n  script:\n    - go install ./cmd/vibe\n    - vibe --version | tee version.txt\n")},
		{Name: "vibe --version in a job that calls verify", Want: "fail",
			Edits:    glWithJob("v:\n  before_script: [vibe --version]\n  script: [task verify]\n"),
			Findings: []string{"job v, before_script", "`vibe --version`"}},
		{Name: "vibe --version in a job that calls verify in after_script", Want: "fail",
			Edits:    glWithJob("v:\n  script: [vibe --version]\n  after_script: [task verify]\n"),
			Findings: []string{"`vibe --version`"}},
		{Name: "vibe --version with more words is a call", Want: "fail",
			Edits: glWithJob("install:\n  script: [vibe --version --json]\n")},
		{Name: "a .gitlab-ci.yml that does not parse", Want: "fail", Edits: gitlab("x: [\n"),
			Findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// A GitLab-only repository: the baseline decides every C rule, D-03 and
// R-01's static part, C-03 is not applicable, and the report names GitLab
// CI (spec 0055, acceptance criterion 2).
func TestGitLabBaseline(t *testing.T) {
	t.Parallel()
	rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline, gitlab(glBase)...), contract.Options{Static: true})
	for _, id := range []string{"C-01", "C-02", "C-04", "C-05", "D-03"} {
		contracttest.AssertRule(t, rep, id, contract.ResultPass)
	}
	contracttest.AssertRule(t, rep, "C-03", contract.ResultNotApplicable, "GitLab")
	contracttest.AssertPart(t, rep, "R-01", contract.Static, contract.ResultPass)
	if rep.CIProvider != contract.CIGitLab {
		t.Errorf("ci_provider = %q, want gitlab_ci", rep.CIProvider)
	}
	// The text report names it as "CI provider read: GitLab CI"; the CLI's
	// TestContractTextReportCIProvider prints it.
	if name := rep.CIProvider.Name(); name != "GitLab CI" {
		t.Errorf("provider name = %q, want GitLab CI", name)
	}
}

// A repository with both GitHub workflows and a .gitlab-ci.yml gets the
// results of the GitHub baseline, whatever the GitLab file says (spec
// 0055, acceptance criterion 4).
func TestBothProvidersReadAsGitHub(t *testing.T) {
	t.Parallel()
	static := contract.Options{Static: true}
	github := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline), static)
	bad := "include:\n  - template: Jobs/SAST.gitlab-ci.yml\n" +
		"x:\n  image: golang:1.25\n  services: [redis:7]\n  tags: [docker]\n  script: [vibe sync, go install a/b@latest]\n"
	both := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile(gitlabFile, bad)), static)
	for _, id := range []string{"C-01", "C-02", "C-03", "C-04", "C-05", "D-03", "R-01"} {
		g, b := contracttest.RuleResult(t, github, id), contracttest.RuleResult(t, both, id)
		gt, bt := contracttest.FindingText(g), contracttest.FindingText(b)
		if g.Result != b.Result || gt != bt {
			t.Errorf("%s with both providers = %s %q, want the GitHub result %s %q", id, b.Result, bt, g.Result, gt)
		}
	}
}
