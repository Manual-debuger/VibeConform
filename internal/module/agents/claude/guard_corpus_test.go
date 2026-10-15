package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// corpusCase is one entry of testdata/guard_corpus.json.
type corpusCase struct {
	Name    string          `json:"name"`
	Want    string          `json:"want"`
	Payload json.RawMessage `json:"payload"`
	Stdin   *string         `json:"stdin"`
	// Local, or LocalText for a file that is not JSON, is the case's
	// policy.local.json (spec 0050). Neither means no local file.
	Local     json.RawMessage `json:"local"`
	LocalText *string         `json:"local_text"`
}

func loadCorpus(t *testing.T) []corpusCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "guard_corpus.json"))
	if err != nil {
		t.Fatalf("reading corpus: %v", err)
	}
	var c struct {
		Cases []corpusCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("parsing corpus: %v", err)
	}
	if len(c.Cases) == 0 {
		t.Fatal("corpus has no cases")
	}
	for _, tc := range c.Cases {
		if tc.Want != "deny" && tc.Want != "allow" {
			t.Fatalf("case %q: want = %q, must be deny or allow", tc.Name, tc.Want)
		}
		if (tc.Payload == nil) == (tc.Stdin == nil) {
			t.Fatalf("case %q: needs exactly one of payload and stdin", tc.Name)
		}
		if tc.Local != nil && tc.LocalText != nil {
			t.Fatalf("case %q: has both local and local_text", tc.Name)
		}
	}
	return c.Cases
}

// localFile returns the case's policy.local.json content, or nil.
func (c corpusCase) localFile() []byte {
	if c.LocalText != nil {
		return []byte(*c.LocalText)
	}
	return c.Local
}

// writeLocal writes the case's local file to path for the duration of the
// subtest. It refuses to replace a file that is already there: the
// end-to-end run writes into a real repository.
func writeLocal(t *testing.T, path string, content []byte) {
	t.Helper()
	if content == nil {
		return
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s already exists; the corpus will not overwrite it", path)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil {
			t.Errorf("removing %s: %v", path, err)
		}
	})
}

func (c corpusCase) input() []byte {
	if c.Stdin != nil {
		return []byte(*c.Stdin)
	}
	return c.Payload
}

// runCorpus feeds every case to the command and checks the exit code:
// 2 with a reason on stderr for deny, 0 with no output for allow. local is
// where the guard looks for policy.local.json, relative to dir.
func runCorpus(t *testing.T, dir, local string, argv ...string) {
	t.Helper()
	for _, tc := range loadCorpus(t) {
		t.Run(tc.Name, func(t *testing.T) {
			writeLocal(t, filepath.Join(dir, filepath.FromSlash(local)), tc.localFile())
			// #nosec G204 -- argv is fixed by the calling test, never user input
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Dir = dir
			cmd.Stdin = bytes.NewReader(tc.input())
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr

			code := 0
			if err := cmd.Run(); err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatalf("running %v: %v", argv, err)
				}
				code = exit.ExitCode()
			}

			switch tc.Want {
			case "deny":
				if code != 2 {
					t.Errorf("exit %d, want 2 (deny); stderr: %s", code, stderr.String())
				}
				if stderr.Len() == 0 {
					t.Error("denied with nothing on stderr; the agent would show no reason")
				}
			case "allow":
				if code != 0 {
					t.Errorf("exit %d, want 0 (allow); stderr: %s", code, stderr.String())
				}
				// Only on allow: both agents parse stdout as JSON on exit 0,
				// so stray output there could change the decision. On exit 2
				// they block regardless and take the reason from stderr.
				// That matters in CI: under GitHub Actions, Task prints an
				// "::error title=Task 'hook:guard' failed::" annotation to
				// stdout whenever a task fails, which a deny is.
				if stdout.Len() != 0 {
					t.Errorf("allowed with output on stdout (%q); agents parse stdout on exit 0", stdout.String())
				}
			}
		})
	}
}

// guardDir writes the rendered policy into a temporary directory and
// returns it, for running a guard the way hook:guard does.
func guardDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content, err := renderPolicy(policy)
	if err != nil {
		t.Fatalf("renderPolicy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "policy.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func requireTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not on PATH; the hook-guard workflow runs this guard in CI", name)
	}
	return path
}

// guardTemplate returns the absolute path of a guard template in a sibling
// repo-tooling module.
func guardTemplate(t *testing.T, module, file string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", module, "templates", file))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGuardCorpusGo runs the prod-go guard. Go is always present where this
// test runs, so it never skips: the reference implementation is always
// checked.
func TestGuardCorpusGo(t *testing.T) {
	dir := guardDir(t)
	bin := filepath.Join(dir, "guard")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	// #nosec G204 -- fixed arguments
	build := exec.Command("go", "build", "-o", bin, "./templates")
	build.Dir = filepath.Join("..", "..", "repotooling")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building guard.go: %v\n%s", err, out)
	}
	runCorpus(t, dir, "policy.local.json", bin, "policy.json")
}

