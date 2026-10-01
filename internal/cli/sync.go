package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/atomicfile"
	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// lefthookResourcePath is the resource whose presence in a standard means
// the repository has asked for git hooks. See
// docs/specs/0014-m2-milestone.md.
const lefthookResourcePath = "lefthook.yml"

// errLefthookNotFound reports that the lefthook binary is not on PATH, as
// distinct from lefthook running and failing.
var errLefthookNotFound = errors.New("lefthook not found on PATH")

// installGitHooks is a test seam. The CLI suite syncs into t.TempDir(),
// which is not a git work tree, so every sync test would shell out to a
// lefthook install that cannot succeed.
var installGitHooks = runLefthookInstall

func newSyncCmd() *cobra.Command {
	var repoRoot string
	var allowDowngrade bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile the repository against the desired standard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, repoRoot, allowDowngrade)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to sync")
	cmd.Flags().BoolVar(&allowDowngrade, "allow-downgrade", false,
		"write managed files even though this vibe is older than the one that last synced this repository")

	return cmd
}

// syncCounts tallies what a sync run did, for the closing summary line.
type syncCounts struct {
	created   int
	updated   int
	unchanged int
	removed   int
	conflicts int
	ignored   int
}

func runSync(cmd *cobra.Command, repoRoot string, allowDowngrade bool) error {
	p, err := buildPlan(repoRoot)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "standard: %s/%s\n", p.Standard.Name, p.Standard.Version); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	// Refuse before writing anything, not after. An older binary's
	// templates predate this repository's state, so syncing would revert
	// managed files and then record the reverted content as correct —
	// verified to undo entire specs in one command (spec 0019). Reverting
	// on purpose is legitimate, so there is an opt-in, but it has to be
	// typed rather than stumbled into.
	recorded, running := p.Previous.VibeVersion, runningVersion(cmd)
	if state.CompareWriters(recorded, running) == state.WriterRunningOlder && !allowDowngrade {
		return fmt.Errorf("sync: %w: syncing would revert managed files to older "+
			"templates; pass --allow-downgrade if that is what you mean",
			&staleBinaryError{recorded: recorded, running: running})
	}

	// Before anything is written: if this machine cannot run what the
	// standard configures, say so above the report rather than below it.
	printWarnings(cmd.ErrOrStderr(), p)
	warnMissingTools(cmd.ErrOrStderr(), p.Standard, p.Context)
	warnUnpinnable(cmd.ErrOrStderr(), repoRoot, runningVersion(cmd))

	// Start from what was recorded before, so resources this run refuses to
	// touch — conflicts — keep the entry they already had. Provenance is
	// this run's, not the previous one's: the file records who wrote it
	// last, and that is about to be us (spec 0019).
	next := &state.State{
		Schema:      state.SchemaVersion,
		VibeVersion: runningVersion(cmd),
		Standard:    fmt.Sprintf("%s/%s", p.Standard.Name, p.Standard.Version),
		Resources:   make(map[string]state.ResourceState, len(p.Previous.Resources)),
		Sections:    make(map[state.SectionKey]state.SectionState, len(p.Previous.Sections)),
	}
	maps.Copy(next.Resources, p.Previous.Resources)
	maps.Copy(next.Sections, p.Previous.Sections)

	var counts syncCounts
	applyErr := applyPlan(out, repoRoot, p.Resources, next, &counts)
	if applyErr == nil {
		applyErr = applyPrunes(out, repoRoot, p.Prunes, next, &counts)
	}

	// Record what actually landed before surfacing any failure: a run that
	// wrote some resources and then died must not leave them unrecorded, or
	// the next run reports them as conflicts.
	saveErr := state.Save(repoRoot, next)

	if applyErr != nil || saveErr != nil {
		return fmt.Errorf("sync: %w", errors.Join(applyErr, saveErr))
	}

	// The removed count appears only when something was removed, so a
	// repository with nothing deselected reports exactly as before.
	summary := fmt.Sprintf("%d created, %d updated, %d unchanged, %d conflicts",
		counts.created, counts.updated, counts.unchanged, counts.conflicts)
	if counts.removed > 0 {
		summary += fmt.Sprintf(", %d removed", counts.removed)
	}
	if _, err := fmt.Fprintln(out, summary); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if counts.ignored > 0 {
		return fmt.Errorf("sync: %d managed file(s) ignored by git, not written: "+
			"add them to .gitignore as exceptions, then re-run", counts.ignored)
	}

	if counts.conflicts > 0 {
		return fmt.Errorf("sync: %d conflict(s): resolve by hand or delete the file, then re-run", counts.conflicts)
	}

	// Only after a clean run: a sync that refused to write part of the
	// standard has not finished configuring the repository, and registering
	// hooks against a half-applied one is worse than leaving it to whoever
	// now has a conflict to resolve.
	registerGitHooks(cmd, repoRoot, p)
	return nil
}

