package cli

import (
	"strings"
	"testing"
)

// The CI rules on GitLab CI, through the vibe audit seam (spec 0055, "The
// GitLab pipeline model" and "The rules on GitLab CI"). The GitLab
// baseline is the contract baseline with its GitHub workflows replaced by
// glBase; each case changes one thing.

const gitlabFile = ".gitlab-ci.yml"

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
func gitlab(pipeline string, more ...fixtureEdit) []fixtureEdit {
	return append([]fixtureEdit{withoutFile(ciFile), withoutFile(auditFile), withFile(gitlabFile, pipeline)}, more...)
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
		name, provider, c01 string
		edits               []fixtureEdit
		findings            []string
	}{
		{name: "GitHub only", provider: "github_actions", c01: "pass"},
		{name: "GitLab only", provider: "gitlab_ci", c01: "pass", edits: gitlab(glBase)},
		{name: "both: GitHub wins over a GitLab file that would fail", provider: "github_actions", c01: "pass",
			edits: []fixtureEdit{withFile(gitlabFile, "test:\n  script: echo no verify\n")}},
		{name: "both: GitHub wins over a GitLab file that does not parse", provider: "github_actions", c01: "pass",
			edits: []fixtureEdit{withFile(gitlabFile, "test: [\n")}},
		{name: "an untracked .gitlab-ci.yml is not CI", c01: "fail",
			edits:    []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile), withUntracked(gitlabFile, glBase)},
			findings: []string{"no CI configuration"}},
		{name: "another provider cannot be decided", c01: "undecided",
			edits:    []fixtureEdit{withoutFile(ciFile), withoutFile(auditFile), withFile(".circleci/config.yml", "version: 2.1\n")},
			findings: []string{".circleci/config.yml: ", "GitHub Actions", "GitLab CI"}},
		{name: "another provider beside GitLab: GitLab is read", provider: "gitlab_ci", c01: "pass",
			edits: gitlab(glBase, withFile("Jenkinsfile", "pipeline {}\n"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := contractFixture(t, tc.edits...)
			rep, _ := auditJSON(t, dir, "--static")
			assertRule(t, rep, "C-01", tc.c01, tc.findings...)
			if rep.CIProvider != tc.provider {
				t.Errorf("ci_provider = %q, want %q", rep.CIProvider, tc.provider)
			}
			text := runVibeAudit(t, "--repo-root", dir, "--static").stdout
			want := map[string]string{"github_actions": "GitHub Actions", "gitlab_ci": "GitLab CI"}[tc.provider]
			if line := "CI provider read: " + want + "\n"; want != "" && !strings.Contains(text, line) {
				t.Errorf("text report does not say %q:\n%s", line, text)
			}
			if want == "" && strings.Contains(text, "CI provider read:") {
				t.Errorf("text report names a provider although none was read:\n%s", text)
			}
		})
	}
}