// TestGoRunReportsGuardDenyAsOne pins why every hook:guard command ends in
// "|| exit 2" (plan 0021). go run does not pass its program's exit code
// through: the guard's deny (2) comes out as 1, which both agents treat as
// allow. Found by TestGuardCorpusEndToEnd; this runs in every go test, so if
// go run ever starts propagating the code, the reason for the workaround is
// visibly gone rather than silently assumed.
func TestGoRunReportsGuardDenyAsOne(t *testing.T) {
	dir := guardDir(t)
	src := guardTemplate(t, "repotooling", "guard.go")
	// #nosec G204 -- fixed arguments
	cmd := exec.Command("go", "run", src, "policy.json")
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader([]byte(`{"tool_name": "Bash", "tool_input": {"command": "git branch -D x"}}`))

	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("go run of a denying guard: err = %v, want an exit error", err)
	}
	if got := exit.ExitCode(); got != 1 {
		t.Errorf("go run exit code = %d; the \"|| exit 2\" workaround assumes 1. "+
			"Re-check whether it is still needed", got)
	}
}

// TestGuardCorpusNode runs the prod-ts guard when node is installed.
func TestGuardCorpusNode(t *testing.T) {
	node := requireTool(t, "node")
	runCorpus(t, guardDir(t), "policy.local.json", node, guardTemplate(t, "tsrepotooling", "guard.mjs"), "policy.json")
}

// TestGuardCorpusPython runs the prod-py guard when uv is installed, the
// same way hook:guard does.
func TestGuardCorpusPython(t *testing.T) {
	uv := requireTool(t, "uv")
	runCorpus(t, guardDir(t), "policy.local.json", uv, "run", "--no-project", "python",
		guardTemplate(t, "pyrepotooling", "guard.py"), "policy.json")
}

// TestGuardLongCommand pins spec 0047's bounded gaps. Python and JavaScript
// regular expressions backtrack, and a pattern that allows any text between
// two options takes over a minute on a long commit message full of
// near-misses. Go's RE2 is linear, so only the backtracking runtimes are
// timed. The time is the long command's minus a short one's, so the
// runtime's own start-up does not count.
func TestGuardLongCommand(t *testing.T) {
	long := "git commit -m '" + strings.Repeat("rm -r and git reset and git restore . and Remove-Item -Recurse ", 550) + "'"
	payload := func(command string) []byte {
		b, err := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": command}})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	elapsed := func(dir string, argv []string, stdin []byte) time.Duration {
		// #nosec G204 -- argv is fixed by the calling test, never user input
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Stdin = bytes.NewReader(stdin)
		start := time.Now()
		_ = cmd.Run() // the decision is the corpus tests' business; only the time matters here
		return time.Since(start)
	}
	for _, rt := range []struct {
		name, tool string
		argv       func(tool string) []string
	}{
		{"node", "node", func(node string) []string {
			return []string{node, guardTemplate(t, "tsrepotooling", "guard.mjs"), "policy.json"}
		}},
		{"python", "uv", func(uv string) []string {
			return []string{uv, "run", "--no-project", "python", guardTemplate(t, "pyrepotooling", "guard.py"), "policy.json"}
		}},
	} {
		t.Run(rt.name, func(t *testing.T) {
			argv := rt.argv(requireTool(t, rt.tool))
			dir := guardDir(t)
			base := elapsed(dir, argv, payload("ls"))
			if extra := elapsed(dir, argv, payload(long)) - base; extra > 2*time.Second {
				t.Errorf("a %d-character command took %v longer than ls; a pattern backtracks", len(long), extra)
			}
		})
	}
}

// TestGuardCorpusEndToEnd runs the corpus through the exact command the
// agent configs contain, in a synced repository, when VIBE_GUARD_E2E_DIR
// names one. That covers the Taskfile wiring, Task's -x exit-code
// passthrough, and the guard in one step. It is skipped otherwise, so task
// verify needs neither Task in a particular state nor any runtime beyond Go;
// .github/workflows/hook-guard.yml sets the variable for each standard.
func TestGuardCorpusEndToEnd(t *testing.T) {
	dir := os.Getenv("VIBE_GUARD_E2E_DIR")
	if dir == "" {
		t.Skip("VIBE_GUARD_E2E_DIR not set")
	}
	// Fail, don't skip: whoever set the variable asked for this check, and a
	// skipped end-to-end run would report green having proved nothing.
	task, err := exec.LookPath("task")
	if err != nil {
		t.Fatalf("VIBE_GUARD_E2E_DIR is set but task is not on PATH: %v", err)
	}
	runCorpus(t, dir, ".claude/hooks/policy.local.json", task, "-x", "hook:guard")
}
