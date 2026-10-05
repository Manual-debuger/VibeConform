package cli

import (
	"strings"
	"testing"
)

const defaultsPath = ".gitlab-ci.defaults.yml"

// TestGitLabDefaultsShape pins spec 0040 §2 end to end: VibeConform never
// writes .gitlab-ci.defaults.yml, conforms with it absent or holding only
// default:, and reports any other top-level key as a conflict in audit,
// diff and sync.
func TestGitLabDefaultsShape(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, withProvider("gitlab"))
	out := mustSync(t, dir)
	if exists(t, dir, defaultsPath) || strings.Contains(out, defaultsPath) {
		t.Fatalf("sync wrote or mentioned an absent %s:\n%s", defaultsPath, out)
	}
	if out, err := runAuditIn(t, dir); err != nil || strings.Contains(out, defaultsPath) {
		t.Fatalf("audit with no defaults file: %v\n%s", err, out)
	}

	good := "default:\n  tags: [linux]\n  retry: 1\n"
	writeFile(t, dir, defaultsPath, good)
	out, err := runAuditIn(t, dir)
	if err != nil || !strings.Contains(out, defaultsPath+": ok (project-owned)") {
		t.Fatalf("audit with default: only: %v\n%s", err, out)
	}

	bad := "default:\n  tags: [linux]\napi:\n  allow_failure: true\nvariables:\n  X: y\n"
	writeFile(t, dir, defaultsPath, bad)
	out, err = runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("audit exit %d, want 2: %v\n%s", code, err, out)
	}
	for _, key := range []string{`"api"`, `"variables"`} {
		if !strings.Contains(out, defaultsPath+": conflict: top-level key "+key) {
			t.Errorf("audit does not name %s:\n%s", key, out)
		}
	}
	if !strings.Contains(out, "not conformant") {
		t.Errorf("audit verdict:\n%s", out)
	}
	if out := runDiffIn(t, dir); !strings.Contains(out, defaultsPath+": conflict: top-level key \"api\"") {
		t.Errorf("diff does not report the conflict:\n%s", out)
	}

	// Sync reports it, exits non-zero, leaves the file alone, and still
	// applies everything else.
	writeFile(t, dir, ".gitlab-ci.yml", "# edited\n")
	out, err = runSyncIn(t, dir)
	if err == nil || !strings.Contains(out, defaultsPath+": conflict:") || !strings.Contains(out, "1 conflicts") {
		t.Errorf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, dir, defaultsPath); got != bad {
		t.Errorf("sync changed %s: %q", defaultsPath, got)
	}
	if got := readFile(t, dir, ".gitlab-ci.yml"); got == "# edited\n" {
		t.Error("sync did not restore .gitlab-ci.yml beside the guard conflict")
	}

	writeFile(t, dir, defaultsPath, good)
	mustConform(t, dir)
}

// TestGitLabDefaultsIgnoredElsewhere: under github and none the file is
// project business like any other, never read.
func TestGitLabDefaultsIgnoredElsewhere(t *testing.T) {
	for _, provider := range []string{"", "none"} {
		dir := t.TempDir()
		writeVibeYAML(t, dir, withProvider(provider))
		mustSync(t, dir)
		writeFile(t, dir, defaultsPath, "api:\n  allow_failure: true\n")
		if out, err := runAuditIn(t, dir); err != nil || strings.Contains(out, defaultsPath) {
			t.Errorf("provider %q: audit: %v\n%s", provider, err, out)
		}
	}
}
