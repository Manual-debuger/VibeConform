package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The probe copy (spec 0053, "The probe copy"): one temporary Git
// repository per audit, holding the working tree's tracked files, in
// which every execution and probe part runs. The audited repository is
// never written; its .git is never read by a run.

// ProbeOptions shape the runs of the execution and probe parts.
type ProbeOptions struct {
	// Timeout limits each entrypoint run; zero means DefaultProbeTimeout.
	Timeout time.Duration
	// KeepCopy keeps the probe copy after the audit and prints its path
	// on Stderr.
	KeepCopy bool
	// Progress receives a line before each run, or nil for none. The CLI
	// passes stderr in text mode only.
	Progress io.Writer
	// Stderr receives messages that are not progress, such as the kept
	// copy's path. Nil discards them.
	Stderr io.Writer
	// Env is the environment runs inherit before the audit's changes. Nil
	// means this process's environment. Tests narrow PATH with it; the
	// audit's own git commands always use this process's environment.
	Env []string
}

// probeSession is the probe copy of one audit and everything a run
// needs. Parts reach it through Env.probe. It is built before the first
// execution or probe part and deleted when the audit ends.
type probeSession struct {
	ctx  context.Context
	opts ProbeOptions
	// tmp is the audit's temporary directory: the copy, the shim and its
	// log live in it.
	tmp string
	// root is the copy's work tree.
	root string
	// head is the copy's one commit; reset returns to it.
	head string
	// runEnv is the environment of every run: GIT_* repository variables
	// removed, CLAUDE_PROJECT_DIR the copy, PATH led by the shim.
	runEnv []string
	shim   *shim
	// bash runs hook commands; bashErr says why there is none.
	bash    string
	bashErr error
	// broken is set when the copy could not be built or reset: every part
	// that needs it cannot be decided.
	broken *Finding
	// setup is the setup entrypoint's run, when it was declared.
	setup *entrypointRun
	// baselines caches the baseline runs of verify and verify-fast.
	baselines map[string]*entrypointRun
	// shimCalls is every call of vibe the shim recorded, in run order.
	shimCalls []shimCall
	// guardDeny is A-02's verdict on its deny cases, as R-01's execution
	// part reads it. A-02 (#103) sets it; nil means A-02 did not run.
	guardDeny *PartResult
	// progressMu orders the progress lines of runs that happen
	// concurrently (A-02 and A-03 feed corpus cases in parallel).
	progressMu sync.Mutex
}

// startProbeSession builds the probe copy and runs setup. A failure is
// recorded in the session (broken), so the parts report it.
func startProbeSession(env *Env, opts ProbeOptions) *probeSession {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultProbeTimeout
	}
	base := opts.Env
	if base == nil {
		base = os.Environ()
	}
	s := &probeSession{ctx: env.ctx, opts: opts, baselines: map[string]*entrypointRun{}}
	s.bash, s.bashErr = resolveBash(base)

	tmp, err := os.MkdirTemp("", "vibe-audit-")
	if err != nil {
		s.broken = &Finding{Message: "cannot create the probe copy: " + err.Error()}
		return s
	}
	s.tmp = tmp
	s.root = filepath.Join(tmp, "copy")

	if s.shim, err = installShim(tmp); err != nil {
		s.broken = &Finding{Message: err.Error()}
		return s
	}
	if f := s.build(env); f != nil {
		s.broken = f
		return s
	}

	runEnv := envUnset(base, gitRepoVariables...)
	runEnv = envSet(runEnv, "CLAUDE_PROJECT_DIR", s.root)
	path, _ := envGet(base, "PATH")
	if path == "" {
		path = s.shim.dir
	} else {
		path = s.shim.dir + string(os.PathListSeparator) + path
	}
	s.runEnv = envSet(runEnv, "PATH", path)

	s.runSetup(env)
	return s
}

// close deletes the probe copy, or keeps it for --keep-probe-copy.
func (s *probeSession) close() {
	if s.tmp == "" {
		return
	}
	if s.opts.KeepCopy {
		if s.opts.Stderr != nil {
			_, _ = fmt.Fprintf(s.opts.Stderr, "vibe audit: keeping the probe copy at %s\n", s.root)
		}
		return
	}
	removeAllRetry(s.tmp)
}