// registerGitHooks runs lefthook install when the standard manages
// lefthook.yml. Writing that file without registering the hooks leaves a
// pre-commit gate that is configured and off — a guardrail that fails
// silently and fails open, which is the failure mode the standard exists to
// prevent.
//
// Nothing here can fail a sync. The resources are written and recorded by
// the time it runs, and a machine without lefthook — or a directory that is
// not a git work tree — is not a conformance problem.
func registerGitHooks(cmd *cobra.Command, repoRoot string, p *repoPlan) {
	if !planManagesLefthook(p) {
		return
	}

	// Write errors are dropped throughout: there is nowhere left to report
	// them, and a status line that failed to print must not turn a
	// successful sync into a failed one.
	switch err := installGitHooks(cmd.Context(), repoRoot); {
	case err == nil:
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "lefthook: git hooks registered")
	case errors.Is(err, errLefthookNotFound):
		// Deliberately silent. repo-tooling declares lefthook, so
		// warnMissingTools already named it at the top of the run, and one
		// missing binary should produce one line of output.
	default:
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: git hooks were not registered: %v\n", err)
	}
}

// planManagesLefthook reports whether the resolved standard owns
// lefthook.yml, which is what makes hook registration VibeConform's
// business rather than the operator's.
func planManagesLefthook(p *repoPlan) bool {
	for _, rp := range p.Resources {
		if rp.Supported && rp.Resource.Path == lefthookResourcePath {
			return true
		}
	}
	return false
}

// runLefthookInstall registers the hooks described by repoRoot's
// lefthook.yml.
func runLefthookInstall(ctx context.Context, repoRoot string) error {
	if _, err := exec.LookPath("lefthook"); err != nil {
		return errLefthookNotFound
	}

	// Literal command and argument: nothing here is caller-controlled
	// except the working directory.
	c := exec.CommandContext(ctx, "lefthook", "install")
	c.Dir = repoRoot

	out, err := c.CombinedOutput()
	if err == nil {
		return nil
	}
	// "exit status 128" on its own is useless; the reason is in lefthook's
	// output, and not on a line this code can predict — it boxes a command
	// echo above the actual message. Pass the whole thing through rather
	// than guessing which line matters.
	if detail := trimOutput(out, err.Error()); detail != "" {
		return fmt.Errorf("%w\n%s", err, detail)
	}
	return err
}

// maxOutputLines caps how much of a failing subprocess's output is repeated
// into a warning, so a chatty tool cannot bury the sync report above it.
const maxOutputLines = 10

