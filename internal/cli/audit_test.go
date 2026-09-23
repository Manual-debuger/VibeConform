package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
		"0 drifted, 0 out of date, 0 conflicts",
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

// TestAuditCmdReportsOutOfDateWhenStandardMoved covers the case issue #22
// is about: the repository is exactly as VibeConform last wrote it, and the
// module's target has moved on. Nobody edited anything, so audit must not
// say "drifted".
//
// This test previously asserted the opposite string under the name
// TestAuditCmdReportsDriftAgainstRecordedState. Its setup was always this
// case — file and recorded state agreeing with each other but not with the
// target — so it was pinning the misreport rather than catching it.
func TestAuditCmdReportsOutOfDateWhenStandardMoved(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("seeding sync: %v", err)
	}

	// File and recorded state agree with each other but no longer with the
	// module's target: C == P, T != P.
	stale := []byte("previously-applied: true\n")
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), stale, 0o600); err != nil {
		t.Fatalf("staling .golangci.yml: %v", err)
	}
	recordState(t, dir, ".golangci.yml", sha256Hex(stale))

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err %v)\n%s", code, err, out)
	}

	assertAuditOutput(t, out,
		".golangci.yml: out of date (standard moved; run vibe sync to update)",
		"1 out of date", "not conformant")
	if strings.Contains(out, ".golangci.yml: drifted") {
		t.Errorf("audit blamed the user for a file they did not touch:\n%s", out)
	}
}

// TestAuditCmdReportsLocalDriftWhenFileEdited is the counterpart: the
// target has not moved, but the managed file was edited since it was last
// applied. Without this, deleting the LocalDrift branch would leave
// TestAuditCmdReportsOutOfDateWhenStandardMoved passing.
func TestAuditCmdReportsLocalDriftWhenFileEdited(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("seeding sync: %v", err)
	}

	// Edit the file only, leaving recorded state matching the target:
	// C != P, T == P.
	edited := []byte("hand-edited: true\n")
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), edited, 0o600); err != nil {
		t.Fatalf("editing .golangci.yml: %v", err)
	}

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err %v)\n%s", code, err, out)
	}

	assertAuditOutput(t, out,
		".golangci.yml: drifted (edited since last sync; run vibe sync to restore)",
		"1 drifted", "not conformant")
	if strings.Contains(out, "out of date (standard moved") {
		t.Errorf("audit excused an edit as a moved standard:\n%s", out)
	}
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
