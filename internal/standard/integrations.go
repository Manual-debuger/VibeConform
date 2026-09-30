package standard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

// Integration is one optional module in a standard's catalog: tooling
// around the production contract that a repository selects in vibe.yaml's
// integrations: map (docs/decisions/0013-optional-integrations.md).
type Integration struct {
	// Name is what vibe.yaml selects it by, unique across the catalog.
	Name string
	// Category is one of manifest.Categories.
	Category string
	// Module resolves the integration's resources.
	Module module.Module
	// Default is whether an absent category selects it.
	Default bool
	// Requires names integrations that must be selected with this one.
	Requires []string
	// Excludes names integrations that must not be selected with this one.
	Excludes []string
}

// Defaults returns the integrations an absent integrations: key selects,
// in catalog order.
func (s *Standard) Defaults() []string {
	selected := []string{}
	for _, in := range s.Integrations {
		if in.Default {
			selected = append(selected, in.Name)
		}
	}
	return selected
}

// Select resolves vibe.yaml's integrations: map against the catalog. An
// absent category takes its defaults, an empty one selects none. The
// result is in catalog order whatever order vibe.yaml lists names in, and
// never nil.
func (s *Standard) Select(in *manifest.Integrations) ([]string, error) {
	chosen := map[string]bool{}
	for _, category := range manifest.Categories {
		names := in.Get(category)
		if names == nil {
			for _, it := range s.Integrations {
				if it.Category == category && it.Default {
					chosen[it.Name] = true
				}
			}
			continue
		}
		valid := s.names(category)
		for i, name := range *names {
			if !slices.Contains(valid, name) {
				where := fmt.Sprintf("integrations.%s[%d] (%s)", category, i, name)
				if len(valid) == 0 {
					if category == manifest.CategoryIntelligence {
						return nil, fmt.Errorf("%s: no code-intelligence providers are available yet", where)
					}
					return nil, fmt.Errorf("%s: %s/%s offers no %s integrations", where, s.Name, s.Version, category)
				}
				return nil, fmt.Errorf("%s: unknown %s integration (valid: %s)", where, category, strings.Join(valid, ", "))
			}
			chosen[name] = true
		}
	}

	selected := []string{}
	for _, it := range s.Integrations {
		if chosen[it.Name] {
			selected = append(selected, it.Name)
		}
	}
	for _, it := range s.Integrations {
		if !chosen[it.Name] {
			continue
		}
		for _, r := range it.Requires {
			if !chosen[r] {
				return nil, fmt.Errorf("integration %s requires %s, which is not selected", it.Name, r)
			}
		}
		for _, x := range it.Excludes {
			if chosen[x] {
				return nil, fmt.Errorf("integration %s cannot be selected together with %s", it.Name, x)
			}
		}
	}
	return selected, nil
}

// ModulesFor returns the core modules followed by the selected
// integrations' modules, in catalog order.
func (s *Standard) ModulesFor(selected []string) []module.Module {
	mods := slices.Clone(s.Modules)
	for _, it := range s.Integrations {
		if slices.Contains(selected, it.Name) {
			mods = append(mods, it.Module)
		}
	}
	return mods
}

// Integration returns the catalog entry named name.
func (s *Standard) Integration(name string) (Integration, bool) {
	for _, it := range s.Integrations {
		if it.Name == name {
			return it, true
		}
	}
	return Integration{}, false
}

// names lists the catalog's integrations in category, in catalog order.
func (s *Standard) names(category string) []string {
	var names []string
	for _, it := range s.Integrations {
		if it.Category == category {
			names = append(names, it.Name)
		}
	}
	return names
}
