package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/gotooling"
)

func goToolingContent(t *testing.T) []byte {
	t.Helper()
	resources, err := gotooling.New().Resolve(context.Background(), &module.Context{})
	if err != nil {
		t.Fatalf("resolving go-tooling module: %v", err)
	}
	return resources[0].Content
}

func writeManifest(t *testing.T, dir string) {
	t.Helper()
	manifestPath := filepath.Join(dir, "vibe.yaml")
	if err := os.WriteFile(manifestPath, []byte("standard: production\nversion: v1\n"), 0o644); err != nil {
		t.Fatalf("seeding vibe.yaml: %v", err)
	}
}

func TestDiffCmdReportsCreateWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)

	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"diff", "--repo-root", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("diff returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"standard: production/v1", ".golangci.yml: create (no file on disk)"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("diff output missing %q\n%s", want, got)
		}
	}
}

func TestDiffCmdReportsNoChangeWhenFileMatchesTarget(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), goToolingContent(t), 0o644); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}

	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"diff", "--repo-root", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("diff returned error: %v", err)
	}

	if want := ".golangci.yml: no change"; !bytes.Contains(out.Bytes(), []byte(want)) {
		t.Errorf("diff output missing %q\n%s", want, out.String())
	}
}

func TestDiffCmdReportsConflictWhenFileDiffersWithNoPriorState(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte("hand-authored: true\n"), 0o644); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}

	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"diff", "--repo-root", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("diff returned error: %v", err)
	}

	if want := ".golangci.yml: conflict: manual changes detected, review before sync"; !bytes.Contains(out.Bytes(), []byte(want)) {
		t.Errorf("diff output missing %q\n%s", want, out.String())
	}
}

func TestDiffCmdFailsIfManifestMissing(t *testing.T) {
	dir := t.TempDir()

	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"diff", "--repo-root", dir})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error when vibe.yaml is missing, got nil")
	}
}

func TestDiffCmdFailsIfStandardUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte("standard: production\nversion: v99\n"), 0o644); err != nil {
		t.Fatalf("seeding vibe.yaml: %v", err)
	}

	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"diff", "--repo-root", dir})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown standard/version, got nil")
	}
}
