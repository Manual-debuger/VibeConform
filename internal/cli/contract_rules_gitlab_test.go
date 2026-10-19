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

// glSHA is a full commit SHA, as a pinned include names it.
const glSHA = "08eba0b27e820071cde6df949e0beb9ba4906955"

// glUnpinned is an image without a digest.
const glUnpinned = "golang:1.25"

// glWithJob is the baseline with one more job.
func glWithJob(job string) []fixtureEdit { return gitlab(glBase + "\n" + job) }

// glInclude is the baseline with a top-level include list.
func glInclude(entries string) []fixtureEdit { return gitlab("include:\n" + entries + glBase) }

// C-02 on GitLab CI: images and services pinned by digest, unfollowed
// includes pinned, no template: include, no @latest in any script.
func TestRuleC02GitLab(t *testing.T) {
	t.Parallel()
	run := func(key, line string) []fixtureEdit {
		return glWithJob("x:\n  " + key + ":\n    - " + line + "\n  script: [echo]\n")
	}
	runRuleCases(t, "C-02", []ruleCase{
		{name: "baseline", want: "pass", edits: gitlab(glBase)},

		// Images and services.
		{name: "a job image by tag", want: "fail", edits: glWithJob("x:\n  image: " + glUnpinned + "\n  script: [echo]\n"),
			findings: []string{".gitlab-ci.yml: ", "line 24: ", "job x", "image golang:1.25", "@sha256:"}},
		{name: "a job image pinned", want: "pass", edits: glWithJob("x:\n  image: " + glImage + "\n  script: [echo]\n")},
		{name: "a job image in name: form by tag", want: "fail",
			edits:    glWithJob("x:\n  image:\n    name: " + glUnpinned + "\n    entrypoint: ['']\n  script: [echo]\n"),
			findings: []string{"image golang:1.25"}},
		{name: "a job image in name: form pinned", want: "pass",
			edits: glWithJob("x:\n  image:\n    name: " + glImage + "\n  script: [echo]\n")},
		{name: "a default image by tag", want: "fail",
			edits:    gitlab(strings.Replace(glBase, glImage, glUnpinned, 1)),
			findings: []string{".gitlab-ci.yml: ", "line 2: ", "image golang:1.25"}},
		{name: "a short digest", want: "fail",
			edits: glWithJob("x:\n  image: golang:1.25@sha256:0123abcd\n  script: [echo]\n")},
		{name: "a job service by tag", want: "fail",
			edits:    glWithJob("x:\n  services:\n    - postgres:16\n  script: [echo]\n"),
			findings: []string{"services postgres:16"}},
		{name: "a job service in name: form by tag", want: "fail",
			edits:    glWithJob("x:\n  services:\n    - name: postgres:16\n      alias: db\n  script: [echo]\n"),
			findings: []string{"services postgres:16"}},
		{name: "a job service pinned", want: "pass",
			edits: glWithJob("x:\n  services:\n    - name: postgres:16@sha256:" + strings.Repeat("ab", 32) + "\n  script: [echo]\n")},
		{name: "a default service by tag", want: "fail",
			edits:    gitlab("default:\n  services: [redis:7]\n" + glBase[len("default:\n"):]),
			findings: []string{"services redis:7"}},
		{name: "an image from a template", want: "fail",
			edits:    glWithJob(".tpl:\n  image: " + glUnpinned + "\nx:\n  extends: .tpl\n  script: [echo]\n"),
			findings: []string{"image golang:1.25"}},
		{name: "an unpinned image in a template no job uses", want: "pass",
			edits: glWithJob(".tpl:\n  image: " + glUnpinned + "\n")},
		{name: "an image in a local include", want: "fail", edits: gitlab("include: ci/x.yml\n"+glBase,
			withFile("ci/x.yml", "x:\n  image: "+glUnpinned+"\n  script: [echo]\n")),
			findings: []string{"ci/x.yml: ", "line 2: "}},
		{name: "an image in a child pipeline", want: "fail", edits: gitlab(
			glBase+"child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n",
			withFile("ci/child.yml", "x:\n  image: "+glUnpinned+"\n  script: [echo]\n")),
			findings: []string{"ci/child.yml: ", "image golang:1.25"}},

		// Includes that are not followed.
		{name: "a project include pinned", want: "pass",
			edits: glInclude("  - project: group/ci\n    ref: " + glSHA + "\n    file: verify.yml\n")},
		{name: "a project include by tag", want: "fail",
			edits:    glInclude("  - project: group/ci\n    ref: v1.2.3\n    file: verify.yml\n"),
			findings: []string{".gitlab-ci.yml: ", "line 2: ", "project group/ci", "40-character"}},
		{name: "a project include without ref", want: "fail",
			edits: glInclude("  - project: group/ci\n    file: verify.yml\n")},
		{name: "a component pinned", want: "pass",
			edits: glInclude("  - component: $CI_SERVER_FQDN/group/lint/golangci@" + glSHA + "\n")},
		{name: "a component by version", want: "fail",
			edits:    glInclude("  - component: $CI_SERVER_FQDN/group/lint/golangci@1.2.0\n"),
			findings: []string{"component $CI_SERVER_FQDN/group/lint/golangci@1.2.0"}},
		{name: "a component at ~latest", want: "fail",
			edits: glInclude("  - component: gitlab.com/group/lint/golangci@~latest\n")},
		{name: "a remote include with integrity", want: "pass",
			edits: glInclude("  - remote: https://example.com/ci.yml\n    integrity: sha256-" + strings.Repeat("A", 43) + "=\n")},
		{name: "a remote include without integrity", want: "fail",
			edits:    glInclude("  - remote: https://example.com/ci.yml\n"),
			findings: []string{"remote https://example.com/ci.yml", "integrity:"}},
		{name: "a remote include as a string", want: "fail", edits: glInclude("  - https://example.com/ci.yml\n")},
		{name: "a template include cannot be pinned", want: "fail",
			edits:    glInclude("  - template: Jobs/SAST.gitlab-ci.yml\n"),
			findings: []string{"template Jobs/SAST.gitlab-ci.yml", "cannot be pinned"}},
		{name: "an unpinned include in a local include", want: "fail", edits: gitlab("include: ci/x.yml\n"+glBase,
			withFile("ci/x.yml", "include:\n  - project: group/ci\n    file: x.yml\n")),
			findings: []string{"ci/x.yml: "}},
		{name: "an unpinned include in a trigger", want: "fail", edits: glWithJob(
			"child:\n  trigger:\n    include:\n      - project: group/ci\n        file: child.yml\n    strategy: depend\n")},

		// @latest in scripts.
		{name: "@latest in script", want: "fail", edits: run("script", "go install golang.org/x/tools/cmd/goimports@latest"),
			findings: []string{".gitlab-ci.yml: ", "job x, script", "goimports@latest", "pin"}},
		{name: "@latest in before_script", want: "fail", edits: run("before_script", "npx prettier@latest --check ."),
			findings: []string{"job x, before_script", "prettier@latest"}},
		{name: "@latest in after_script", want: "fail", edits: run("after_script", "npx prettier@latest --check ."),
			findings: []string{"job x, after_script"}},
		{name: "a pinned version", want: "pass", edits: run("script", "go install golang.org/x/tools/cmd/goimports@v0.30.0")},
		{name: "@latest inside a word is not a version", want: "pass", edits: run("script", "echo user@latest.example")},
		{name: "a .gitlab-ci.yml that does not parse", want: "fail", edits: gitlab("x: [\n"),
			findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// C-03 on GitLab CI is not applicable: job tokens have no per-file
// permissions.
func TestRuleC03GitLab(t *testing.T) {
	t.Parallel()
	runRuleCases(t, "C-03", []ruleCase{
		{name: "GitLab only", want: "not_applicable", edits: gitlab(glBase),
			findings: []string{"not applicable", "GitLab", "job tokens have no per-file permissions", "project setting"}},
		{name: "even when .gitlab-ci.yml does not parse", want: "not_applicable", edits: gitlab("x: [\n")},
		{name: "both providers: GitHub is read", want: "pass", edits: []fixtureEdit{withFile(gitlabFile, glBase)}},
	})
}

// C-05 on GitLab CI: platforms from tags after parallel: matrix expansion;
// no tags and an image is Linux.
func TestRuleC05GitLab(t *testing.T) {
	t.Parallel()
	only := func(jobs string) []fixtureEdit { return gitlab(glOnly(jobs)) }
	matrix := func(tags, entries string) []fixtureEdit {
		return only(glVerify("verify", "  parallel:\n    matrix:\n"+entries+"  tags: "+tags+"\n"))
	}
	runRuleCases(t, "C-05", []ruleCase{
		{name: "baseline", want: "pass", edits: gitlab(glBase)},
		{name: "tags ignore case and match inside a tag", want: "pass", edits: only(
			glVerify("a", "  tags: [saas-Linux-small-amd64]\n") + glVerify("b", "  tags: [SAAS-WINDOWS-MEDIUM-AMD64]\n"))},
		{name: "untagged with an image is Linux", want: "pass", edits: only(
			glVerify("a", "  image: "+glImage+"\n") + glVerify("b", "  tags: [windows]\n"))},
		{name: "untagged with a default image is Linux", want: "pass",
			edits: gitlab("default:\n  image: " + glImage + "\n" + glOnly(glVerify("a", "")+glVerify("b", "  tags: [windows]\n")))},
		{name: "untagged without an image is unknown", want: "fail", edits: only(
			glVerify("a", "") + glVerify("b", "  tags: [windows]\n")),
			findings: []string{".gitlab-ci.yml: ", "`task verify`", "no Linux runner", "job a", "no tags and no image"}},
		{name: "a tag that names no platform is unknown", want: "fail", edits: only(
			glVerify("a", "  tags: [docker]\n") + glVerify("b", "  tags: [windows]\n")),
			findings: []string{"no Linux runner", "docker"}},
		{name: "a tag beside a platform tag", want: "pass", edits: only(
			glVerify("a", "  tags: [docker, linux]\n") + glVerify("b", "  tags: [windows]\n"))},
		{name: "Linux only", want: "fail", edits: only(glVerify("a", "  tags: [linux]\n")),
			findings: []string{"no Windows runner", "tags: [windows]"}},
		{name: "macOS and Windows", want: "fail", edits: only(
			glVerify("a", "  tags: [saas-macos-medium-m1]\n") + glVerify("b", "  tags: [windows]\n")),
			findings: []string{"no Linux runner"}},
		{name: "a matrix over tags", want: "pass", edits: matrix("['${RUNNER}']", "      - RUNNER: [linux, windows]\n")},
		{name: "a matrix with $VAR", want: "pass", edits: matrix("[$RUNNER]", "      - RUNNER: [linux, windows]\n")},
		{name: "a matrix of several entries", want: "pass",
			edits: matrix("[$OS]", "      - OS: linux\n        GO: ['1.24', '1.25']\n      - OS: windows\n")},
		{name: "a matrix that leaves Windows out", want: "fail",
			edits: matrix("[$OS]", "      - OS: linux\n        GO: ['1.24', '1.25']\n")},
		{name: "a tag variable the matrix does not set is unknown", want: "fail",
			edits:    matrix("[$RUNNER]", "      - OS: [linux, windows]\n"),
			findings: []string{"$RUNNER"}},
		{name: "after_script does not count", want: "fail", edits: only(glVerify("a", "  tags: [linux]\n") +
			"b:\n  tags: [windows]\n  script: [echo]\n  after_script: [task verify]\n")},
		{name: "tags from default:", want: "pass",
			edits: gitlab("default:\n  tags: [windows]\n" + glOnly(glVerify("a", "  tags: [linux]\n")+glVerify("b", "")))},
		{name: "list verify, one command on Linux only", want: "fail", edits: append(only(
			"lint:\n  tags: [linux]\n  script: [task lint]\n"+
				"test:\n  parallel:\n    matrix:\n      - T: [linux, windows]\n  tags: [$T]\n  script: [task test]\n"), listVerify),
			findings: []string{"`task lint`"}},
		{name: "verify called nowhere", want: "fail", edits: only("x:\n  tags: [linux]\n  script: [echo]\n"),
			findings: []string{".gitlab-ci.yml: ", "no job calls"}},
		{name: "not applicable without windows", want: "not_applicable",
			edits: gitlab(glOnly(glVerify("a", "  tags: [linux]\n")),
				withFile("vibe.yaml", configWith(section{"platforms", "platforms: [linux]\n"})))},
		{name: "a .gitlab-ci.yml that does not parse", want: "fail", edits: gitlab("x: [\n"),
			findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// D-03 on GitLab CI: a scanner in any script, or GitLab's
// Dependency-Scanning template or component.
func TestRuleD03GitLab(t *testing.T) {
	t.Parallel()
	noScan := strings.Replace(glBase,
		"scan:\n  script:\n    - go install golang.org/x/vuln/cmd/govulncheck@v1.1.4\n    - govulncheck ./...\n\n", "", 1)
	with := func(more string) []fixtureEdit { return gitlab(noScan + more) }
	include := func(entry string) []fixtureEdit { return gitlab("include:\n" + entry + noScan) }
	runRuleCases(t, "D-03", []ruleCase{
		{name: "baseline govulncheck", want: "pass", edits: gitlab(glBase)},
		{name: "pip-audit in before_script", want: "pass", edits: with("scan:\n  before_script: [pip-audit]\n  script: [echo]\n")},
		{name: "trivy in after_script", want: "pass", edits: with("scan:\n  script: [echo]\n  after_script: [trivy fs .]\n")},
		{name: "a scanner in a child pipeline", want: "pass", edits: gitlab(
			noScan+"child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n",
			withFile("ci/child.yml", "scan:\n  script: [osv-scanner scan -r .]\n"))},
		{name: "a scanner in a job with rules", want: "pass",
			edits: with("scan:\n  rules:\n    - if: '$CI_PIPELINE_SOURCE == \"schedule\"'\n  script: [npm audit]\n")},
		{name: "the Dependency-Scanning template", want: "pass",
			edits: include("  - template: Jobs/Dependency-Scanning.gitlab-ci.yml\n")},
		{name: "the Security Dependency-Scanning template", want: "pass",
			edits: include("  - template: Security/Dependency-Scanning.latest.gitlab-ci.yml\n")},
		{name: "another template", want: "fail", edits: include("  - template: Jobs/SAST.gitlab-ci.yml\n")},
		{name: "the dependency-scanning component", want: "pass",
			edits: include("  - component: $CI_SERVER_FQDN/components/dependency-scanning/main@" + glSHA + "\n")},
		{name: "the dependency-scanning component unpinned is still recognized", want: "pass",
			edits: include("  - component: gitlab.com/components/dependency-scanning/main@0\n")},
		{name: "another component", want: "fail", edits: include("  - component: gitlab.com/components/sast/main@" + glSHA + "\n")},
		{name: "installing a scanner is not running it", want: "fail",
			edits: with("scan:\n  script: [go install golang.org/x/vuln/cmd/govulncheck@v1.1.4]\n")},
		{name: "no scanner", want: "fail", edits: gitlab(noScan),
			findings: []string{".gitlab-ci.yml: ", "govulncheck", "Dependency-Scanning"}},
		{name: "a .gitlab-ci.yml that does not parse", want: "fail", edits: gitlab("x: [\n"),
			findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// The Dependency-Scanning template is still a template: C-02 fails it.
func TestRuleD03GitLabTemplateFailsC02(t *testing.T) {
	t.Parallel()
	rep, _ := auditJSON(t, contractFixture(t, glInclude("  - template: Jobs/Dependency-Scanning.gitlab-ci.yml\n")...), "--static")
	assertRule(t, rep, "D-03", "pass")
	assertRule(t, rep, "C-02", "fail", "template Jobs/Dependency-Scanning.gitlab-ci.yml")
}

// R-01 on GitLab CI: every job's scripts, after_script included, except the
// C-04 audit job's; vibe --version in a job that calls no entrypoint.
func TestRuleR01GitLab(t *testing.T) {
	t.Parallel()
	runStaticPartCases(t, "R-01", []ruleCase{
		{name: "baseline", want: "pass", edits: gitlab(glBase)},
		{name: "vibe in a script", want: "fail", edits: glWithJob("sync:\n  script:\n    - vibe sync && git diff --exit-code\n"),
			findings: []string{".gitlab-ci.yml: ", "line 25: job sync, script", "`vibe sync`"}},
		{name: "vibe in before_script", want: "fail", edits: glWithJob("sync:\n  before_script: [vibe sync]\n  script: [echo]\n"),
			findings: []string{"job sync, before_script"}},
		{name: "vibe in after_script", want: "fail", edits: glWithJob("sync:\n  script: [echo]\n  after_script: [vibe doctor]\n"),
			findings: []string{"job sync, after_script", "`vibe doctor`"}},
		{name: "vibe in a default before_script", want: "fail",
			edits:    gitlab("default:\n  before_script: [vibe sync]\n" + glBase[len("default:\n"):]),
			findings: []string{"`vibe sync`"}},
		{name: "other vibe commands in the audit job", want: "pass",
			edits: gitlab(strings.Replace(glBase, "    - vibe audit --static\n", "    - vibe doctor\n    - vibe audit --static\n", 1))},
		{name: "vibe in the audit job's after_script", want: "pass",
			edits: gitlab(glBase + "  after_script: [vibe doctor]\n")},
		{name: "vibe audit in a job that also calls verify", want: "fail",
			edits:    gitlab(strings.Replace(glBase, "    - vibe audit --static\n", "    - vibe audit --static\n    - task verify\n", 1)),
			findings: []string{"job conformance:audit, script", "`vibe audit --static`"}},
		{name: "vibe audit in a job that does not run in merge requests", want: "fail",
			edits: gitlab(strings.Replace(glBase, "conformance:audit:\n", "conformance:audit:\n"+glRules(glDefault), 1))},
		{name: "the audit job in a child pipeline", want: "pass", edits: gitlab(
			strings.Replace(glBase, glAudit(""), "", 1)+"audit:\n  trigger:\n    include: ci/audit.yml\n    strategy: depend\n",
			withFile("ci/audit.yml", glAudit("  after_script: [vibe doctor]\n")))},
		{name: "vibe in a child pipeline", want: "fail", edits: gitlab(
			glBase+"child:\n  trigger:\n    include: ci/child.yml\n    strategy: depend\n",
			withFile("ci/child.yml", "sync:\n  script: [vibe sync]\n")),
			findings: []string{"ci/child.yml: ", "`vibe sync`"}},
		{name: "vibe by path is not a call of vibe", want: "pass",
			edits: glWithJob("sync:\n  script: ['\"$CI_PROJECT_DIR/bin/vibe\" sync']\n")},
		// vibe --version tests an installed binary (#120 Part A).
		{name: "vibe --version in a job that calls no entrypoint", want: "pass",
			edits: glWithJob("install:\n  script:\n    - go install ./cmd/vibe\n    - vibe --version | tee version.txt\n")},
		{name: "vibe --version in a job that calls verify", want: "fail",
			edits:    glWithJob("v:\n  before_script: [vibe --version]\n  script: [task verify]\n"),
			findings: []string{"job v, before_script", "`vibe --version`"}},
		{name: "vibe --version in a job that calls verify in after_script", want: "fail",
			edits:    glWithJob("v:\n  script: [vibe --version]\n  after_script: [task verify]\n"),
			findings: []string{"`vibe --version`"}},
		{name: "vibe --version with more words is a call", want: "fail",
			edits: glWithJob("install:\n  script: [vibe --version --json]\n")},
		{name: "a .gitlab-ci.yml that does not parse", want: "fail", edits: gitlab("x: [\n"),
			findings: []string{".gitlab-ci.yml: ", "does not parse"}},
	})
}

// A GitLab-only repository: the baseline decides every C rule, D-03 and
// R-01's static part, C-03 is not applicable, and the report names GitLab
// CI (spec 0055, acceptance criterion 2).
func TestGitLabBaseline(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, gitlab(glBase)...)
	rep, _ := auditJSON(t, dir, "--static")
	for _, id := range []string{"C-01", "C-02", "C-04", "C-05", "D-03"} {
		assertRule(t, rep, id, "pass")
	}
	assertRule(t, rep, "C-03", "not_applicable", "GitLab")
	for _, p := range rep.rule(t, "R-01").Parts {
		if p.Check == "static" && p.Result != "pass" {
			t.Errorf("R-01 static part = %s, want pass\n%s", p.Result, rep.rule(t, "R-01").findingText())
		}
	}
	if rep.CIProvider != "gitlab_ci" {
		t.Errorf("ci_provider = %q, want gitlab_ci", rep.CIProvider)
	}
	if text := runVibeAudit(t, "--repo-root", dir, "--static").stdout; !strings.Contains(text, "CI provider read: GitLab CI\n") {
		t.Errorf("text report does not name GitLab CI:\n%s", text)
	}
}

// A repository with both GitHub workflows and a .gitlab-ci.yml gets the
// results of the GitHub baseline, whatever the GitLab file says (spec
// 0055, acceptance criterion 4).
func TestBothProvidersReadAsGitHub(t *testing.T) {
	t.Parallel()
	github, _ := auditJSON(t, contractFixture(t), "--static")
	bad := "include:\n  - template: Jobs/SAST.gitlab-ci.yml\n" +
		"x:\n  image: golang:1.25\n  services: [redis:7]\n  tags: [docker]\n  script: [vibe sync, go install a/b@latest]\n"
	both, _ := auditJSON(t, contractFixture(t, withFile(gitlabFile, bad)), "--static")
	for _, id := range []string{"C-01", "C-02", "C-03", "C-04", "C-05", "D-03", "R-01"} {
		g, b := github.rule(t, id), both.rule(t, id)
		if g.Result != b.Result || g.findingText() != b.findingText() {
			t.Errorf("%s with both providers = %s %q, want the GitHub result %s %q", id, b.Result, b.findingText(), g.Result, g.findingText())
		}
	}
}
