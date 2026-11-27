package testkit_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/testkit"
)

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
}

// TestRepoIgnoresThePersonsConfiguration: git in a Repo reads neither the
// person's global configuration nor a GIT_DIR in the environment, and
// commits as the fixed test author.
func TestRepoIgnoresThePersonsConfiguration(t *testing.T) {
	needGit(t)
	person := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(person, []byte("[user]\n\tname = person\n[core]\n\tautocrlf = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := testkit.InitRepo(t)
	t.Setenv("GIT_CONFIG_GLOBAL", person)
	t.Setenv("GIT_DIR", filepath.Join(other.Dir, ".git"))

	r := testkit.InitWithCommit(t)
	if got := r.Git(t, "log", "-1", "--format=%an <%ae>"); got != "test <test@example.com>" {
		t.Errorf("the commit's author is %q, want the test author", got)
	}
	if got := r.Git(t, "config", "core.autocrlf"); got != "false" {
		t.Errorf("core.autocrlf is %q, want false", got)
	}
	assertSameDir(t, r.CommonDir(t), filepath.Join(r.Dir, ".git"))
}

// TestInitWithCommit: the repository has one commit, on main, of every
// file in it.
func TestInitWithCommit(t *testing.T) {
	t.Parallel()
	needGit(t)
	r := testkit.NewRepo(t, "my repo")
	if filepath.Base(r.Dir) != "my repo" {
		t.Errorf("the work tree is %s, want one named \"my repo\"", r.Dir)
	}
	if err := os.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.Init(t)
	r.Commit(t)
	if got := r.Git(t, "rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("%s commits, want 1", got)
	}
	if got := r.Git(t, "branch", "--show-current"); got != "main" {
		t.Errorf("the branch is %q, want main", got)
	}
	if got := r.Git(t, "ls-files"); got != "a.txt" {
		t.Errorf("git tracks %q, want a.txt", got)
	}
}

// TestNoRepositoryAbove: git in a Repo's directory does not find a
// repository above it.
func TestNoRepositoryAbove(t *testing.T) {
	t.Parallel()
	needGit(t)
	r := testkit.NewRepo(t, "repo")
	above := testkit.Repo{Dir: filepath.Dir(r.Dir), Env: r.Env}
	above.Init(t)
	if out, err := r.TryGit("rev-parse", "--git-dir"); err == nil {
		t.Errorf("git found the repository above: %s", out)
	}
}

// TestCommonDirOfAWorktree: the common Git directory of a linked worktree
// is the main repository's .git.
func TestCommonDirOfAWorktree(t *testing.T) {
	t.Parallel()
	needGit(t)
	r := testkit.InitWithCommit(t)
	wt := filepath.Join(t.TempDir(), "wt")
	r.Git(t, "worktree", "add", "-q", wt)
	linked := testkit.Repo{Dir: wt, Env: r.Env}
	assertSameDir(t, linked.CommonDir(t), filepath.Join(r.Dir, ".git"))
}

func assertSameDir(t *testing.T, got, want string) {
	t.Helper()
	gi, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	wi, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gi, wi) {
		t.Errorf("the directory is %s, want %s", got, want)
	}
}
