// Package cli wires together the vibe command tree.
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd constructs the root "vibe" command with all subcommands attached.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "vibe",
		Short:         "VibeConform: desired-state repository control plane",
		Long:          "VibeConform (vibe) initializes, audits, and reconciles repositories\nagainst a versioned production standard.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       version,
	}

	root.AddCommand(
		newInitCmd(),
		newAuditCmd(),
		newDiffCmd(),
		newSyncCmd(),
		newCheckCmd(),
		newDoctorCmd(),
	)

	return root
}

// runningVersion returns the version string of the binary executing cmd.
//
// It reads cobra's own root Version rather than threading a parameter
// through every command constructor: main already passes the ldflags-set
// version to NewRootCmd, and a second copy of it would be a second thing to
// keep in sync. Commands that record provenance must agree with what
// "vibe --version" prints, and this is the only way to guarantee that.
//
// It is "dev" for any build GoReleaser did not stamp, and whatever a test
// passed to NewRootCmd under test. Neither is valid semver, which
// state.CompareWriters treats as unorderable rather than as a version.
func runningVersion(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return cmd.Root().Version
}
