package guardrule

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveBash finds the bash Claude Code runs hook commands with (spec
// 0053, "Hook commands run the way Claude Code runs them"), from env.
// The contract audit calls it before it changes PATH; vibe doctor (spec
// 0059) and the tests that run hooks call it too, so that all of them
// run the same program (spec 0073). On Windows that is
// CLAUDE_CODE_GIT_BASH_PATH when set, otherwise Git for Windows' bash.exe
// found next to git.exe, never the WSL launcher in System32. Elsewhere
// it is bash on PATH. The error says why there is none.
func ResolveBash(env []string) (string, error) {
	if runtime.GOOS != "windows" {
		if p := LookPath("bash", env, ""); p != "" {
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
	git := LookPath("git", env, "")
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

// LookPath finds a program the way a shell would on the PATH of env, with
// PATHEXT on Windows. A name with a path separator is taken relative to
// dir. It returns "" when nothing is found. ResolveBash finds bash and
// git with it, and the contract audit the programs it runs.
func LookPath(name string, env []string, dir string) string { //nolint:gocognit // baseline: pre-existing complexity
	exts := []string{""}
	if runtime.GOOS == "windows" {
		pathext, _ := envGet(env, "PATHEXT")
		if pathext == "" {
			pathext = ".COM;.EXE;.BAT;.CMD"
		}
		if filepath.Ext(name) == "" {
			exts = nil
		}
		for _, e := range strings.Split(pathext, ";") {
			if e != "" {
				exts = append(exts, strings.ToLower(e))
			}
		}
	}
	try := func(base string) string {
		for _, e := range exts {
			if p := base + e; isExecutable(p) {
				return p
			}
		}
		return ""
	}
	if strings.ContainsAny(name, `/\`) {
		p := name
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, filepath.FromSlash(name))
		}
		return try(p)
	}
	path, _ := envGet(env, "PATH")
	for _, d := range filepath.SplitList(path) {
		if d == "" {
			continue
		}
		if p := try(filepath.Join(d, name)); p != "" {
			return p
		}
	}
	return ""
}

func isExecutable(p string) bool {
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0 || errors.Is(err, fs.ErrPermission)
}

// envGet returns the value of name in env. On Windows names are not case
// sensitive. The last setting wins.
func envGet(env []string, name string) (string, bool) {
	val, found := "", false
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && (k == name || runtime.GOOS == "windows" && strings.EqualFold(k, name)) {
			val, found = v, true
		}
	}
	return val, found
}
