package module_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// The green stamp (spec 0057, Desired Behavior 4-7): the Stop gate skips
// verify:fast when the tree is the one the last green run saw.

// stopTasks are the tasks the Stop command calls besides verify:fast.
var stopTasks = []string{"hook:stamp:check", "hook:stamp:save", "hook:done"}

// verifyFastStub stands in for verify:fast. It reads its stdin, as a tool
// may (the payload must still reach hook:done), notes that it ran in an
// ignored log, and fails while the ignored fail.flag exists.
const verifyFastStub = `cat >/dev/null; echo ran >> ran.log; if [ -e fail.flag ]; then echo 'verify:fast failed: TestAdd'; exit 1; fi`

// stopRepo is a scratch directory holding a Taskfile with a source's stop
// tasks and the verify:fast stub.
type stopRepo struct {
	dir string
	env []string
}

func newStopRepo(t *testing.T, taskfile []byte, withGit bool) stopRepo {
	t.Helper()
	var doc struct {
		Tasks map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &doc); err != nil {
		t.Fatal(err)
	}
	tasks := map[string]any{"verify:fast": map[string]any{"silent": true, "cmds": []string{verifyFastStub}}}
	for _, name := range stopTasks {
		task, ok := doc.Tasks[name]
		if !ok {
			t.Fatalf("no %s task; the Stop command calls it", name)
		}
		tasks[name] = task
	}
	out, err := yaml.Marshal(map[string]any{"version": "3", "tasks": tasks})
	if err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	r := stopRepo{dir: filepath.Join(parent, "my repo")}
	// No user or system git config, and no repository above the temp
	// directory (see newHookCheckRepo).
	global := filepath.Join(parent, "gitconfig")
	if err := os.WriteFile(global, []byte("[user]\n\temail = test@example.com\n\tname = test\n[core]\n\tautocrlf = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+global, "GIT_CEILING_DIRECTORIES="+parent)
	r.write(t, "Taskfile.yml", string(out))
	r.write(t, ".gitignore", "ran.log\nfail.flag\n")
	r.write(t, "calc/calc.go", "package calc\n")
	if withGit {
		r.git(t, "init", "-q", "-b", "main")
		r.git(t, "add", "-A")
		r.git(t, "commit", "-q", "-m", "init")
	}
	return r
}

func (r stopRepo) write(t *testing.T, rel, content string) {
	t.Helper()
	p := filepath.Join(r.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (r stopRepo) git(t *testing.T, args ...string) string {
	t.Helper()
	// #nosec G204 -- fixed arguments
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = r.dir, r.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// runs reports how often verify:fast has run so far.
func (r stopRepo) runs(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.dir, "ran.log"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "ran")
}

// stop runs command in sh as Claude Code runs a Stop hook, with a payload
// whose stop_hook_active is active, and reports the exit code, whether
// verify:fast ran, and stderr. stdout must stay empty on exit 0: agents
// parse it then.
func (r stopRepo) stop(t *testing.T, sh, command string, active bool) (code int, ran bool, stderr string) {
	t.Helper()
	before := r.runs(t)
	payload, err := json.Marshal(map[string]any{"session_id": "s", "hook_event_name": "Stop", "stop_hook_active": active})
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- sh is the shell on PATH, the command is the settings'
	cmd := exec.Command(sh, "-c", command)
	cmd.Dir, cmd.Env = r.dir, r.env
	cmd.Stdin = bytes.NewReader(payload)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running the Stop command: %v", err)
		}
		code = exit.ExitCode()
	}
	if code == 0 && out.Len() > 0 {
		t.Errorf("stdout = %q; agents parse stdout on exit 0", out.String())
	}
	return code, r.runs(t) > before, errOut.String()
}

// stopShell is the POSIX shell a Stop command runs in here: Git's sh on
// Windows, as Claude Code uses Git Bash there.
func stopShell(t *testing.T) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		if os.Getenv("VIBE_HOOKS_E2E") != "" {
			t.Fatalf("VIBE_HOOKS_E2E is set but sh is not on PATH: %v", err)
		}
		t.Skip("sh not on PATH; the hook-guard workflow runs this in CI")
	}
	if runtime.GOOS == "windows" && strings.Contains(strings.ToLower(sh), `\windows\system32`) {
		t.Skip("the sh on PATH is WSL's, not Git's")
	}
	return sh
}