// removeAllRetry deletes dir. Git makes its objects read-only, and on
// Windows a process that just ended can hold a file a moment longer.
func removeAllRetry(dir string) {
	for i := range 10 {
		if os.RemoveAll(dir) == nil {
			return
		}
		_ = filepath.WalkDir(dir, func(p string, _ fs.DirEntry, _ error) error {
			_ = os.Chmod(p, 0o700) // #nosec G122 G302 -- the audit's own temporary files, made deletable
			return nil
		})
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	_ = os.RemoveAll(dir)
}

// build copies the tracked files into the copy and commits them.
func (s *probeSession) build(env *Env) *Finding {
	if err := os.MkdirAll(s.root, 0o750); err != nil {
		return &Finding{Message: "cannot create the probe copy: " + err.Error()}
	}
	repoRoot := env.Repo.Root()
	for _, rel := range env.Repo.Files() {
		src := filepath.Join(repoRoot, filepath.FromSlash(rel))
		dst := filepath.Join(s.root, filepath.FromSlash(rel))
		info, err := os.Lstat(src)
		if err != nil {
			return &Finding{File: rel, Message: "cannot copy into the probe copy: " + err.Error()}
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return &Finding{File: rel, Message: "cannot copy into the probe copy: " + err.Error()}
		}
		switch {
		case info.IsDir():
			// A gitlink: submodules are not copied.
			env.Note("the probe copy leaves out the submodule %s", rel)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(src)
			if err == nil {
				err = os.Symlink(target, dst)
			}
			if err != nil {
				return &Finding{File: rel, Message: "is a symbolic link the probe copy cannot hold on this machine: " + err.Error()}
			}
		default:
			if err := copyFile(src, dst, info.Mode().Perm()|0o600); err != nil {
				return &Finding{File: rel, Message: "cannot copy into the probe copy: " + err.Error()}
			}
		}
	}

	hooks := filepath.Join(s.tmp, "no-hooks")
	if err := os.MkdirAll(hooks, 0o750); err != nil {
		return &Finding{Message: "cannot create the probe copy: " + err.Error()}
	}
	steps := [][]string{
		{"init", "-q", "--initial-branch=main", "--template=" + hooks},
		{"config", "user.name", "vibe audit"},
		{"config", "user.email", "vibe-audit@localhost"},
		{"config", "core.autocrlf", "false"},
		{"config", "core.safecrlf", "false"},
		{"config", "commit.gpgsign", "false"},
		{"config", "core.hooksPath", hooks},
		{"add", "--all", "--force", "."},
		{"commit", "-q", "--no-verify", "--allow-empty", "-m", "vibe audit probe copy"},
	}
	for _, args := range steps {
		if _, err := s.git(args...); err != nil {
			return &Finding{Message: "cannot create the probe copy: " + err.Error()}
		}
	}
	head, err := s.git("rev-parse", "HEAD")
	if err != nil {
		return &Finding{Message: "cannot create the probe copy: " + err.Error()}
	}
	s.head = strings.TrimSpace(string(head))
	return nil
}

// git runs the audit's own git in the copy, with no variable that could
// point it at another repository.
func (s *probeSession) git(args ...string) ([]byte, error) {
	cmd := exec.CommandContext(s.ctx, "git", args...) // #nosec G204 -- literal git arguments
	cmd.Dir = s.root
	cmd.Env = append(envUnset(os.Environ(), gitRepoVariables...), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, &gitError{args: args, dir: s.root, stderr: strings.TrimSpace(stderr.String()), err: err}
	}
	return out, nil
}

// Root returns the copy's work tree.
func (s *probeSession) Root() string { return s.root }

// GitDir returns the copy's Git directory, which is also its common Git
// directory: the copy has no linked worktrees. A-08 writes the unlock
// marker there.
func (s *probeSession) GitDir() string { return filepath.Join(s.root, ".git") }

// Reset restores the copy's tracked files to its commit and deletes
// untracked files that are not ignored. Ignored files, such as what
// setup installed, stay.
func (s *probeSession) Reset() error {
	if _, err := s.git("reset", "-q", "--hard", s.head); err != nil {
		return err
	}
	_, err := s.git("clean", "-ffdq")
	return err
}

// Status returns git status --porcelain of the copy, one entry per path
// ("?? calc/x.go", " M go.mod"), untracked files listed one by one.
func (s *probeSession) Status() ([]string, error) {
	out, err := s.git("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var entries []string
	fields := splitNUL(out)
	for i := 0; i < len(fields); i++ {
		e := fields[i]
		entries = append(entries, e)
		// A rename or copy is followed by its source path.
		if len(e) > 1 && (e[0] == 'R' || e[0] == 'C') {
			i++
		}
	}
	return entries, nil
}

// Plant writes a file into the copy (rel is slash separated). It fails
// when the file already exists.
func (s *probeSession) Plant(rel string, content []byte) error {
	p := filepath.Join(s.root, filepath.FromSlash(rel))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- a path inside the probe copy
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ReadFile reads a file of the copy (rel is slash separated).
func (s *probeSession) ReadFile(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel))) // #nosec G304 -- a path inside the probe copy
}

