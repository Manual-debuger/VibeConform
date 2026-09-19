package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/reconcile"
)

func newAuditCmd() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Read-only compliance and drift check against the desired standard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudit(cmd, repoRoot)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to audit")

	return cmd
}

func runAudit(cmd *cobra.Command, repoRoot string) error {
	p, err := buildPlan(repoRoot)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "standard: %s/%s\n", p.Standard.Name, p.Standard.Version); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	var checked, drifted, conflicts int
	for _, rp := range p.Resources {
		line := "not yet checked (unsupported ownership)"
		if rp.Supported {
			checked++
			line = auditLine(rp.Decision)
			switch rp.Decision {
			case reconcile.Create, reconcile.Overwrite:
				drifted++
			case reconcile.Conflict:
				conflicts++
			case reconcile.NoChange:
			}
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", rp.Resource.Path, line); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
	}

	if _, err := fmt.Fprintf(out, "%d %s checked, %d drifted, %d conflicts\n",
		checked, pluralize(checked, "resource"), drifted, conflicts); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	verdict := "conformant"
	if drifted+conflicts > 0 {
		verdict = "not conformant"
	}
	if _, err := fmt.Fprintln(out, verdict); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	if drifted+conflicts > 0 {
		return fmt.Errorf("audit: %w", &nonConformantError{drifted: drifted, conflicts: conflicts})
	}
	return nil
}

// auditLine describes a resource's present state, as opposed to diffLine's
// description of the action sync would take.
func auditLine(d reconcile.Decision) string {
	switch d {
	case reconcile.NoChange:
		return "ok"
	case reconcile.Create:
		return "missing (run vibe sync)"
	case reconcile.Overwrite:
		return "drifted (run vibe sync)"
	case reconcile.Conflict:
		return "conflict: manual changes detected"
	default:
		return d.String()
	}
}

func pluralize(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
