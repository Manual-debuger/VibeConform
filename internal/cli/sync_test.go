package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/state"
)

func runSyncIn(t *testing.T, dir string) (string, error) {
	t.Helper()
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"sync", "--repo-root", dir})

	err := root.Execute()
	return out.String(), err
}

func seedState(t *testing.T, dir, path, sha string) {
	t.Helper()
	if err := state.Save(dir, &state.State{Resources: map[string]state.ResourceState{
		path: {SHA256: sha},
	}}); err != nil {
		t.Fatalf("seeding state: %v", err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestSyncCmdCreatesResourceAndRecordsState(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	want := goToolingContent(t)

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("sync returned error: %v\n%s", err, out)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".golangci.yml"))
	if err != nil {
		t.Fatalf("reading written resource: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("written .golangci.yml does not match the module's resolved content")
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if recorded := s.Resources[".golangci.yml"].SHA256; recorded != sha256Hex(want) {
		t.Errorf("recorded hash = %q, want %q", recorded, sha256Hex(want))
	}

	for _, wantLine := range []string{
		"standard: production/v1",
		".golangci.yml: created",
		"1 created, 0 updated, 0 unchanged, 0 conflicts",
	} {
		if !bytes.Contains([]byte(out), []byte(wantLine)) {
			t.Errorf("sync output missing %q\n%s", wantLine, out)
		}
	}
}

func TestSyncCmdIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)

	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("first sync returned error: %v", err)
	}
	firstState, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
	if err != nil {
		t.Fatalf("reading state after first sync: %v", err)
	}

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("second sync returned error: %v\n%s", err, out)
	}

	if want := ".golangci.yml: unchanged"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
	if want := "0 created, 0 updated, 1 unchanged, 0 conflicts"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}

	secondState, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
	if err != nil {
		t.Fatalf("reading state after second sync: %v", err)
	}
	if !bytes.Equal(firstState, secondState) {
		t.Errorf("state changed across identical runs:\nfirst:\n%s\nsecond:\n%s", firstState, secondState)
	}
}

func TestSyncCmdRefusesToOverwriteConflict(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	handEdited := []byte("hand-authored: true\n")
	resourceFile := filepath.Join(dir, ".golangci.yml")
	if err := os.WriteFile(resourceFile, handEdited, 0o600); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}

	out, err := runSyncIn(t, dir)
	if err == nil {
		t.Fatalf("expected a non-zero exit for a conflict, got nil\n%s", out)
	}

	got, readErr := os.ReadFile(resourceFile)
	if readErr != nil {
		t.Fatalf("reading conflicted file: %v", readErr)
	}
	if !bytes.Equal(got, handEdited) {
		t.Error("sync overwrote a conflicted file; it must be left exactly as found")
	}

	if want := ".golangci.yml: conflict"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
}

func TestSyncCmdOverwritesDriftFromRecordedState(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	target := goToolingContent(t)

	// The file on disk still matches what was last applied, while the
	// module's target has moved on: reconcile's safe-replacement row.
	stale := []byte("previously-applied: true\n")
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), stale, 0o600); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}
	seedState(t, dir, ".golangci.yml", sha256Hex(stale))

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("sync returned error: %v\n%s", err, out)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".golangci.yml"))
	if err != nil {
		t.Fatalf("reading written resource: %v", err)
	}
	if !bytes.Equal(got, target) {
		t.Error("sync did not replace a stale file with the module's target content")
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if recorded := s.Resources[".golangci.yml"].SHA256; recorded != sha256Hex(target) {
		t.Errorf("recorded hash = %q, want the target's %q", recorded, sha256Hex(target))
	}

	if want := ".golangci.yml: updated"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
	if want := "0 created, 1 updated, 0 unchanged, 0 conflicts"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
}

func TestSyncCmdFailsIfManifestMissing(t *testing.T) {
	dir := t.TempDir()

	if _, err := runSyncIn(t, dir); err == nil {
		t.Fatal("expected error when vibe.yaml is missing, got nil")
	}

	if _, err := os.Stat(filepath.Join(dir, ".vibe")); !os.IsNotExist(err) {
		t.Errorf("sync should not create .vibe/ when it cannot read the manifest, stat err = %v", err)
	}
}

func TestSyncCmdFailsIfStandardUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte("standard: production\nversion: v99\n"), 0o600); err != nil {
		t.Fatalf("seeding vibe.yaml: %v", err)
	}

	if _, err := runSyncIn(t, dir); err == nil {
		t.Fatal("expected error for unknown standard/version, got nil")
	}
}
