package doctor_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// TestGit: the git line, which doctor.Run checks first, whether or not
// the configuration loads.
func TestGit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		m      machine
		status doctor.Status
		detail string
	}{
		{
			name: "healthy",
			m: machine{commands: map[string]reply{
				"git --version":           {stdout: "git version 2.47.1\n"},
				"git rev-parse --git-dir": {stdout: ".git\n"},
			}},
			status: doctor.Pass, detail: "git version 2.47.1 (/stub/git); repository readable",
		},
		{
			name:   "not on PATH",
			m:      machine{missing: []string{"git"}},
			status: doctor.Fail, detail: "not found on PATH",
		},
		{
			name: "not a repository",
			m: machine{commands: map[string]reply{
				"git --version":           {stdout: "git version 2.47.1\n"},
				"git rev-parse --git-dir": {stderr: "fatal: not a git repository\n", err: errExit},
			}},
			status: doctor.Fail, detail: "git version 2.47.1 (/stub/git); not a readable git repository: fatal: not a git repository",
		},
		{
			name: "hangs",
			m: machine{commands: map[string]reply{
				"git --version": {err: context.DeadlineExceeded},
			}},
			status: doctor.Fail, detail: "git --version failed: timed out",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := run(".", tc.m, doctor.Loaded{Err: errors.New("not loaded")})
			want := doctor.Result{Status: tc.status, Name: "git", Detail: tc.detail}
			if len(got) == 0 || got[0] != want {
				t.Errorf("first result = %+v, want %+v", got, want)
			}
		})
	}
}

func TestReportWrite(t *testing.T) {
	t.Parallel()
	var r doctor.Report
	r.Add(
		doctor.Result{Status: doctor.Pass, Name: "git", Detail: "ok"},
		doctor.Result{Status: doctor.Unverified, Name: "worktree", Detail: "not checked"},
		doctor.Result{Status: doctor.Fail, Name: "golangci-lint", Detail: "missing"},
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
	t.Parallel()
	for s, want := range map[doctor.Status]string{doctor.Pass: "PASS", doctor.Fail: "FAIL", doctor.Unverified: "UNVERIFIED"} {
		if s.String() != want {
			t.Errorf("%d.String() = %q, want %q", int(s), s.String(), want)
		}
	}
}
