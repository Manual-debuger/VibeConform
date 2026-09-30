// Package standard defines named, versioned standards: compositions of
// modules a repository's vibe.yaml can declare conformance to. See
// docs/specs/0003-standard-registry.md and docs/architecture/overview.md.
package standard

import (
	"fmt"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/agents/claude"
	"github.com/Manual-debuger/VibeConform/internal/module/agents/codex"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/github"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/githubmono"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/githubpy"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/githubts"
	"github.com/Manual-debuger/VibeConform/internal/module/conformance"
	"github.com/Manual-debuger/VibeConform/internal/module/editors/vscode"
	"github.com/Manual-debuger/VibeConform/internal/module/editors/zed"
	"github.com/Manual-debuger/VibeConform/internal/module/gotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/monorepotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/monotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/pyrepotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/pythontooling"
	"github.com/Manual-debuger/VibeConform/internal/module/repotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/tsrepotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/tstooling"
)

// Standard is a named, versioned bundle of modules.
type Standard struct {
	// Name identifies the standard, e.g. "production".
	Name string
	// Version pins the standard revision, e.g. "v1".
	Version string
	// Modules are the core modules this standard always composes, in the
	// order every report uses.
	Modules []module.Module
	// Integrations is the catalog of optional modules vibe.yaml's
	// integrations: map selects from. Selected ones resolve after Modules,
	// in this order (docs/decisions/0013-optional-integrations.md).
	Integrations []Integration
	// Profile is a single-language standard's language; empty for one
	// that takes components, whose profiles come from vibe.yaml.
	Profile manifest.Profile
	// TakesComponents is true for a standard that resolves from vibe.yaml's
	// components: list and requires at least one. Every other standard
	// accepts none (docs/decisions/0012-manifest-components.md).
	TakesComponents bool
}

type key struct {
	name    string
	version string
}

var registry = map[key]Standard{}

// Register adds a standard to the registry. It panics on a duplicate
// (name, version) pair, since registration only happens at package init()
// time and a collision is a programmer error, not a runtime condition.
func Register(s Standard) {
	k := key{name: s.Name, version: s.Version}
	if _, exists := registry[k]; exists {
		panic(fmt.Sprintf("standard: duplicate registration for %s/%s", s.Name, s.Version))
	}
	registry[k] = s
}

// Lookup returns the registered standard for the given name and version.
func Lookup(name, version string) (*Standard, error) {
	s, ok := registry[key{name: name, version: version}]
	if !ok {
		return nil, fmt.Errorf("standard: no such standard %s/%s", name, version)
	}
	return &s, nil
}

// catalog is the integration catalog every standard shares: editors,
// then agents, then code intelligence. claude and codex are on by
// default, so a vibe.yaml without integrations: composes exactly what
// every standard composed before spec 0026.
func catalog() []Integration {
	return []Integration{
		{Name: "vscode", Category: manifest.CategoryEditors, Module: vscode.New()},
		{Name: "zed", Category: manifest.CategoryEditors, Module: zed.New()},
		{Name: "claude", Category: manifest.CategoryAgents, Module: claude.New(), Default: true},
		{Name: "codex", Category: manifest.CategoryAgents, Module: codex.New(), Default: true},
	}
}

func init() {
	// "prod-go" was "production" through M2, kept unnamespaced because
	// renaming it then would have broken the vibe.yaml and .vibe/state.yaml
	// this repository had already committed, for cosmetic gain. M3 settled
	// the naming across all three standards. See
	// docs/specs/0015-standard-naming.md.
	Register(Standard{
		Name:    "prod-go",
		Version: "v1",
		Profile: manifest.ProfileGo,
		// Order matters: audit, diff, and sync report in module order, core
		// then selected integrations.
		Modules:      []module.Module{gotooling.New(), github.New(), conformance.New(), repotooling.New()},
		Integrations: catalog(),
	})

	// Through M2 these composed only their language-tooling module plus
	// agent-config: no repo-tooling, no CI, so a repository declaring
	// prod-ts/prod-py got lint config but no verification entry point at
	// all. Spec 0016 closes that gap with each language's own repo-tooling
	// and github-ci variant, in the same module order prod-go uses
	// (language tooling, CI, repo-tooling, agent-config). Spec 0022 adds
	// vibe-conformance to all three, directly after CI: it holds the
	// conformance workflow and task audit, the only generated files that
	// run vibe. Spec 0024 splits agent-config into one module per agent
	// runtime, claude-config then codex-config, in the same position; since
	// spec 0026 they are default-on integrations from catalog().
	Register(Standard{
		Name:         "prod-ts",
		Version:      "v1",
		Profile:      manifest.ProfileTS,
		Modules:      []module.Module{tstooling.New(), githubts.New(), conformance.New(), tsrepotooling.New()},
		Integrations: catalog(),
	})

	Register(Standard{
		Name:         "prod-py",
		Version:      "v1",
		Profile:      manifest.ProfilePy,
		Modules:      []module.Module{pythontooling.New(), githubpy.New(), conformance.New(), pyrepotooling.New()},
		Integrations: catalog(),
	})

	// A polyglot monorepo: every core module but vibe-conformance resolves from vibe.yaml's components, in the same module
	// order as the single-language standards. See
	// docs/specs/0025-prod-mono.md.
	Register(Standard{
		Name:            "prod-mono",
		Version:         "v1",
		Modules:         []module.Module{monotooling.New(), githubmono.New(), conformance.New(), monorepotooling.New()},
		Integrations:    catalog(),
		TakesComponents: true,
	})
}
