package cli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/atomicfile"
	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

func newSyncCmd() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile the repository against the desired standard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, repoRoot)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to sync")

	return cmd
}

// syncCounts tallies what a sync run did, for the closing summary line.
type syncCounts struct {
	created   int
	updated   int
	unchanged int
	conflicts int
}

func runSync(cmd *cobra.Command, repoRoot string) error {
	p, err := buildPlan(repoRoot)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "standard: %s/%s\n", p.Standard.Name, p.Standard.Version); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	// Start from what was recorded before, so resources this run refuses to
	// touch — conflicts — keep the entry they already had.
	next := &state.State{Resources: make(map[string]state.ResourceState, len(p.Previous.Resources))}
	maps.Copy(next.Resources, p.Previous.Resources)

	var counts syncCounts
	applyErr := applyPlan(out, repoRoot, p.Resources, next, &counts)

	// Record what actually landed before surfacing any failure: a run that
	// wrote some resources and then died must not leave them unrecorded, or
	// the next run reports them as conflicts.
	saveErr := state.Save(repoRoot, next)

	if applyErr != nil || saveErr != nil {
		return fmt.Errorf("sync: %w", errors.Join(applyErr, saveErr))
	}

	if _, err := fmt.Fprintf(out, "%d created, %d updated, %d unchanged, %d conflicts\n",
		counts.created, counts.updated, counts.unchanged, counts.conflicts); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if counts.conflicts > 0 {
		return fmt.Errorf("sync: %d conflict(s): resolve by hand or delete the file, then re-run", counts.conflicts)
	}
	return nil
}

// applyPlan applies each planned resource in order, reporting one line per
// resource. It stops at the first write failure; the caller still persists
// whatever was recorded into next before returning.
func applyPlan(out io.Writer, repoRoot string, plans []resourcePlan, next *state.State, counts *syncCounts) error {
	for _, rp := range plans {
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

// applyResource carries out the decision for one resource and returns the
// line describing what it did.
func applyResource(repoRoot string, rp resourcePlan, next *state.State, counts *syncCounts) (string, error) {
	if !rp.Supported {
		return "not yet supported by sync", nil
	}

	key := stateKey(rp.Resource.Path)
	switch rp.Decision {
	case reconcile.Create, reconcile.Overwrite:
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
	return atomicfile.Write(path, r.Content, 0o600)
}
