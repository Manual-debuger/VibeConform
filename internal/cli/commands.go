package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// errNotImplemented reports a command that is intentionally scaffolded but not
// yet implemented, so agents and users get a clear signal instead of a silent
// no-op.
func errNotImplemented(cmd string) error {
	return fmt.Errorf("%s: not implemented yet (see docs/plans/0001-bootstrap.md)", cmd)
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new repository from the VibeConform standard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented("init")
		},
	}
}

func newAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "audit",
		Short: "Read-only compliance and drift check against the desired standard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented("audit")
		},
	}
}

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Preview the reconciliation VibeConform would perform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented("diff")
		},
	}
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Reconcile the repository against the desired standard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented("sync")
		},
	}
}

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run affected-component validation for the current change set",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented("check")
		},
	}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose local environment and tooling issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented("doctor")
		},
	}
}
