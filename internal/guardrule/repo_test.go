package guardrule_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
	"github.com/Manual-debuger/VibeConform/internal/testkit"
)

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
		// make builds the layout and returns the directory to start in,
		// with the environment git runs with there.
		make func(t *testing.T) testkit.Repo
	}{
		{"plain repository", func(t *testing.T) testkit.Repo {
			return testkit.InitRepo(t)
		}},
		{"linked worktree", func(t *testing.T) testkit.Repo {
			r := testkit.InitWithCommit(t)
			wt := filepath.Join(t.TempDir(), "wt")
			r.Git(t, "worktree", "add", "-q", wt)
			return testkit.Repo{Dir: wt, Env: r.Env}
		}},
		{"separate Git directory", func(t *testing.T) testkit.Repo {
			r := testkit.NewRepo(t, "wt")
			r.Git(t, "init", "-q", "--separate-git-dir", filepath.Join(t.TempDir(), "repo.git"))
			return r
		}},
		{"worktree with relative paths", func(t *testing.T) testkit.Repo {
			r := testkit.InitWithCommit(t)
			wt := filepath.Join(t.TempDir(), "wt")
			if _, err := r.TryGit("worktree", "add", "-q", "--relative-paths", wt); err != nil {
				t.Skipf("this git has no worktree add --relative-paths: %v", err)
			}
			return testkit.Repo{Dir: wt, Env: r.Env}
		}},
		{"project below the Git top", func(t *testing.T) testkit.Repo {
			r := testkit.InitRepo(t)
			proj := filepath.Join(r.Dir, "a", "proj")
			if err := os.MkdirAll(proj, 0o750); err != nil {
				t.Fatal(err)
			}
			return testkit.Repo{Dir: proj, Env: r.Env}
		}},
	}
	for _, l := range layouts {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			s := l.make(t)
			start := s.Dir
			r := gr.FindRepo(start, start)
			if r.Root != start {
				t.Errorf("project root is %s, want %s", r.Root, start)
			}
			assertSameDir(t, "Git top", r.GitTop, filepath.FromSlash(s.Git(t, "rev-parse", "--show-toplevel")))
			assertSameDir(t, "Git directory", r.GitDir, filepath.FromSlash(s.Git(t, "rev-parse", "--absolute-git-dir")))
			assertSameDir(t, "common Git directory", r.CommonDir, s.CommonDir(t))
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
