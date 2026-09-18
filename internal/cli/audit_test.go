package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditCmdReportsZeroModules(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "vibe.yaml")
	if err := os.WriteFile(manifestPath, []byte("standard: production\nversion: v1\n"), 0o644); err != nil {
		t.Fatalf("seeding vibe.yaml: %v", err)
	}

	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"audit", "--repo-root", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("audit returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"standard: production/v1", "0 modules configured"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("audit output missing %q\n%s", want, got)
		}
	}
}

func TestAuditCmdFailsIfManifestMissing(t *testing.T) {
	dir := t.TempDir()

	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"audit", "--repo-root", dir})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error when vibe.yaml is missing, got nil")
	}
}

func TestAuditCmdFailsIfStandardUnknown(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "vibe.yaml")
	if err := os.WriteFile(manifestPath, []byte("standard: production\nversion: v99\n"), 0o644); err != nil {
		t.Fatalf("seeding vibe.yaml: %v", err)
	}

	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"audit", "--repo-root", dir})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown standard/version, got nil")
	}
}
