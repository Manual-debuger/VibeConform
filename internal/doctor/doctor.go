// Package doctor diagnoses whether this machine can run a repository's
// workflow: the tools its standard needs, git, Task, and the environment
// facts that make Windows and Linux differ. It answers a different
// question from vibe audit, which asks whether the repository's files are
// right; doctor never looks at their content. See
// docs/specs/0028-environment-doctor.md.
//
// The package imports nothing from internal/: the CLI turns the resolved
// plan into check inputs, so doctor depends on no module and every check is
// a function of its inputs and an Env.
package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Status is one check's verdict.
type Status int

const (
	// Pass means checked, and healthy.
	Pass Status = iota
	// Warn means the workflow runs, but diverges from what CI sees or
	// lacks an optional capability.
	Warn
	// Fail means a required local workflow cannot run.
	Fail
	// Unverified means doctor cannot probe this reliably, and says so
	// rather than guessing.
	Unverified
)

// String returns the status as doctor prints it.
func (s Status) String() string {
	switch s {
	case Pass:
		return "PASS"
	case Warn:
		return "WARN"
	case Fail:
		return "FAIL"
	case Unverified:
		return "UNVERIFIED"
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// Result is one line of the report.
type Result struct {
	Status Status
	// Name is what was checked: a tool, or an aspect such as "git".
	Name string
	// Detail is the one-line reason.
	Detail string
}

// Report is every result, in the order the checks ran.
type Report struct {
	Results []Result
}

// Add appends results in order.
func (r *Report) Add(results ...Result) {
	r.Results = append(r.Results, results...)
}

// Count returns how many results have status s.
func (r *Report) Count(s Status) int {
	n := 0
	for _, res := range r.Results {
		if res.Status == s {
			n++
		}
	}
	return n
}

// statusWidth fits the longest status, UNVERIFIED, plus a space.
const statusWidth = len("UNVERIFIED") + 1

// Write prints one aligned line per result, then a summary line.
func (r *Report) Write(w io.Writer) error {
	nameWidth := 0
	for _, res := range r.Results {
		nameWidth = max(nameWidth, len(res.Name))
	}
	var b strings.Builder
	for _, res := range r.Results {
		fmt.Fprintf(&b, "%-*s %-*s  %s\n", statusWidth, res.Status, nameWidth, res.Name, res.Detail)
	}
	fmt.Fprintf(&b, "summary: %d pass, %d warn, %d fail, %d unverified\n",
		r.Count(Pass), r.Count(Warn), r.Count(Fail), r.Count(Unverified))
	_, err := io.WriteString(w, b.String())
	return err
}

// Env is everything a check reads from the machine. Tests replace it:
// contributor laptops and CI runners differ in what they have installed,
// so a check asserting on the real machine would test the machine.
type Env struct {
	// LookPath finds a binary on PATH.
	LookPath func(name string) (string, error)
	// Run runs name with args in dir and returns its stdout and stderr.
	// It must honor ctx's deadline.
	Run func(ctx context.Context, dir, name string, args ...string) (stdout, stderr string, err error)
	// Exists reports whether a path exists.
	Exists func(path string) bool
	// Open opens a file for reading, so a large file can be streamed.
	Open func(path string) (io.ReadCloser, error)
	// GOOS and GOARCH describe this machine, as Go names them.
	GOOS, GOARCH string
}

// System returns the Env of the running process.
func System() Env {
	return Env{
		LookPath: exec.LookPath,
		Run:      run,
		Exists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		Open: func(path string) (io.ReadCloser, error) {
			return os.Open(path) // #nosec G304 -- a path fixed by the check, under the repository root
		},
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
}

func run(ctx context.Context, dir, name string, args ...string) (string, string, error) {
	// #nosec G204 -- name and args are fixed by the checks in this package
	// or declared by a module's ToolRequirer, never read from the repository.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return stdout.String(), stderr.String(), err
}

// Timeouts. A version probe that hangs is a warning; git or Task hanging
// means the workflow cannot rely on them.
const (
	probeTimeout   = 5 * time.Second
	commandTimeout = 10 * time.Second
)

// firstLine returns s's first non-blank line, trimmed.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// why describes a failed command in a few words: its first line of stderr,
// or the error itself.
func why(err error, stderr string) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	if line := firstLine(stderr); line != "" {
		return line
	}
	return err.Error()
}

// Git checks that git runs and that repoRoot is inside a repository. Every
// later check that needs git depends on this one passing.
func Git(ctx context.Context, env Env, repoRoot string) Result {
	res := Result{Name: "git"}
	path, err := env.LookPath("git")
	if err != nil {
		res.Status, res.Detail = Fail, "not found on PATH"
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, stderr, err := env.Run(ctx, repoRoot, "git", "--version")
	if err != nil {
		res.Status, res.Detail = Fail, "git --version failed: "+why(err, stderr)
		return res
	}
	// The path says which git runs: in WSL, a Linux one or a Windows one
	// through interop (spec 0039).
	version := firstLine(out) + " (" + path + ")"
	if _, stderr, err := env.Run(ctx, repoRoot, "git", "rev-parse", "--git-dir"); err != nil {
		res.Status, res.Detail = Fail, version+"; not a readable git repository: "+why(err, stderr)
		return res
	}
	res.Status, res.Detail = Pass, version+"; repository readable"
	return res
}

// Tool is one binary the standard's modules require.
type Tool struct {
	Name string
	// Module and Why say who needs it and what stops working without it.
	Module, Why string
	// Version is the arguments that make it print its version; nil when
	// it has none.
	Version []string
	// Optional tools only degrade an optional capability: missing is a
	// warning, with Install saying how to get one.
	Optional bool
	Install  string
}

// Tools checks each required binary: on PATH, and reporting a version
// when it declares how. Task gets one more check on the same line, that the
// repository's Taskfile loads: a Taskfile that doesn't load turns off
// every task entry point, the agent guard included.
func Tools(ctx context.Context, env Env, repoRoot string, tools []Tool) []Result {
	results := make([]Result, 0, len(tools))
	for _, t := range tools {
		results = append(results, tool(ctx, env, repoRoot, t))
	}
	return results
}

func tool(ctx context.Context, env Env, repoRoot string, t Tool) Result {
	res := Result{Name: t.Name}
	path, err := env.LookPath(t.Name)
	if err != nil {
		if t.Optional {
			res.Status = Warn
			res.Detail = fmt.Sprintf("not found on PATH (optional, for %s: %s); install: %s", t.Module, t.Why, t.Install)
			return res
		}
		res.Status = Fail
		res.Detail = fmt.Sprintf("not found on PATH (required by %s: %s)", t.Module, t.Why)
		return res
	}

	res.Status, res.Detail = Pass, path
	if len(t.Version) > 0 {
		probe, cancel := context.WithTimeout(ctx, probeTimeout)
		out, _, err := env.Run(probe, repoRoot, t.Name, t.Version...)
		cancel()
		if version := firstLine(out); err == nil && version != "" {
			// Keep the path beside the version: in WSL it is what tells a
			// Linux binary from a Windows one (spec 0039).
			res.Detail = version + " (" + path + ")"
		} else {
			res.Status, res.Detail = Warn, path+": found on PATH but did not report a version"
		}
	}

	if t.Name == "task" {
		load, cancel := context.WithTimeout(ctx, commandTimeout)
		_, stderr, err := env.Run(load, repoRoot, "task", "--list-all")
		cancel()
		if err != nil {
			res.Status = Fail
			res.Detail += "; Taskfile.yml does not load: " + why(err, stderr)
		} else {
			res.Detail += "; Taskfile.yml loads"
		}
	}
	return res
}
