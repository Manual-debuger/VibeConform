// Package doctor diagnoses whether this machine can run a repository's
// workflow, as its vibe.yaml version 2 and .claude/settings.json declare
// it: git, the program of each entrypoint command, the bash that runs the
// hook commands, and vibe-guard when a hook calls it (spec 0059, Desired
// Behavior 11). It answers a different question from vibe audit, which
// asks whether the repository conforms to the contract.
//
// It reads the configuration and the hooks with the contract audit's own
// readers, and imports no other package of this module. Every check is a
// function of its inputs and an Env.
package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
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
	// Name is what was checked: a program, or an aspect such as "git".
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
	// Bash finds the bash that runs hook commands, or says why there is
	// none.
	Bash func() (string, error)
}

// System returns the Env of the running process. Its Bash is the contract
// audit's: Git Bash on Windows, bash on PATH elsewhere.
func System() Env {
	return Env{
		LookPath: exec.LookPath,
		Run:      run,
		Exists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		Bash: func() (string, error) { return contract.ResolveBash(os.Environ()) },
	}
}

func run(ctx context.Context, dir, name string, args ...string) (string, string, error) {
	// #nosec G204 -- name and args are fixed by the checks in this package.
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

// commandTimeout bounds each git command: git hanging means the workflow
// cannot rely on it.
const commandTimeout = 10 * time.Second

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
