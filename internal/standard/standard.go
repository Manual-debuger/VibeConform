// Package standard defines named, versioned standards: compositions of
// modules a repository's vibe.yaml can declare conformance to. See
// docs/specs/0003-standard-registry.md and docs/architecture/overview.md.
package standard

import (
	"fmt"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/gotooling"
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
	Register(Standard{
		Name:    "production",
		Version: "v1",
		Modules: []module.Module{gotooling.New()},
	})
}
