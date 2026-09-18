package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
)

func TestInitCmdWritesManifest(t *testing.T) {
	dir := t.TempDir()

	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"init", "production", "v1", "--repo-root", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("init returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "vibe.yaml"))
	if err != nil {
		t.Fatalf("reading vibe.yaml: %v", err)
	}

	m, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("parsing written vibe.yaml: %v", err)
	}
	if m.Standard != "production" || m.Version != "v1" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestInitCmdFailsIfManifestExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "vibe.yaml")
	if err := os.WriteFile(existing, []byte("standard: production\nversion: v1\n"), 0o644); err != nil {
		t.Fatalf("seeding existing vibe.yaml: %v", err)
	}

	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"init", "production", "v2", "--repo-root", dir})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error when vibe.yaml already exists, got nil")
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("reading vibe.yaml: %v", err)
	}
	if string(data) != "standard: production\nversion: v1\n" {
		t.Fatalf("existing vibe.yaml was modified: %s", data)
	}
}

func TestInitCmdRequiresTwoArgs(t *testing.T) {
	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"init", "production"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for missing version argument, got nil")
	}
}
