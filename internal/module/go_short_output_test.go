package module_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestHookShortTestsPrintOnlyFailures pins spec 0056 Desired Behavior 10
// for every Go setup: test:short, which the Stop hook and hook:check run,
// drops go test's "ok" and "[no test files]" lines, keeps every other line
// and keeps go test's exit code. It runs the real task in a scratch module
// with a passing package, a failing one and one without tests, then
// without the failing one. The name starts with TestHook so the
// hook-guard workflow runs it with task on PATH.
func TestHookShortTestsPrintOnlyFailures(t *testing.T) {
	task, err := exec.LookPath("task")
	if err != nil {
		if os.Getenv("VIBE_HOOKS_E2E") != "" {
			t.Fatalf("VIBE_HOOKS_E2E is set but task is not on PATH: %v", err)
		}
		t.Skip("task not on PATH; the hook-guard workflow runs this in CI")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	for name, setup := range goTaskSetups {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := setup(t)
			taskfile := s.root
			if c := s.goComponents[0]; c != "" {
				taskfile = s.includes[c]
			}
			dir := shortTestModule(t, taskfile)

			code, out := runShortTests(t, task, dir)
			if code == 0 {
				t.Errorf("test:short exits 0 with a failing package; want go test's non-zero exit\n%s", out)
			}
			for _, want := range []string{"--- FAIL: TestFails", "FAIL\tscratch/failing"} {
				if !strings.Contains(out, want) {
					t.Errorf("test:short output has no %q line; it must keep go test's failures\n%s", want, out)
				}
			}
			assertNoPassLines(t, out)

			if err := os.RemoveAll(filepath.Join(dir, "failing")); err != nil {
				t.Fatal(err)
			}
			code, out = runShortTests(t, task, dir)
			if code != 0 {
				t.Errorf("test:short exits %d on a passing module; want 0\n%s", code, out)
			}
			assertNoPassLines(t, out)
		})
	}
}

// shortTestModule writes a Go module holding taskfile's test:short task
// alone, a passing package, a failing package and a package without tests.
func shortTestModule(t *testing.T, taskfile []byte) string {
	t.Helper()
	var doc struct {
		Tasks map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &doc); err != nil {
		t.Fatal(err)
	}
	short, ok := doc.Tasks["test:short"]
	if !ok {
		t.Fatal("the Taskfile defines no test:short")
	}
	tf, err := yaml.Marshal(map[string]any{"version": "3", "tasks": map[string]any{"test:short": short}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, content := range map[string]string{
		"Taskfile.yml":            string(tf),
		"go.mod":                  "module scratch\n\ngo 1.22\n",
		"passing/passing.go":      "package passing\n",
		"passing/passing_test.go": "package passing\n\nimport \"testing\"\n\nfunc TestPasses(t *testing.T) {}\n",
		"failing/failing.go":      "package failing\n",
		"failing/failing_test.go": "package failing\n\nimport \"testing\"\n\nfunc TestFails(t *testing.T) { t.Fatal(\"fails on purpose\") }\n",
		"untested/untested.go":    "package untested\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runShortTests runs "task test:short" in dir and returns its exit code and
// its stdout and stderr together, as the Stop hook passes them on.
func runShortTests(t *testing.T, task, dir string) (int, string) {
	t.Helper()
	// #nosec G204 -- task is the binary on PATH, the arguments are fixed
	cmd := exec.Command(task, "test:short")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running task: %v", err)
		}
		code = exit.ExitCode()
	}
	return code, out.String()
}

func assertNoPassLines(t *testing.T, out string) {
	t.Helper()
	for line := range strings.Lines(out) {
		if strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "? ") {
			t.Errorf("test:short prints %q; it must drop go test's ok and [no test files] lines\n%s", strings.TrimRight(line, "\r\n"), out)
		}
	}
}
