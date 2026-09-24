package monotooling

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

var components = []manifest.Component{
	{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
	{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
	{ID: "worker", Path: "services/worker", Profile: manifest.ProfilePy},
}

// TestResolveReroots pins what a component gets: its profile's tooling
// module's files, at its path, byte for byte, in vibe.yaml order.
func TestResolveReroots(t *testing.T) {
	rs, err := New().Resolve(context.Background(), &module.Context{Components: components})
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, r := range rs {
		got = append(got, r.Path)
	}
	want := []string{
		"services/api/.golangci.yml",
		"apps/web/eslint.config.js", "apps/web/.prettierrc.json", "apps/web/tsconfig.base.json",
		"services/worker/ruff.toml", "services/worker/pyrightconfig.json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths %v, want %v", got, want)
	}

	i := 0
	for _, c := range components {
		source, err := tooling[c.Profile].Resolve(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range source {
			r := rs[i]
			i++
			if !bytes.Equal(r.Content, s.Content) || r.Ownership != s.Ownership || r.Mode != s.Mode {
				t.Errorf("%s differs from %s's %s", r.Path, tooling[c.Profile].Name(), s.Path)
			}
		}
	}
}

func TestResolveNeedsComponents(t *testing.T) {
	if _, err := New().Resolve(context.Background(), nil); err == nil {
		t.Fatal("Resolve succeeded with no components")
	}
}

func TestRequiredToolsFollowProfiles(t *testing.T) {
	names := func(cs []manifest.Component) []string {
		var out []string
		for _, tool := range New().(module.ToolRequirer).RequiredTools(&module.Context{Components: cs}) {
			out = append(out, tool.Name)
		}
		return out
	}
	if got := names(components[2:]); len(got) != 0 {
		t.Errorf("py only: %v, want none (python-tooling declares none)", got)
	}
	if got, want := names(components), []string{"golangci-lint", "node"}; !reflect.DeepEqual(got, want) {
		t.Errorf("all: %v, want %v", got, want)
	}
}
