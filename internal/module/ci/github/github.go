// Package github provides the GitHub CI module: the workflow, dependency
// update policy, and pull request template a conformant repository gets.
// See docs/specs/0010-github-ci-module.md.
package github

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/ci.yml
var ciWorkflow []byte

//go:embed templates/dependabot.yml
var dependabotConfig []byte

//go:embed templates/pull_request_template.md
var pullRequestTemplate []byte

type githubModule struct{}

// New returns the github-ci module.
func New() module.Module {
	return githubModule{}
}

func (githubModule) Name() string {
	return "github-ci"
}

// Resolve returns this module's resources in a fixed order. Order is part of
// the module's contract: audit, diff, and sync report in resolution order, so
// a stable order is what keeps their output stable across runs and platforms.
//
// Paths are repository-relative identifiers and .vibe/state.yaml keys, so
// they are always slash-separated here; internal/cli converts them to host
// paths at the I/O boundary.
func (githubModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      ".github/workflows/ci.yml",
			Ownership: resource.Generated,
			Content:   ciWorkflow,
		},
		{
			Path:      ".github/dependabot.yml",
			Ownership: resource.Generated,
			Content:   dependabotConfig,
		},
		{
			Path:      ".github/pull_request_template.md",
			Ownership: resource.Generated,
			Content:   pullRequestTemplate,
		},
	}, nil
}
