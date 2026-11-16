package doctor_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// machine is a scripted machine. Every program is on PATH under /stub/
// except those named missing; "bash" missing means no bash resolves.
// A command line in commands prints or fails as its reply says; with no
// commands, every command prints a git version. With no exists map, a
// path exists when it exists on disk, since the repository under test is
// the test's own temp directory.
type machine struct {
	missing  []string
	commands map[string]reply
	exists   map[string]bool
}

type reply struct {
	stdout, stderr string
	err            error
}

var errExit = errors.New("exit status 1")

func (m machine) env() doctor.Env {
	return doctor.Env{
		LookPath: func(name string) (string, error) {
			if slices.Contains(m.missing, name) {
				return "", exec.ErrNotFound
			}
			return "/stub/" + name, nil
		},
		Run: func(_ context.Context, _, name string, args ...string) (string, string, error) {
			if m.commands == nil {
				return "git version 2.47.1\n", "", nil
			}
			r, ok := m.commands[strings.Join(append([]string{name}, args...), " ")]
			if !ok {
				return "", "unexpected command", errExit
			}
			return r.stdout, r.stderr, r.err
		},
		Exists: func(path string) bool {
			if m.exists != nil {
				return m.exists[path]
			}
			_, err := os.Stat(path)
			return err == nil
		},
		Bash: func() (string, error) {
			if slices.Contains(m.missing, "bash") {
				return "", errors.New("no bash found")
			}
			return "/stub/bash", nil
		},
	}
}

// load writes files into a new directory and loads its configuration as
// the CLI does, through contract.Load, with an opener that lists exactly
// those files. It returns the directory and the outcome, a load error
// included.
func load(t *testing.T, files map[string]string) (string, doctor.Loaded) {
	t.Helper()
	dir := t.TempDir()
	var names []string
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	open := func(_ context.Context, root string) (*contract.Repo, error) {
		return contract.NewRepo(root, names), nil
	}
	cfg, repo, err := contract.Load(context.Background(), dir, "", open)
	return dir, doctor.Loaded{Config: cfg, Repo: repo, Err: err}
}

// with returns the baseline's files with the given ones added, replaced,
// or removed when their content is empty.
func with(files map[string]string) map[string]string {
	out := map[string]string{
		"go.mod":                "module fixture\n\ngo 1.22\n",
		"vibe.yaml":             baselineConfig,
		".claude/settings.json": baselineSettings,
	}
	for name, content := range files {
		if content == "" {
			delete(out, name)
			continue
		}
		out[name] = content
	}
	return out
}

// baselineConfig declares the four entrypoints, each run by task.
const baselineConfig = "contract: 1.2.0\nplatforms: [linux, windows]\nagents: [claude]\nentrypoints:\n" +
	"  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"

// baselineSettings has command hooks; its guard hook is a task, not
// vibe-guard.
const baselineSettings = `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "task -x hook:guard"}]}],
 "Stop": [{"hooks": [{"type": "command", "command": "task verify:fast 1>&2 || exit 2"}]}]}}
`

// guardSettings is a .claude/settings.json whose PreToolUse hook runs
// vibe-guard, as the setup skill's reference does.
const guardSettings = `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "vibe-guard hook"}]}],
 "Stop": [{"hooks": [{"type": "command", "command": "task verify:fast 1>&2 || exit 2"}]}]}}
`

var gitPass = doctor.Result{Status: doctor.Pass, Name: "git", Detail: "git version 2.47.1 (/stub/git); repository readable"}

func run(dir string, m machine, loaded doctor.Loaded) []doctor.Result {
	return doctor.Run(context.Background(), m.env(), dir, loaded).Results
}

func assertResults(t *testing.T, got, want []doctor.Result) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("results:\n got %+v\nwant %+v", got, want)
	}
}

// TestDoctorConfigurationLikeAudit: a configuration that does not load is
// the vibe.yaml line's FAIL, with the text the caller chose for the load
// error, and the checks that need the configuration do not run. git is
// still checked (spec 0059, Desired Behavior 11; spec 0065).
func TestDoctorConfigurationLikeAudit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		files map[string]string
	}{
		{"no file", with(map[string]string{"vibe.yaml": ""})},
		{"version 1", with(map[string]string{"vibe.yaml": "standard: prod-go\nversion: v1\n"})},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, loaded := load(t, c.files)
			if loaded.Err == nil {
				t.Fatal("the configuration loaded")
			}
			loaded.Err = fmt.Errorf("%w; the caller's advice", loaded.Err)
			assertResults(t, run(dir, machine{}, loaded), []doctor.Result{
				gitPass,
				{Status: doctor.Fail, Name: "vibe.yaml", Detail: loaded.Err.Error()},
			})
		})
	}
}

// TestDoctorChecksTheWorkflow: on a machine with everything, doctor
// reports git, the configuration, each entrypoint program, bash for the
// hooks and vibe-guard, one line each, and fails nothing.
func TestDoctorChecksTheWorkflow(t *testing.T) {
	t.Parallel()
	dir, loaded := load(t, with(map[string]string{".claude/settings.json": guardSettings}))
	assertResults(t, run(dir, machine{}, loaded), []doctor.Result{
		gitPass,
		{Status: doctor.Pass, Name: "vibe.yaml", Detail: "version 2, contract 1.2.0"},
		{Status: doctor.Pass, Name: "task", Detail: "/stub/task; runs verify, verify-fast, fmt, fmt-check"},
		{Status: doctor.Pass, Name: "bash", Detail: "/stub/bash; runs the hook commands"},
		{Status: doctor.Pass, Name: "vibe-guard", Detail: "/stub/vibe-guard; the PreToolUse hook runs it"},
	})
}

