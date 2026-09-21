// Package standard defines named, versioned standards: compositions of
// modules a repository's vibe.yaml can declare conformance to. See
// docs/specs/0003-standard-registry.md and docs/architecture/overview.md.
package standard

import (
	"fmt"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/agents"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/github"
	"github.com/Manual-debuger/VibeConform/internal/module/gotooling"
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
	// Modules are the modules this standard composes.
	Modules []module.Module
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

func init() {
	// "prod-go" was "production" through M2, kept unnamespaced because
	// renaming it then would have broken the vibe.yaml and .vibe/state.yaml
	// this repository had already committed, for cosmetic gain. M3 settled
	// the naming across all three standards. See
	// docs/specs/0015-standard-naming.md.
	Register(Standard{
		Name:    "prod-go",
		Version: "v1",
		// Order matters: audit, diff, and sync report in module order.
		Modules: []module.Module{gotooling.New(), github.New(), repotooling.New(), agents.New()},
	})

	// Through M2 these composed only their language-tooling module plus
	// agent-config: no repo-tooling, no CI, so a repository declaring
	// prod-ts/prod-py got lint config but no verification entry point at
	// all. Spec 0016 increment 1 adds each language's own repo-tooling
	// variant here; increment 2 adds a github-ci variant the same way.
	Register(Standard{
		Name:    "prod-ts",
		Version: "v1",
		Modules: []module.Module{tstooling.New(), tsrepotooling.New(), agents.New()},
	})

	Register(Standard{
		Name:    "prod-py",
		Version: "v1",
		Modules: []module.Module{pythontooling.New(), pyrepotooling.New(), agents.New()},
	})
}
