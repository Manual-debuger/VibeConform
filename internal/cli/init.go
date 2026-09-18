package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
)

const manifestFileName = "vibe.yaml"

func newInitCmd() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:   "init <standard> <version>",
		Short: "Write a vibe.yaml desired-state declaration for a repository",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, repoRoot, args[0], args[1])
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to initialize")

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
