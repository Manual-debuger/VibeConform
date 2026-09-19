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
