package contract_test

import (
	"slices"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// TestRepoPathsAreThePathsTheRulesRead: RepoPaths lists the repository
// paths that the rules read, by their path from the root, a directory
// with a trailing slash, and a path built from a component's directory
// by its file name. The list below is worked out from the rules in spec
// 0051, 0052 and 0055 and the readers in this package.
func TestRepoPathsAreThePathsTheRulesRead(t *testing.T) {
	want := []string{
		// The configuration and the agent files (V-01, A-01, A-02).
		"vibe.yaml", "AGENTS.md", "CLAUDE.md", ".claude/settings.json",
		// The guard's policy (A-08) and its local adjustments.
		".claude/hooks/policy.json", ".claude/hooks/policy.local.json",
		// CI (C rules): GitHub Actions, GitLab CI and the other providers.
		".github/workflows/", ".gitlab-ci.yml",
		".circleci/config.yml", "Jenkinsfile", "azure-pipelines.yml", ".travis.yml",
		"bitbucket-pipelines.yml", ".buildkite/", ".drone.yml",
		// Update bots (D-02).
		".github/dependabot.yml", ".github/dependabot.yaml",
		"renovate.json", "renovate.json5", ".github/renovate.json", ".github/renovate.json5",
		".gitlab/renovate.json", ".gitlab/renovate.json5",
		".renovaterc", ".renovaterc.json", ".renovaterc.json5",
		// Git hook managers (A-03, A-04).
		"lefthook.yml", ".lefthook.yml", ".lefthook/", ".pre-commit-config.yaml", ".husky/",
		// Pull and merge request templates (R-03).
		"pull_request_template.md", ".github/pull_request_template.md", "docs/pull_request_template.md",
		".github/pull_request_template/", ".gitlab/merge_request_templates/",
		// A component's manifest, companions and lockfiles (V-04, D-01),
		// and the root package.json's renovate key (D-02).
		"go.mod", "go.sum", "pyproject.toml", "uv.lock", "poetry.lock",
		"package.json", "tsconfig.json", "pnpm-lock.yaml", "package-lock.json",
	}
	got := contract.RepoPaths()
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("RepoPaths() = %q\nwant %q", got, want)
	}
	got[0] = "changed"
	if slices.Contains(contract.RepoPaths(), "changed") {
		t.Error("changing RepoPaths' result changed the next call's")
	}
}
