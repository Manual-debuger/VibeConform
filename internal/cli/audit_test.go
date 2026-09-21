package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func runAuditIn(t *testing.T, dir string) (string, error) {
	t.Helper()
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"audit", "--repo-root", dir})

	err := root.Execute()
	return out.String(), err
}

func assertAuditOutput(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !bytes.Contains([]byte(out), []byte(w)) {
			t.Errorf("audit output missing %q\n%s", w, out)
		}
	}
}

func TestAuditCmdConformantRepo(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	// A synced repository is the definition of a conformant one; seeding via
	// sync keeps this test correct as modules are added to the standard.
	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("seeding sync: %v", err)
	}

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 0 {
		t.Fatalf("ExitCode = %d, want 0 (err %v)\n%s", code, err, out)
	}

	assertAuditOutput(t, out,
		"standard: prod-go/v1",
		".golangci.yml: ok",
		"drifted, 0 conflicts",
		"conformant",
	)
}

func TestAuditCmdReportsMissingResource(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err %v)\n%s", code, err, out)
	}

	assertAuditOutput(t, out, ".golangci.yml: missing (run vibe sync)", "not conformant")
}

func TestAuditCmdReportsDriftAgainstRecordedState(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("seeding sync: %v", err)
	}

	// Drift exactly one resource: file and recorded state agree with each
	// other but no longer with the module's target.
	stale := []byte("previously-applied: true\n")
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), stale, 0o600); err != nil {
		t.Fatalf("drifting .golangci.yml: %v", err)
	}
	recordState(t, dir, ".golangci.yml", sha256Hex(stale))

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err %v)\n%s", code, err, out)
	}

	assertAuditOutput(t, out, ".golangci.yml: drifted (run vibe sync)", "1 drifted", "not conformant")
}

func TestAuditCmdReportsConflict(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte("hand-authored: true\n"), 0o600); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err %v)\n%s", code, err, out)
	}

	assertAuditOutput(t, out, ".golangci.yml: conflict: manual changes detected", "1 conflicts", "not conformant")
}

func TestAuditCmdFailsIfManifestMissing(t *testing.T) {
	dir := t.TempDir()

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 1 {
		t.Fatalf("ExitCode = %d, want 1 — a missing manifest is a tool failure, not drift (err %v)\n%s", code, err, out)
	}
}

func TestAuditCmdFailsIfStandardUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte("standard: prod-go\nversion: v99\n"), 0o600); err != nil {
		t.Fatalf("seeding vibe.yaml: %v", err)
	}

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 1 {
		t.Fatalf("ExitCode = %d, want 1 (err %v)\n%s", code, err, out)
	}
}
