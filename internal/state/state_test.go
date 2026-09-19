package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyState(t *testing.T) {
	dir := t.TempDir()

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s == nil || s.Resources == nil {
		t.Fatal("Load: expected non-nil State with non-nil Resources map")
	}
	if len(s.Resources) != 0 {
		t.Fatalf("Load: expected empty Resources, got %v", s.Resources)
	}
}

func TestLoadMalformedFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o755); err != nil {
		t.Fatalf("seeding .vibe dir: %v", err)
	}
	path := filepath.Join(dir, ".vibe", "state.yaml")
	if err := os.WriteFile(path, []byte("resources: [not, a, map]"), 0o644); err != nil {
		t.Fatalf("seeding state.yaml: %v", err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("Load: expected error for malformed state.yaml, got nil")
	}
}

func TestLoadValidFileParsesResources(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o755); err != nil {
		t.Fatalf("seeding .vibe dir: %v", err)
	}
	path := filepath.Join(dir, ".vibe", "state.yaml")
	content := "resources:\n  .golangci.yml:\n    sha256: deadbeef\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seeding state.yaml: %v", err)
	}

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := s.Resources[".golangci.yml"]
	if !ok {
		t.Fatalf("Load: expected .golangci.yml entry, got %v", s.Resources)
	}
	if got.SHA256 != "deadbeef" {
		t.Errorf("Load: SHA256 = %q, want %q", got.SHA256, "deadbeef")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := &State{Resources: map[string]ResourceState{
		".golangci.yml":            {SHA256: "aaa"},
		".github/workflows/ci.yml": {SHA256: "bbb"},
	}}

	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Resources) != len(want.Resources) {
		t.Fatalf("Load: got %v, want %v", got.Resources, want.Resources)
	}
	for path, rs := range want.Resources {
		if got.Resources[path] != rs {
			t.Errorf("Load: %s = %v, want %v", path, got.Resources[path], rs)
		}
	}
}

func TestSaveCreatesStateDirectory(t *testing.T) {
	dir := t.TempDir()

	if err := Save(dir, &State{Resources: map[string]ResourceState{".golangci.yml": {SHA256: "abc"}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".vibe", "state.yaml")); err != nil {
		t.Fatalf("expected .vibe/state.yaml to exist: %v", err)
	}
}

func TestSaveReplacesExistingState(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, &State{Resources: map[string]ResourceState{"old.yml": {SHA256: "old"}}}); err != nil {
		t.Fatalf("seeding Save: %v", err)
	}

	if err := Save(dir, &State{Resources: map[string]ResourceState{"new.yml": {SHA256: "new"}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, stale := got.Resources["old.yml"]; stale {
		t.Errorf("Save: stale entry survived a rewrite: %v", got.Resources)
	}
	if got.Resources["new.yml"].SHA256 != "new" {
		t.Errorf("Save: new.yml = %v, want sha256 %q", got.Resources["new.yml"], "new")
	}
}

func TestSaveEmptyStateLoadsAsEmptyMap(t *testing.T) {
	dir := t.TempDir()

	if err := Save(dir, &State{Resources: map[string]ResourceState{}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Resources == nil {
		t.Fatal("Load: Resources map should be non-nil after saving an empty state")
	}
	if len(got.Resources) != 0 {
		t.Errorf("Load: got %v, want empty", got.Resources)
	}
}
