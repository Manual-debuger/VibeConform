// Package conformance provides the vibe-conformance module: the only
// generated files that run vibe. It owns the Conformance workflow and the
// Taskfile holding task audit, so removing VibeConform from a repository is
// deleting files rather than editing shared ones. Language-neutral, and
// composed into every standard. See docs/specs/0022-conformance-isolation.md.
package conformance

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/conformance.yml
var conformanceWorkflow []byte

//go:embed templates/Taskfile.vibe.yml
var vibeTaskfile []byte

type conformanceModule struct{}

// New returns the vibe-conformance module.
func New() module.Module {
	return conformanceModule{}
}

func (conformanceModule) Name() string {
	return "vibe-conformance"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
//
// The module deliberately implements no RequiredTools. Its one external
// need, go, matters only when vibe is not on PATH, and task audit explains
// that case itself when it happens.
func (conformanceModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      ".github/workflows/conformance.yml",
			Ownership: resource.Generated,
			Content:   conformanceWorkflow,
		},
		{
			Path:      "Taskfile.vibe.yml",
			Ownership: resource.Generated,
			Content:   vibeTaskfile,
		},
	}, nil
}
