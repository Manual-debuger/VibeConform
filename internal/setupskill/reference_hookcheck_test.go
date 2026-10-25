package setupskill_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// The reference hook:check (spec 0056 Desired Behavior 1, spec 0057
// Desired Behavior 1 and 2), moved here from internal/module by spec 0059
// (Desired Behavior 13, Seam 2).

// readRef returns a file of the embedded skill, by its slash path.
func readRef(t *testing.T, path string) []byte {
	t.Helper()
	data, err := fs.ReadFile(setupskill.FS(), path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// refTaskfiles are the reference root Taskfiles, each of which defines
// hook:check and the Stop gate's tasks, by stack.
var refTaskfiles = map[string]string{
	"go":         "reference/taskrunner/go/Taskfile.yml",
	"typescript": "reference/taskrunner/typescript/Taskfile.yml",
	"python":     "reference/taskrunner/python/Taskfile.yml",
	"monorepo":   "reference/taskrunner/monorepo/Taskfile.yml",
}

// hookCheckWant is, per reference Taskfile, the checks hook:check runs:
// the stack's typecheck and lint, and each monorepo component's for the
// monorepo.
var hookCheckWant = map[string][]string{
	"go":         {"lint", "typecheck"},
	"typescript": {"lint", "typecheck"},
	"python":     {"lint", "typecheck"},
	"monorepo":   {"api:lint", "api:typecheck", "web:lint", "web:typecheck", "worker:lint", "worker:typecheck"},
}

// checkTasks finds the tasks hook:check runs: the names between "task -s"
// and "1>&2".
var checkTasks = regexp.MustCompile(`task -s((?: [\w:-]+)+) 1>&2`)

// testTaskNames are the test tasks of every stack and of the reference
// monorepo's components (api, web, worker).
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

// ran reports whether the stub of name ran.
func (r hookCheckRepo) ran(stderr, name string) bool {
	return strings.Contains(stderr, "checked: "+name+"\n") || strings.Contains(stderr, "checked: "+name+"\r\n")
}

// marshalPayload encodes a hook payload as Claude Code sends it, in
// either JSON spacing: on Windows the backslashes of a path come doubled
// (C:\\Users\\...).
func marshalPayload(t *testing.T, payload map[string]any, indent bool) string {
	t.Helper()
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

// editPayload is a Claude Code PostToolUse payload for an edit of path.
func editPayload(t *testing.T, key, path string, indent bool) string {
	t.Helper()
	return marshalPayload(t, map[string]any{
		"session_id":      "s",
		"hook_event_name": "PostToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{key: path, "old_string": "a", "new_string": "b"},
	}, indent)
}

// agentPayload is a PostToolUse payload for an edit of path with a
// top-level agent_id, as Claude Code sends it from inside a sub-agent.
func agentPayload(t *testing.T, path, agentID string, indent bool) string {
	t.Helper()
	return marshalPayload(t, map[string]any{
		"session_id":      "s",
		"agent_id":        agentID,
		"agent_type":      "general-purpose",
		"hook_event_name": "PostToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{"file_path": path, "old_string": "a", "new_string": "b"},
	}, indent)
}

// hookCheckCase is one payload hook:check gets, and whether the checks
// must run.
type hookCheckCase struct {
	name    string
	noGit   bool
	payload func(t *testing.T, repo string) string
	checks  bool
}

// runHookCheckCases runs every case against every reference Taskfile's
// hook:check. Exit 0 and an empty stdout hold for every case: agents parse
// stdout on exit 0.
func runHookCheckCases(t *testing.T, cases []hookCheckCase, also func(t *testing.T, stack string, repo hookCheckRepo, stderr string)) {
	t.Helper()
	task := taskBinary(t)
	if _, err := exec.LookPath("git"); err != nil {
		missingTool(t, "git", err)
	}
	for stack, path := range refTaskfiles {
		t.Run(stack, func(t *testing.T) {
			t.Parallel()
			taskfile := readRef(t, path)
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
						if ran := repo.ran(stderr, name); ran != tc.checks {
							t.Errorf("%s ran = %v, want %v\npayload: %s\nstderr: %s", name, ran, tc.checks, payload, stderr)
						}
					}
					if also != nil {
						also(t, stack, repo, stderr)
					}
				})
			}
		})
	}
}

// TestHookCheckSkipsEditsThatCannotMatter pins spec 0056 Desired Behavior
// 1 for every reference Taskfile: an edit outside the repository or of an
// ignored file runs no check, any other edit runs them all, and a payload
// hook:check cannot read a path from runs them too.
func TestHookCheckSkipsEditsThatCannotMatter(t *testing.T) {
	t.Parallel()
	runHookCheckCases(t, []hookCheckCase{
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
	}, nil)
}

// TestHookCheckRunsTypecheckAndLintOnly pins spec 0057 Desired Behavior 1
// and 2 for every reference Taskfile: hook:check runs the stack's
// typecheck and lint and no tests (every test task is a stub that fails),
// and a payload from a sub-agent (a non-empty agent_id, in either JSON
// spacing) runs nothing, even where the path filter would run the checks.
func TestHookCheckRunsTypecheckAndLintOnly(t *testing.T) {
	t.Parallel()
	runHookCheckCases(t, []hookCheckCase{
		{"main session", false, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(repo, "a b.go"), false)
		}, true},
		{"sub-agent", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "a1b2c3", false)
		}, false},
		{"sub-agent, indented payload", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "a1b2c3", true)
		}, false},
		{"sub-agent, not a git repository", true, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "a1b2c3", false)
		}, false},
		{"empty agent_id", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "", false)
		}, true},
		{"empty agent_id, indented payload", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "", true)
		}, true},
	}, func(t *testing.T, stack string, repo hookCheckRepo, stderr string) {
		t.Helper()
		if got, want := sortedCopy(repo.stubs), hookCheckWant[stack]; !slices.Equal(got, want) {
			t.Errorf("hook:check runs %q, want %q (typecheck and lint, no tests)", got, want)
		}
		if strings.Contains(stderr, "tested: ") {
			t.Errorf("hook:check ran a test task:\n%s", stderr)
		}
	})
}

func sortedCopy(ids []string) []string {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	return ids
}
