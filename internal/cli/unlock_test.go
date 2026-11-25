package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
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
	contracttest.Git(t, dir, "init", "-q")
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
	contracttest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	contracttest.Git(t, dir, "worktree", "add", "-q", wt)
	before := time.Now()
	if r := runVibeUnlock(t, "--repo-root", wt); r.err != nil {
		t.Fatalf("vibe unlock: %v\n%s", r.err, r.stderr)
	}
	assertExpiryIn(t, readMarker(t, filepath.Join(dir, ".git")), before, 15*time.Minute)
}

// TestUnlockFixedClockWritesExactExpiry: vibe unlock reads the current
// time from the CLI's seams (spec 0070, "vibe unlock's clock"), so with a
// fixed clock the marker holds the exact expiry: now plus --minutes, in
// UTC, to the second.
func TestUnlockFixedClockWritesExactExpiry(t *testing.T) {
	t.Parallel()
	dir := gitInit(t)
	now := time.Date(2026, 3, 4, 13, 20, 5, 900_000_000, time.FixedZone("UTC+2", 2*60*60))
	root := newRootCmd("test", cliSeams{now: func() time.Time { return now }})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"unlock", "--repo-root", dir, "--minutes", "45"})
	if err := root.Execute(); err != nil {
		t.Fatalf("vibe unlock: %v\n%s", err, errOut.String())
	}
	marker := filepath.Join(dir, ".git", "vibe-unlock")
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("the marker was not written: %v", err)
	}
	if got, want := string(data), "2026-03-04T12:05:05Z\n"; got != want {
		t.Errorf("marker = %q, want %q", got, want)
	}
	if want := "unlocked until 2026-03-04T12:05:05Z (" + marker + ")\n"; !strings.HasPrefix(out.String(), want) {
		t.Errorf("the output does not start with %q:\n%s", want, out.String())
	}
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

// assertNoMarker fails when gitDir holds an unlock marker.
func assertNoMarker(t *testing.T, gitDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(gitDir, "vibe-unlock")); err == nil {
		t.Errorf("a marker was written in %s", gitDir)
	}
}

// TestUnlockSeparateGitDir: in a repository made with
// --separate-git-dir, the marker goes into the separate Git directory,
// which is where vibe-guard reads it (spec 0069).
func TestUnlockSeparateGitDir(t *testing.T) {
	t.Parallel()
	wt := filepath.Join(t.TempDir(), "wt")
	gitDir := filepath.Join(t.TempDir(), "repo.git")
	contracttest.Git(t, t.TempDir(), "init", "-q", "--separate-git-dir", gitDir, wt)
	before := time.Now()
	if r := runVibeUnlock(t, "--repo-root", wt); r.err != nil {
		t.Fatalf("vibe unlock: %v\n%s", r.err, r.stderr)
	}
	assertExpiryIn(t, readMarker(t, gitDir), before, 15*time.Minute)
}

