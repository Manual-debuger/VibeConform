package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

const manifestFileName = "vibe.yaml"

func newInitCmd() *cobra.Command {
	var (
		repoRoot string
		skill    bool
	)

	cmd := &cobra.Command{
		Use:   "init <standard> <version> | init --skill",
		Short: "Write a vibe.yaml desired-state declaration, or the setup skill, for a repository",
		Long: "vibe init <standard> <version> writes a vibe.yaml for the template-sync system.\n\n" +
			"vibe init --skill writes the setup skill, " + setupskill.Name + ", to " +
			strings.Join(setupskill.Dirs, " and ") + ",\n" +
			"replacing its files and removing files it no longer has. It writes nothing else.",
		Args: func(cmd *cobra.Command, args []string) error {
			if skill {
				if len(args) != 0 {
					return fmt.Errorf("init --skill takes no arguments, got %d", len(args))
				}
				return nil
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if skill {
				return runInitSkill(cmd, repoRoot)
			}
			return runInit(cmd, repoRoot, args[0], args[1])
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to initialize")
	cmd.Flags().BoolVar(&skill, "skill", false, "write the setup skill ("+setupskill.Name+") instead of vibe.yaml")

	return cmd
}

func runInit(cmd *cobra.Command, repoRoot, standard, version string) error {
	m, err := manifest.New(standard, version)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	path := filepath.Join(repoRoot, manifestFileName)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("init: %s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("init: %w", err)
	}

	data, err := m.Marshal()
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("init: write %s: %w", path, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (standard=%s version=%s)\n", path, standard, version); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return nil
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
