// Package repotooling provides the repository's verification entry points:
// the Taskfile that CI, git hooks, and docs all call rather than duplicating
// command lists, and the lefthook configuration that runs fast checks before
// a commit. See docs/specs/0011-repo-tooling-module.md.
//
// It is separate from internal/module/gotooling because the two answer
// different questions: gotooling owns language-specific lint configuration,
// this module owns how a repository is verified at all.
package repotooling

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/Taskfile.yml
var taskfile []byte

//go:embed templates/lefthook.yml
var lefthookConfig []byte

type repotoolingModule struct{}

// New returns the repo-tooling module.
func New() module.Module {
	return repotoolingModule{}
}

func (repotoolingModule) Name() string {
	return "repo-tooling"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (repotoolingModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      "Taskfile.yml",
			Ownership: resource.Generated,
			Content:   taskfile,
		},
		{
			Path:      "lefthook.yml",
			Ownership: resource.Generated,
			Content:   lefthookConfig,
		},
	}, nil
}
