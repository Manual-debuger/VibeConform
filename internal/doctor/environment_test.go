package doctor

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAgentHooks(t *testing.T) {
	f := fake{
		onPath: map[string]bool{"task": true},
		exists: map[string]bool{filepath.Join("repo", ".claude", "settings.json"): true},
	}
	got := AgentHooks(f.env(), "repo", []AgentHook{
		{Name: "claude", Config: ".claude/settings.json", Binaries: []string{"task"}},
		{Name: "claude", Config: ".claude/settings.json", Binaries: []string{"task", "go"}},
		{Name: "claude", Config: ".claude/other.json", Binaries: []string{"task"}},
		{Name: "codex", Suspended: "suspended (spec 0024); nothing to check"},
	})
	want := []Result{
		{Status: Pass, Name: "claude hooks", Detail: ".claude/settings.json present; on PATH: task (/bin/task)"},
		{Status: Fail, Name: "claude hooks", Detail: ".claude/settings.json present, but go not on PATH: the hooks, the guard included, cannot run"},
		{Status: Fail, Name: "claude hooks", Detail: ".claude/other.json is missing; run vibe sync"},
		{Status: Pass, Name: "codex hooks", Detail: "suspended (spec 0024); nothing to check"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("AgentHooks() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestLineEndings(t *testing.T) {
	cases := []struct {
		name     string
		autocrlf *reply
		attr     reply
		status   Status
		detail   string
	}{
		{"pinned lf", &reply{stdout: "true\n"}, reply{stdout: "Taskfile.yml: eol: lf\n"}, Pass, "core.autocrlf=true; Taskfile.yml has eol=lf"},
		{"unpinned, autocrlf", &reply{stdout: "true\n"}, reply{stdout: "Taskfile.yml: eol: unspecified\n"}, Warn, "core.autocrlf=true and Taskfile.yml has no eol attribute: managed files check out as CRLF"},
		{"unpinned, autocrlf unset", nil, reply{stdout: "Taskfile.yml: eol: unspecified\n"}, Pass, "core.autocrlf=unset; Taskfile.yml has eol=unspecified (not pinned)"},
		{"pinned crlf", &reply{stdout: "input\n"}, reply{stdout: "Taskfile.yml: eol: crlf\n"}, Warn, "core.autocrlf=input; Taskfile.yml has eol=crlf: managed files check out as CRLF"},
		{"check-attr fails", nil, reply{stderr: "fatal: boom\n", err: errExit}, Unverified, "git check-attr failed: fatal: boom"},
		{"check-attr garbled", nil, reply{stdout: "\n"}, Unverified, `git check-attr printed "", not an attribute`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fake{commands: map[string]reply{"git check-attr eol -- Taskfile.yml": tc.attr}}
			if tc.autocrlf != nil {
				f.commands["git config --get core.autocrlf"] = *tc.autocrlf
			}
			got := LineEndings(context.Background(), f.env(), ".", "Taskfile.yml")
			if want := (Result{Status: tc.status, Name: "line endings", Detail: tc.detail}); got != want {
				t.Errorf("LineEndings() = %+v, want %+v", got, want)
			}
		})
	}
	if got := LineEndings(context.Background(), (&fake{}).env(), ".", ""); got.Status != Unverified {
		t.Errorf("no generated file: %+v, want UNVERIFIED", got)
	}
}

// TestLineEndingPolicy is spec 0029 §7's table.
func TestLineEndingPolicy(t *testing.T) {
	lf := reply{stdout: "Taskfile.yml: eol: lf\n"}
	cases := []struct {
		name   string
		attr   reply
		index  reply
		status Status
		detail string
	}{
		{"healthy", lf, reply{stdout: "i/lf    w/lf    attr/text=auto eol=lf \tTaskfile.yml\n"}, Pass,
			"policy line_endings: lf; core.autocrlf=true; Taskfile.yml has eol=lf"},
		{"crlf in the index", lf, reply{stdout: "i/crlf  w/lf    attr/text=auto eol=lf \ta.txt\ni/crlf  w/lf    attr/text=auto eol=lf \tb.txt\ni/lf    w/lf    attr/text=auto eol=lf \tc.txt\n"}, Warn,
			"policy line_endings: lf, but 2 tracked files are stored with CRLF; run git add --renormalize . and commit"},
		{"overridden", reply{stdout: "Taskfile.yml: eol: crlf\n"}, reply{}, Fail,
			"policy.line_endings is lf, but Taskfile.yml has eol=crlf: a rule in a nested .gitattributes, .git/info/attributes, or core.attributesFile overrides it"},
		{"ls-files fails", lf, reply{stderr: "fatal: boom\n", err: errExit}, Warn,
			"policy line_endings: lf; could not list how tracked files are stored: fatal: boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fake{commands: map[string]reply{
				"git config --get core.autocrlf":     {stdout: "true\n"},
				"git check-attr eol -- Taskfile.yml": tc.attr,
				"git ls-files --eol":                 tc.index,
			}}
			got := LineEndingPolicy(context.Background(), f.env(), ".", "Taskfile.yml")
			if want := (Result{Status: tc.status, Name: "line endings", Detail: tc.detail}); got != want {
				t.Errorf("LineEndingPolicy() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRuntime(t *testing.T) {
	for _, tc := range []struct {
		name   string
		goos   string
		exists map[string]bool
		want   string
	}{
		{"linux native", "linux", nil, "linux/amd64, native"},
		{"wsl", "linux", map[string]bool{"/proc/sys/fs/binfmt_misc/WSLInterop": true}, "linux/amd64, wsl"},
		{"docker", "linux", map[string]bool{"/.dockerenv": true}, "linux/amd64, container"},
		{"podman", "linux", map[string]bool{"/run/.containerenv": true}, "linux/amd64, container"},
		// The markers are Linux paths; on Windows a stray \.dockerenv on
		// the current drive proves nothing.
		{"windows ignores markers", "windows", map[string]bool{"/.dockerenv": true}, "windows/amd64, native"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fake{exists: tc.exists}
			env := f.env()
			env.GOOS = tc.goos
			if got := Runtime(env); got != (Result{Status: Pass, Name: "runtime", Detail: tc.want}) {
				t.Errorf("Runtime() = %+v, want detail %q", got, tc.want)
			}
		})
	}
}

func TestWorktree(t *testing.T) {
	const gitDir = "git rev-parse --path-format=absolute --git-dir"
	const common = "git rev-parse --path-format=absolute --git-common-dir"
	for _, tc := range []struct {
		name     string
		commands map[string]reply
		want     Result
	}{
		{"main checkout", map[string]reply{gitDir: {stdout: "D:/repo/.git\n"}, common: {stdout: "D:/repo/.git\n"}},
			Result{Status: Pass, Name: "worktree", Detail: "main checkout, not a linked worktree"}},
		{"linked", map[string]reply{gitDir: {stdout: "D:/repo/.git/worktrees/side\n"}, common: {stdout: "D:/repo/.git\n"}},
			Result{Status: Unverified, Name: "worktree", Detail: "linked worktree (main checkout at D:/repo); collisions with other worktrees (shared Git hooks, tool caches) not checked"}},
		{"old git", map[string]reply{gitDir: {stderr: "error: unknown option\n", err: errExit}},
			Result{Status: Unverified, Name: "worktree", Detail: "git rev-parse failed: error: unknown option"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fake{commands: tc.commands}
			if got := Worktree(context.Background(), f.env(), "."); got != tc.want {
				t.Errorf("Worktree() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestRuntimeMarkersMatchTemplates holds the Go markers and the four
// generated hook:context scripts together: both claim to detect the same
// runtimes, and a marker added to one alone would make them disagree. The
// templates are read from disk rather than imported, so doctor keeps
// depending on no module.
func TestRuntimeMarkersMatchTemplates(t *testing.T) {
	for _, path := range []string{
		"../module/repotooling/templates/Taskfile.yml",
		"../module/tsrepotooling/templates/Taskfile.yml",
		"../module/pyrepotooling/templates/Taskfile.yml",
		"../module/monorepotooling/templates/Taskfile.yml.tmpl",
	} {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatal(err)
		}
		script := string(data)
		for _, m := range RuntimeMarkers {
			if !strings.Contains(script, "test -e "+m.Path) {
				t.Errorf("%s does not test for %s (%s)", path, m.Path, m.Runtime)
			}
		}
		if n := strings.Count(script, "test -e /"); n != len(RuntimeMarkers) {
			t.Errorf("%s tests %d marker paths, doctor knows %d", path, n, len(RuntimeMarkers))
		}
	}
}
