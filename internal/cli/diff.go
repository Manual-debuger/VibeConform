package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/reconcile"
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
	p, err := buildPlan(repoRoot)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "standard: %s/%s\n", p.Standard.Name, p.Standard.Version); err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	for _, rp := range p.Resources {
		line := "not yet supported by diff"
		if rp.Supported {
			line = diffLine(rp.Decision)
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", rp.Resource.Path, line); err != nil {
			return fmt.Errorf("diff: %w", err)
		}
	}

	return nil
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
