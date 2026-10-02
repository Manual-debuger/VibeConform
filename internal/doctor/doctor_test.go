package doctor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fake is a scripted machine: which binaries are on PATH, and what each
// command line prints or fails with.
type fake struct {
	onPath   map[string]bool
	commands map[string]reply
	exists   map[string]bool
	// files maps a slash path to what Open reads from it.
	files map[string]string
	ran   []string
}

type reply struct {
	stdout, stderr string
	err            error
}

func (f *fake) env() Env {
	return Env{
		LookPath: func(name string) (string, error) {
			if f.onPath[name] {
				return "/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, _, name string, args ...string) (string, string, error) {
			line := strings.Join(append([]string{name}, args...), " ")
			f.ran = append(f.ran, line)
			r, ok := f.commands[line]
			if !ok {
				return "", "unexpected command", errors.New("exit status 1")
			}
			return r.stdout, r.stderr, r.err
		},
		Exists: func(path string) bool { return f.exists[path] },
		Open: func(path string) (io.ReadCloser, error) {
			content, ok := f.files[filepath.ToSlash(path)]
			if !ok {
				return nil, fs.ErrNotExist
			}
			return io.NopCloser(strings.NewReader(content)), nil
		},
		GOOS:   "linux",
		GOARCH: "amd64",
	}
}

var errExit = errors.New("exit status 1")

func TestGit(t *testing.T) {
	cases := []struct {
		name   string
		fake   fake
		status Status
		detail string
	}{
		{
			name: "healthy",
			fake: fake{onPath: map[string]bool{"git": true}, commands: map[string]reply{
				"git --version":           {stdout: "git version 2.47.1\n"},
				"git rev-parse --git-dir": {stdout: ".git\n"},
			}},
			status: Pass, detail: "git version 2.47.1; repository readable",
		},
		{
			name:   "not on PATH",
			fake:   fake{},
			status: Fail, detail: "not found on PATH",
		},
		{
			name: "not a repository",
			fake: fake{onPath: map[string]bool{"git": true}, commands: map[string]reply{
				"git --version":           {stdout: "git version 2.47.1\n"},
				"git rev-parse --git-dir": {stderr: "fatal: not a git repository\n", err: errExit},
			}},
			status: Fail, detail: "git version 2.47.1; not a readable git repository: fatal: not a git repository",
		},
		{
			name: "hangs",
			fake: fake{onPath: map[string]bool{"git": true}, commands: map[string]reply{
				"git --version": {err: context.DeadlineExceeded},
			}},
			status: Fail, detail: "git --version failed: timed out",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Git(context.Background(), tc.fake.env(), ".")
			want := Result{Status: tc.status, Name: "git", Detail: tc.detail}
			if got != want {
				t.Errorf("Git() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestTools(t *testing.T) {
	tools := []Tool{
		{Name: "go", Module: "repo-tooling", Why: "builds", Version: []string{"env", "GOVERSION"}},
		{Name: "goimports", Module: "repo-tooling", Why: "task fmt"},
		{Name: "lefthook", Module: "repo-tooling", Why: "hooks", Version: []string{"version"}},
		{Name: "actionlint", Module: "repo-tooling", Why: "task workflows:lint", Version: []string{"-version"}},
	}
	f := fake{
		onPath: map[string]bool{"go": true, "goimports": true, "lefthook": true},
		commands: map[string]reply{
			"go env GOVERSION": {stdout: "go1.27.0\n"},
			"lefthook version": {err: context.DeadlineExceeded},
		},
	}
	got := Tools(context.Background(), f.env(), ".", tools)
	want := []Result{
		{Status: Pass, Name: "go", Detail: "go1.27.0"},
		{Status: Pass, Name: "goimports", Detail: "/bin/goimports"},
		{Status: Warn, Name: "lefthook", Detail: "/bin/lefthook: found on PATH but did not report a version"},
		{Status: Fail, Name: "actionlint", Detail: "not found on PATH (required by repo-tooling: task workflows:lint)"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Tools() =\n%+v\nwant\n%+v", got, want)
	}
	if slices.Contains(f.ran, "actionlint -version") {
		t.Error("probed the version of a binary that is not on PATH")
	}
}

// TestToolsTaskfileLoads: task's line also says whether the Taskfile
// loads, and a Taskfile that doesn't is a FAIL even with task installed.
func TestToolsTaskfileLoads(t *testing.T) {
	task := []Tool{{Name: "task", Module: "repo-tooling", Why: "everything", Version: []string{"--version"}}}
	for _, tc := range []struct {
		name string
		load reply
		want Result
	}{
		{"loads", reply{stdout: "task: Available tasks\n"}, Result{Status: Pass, Name: "task", Detail: "3.53.1; Taskfile.yml loads"}},
		{"broken", reply{stderr: "task: Failed to parse Taskfile.yml\n", err: errExit}, Result{Status: Fail, Name: "task", Detail: "3.53.1; Taskfile.yml does not load: task: Failed to parse Taskfile.yml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fake{onPath: map[string]bool{"task": true}, commands: map[string]reply{
				"task --version":  {stdout: "3.53.1\n"},
				"task --list-all": tc.load,
			}}
			if got := Tools(context.Background(), f.env(), ".", task); len(got) != 1 || got[0] != tc.want {
				t.Errorf("Tools() = %+v, want [%+v]", got, tc.want)
			}
		})
	}
}

func TestReportWrite(t *testing.T) {
	var r Report
	r.Add(
		Result{Status: Pass, Name: "git", Detail: "ok"},
		Result{Status: Unverified, Name: "worktree", Detail: "not checked"},
		Result{Status: Fail, Name: "golangci-lint", Detail: "missing"},
		Result{Status: Warn, Name: "go", Detail: "slow"},
	)
	var out bytes.Buffer
	if err := r.Write(&out); err != nil {
		t.Fatal(err)
	}
	want := "" +
		"PASS        git            ok\n" +
		"UNVERIFIED  worktree       not checked\n" +
		"FAIL        golangci-lint  missing\n" +
		"WARN        go             slow\n" +
		"summary: 1 pass, 1 warn, 1 fail, 1 unverified\n"
	if out.String() != want {
		t.Errorf("Write() =\n%s\nwant\n%s", out.String(), want)
	}
}

func TestStatusString(t *testing.T) {
	for s, want := range map[Status]string{Pass: "PASS", Warn: "WARN", Fail: "FAIL", Unverified: "UNVERIFIED"} {
		if s.String() != want {
			t.Errorf("%d.String() = %q, want %q", int(s), s.String(), want)
		}
	}
}
