package cli

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// The contract audit's tests run through one seam, the vibe audit command
// in-process, against fixture repositories (spec 0052, "Testing
// Decisions"). A fixture is the baseline tree, copied into a temporary
// directory, changed by the test's edits and made a Git repository whose
// files are all tracked: the baseline plus one change, so that a test
// shows which change flips which result. The cli TestMain clears GIT_DIR
// and its relatives, so every git here uses the fixture.

// baselineFixture is the tree every contract fixture starts from.
var baselineFixture = filepath.Join("testdata", "contract", "baseline")

// fixtureDotClaude is the name testdata uses for a fixture's .claude
// directory. The agent guard refuses edits of any path ending in
// .claude/hooks/policy.json, including fixtures, so testdata keeps them
// under dot-claude/ and the copy renames it.
const fixtureDotClaude = "dot-claude"

// fixturePath maps a path in testdata (OS separators) to the path in the
// fixture: every dot-claude component becomes .claude.
func fixturePath(rel string) string {
	parts := strings.Split(rel, string(filepath.Separator))
	for i, p := range parts {
		if p == fixtureDotClaude {
			parts[i] = ".claude"
		}
	}
	return filepath.Join(parts...)
}

// fixtureEdit changes a fixture before or after git add.
type fixtureEdit struct {
	path      string
	content   string
	remove    bool
	untracked bool
}

// withFile writes path (slash separated) with content before git add, so
// the file is tracked.
func withFile(path, content string) fixtureEdit {
	return fixtureEdit{path: path, content: content}
}

// withoutFile removes path from the fixture before git add.
func withoutFile(path string) fixtureEdit {
	return fixtureEdit{path: path, remove: true}
}

// withUntracked writes path after git add, so the file exists in the
// working tree but is not tracked.
func withUntracked(path, content string) fixtureEdit {
	return fixtureEdit{path: path, content: content, untracked: true}
}

// contractFixture builds a fixture repository from the baseline with
// edits applied and returns its root.
func contractFixture(t *testing.T, edits ...fixtureEdit) string {
	t.Helper()
	return fixtureFrom(t, baselineFixture, edits...)
}

// fixtureFrom builds a fixture repository from the tree src with edits
// applied and returns its root.
func fixtureFrom(t *testing.T, src string, edits ...fixtureEdit) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, fixturePath(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying the baseline fixture: %v", err)
	}

	for _, e := range edits {
		if !e.untracked {
			applyFixtureEdit(t, dir, e)
		}
	}
	fixtureGit(t, dir, "init", "-q")
	fixtureGit(t, dir, "-c", "core.autocrlf=false", "add", "-A")
	for _, e := range edits {
		if e.untracked {
			applyFixtureEdit(t, dir, e)
		}
	}
	return dir
}

func applyFixtureEdit(t *testing.T, dir string, e fixtureEdit) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(e.path))
	if e.remove {
		if err := os.Remove(p); err != nil {
			t.Fatalf("removing %s: %v", e.path, err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(e.content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// auditResult is what one vibe audit run printed and its exit code.
type auditResult struct {
	stdout string
	stderr string
	code   int
}

// runVibeAudit runs vibe audit in-process with args after "audit".
func runVibeAudit(t *testing.T, args ...string) auditResult {
	t.Helper()
	return runVibeAuditRules(t, nil, args...)
}

// runVibeAuditRules runs vibe audit with rules in place of the registry
// (nil for the registry). Every report it prints is watched. The runs of
// the execution and probe parts find no program: PATH is an empty
// directory and no bash is named, so that a fixture's result does not
// depend on what this machine has installed. Probe tests use auditProbe,
// which runs only the rules they read, in this process's environment.
func runVibeAuditRules(t *testing.T, rules []contract.Rule, args ...string) auditResult {
	t.Helper()
	return runVibeAuditSeams(t, contractSeams{rules: rules, probeEnv: emptyProbeEnv(t)}, args...)
}

// emptyProbeEnv is this process's environment with PATH set to an empty
// directory and CLAUDE_CODE_GIT_BASH_PATH removed.
func emptyProbeEnv(t *testing.T) []string {
	t.Helper()
	return probeEnvWithPath(t, t.TempDir())
}

// probeEnvWithPath is this process's environment with PATH replaced and
// CLAUDE_CODE_GIT_BASH_PATH removed.
func probeEnvWithPath(t *testing.T, path string) []string {
	t.Helper()
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(name, "PATH") || strings.EqualFold(name, "CLAUDE_CODE_GIT_BASH_PATH") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "PATH="+path)
}

func runVibeAuditSeams(t *testing.T, seams contractSeams, args ...string) auditResult {
	t.Helper()
	seams.seen = watchReports(t)
	root := newRootCmd("test", seams)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"audit"}, args...))
	err := root.Execute()
	return auditResult{stdout: out.String(), stderr: errOut.String(), code: ExitCode(err)}
}

func TestFixturePathMapsDotClaude(t *testing.T) {
	in := filepath.Join("dot-claude", "hooks", "policy.json")
	if got, want := fixturePath(in), filepath.Join(".claude", "hooks", "policy.json"); got != want {
		t.Errorf("fixturePath(%q) = %q, want %q", in, got, want)
	}
	if got := fixturePath("AGENTS.md"); got != "AGENTS.md" {
		t.Errorf("fixturePath changed a plain path: %q", got)
	}
}
