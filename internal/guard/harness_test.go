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
	"sync"
	"testing"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// The guard's tests have two seams (spec 0053, "Testing Decisions"; spec
// 0070, "The guard's tests"):
//
//  1. guard.Main in-process, through runGuard and the repo helpers. The
//     Env holds the test's stdin, captured stdout and stderr, a Getenv
//     that reads only the variables the test sets, the directory the test
//     names and the fixed clock testNow. The corpus and every behaviour
//     test run here, under -short too.
//  2. vibe-guard as a subprocess, through runBinary: the reference-hook
//     tests and the binary set (binary_test.go). The binary is built once,
//     on first use. VIBE_GUARD_BIN names a prebuilt binary instead, so
//     that a prebuilt test binary (go test -c) runs with no Go toolchain
//     and no network, as CI's network-disabled job does.

// testNow is the clock of every in-process run. An unlock marker that an
// in-process test writes is read against it, not against the wall clock.
var testNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestMain(m *testing.M) {
	// In-process runs read no process environment. A GIT_DIR or
	// CLAUDE_PROJECT_DIR leaked from a hook would still reach the
	// subprocess tests, which inherit it.
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "CLAUDE_PROJECT_DIR"} {
		os.Unsetenv(k)
	}
	code := m.Run()
	if binDir != "" {
		os.RemoveAll(binDir)
	}
	os.Exit(code)
}

var (
	binOnce sync.Once
	binDir  string // the directory a built binary is in, removed by TestMain
	binPath string
	binErr  error
)

// guardBin returns the vibe-guard binary for the subprocess tests:
// VIBE_GUARD_BIN, or one built from this checkout the first time it is
// asked for.
func guardBin(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		if bin := os.Getenv("VIBE_GUARD_BIN"); bin != "" {
			binPath, binErr = filepath.Abs(bin)
			return
		}
		binDir, binErr = os.MkdirTemp("", "vibe-guard-bin-")
		if binErr != nil {
			return
		}
		binPath = filepath.Join(binDir, "vibe-guard"+exeSuffix())
		cmd := exec.Command("go", "build", "-o", binPath, "github.com/Manual-debuger/VibeConform/cmd/vibe-guard")
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		if err := cmd.Run(); err != nil {
			binErr = fmt.Errorf("building vibe-guard: %w\n%s", err, errOut.String())
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return binPath
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

// runGuard runs guard.Main in-process with args and stdin, its Env's
// working directory dir and Getenv reading only env ("KEY=value"
// entries). The test process's own directory, environment and clock are
// never read; the clock is testNow.
func runGuard(t *testing.T, dir, stdin string, env []string, args ...string) result {
	t.Helper()
	vars := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			t.Fatalf("env entry %q is not KEY=value", kv)
		}
		vars[k] = v
	}
	var out, errOut bytes.Buffer
	code := guard.Main(args, guard.Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		Getenv: func(k string) string { return vars[k] },
		Dir:    dir,
		Now:    func() time.Time { return testNow },
	}, "test")
	return result{code, out.String(), errOut.String()}
}

// runBinary runs the vibe-guard binary with args, stdin on its real
// stdin, in dir, with env added to the test process's environment.
func runBinary(t *testing.T, dir, stdin string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(guardBin(t), args...)
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

// projectEnv is the environment Claude Code gives a hook in r.
func (r repo) projectEnv() []string { return []string{"CLAUDE_PROJECT_DIR=" + r.Root} }

// hook runs "vibe-guard hook" in-process in r, with CLAUDE_PROJECT_DIR
// set to it.
func (r repo) hook(t *testing.T, stdin string) result {
	t.Helper()
	return runGuard(t, r.Root, stdin, r.projectEnv(), "hook")
}

// explain runs "vibe-guard explain" in-process in r, with
// CLAUDE_PROJECT_DIR set to it.
func (r repo) explain(t *testing.T, args ...string) result {
	t.Helper()
	return runGuard(t, r.Root, "", r.projectEnv(), append([]string{"explain"}, args...)...)
}
