package standard

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

type fakeModule string

func (f fakeModule) Name() string { return string(f) }
func (fakeModule) Resolve(context.Context, *module.Context) ([]resource.Resource, error) {
	return nil, nil
}

// fakeStandard has a catalog shaped to exercise every rule Select checks,
// including Requires and Excludes, which no shipped integration declares
// yet.
func fakeStandard() *Standard {
	return &Standard{
		Name:    "fake",
		Version: "v1",
		Modules: []module.Module{fakeModule("core")},
		Integrations: []Integration{
			{Name: "ed-a", Category: manifest.CategoryEditors, Module: fakeModule("ed-a")},
			{Name: "ed-b", Category: manifest.CategoryEditors, Module: fakeModule("ed-b"), Excludes: []string{"ag-y"}},
			{Name: "ag-x", Category: manifest.CategoryAgents, Module: fakeModule("ag-x"), Default: true},
			{Name: "ag-y", Category: manifest.CategoryAgents, Module: fakeModule("ag-y"), Default: true},
			{Name: "in-z", Category: manifest.CategoryIntelligence, Module: fakeModule("in-z"), Requires: []string{"ag-x"}},
		},
	}
}

func list(names ...string) *[]string { return &names }

func TestSelect(t *testing.T) {
	for name, tc := range map[string]struct {
		in   *manifest.Integrations
		want []string
	}{
		"absent key":          {nil, []string{"ag-x", "ag-y"}},
		"absent categories":   {&manifest.Integrations{}, []string{"ag-x", "ag-y"}},
		"no agents":           {&manifest.Integrations{Agents: list()}, []string{}},
		"editors add":         {&manifest.Integrations{Editors: list("ed-a")}, []string{"ed-a", "ag-x", "ag-y"}},
		"catalog order":       {&manifest.Integrations{Editors: list("ed-a"), Agents: list("ag-y", "ag-x")}, []string{"ed-a", "ag-x", "ag-y"}},
		"requires satisfied":  {&manifest.Integrations{Agents: list("ag-x"), Intelligence: list("in-z")}, []string{"ag-x", "in-z"}},
		"excludes avoided":    {&manifest.Integrations{Editors: list("ed-b"), Agents: list("ag-x")}, []string{"ed-b", "ag-x"}},
		"everything reversed": {&manifest.Integrations{Editors: list("ed-a"), Agents: list("ag-x"), Intelligence: list("in-z")}, []string{"ed-a", "ag-x", "in-z"}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := fakeStandard().Select(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || !slices.Equal(got, tc.want) {
				t.Errorf("Select = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSelectRejects(t *testing.T) {
	for name, tc := range map[string]struct {
		in   *manifest.Integrations
		want string
	}{
		"unknown":       {&manifest.Integrations{Agents: list("cursor")}, "integrations.agents[0] (cursor): unknown agents integration (valid: ag-x, ag-y)"},
		"wrong bucket":  {&manifest.Integrations{Editors: list("ag-x")}, "unknown editors integration (valid: ed-a, ed-b)"},
		"requires":      {&manifest.Integrations{Agents: list("ag-y"), Intelligence: list("in-z")}, "in-z requires ag-x"},
		"excludes":      {&manifest.Integrations{Editors: list("ed-b")}, "ed-b cannot be selected together with ag-y"},
		"no intel real": {&manifest.Integrations{Intelligence: list("gitnexus")}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			s := fakeStandard()
			if name == "no intel real" {
				s = mustLookup(t, "prod-go")
				tc.want = "integrations.intelligence[0] (gitnexus): no code-intelligence providers are available yet"
			}
			_, err := s.Select(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestModulesForCoreThenCatalogOrder(t *testing.T) {
	var got []string
	for _, m := range fakeStandard().ModulesFor([]string{"ag-y", "ed-a"}) {
		got = append(got, m.Name())
	}
	if want := []string{"core", "ed-a", "ag-y"}; !slices.Equal(got, want) {
		t.Errorf("ModulesFor = %v, want %v", got, want)
	}
}

// TestAgentModulesAreNotCore: an agent module left in a core list could
// never be deselected, and would resolve twice with the defaults.
func TestAgentModulesAreNotCore(t *testing.T) {
	for k, s := range registry {
		for _, m := range s.Modules {
			for _, it := range s.Integrations {
				if it.Module.Name() == m.Name() {
					t.Errorf("%s/%s: %s is both core and the %s integration", k.name, k.version, m.Name(), it.Name)
				}
			}
		}
		if got := s.Defaults(); !slices.Equal(got, []string{"claude", "codex"}) {
			t.Errorf("%s/%s defaults %v, want [claude codex]", k.name, k.version, got)
		}
	}
}

func mustLookup(t *testing.T, name string) *Standard {
	t.Helper()
	s, err := Lookup(name, "v1")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
