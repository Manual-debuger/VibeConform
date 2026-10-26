package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// migrationGuidePath is the setup skill's migration guide as vibe init
// --skill writes it, in Claude Code's skill path.
var migrationGuidePath = setupskill.Dirs[0] + "/reference/migrate.md"

// migrationPointer is the line that sends a user of the template-sync
// system to the migration guide (spec 0059, Desired Behavior 8 and 9).
func migrationPointer() string {
	return "to leave the template-sync system, run `vibe init --skill` and follow the migration guide, " +
		migrationGuidePath
}

// newRemovedCmd is a hidden stub for a template-sync command that vibe no
// longer has: it accepts any arguments and flags, prints the migration
// pointer and exits 1.
func newRemovedCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("vibe %s was removed with the template-sync system; %s", name, migrationPointer())
		},
	}
}
