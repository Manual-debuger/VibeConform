// Package tstooling provides the TypeScript half of prod-ts/v1:
// lint, format, and typecheck configuration, mirroring gotooling's shape.
//
// Configuration only. It does not manage package.json, so it pins no tool
// versions and installs nothing — see docs/specs/0014-m2-milestone.md for
// why that boundary is where it is.
package tstooling

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/eslint.config.js
var eslintConfig []byte

// Template files are named without their leading dot: go:embed excludes
// dot-prefixed names from directory patterns, and gotooling already sets the
// precedent of embedding golangci.yml as .golangci.yml.
//
//go:embed templates/prettierrc.json
var prettierConfig []byte

//go:embed templates/tsconfig.base.json
var tsconfigBase []byte

type tstoolingModule struct{}

// New returns the ts-tooling module.
func New() module.Module {
	return tstoolingModule{}
}

func (tstoolingModule) Name() string {
	return "ts-tooling"
}

// RequiredTools declares node and nothing else. eslint, prettier, and tsc
// are project-local, installed into node_modules rather than onto PATH, so
// warning about them would fire on every correctly configured repository.
func (tstoolingModule) RequiredTools() []module.Tool {
	return []module.Tool{
		{Name: "node", Why: "running eslint, prettier, and tsc out of node_modules"},
	}
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (tstoolingModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      "eslint.config.js",
			Ownership: resource.Generated,
			Content:   eslintConfig,
		},
		{
			Path:      ".prettierrc.json",
			Ownership: resource.Generated,
			Content:   prettierConfig,
		},
		{
			// The base, not tsconfig.json: the standard owns the compiler
			// options, and the repository owns a tsconfig.json extending
			// this one with its own include, outDir, and paths. That split
			// is the closest thing to a managed section available without
			// any change to apply logic.
			Path:      "tsconfig.base.json",
			Ownership: resource.Generated,
			Content:   tsconfigBase,
		},
	}, nil
}
