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

	printWarnings(cmd.ErrOrStderr(), p)

	for _, rp := range p.Resources {
		if rp.Section != nil && !rp.Ignored {
			if _, err := fmt.Fprintf(out, "%s: %s\n", sectionLabel(rp.Resource.Path, rp.Section.ID), diffSectionLine(rp.SectionFile, rp.Section)); err != nil {
				return fmt.Errorf("diff: %w", err)
			}
			continue
		}
		if rp.Patch != nil && rp.Supported && !rp.Ignored {
			if err := printElementLines(out, rp.Resource.Path, elementLines(rp.Patch, diffElement), "no change"); err != nil {
				return fmt.Errorf("diff: %w", err)
			}
			continue
		}
		line := "not yet supported by diff"
		switch {
		case rp.Supported && rp.Ignored:
			line = ignoredLine(rp.Resource.Path)
		case rp.Supported:
			line = diffLine(rp.Decision)
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", rp.Resource.Path, line); err != nil {
			return fmt.Errorf("diff: %w", err)
		}
	}
	for _, pp := range p.Prunes {
		if pp.Section != nil {
			if _, err := fmt.Fprintf(out, "%s: %s\n", sectionLabel(pp.Path, pp.Section.ID), diffSectionPruneLine(pp.SectionFile, pp.Section, pp.cause())); err != nil {
				return fmt.Errorf("diff: %w", err)
			}
			continue
		}
		if pp.Patch != nil {
			if err := printElementLines(out, pp.Path, patchPruneLines(pp, diffElement), ""); err != nil {
				return fmt.Errorf("diff: %w", err)
			}
			continue
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", pp.Path, diffPruneLine(pp)); err != nil {
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
	case reconcile.LocalDrift:
		return "would update (file edited since last applied state)"
	case reconcile.OutOfDate:
		return "would update (standard moved since last applied state)"
	case reconcile.Conflict:
		return "conflict: manual changes detected, review before sync"
	default:
		return d.String()
	}
}
