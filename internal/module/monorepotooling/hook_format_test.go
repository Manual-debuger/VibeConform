package monorepotooling

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/internal/hooktest"
)

func TestMain(m *testing.M) {
	hooktest.Main()
	// A git hook running the suite, such as pre-push in a linked worktree,
	// exports GIT_DIR. Every git the tests start would then use this
	// repository instead of the one in its temp directory.
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		_ = os.Unsetenv(v)
	}
	os.Exit(m.Run())
}

// TestHookFormatFromSubdirectory is #129 for prod-mono: the rendered root
// hook:format, run from a directory of the repository that is not a
// component's, formats a new file in every component. TASK_CORE_UTILS=1
// makes every platform use Task's built-in xargs, as Windows does, and
// stubs stand in for the formatters (hooktest).
func TestHookFormatFromSubdirectory(t *testing.T) {
	task, err := exec.LookPath("task")
	if err != nil {
		t.Skip("task not on PATH")
	}
	dir := t.TempDir()
	rs := resolve(t, allProfiles)
	files := map[string]string{"docs/README.md": "docs\n"}
	files["Taskfile.yml"] = string(rs["Taskfile.yml"].Content)
	for _, c := range allProfiles {
		files[c.Path+"/Taskfile.yml"] = string(rs[c.Path+"/Taskfile.yml"].Content)
	}
	for name, content := range files {
		write(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"config", "core.autocrlf", "false"},
		{"add", "-A"},
		{"commit", "-q", "-m", "init"},
	} {
		// #nosec G204 -- fixed arguments
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	var added []string
	for _, name := range []string{"services/api/x.go", "apps/web/src/y.ts", "services/worker/src/example/x.py"} {
		file := filepath.Join(dir, filepath.FromSlash(name))
		write(t, file, "1\n")
		added = append(added, file)
	}

	log := hooktest.Install(t, "goimports", "gofmt", "uv", "pnpm")
	t.Setenv("TASK_CORE_UTILS", "1")
	// #nosec G204 -- task from PATH, fixed arguments
	cmd := exec.Command(task, "-x", "hook:format")
	cmd.Dir = filepath.Join(dir, "docs")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook:format: %v\n%s", err, out)
	}
	got := hooktest.Formatted(t, log)
	for _, file := range added {
		if !slices.Contains(got, hooktest.Resolve(t, file)) {
			t.Errorf("%s was not formatted; the formatters got %q", file, got)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
