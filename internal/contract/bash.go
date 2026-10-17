package contract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// resolveBash finds the bash Claude Code runs hook commands with (spec
// 0053, "Hook commands run the way Claude Code runs them"), from env and
// before the audit changes PATH. On Windows that is
// CLAUDE_CODE_GIT_BASH_PATH when set, otherwise Git for Windows' bash.exe
// found next to git.exe, never the WSL launcher in System32. Elsewhere
// it is bash on PATH. The error says why there is none.
func resolveBash(env []string) (string, error) {
	if runtime.GOOS != "windows" {
		if p := lookPath("bash", env, ""); p != "" {
			return p, nil
		}
		return "", errNoBash
	}

	if p, ok := envGet(env, "CLAUDE_CODE_GIT_BASH_PATH"); ok && p != "" {
		if isExecutable(p) {
			return p, nil
		}
		return "", fmt.Errorf("CLAUDE_CODE_GIT_BASH_PATH names %s, which is not a file; Claude Code runs hook commands with that bash", p)
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
