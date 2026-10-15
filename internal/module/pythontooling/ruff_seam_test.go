package pythontooling

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestRuffSeam runs examples/python's real Ruff through the seed (spec
// 0049): ruff.toml holds the extend line and the base the standard's
// settings, with a generated: path. A selected rule fires, the generated
// path is skipped even when passed by name, and the repository's own
// settings add and remove rules. It skips where the example has no
// virtualenv (uv sync); the examples/python job in examples.yml runs it.
func TestRuffSeam(t *testing.T) {
	ruff := exampleRuff(t)
	base, err := excludeInRuff(ruffConfig, []string{"src/gen/**"})
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, own string
		want      map[string][]string
	}{
		{"seed", "", map[string][]string{"src/a.py": {"F401"}}},
		{"ignore removes a rule", "\n[lint]\nignore = [\"F401\"]\n", map[string][]string{}},
		{"extend-select adds a rule", "\n[lint]\nextend-select = [\"T20\"]\n",
			map[string][]string{"src/a.py": {"F401"}, "src/b.py": {"T201"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "ruff.base.toml", string(base))
			write(t, dir, "ruff.toml", string(ruffSeed)+c.own)
			write(t, dir, "src/a.py", "import os\n")
			write(t, dir, "src/b.py", "print(1)\n")
			write(t, dir, "src/gen/g.py", "import os\nprint(1)\n")

			if got := runRuff(t, ruff, dir, "."); !equalReports(got, c.want) {
				t.Errorf("reported %v, want %v", got, c.want)
			}
			if got := runRuff(t, ruff, dir, "src/gen/g.py"); len(got) != 0 {
				t.Errorf("the generated file passed by name was linted: %v", got)
			}
		})
	}
}

// exampleRuff returns examples/python's Ruff, or skips.
func exampleRuff(t *testing.T) string {
	t.Helper()
	bin := filepath.Join("bin", "ruff")
	if runtime.GOOS == "windows" {
		bin = filepath.Join("Scripts", "ruff.exe")
	}
	ruff, err := filepath.Abs(filepath.Join("..", "..", "..", "examples", "python", ".venv", bin))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ruff); err != nil {
		t.Skip("examples/python has no .venv (uv sync); the examples/python job runs this test in CI")
	}
	return ruff
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runRuff checks target in dir and returns, per slash-separated file
// relative to dir, the sorted rule codes reported. Exit 1 (problems found)
// is expected; anything else, such as a config Ruff cannot load, fails.
func runRuff(t *testing.T, ruff, dir, target string) map[string][]string {
	t.Helper()
	cmd := exec.Command(ruff, "check", target, "--no-cache", "--output-format", "json")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		t.Fatalf("ruff: %v\nstderr:\n%s", err, stderr.String())
	}
	var results []struct {
		Code     string `json:"code"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("ruff output: %v\n%s", err, stdout.String())
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, r := range results {
		file, err := filepath.EvalSymlinks(r.Filename)
		if err != nil {
			file = r.Filename
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		got[rel] = append(got[rel], r.Code)
		slices.Sort(got[rel])
	}
	return got
}

func equalReports(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if strings.Join(v, ",") != strings.Join(b[k], ",") {
			return false
		}
	}
	return true
}
