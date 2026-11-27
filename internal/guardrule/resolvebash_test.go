package guardrule_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// program writes an executable file at path and returns path.
func program(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	// #nosec G306 -- a test's own program, which must be executable
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// assertNoBash checks that ResolveBash finds no bash and says why.
func assertNoBash(t *testing.T, env []string) {
	t.Helper()
	got, err := gr.ResolveBash(env)
	if err == nil {
		t.Fatalf("ResolveBash = %s, want no bash", got)
	}
	if !strings.HasPrefix(err.Error(), "no bash found; Claude Code runs hook commands with bash") {
		t.Errorf("the error does not say why there is no bash: %v", err)
	}
}

// TestResolveBashOnPath: outside Windows, ResolveBash finds bash on the
// PATH of env, not of the process.
func TestResolveBashOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on Windows ResolveBash finds Git Bash; TestResolveBashGitBash covers it")
	}
	t.Parallel()
	dir := t.TempDir()
	want := program(t, filepath.Join(dir, "bash"))
	got, err := gr.ResolveBash([]string{"PATH=" + dir})
	if err != nil || got != want {
		t.Errorf("ResolveBash = %q, %v; want %s", got, err, want)
	}
	assertNoBash(t, []string{"PATH=" + t.TempDir()})
}

// TestResolveBashGitBash: on Windows, ResolveBash finds the bash Claude
// Code runs hooks with: CLAUDE_CODE_GIT_BASH_PATH when it names an
// existing bash or sh, otherwise Git for Windows' bash.exe next to the
// git.exe on PATH, never one under SystemRoot.
func TestResolveBashGitBash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Git Bash is found on Windows only")
	}
	t.Parallel()
	type layout struct {
		git  string   // git.exe, relative to the Git root
		bash []string // the bash.exe files, relative to the Git root
		want string   // the one found, relative to the Git root; "" for none
	}
	for _, l := range []layout{
		{"cmd/git.exe", []string{"bin/bash.exe"}, "bin/bash.exe"},
		{"bin/git.exe", []string{"bin/bash.exe"}, "bin/bash.exe"},
		{"mingw64/bin/git.exe", []string{"bin/bash.exe"}, "bin/bash.exe"},
		{"cmd/git.exe", []string{"usr/bin/bash.exe"}, "usr/bin/bash.exe"},
		{"cmd/git.exe", nil, ""},
	} {
		t.Run(l.git+" "+strings.Join(l.bash, " "), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			git := program(t, filepath.Join(root, filepath.FromSlash(l.git)))
			for _, b := range l.bash {
				program(t, filepath.Join(root, filepath.FromSlash(b)))
			}
			env := []string{"PATH=" + filepath.Dir(git), "SystemRoot=" + t.TempDir()}
			if l.want == "" {
				assertNoBash(t, env)
				return
			}
			got, err := gr.ResolveBash(env)
			if want := filepath.Join(root, filepath.FromSlash(l.want)); err != nil || got != want {
				t.Errorf("ResolveBash = %q, %v; want %s", got, err, want)
			}
		})
	}

	t.Run("CLAUDE_CODE_GIT_BASH_PATH", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		for _, name := range []string{"bash.exe", "sh.exe", "bash", "sh"} {
			p := program(t, filepath.Join(dir, name))
			got, err := gr.ResolveBash([]string{"PATH=", "CLAUDE_CODE_GIT_BASH_PATH=" + p})
			if err != nil || got != p {
				t.Errorf("CLAUDE_CODE_GIT_BASH_PATH=%s: ResolveBash = %q, %v", p, got, err)
			}
		}
		// Claude Code ignores one that does not exist or names no bash or
		// sh, and finds Git Bash as if it were unset.
		for _, p := range []string{filepath.Join(dir, "missing", "bash.exe"), program(t, filepath.Join(dir, "zsh.exe")), ""} {
			assertNoBash(t, []string{"PATH=", "CLAUDE_CODE_GIT_BASH_PATH=" + p})
		}
	})

	t.Run("not under SystemRoot", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		git := program(t, filepath.Join(root, "cmd", "git.exe"))
		program(t, filepath.Join(root, "bin", "bash.exe"))
		assertNoBash(t, []string{"PATH=" + filepath.Dir(git), "SystemRoot=" + root})
	})
}

// TestLookPath: LookPath finds a program on env's PATH, or relative to
// dir when the name has a path separator, and returns "" for none.
func TestLookPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	name := "prog"
	if runtime.GOOS == "windows" {
		name = "prog.exe"
	}
	want := program(t, filepath.Join(dir, "bin", name))
	if got := gr.LookPath("prog", []string{"PATH=" + filepath.Join(dir, "bin")}, ""); got != want {
		t.Errorf("LookPath on PATH = %q, want %s", got, want)
	}
	if got := gr.LookPath("./bin/prog", []string{"PATH="}, dir); got != want {
		t.Errorf("LookPath relative to dir = %q, want %s", got, want)
	}
	if got := gr.LookPath("prog", []string{"PATH=" + t.TempDir()}, dir); got != "" {
		t.Errorf("LookPath of a missing program = %q, want none", got)
	}
}
