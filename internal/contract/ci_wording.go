package contract

import (
	"fmt"
	"strings"
)

// The words of C-01, C-04, C-05 and D-03 findings that differ by CI
// provider (spec 0067, "The model"). Each reader puts its provider's
// wording in ciModel.wording; the rules take every provider-specific phrase,
// message and fix from it, so a rule is written once and keeps each
// provider's text. The two wordings are next to each other here for
// comparison. A finding about the CI as a whole names ciModel.Dir. The fix of
// a finding about a skipped job is in the run (ciSkip.Fix): only the
// GitLab reader skips jobs, and it words the fix with gitlabSkipFix.

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
	notCalled    func(what string, c shellCommand) string
	notCalledFix func(what string, c shellCommand) string

	// C-04: no job that runs for merge requests runs vibe audit. The
	// message is followed by hint.
	noAuditJob    string
	noAuditJobFix string

	// C-05: no job calls a verify command, and the fix of a verify command
	// that runs on no runner of the lacking platforms.
	uncalledOnRunner    func(c shellCommand) string
	uncalledOnRunnerFix func(c shellCommand) string
	lackingPlatformFix  func(c shellCommand, lacking string) string
	// C-05 under contract 2.x, which follows platforms (spec 0074): the
	// same two fixes, for the declared platforms, given as platform
	// values such as "macos".
	uncalledOnPlatformsFix func(c shellCommand, declared []string) string
	lackingPlatformsFix    func(c shellCommand, lacking, declared []string) string

	// D-03: no job runs a recognized scanner.
	noScanner    string
	noScannerFix string
}

