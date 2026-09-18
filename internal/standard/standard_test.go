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
