package guardrule_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// git runs git in dir and returns its trimmed output. ok is false when
// git fails.
func git(t *testing.T, dir string, args ...string) (out string, ok bool) {
	t.Helper()
	cmd := exec.Command("git", args...) // #nosec G204 -- a test's own arguments
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("git %v: %v\n%s", args, err, b)
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, ok := git(t, dir, args...)
	if !ok {
		t.Fatalf("git %v failed", args)
	}
	return out
}

// gitInitWithCommit makes a repository with one commit, so that a
// linked worktree can be added to it.
func gitInitWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")
	mustGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	return dir
}

// gitCommonDir is git rev-parse --git-common-dir run in dir, absolute.
func gitCommonDir(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.FromSlash(mustGit(t, dir, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p
}

func assertSameDir(t *testing.T, what, got, want string) {
	t.Helper()
	if got == "" {
		t.Fatalf("%s: none found, git says %s", what, want)
	}
	gi, err := os.Stat(got)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	wi, err := os.Stat(want)
	if err != nil {
		t.Fatalf("%s: git's %v", what, err)
	}
	if !os.SameFile(gi, wi) {
		t.Errorf("%s is %s, git says %s", what, got, want)
	}
}

// TestFindRepoAgreesWithGit: the Git-directory finder vibe-guard and
// vibe unlock share finds the common Git directory that
// git rev-parse --git-common-dir names, for every layout the guard
// supports (spec 0069, bug 3).
func TestFindRepoAgreesWithGit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	layouts := []struct {
		name string
		// make builds the layout and returns the directory to start in.
		make func(t *testing.T) string
	}{
		{"plain repository", func(t *testing.T) string {
			dir := t.TempDir()
			mustGit(t, dir, "init", "-q")
			return dir
		}},
		{"linked worktree", func(t *testing.T) string {
			wt := filepath.Join(t.TempDir(), "wt")
			mustGit(t, gitInitWithCommit(t), "worktree", "add", "-q", wt)
			return wt
		}},
		{"separate Git directory", func(t *testing.T) string {
			wt := filepath.Join(t.TempDir(), "wt")
			mustGit(t, t.TempDir(), "init", "-q", "--separate-git-dir", filepath.Join(t.TempDir(), "repo.git"), wt)
			return wt
		}},
		{"worktree with relative paths", func(t *testing.T) string {
			wt := filepath.Join(t.TempDir(), "wt")
			if _, ok := git(t, gitInitWithCommit(t), "worktree", "add", "-q", "--relative-paths", wt); !ok {
				t.Skip("this git has no worktree add --relative-paths")
			}
			return wt
		}},
		{"project below the Git top", func(t *testing.T) string {
			top := t.TempDir()
			mustGit(t, top, "init", "-q")
			proj := filepath.Join(top, "a", "proj")
			if err := os.MkdirAll(proj, 0o750); err != nil {
				t.Fatal(err)
			}
			return proj
		}},
	}
	for _, l := range layouts {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			start := l.make(t)
			r := gr.FindRepo(start, start)
			if r.Root != start {
				t.Errorf("project root is %s, want %s", r.Root, start)
			}
			assertSameDir(t, "Git top", r.GitTop, filepath.FromSlash(mustGit(t, start, "rev-parse", "--show-toplevel")))
			assertSameDir(t, "Git directory", r.GitDir, filepath.FromSlash(mustGit(t, start, "rev-parse", "--absolute-git-dir")))
			assertSameDir(t, "common Git directory", r.CommonDir, gitCommonDir(t, start))
		})
	}
}

// TestFindRepoWithoutGit: without .git above the start, there is no Git
// top and no Git directory; the project root is root, else start.
func TestFindRepoWithoutGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if r := gr.FindRepo(dir, ""); r != (gr.Repo{Root: dir}) {
		t.Errorf("FindRepo(%s, \"\") = %+v, want only the root", dir, r)
	}
	if r := gr.FindRepo(dir, "x"); r != (gr.Repo{Root: "x"}) {
		t.Errorf("FindRepo(%s, \"x\") = %+v, want only the root x", dir, r)
	}
}

// TestFindRepoUnreadableGitFile: a .git file without a gitdir: line, or
// a commondir file that is empty, leaves the directories it would name
// unknown, so no unlock marker is read or written.
func TestFindRepoUnreadableGitFile(t *testing.T) {
	t.Parallel()
	t.Run("no gitdir line", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".git"), "not a gitdir line\n")
		r := gr.FindRepo(dir, "")
		if r.GitTop != dir || r.GitDir != "" || r.CommonDir != "" {
			t.Errorf("FindRepo = %+v, want the Git top %s and no Git directory", r, dir)
		}
	})
	t.Run("empty commondir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		gitDir := filepath.Join(dir, ".git")
		if err := os.MkdirAll(gitDir, 0o750); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(gitDir, "commondir"), "\n")
		r := gr.FindRepo(dir, "")
		if r.GitDir != gitDir || r.CommonDir != "" {
			t.Errorf("FindRepo = %+v, want the Git directory %s and no common one", r, gitDir)
		}
	})
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
