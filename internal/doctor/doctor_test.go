package doctor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// fake is a scripted machine: which binaries are on PATH, which paths
// exist, what each command line prints or fails with, and its bash.
type fake struct {
	onPath   map[string]bool
	commands map[string]reply
	exists   map[string]bool
	bashErr  error
	ran      []string
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
		Bash: func() (string, error) {
			if f.bashErr != nil {
				return "", f.bashErr
			}
			return "/bin/bash", nil
		},
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
			status: Pass, detail: "git version 2.47.1 (/bin/git); repository readable",
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
			status: Fail, detail: "git version 2.47.1 (/bin/git); not a readable git repository: fatal: not a git repository",
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

func TestReportWrite(t *testing.T) {
	var r Report
	r.Add(
		Result{Status: Pass, Name: "git", Detail: "ok"},
		Result{Status: Unverified, Name: "worktree", Detail: "not checked"},
		Result{Status: Fail, Name: "golangci-lint", Detail: "missing"},
	)
	var out bytes.Buffer
	if err := r.Write(&out); err != nil {
		t.Fatal(err)
	}
	want := "" +
		"PASS        git            ok\n" +
		"UNVERIFIED  worktree       not checked\n" +
		"FAIL        golangci-lint  missing\n" +
		"summary: 1 pass, 1 fail, 1 unverified\n"
	if out.String() != want {
		t.Errorf("Write() =\n%s\nwant\n%s", out.String(), want)
	}
}

func TestStatusString(t *testing.T) {
	for s, want := range map[Status]string{Pass: "PASS", Fail: "FAIL", Unverified: "UNVERIFIED"} {
		if s.String() != want {
			t.Errorf("%d.String() = %q, want %q", int(s), s.String(), want)
		}
	}
}
