package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/state"
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

	var checked, drifted, outOfDate, conflicts int
	for _, rp := range p.Resources {
		line := "not yet checked (unsupported ownership)"
		if rp.Supported {
			checked++
			line = auditLine(rp.Decision)
			switch rp.Decision {
			case reconcile.Create, reconcile.LocalDrift:
				drifted++
			case reconcile.OutOfDate:
				outOfDate++
			case reconcile.Conflict:
				conflicts++
			case reconcile.NoChange:
			}
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", rp.Resource.Path, line); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
	}

	if _, err := fmt.Fprintf(out, "%d %s checked, %d drifted, %d out of date, %d conflicts\n",
		checked, pluralize(checked, "resource"), drifted, outOfDate, conflicts); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	// A binary older than the one that wrote the state cannot judge this
	// repository: its embedded templates predate the repository's own
	// state, so every finding above is suspect and "run vibe sync" would
	// revert managed files rather than update them. No verdict is printed,
	// because there isn't one to give — announcing "not conformant" and
	// then explaining that the findings cannot be trusted would be two
	// contradictory claims in a row (spec 0019).
	recorded := p.Previous.VibeVersion
	running := runningVersion(cmd)
	if state.CompareWriters(recorded, running) == state.WriterRunningOlder {
		if _, err := fmt.Fprintf(out,
			"no verdict: this vibe is %s, older than the %s that last synced this\n"+
				"repository. Its templates predate this repository's state, so the\n"+
				"findings above cannot be trusted and vibe sync would revert managed\n"+
				"files. Upgrade vibe and audit again.\n", running, recorded); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return fmt.Errorf("audit: %w", &staleBinaryError{recorded: recorded, running: running})
	}

	verdict := "conformant"
	if drifted+outOfDate+conflicts > 0 {
		verdict = "not conformant"
	}
	if _, err := fmt.Fprintln(out, verdict); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	// Being out of date is not the repository's doing, so name the two
	// binaries whenever they are known — whether or not they order. The
	// mismatch being visible is what works in every case; ordering it is a
	// bonus that needs both versions to be valid semver.
	if outOfDate > 0 && recorded != "" && recorded != running {
		if _, err := fmt.Fprintf(out,
			"last synced by vibe %s; this vibe is %s\n", recorded, running); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
	}

	if drifted+outOfDate+conflicts > 0 {
		return fmt.Errorf("audit: %w", &nonConformantError{
			drifted: drifted, outOfDate: outOfDate, conflicts: conflicts,
		})
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
	case reconcile.LocalDrift:
		return "drifted (edited since last sync; run vibe sync to restore)"
	case reconcile.OutOfDate:
		return "out of date (standard moved; run vibe sync to update)"
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
