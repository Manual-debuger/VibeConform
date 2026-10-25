package setupskill_test

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

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

func TestMain(m *testing.M) {
	// A git hook running the suite, such as pre-push in a linked worktree,
	// exports GIT_DIR. Every git the tests start would then use this
	// repository instead of the fixture in its temp directory.
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		_ = os.Unsetenv(v)
	}
	os.Exit(m.Run())
}

// The reference hooks from any directory (spec 0058, Desired Behavior
// 8-10, Seam 2; #133): Claude Code runs a hook command in the agent's
// current directory, and Task looks upward for the nearest Taskfile. In a
// monorepo built from the references, every hook that calls Task must run
// the root's tasks from the root, a component, a component's subdirectory
// and a plain subdirectory alike.

// monorepoComponents are the components of the reference monorepo: the
// directory, the id the root Taskfile includes it under, and the
// reference Taskfile it is built from.
var monorepoComponents = []struct{ dir, id, taskfile string }{
	{"services/api", "api", "reference/taskrunner/monorepo/go.Taskfile.yml"},
	{"apps/web", "web", "reference/taskrunner/monorepo/typescript.Taskfile.yml"},
	{"services/worker", "worker", "reference/taskrunner/monorepo/python.Taskfile.yml"},
}

// hookDirs are the directories, relative to the root, the hooks run from.
var hookDirs = []string{".", "services/api", "services/api/internal/calc", "docs"}

// hookRepo is a fixture monorepo: the reference root Taskfile and
// settings, and each component's reference Taskfile with every task's
// commands replaced by a stub. A stub notes "<id>:<task>" in a log
// outside the repository and fails while the component's fail flag
// exists, also outside, so that neither changes the tree the green stamp
// hashes.
type hookRepo struct {
	root, scratch string
	env           []string
	hooks         map[string][]settingsEntry
}

type settingsEntry struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Command string `json:"command"`
	} `json:"hooks"`
}

func newHookRepo(t *testing.T) hookRepo {
	t.Helper()
	files, err := setupskill.Render("v0.12.0-alpha.1")
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	r := hookRepo{root: filepath.Join(parent, "my repo"), scratch: filepath.Join(parent, "scratch")}
	if err := os.MkdirAll(r.scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]settingsEntry `json:"hooks"`
	}
	if err := json.Unmarshal(files["reference/agents/settings.json"], &cfg); err != nil {
		t.Fatalf("reference/agents/settings.json: %v", err)
	}
	r.hooks = cfg.Hooks

	r.write(t, "Taskfile.yml", string(files["reference/taskrunner/monorepo/Taskfile.yml"]))
	r.write(t, ".claude/settings.json", string(files["reference/agents/settings.json"]))
	for _, c := range monorepoComponents {
		r.write(t, c.dir+"/Taskfile.yml", r.stubbed(t, c.id, files[c.taskfile]))
	}
	r.write(t, "services/api/main.go", "package main\n\nfunc main() {}\n")
	r.write(t, "services/api/internal/calc/calc.go", "package calc\n")
	r.write(t, "apps/web/src/add.ts", "export const add = (a: number, b: number): number => a + b;\n")
	r.write(t, "services/worker/app/__init__.py", "")
	r.write(t, "docs/README.md", "# Docs\n")

	// No user or system git config, and no repository above the fixture.
	global := filepath.Join(parent, "gitconfig")
	if err := os.WriteFile(global, []byte("[user]\n\temail = test@example.com\n\tname = test\n[core]\n\tautocrlf = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CLAUDE_PROJECT_DIR=") {
			r.env = append(r.env, kv)
		}
	}
	r.env = append(r.env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+global, "GIT_CEILING_DIRECTORIES="+parent,
		"CLAUDE_PROJECT_DIR="+r.root)
	r.git(t, "init", "-q", "-b", "main")
	r.git(t, "add", "-A")
	r.git(t, "commit", "-q", "-m", "init")
	return r
}