// runSetup runs the setup entrypoint once, first, when it is declared.
func (s *probeSession) runSetup(env *Env) {
	if _, declared := env.Config.Entrypoints[EntrypointSetup]; !declared {
		return
	}
	run := s.RunEntrypoint(env, EntrypointSetup, EntrypointSetup)
	s.setup = &run
}

// precondition says why no part can run in this copy: it could not be
// built, or setup did not succeed. Nil when parts can run.
func (s *probeSession) precondition(env *Env) *Finding {
	if s.broken != nil {
		f := *s.broken
		f.Message = "cannot decide: " + f.Message
		return &f
	}
	if s.setup != nil && !s.setup.OK() {
		return &Finding{
			File:    env.Config.File,
			Message: "cannot decide: setup did not succeed, so no execution or probe part can run: " + s.setup.describe(),
			Fix:     "fix entrypoints.setup so that it succeeds in a fresh clone",
		}
	}
	return nil
}

// progress writes a progress line before a run.
func (s *probeSession) progress(label string) {
	if s.opts.Progress != nil {
		s.progressMu.Lock()
		defer s.progressMu.Unlock()
		_, _ = fmt.Fprintf(s.opts.Progress, "vibe audit: %s\n", label)
	}
}

// RunOpts change one run.
type RunOpts struct {
	// Dir is the run's directory, slash separated and relative to the
	// copy's root; "" is the root.
	Dir string
	// Env replaces the run's environment (A-03 reduces PATH with it).
	// Nil means the session's run environment.
	Env []string
	// Limit replaces the run's time limit.
	Limit time.Duration
}

func (s *probeSession) dir(o RunOpts) string {
	if o.Dir == "" || o.Dir == "." {
		return s.root
	}
	return filepath.Join(s.root, filepath.FromSlash(o.Dir))
}

func (s *probeSession) env(o RunOpts) []string {
	if o.Env != nil {
		return o.Env
	}
	return s.runEnv
}

// RunProgram runs argv directly, without a shell: the program found on
// the run's PATH. label names the run; file declares it.
func (s *probeSession) RunProgram(label, file string, argv []string, o RunOpts) runOutcome {
	out := runOutcome{Label: label, Command: strings.Join(argv, " "), File: file}
	env := s.env(o)
	prog := lookPath(argv[0], env, s.dir(o))
	if prog == "" {
		out.Undecided = fmt.Sprintf("`%s` is not installed on this machine", argv[0])
		return out
	}
	limit := o.Limit
	if limit <= 0 {
		limit = s.opts.Timeout
	}
	s.progress(label)
	out.ExitCode, out.Output, out.Undecided = execute(s.ctx, process{
		argv: append([]string{prog}, argv[1:]...), dir: s.dir(o), env: env, limit: limit,
	})
	s.recordShimCalls(&out)
	return out
}

// HookPayload is the JSON Claude Code sends a hook on stdin: the common
// fields and the event's own (spec 0053).
func (s *probeSession) HookPayload(event string, fields map[string]any) []byte {
	p := map[string]any{
		"session_id":      "vibe-audit",
		"transcript_path": "",
		"cwd":             s.root,
		"hook_event_name": event,
	}
	for k, v := range fields {
		p[k] = v
	}
	data, err := json.Marshal(p)
	if err != nil {
		panic(err) // the fields are plain values
	}
	return data
}

// RunHook runs one hook command the way Claude Code does: bash -c
// '<command>' with payload on stdin, in the copy. limit is the hook's own
// timeout, or zero for DefaultHookTimeout. Without bash, the outcome is
// undecided.
func (s *probeSession) RunHook(label, command string, limit time.Duration, payload []byte, o RunOpts) runOutcome {
	out := s.runHook(label, command, limit, payload, o)
	s.recordShimCalls(&out)
	return out
}

