// Package cli wires together the vibe command tree.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// NewRootCmd constructs the root "vibe" command with all subcommands attached.
func NewRootCmd(version string) *cobra.Command {
	return newRootCmd(version, contractSeams{})
}

// newRootCmd is NewRootCmd with the contract audit's test seams.
func newRootCmd(version string, seams contractSeams) *cobra.Command {
	root := &cobra.Command{
		Use:   "vibe",
		Short: "VibeConform: audit a repository against the VibeConform contract",
		Long: "VibeConform (vibe) audits a repository against the VibeConform contract\n" +
			"(contract " + contract.Implemented + "). The repository declares its entrypoints, components,\n" +
			"platforms and agents in vibe.yaml version 2 and owns every file; vibe audit\n" +
			"reads them and reports, rule by rule, whether the repository conforms.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       version,
	}
	// vibe --version names the contract version vibe implements (spec
	// 0054); the setup skill's upgrade flow reads it.
	root.SetVersionTemplate("vibe version {{.Version}} (contract " + contract.Implemented + ")\n")

	root.AddCommand(
		newInitCmd(),
		newAuditCmd(seams),
		newDoctorCmd(),
		newUnlockCmd(),
	)
	for _, name := range []string{"sync", "diff", "check"} {
		root.AddCommand(newRemovedCmd(name))
	}

	return root
}

// runningVersion returns the version string of the binary executing cmd.
//
// It reads cobra's own root Version rather than threading a parameter
// through every command constructor: main already passes the ldflags-set
// version to NewRootCmd, and a second copy of it would be a second thing to
// keep in sync. The audit report and the setup skill name this version, and
// must agree with what "vibe --version" prints.
//
// It is "dev" for any build GoReleaser did not stamp, and whatever a test
// passed to NewRootCmd under test.
func runningVersion(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return cmd.Root().Version
}
