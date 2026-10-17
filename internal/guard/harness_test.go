package guard_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Seam 2 (spec 0053, "Testing Decisions"): vibe-guard runs as a
// subprocess. The binary is built once. VIBE_GUARD_BIN names a prebuilt
// binary instead, so that a prebuilt test binary (go test -c) runs with
// no Go toolchain and no network, as CI's network-disabled job does.

var guardBin string

func TestMain(m *testing.M) {
	// No test here runs git, but a GIT_DIR leaked from a hook would
	// still mislead a reader of the environment.
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "CLAUDE_PROJECT_DIR"} {
		os.Unsetenv(k)
	}
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	if bin := os.Getenv("VIBE_GUARD_BIN"); bin != "" {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return 0, err
		}
		guardBin = abs
		return m.Run(), nil
	}
	dir, err := os.MkdirTemp("", "vibe-guard-bin-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	guardBin = filepath.Join(dir, "vibe-guard"+exeSuffix())
	cmd := exec.Command("go", "build", "-o", guardBin, "github.com/Manual-debuger/VibeConform/cmd/vibe-guard")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("building vibe-guard: %w", err)
	}
	return m.Run(), nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// repo is a fixture repository: a directory with a .git directory (the
// guard reads files; it never runs git) and, optionally, a policy.json.
type repo struct {
	Root      string
	CommonDir string
}

const enforcePolicy = `{"version": 2, "mode": "enforce", "rules": [], "exceptions": []}`

// newRepo makes a fixture repository. policy is policy.json's text, or
// "" for none.
func newRepo(t *testing.T, policy string) repo {
	t.Helper()
	root := t.TempDir()
	git := filepath.Join(root, ".git")
	if err := os.MkdirAll(git, 0o755); err != nil {
		t.Fatal(err)
	}
	if policy != "" {
		writeFile(t, filepath.Join(root, ".claude", "hooks", "policy.json"), policy)
	}
	return repo{Root: root, CommonDir: git}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type result struct {
	Code   int
	Stdout string
	Stderr string
}

// runGuard runs vibe-guard with args and stdin, in dir, with env added.
func runGuard(t *testing.T, dir, stdin string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(guardBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		t.Fatalf("running vibe-guard: %v", err)
	}
	return result{code, out.String(), errOut.String()}
}

// hook runs "vibe-guard hook" in r, with CLAUDE_PROJECT_DIR set to it.
func (r repo) hook(t *testing.T, stdin string) result {
	t.Helper()
	return runGuard(t, r.Root, stdin, []string{"CLAUDE_PROJECT_DIR=" + r.Root}, "hook")
}

// explain runs "vibe-guard explain" in r.
func (r repo) explain(t *testing.T, args ...string) result {
	t.Helper()
	return runGuard(t, r.Root, "", []string{"CLAUDE_PROJECT_DIR=" + r.Root}, append([]string{"explain"}, args...)...)
}
