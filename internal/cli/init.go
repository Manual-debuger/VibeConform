package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

func newInitCmd() *cobra.Command {
	var (
		repoRoot string
		skill    bool
	)

	cmd := &cobra.Command{
		Use:   "init --skill",
		Short: "Write the setup skill into a repository",
		Long: "vibe init --skill writes the setup skill, " + setupskill.Name + ", to " +
			strings.Join(setupskill.Dirs, " and ") + ",\n" +
			"replacing its files and removing files it no longer has. It writes nothing else.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !skill {
				return errors.New("init: only vibe init --skill is available")
			}
			return runInitSkill(cmd, repoRoot)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to initialize")
	cmd.Flags().BoolVar(&skill, "skill", false, "write the setup skill ("+setupskill.Name+")")

	return cmd
}

// runInitSkill writes the setup skill into the repository (spec 0054,
// "vibe init --skill"). It reads no vibe.yaml, so it works with a
// version 1 configuration, a version 2 one or none.
func runInitSkill(cmd *cobra.Command, repoRoot string) error {
	res, err := setupskill.Install(repoRoot, runningVersion(cmd))
	if err != nil {
		return fmt.Errorf("init --skill: %w", err)
	}
	w := cmd.OutOrStdout()
	for _, p := range res.Removed {
		if _, err := fmt.Fprintf(w, "removed %s\n", p); err != nil {
			return fmt.Errorf("init --skill: %w", err)
		}
	}
	for _, p := range res.Written {
		if _, err := fmt.Fprintf(w, "wrote %s\n", p); err != nil {
			return fmt.Errorf("init --skill: %w", err)
		}
	}
	return nil
}
