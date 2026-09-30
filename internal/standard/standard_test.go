package standard

import (
	"slices"
	"testing"
)

func TestLookupHit(t *testing.T) {
	s, err := Lookup("prod-go", "v1")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if s.Name != "prod-go" || s.Version != "v1" {
		t.Fatalf("unexpected standard: %+v", s)
	}
}

// TestLookupProdGoV1ModulesInOrder pins the module order, not just the
// set: audit, diff, and sync all report in module order, so reordering here
// silently reorders every report.
func TestLookupProdGoV1ModulesInOrder(t *testing.T) {
	s, err := Lookup("prod-go", "v1")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}

	// Since spec 0026 the agent modules are default-on integrations; with
	// defaults the order is exactly what it was before.
	mods := s.ModulesFor(s.Defaults())
	want := []string{"go-tooling", "github-ci", "vibe-conformance", "repo-tooling", "claude-config", "codex-config"}
	if len(mods) != len(want) {
		t.Fatalf("prod-go/v1 has %d modules, want %d", len(mods), len(want))
	}
	for i, name := range want {
		if got := mods[i].Name(); got != name {
			t.Errorf("module[%d].Name() = %q, want %q", i, got, name)
		}
	}
}

func TestLookupMiss(t *testing.T) {
	if _, err := Lookup("prod-go", "v99"); err == nil {
		t.Fatal("expected error for unknown standard, got nil")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for duplicate registration, got none")
		}
	}()
	Register(Standard{Name: "prod-go", Version: "v1"})
}

// TestLookupProdMonoV1 pins prod-mono's module order and that it is the
// one standard resolving from vibe.yaml's components (spec 0025).
func TestLookupProdMonoV1(t *testing.T) {
	s, err := Lookup("prod-mono", "v1")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if !s.TakesComponents {
		t.Error("prod-mono/v1 does not take components")
	}
	want := []string{"mono-tooling", "github-ci-mono", "vibe-conformance", "mono-repo-tooling", "claude-config", "codex-config"}
	var got []string
	for _, m := range s.ModulesFor(s.Defaults()) {
		got = append(got, m.Name())
	}
	if !slices.Equal(got, want) {
		t.Errorf("modules %v, want %v", got, want)
	}
}

func TestOnlyProdMonoTakesComponents(t *testing.T) {
	for k, s := range registry {
		if s.TakesComponents != (k.name == "prod-mono") {
			t.Errorf("%s/%s: TakesComponents = %v", k.name, k.version, s.TakesComponents)
		}
	}
}
