// Package githubpy provides the Python variant of the GitHub CI module: a
// workflow on astral-sh/setup-uv instead of actions/setup-go, its own
// dependency-update policy (pip, not gomod), and the language-neutral pull
// request template. Mirrors internal/module/ci/github's shape for
// prod-go/v1. See docs/specs/0016-ts-py-tooling-parity.md.
package githubpy

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/pins"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/ci.yml.tmpl
var ciSrc string

var ciTemplate = pins.Parse("ci.yml", ciSrc)

//go:embed templates/dependabot.yml
var dependabotConfig []byte

//go:embed templates/pull_request_template.md
var pullRequestTemplate []byte

type githubpyModule struct{}

// New returns the github-ci-py module.
func New() module.Module {
	return githubpyModule{}
}

func (githubpyModule) Name() string {
	return "github-ci-py"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (githubpyModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	ci, err := pins.RenderCurrent(ciTemplate)
	if err != nil {
		return nil, err
	}
	return []resource.Resource{
		{
			Path:      ".github/workflows/ci.yml",
			Ownership: resource.Generated,
			Content:   ci,
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
