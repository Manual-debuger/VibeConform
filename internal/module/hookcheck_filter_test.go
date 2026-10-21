package module_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/monorepotooling"
)

// hookCheckSources are every Taskfile that defines hook:check: the three
// single-language templates, the monorepo root as vibe sync renders it,
// and the setup skill's four reference Taskfiles. Each returns the
// Taskfile's bytes.
var hookCheckSources = map[string]func(t *testing.T) []byte{
	"repotooling/templates/Taskfile.yml":   readSource("repotooling/templates/Taskfile.yml"),
	"tsrepotooling/templates/Taskfile.yml": readSource("tsrepotooling/templates/Taskfile.yml"),
	"pyrepotooling/templates/Taskfile.yml": readSource("pyrepotooling/templates/Taskfile.yml"),
	"monorepotooling/templates/Taskfile.yml.tmpl": func(t *testing.T) []byte {
		t.Helper()
		rs, err := monorepotooling.New().Resolve(context.Background(), &module.Context{Components: []manifest.Component{
			{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
			{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
			{ID: "worker", Path: "services/worker", Profile: manifest.ProfilePy},
		}})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		for _, r := range rs {
			if r.Path == "Taskfile.yml" {
				return r.Content
			}
		}
		t.Fatal("the monorepo module writes no root Taskfile.yml")
		return nil
	},
	"setupskill/reference/go":         readSource("../setupskill/skill/reference/taskrunner/go/Taskfile.yml"),
	"setupskill/reference/typescript": readSource("../setupskill/skill/reference/taskrunner/typescript/Taskfile.yml"),
	"setupskill/reference/python":     readSource("../setupskill/skill/reference/taskrunner/python/Taskfile.yml"),
	"setupskill/reference/monorepo":   readSource("../setupskill/skill/reference/taskrunner/monorepo/Taskfile.yml"),
}

func readSource(path string) func(t *testing.T) []byte {
	return func(t *testing.T) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
}

// checkTasks finds the tasks hook:check runs: the names between "task -s"
// and "1>&2".
var checkTasks = regexp.MustCompile(`task -s((?: [\w:-]+)+) 1>&2`)

// testTaskNames are the test tasks of every standard and of the monorepo
// components hookCheckSources render (api, web, worker).
var testTaskNames = []string{
	"test", "test:short",
	"api:test", "api:test:short", "web:test", "worker:test",
}

// hookCheckRepo is a scratch git repository holding hook:check and a stub
// for each task it runs. A stub prints "checked: <name>", so the output
// shows whether the checks ran.
type hookCheckRepo struct {
	dir   string   // the repository; its name has a space
	stubs []string // the stubbed task names
	env   []string
}

func newHookCheckRepo(t *testing.T, taskfile []byte, withGit bool) hookCheckRepo {
	t.Helper()
	var doc struct {
		Tasks map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &doc); err != nil {
		t.Fatal(err)
	}
	hook, _ := doc.Tasks["hook:check"].(map[string]any)
	cmds, _ := hook["cmds"].([]any)
	if len(cmds) != 1 {
		t.Fatalf("hook:check has %d cmds, want one script", len(cmds))
	}
	script, _ := cmds[0].(string)
	m := checkTasks.FindStringSubmatch(script)
	if m == nil {
		t.Fatalf("hook:check runs no \"task -s ... 1>&2\":\n%v", cmds[0])
	}

	parent := t.TempDir()
	repo := hookCheckRepo{dir: filepath.Join(parent, "my repo"), stubs: strings.Fields(m[1])}
	if err := os.Mkdir(repo.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// No user or system git config (a global excludes file would change
	// what is ignored), and no repository above the temp directory. Each
	// git call is a process start, slow on Windows, so the setup is three.
	global := filepath.Join(parent, "gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	repo.env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+global, "GIT_CEILING_DIRECTORIES="+parent)

	tasks := map[string]any{"hook:check": doc.Tasks["hook:check"]}
	for _, name := range repo.stubs {
		tasks[name] = map[string]any{"silent": true, "cmds": []string{"echo 'checked: " + name + "'"}}
	}
	// Every test task is a stub that fails, so a hook:check that ran one
	// would exit 2 (spec 0057 Desired Behavior 1).
	for _, name := range testTaskNames {
		tasks[name] = map[string]any{"silent": true, "cmds": []string{"echo 'tested: " + name + "' && exit 1"}}
	}
	out, err := yaml.Marshal(map[string]any{"version": "3", "tasks": tasks})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		"Taskfile.yml":       out,
		".gitignore":         []byte("ignored/\n"),
		"a b.go":             []byte("package scratch\n"),
		"ignored/x y.txt":    nil,
		"ignored/nb.ipynb":   nil,
		"untracked file.go":  nil,
		"sub dir/tracked.go": []byte("package sub\n"),
	} {
		path := filepath.Join(repo.dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !withGit {
		return repo
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "Taskfile.yml", ".gitignore", "a b.go", "sub dir/tracked.go"},
		{"-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-q", "-m", "init"},
	} {
		// #nosec G204 -- fixed arguments
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = repo.dir, repo.env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

// run runs "task -x hook:check" in the repository with payload on stdin,
// as an agent's PostToolUse hook does.
func (r hookCheckRepo) run(t *testing.T, task, payload string) (code int, stdout, stderr string) {
	t.Helper()
	// #nosec G204 -- task is the binary on PATH, the arguments are fixed
	cmd := exec.Command(task, "-x", "hook:check")
	cmd.Dir, cmd.Env = r.dir, r.env
	cmd.Stdin = strings.NewReader(payload)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running task: %v", err)
		}
		code = exit.ExitCode()
	}
	return code, out.String(), errOut.String()
}

// editPayload is a Claude Code PostToolUse payload for an edit of path,
// JSON-encoded as Claude Code sends it: on Windows the backslashes come
// doubled (C:\\Users\\...).
func editPayload(t *testing.T, key, path string, indent bool) string {
	t.Helper()
	payload := map[string]any{
		"session_id":      "s",
		"hook_event_name": "PostToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{key: path, "old_string": "a", "new_string": "b"},
	}
	var data []byte
	var err error
	if indent {
		data, err = json.MarshalIndent(payload, "", "  ")
	} else {
		data, err = json.Marshal(payload)
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestHookCheckSkipsEditsThatCannotMatter pins spec 0056 Desired Behavior
// 1 for every Taskfile that defines hook:check: an edit outside the
// repository or of an ignored file runs no check, any other edit runs them
// all, and a payload hook:check cannot read a path from runs them too.
func TestHookCheckSkipsEditsThatCannotMatter(t *testing.T) {
	task, err := exec.LookPath("task")
	if err != nil {
		if os.Getenv("VIBE_HOOKS_E2E") != "" {
			t.Fatalf("VIBE_HOOKS_E2E is set but task is not on PATH: %v", err)
		}
		t.Skip("task not on PATH; the hook-guard workflow runs this in CI")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	cases := []struct {
		name    string
		noGit   bool
		payload func(t *testing.T, repo string) string
		checks  bool
	}{
		{"outside the repository", false, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(filepath.Dir(repo), "memory dir", "MEMORY.md"), false)
		}, false},
		{"ignored", false, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(repo, "ignored", "x y.txt"), false)
		}, false},
		{"ignored notebook, indented payload", false, func(t *testing.T, repo string) string {
			return editPayload(t, "notebook_path", filepath.Join(repo, "ignored", "nb.ipynb"), true)
		}, false},
		{"tracked", false, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(repo, "a b.go"), false)
		}, true},
		{"tracked in a subdirectory, indented payload", false, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(repo, "sub dir", "tracked.go"), true)
		}, true},
		{"untracked, not ignored", false, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(repo, "untracked file.go"), false)
		}, true},
		{"no path", false, func(*testing.T, string) string {
			return `{"hook_event_name":"PostToolUse","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch"}}`
		}, true},
		{"empty payload", false, func(*testing.T, string) string { return "" }, true},
		{"malformed payload", false, func(*testing.T, string) string { return "not json" }, true},
		{"truncated payload", false, func(t *testing.T, repo string) string {
			p := editPayload(t, "file_path", filepath.Join(repo, "ignored", "x y.txt"), false)
			return p[:strings.Index(p, `"file_path":"`)+len(`"file_path":"`)+3]
		}, true},
		{"not a git repository", true, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(repo, "ignored", "x y.txt"), false)
		}, true},
	}

	for source, read := range hookCheckSources {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			taskfile := read(t)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					repo := newHookCheckRepo(t, taskfile, !tc.noGit)
					payload := tc.payload(t, repo.dir)
					code, stdout, stderr := repo.run(t, task, payload)
					if code != 0 {
						t.Fatalf("exit %d, want 0; stderr: %s\npayload: %s", code, stderr, payload)
					}
					if stdout != "" {
						t.Errorf("stdout = %q; agents parse stdout on exit 0", stdout)
					}
					for _, name := range repo.stubs {
						ran := strings.Contains(stderr, "checked: "+name+"\n") || strings.Contains(stderr, "checked: "+name+"\r\n")
						if ran != tc.checks {
							t.Errorf("%s ran = %v, want %v\npayload: %s\nstderr: %s", name, ran, tc.checks, payload, stderr)
						}
					}
				})
			}
		})
	}
}
