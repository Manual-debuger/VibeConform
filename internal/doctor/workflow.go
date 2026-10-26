package doctor

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// entrypointOrder is the order the entrypoints are checked in: the
// required ones as V-04 reports them, then setup.
var entrypointOrder = append(slices.Clone(contract.RequiredEntrypoints), contract.EntrypointSetup)

// Entrypoints checks the program of each declared entrypoint command, one
// line per program, naming the entrypoints that run it. A program named by
// an absolute path (a leading slash on every OS) is looked for there, one
// named by a relative path under repoRoot, any other on PATH. A command that
// is not one simple command, as V-04 reads it, has no program to look
// for: its line is UNVERIFIED, and V-04 reports it. Undeclared entrypoints
// are V-04's too.
func Entrypoints(env Env, repoRoot string, eps contract.Entrypoints) []Result {
	type program struct {
		name        string
		entrypoints []string
	}
	var programs []*program
	var results []Result
	unverified := func(entrypoint, detail string) {
		results = append(results, Result{Status: Unverified, Name: "entrypoints." + entrypoint, Detail: detail})
	}
	for _, name := range entrypointOrder {
		for _, src := range eps[name].Commands {
			cmd, err := contract.ParseSimpleCommand(src)
			if err != nil {
				unverified(name, fmt.Sprintf("%q %s; vibe audit's V-04 reports it", src, err))
				continue
			}
			first := cmd.Words[0]
			if !first.Literal {
				unverified(name, fmt.Sprintf("%q: its program is only known when it runs", src))
				continue
			}
			i := slices.IndexFunc(programs, func(p *program) bool { return p.name == first.Text })
			if i < 0 {
				programs = append(programs, &program{name: first.Text})
				i = len(programs) - 1
			}
			if p := programs[i]; !slices.Contains(p.entrypoints, name) {
				p.entrypoints = append(p.entrypoints, name)
			}
		}
	}
	if len(programs) == 0 && len(results) == 0 {
		return []Result{{Status: Unverified, Name: "entrypoints", Detail: "none declared; vibe audit's V-04 says what to declare"}}
	}

	out := make([]Result, 0, len(programs)+len(results))
	for _, p := range programs {
		names := strings.Join(p.entrypoints, ", ")
		res := Result{Name: p.name}
		switch {
		// A leading slash is absolute on every OS: the command runs in a
		// POSIX shell, where /usr/bin/x is not under the repository.
		case filepath.IsAbs(p.name) || strings.HasPrefix(p.name, "/"):
			if env.Exists(p.name) {
				res.Status, res.Detail = Pass, "found; runs "+names
			} else {
				res.Status, res.Detail = Fail, "not found; "+names+" cannot run"
			}
		case strings.ContainsAny(p.name, `/\`):
			if env.Exists(filepath.Join(repoRoot, filepath.FromSlash(p.name))) {
				res.Status, res.Detail = Pass, "in the repository; runs "+names
			} else {
				res.Status, res.Detail = Fail, "not in the repository; "+names+" cannot run"
			}
		default:
			found, err := env.LookPath(p.name)
			if err != nil {
				res.Status, res.Detail = Fail, "not on PATH; "+names+" cannot run"
				break
			}
			res.Status, res.Detail = Pass, found+"; runs "+names
		}
		out = append(out, res)
	}
	return append(out, results...)
}

// Bash checks the bash Claude Code runs the hook commands with, found as
// the audit finds it. With no command hook, nothing needs it.
func Bash(env Env, hooks contract.ClaudeHooks) Result {
	res := Result{Name: "bash"}
	if len(hooks.Commands) == 0 {
		res.Status, res.Detail = Pass, "not needed: "+contract.ClaudeSettingsFile+" has no command hook"
		return res
	}
	path, err := env.Bash()
	if err != nil {
		res.Status, res.Detail = Fail, err.Error()+"; the hooks cannot run"
		return res
	}
	res.Status, res.Detail = Pass, path+"; runs the hook commands"
	return res
}

// VibeGuard checks vibe-guard on PATH when a PreToolUse hook runs
// `vibe-guard hook`, and returns no result otherwise.
func VibeGuard(env Env, hooks contract.ClaudeHooks) []Result {
	if !hooks.RunsVibeGuard {
		return nil
	}
	if path, err := env.LookPath("vibe-guard"); err == nil {
		return []Result{{Status: Pass, Name: "vibe-guard", Detail: path + "; the PreToolUse hook runs it"}}
	}
	return []Result{{Status: Fail, Name: "vibe-guard", Detail: "not on PATH; the PreToolUse hook that runs it fails on every tool call"}}
}
