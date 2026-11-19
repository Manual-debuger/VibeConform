package contract

import "fmt"

// The words of C-01, C-04, C-05 and D-03 findings that differ by CI
// provider (spec 0067, "The model"). Each reader puts its provider's
// wording in CI.wording; the rules take every provider-specific phrase,
// message and fix from it, so a rule is written once and keeps each
// provider's text. The two wordings are next to each other here for
// comparison. A finding about the CI as a whole names CI.Dir.

// ciWording is one provider's words for the CI rules' findings.
type ciWording struct {
	// mergeRequests names the merge request pipeline kind. A push to the
	// default branch is "pushes to <branch>" on every provider (kind).
	mergeRequests string
	// hint says what makes a job not count. It follows a "no job runs"
	// message.
	hint string

	// C-01: no job runs for the kind (what), and a verify command that no
	// job of the kind calls. verify is the quoted verify commands.
	noJobRuns    func(what string) string
	noJobRunsFix func(what, verify string) string
	notCalled    func(what string, c Command) string
	notCalledFix func(what string, c Command) string
	// skipFix is the fix of a finding about a skipped job: a job that
	// would count for what, but that a condition left out. Only GitLab runs
	// have skipped jobs.
	skipFix func(what string) string

	// C-04: no job that runs for merge requests runs vibe audit. The
	// message is followed by hint.
	noAuditJob    string
	noAuditJobFix string

	// C-05: no job calls a verify command, and the fix of a verify command
	// that runs on no runner of the lacking platforms.
	uncalledOnRunner    func(c Command) string
	uncalledOnRunnerFix func(c Command) string
	lackingPlatformFix  func(c Command, lacking string) string

	// D-03: no job runs a recognized scanner.
	noScanner    string
	noScannerFix string
}

// kind names a pipeline kind for findings.
func (w *ciWording) kind(k PipelineKind, defaultBranch string) string {
	if k == KindMergeRequest {
		return w.mergeRequests
	}
	return "pushes to " + defaultBranch
}

// githubWording is GitHub Actions' wording.
var githubWording = &ciWording{
	mergeRequests: "pull requests",
	hint: " (a paths filter, a tags filter without branches, a types filter without opened, " +
		"synchronize and reopened, or branch filters that leave the branch out make a workflow not count)",

	noJobRuns: func(what string) string { return "no workflow runs on " + what },
	noJobRunsFix: func(what, verify string) string {
		return fmt.Sprintf("trigger a workflow that calls %s on %s", verify, what)
	},
	notCalled: func(what string, c Command) string {
		return fmt.Sprintf("no workflow that runs on %s calls the verify command `%s`", what, c)
	},
	notCalledFix: func(what string, c Command) string {
		return fmt.Sprintf("add a step that runs `%s` as its own command to a job that runs on %s", c, what)
	},
	// A GitHub run has no skipped jobs (spec 0067), so no finding uses
	// this fix yet. It keeps every func of the wording set.
	skipFix: func(what string) string { return "make the job run on " + what },

	noAuditJob:    "no job of a workflow that runs on pull requests runs vibe audit",
	noAuditJobFix: "add a job, triggered on pull_request, whose step runs vibe audit (any arguments) and that does not call verify",

	uncalledOnRunner: func(c Command) string {
		return fmt.Sprintf("no workflow step calls the verify command `%s`, so it runs on no runner", c)
	},
	uncalledOnRunnerFix: func(c Command) string {
		return fmt.Sprintf("call `%s` in a job that runs on ubuntu-latest and windows-latest (a matrix over os)", c)
	},
	lackingPlatformFix: func(c Command, lacking string) string {
		return fmt.Sprintf("run `%s` on %s too, such as with a matrix over os: [ubuntu-latest, windows-latest] and runs-on: ${{ matrix.os }}",
			c, lacking)
	},

	noScanner: "no workflow step runs a recognized vulnerability scanner (govulncheck, pip-audit, pnpm audit, npm audit, osv-scanner or trivy)",
	noScannerFix: "add a CI step that runs the scanner for the component's language, such as govulncheck ./... for Go, " +
		"pip-audit for Python or pnpm audit for TypeScript",
}

// gitlabWording is GitLab CI's wording.
var gitlabWording = &ciWording{
	mergeRequests: "merge request pipelines",
	hint: " (workflow:rules, rules: or only/except that leave the pipeline out, changes: or exists:, " +
		"and conditions the audit does not read make a job not count)",

	noJobRuns: func(what string) string { return "no job runs in " + what },
	noJobRunsFix: func(what, verify string) string {
		return fmt.Sprintf("add a job that runs in %s and calls %s", what, verify)
	},
	notCalled: func(what string, c Command) string {
		return fmt.Sprintf("no job that runs in %s calls the verify command `%s`", what, c)
	},
	notCalledFix: func(what string, c Command) string {
		return fmt.Sprintf("add `%s` as its own command to the script of a job that runs in %s", c, what)
	},
	skipFix: func(what string) string {
		return "make the job run in " + what + " with conditions the audit reads " +
			"($CI_PIPELINE_SOURCE, $CI_MERGE_REQUEST_IID, $CI_COMMIT_BRANCH, $CI_COMMIT_TAG, $CI_COMMIT_REF_NAME, $CI_DEFAULT_BRANCH), " +
			"without changes: or exists:"
	},

	noAuditJob: "no job that runs in merge request pipelines runs vibe audit",
	noAuditJobFix: "add a job that runs in merge request pipelines, whose script runs vibe audit (any arguments) " +
		"and that calls no verify command",

	uncalledOnRunner: func(c Command) string {
		return fmt.Sprintf("no job calls the verify command `%s`, so it runs on no runner", c)
	},
	uncalledOnRunnerFix: func(c Command) string {
		return fmt.Sprintf("call `%s` in a job on a Linux runner (tags: [linux], or no tags and an image:) and in one with tags: [windows]", c)
	},
	lackingPlatformFix: func(c Command, lacking string) string {
		return fmt.Sprintf("run `%s` on %s too: in a job whose tags name a runner of that platform, such as tags: [windows] "+
			"(saas-windows-medium-amd64 on GitLab.com), or with a parallel: matrix over the tags", c, lacking)
	},

	noScanner: "no job runs a recognized vulnerability scanner (govulncheck, pip-audit, pnpm audit, npm audit, osv-scanner or trivy), " +
		"and the pipeline includes neither GitLab's Dependency-Scanning template nor its dependency-scanning component",
	noScannerFix: "add a job whose script runs the scanner for the component's language, such as govulncheck ./... for Go, pip-audit for Python " +
		"or pnpm audit for TypeScript, or include the dependency-scanning CI/CD component pinned to a commit SHA",
}