// runHook is RunHook without recording the shim's calls, so that runs
// can happen concurrently. The caller records the calls afterwards.
func (s *probeSession) runHook(label, command string, limit time.Duration, payload []byte, o RunOpts) runOutcome {
	out := runOutcome{Label: label, Command: command, File: claudeSettingsFile}
	if s.bashErr != nil {
		out.Undecided = s.bashErr.Error()
		return out
	}
	if limit <= 0 {
		limit = DefaultHookTimeout
	}
	if o.Limit > 0 {
		limit = o.Limit
	}
	s.progress(label)
	out.ExitCode, out.Output, out.Undecided = execute(s.ctx, process{
		argv: []string{s.bash, "-c", command}, dir: s.dir(o), env: s.env(o), stdin: payload, limit: limit,
	})
	return out
}

// HasBash reports whether hooks can run, and why not.
func (s *probeSession) HasBash() (bool, string) {
	if s.bashErr != nil {
		return false, s.bashErr.Error()
	}
	return true, ""
}

func (s *probeSession) recordShimCalls(out *runOutcome) {
	if s.shim == nil {
		return
	}
	for _, c := range s.shim.calls() {
		c.Run, c.File = out.Label, out.File
		out.ShimCalls = append(out.ShimCalls, c)
		s.shimCalls = append(s.shimCalls, c)
	}
}

// entrypointRun is one entrypoint run: its commands in order, stopping at
// the first that does not succeed (spec 0051).
type entrypointRun struct {
	runOutcome
	// Missing is V-04's finding when the entrypoint is unusable; nothing
	// ran.
	Missing *Finding
}

// RunEntrypoint runs a declared entrypoint in the copy's root, each
// command as a program with its arguments and its leading assignments
// added to the environment. label names the run, such as "fmt-check
// with the format defect in .".
func (s *probeSession) RunEntrypoint(env *Env, name, label string) entrypointRun {
	file := env.Config.File
	if _, missing := env.Entrypoint(name); missing != nil {
		return entrypointRun{runOutcome: runOutcome{Label: label, File: file, Undecided: missing.Message}, Missing: missing}
	}
	sources := env.EntrypointSource(name)
	run := entrypointRun{runOutcome: runOutcome{Label: label, Command: strings.Join(sources, "; "), File: file}}
	s.progress(label)
	for _, src := range sources {
		argv, assigns, why := runnableCommand(src)
		if why != "" {
			run.Undecided = fmt.Sprintf("cannot run %q without a shell: %s", src, why)
			return run
		}
		runEnv := s.runEnv
		for _, a := range assigns {
			k, v, _ := strings.Cut(a, "=")
			runEnv = envSet(runEnv, k, v)
		}
		prog := lookPath(argv[0], runEnv, s.root)
		if prog == "" {
			run.Undecided = fmt.Sprintf("`%s` is not installed on this machine", argv[0])
			return run
		}
		var out []byte
		run.ExitCode, out, run.Undecided = execute(s.ctx, process{
			argv: append([]string{prog}, argv[1:]...), dir: s.root, env: runEnv, limit: s.opts.Timeout,
		})
		run.Output = append(run.Output, out...)
		if len(run.Output) > maxCaptured {
			run.Output = run.Output[len(run.Output)-maxCaptured:]
		}
		s.recordShimCalls(&run.runOutcome)
		if !run.OK() {
			if len(sources) > 1 {
				run.Command = src
			}
			return run
		}
	}
	return run
}

// Baseline runs an entrypoint once in the reset copy and remembers the
// run: V-01, V-02 and R-01 read the same baseline of verify and
// verify-fast.
func (s *probeSession) Baseline(env *Env, name string) (entrypointRun, error) {
	if run, ok := s.baselines[name]; ok {
		return *run, nil
	}
	if err := s.Reset(); err != nil {
		return entrypointRun{}, err
	}
	run := s.RunEntrypoint(env, name, name)
	s.baselines[name] = &run
	return run, nil
}

// runnableCommand splits an entrypoint command into the program and its
// arguments, and its leading assignments as NAME=value. A word with a
// parameter expansion or command substitution needs a shell, so why
// says the command cannot run.
func runnableCommand(src string) (argv, assigns []string, why string) {
	cmd, err := ParseSimpleCommand(src)
	if err != nil {
		return nil, nil, err.Error()
	}
	for _, w := range cmd.Words {
		if !w.Literal {
			return nil, nil, fmt.Sprintf("the word %q holds a parameter expansion or command substitution", guardrule.ShowExpansions(w.Text))
		}
		argv = append(argv, w.Text)
	}
	assigns, why = commandAssignments(src)
	return argv, assigns, why
}
