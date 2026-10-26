package cli

import (
	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

func newAuditCmd(seams contractSeams) *cobra.Command {
	f := auditFlags{seams: seams}

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Read-only audit of the repository against the contract",
		Long: "Read-only check of the repository against contract " + contract.Implemented + ".\n\n" +
			"It reads vibe.yaml version 2 (a top-level contract key). A version 1 file\n" +
			"(a top-level standard key) belongs to the template-sync system, which vibe\n" +
			"no longer has: vibe audit then names the migration guide and exits 1.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dispatchAudit(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.repoRoot, "repo-root", ".", "repository root to audit")
	cmd.Flags().StringVar(&f.config, "config", "", "configuration file to read instead of vibe.yaml at the repository root; must be version 2 (contract:)")
	cmd.Flags().BoolVar(&f.static, "static", false, "run only the static checks; never reaches a verdict")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the report as JSON")
	cmd.Flags().StringVar(&f.explain, "explain", "", "explain the rule with this ID and read nothing else")
	cmd.Flags().DurationVar(&f.probeTimeout, "probe-timeout", contract.DefaultProbeTimeout,
		"time limit of each entrypoint run in the probe copy")
	cmd.Flags().BoolVar(&f.keepProbeCopy, "keep-probe-copy", false,
		"keep the probe copy after the audit and print its path on stderr")

	return cmd
}
