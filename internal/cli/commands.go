package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// notImplementedDoc is where errNotImplemented points: the architecture
// overview's "Not built yet" list, which tracks what these commands wait on.
const notImplementedDoc = "docs/architecture/overview.md"

// errNotImplemented reports a command that is intentionally scaffolded but not
// yet implemented, so agents and users get a clear signal instead of a silent
// no-op.
func errNotImplemented(cmd string) error {
	return fmt.Errorf("%s: not implemented yet (see %s)", cmd, notImplementedDoc)
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