// templateStopCommand is the Stop command of the Claude settings template.
func templateStopCommand(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("agents", "claude", "templates", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	stop := settings.Hooks["Stop"]
	if len(stop) != 1 || len(stop[0].Hooks) != 1 {
		t.Fatalf("settings.json should have one Stop hook, has %+v", stop)
	}
	return stop[0].Hooks[0].Command
}

// stopCommands are the Stop commands vibe writes: the template's, and the
// reference command of policy.agent_guard vibe-guard, which relocks first.
func stopCommands(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"template": templateStopCommand(t), "vibe-guard": guard.ReferenceStop}
}

// TestStopStampTasksAgree: every Taskfile that defines the Stop gate's
// tasks defines the stamp tasks the same way, so the behavior tests below,
// which run the Go template's, hold for all of them. Each source's tasks
// also run once in TestStopGateEverySource.
func TestStopStampTasksAgree(t *testing.T) {
	stampTasks := func(t *testing.T, taskfile []byte) map[string]any {
		t.Helper()
		var doc struct {
			Tasks map[string]any `yaml:"tasks"`
		}
		if err := yaml.Unmarshal(taskfile, &doc); err != nil {
			t.Fatal(err)
		}
		out := map[string]any{}
		for _, name := range []string{"hook:stamp:check", "hook:stamp:save"} {
			task, ok := doc.Tasks[name]
			if !ok {
				t.Fatalf("no %s task; the Stop command calls it", name)
			}
			out[name] = task
		}
		return out
	}
	want := stampTasks(t, readSource("repotooling/templates/Taskfile.yml")(t))
	for source, read := range hookCheckSources {
		t.Run(source, func(t *testing.T) {
			if got := stampTasks(t, read(t)); !reflect.DeepEqual(got, want) {
				t.Errorf("stamp tasks differ from the Go template's:\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// TestStopGateEverySource runs each source's Stop tasks with both Stop
// commands: a green Stop runs verify:fast, the next one on the same tree
// does not.
func TestStopGateEverySource(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the Stop gate for every Taskfile; task verify runs it")
	}
	taskOnPATH(t)
	sh := stopShell(t)
	for source, read := range hookCheckSources {
		for name, command := range stopCommands(t) {
			t.Run(source+"/"+name, func(t *testing.T) {
				t.Parallel()
				r := newStopRepo(t, read(t), true)
				for i, want := range []bool{true, false} {
					if code, ran, stderr := r.stop(t, sh, command, false); code != 0 || ran != want {
						t.Errorf("stop %d: exit %d, verify:fast ran %v; want 0 and ran %v\nstderr: %s", i+1, code, ran, want, stderr)
					}
				}
			})
		}
	}
}

// TestStopGateGreenStamp pins spec 0057 Desired Behavior 5-7: a green Stop
// records a stamp and the next Stop with the same tree runs nothing; a
// tracked edit, a staged file, an untracked file and a commit each make
// the next Stop run; a failing run records no stamp, so the next Stop runs
// again, and blocks once; the stamp never shows in git status.
func TestStopGateGreenStamp(t *testing.T) {
	t.Parallel()
	taskOnPATH(t)
	sh := stopShell(t)
	command := templateStopCommand(t)
	r := newStopRepo(t, readSource("repotooling/templates/Taskfile.yml")(t), true)
	expect := func(step string, active bool, wantCode int, wantRan bool) {
		t.Helper()
		code, ran, stderr := r.stop(t, sh, command, active)
		if code != wantCode || ran != wantRan {
			t.Fatalf("%s: exit %d, verify:fast ran %v; want exit %d, ran %v\nstderr: %s", step, code, ran, wantCode, wantRan, stderr)
		}
	}
	expect("first stop", false, 0, true)
	expect("same tree", false, 0, false)
	if st := r.git(t, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Errorf("the stamp shows in git status:\n%s", st)
	}

	r.write(t, "calc/calc.go", "package calc\n\nconst X = 1\n")
	expect("tracked edit", false, 0, true)
	expect("after the tracked edit", false, 0, false)

	r.write(t, "calc/staged.go", "package calc\n")
	r.git(t, "add", "calc/staged.go")
	expect("staged file", false, 0, true)
	expect("after the staged file", false, 0, false)

	r.write(t, "calc/untracked.go", "package calc\n")
	expect("untracked file", false, 0, true)
	expect("after the untracked file", false, 0, false)
	r.write(t, "calc/untracked.go", "package calc\n\nconst Y = 2\n")
	expect("untracked file edited", false, 0, true)

	r.git(t, "add", "-A")
	r.git(t, "commit", "-q", "-m", "more")
	expect("commit", false, 0, true)
	expect("after the commit", false, 0, false)

	// A failing run records nothing: the same tree runs again,
	// blocks once, and lets a continued turn end.
	r.write(t, "fail.flag", "")
	r.write(t, "calc/calc.go", "package calc\n\nconst X = 3\n")
	code, ran, stderr := r.stop(t, sh, command, false)
	if code != 2 || !ran || !strings.Contains(stderr, "verify:fast failed") {
		t.Fatalf("failing stop: exit %d, ran %v, want 2 and ran with the failure on stderr\nstderr: %s", code, ran, stderr)
	}
	expect("failing again, same tree", false, 2, true)
	expect("failing, stop_hook_active", true, 0, true)
	if err := os.Remove(filepath.Join(r.dir, "fail.flag")); err != nil {
		t.Fatal(err)
	}
	expect("fixed, same tree", false, 0, true)
	expect("after the fix", false, 0, false)
}

// TestStopGatePlantedFailureAfterGreen is contract rule A-05's probe as
// it meets the stamp: the probe copy may hold a stamp from a green run of
// the clean tree, and then plants a failing test. The planted file changes
// the tree, so the Stop hook runs verify:fast and blocks (exit 2), and
// with stop_hook_active it lets the turn end (exit 0).
func TestStopGatePlantedFailureAfterGreen(t *testing.T) {
	t.Parallel()
	taskOnPATH(t)
	sh := stopShell(t)
	for name, command := range stopCommands(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newStopRepo(t, readSource("repotooling/templates/Taskfile.yml")(t), true)
			if code, ran, stderr := r.stop(t, sh, command, false); code != 0 || !ran {
				t.Fatalf("green stop: exit %d, ran %v; stderr: %s", code, ran, stderr)
			}
			r.write(t, "calc/vibe_probe_test.go", "package calc\n")
			r.write(t, "fail.flag", "")
			for _, active := range []bool{false, true} {
				want := 2
				if active {
					want = 0
				}
				if code, ran, stderr := r.stop(t, sh, command, active); code != want || !ran {
					t.Errorf("planted failure, stop_hook_active %v: exit %d, ran %v; want %d and ran\nstderr: %s", active, code, ran, want, stderr)
				}
			}
		})
	}
}

// TestStopGateOutsideGit: without a Git repository there is no stamp, so
// every Stop runs verify:fast.
func TestStopGateOutsideGit(t *testing.T) {
	t.Parallel()
	taskOnPATH(t)
	sh := stopShell(t)
	r := newStopRepo(t, readSource("repotooling/templates/Taskfile.yml")(t), false)
	for i := range 2 {
		if code, ran, stderr := r.stop(t, sh, templateStopCommand(t), false); code != 0 || !ran {
			t.Errorf("stop %d outside git: exit %d, ran %v; want 0 and ran\nstderr: %s", i+1, code, ran, stderr)
		}
	}
}

// TestStopGateLinkedWorktrees: each linked worktree keeps its own stamp,
// so a green run in one never stands for another, even on the same tree.
func TestStopGateLinkedWorktrees(t *testing.T) {
	t.Parallel()
	taskOnPATH(t)
	sh := stopShell(t)
	command := templateStopCommand(t)
	main := newStopRepo(t, readSource("repotooling/templates/Taskfile.yml")(t), true)
	side := stopRepo{dir: filepath.Join(filepath.Dir(main.dir), "side"), env: main.env}
	main.git(t, "worktree", "add", "-q", "-b", "side", side.dir)

	if code, ran, _ := main.stop(t, sh, command, false); code != 0 || !ran {
		t.Fatalf("main, first stop: exit %d, ran %v", code, ran)
	}
	if code, ran, _ := side.stop(t, sh, command, false); code != 0 || !ran {
		t.Errorf("side, first stop: exit %d, ran %v; want 0 and ran (main's stamp is not side's)", code, ran)
	}
	if _, ran, _ := side.stop(t, sh, command, false); ran {
		t.Error("side, second stop ran verify:fast; want its own stamp to skip it")
	}
	side.write(t, "calc/side.go", "package calc\n")
	if _, ran, _ := side.stop(t, sh, command, false); !ran {
		t.Error("side, after an edit: verify:fast did not run")
	}
	if _, ran, _ := main.stop(t, sh, command, false); ran {
		t.Error("main ran verify:fast after an edit in side; want main's stamp to still match")
	}
}

// taskOnPATH returns the task binary, skipping when it's absent unless
// VIBE_HOOKS_E2E is set.
func taskOnPATH(t *testing.T) string {
	t.Helper()
	task, err := exec.LookPath("task")
	if err != nil {
		if os.Getenv("VIBE_HOOKS_E2E") != "" {
			t.Fatalf("VIBE_HOOKS_E2E is set but task is not on PATH: %v", err)
		}
		t.Skip("task not on PATH; the hook-guard workflow runs this in CI")
	}
	return task
}
