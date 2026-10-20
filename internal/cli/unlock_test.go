package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// unlockResult is what one vibe unlock run printed and returned.
type unlockResult struct {
	stdout string
	stderr string
	err    error
}

// runVibeUnlock runs vibe unlock in-process with args after "unlock".
func runVibeUnlock(t *testing.T, args ...string) unlockResult {
	t.Helper()
	root := NewRootCmd("test")
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"unlock"}, args...))
	err := root.Execute()
	return unlockResult{stdout: out.String(), stderr: errOut.String(), err: err}
}

func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "-q")
	return dir
}

// readMarker reads the unlock marker's expiry from gitDir.
func readMarker(t *testing.T, gitDir string) time.Time {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(gitDir, "vibe-unlock"))
	if err != nil {
		t.Fatalf("the marker was not written: %v", err)
	}
	s := string(data)
	if strings.Count(s, "\n") != 1 || !strings.HasSuffix(s, "\n") {
		t.Fatalf("the marker is not one line: %q", s)
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSuffix(s, "\n"))
	if err != nil {
		t.Fatalf("the marker is not RFC 3339: %q: %v", s, err)
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "Z") {
		t.Errorf("the marker is not UTC: %q", s)
	}
	return at
}

func assertExpiryIn(t *testing.T, at time.Time, before time.Time, d time.Duration) {
	t.Helper()
	after := time.Now()
	lo, hi := before.Add(d).Truncate(time.Second), after.Add(d).Add(time.Second)
	if at.Before(lo) || at.After(hi) {
		t.Errorf("expiry %s is not %s from now (between %s and %s)", at, d, lo, hi)
	}
}

// TestUnlockDefaultsToFifteenMinutes: vibe unlock writes one RFC 3339 UTC
// line, fifteen minutes ahead, to vibe-unlock in the common Git
// directory, and prints the expiry and that the next Stop locks again.
func TestUnlockDefaultsToFifteenMinutes(t *testing.T) {
	t.Parallel()
	dir := gitInit(t)
	before := time.Now()
	r := runVibeUnlock(t, "--repo-root", dir)
	if r.err != nil {
		t.Fatalf("vibe unlock: %v\n%s", r.err, r.stderr)
	}
	at := readMarker(t, filepath.Join(dir, ".git"))
	assertExpiryIn(t, at, before, 15*time.Minute)
	if !strings.Contains(r.stdout, at.UTC().Format(time.RFC3339)) {
		t.Errorf("the output does not name the expiry %s:\n%s", at.UTC().Format(time.RFC3339), r.stdout)
	}
	for _, want := range []string{"next Stop", "vibe-guard relock"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the output does not say %q:\n%s", want, r.stdout)
		}
	}
}

// TestUnlockMinutesAndReplace: --minutes sets the expiry, and a second
// unlock replaces the first.
func TestUnlockMinutesAndReplace(t *testing.T) {
	t.Parallel()
	dir := gitInit(t)
	if r := runVibeUnlock(t, "--repo-root", dir, "--minutes", "240"); r.err != nil {
		t.Fatalf("vibe unlock: %v", r.err)
	}
	before := time.Now()
	if r := runVibeUnlock(t, "--repo-root", dir, "--minutes", "1"); r.err != nil {
		t.Fatalf("vibe unlock: %v", r.err)
	}
	assertExpiryIn(t, readMarker(t, filepath.Join(dir, ".git")), before, time.Minute)
}

// TestUnlockMinutesOutOfRange: N must be 1 to 240; nothing is written
// otherwise.
func TestUnlockMinutesOutOfRange(t *testing.T) {
	t.Parallel()
	for _, n := range []string{"0", "241", "-5", "x"} {
		t.Run(n, func(t *testing.T) {
			dir := gitInit(t)
			r := runVibeUnlock(t, "--repo-root", dir, "--minutes", n)
			if r.err == nil {
				t.Fatalf("vibe unlock --minutes %s succeeded", n)
			}
			if _, err := os.Stat(filepath.Join(dir, ".git", "vibe-unlock")); err == nil {
				t.Errorf("a marker was written for --minutes %s", n)
			}
		})
	}
}

// TestUnlockLinkedWorktreeSharesCommonDir: from a linked worktree, the
// marker goes into the main repository's Git directory, which every
// worktree shares.
func TestUnlockLinkedWorktreeSharesCommonDir(t *testing.T) {
	t.Parallel()
	dir := gitInit(t)
	fixtureGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	fixtureGit(t, dir, "worktree", "add", "-q", wt)
	before := time.Now()
	if r := runVibeUnlock(t, "--repo-root", wt); r.err != nil {
		t.Fatalf("vibe unlock: %v\n%s", r.err, r.stderr)
	}
	assertExpiryIn(t, readMarker(t, filepath.Join(dir, ".git")), before, 15*time.Minute)
}

// TestUnlockOutsideGit: a directory that is not a Git repository is an
// error, and nothing is written.
func TestUnlockOutsideGit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	r := runVibeUnlock(t, "--repo-root", dir)
	if r.err == nil {
		t.Fatal("vibe unlock outside a Git repository succeeded")
	}
	if _, err := os.Stat(filepath.Join(dir, "vibe-unlock")); err == nil {
		t.Error("a marker was written outside Git")
	}
}
