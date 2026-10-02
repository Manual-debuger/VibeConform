package standard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

// Group is where vibe.yaml selects an option from: an integration category
// under integrations:, selected as a list, or a repository policy key,
// selected as one value (docs/decisions/0014-managed-sections.md).
type Group struct {
	// Key is the category or policy key, e.g. "editors".
	Key string
	// Scalar is true for a group vibe.yaml gives one value, so at most one
	// of its options is selected.
	Scalar bool
	// Map is the top-level vibe.yaml map a scalar group's key is under:
	// manifest.MapPolicy or manifest.MapDevelopment. Empty for an
	// integration category.
	Map string
}

// Option is one optional module in a standard's catalog: an integration,
// tooling around the production contract that a repository selects in
// vibe.yaml's integrations: map (docs/decisions/0013-optional-integrations.md),
// or a repository policy.
type Option struct {
	// Group is what vibe.yaml selects it under.
	Group Group
	// Name is what vibe.yaml selects it by, unique within its group.
	Name string
	// Module resolves the option's resources.
	Module module.Module
	// Default is whether an absent group selects it.
	Default bool
	// Requires names options that must be selected with this one.
	Requires []string
	// Excludes names options that must not be selected with this one.
	Excludes []string
}

// Label is how messages name o: an integration by its name, a scalar
// option by its key in vibe.yaml, e.g. policy.line_endings.
func (o Option) Label() string {
	if o.Group.Scalar {
		return o.Group.Map + "." + o.Group.Key
	}
	return o.Name
}

// scalarGroup returns the group for one single-valued vibe.yaml key.
func scalarGroup(k manifest.ScalarKey) Group {
	return Group{Key: k.Key, Scalar: true, Map: k.Map}
}

// noun is what an error calls a missing scalar key's options: a policy
// under policy:, a setting under development:.
func noun(k manifest.ScalarKey) string {
	if k.Map == manifest.MapPolicy {
		return "policy"
	}
	return "setting"
}

// integrationGroup returns the list group for an integration category.
func integrationGroup(category string) Group {
	return Group{Key: category}
}

// Selection is what vibe.yaml selects from a standard's catalog.
type Selection struct {
	// Integrations names the selected integrations, in catalog order;
	// never nil once resolved.
	Integrations []string
	// Policies maps each selected policy key to its value.
	Policies map[string]string
}

// Has reports whether o is selected.
func (sel Selection) Has(o Option) bool {
	if o.Group.Scalar {
		v, ok := sel.Policies[o.Group.Key]
		return ok && v == o.Name
	}
	return slices.Contains(sel.Integrations, o.Name)
}

// With returns sel with o also selected, in catalog order: the trial
// selection pruning asks what a deselected option would produce.
func (s *Standard) With(sel Selection, o Option) Selection {
	next := Selection{Policies: map[string]string{}}
	for k, v := range sel.Policies {
		next.Policies[k] = v
	}
	if o.Group.Scalar {
		next.Policies[o.Group.Key] = o.Name
		next.Integrations = slices.Clone(sel.Integrations)
		return next
	}
	next.Integrations = []string{}
	for _, it := range s.Options {
		if !it.Group.Scalar && (it.Name == o.Name || slices.Contains(sel.Integrations, it.Name)) {
			next.Integrations = append(next.Integrations, it.Name)
		}
	}
	return next
}

// Defaults returns what a vibe.yaml selecting nothing selects.
func (s *Standard) Defaults() Selection {
	sel := Selection{Integrations: []string{}, Policies: map[string]string{}}
	for _, o := range s.Options {
		if !o.Default {
			continue
		}
		if o.Group.Scalar {
			sel.Policies[o.Group.Key] = o.Name
			continue
		}
		sel.Integrations = append(sel.Integrations, o.Name)
	}
	return sel
}