// stubbed returns a component's Taskfile with each task's commands
// replaced by the stub; calls of other tasks stay.
func (r hookRepo) stubbed(t *testing.T, id string, taskfile []byte) string {
	t.Helper()
	var doc struct {
		Version string                    `yaml:"version"`
		Tasks   map[string]map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &doc); err != nil {
		t.Fatal(err)
	}
	log := filepath.ToSlash(filepath.Join(r.scratch, "ran.log"))
	flag := filepath.ToSlash(filepath.Join(r.scratch, "fail-"+id))
	tasks := map[string]any{}
	for name, task := range doc.Tasks {
		cmds := []any{"echo '" + id + ":" + name + "' >> '" + log + "'; if [ -e '" + flag + "' ]; then echo '" +
			id + ":" + name + " failed' >&2; exit 1; fi"}
		list, _ := task["cmds"].([]any)
		for _, c := range list {
			if m, ok := c.(map[string]any); ok && m["task"] != nil {
				cmds = append(cmds, map[string]any{"task": m["task"]})
			}
		}
		tasks[name] = map[string]any{"silent": true, "cmds": cmds}
	}
	out, err := yaml.Marshal(map[string]any{"version": doc.Version, "tasks": tasks})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func (r hookRepo) write(t *testing.T, rel, content string) {
	t.Helper()
	p := filepath.Join(r.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (r hookRepo) git(t *testing.T, args ...string) {
	t.Helper()
	// #nosec G204 -- fixed arguments
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = r.root, r.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// command is the one hook command of event whose text contains part.
func (r hookRepo) command(t *testing.T, event, part string) string {
	t.Helper()
	var found []string
	for _, e := range r.hooks[event] {
		for _, h := range e.Hooks {
			if strings.Contains(h.Command, part) {
				found = append(found, h.Command)
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d %s hooks contain %q, want 1", len(found), event, part)
	}
	return found[0]
}

// hookRun is what one hook command did: its exit code, its stdout, and
// the component tasks it ran, sorted.
type hookRun struct {
	code   int
	stdout string
	ran    []string
}

// run runs command with bash -c from dir, as Claude Code runs a hook,
// with payload on stdin.
func (r hookRepo) run(t *testing.T, bash, dir, command string, payload map[string]any) (hookRun, string) {
	t.Helper()
	log := filepath.Join(r.scratch, "ran.log")
	if err := os.Remove(log); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	in, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- bash is the shell on PATH, the command is the reference settings'
	cmd := exec.Command(bash, "-c", command)
	cmd.Dir, cmd.Env = filepath.Join(r.root, filepath.FromSlash(dir)), r.env
	cmd.Stdin = bytes.NewReader(in)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	var res hookRun
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running %s: %v", command, err)
		}
		res.code = exit.ExitCode()
	}
	res.stdout = out.String()
	if data, err := os.ReadFile(log); err == nil {
		res.ran = strings.Fields(string(data))
		slices.Sort(res.ran)
		res.ran = slices.Compact(res.ran)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return res, errOut.String()
}

// hookShell is the bash a hook runs in here: Git Bash on Windows, as
// Claude Code uses it there, found next to git when the bash on PATH is
// WSL's. It skips without bash or Task, unless VIBE_HOOKS_E2E or CI is
// set: in CI a skip would hide the Windows run spec 0058 asks for.
func hookShell(t *testing.T) string {
	t.Helper()
	missing := func(name string, err error) { t.Helper(); missingTool(t, name, err) }
	taskBinary(t)
	if runtime.GOOS == "windows" {
		// Git for Windows keeps <root>\bin\bash.exe, and git.exe in
		// <root>\cmd, <root>\bin or <root>\mingw64\bin: which one is on
		// PATH depends on the shell the test starts from.
		git, err := exec.LookPath("git")
		if err != nil {
			missing("git", err)
		}
		for dir := filepath.Dir(filepath.Dir(git)); ; dir = filepath.Dir(dir) {
			bash := filepath.Join(dir, "bin", "bash.exe")
			if _, err := os.Stat(bash); err == nil {
				return bash
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
		missing("Git Bash", errors.New("no bin/bash.exe above "+git))
		return ""
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		missing("bash", err)
	}
	return bash
}

// missingTool skips a test that needs the tool name, which is not found,
// unless VIBE_HOOKS_E2E or CI is set: then it fails, since a skip in CI
// would hide the run.
func missingTool(t *testing.T, name string, err error) {
	t.Helper()
	if os.Getenv("VIBE_HOOKS_E2E") != "" || os.Getenv("CI") != "" {
		t.Fatalf("VIBE_HOOKS_E2E or CI is set but %s is not found: %v", name, err)
	}
	t.Skipf("%s not found (%v); task test in CI runs this", name, err)
}

// taskBinary returns the task binary on PATH (see missingTool).
func taskBinary(t *testing.T) string {
	t.Helper()
	task, err := exec.LookPath("task")
	if err != nil {
		missingTool(t, "task", err)
	}
	return task
}

// each returns "<id>:<task>" for every component.
func each(task string) []string {
	var out []string
	for _, c := range monorepoComponents {
		out = append(out, c.id+":"+task)
	}
	return out
}

func TestReferenceHooksFromAnyDirectory(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("short mode skips this test, which runs every reference hook from four directories; task test runs it")
	}
	bash := hookShell(t)

	// steps run in order in a fresh fixture from each directory. want
	// lists the component tasks the step must run; the root's run is the
	// one every other directory must match.
	type step struct {
		name, event, part string
		setup             func(t *testing.T, r hookRepo)
		payload           func(r hookRepo) map[string]any
		code              int
		want              []string
	}
	stop := func(active bool) func(hookRepo) map[string]any {
		return func(hookRepo) map[string]any {
			return map[string]any{"session_id": "s", "hook_event_name": "Stop", "stop_hook_active": active}
		}
	}
	steps := []step{
		{name: "session start", event: "SessionStart", part: "hook:context",
			payload: func(hookRepo) map[string]any { return map[string]any{"hook_event_name": "SessionStart"} }},
		{name: "format", event: "PostToolUse", part: "task fmt", payload: func(r hookRepo) map[string]any {
			return map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Edit",
				"tool_input": map[string]any{"file_path": filepath.Join(r.root, "services", "api", "main.go")}}
		}, want: each("fmt")},
		{name: "check", event: "PostToolUse", part: "hook:check", payload: func(r hookRepo) map[string]any {
			return map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Edit",
				"tool_input": map[string]any{"file_path": filepath.Join(r.root, "services", "api", "main.go")}}
		}, want: append(each("typecheck"), each("lint")...)},
		{name: "green stop", event: "Stop", part: "verify:fast", payload: stop(false), want: each("verify:fast")},
		{name: "unchanged tree", event: "Stop", part: "verify:fast", payload: stop(false)},
		{name: "another component fails", event: "Stop", part: "verify:fast", payload: stop(false), code: 2,
			setup: func(t *testing.T, r hookRepo) {
				t.Helper()
				r.write(t, "notes.txt", "a change\n")
				if err := os.WriteFile(filepath.Join(r.scratch, "fail-worker"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}, want: []string{"worker:verify:fast"}},
		{name: "blocked once", event: "Stop", part: "verify:fast", payload: stop(true), want: []string{"worker:verify:fast"}},
	}

	results := map[string][]hookRun{}
	for _, dir := range hookDirs {
		r := newHookRepo(t)
		for _, s := range steps {
			if s.setup != nil {
				s.setup(t, r)
			}
			got, stderr := r.run(t, bash, dir, r.command(t, s.event, s.part), s.payload(r))
			results[dir] = append(results[dir], got)
			if got.code != s.code {
				t.Errorf("from %s, %s: exit %d, want %d\nstdout: %s\nstderr: %s", dir, s.name, got.code, s.code, got.stdout, stderr)
			}
			for _, w := range s.want {
				if !slices.Contains(got.ran, w) {
					t.Errorf("from %s, %s: ran %v, want %s among them\nstderr: %s", dir, s.name, got.ran, w, stderr)
				}
			}
			if s.want == nil && len(got.ran) > 0 {
				t.Errorf("from %s, %s: ran %v, want nothing\nstderr: %s", dir, s.name, got.ran, stderr)
			}
		}
		if out := results[dir][0].stdout; !strings.Contains(out, "Repo: my repo") || !strings.Contains(out, "  worker: services/worker (python)") {
			t.Errorf("from %s, session start printed no root facts:\n%s", dir, out)
		}
	}
	for _, dir := range hookDirs[1:] {
		for i, s := range steps {
			if got, want := results[dir][i], results["."][i]; got.code != want.code || !slices.Equal(got.ran, want.ran) {
				t.Errorf("%s from %s: exit %d, ran %v; from the root: exit %d, ran %v",
					s.name, dir, got.code, got.ran, want.code, want.ran)
			}
		}
	}
}