// kind names a pipeline kind for findings.
func (w *ciWording) kind(k pipelineKind, defaultBranch string) string {
	if k == kindMergeRequest {
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
	notCalled: func(what string, c shellCommand) string {
		return fmt.Sprintf("no workflow that runs on %s calls the verify command `%s`", what, c)
	},
	notCalledFix: func(what string, c shellCommand) string {
		return fmt.Sprintf("add a step that runs `%s` as its own command to a job that runs on %s", c, what)
	},
	noAuditJob:    "no job of a workflow that runs on pull requests runs vibe audit",
	noAuditJobFix: "add a job, triggered on pull_request, whose step runs vibe audit (any arguments) and that does not call verify",

	uncalledOnRunner: func(c shellCommand) string {
		return fmt.Sprintf("no workflow step calls the verify command `%s`, so it runs on no runner", c)
	},
	uncalledOnRunnerFix: func(c shellCommand) string {
		return fmt.Sprintf("call `%s` in a job that runs on ubuntu-latest and windows-latest (a matrix over os)", c)
	},
	lackingPlatformFix: func(c shellCommand, lacking string) string {
		return fmt.Sprintf("run `%s` on %s too, such as with a matrix over os: [ubuntu-latest, windows-latest] and runs-on: ${{ matrix.os }}",
			c, lacking)
	},
	uncalledOnPlatformsFix: func(c shellCommand, declared []string) string {
		labels := mapWords(declared, githubRunnerLabel)
		if len(labels) == 1 {
			return fmt.Sprintf("call `%s` in a job that runs on %s", c, labels[0])
		}
		return fmt.Sprintf("call `%s` in a job that runs on %s (a matrix over os)", c, joinWords(labels, " and "))
	},
	lackingPlatformsFix: func(c shellCommand, lacking, declared []string) string {
		names := joinWords(mapWords(lacking, platformName), " and ")
		if len(declared) == 1 {
			return fmt.Sprintf("run `%s` on %s, such as in a job with runs-on: %s", c, names, githubRunnerLabel(declared[0]))
		}
		return fmt.Sprintf("run `%s` on %s too, such as with a matrix over os: [%s] and runs-on: ${{ matrix.os }}",
			c, names, strings.Join(mapWords(declared, githubRunnerLabel), ", "))
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
	notCalled: func(what string, c shellCommand) string {
		return fmt.Sprintf("no job that runs in %s calls the verify command `%s`", what, c)
	},
	notCalledFix: func(what string, c shellCommand) string {
		return fmt.Sprintf("add `%s` as its own command to the script of a job that runs in %s", c, what)
	},
	noAuditJob: "no job that runs in merge request pipelines runs vibe audit",
	noAuditJobFix: "add a job that runs in merge request pipelines, whose script runs vibe audit (any arguments) " +
		"and that calls no verify command",

	uncalledOnRunner: func(c shellCommand) string {
		return fmt.Sprintf("no job calls the verify command `%s`, so it runs on no runner", c)
	},
	uncalledOnRunnerFix: func(c shellCommand) string {
		return fmt.Sprintf("call `%s` in a job on a Linux runner (tags: [linux], or no tags and an image:) and in one with tags: [windows]", c)
	},
	lackingPlatformFix: func(c shellCommand, lacking string) string {
		return fmt.Sprintf("run `%s` on %s too: in a job whose tags name a runner of that platform, such as tags: [windows] "+
			"(saas-windows-medium-amd64 on GitLab.com), or with a parallel: matrix over the tags", c, lacking)
	},
	uncalledOnPlatformsFix: func(c shellCommand, declared []string) string {
		return fmt.Sprintf("call `%s` in a job on a runner of each platform in platforms, such as %s",
			c, joinWords(mapWords(declared, gitlabRunnerTags), " and "))
	},
	lackingPlatformsFix: func(c shellCommand, lacking, declared []string) string {
		too, job := " too", "a job"
		if len(declared) == 1 {
			too = ""
		}
		if len(lacking) > 1 {
			job = "a job for each"
		}
		return fmt.Sprintf("run `%s` on %s%s: in %s whose tags name a runner of that platform, such as %s, "+
			"or with a parallel: matrix over the tags", c, joinWords(mapWords(lacking, platformName), " and "), too, job,
			joinWords(mapWords(lacking, gitlabRunnerTags), " and "))
	},

	noScanner: "no job runs a recognized vulnerability scanner (govulncheck, pip-audit, pnpm audit, npm audit, osv-scanner or trivy), " +
		"and the pipeline includes neither GitLab's Dependency-Scanning template nor its dependency-scanning component",
	noScannerFix: "add a job whose script runs the scanner for the component's language, such as govulncheck ./... for Go, pip-audit for Python " +
		"or pnpm audit for TypeScript, or include the dependency-scanning CI/CD component pinned to a commit SHA",
}

// githubRunnerLabel is a GitHub-hosted runner label of the platform.
func githubRunnerLabel(platform string) string {
	switch platform {
	case platformLinux:
		return "ubuntu-latest"
	case platformWindows:
		return "windows-latest"
	}
	return "macos-latest"
}

// gitlabRunnerTags are job keys that put a job on a runner of the
// platform, as the GitLab reader reads them.
func gitlabRunnerTags(platform string) string {
	switch platform {
	case platformLinux:
		return "tags: [linux] (or no tags and an image:)"
	case platformWindows:
		return "tags: [windows] (saas-windows-medium-amd64 on GitLab.com)"
	}
	return "tags: [macos] (saas-macos-medium-m1 on GitLab.com)"
}

// mapWords is each of words through f, in order.
func mapWords(words []string, f func(string) string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = f(w)
	}
	return out
}

// gitlabSkipFix is the fix of a finding about a job that would count for
// what (the pipeline kind, worded by gitlabWording.kind) but that a
// condition left out. The GitLab reader puts it in each ciSkip.
func gitlabSkipFix(what string) string {
	return "make the job run in " + what + " with conditions the audit reads " +
		"($CI_PIPELINE_SOURCE, $CI_MERGE_REQUEST_IID, $CI_COMMIT_BRANCH, $CI_COMMIT_TAG, $CI_COMMIT_REF_NAME, $CI_DEFAULT_BRANCH), " +
		"without changes: or exists:"
}
