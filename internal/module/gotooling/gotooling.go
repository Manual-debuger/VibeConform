// Package gotooling provides VibeConform's first concrete module: a fixed
// golangci-lint configuration resolved as a single Generated resource. See
// docs/specs/0005-gotooling-module.md.
package gotooling

import (
	_ "embed"

	"context"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed golangci.yml
var golangciConfig []byte

type gotoolingModule struct{}

// New returns the go-tooling module.
func New() module.Module {
	return gotoolingModule{}
}

func (gotoolingModule) Name() string {
	return "go-tooling"
}

func (gotoolingModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      ".golangci.yml",
			Ownership: resource.Generated,
			Content:   golangciConfig,
		},
	}, nil
}
