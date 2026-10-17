package guard_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// findBash returns the bash Claude Code runs hooks in: bash on PATH, or
// on Windows the bash next to Git's sh (the bash.exe PATH finds first
// may be WSL's). Without one the test is skipped, unless
// VIBE_TEST_REQUIRE_TOOLS names bash.
func findBash(t *testing.T) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if runtime.GOOS == "windows" {
		bash, err = "", errors.New("no Git Bash: no CLAUDE_CODE_GIT_BASH_PATH, and no Git sh or git on PATH")
		var candidates []string
		if p := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); p != "" {
			candidates = append(candidates, p)
		}
		if sh, shErr := exec.LookPath("sh"); shErr == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(sh), "bash.exe"))
		}
		if git, gitErr := exec.LookPath("git"); gitErr == nil {
			// <Git>\cmd\git.exe or <Git>\bin\git.exe; bash is <Git>\bin\bash.exe.
			candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(git)), "bin", "bash.exe"))
		}
		for _, c := range candidates {
			if _, statErr := os.Stat(c); statErr == nil {
				bash, err = c, nil
				break
			}
		}
	}
	if err != nil {
		if slices.Contains(strings.Split(os.Getenv("VIBE_TEST_REQUIRE_TOOLS"), ","), "bash") {
			t.Fatalf("VIBE_TEST_REQUIRE_TOOLS names bash, but none was found: %v", err)
		}
		t.Skipf("no bash: %v", err)
	}
	return bash
}

// referenceHook runs the reference PreToolUse command through bash with
// PATH set to path only.
func referenceHook(t *testing.T, bash, path string, r repo, stdin string) result {
	t.Helper()
	cmd := exec.Command(bash, "-c", guard.ReferencePreToolUse("v0.0.0-test"))
	cmd.Dir = r.Root
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !strings.EqualFold(k, "PATH") {
			env = append(env, kv)
		}
	}
	env = append(env, "PATH="+path, "CLAUDE_PROJECT_DIR="+r.Root)
	cmd.Env = env
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
		t.Fatalf("running bash: %v", err)
	}
	return result{code, out.String(), errOut.String()}
}

// TestReferenceHookWithoutVibeGuard: with vibe-guard missing from PATH,
// the reference PreToolUse command refuses every corpus case, allow
// cases included, and says how to install the guard (#105's done-when).
func TestReferenceHookWithoutVibeGuard(t *testing.T) {
	bash := findBash(t)
	empty := t.TempDir()
	for _, c := range contract.GuardCorpus() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := corpusRepo(t, c)
			got := referenceHook(t, bash, empty, r, payloadFor(t, c, r.Root))
			if got.Code != 2 || !strings.HasPrefix(got.Stderr, "vibe-guard is not installed; install it: go install ") {
				t.Errorf("exit %d, stderr %q; want exit 2 and the install line", got.Code, got.Stderr)
			}
		})
	}
}

// TestReferenceHookWithVibeGuard: with only vibe-guard on PATH, the
// reference command decides the corpus as vibe-guard does.
func TestReferenceHookWithVibeGuard(t *testing.T) {
	bash := findBash(t)
	dir := filepath.Dir(guardBin)
	if filepath.Base(guardBin) != "vibe-guard"+exeSuffix() {
		dir = t.TempDir()
		data, err := os.ReadFile(guardBin)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "vibe-guard"+exeSuffix()), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range contract.GuardCorpus() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := corpusRepo(t, c)
			got := referenceHook(t, bash, dir, r, payloadFor(t, c, r.Root))
			want := 0
			if c.Want == contract.WantDeny {
				want = 2
			}
			if got.Code != want || want == 2 && !strings.Contains(got.Stderr, "refused by rule "+c.Rule+":") {
				t.Errorf("exit %d, stderr %q; want exit %d", got.Code, got.Stderr, want)
			}
		})
	}
}