// C-01 on GitLab CI: jobs that run in merge request pipelines, and jobs
// that run on pushes to the default branch, call verify.
func TestRuleC01GitLab(t *testing.T) {
	t.Parallel()
	v := func(extra string) []fixtureEdit { return gitlab(glOnly(glVerify("verify", extra))) }
	rules := func(conds ...string) []fixtureEdit { return v(glRules(conds...)) }
	runRuleCases(t, "C-01", []ruleCase{
		{name: "baseline", want: "pass", edits: gitlab(glBase)},
		{name: "a job with no rules runs in both kinds", want: "pass", edits: v("")},
		{name: "verify inside a condition", want: "pass",
			edits: gitlab(glOnly("verify:\n  script:\n    - if true; then task verify; fi\n"))},
		{name: "a script as one string", want: "pass", edits: gitlab(glOnly("verify:\n  script: task verify\n"))},
		{name: "a multi-line script", want: "pass",
			edits: gitlab(glOnly("verify:\n  script:\n    - |\n      go mod download\n      task verify\n"))},
		{name: "verify with an extra argument is not called", want: "fail",
			edits: gitlab(glOnly("verify:\n  script:\n    - task verify --force\n"))},
		{name: "a script line that does not parse calls nothing", want: "fail",
			edits: gitlab(glOnly("verify:\n  script:\n    - task verify 'unclosed\n"))},
		{name: "no verify anywhere", want: "fail", edits: gitlab(glOnly("lint:\n  script: echo\n")),
			findings: []string{".gitlab-ci.yml: ", "`task verify`", "merge request pipelines", "pushes to main"}},
		{name: "list verify, commands in different jobs", want: "pass", edits: append(gitlab(glOnly(
			"lint:\n  script: task lint\ntest:\n  script:\n    - go mod download && task test\n")), listVerify)},
		{name: "list verify, one command not called", want: "fail",
			edits:    append(gitlab(glOnly("lint:\n  script: task lint\n")), listVerify),
			findings: []string{"`task test`"}},

		// Commands: before_script and script count, after_script does not.
		{name: "before_script counts", want: "pass",
			edits: gitlab(glOnly("verify:\n  before_script: [task verify]\n  script: [echo done]\n"))},
		{name: "after_script does not count", want: "fail",
			edits:    gitlab(glOnly("verify:\n  script: [echo start]\n  after_script: [task verify]\n")),
			findings: []string{"merge request pipelines", "`task verify`"}},
		{name: "allow_failure is ignored", want: "pass", edits: v("  allow_failure: true\n")},

		// Rules for the two pipeline kinds.
		{name: "merge requests and the default branch", want: "pass", edits: rules(glMR, glDefault)},
		{name: "merge requests only", want: "fail", edits: rules(glMR),
			findings: []string{"pushes to main", "job verify does not count for pushes to main", "no rule admits it"}},
		{name: "the default branch only", want: "fail", edits: rules(glDefault),
			findings: []string{"merge request pipelines"}},
		{name: "$CI_MERGE_REQUEST_IID and a literal default branch", want: "pass",
			edits: rules("$CI_MERGE_REQUEST_IID", `$CI_COMMIT_BRANCH == "main"`)},
		{name: "$CI_COMMIT_REF_NAME against the default branch", want: "pass",
			edits: rules(glMR, `$CI_COMMIT_REF_NAME == $CI_DEFAULT_BRANCH`)},
		{name: "the default branch written first", want: "pass",
			edits: rules(glMR, `$CI_DEFAULT_BRANCH == $CI_COMMIT_BRANCH`)},
		{name: "another literal branch", want: "fail", edits: rules(glMR, `$CI_COMMIT_BRANCH == "develop"`),
			findings: []string{"pushes to main"}},
		{name: "push pipelines (branch pipelines show on the merge request)", want: "pass",
			edits: rules(`$CI_PIPELINE_SOURCE == "push"`)},
		{name: "every branch", want: "pass", edits: rules("$CI_COMMIT_BRANCH")},
		{name: "tags only", want: "fail", edits: rules("$CI_COMMIT_TAG"),
			findings: []string{"merge request pipelines", "pushes to main"}},
		{name: "schedules only", want: "fail", edits: rules(`$CI_PIPELINE_SOURCE == "schedule"`)},
		{name: "not on schedules", want: "pass", edits: rules(`$CI_PIPELINE_SOURCE != "schedule"`)},
		{name: "&&, || and parentheses", want: "pass", edits: rules(
			`$CI_PIPELINE_SOURCE == "merge_request_event" || ($CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH)`)},
		{name: "an unread condition that the rest decides", want: "pass", edits: v(
			"  rules:\n    - if: '$CI_COMMIT_TAG && $DEPLOY == \"yes\"'\n      when: never\n    - when: on_success\n")},
		{name: "when: never excludes tags", want: "pass",
			edits: v("  rules:\n    - if: $CI_COMMIT_TAG\n      when: never\n    - when: always\n")},
		{name: "when: never excludes the default branch", want: "fail",
			edits:    v("  rules:\n    - if: '" + glDefault + "'\n      when: never\n    - when: always\n"),
			findings: []string{"pushes to main", "when: never"}},
		{name: "a condition the audit does not read is quoted with its line", want: "fail",
			edits: rules(`$CI_COMMIT_MESSAGE =~ /verify/`),
			findings: []string{".gitlab-ci.yml: line 3: job verify does not count for merge request pipelines",
				"`$CI_COMMIT_MESSAGE =~ /verify/`", "does not read"}},
		{name: "a variable the audit does not read", want: "fail", edits: rules(`$RUN_VERIFY == "true"`),
			findings: []string{"`$RUN_VERIFY == \"true\"`"}},
		{name: "changes: does not count", want: "fail",
			edits:    v("  rules:\n    - if: '" + glMR + "'\n      changes: [go.mod]\n    - if: '" + glDefault + "'\n"),
			findings: []string{"line 4: job verify does not count for merge request pipelines", "changes:"}},
		{name: "exists: does not count", want: "fail",
			edits:    v("  rules:\n    - exists: [go.mod]\n"),
			findings: []string{"exists:"}},
		{name: "changes: in a rule that does not match is not read", want: "pass",
			edits: v("  rules:\n    - if: $CI_COMMIT_TAG\n      changes: [go.mod]\n    - when: on_success\n")},
		{name: "rules from a !reference", want: "pass",
			edits: gitlab(glOnly(".rules:\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" +
				glVerify("verify", "  rules:\n    - !reference [.rules, rules]\n")))},

		// workflow:rules decide whether the pipeline exists.
		{name: "workflow rules for both kinds", want: "pass", edits: gitlab(
			"workflow:\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" + glVerify("verify", glRules(glMR, glDefault)))},
		{name: "workflow rules for both kinds, a job without rules is not in merge request pipelines", want: "fail", edits: gitlab(
			"workflow:\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" + glVerify("verify", "")),
			findings: []string{"line 6: job verify does not count for merge request pipelines", "branch and tag pipelines only"}},
		{name: "workflow rules for merge requests only", want: "fail", edits: gitlab(
			"workflow:\n  rules:\n    - if: '" + glMR + "'\n" + glOnly(glVerify("verify", ""))),
			findings: []string{"pushes to main", "workflow:rules: no rule admits it"}},
		{name: "workflow rules with a condition the audit does not read", want: "fail", edits: gitlab(
			"workflow:\n  rules:\n    - if: $CI_OPEN_MERGE_REQUESTS\n" + glOnly(glVerify("verify", ""))),
			findings: []string{"line 3: ", "workflow:rules: if: `$CI_OPEN_MERGE_REQUESTS`"}},

		// only and except.
		{name: "only merge_requests", want: "fail", edits: v("  only: [merge_requests]\n"),
			findings: []string{"pushes to main", "only: does not admit it"}},
		{name: "only merge_requests and the default branch", want: "pass", edits: v("  only: [merge_requests, main]\n")},
		{name: "only branches", want: "pass", edits: v("  only: [branches]\n")},
		{name: "only pushes", want: "pass", edits: v("  only: [pushes]\n")},
		{name: "only tags", want: "fail", edits: v("  only: [tags]\n")},
		{name: "only schedules", want: "fail", edits: v("  only: [schedules]\n")},
		{name: "except the default branch", want: "fail", edits: v("  except: [main]\n"),
			findings: []string{"pushes to main", "except: excludes it"}},
		{name: "except tags", want: "pass", edits: v("  except: [tags]\n")},
		{name: "except merge_requests still runs in branch pipelines", want: "pass", edits: v("  except: [merge_requests]\n")},
		{name: "only a regular expression", want: "fail", edits: v("  only: ['/^release/']\n"),
			findings: []string{"`/^release/`"}},
		{name: "only refs with changes", want: "fail", edits: v("  only:\n    refs: [branches]\n    changes: [go.mod]\n"),
			findings: []string{"changes:"}},
		{name: "only refs as a map", want: "pass", edits: v("  only:\n    refs: [branches, merge_requests]\n")},
		{name: "a job that never runs", want: "fail", edits: v("  when: never\n")},

		// Anchors, merge keys, templates, extends, default and !reference.
		{name: "a hidden job is a template, not a job", want: "fail", edits: gitlab(glOnly(glVerify(".verify", "")))},
		{name: "extends a template", want: "pass", edits: gitlab(glOnly(glVerify(".verify", "") + "verify:\n  extends: .verify\n"))},
		{name: "extends several templates, later ones win", want: "fail", edits: gitlab(glOnly(
			glVerify(".a", "") + ".b:\n  script: [echo b]\nverify:\n  extends: [.a, .b]\n"))},
		{name: "extends deep-merges maps", want: "pass", edits: gitlab(glOnly(
			".base:\n  variables:\n    A: '1'\n  rules:\n    - if: '" + glMR + "'\n    - if: '" + glDefault + "'\n" +
				"verify:\n  extends: .base\n  variables:\n    B: '2'\n  script: [task verify]\n"))},
		{name: "extends in a chain", want: "pass", edits: gitlab(glOnly(
			glVerify(".a", "") + ".b:\n  extends: .a\n.c:\n  extends: .b\nverify:\n  extends: .c\n"))},
		{name: "extends a job that is not defined", want: "fail", edits: gitlab(glOnly("verify:\n  extends: .missing\n  script: [task verify]\n")),
			findings: []string{".gitlab-ci.yml: ", "line 2: job verify extends .missing, which is not defined"}},
		{name: "an extends cycle", want: "fail", edits: gitlab(glOnly(
			".a:\n  extends: .b\n  script: [task verify]\n.b:\n  extends: .a\nverify:\n  extends: .a\n")),
			findings: []string{".gitlab-ci.yml: ", "extends cycle"}},
		{name: "extends nested within the limit", want: "pass", edits: gitlab(glOnly(glExtendsChain(11)))},
		{name: "extends nested beyond the limit", want: "fail", edits: gitlab(glOnly(glExtendsChain(12))),
			findings: []string{"extends nests more than 11 levels"}},
		{name: "default before_script", want: "pass",
			edits: gitlab("default:\n  before_script: [task verify]\n" + glOnly("build:\n  script: [echo build]\n"))},
		{name: "a job's own before_script replaces the default", want: "fail",
			edits: gitlab("default:\n  before_script: [task verify]\nbuild:\n  before_script: [echo]\n  script: [echo build]\n")},
		{name: "inherit default false", want: "fail",
			edits: gitlab("default:\n  before_script: [task verify]\nbuild:\n  inherit:\n    default: false\n  script: [echo build]\n")},
		{name: "inherit a list of default keys", want: "pass",
			edits: gitlab("default:\n  before_script: [task verify]\n" + glOnly("build:\n  inherit:\n    default: [before_script]\n  script: [echo build]\n"))},
		{name: "a top-level before_script is a default", want: "pass",
			edits: gitlab("before_script: [task verify]\n" + glOnly("build:\n  script: [echo build]\n"))},
		{name: "an anchor", want: "pass",
			edits: gitlab(glOnly(".scripts:\n  verify: &verify\n    - task verify\nverify:\n  script: *verify\n"))},
		{name: "a merge key", want: "pass",
			edits: gitlab(glOnly(".base: &base\n  script: [task verify]\nverify:\n  <<: *base\n"))},
		{name: "a merge key the job overrides", want: "fail",
			edits: gitlab(glOnly(".base: &base\n  script: [task verify]\nverify:\n  <<: *base\n  script: [echo]\n"))},
		{name: "merge keys from a list", want: "pass", edits: gitlab(glOnly(
			".a: &a\n  script: [task verify]\n.b: &b\n  script: [echo]\n  tags: [linux]\nverify:\n  <<: [*a, *b]\n"))},
		{name: "!reference to a script", want: "pass",
			edits: gitlab(glOnly(".tpl:\n  script: [task verify]\nverify:\n  script:\n    - echo start\n    - !reference [.tpl, script]\n"))},
		{name: "!reference nested in a referenced key", want: "pass", edits: gitlab(glOnly(
			".a:\n  script: [task verify]\n.b:\n  script:\n    - !reference [.a, script]\nverify:\n  script:\n    - !reference [.b, script]\n"))},
		{name: "!reference that points at nothing", want: "fail",
			edits:    gitlab(glOnly("verify:\n  script:\n    - !reference [.tpl, script]\n")),
			findings: []string{".gitlab-ci.yml: ", "line 3: ", "!reference [.tpl, script] points at nothing"}},
		{name: "!reference that refers to itself", want: "fail",
			edits:    gitlab(glOnly(".a:\n  script:\n    - !reference [.a, script]\nverify:\n  script: [task verify]\n")),
			findings: []string{"refers to itself"}},

		// Local includes.
		{name: "a local include", want: "pass",
			edits: gitlab("include:\n  - local: ci/verify.yml\n"+glOnly(""), withFile("ci/verify.yml", glVerify("verify", "")))},
		{name: "a local include as a string", want: "pass",
			edits: gitlab("include: /ci/verify.yml\n"+glOnly(""), withFile("ci/verify.yml", glVerify("verify", "")))},
		{name: "a nested local include", want: "pass", edits: gitlab("include: ci/a.yml\n"+glOnly(""),
			withFile("ci/a.yml", "include: ci/b.yml\n"), withFile("ci/b.yml", glVerify("verify", "")))},
		{name: "a wildcard include", want: "pass", edits: gitlab("include: 'ci/*.yml'\n"+glOnly(""),
			withFile("ci/lint.yml", "lint:\n  script: [echo]\n"), withFile("ci/verify.yml", glVerify("verify", "")))},
		{name: "a single star does not cross directories", want: "fail", edits: gitlab("include: 'ci/*.yml'\n"+glOnly(""),
			withFile("ci/lint.yml", "lint:\n  script: [echo]\n"), withFile("ci/sub/verify.yml", glVerify("verify", "")))},
		{name: "a double star crosses directories", want: "pass", edits: gitlab("include: 'ci/**.yml'\n"+glOnly(""),
			withFile("ci/sub/verify.yml", glVerify("verify", "")))},
		{name: "the including file wins over the included one", want: "fail",
			edits: gitlab("include: ci/verify.yml\n"+glOnly("verify:\n  script: [echo overridden]\n"),
				withFile("ci/verify.yml", glVerify("verify", "")))},
		{name: "an included job extends a template of the including file", want: "pass",
			edits: gitlab("include: ci/verify.yml\n"+glOnly(glVerify(".verify", "")),
				withFile("ci/verify.yml", "verify:\n  extends: .verify\n"))},
		{name: "a local include that names no file", want: "fail",
			edits:    gitlab("include: ci/missing.yml\n" + glBase),
			findings: []string{".gitlab-ci.yml: ", "line 1: include local: ci/missing.yml names no tracked file"}},
		{name: "an untracked include", want: "fail",
			edits:    gitlab("include: ci/verify.yml\n"+glBase, withUntracked("ci/verify.yml", "x:\n  script: [echo]\n")),
			findings: []string{"names no tracked file"}},
		{name: "an include cycle", want: "fail", edits: gitlab("include: ci/a.yml\n"+glBase,
			withFile("ci/a.yml", "include: ci/b.yml\n"), withFile("ci/b.yml", "include: ci/a.yml\n")),
			findings: []string{"include cycle"}},
		{name: "an included file that does not parse", want: "fail",
			edits:    gitlab("include: ci/a.yml\n"+glBase, withFile("ci/a.yml", "x: [\n")),
			findings: []string{"ci/a.yml: ", "does not parse"}},
		{name: "an include whose rules admit only merge requests", want: "fail", edits: gitlab(
			"include:\n  - local: ci/verify.yml\n    rules:\n      - if: '"+glMR+"'\n"+glOnly(""),
			withFile("ci/verify.yml", glVerify("verify", ""))),
			findings: []string{"pushes to main"}},
		{name: "an include whose rules admit both kinds", want: "pass", edits: gitlab(
			"include:\n  - local: ci/verify.yml\n    rules:\n      - if: '"+glMR+"'\n      - if: $CI_COMMIT_BRANCH\n"+glOnly(""),
			withFile("ci/verify.yml", glVerify("verify", "")))},
		{name: "a project include is not followed", want: "fail", edits: gitlab(
			"include:\n  - project: group/ci\n    ref: 08eba0b27e820071cde6df949e0beb9ba4906955\n    file: verify.yml\n" + glOnly(""))},
		{name: "a remote include is not followed", want: "fail", edits: gitlab(
			"include: https://example.com/verify.yml\n" + glOnly(""))},

		// Child pipelines.
		{name: "a child pipeline the parent waits for", want: "pass", edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			withFile("ci/child.yml", glVerify("verify", "")))},
		{name: "a child pipeline with strategy mirror", want: "pass", edits: gitlab(
			glOnly("child:\n  trigger:\n    include:\n      - local: ci/child.yml\n    strategy: mirror\n"),
			withFile("ci/child.yml", glVerify("verify", "")))},
		{name: "a child pipeline the parent does not wait for", want: "fail", edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n"),
			withFile("ci/child.yml", glVerify("verify", "")))},
		{name: "the trigger job's rules are the outer condition", want: "fail", edits: gitlab(
			glOnly("child:\n"+glRules(glMR)+"  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			withFile("ci/child.yml", glVerify("verify", ""))),
			findings: []string{"ci/child.yml", "pushes to main", "its trigger job child does not run"}},
		{name: "a child's workflow rules for parent pipelines", want: "pass", edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			withFile("ci/child.yml", "workflow:\n  rules:\n    - if: '$CI_PIPELINE_SOURCE == \"parent_pipeline\"'\n"+glVerify("verify", "")))},
		{name: "a child's jobs gated to merge request events", want: "fail", edits: gitlab(
			glOnly("child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n"),
			withFile("ci/child.yml", glVerify("verify", glRules(glMR))))},
		{name: "a child pipeline file that names no file", want: "fail", edits: gitlab(
			glBase + "child:\n  trigger:\n    include: ci/missing.yml\n    strategy: depend\n"),
			findings: []string{"names no tracked file"}},
		{name: "a child pipeline from another project is not followed", want: "fail", edits: gitlab(
			glOnly("child:\n  trigger:\n    include:\n      - project: group/ci\n        file: verify.yml\n    strategy: depend\n"))},
		{name: "a dynamic child pipeline is not followed", want: "fail", edits: gitlab(
			glOnly("child:\n  trigger:\n    include:\n      - artifact: generated.yml\n        job: generate\n    strategy: depend\n"))},

		// Failures.
		{name: "a .gitlab-ci.yml that does not parse", want: "fail", edits: gitlab("verify: [\n"),
			findings: []string{".gitlab-ci.yml: ", "does not parse"}},
		{name: "a .gitlab-ci.yml that is not a map", want: "fail", edits: gitlab("- a\n- b\n"),
			findings: []string{"must be a map"}},
		{name: "an empty .gitlab-ci.yml", want: "fail", edits: gitlab(""),
			findings: []string{"the file is empty"}},
		{name: "a job that is not a map", want: "fail", edits: gitlab(glBase + "broken: 3\n"),
			findings: []string{"job broken must be a map"}},
		{name: "a script that is not a string or a list", want: "fail", edits: gitlab(glBase + "broken:\n  script: {a: b}\n"),
			findings: []string{"job broken: script must be a string or a list of strings"}},
		{name: "verify missing points at V-04", want: "fail", edits: append(gitlab(glBase), noVerify),
			findings: []string{"vibe.yaml: ", "V-04"}},
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
	onBranch := func(b string) []fixtureEdit {
		return gitlab(glOnly(glVerify("verify", glRules(glMR, `$CI_COMMIT_BRANCH == "`+b+`"`))))
	}
	t.Run("origin/HEAD names trunk", func(t *testing.T) {
		dir := contractFixture(t, onBranch("trunk")...)
		fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
		rep, _ := auditJSON(t, dir, "--static")
		assertRule(t, rep, "C-01", "pass")
	})
	t.Run("master is assumed when the conditions name it more often", func(t *testing.T) {
		dir := contractFixture(t, gitlab(glOnly(glVerify("verify", glRules(glMR, `$CI_COMMIT_BRANCH == "master"`))+
			glVerify("v2", glRules(`$CI_COMMIT_BRANCH == "master"`))))...)
		r := runVibeAudit(t, "--repo-root", dir, "--static")
		if !strings.Contains(r.stdout, "assume the default branch is master") {
			t.Errorf("report does not say master was assumed:\n%s", r.stdout)
		}
	})
	t.Run("main on a tie", func(t *testing.T) {
		tie := gitlab(glVerify("verify", glRules(glMR, `$CI_COMMIT_BRANCH == "master"`)) +
			glAudit(glRules(glMR, `$CI_COMMIT_BRANCH == "main"`)))
		rep, _ := auditJSON(t, contractFixture(t, tie...), "--static")
		assertRule(t, rep, "C-01", "fail", "pushes to main")
	})
}

// C-04 on GitLab CI: a job that runs in merge request pipelines runs vibe
// audit and calls no verify command.
func TestRuleC04GitLab(t *testing.T) {
	t.Parallel()
	verify := glVerify("verify:linux", "") + glVerify("verify:windows", "")
	audit := func(extra, script string) []fixtureEdit {
		return gitlab(verify + "conformance:audit:\n" + extra + "  script:\n" + script)
	}
	runRuleCases(t, "C-04", []ruleCase{
		{name: "baseline", want: "pass", edits: gitlab(glBase)},
		{name: "merge requests only", want: "pass", edits: audit(glRules(glMR), "    - vibe audit\n")},
		{name: "after building vibe, in one line", want: "pass",
			edits: audit("", "    - go build -o \"$HOME/bin/vibe\" ./cmd/vibe && vibe audit --json\n")},
		{name: "with leading assignments", want: "pass", edits: audit("", "    - NO_COLOR=1 vibe audit\n")},
		{name: "the audit job also calls verify", want: "fail", edits: audit("", "    - vibe audit\n    - task verify\n"),
			findings: []string{".gitlab-ci.yml: ", "job conformance:audit", "`task verify`", "own"}},
		{name: "verify in the audit job's before_script", want: "fail",
			edits: audit("  before_script: [task verify]\n", "    - vibe audit\n")},
		{name: "verify in the audit job's after_script does not count", want: "pass",
			edits: audit("  after_script: [task verify]\n", "    - vibe audit\n")},
		{name: "vibe audit in after_script does not count", want: "fail",
			edits: audit("  after_script: [vibe audit]\n", "    - echo\n")},
		{name: "no audit job", want: "fail", edits: gitlab(verify),
			findings: []string{".gitlab-ci.yml: ", "no job that runs in merge request pipelines runs vibe audit"}},
		{name: "the default branch only", want: "fail", edits: audit(glRules(glDefault), "    - vibe audit\n"),
			findings: []string{"job conformance:audit does not count for merge request pipelines", "no rule admits it"}},
		{name: "tags only", want: "fail", edits: audit("  only: [tags]\n", "    - vibe audit\n")},
		{name: "a condition the audit does not read", want: "fail",
			edits:    audit(glRules(`$CI_MERGE_REQUEST_LABELS =~ /audit/`), "    - vibe audit\n"),
			findings: []string{"`$CI_MERGE_REQUEST_LABELS =~ /audit/`", "line "}},
		{name: "changes: does not count", want: "fail",
			edits:    audit("  rules:\n    - changes: [vibe.yaml]\n", "    - vibe audit\n"),
			findings: []string{"changes:"}},
		{name: "in a child pipeline the parent waits for", want: "pass", edits: gitlab(
			verify+"audit:\n  trigger:\n    include: ci/audit.yml\n    strategy: depend\n",
			withFile("ci/audit.yml", glAudit("")))},
		{name: "in a child pipeline the parent does not wait for", want: "fail", edits: gitlab(
			verify+"audit:\n  trigger:\n    include: ci/audit.yml\n",
			withFile("ci/audit.yml", glAudit("")))},
		{name: "from a template", want: "pass", edits: gitlab(verify + ".audit:\n  script: [vibe audit]\nconformance:audit:\n  extends: .audit\n")},
		{name: "vibe without audit", want: "fail", edits: audit("", "    - vibe doctor\n")},
		{name: "vibe audit by path is not vibe audit", want: "fail", edits: audit("", "    - ./bin/vibe audit\n")},
		{name: "verify missing points at V-04", want: "fail", edits: append(gitlab(glBase), noVerify),
			findings: []string{"vibe.yaml: ", "V-04"}},
		{name: "a .gitlab-ci.yml that does not parse", want: "fail", edits: gitlab("x: [\n"),
			findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}