// TestUnlockProjectBelowGitTop: a --repo-root below the Git top is
// walked up to the Git top, and the marker goes into its Git directory.
func TestUnlockProjectBelowGitTop(t *testing.T) {
	t.Parallel()
	top := gitInit(t)
	proj := filepath.Join(top, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	if r := runVibeUnlock(t, "--repo-root", proj); r.err != nil {
		t.Fatalf("vibe unlock: %v\n%s", r.err, r.stderr)
	}
	assertExpiryIn(t, readMarker(t, filepath.Join(top, ".git")), before, 15*time.Minute)
}

// TestUnlockIgnoresGitEnvironment: with GIT_DIR or GIT_COMMON_DIR set to
// another repository, vibe unlock still writes the marker in
// --repo-root's own common Git directory, where vibe-guard reads it, and
// nothing in the other repository (spec 0069, bug 3). Not parallel: it
// sets the environment.
func TestUnlockIgnoresGitEnvironment(t *testing.T) {
	for _, name := range []string{"GIT_DIR", "GIT_COMMON_DIR"} {
		t.Run(name, func(t *testing.T) {
			dir, other := gitInit(t), gitInit(t)
			t.Setenv(name, filepath.Join(other, ".git"))
			before := time.Now()
			if r := runVibeUnlock(t, "--repo-root", dir); r.err != nil {
				t.Fatalf("vibe unlock: %v\n%s", r.err, r.stderr)
			}
			assertExpiryIn(t, readMarker(t, filepath.Join(dir, ".git")), before, 15*time.Minute)
			assertNoMarker(t, filepath.Join(other, ".git"))
		})
	}
}

// TestUnlockWorkTreeWithoutDotGit: a work tree used with a bare Git
// directory through GIT_DIR has no .git, so vibe-guard finds no Git
// directory there. vibe unlock fails, writes nothing and says that
// GIT_DIR is not read. Not parallel: it sets the environment.
func TestUnlockWorkTreeWithoutDotGit(t *testing.T) {
	bare := filepath.Join(t.TempDir(), "dotfiles.git")
	contracttest.Git(t, t.TempDir(), "init", "-q", "--bare", bare)
	work := t.TempDir()
	t.Setenv("GIT_DIR", bare)
	t.Setenv("GIT_WORK_TREE", work)
	r := runVibeUnlock(t, "--repo-root", work)
	if r.err == nil {
		t.Fatal("vibe unlock in a work tree without .git succeeded")
	}
	for _, want := range []string{work, ".git", "vibe-guard", "GIT_DIR", "GIT_COMMON_DIR"} {
		if !strings.Contains(r.err.Error(), want) {
			t.Errorf("the message does not say %q: %v", want, r.err)
		}
	}
	assertNoMarker(t, bare)
	assertNoMarker(t, work)
}

// TestUnlockRunsNoGit: vibe unlock finds the common Git directory by
// reading files, as vibe-guard does, so it works without git on PATH.
// Not parallel: it sets the environment.
func TestUnlockRunsNoGit(t *testing.T) {
	dir := gitInit(t)
	t.Setenv("PATH", t.TempDir())
	before := time.Now()
	if r := runVibeUnlock(t, "--repo-root", dir); r.err != nil {
		t.Fatalf("vibe unlock without git on PATH: %v\n%s", r.err, r.stderr)
	}
	assertExpiryIn(t, readMarker(t, filepath.Join(dir, ".git")), before, 15*time.Minute)
}

// TestUnlockUnreadableDotGit: a .git that exists but leads to no common
// Git directory (a .git file without a "gitdir:" line, or a Git
// directory whose commondir file is empty) is an error that names that
// .git, in the Git top above --repo-root, and says it could not be read.
// It does not say there is no .git. Nothing is written.
func TestUnlockUnreadableDotGit(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, top string) (gitDir string){
		"gitdir file": func(t *testing.T, top string) string {
			writeTestFile(t, filepath.Join(top, ".git"), "not a gitdir line\n")
			return ""
		},
		"empty commondir": func(t *testing.T, top string) string {
			gitDir := filepath.Join(t.TempDir(), "wt-git")
			writeTestFile(t, filepath.Join(gitDir, "commondir"), "\n")
			writeTestFile(t, filepath.Join(top, ".git"), "gitdir: "+gitDir+"\n")
			return gitDir
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			top := t.TempDir()
			gitDir := setup(t, top)
			proj := filepath.Join(top, "proj")
			if err := os.MkdirAll(proj, 0o750); err != nil {
				t.Fatal(err)
			}
			r := runVibeUnlock(t, "--repo-root", proj)
			if r.err == nil {
				t.Fatal("vibe unlock with an unreadable .git succeeded")
			}
			for _, want := range []string{filepath.Join(top, ".git"), "could not be read", "GIT_DIR", "GIT_COMMON_DIR"} {
				if !strings.Contains(r.err.Error(), want) {
					t.Errorf("the message does not say %q: %v", want, r.err)
				}
			}
			if strings.Contains(r.err.Error(), "without .git") {
				t.Errorf("the message says there is no .git: %v", r.err)
			}
			assertNoMarker(t, top)
			assertNoMarker(t, proj)
			if gitDir != "" {
				assertNoMarker(t, gitDir)
			}
		})
	}
}
