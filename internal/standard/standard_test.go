package standard

import "testing"

func TestLookupHit(t *testing.T) {
	s, err := Lookup("production", "v1")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if s.Name != "production" || s.Version != "v1" {
		t.Fatalf("unexpected standard: %+v", s)
	}
}

// TestLookupProductionV1ModulesInOrder pins the module order, not just the
// set: audit, diff, and sync all report in module order, so reordering here
// silently reorders every report.
func TestLookupProductionV1ModulesInOrder(t *testing.T) {
	s, err := Lookup("production", "v1")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}

	want := []string{"go-tooling", "github-ci", "repo-tooling", "agent-config"}
	if len(s.Modules) != len(want) {
		t.Fatalf("production/v1 has %d modules, want %d", len(s.Modules), len(want))
	}
	for i, name := range want {
		if got := s.Modules[i].Name(); got != name {
			t.Errorf("module[%d].Name() = %q, want %q", i, got, name)
		}
	}
}

func TestLookupMiss(t *testing.T) {
	if _, err := Lookup("production", "v99"); err == nil {
		t.Fatal("expected error for unknown standard, got nil")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for duplicate registration, got none")
		}
	}()
	Register(Standard{Name: "production", Version: "v1"})
}
