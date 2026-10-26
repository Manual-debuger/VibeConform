package contract

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveBash finds the bash Claude Code runs hook commands with (spec
// 0053, "Hook commands run the way Claude Code runs them"), from env and
// before the audit changes PATH. vibe doctor finds it the same way (spec
// 0059). On Windows that is
// CLAUDE_CODE_GIT_BASH_PATH when set, otherwise Git for Windows' bash.exe
// found next to git.exe, never the WSL launcher in System32. Elsewhere
// it is bash on PATH. The error says why there is none.
func ResolveBash(env []string) (string, error) {
	if runtime.GOOS != "windows" {
		if p := lookPath("bash", env, ""); p != "" {
			return p, nil
		}
		return "", errNoBash
	}

	// Claude Code ignores a CLAUDE_CODE_GIT_BASH_PATH that does not exist
	// or does not name bash or sh, and finds Git Bash as if it were unset
	// (https://code.claude.com/docs/en/env-vars, checked 2026-10-18).
	if p, ok := envGet(env, "CLAUDE_CODE_GIT_BASH_PATH"); ok && p != "" && gitBashName(p) && isExecutable(p) {
		return p, nil
	}
	git := lookPath("git", env, "")
	if git == "" {
		return "", errNoBash
	}
	// git.exe sits in <Git>\cmd, <Git>\bin or <Git>\mingw64\bin; bash.exe
	// in <Git>\bin or <Git>\usr\bin.
	dir := filepath.Dir(git)
	for range 3 {
		for _, cand := range []string{filepath.Join(dir, "bin", "bash.exe"), filepath.Join(dir, "usr", "bin", "bash.exe")} {
			if isExecutable(cand) && !inSystemRoot(cand, env) {
				return cand, nil
			}
		}
		dir = filepath.Dir(dir)
	}
	return "", errNoBash
}

// gitBashName reports whether p's file name is one Claude Code accepts in
// CLAUDE_CODE_GIT_BASH_PATH: bash.exe, sh.exe, bash or sh.
func gitBashName(p string) bool {
	switch strings.ToLower(filepath.Base(p)) {
	case "bash.exe", "sh.exe", "bash", "sh":
		return true
	}
	return false
}

// errNoBash says the same on every platform, so that reports compare.
var errNoBash = errors.New("no bash found; Claude Code runs hook commands with bash " +
	"(on Windows, Git Bash next to git.exe or the one CLAUDE_CODE_GIT_BASH_PATH names)")

// inSystemRoot reports whether p lies under %SystemRoot%, where the WSL
// launcher bash.exe lives.
func inSystemRoot(p string, env []string) bool {
	root, _ := envGet(env, "SystemRoot")
	if root == "" {
		root = os.Getenv("SystemRoot")
	}
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, p)
	return err == nil && !strings.HasPrefix(rel, "..")
}
