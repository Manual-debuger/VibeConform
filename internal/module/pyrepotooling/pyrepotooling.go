// Package pyrepotooling provides the Python verification entry points: the
// Taskfile that CI, git hooks, and docs all call rather than duplicating
// command lists, and the lefthook configuration that runs fast checks
// before a commit. Mirrors internal/module/repotooling's shape for
// prod-go/v1. See docs/specs/0016-ts-py-tooling-parity.md.
package pyrepotooling

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

type pyrepotoolingModule struct{}

// New returns the py-repo-tooling module.
func New() module.Module {
	return pyrepotoolingModule{}
}

func (pyrepotoolingModule) Name() string {
	return "py-repo-tooling"
}

// RequiredTools reports the binaries this module's two resources are
// instructions for. Both are normally installed globally rather than per
// project, so their absence from PATH is a real finding rather than a
// false alarm about how the repository manages its dependencies.
func (pyrepotoolingModule) RequiredTools() []module.Tool {
	return []module.Tool{
		{Name: "task", Why: "every verification entry point Taskfile.yml defines"},
		{Name: "lefthook", Why: "the pre-commit hooks lefthook.yml describes, which vibe sync registers"},
	}
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (pyrepotoolingModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
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