// Select resolves vibe.yaml's integrations:, policy: and development:
// maps against the catalog. An absent category takes its defaults, an empty one selects
// none; an absent policy key selects its default, which no shipped policy
// has. Integrations are in catalog order whatever order vibe.yaml lists
// names in, and never nil.
func (s *Standard) Select(m *manifest.Manifest) (Selection, error) {
	chosen := map[string]bool{}
	in := m.Integrations
	for _, category := range manifest.Categories {
		names := in.Get(category)
		if names == nil {
			for _, o := range s.Options {
				if o.Group == integrationGroup(category) && o.Default {
					chosen[o.Name] = true
				}
			}
			continue
		}
		valid := s.names(integrationGroup(category))
		for i, name := range *names {
			if !slices.Contains(valid, name) {
				where := fmt.Sprintf("integrations.%s[%d] (%s)", category, i, name)
				if len(valid) == 0 {
					if category == manifest.CategoryIntelligence {
						return Selection{}, fmt.Errorf("%s: no code-intelligence providers are available yet", where)
					}
					return Selection{}, fmt.Errorf("%s: %s/%s offers no %s integrations", where, s.Name, s.Version, category)
				}
				return Selection{}, fmt.Errorf("%s: unknown %s integration (valid: %s)", where, category, strings.Join(valid, ", "))
			}
			chosen[name] = true
		}
	}

	sel := Selection{Integrations: []string{}, Policies: map[string]string{}}
	for _, o := range s.Options {
		if o.Group.Scalar {
			if o.Default {
				sel.Policies[o.Group.Key] = o.Name
			}
			continue
		}
		if chosen[o.Name] {
			sel.Integrations = append(sel.Integrations, o.Name)
		}
	}
	for _, k := range manifest.ScalarKeys {
		value := m.Scalar(k)
		if value == nil {
			continue
		}
		where := fmt.Sprintf("%s.%s (%s)", k.Map, k.Key, *value)
		valid := s.names(scalarGroup(k))
		switch {
		case len(valid) == 0:
			return Selection{}, fmt.Errorf("%s: %s/%s offers no %s %s", where, s.Name, s.Version, k.Key, noun(k))
		case !slices.Contains(valid, *value):
			return Selection{}, fmt.Errorf("%s: unknown value (valid: %s)", where, strings.Join(valid, ", "))
		}
		sel.Policies[k.Key] = *value
	}
	if err := s.checkRelations(sel); err != nil {
		return Selection{}, err
	}
	return sel, nil
}

// checkRelations validates every selected option's Requires and Excludes.
func (s *Standard) checkRelations(sel Selection) error {
	selected := func(name string) bool {
		for _, o := range s.Options {
			if o.Name == name && sel.Has(o) {
				return true
			}
		}
		return false
	}
	kind := func(o Option) string {
		if o.Group.Scalar {
			return fmt.Sprintf("%s: %s", o.Label(), o.Name)
		}
		return "integration " + o.Name
	}
	// required names a required option as vibe.yaml would select it: a
	// setting by its key and value, anything else by its name.
	required := func(name string) string {
		for _, o := range s.Options {
			if o.Name == name && o.Group.Scalar {
				return kind(o)
			}
		}
		return name
	}
	for _, o := range s.Options {
		if !sel.Has(o) {
			continue
		}
		for _, r := range o.Requires {
			if !selected(r) {
				return fmt.Errorf("%s requires %s, which is not selected", kind(o), required(r))
			}
		}
		for _, x := range o.Excludes {
			if selected(x) {
				return fmt.Errorf("%s cannot be selected together with %s", kind(o), x)
			}
		}
	}
	return nil
}

// ModulesFor returns the core modules followed by the selected options'
// modules, in catalog order.
func (s *Standard) ModulesFor(sel Selection) []module.Module {
	mods := slices.Clone(s.Modules)
	for _, o := range s.Options {
		if sel.Has(o) {
			mods = append(mods, o.Module)
		}
	}
	return mods
}

// names lists the catalog's options in group, in catalog order.
func (s *Standard) names(group Group) []string {
	var names []string
	for _, o := range s.Options {
		if o.Group == group {
			names = append(names, o.Name)
		}
	}
	return names
}