// TestDoctorReportsWhatIsMissing: a missing entrypoint program, a missing
// bash and a missing vibe-guard each fail their own line, and only that
// line.
func TestDoctorReportsWhatIsMissing(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		missing string
		want    doctor.Result
	}{
		{"task", doctor.Result{Status: doctor.Fail, Name: "task", Detail: "not on PATH; verify, verify-fast, fmt, fmt-check cannot run"}},
		{"bash", doctor.Result{Status: doctor.Fail, Name: "bash", Detail: "no bash found; the hooks cannot run"}},
		{"vibe-guard", doctor.Result{Status: doctor.Fail, Name: "vibe-guard",
			Detail: "not on PATH; the PreToolUse hook that runs it fails on every tool call"}},
	} {
		t.Run(c.missing, func(t *testing.T) {
			t.Parallel()
			dir, loaded := load(t, with(map[string]string{".claude/settings.json": guardSettings}))
			report := doctor.Run(context.Background(), machine{missing: []string{c.missing}}.env(), dir, loaded)
			if !slices.Contains(report.Results, c.want) {
				t.Errorf("no line %+v in %+v", c.want, report.Results)
			}
			if n := report.Count(doctor.Fail); n != 1 {
				t.Errorf("%d lines fail, want 1: %+v", n, report.Results)
			}
		})
	}
}

// TestDoctorChecksVibeGuardOnlyWhenAHookCallsIt: the baseline's guard
// hook is a task, so vibe-guard is not checked.
func TestDoctorChecksVibeGuardOnlyWhenAHookCallsIt(t *testing.T) {
	t.Parallel()
	dir, loaded := load(t, with(nil))
	for _, res := range run(dir, machine{missing: []string{"vibe-guard"}}, loaded) {
		if res.Name == "vibe-guard" {
			t.Errorf("doctor checked vibe-guard, which no hook calls: %+v", res)
		}
	}
}

// TestDoctorEntrypointPrograms: each distinct program gets one line naming
// its entrypoints, in the contract's entrypoint order, setup last; a
// program given by path is looked for in the repository; a command that
// is not one simple command is left to V-04.
func TestDoctorEntrypointPrograms(t *testing.T) {
	t.Parallel()
	dir, loaded := load(t, with(map[string]string{
		"vibe.yaml": "contract: 1.1.0\nplatforms: [linux]\nagents: [claude]\nentrypoints:\n" +
			"  setup: task setup\n  verify: [task verify, ./scripts/check.sh]\n  verify-fast: make fast\n" +
			"  fmt: task fmt && task lint\n  fmt-check: ./scripts/gone.sh\n",
		"scripts/check.sh":      "#!/bin/sh\n",
		".claude/settings.json": "",
	}))
	got := run(dir, machine{missing: []string{"make"}}, loaded)
	if len(got) < 2 {
		t.Fatalf("results = %+v", got)
	}
	assertResults(t, got[2:], []doctor.Result{
		{Status: doctor.Pass, Name: "task", Detail: "/stub/task; runs verify, setup"},
		{Status: doctor.Pass, Name: "./scripts/check.sh", Detail: "in the repository; runs verify"},
		{Status: doctor.Fail, Name: "make", Detail: "not on PATH; verify-fast cannot run"},
		{Status: doctor.Fail, Name: "./scripts/gone.sh", Detail: "not in the repository; fmt-check cannot run"},
		{Status: doctor.Unverified, Name: "entrypoints.fmt",
			Detail: `"task fmt && task lint" is a list joined by &&, not one simple command; vibe audit's V-04 reports it`},
		{Status: doctor.Pass, Name: "bash", Detail: "not needed: .claude/settings.json has no command hook"},
	})
}

// TestDoctorAfterInit: in a repository with the configuration vibe init
// writes (no entrypoints, only comments that show them) and no hooks yet,
// doctor says the entrypoints are V-04's to declare and that nothing
// needs bash, and fails nothing.
func TestDoctorAfterInit(t *testing.T) {
	t.Parallel()
	dir, loaded := load(t, with(map[string]string{
		"vibe.yaml": "# vibe.yaml version 2, written by vibe init.\ncontract: 1.2.0\nplatforms: [linux, windows]\nagents: [claude]\n" +
			"# entrypoints:\n#   verify: task verify\n",
		".claude/settings.json": "",
	}))
	report := doctor.Run(context.Background(), machine{missing: []string{"bash"}}.env(), dir, loaded)
	if len(report.Results) < 2 {
		t.Fatalf("results = %+v", report.Results)
	}
	assertResults(t, report.Results[2:], []doctor.Result{
		{Status: doctor.Unverified, Name: "entrypoints", Detail: "none declared; vibe audit's V-04 says what to declare"},
		{Status: doctor.Pass, Name: "bash", Detail: "not needed: .claude/settings.json has no command hook"},
	})
	if n := report.Count(doctor.Fail); n != 0 {
		t.Errorf("%d lines fail: %+v", n, report.Results)
	}
}

// TestDoctorWritesNothing: doctor diagnoses and never mutates.
func TestDoctorWritesNothing(t *testing.T) {
	t.Parallel()
	dir, loaded := load(t, with(map[string]string{".claude/settings.json": guardSettings}))
	before := snapshot(t, dir)
	run(dir, machine{}, loaded)
	if after := snapshot(t, dir); !maps.Equal(before, after) {
		t.Errorf("doctor changed the repository:\nbefore %v\nafter  %v", slices.Sorted(maps.Keys(before)), slices.Sorted(maps.Keys(after)))
	}
}

// snapshot maps every file under dir to its content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- walking the test's own temp directory
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
