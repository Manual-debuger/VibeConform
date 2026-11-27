package contract

import "slices"

// The repository paths the rules read (spec 0073). Each reader uses the
// named value that RepoPaths lists, so that the list and the readers
// cannot drift.

// The agent instruction files (A-01, A-02).
const (
	agentsFile   = "AGENTS.md"
	claudeMDFile = "CLAUDE.md"
)

// The pull request templates R-03 recognizes, compared case-insensitively
// (spec 0055, R-03): githubPRTemplate and prTemplateFiles, any file under
// prTemplateDir, and a .md file directly under mrTemplateDir.
const (
	githubPRTemplate = ".github/pull_request_template.md"
	prTemplateDir    = ".github/pull_request_template"
	mrTemplateDir    = ".gitlab/merge_request_templates"
)

var prTemplateFiles = []string{"pull_request_template.md", githubPRTemplate, "docs/pull_request_template.md"}

// lefthookScriptDir holds lefthook's scripts: <dir>/<hook>/<name>.
const lefthookScriptDir = ".lefthook"

// rootPackageJSON is the root package.json, whose renovate key Renovate
// reads after its own files (D-02).
const rootPackageJSON = "package.json"

// RepoPaths returns the repository paths that the rules read: files by
// their path from the repository root, and directories, whose files the
// rules read, with a trailing slash. A path that a rule builds from the
// configuration, such as a component's manifest or lockfile, contributes
// its file name. Each call returns a new slice.
func RepoPaths() []string {
	paths := []string{
		ConfigFile, agentsFile, claudeMDFile, claudeSettingsFile,
		guardPolicyFile, localPolicyFile,
		workflowDir + "/", gitlabCIFile,
	}
	paths = append(paths, otherCIFiles...)
	paths = append(paths, dependabotFiles...)
	for _, rf := range renovateFiles {
		paths = append(paths, rf.file)
	}
	paths = append(paths, lefthookFiles...)
	paths = append(paths, lefthookScriptDir+"/", preCommitFile, huskyDir+"/")
	paths = append(paths, prTemplateFiles...)
	paths = append(paths, prTemplateDir+"/", mrTemplateDir+"/")
	for _, l := range languageRegistry {
		paths = append(paths, l.manifest)
		paths = append(paths, l.companions...)
		for _, lf := range l.lockfiles {
			paths = append(paths, lf.file)
		}
	}
	if !slices.Contains(paths, rootPackageJSON) {
		paths = append(paths, rootPackageJSON)
	}
	return paths
}