// trimOutput reduces subprocess output to the lines worth repeating under a
// warning: nothing that carries no information, nothing that merely repeats
// the error already being reported as redundant, and no more than
// maxOutputLines of it. Callers pass the error text as redundant.
//
// The rules are deliberately about information rather than about any one
// tool's formatting: lefthook draws a box and pads it to a fixed width, but
// encoding that here would make the next subprocess someone else's problem.
func trimOutput(b []byte, redundant string) string {
	var kept []string
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if !informative(line) || strings.TrimSpace(line) == redundant {
			continue
		}
		if len(kept) == maxOutputLines {
			kept = append(kept, "...")
			break
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// informative reports whether a line says anything: one with no letters and
// no digits is whitespace, box drawing, or a rule, and repeating it into a
// warning only costs the reader a line.
func informative(line string) bool {
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// applyPlan applies each planned resource in order, reporting one line per
// resource. It stops at the first write failure; the caller still persists
// whatever was recorded into next before returning.
func applyPlan(out io.Writer, repoRoot string, plans []resourcePlan, next *state.State, counts *syncCounts) error {
	for _, rp := range plans {
		if rp.Patch != nil && rp.Supported && !rp.Ignored {
			lines, err := applyPatchResource(repoRoot, rp, next, counts)
			if err != nil {
				return fmt.Errorf("%s: %w", rp.Resource.Path, err)
			}
			if err := printElementLines(out, rp.Resource.Path, lines, "unchanged"); err != nil {
				return err
			}
			continue
		}
		line, err := applyResource(repoRoot, rp, next, counts)
		if err != nil {
			return fmt.Errorf("%s: %w", rp.Resource.Path, err)
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", rp.Resource.Path, line); err != nil {
			return err
		}
	}
	return nil
}

// applyPatchResource applies one structured-patch resource and returns a
// line per element it changed.
func applyPatchResource(repoRoot string, rp resourcePlan, next *state.State, counts *syncCounts) ([]string, error) {
	pp := rp.Patch
	lines := elementLines(pp, syncElement)
	if pp.conflicted() {
		counts.conflicts++
		return lines, nil
	}
	if err := applyPatch(repoRoot, rp.Resource, pp); err != nil {
		return nil, err
	}
	key := stateKey(rp.Resource.Path)
	if ns := pp.nextState(); ns != nil {
		next.Resources[key] = *ns
	} else {
		delete(next.Resources, key)
	}
	switch {
	case !pp.Exists && pp.Content != nil:
		counts.created++
		return append([]string{"created"}, lines...), nil
	case rp.Decision == reconcile.NoChange:
		counts.unchanged++
	default:
		counts.updated++
	}
	return lines, nil
}

// applyPrunes applies each removal in order, as applyPlan does for
// resources.
func applyPrunes(out io.Writer, repoRoot string, prunes []prunePlan, next *state.State, counts *syncCounts) error {
	for _, pp := range prunes {
		lines, err := applyPrune(repoRoot, pp, next, counts)
		if err != nil {
			return fmt.Errorf("%s: %w", pp.Path, err)
		}
		if err := printElementLines(out, pp.Path, lines, ""); err != nil {
			return err
		}
	}
	return nil
}

// applyResource carries out the decision for one resource and returns the
// line describing what it did.
func applyResource(repoRoot string, rp resourcePlan, next *state.State, counts *syncCounts) (string, error) {
	if !rp.Supported {
		return "not yet supported by sync", nil
	}
	if rp.Ignored {
		counts.ignored++
		return ignoredLine(rp.Resource.Path), nil
	}

	key := stateKey(rp.Resource.Path)
	switch rp.Decision {
	// LocalDrift and OutOfDate both write the target; they differ only in
	// what audit says about how the repository got here (spec 0019).
	case reconcile.Create, reconcile.LocalDrift, reconcile.OutOfDate:
		if err := writeResource(repoRoot, rp.Resource); err != nil {
			return "", err
		}
		next.Resources[key] = state.ResourceState{SHA256: rp.TargetHash}
		if rp.Decision == reconcile.Create {
			counts.created++
			return "created", nil
		}
		counts.updated++
		return "updated", nil

	case reconcile.NoChange:
		// Record the hash even though nothing was written: a repository
		// that already matched becomes tracked, so later runs can decide
		// three-way instead of falling back to two-way.
		next.Resources[key] = state.ResourceState{SHA256: rp.TargetHash}
		counts.unchanged++
		return "unchanged", nil

	case reconcile.Conflict:
		counts.conflicts++
		return "conflict: manual changes detected, resolve before sync", nil

	default:
		return "", fmt.Errorf("unknown decision %v", rp.Decision)
	}
}

// writeResource writes one resolved resource to the repository, creating
// any missing parent directories.
func writeResource(repoRoot string, r resource.Resource) error {
	path := resourcePath(repoRoot, r.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// Content is written byte for byte as the module resolved it, with no
	// line-ending translation: this repository pins eol=lf in
	// .gitattributes, and translating on write would hash differently on
	// Windows and report drift forever.
	return atomicfile.Write(path, r.Content, r.ModeOrDefault())
}
