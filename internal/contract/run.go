package contract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// How the audit runs commands (spec 0053, "How the audit runs
// commands"): entrypoints without a shell, hooks through bash, each run
// with a time limit, its process tree killed when it exceeds it, and its
// output captured.

// DefaultProbeTimeout is the time limit of one entrypoint run unless
// --probe-timeout says otherwise.
const DefaultProbeTimeout = 30 * time.Minute

// DefaultHookTimeout is the time limit of a hook that sets no timeout of
// its own: Claude Code's default for command hooks on PreToolUse, Stop and
// SessionStart (https://code.claude.com/docs/en/hooks, checked 2026-10-18).
const DefaultHookTimeout = 600 * time.Second

// tailLines is how many lines of a failed run's output a finding shows.
const tailLines = 20

// maxCaptured bounds the output kept per run; only its end is shown.
const maxCaptured = 256 << 10

// runOutcome is one run as the audit observed it.
type runOutcome struct {
	// Label names the run in progress lines and findings, such as
	// "verify" or "the SessionStart hook".
	Label string
	// Command is what ran, as the configuration writes it.
	Command string
	// File is the file that declares what ran: the configuration for an
	// entrypoint, .claude/settings.json for a hook.
	File string
	// Undecided says why the run could not happen or end on its own: a
	// program that is not installed, a word that needs a shell, a time
	// limit, an interrupt. Empty when the run ended by itself.
	Undecided string
	// ExitCode is the run's exit code when it ended by itself.
	ExitCode int
	// Output is the end of the run's stdout and stderr, interleaved.
	Output []byte
	// ShimCalls are the calls of vibe the R-01 shim recorded during the
	// run.
	ShimCalls []shimCall
}

// Ran reports whether the run ended by itself, with any exit code.
func (o runOutcome) Ran() bool { return o.Undecided == "" }

// OK reports whether the run ended by itself with exit code 0.
func (o runOutcome) OK() bool { return o.Ran() && o.ExitCode == 0 }

// Tail returns the last lines of the run's output, indented for a
// finding, or "" when it printed nothing.
func (o runOutcome) Tail() string { return outputTail(o.Output) }

// outputTail returns the last tailLines lines of out.
func outputTail(out []byte) string {
	text := strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	return strings.Join(lines, "\n")
}

// describe says how a run ended, for a finding: "verify (task verify)
// exited 1", followed by the end of its output.
func (o runOutcome) describe() string {
	what := o.Label
	if o.Command != "" && o.Command != o.Label {
		what += " (" + o.Command + ")"
	}
	var msg string
	if o.Ran() {
		msg = fmt.Sprintf("%s exited %d", what, o.ExitCode)
	} else {
		msg = fmt.Sprintf("%s: %s", what, o.Undecided)
	}
	if tail := o.Tail(); tail != "" {
		msg += "; the end of its output:\n" + tail
	}
	return msg
}

// tailBuffer keeps the last maxCaptured bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - maxCaptured; over > 0 {
		b.buf = append(b.buf[:0:0], b.buf[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf)
}

// process is one program to start.
type process struct {
	argv  []string
	dir   string
	env   []string
	stdin []byte
	limit time.Duration
}

// execute runs p and waits for it. A run that exceeds its limit, or whose
// context ends, is killed with its child processes.
func execute(ctx context.Context, p process) (exitCode int, output []byte, undecided string) {
	runCtx, cancel := context.WithTimeout(ctx, p.limit)
	defer cancel()

	cmd := exec.CommandContext(runCtx, p.argv[0], p.argv[1:]...) // #nosec G204 -- the audit runs the repository's declared entrypoints and hooks; that is its job
	cmd.Dir = p.dir
	cmd.Env = p.env
	cmd.Stdin = bytes.NewReader(p.stdin)
	out := &tailBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	tree := newProcTree(cmd)
	defer tree.close()
	cmd.Cancel = tree.kill
	// A child left running that holds the output open must not hold the
	// audit.
	cmd.WaitDelay = 5 * time.Second

	if err := cmd.Start(); err != nil {
		return -1, nil, fmt.Sprintf("cannot start %s: %v", p.argv[0], err)
	}
	tree.attach()
	err := cmd.Wait()

	switch {
	case ctx.Err() != nil:
		return -1, out.Bytes(), "interrupted"
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return -1, out.Bytes(), "timed out after " + p.limit.String()
	}
	var exit *exec.ExitError
	switch {
	case err == nil, errors.Is(err, exec.ErrWaitDelay):
		return cmd.ProcessState.ExitCode(), out.Bytes(), ""
	case errors.As(err, &exit):
		return exit.ExitCode(), out.Bytes(), ""
	default:
		return -1, out.Bytes(), err.Error()
	}
}

// envGet returns the value of name in env. On Windows names are not case
// sensitive. The last setting wins.
func envGet(env []string, name string) (string, bool) {
	val, found := "", false
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && envNameEqual(k, name) {
			val, found = v, true
		}
	}
	return val, found
}

// envSet returns env with name set to value, every other setting of name
// removed.
func envSet(env []string, name, value string) []string {
	out := envUnset(env, name)
	return append(out, name+"="+value)
}

// envUnset returns env without name.
func envUnset(env []string, names ...string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, n := range names {
			drop = drop || envNameEqual(k, n)
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

func envNameEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// gitRepoVariables are the variables that point Git at a repository. A
// run never sees them, so it cannot reach the audited repository (spec
// 0053, "Environment").
var gitRepoVariables = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
}

// lookPath finds a program the way a shell would on the run's PATH, with
// PATHEXT on Windows. A name with a path separator is taken relative to
// dir. It returns "" when nothing is found.
func lookPath(name string, env []string, dir string) string {
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
