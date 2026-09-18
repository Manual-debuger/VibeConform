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
