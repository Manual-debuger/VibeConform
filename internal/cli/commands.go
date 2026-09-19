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
