// Package pythontooling provides the Python half of prod-py/v1:
// ruff for lint and format, pyright for typecheck, mirroring gotooling's
// shape.
//
// Both configurations are standalone files rather than pyproject.toml
// sections. VibeConform owns whole files, and pyproject.toml also holds
// [project] metadata belonging to the repository — see
// docs/specs/0014-m2-milestone.md.
package pythontooling

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/ruff.toml
var ruffConfig []byte

//go:embed templates/pyrightconfig.json
var pyrightConfig []byte

type pythontoolingModule struct{}

// New returns the python-tooling module.
func New() module.Module {
	return pythontoolingModule{}
}

func (pythontoolingModule) Name() string {
	return "python-tooling"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
//
// The module deliberately implements no RequiredTools: ruff and pyright are
// normally pinned per project and run through uv or a virtualenv, so
// checking PATH for them would warn on a perfectly healthy repository.
func (pythontoolingModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      "ruff.toml",
			Ownership: resource.Generated,
			Content:   ruffConfig,
		},
		{
			Path:      "pyrightconfig.json",
			Ownership: resource.Generated,
			Content:   pyrightConfig,
		},
	}, nil
}
