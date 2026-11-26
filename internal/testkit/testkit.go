// Package testkit holds the tests' isolated temporary Git repository
// (spec 0073): git run with the person's global and system configuration
// ignored, none of the person's GIT_ variables, and no repository above
// the temporary directory found. Every test that runs git in a
// repository of its own goes through it, so that each gets the same
// isolation.
//
// Only _test.go files and internal/contract/contracttest, which only
// _test.go files import (ADR 0028), import it, so it never reaches a
// binary, and it imports no package of this module; two depguard rules
// in .golangci.yml enforce both. It does not skip a test without git: each test keeps its
// own skip policy.
package testkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// globalConfig is the global Git configuration of every Repo: a fixed
// author, no line-ending conversion and main as the first branch. It has
// no excludes file, so it changes nothing that git ignores.
const globalConfig = "[user]\n\tname = test\n\temail = test@example.com\n" +
	"[core]\n\tautocrlf = false\n" +
	"[init]\n\tdefaultBranch = main\n"

// GitEnv returns the environment for git in a temporary repository below
// parent, and writes parent/gitconfig, its global configuration. The
// environment is the process's without any GIT_ variable, with the
// system configuration ignored, gitconfig as the global one, and parent
// as a ceiling, so that git finds no repository above it. A test that
// has no testing.TB, or runs other programs that run git (Task, a hook),
// uses it directly; the others use NewRepo.
func GitEnv(parent string) ([]string, error) {
	global := filepath.Join(parent, "gitconfig")
	if err := os.WriteFile(global, []byte(globalConfig), 0o600); err != nil {
		return nil, err
	}
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+global, "GIT_CEILING_DIRECTORIES="+parent), nil
}

// RunGit runs git with args in dir under env and returns its output,
// trimmed. The error holds the output.
func RunGit(env []string, dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) // #nosec G204 -- a test's own arguments
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v in %s: %w\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// Repo is a directory for a temporary Git repository and the environment
// that isolates git in it. A Repo with another Dir and the same Env, such
// as a linked worktree or a subdirectory, runs git with the same
// isolation.
type Repo struct {
	// Dir is the work tree.
	Dir string
	// Env is GitEnv of Dir's parent. A test gives it to a program that
	// runs git in Dir.
	Env []string
}

// NewRepo makes the empty directory name (it may hold spaces) in a new
// temporary directory, and its environment. It does not run git.
func NewRepo(t testing.TB, name string) Repo {
	t.Helper()
	parent := t.TempDir()
	r := Repo{Dir: filepath.Join(parent, name)}
	if err := os.MkdirAll(r.Dir, 0o750); err != nil {
		t.Fatal(err)
	}
	env, err := GitEnv(parent)
	if err != nil {
		t.Fatal(err)
	}
	r.Env = env
	return r
}

// InitRepo is a new repository with no commit, in a directory named repo.
func InitRepo(t testing.TB) Repo {
	t.Helper()
	r := NewRepo(t, "repo")
	r.Init(t)
	return r
}

// InitWithCommit is a new repository with one empty commit, so that a
// linked worktree can be added to it.
func InitWithCommit(t testing.TB) Repo {
	t.Helper()
	r := InitRepo(t)
	r.Commit(t)
	return r
}

// Init makes Dir a repository on the branch main.
func (r Repo) Init(t testing.TB) {
	t.Helper()
	r.Git(t, "init", "-q", "-b", "main")
}

// Commit adds every file in Dir and commits them, or nothing, as one
// commit.
func (r Repo) Commit(t testing.TB) {
	t.Helper()
	r.Git(t, "add", "-A")
	r.Git(t, "commit", "-q", "--allow-empty", "-m", "init")
}

// Git runs git with args in Dir and returns its output, trimmed. It fails
// the test when git fails.
func (r Repo) Git(t testing.TB, args ...string) string {
	t.Helper()
	out, err := r.TryGit(args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TryGit runs git with args in Dir and returns its output, trimmed, or
// the error, which holds the output.
func (r Repo) TryGit(args ...string) (string, error) {
	return RunGit(r.Env, r.Dir, args...)
}

// CommonDir is the absolute common Git directory of Dir, as
// git rev-parse --git-common-dir names it.
func (r Repo) CommonDir(t testing.TB) string {
	t.Helper()
	p := filepath.FromSlash(r.Git(t, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Dir, p)
	}
	return p
}
