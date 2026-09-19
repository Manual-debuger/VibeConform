package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/standard"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

func newDiffCmd() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Preview the reconciliation VibeConform would perform",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd, repoRoot)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to diff")

	return cmd
}

func runDiff(cmd *cobra.Command, repoRoot string) error {
	path := filepath.Join(repoRoot, manifestFileName)
	data, err := os.ReadFile(path) // #nosec G304 -- repoRoot is an operator-supplied CLI flag, same trust boundary as init.go's WriteFile target
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	m, err := manifest.Parse(data)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	s, err := standard.Lookup(m.Standard, m.Version)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	st, err := state.Load(repoRoot)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "standard: %s/%s\n", s.Name, s.Version); err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	mctx := &module.Context{RepoRoot: repoRoot}
	for _, mod := range s.Modules {
		resources, err := mod.Resolve(context.Background(), mctx)
		if err != nil {
			return fmt.Errorf("diff: resolve %s: %w", mod.Name(), err)
		}
		for _, r := range resources {
			if err := reportResource(out, repoRoot, st, r); err != nil {
				return fmt.Errorf("diff: %w", err)
			}
		}
	}

	return nil
}

func reportResource(out io.Writer, repoRoot string, st *state.State, r resource.Resource) error {
	if r.Ownership != resource.Generated {
		_, err := fmt.Fprintf(out, "%s: not yet supported by diff\n", r.Path)
		return err
	}

	target := hashHex(r.Content)

	var current *string
	currentData, err := os.ReadFile(filepath.Join(repoRoot, r.Path)) // #nosec G304 -- repoRoot/r.Path come from an operator-supplied CLI flag and a registered module's fixed resource path
	switch {
	case errors.Is(err, os.ErrNotExist):
		current = nil
	case err != nil:
		return err
	default:
		h := hashHex(currentData)
		current = &h
	}

	var previous *string
	if rs, ok := st.Resources[r.Path]; ok {
		previous = &rs.SHA256
	}

	decision := reconcile.Decide(previous, current, target)
	_, err = fmt.Fprintf(out, "%s: %s\n", r.Path, diffLine(decision))
	return err
}

func diffLine(d reconcile.Decision) string {
	switch d {
	case reconcile.Create:
		return "create (no file on disk)"
	case reconcile.NoChange:
		return "no change"
	case reconcile.Overwrite:
		return "would update (drift from last applied state)"
	case reconcile.Conflict:
		return "conflict: manual changes detected, review before sync"
	default:
		return d.String()
	}
}

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
