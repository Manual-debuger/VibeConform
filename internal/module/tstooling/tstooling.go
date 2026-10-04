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
	"fmt"
	"strings"

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

// GeneratedSectionID is the .prettierignore section that lists vibe.yaml's
// generated paths (spec 0037 §4).
const GeneratedSectionID = "generated"

// PrettierIgnorePath is where that section lives.
const PrettierIgnorePath = ".prettierignore"

// eslintIgnores is the global ignores line of eslint.config.js, the anchor
// generated paths are appended to.
const eslintIgnores = "ignores: ['**/dist/**', '**/coverage/**']"

// generatedSection is the .prettierignore section shape, without content.
var generatedSection = resource.Resource{
	Path:      PrettierIgnorePath,
	Ownership: resource.ManagedSection,
	SectionID: GeneratedSectionID,
	Markers:   resource.HashComment,
	Placement: resource.Bottom,
}

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
func (tstoolingModule) RequiredTools(_ *module.Context) []module.Tool {
	return []module.Tool{
		{Name: "node", Why: "running eslint, prettier, and tsc out of node_modules", Version: []string{"--version"}},
	}
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
//
// Generated paths (spec 0037) are appended to eslint.config.js's global
// ignores and listed in a .prettierignore section, so both tools skip them
// at every entry point. Without any, every file is the embedded template.
func (tstoolingModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	generated := module.GeneratedOf(mctx)
	eslint, err := ignoreInESLint(eslintConfig, generated)
	if err != nil {
		return nil, err
	}
	rs := []resource.Resource{
		{
			Path:      "eslint.config.js",
			Ownership: resource.Generated,
			Content:   eslint,
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
	}
	if len(generated) > 0 {
		section := generatedSection
		var b strings.Builder
		b.WriteString("# Managed by VibeConform: generated: in vibe.yaml. Not formatted.\n")
		for _, g := range generated {
			b.WriteString(g + "\n")
		}
		section.Content = []byte(b.String())
		rs = append(rs, section)
	}
	return rs, nil
}

// ConditionalSections declares the generated section, which exists only
// while vibe.yaml lists generated paths.
func (tstoolingModule) ConditionalSections() []resource.Resource {
	return []resource.Resource{generatedSection}
}

// ignoreInESLint appends each generated path to the global ignores. The
// grammar admits no quote or backslash, so single quotes need no escaping.
func ignoreInESLint(config []byte, generated []string) ([]byte, error) {
	if len(generated) == 0 {
		return config, nil
	}
	var b strings.Builder
	b.WriteString(eslintIgnores)
	for _, g := range generated {
		fmt.Fprintf(&b, ", '%s'", g)
	}
	return module.ReplaceOnce(config, "eslint.config.js", "ignore generated paths", eslintIgnores, b.String())
}
