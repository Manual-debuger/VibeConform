package guard_test

import (
	"os"
	"path/filepath"
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

// TestExplainDefaultToolFromSubdir: one word that names a file in the
// directory explain runs in means Write, and the decision reads the same
// file.
func TestExplainDefaultToolFromSubdir(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	sub := subdir(t, r)
	writeFile(t, filepath.Join(sub, "x.env"), "X=1\n")
	got := mainIn(t, sub, map[string]string{"CLAUDE_PROJECT_DIR": r.Root}, "explain", "x.env")
	if got.Code != 2 || !hasLine(got.Stdout, "tool: Write") || !hasLine(got.Stdout, "rule: secret-file") {
		t.Errorf("exit %d, output\n%s\nwant exit 2, tool Write and rule secret-file", got.Code, got.Stdout)
	}
}

// TestExplainRelativePathFromSubdir: sub/.claude/settings.json is not
// the project's settings file, as hook decides it with cwd sub.
func TestExplainRelativePathFromSubdir(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	sub := subdir(t, r)
	vars := map[string]string{"CLAUDE_PROJECT_DIR": r.Root}
	got := mainIn(t, sub, vars, "explain", "--tool", "Write", ".claude/settings.json")
	if got.Code != 0 || !hasLine(got.Stdout, "decision: allow") || !hasLine(got.Stdout, "rule: none") {
		t.Errorf("from sub: exit %d, output\n%s\nwant exit 0, allow and rule none", got.Code, got.Stdout)
	}
	got = mainIn(t, r.Root, vars, "explain", "--tool", "Write", ".claude/settings.json")
	if got.Code != 2 || !hasLine(got.Stdout, "rule: guardrail-settings") {
		t.Errorf("from the root: exit %d, output\n%s\nwant exit 2 and rule guardrail-settings", got.Code, got.Stdout)
	}
}

// TestExplainFromRootInProcess: explain .env from the project root gives
// the answer it gave before explain read Env's directory.
func TestExplainFromRootInProcess(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	writeFile(t, filepath.Join(r.Root, ".env"), "X=1\n")
	got := mainIn(t, r.Root, map[string]string{"CLAUDE_PROJECT_DIR": r.Root}, "explain", ".env")
	if got.Code != 2 || !hasLine(got.Stdout, "tool: Write") || !hasLine(got.Stdout, "rule: secret-file") {
		t.Errorf("exit %d, output\n%s\nwant exit 2, tool Write and rule secret-file", got.Code, got.Stdout)
	}
}

// TestRelativeProjectDirReadAgainstEnv: a relative CLAUDE_PROJECT_DIR is
// joined to Env's working directory, not to the test process's.
func TestRelativeProjectDirReadAgainstEnv(t *testing.T) {
	r := newRepo(t, `{"version": 2, "rules": [{"id": "no-hi", "level": "deny", "command": "echo hi", "reason": "the project's rule"}]}`)
	parent, name := filepath.Dir(r.Root), filepath.Base(r.Root)
	got := mainIn(t, parent, map[string]string{"CLAUDE_PROJECT_DIR": name}, "explain", "echo hi")
	if got.Code != 2 || !hasLine(got.Stdout, "rule: no-hi") {
		t.Errorf("exit %d, output\n%s\nwant exit 2 and rule no-hi from %s's policy", got.Code, got.Stdout, name)
	}
}
