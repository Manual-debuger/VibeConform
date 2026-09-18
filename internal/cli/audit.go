package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/standard"
)

func newAuditCmd() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Read-only compliance and drift check against the desired standard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudit(cmd, repoRoot)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to audit")

	return cmd
}

func runAudit(cmd *cobra.Command, repoRoot string) error {
	path := filepath.Join(repoRoot, manifestFileName)
	data, err := os.ReadFile(path) // #nosec G304 -- repoRoot is an operator-supplied CLI flag, same trust boundary as init.go's WriteFile target
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	m, err := manifest.Parse(data)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	s, err := standard.Lookup(m.Standard, m.Version)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "standard: %s/%s\n%d modules configured, nothing to check\n", s.Name, s.Version, len(s.Modules)); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}
