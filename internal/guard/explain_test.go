package guard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// explain reads a relative path against the directory it runs in, Env's
// working directory, as hook reads it against the payload's cwd (spec
// 0070, "vibe-guard explain"). These tests call guard.Main in-process,
// so the test process's own directory plays no part.

// subdir makes r.Root/sub and returns its path.
func subdir(t *testing.T, r repo) string {
	t.Helper()
	sub := filepath.Join(r.Root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	return sub
}

// assertDefaultToolRefused checks that explain, run without --tool,
// chose tool and refused (exit 2) under rule.
func assertDefaultToolRefused(t *testing.T, got result, tool, rule string) {
	t.Helper()
	if got.Code != 2 || !hasLine(got.Stdout, "tool: "+tool) || !hasLine(got.Stdout, "rule: "+rule) {
		t.Errorf("exit %d, output\n%s\nwant exit 2, tool %s and rule %s", got.Code, got.Stdout, tool, rule)
	}
}

// gitBashPath is the absolute path p as Git Bash writes it: C:\a\b is
// /c/a/b. The test is skipped where p has no drive letter.
func gitBashPath(t *testing.T, p string) string {
	t.Helper()
	if len(p) < 2 || p[1] != ':' {
		t.Skip("the path has no drive letter, so it has no Git Bash form")
	}
	return "/" + strings.ToLower(p[:1]) + filepath.ToSlash(p[2:])
}

// TestExplainDefaultToolFromSubdir: one word that names a file in the
// directory explain runs in means Write, and the decision reads the same
// file.
func TestExplainDefaultToolFromSubdir(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	sub := subdir(t, r)
	writeFile(t, filepath.Join(sub, "x.env"), "X=1\n")
	assertDefaultToolRefused(t, runGuard(t, sub, "", r.projectEnv(), "explain", "x.env"), "Write", "secret-file")
}

// TestExplainDefaultToolReadsPathAsGuard: the default-tool test reads
// its argument as the guard reads a path. A Git Bash path to an existing
// file, /c/…/x.env, means Write, as hook reads it as C:/…/x.env.
func TestExplainDefaultToolReadsPathAsGuard(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	file := filepath.Join(r.Root, "x.env")
	writeFile(t, file, "X=1\n")
	assertDefaultToolRefused(t, r.explain(t, gitBashPath(t, file)), "Write", "secret-file")
}

// TestExplainRelativePathFromSubdir: sub/.claude/settings.json is not
// the project's settings file, as hook decides it with cwd sub.
func TestExplainRelativePathFromSubdir(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	sub := subdir(t, r)
	vars := r.projectEnv()
	got := runGuard(t, sub, "", vars, "explain", "--tool", "Write", ".claude/settings.json")
	if got.Code != 0 || !hasLine(got.Stdout, "decision: allow") || !hasLine(got.Stdout, "rule: none") {
		t.Errorf("from sub: exit %d, output\n%s\nwant exit 0, allow and rule none", got.Code, got.Stdout)
	}
	got = runGuard(t, r.Root, "", vars, "explain", "--tool", "Write", ".claude/settings.json")
	if got.Code != 2 || !hasLine(got.Stdout, "rule: guardrail-settings") {
		t.Errorf("from the root: exit %d, output\n%s\nwant exit 2 and rule guardrail-settings", got.Code, got.Stdout)
	}
}

// TestExplainFromRootInProcess: explain .env from the project root gives
// the answer it gave before explain read Env's directory.
func TestExplainFromRootInProcess(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	writeFile(t, filepath.Join(r.Root, ".env"), "X=1\n")
	assertDefaultToolRefused(t, runGuard(t, r.Root, "", r.projectEnv(), "explain", ".env"), "Write", "secret-file")
}

// noHiPolicy is a policy.json whose own rule refuses echo hi.
const noHiPolicy = `{"version": 2, "rules": [{"id": "no-hi", "level": "deny", "command": "echo hi", "reason": "the project's rule"}]}`

// TestRelativeProjectDirReadAgainstEnv: a relative CLAUDE_PROJECT_DIR is
// joined to Env's working directory, not to the test process's.
func TestRelativeProjectDirReadAgainstEnv(t *testing.T) {
	r := newRepo(t, noHiPolicy)
	parent, name := filepath.Dir(r.Root), filepath.Base(r.Root)
	got := runGuard(t, parent, "", []string{"CLAUDE_PROJECT_DIR=" + name}, "explain", "echo hi")
	if got.Code != 2 || !hasLine(got.Stdout, "rule: no-hi") {
		t.Errorf("exit %d, output\n%s\nwant exit 2 and rule no-hi from %s's policy", got.Code, got.Stdout, name)
	}
}

// TestProjectDirReadAsGuard: CLAUDE_PROJECT_DIR is read as the guard
// reads a path, so its Git Bash form, /c/…, names the project at C:/….
func TestProjectDirReadAsGuard(t *testing.T) {
	r := newRepo(t, noHiPolicy)
	got := runGuard(t, r.Root, "", []string{"CLAUDE_PROJECT_DIR=" + gitBashPath(t, r.Root)}, "explain", "echo hi")
	if got.Code != 2 || !hasLine(got.Stdout, "rule: no-hi") {
		t.Errorf("exit %d, output\n%s\nwant exit 2 and rule no-hi from the project's policy", got.Code, got.Stdout)
	}
}
