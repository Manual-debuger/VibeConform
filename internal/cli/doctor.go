package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

func newDoctorCmd(seams cliSeams) *cobra.Command {
	var repoRoot string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose whether this machine can run the repository's workflow",
		Long: "vibe doctor reads vibe.yaml version 2 and .claude/settings.json and checks,\n" +
			"one line each: git; the program of each entrypoint command (on PATH; at its\n" +
			"path when absolute, a leading / included; in the repository when named by a\n" +
			"relative path); the bash that runs the hook commands\n" +
			"(Git Bash on Windows), found as vibe audit finds it; and vibe-guard on PATH\n" +
			"when a PreToolUse hook runs it. It prints PASS, FAIL or UNVERIFIED\n" +
			"per check and exits 1 if any check fails. Read-only: it installs nothing,\n" +
			"writes nothing, and does not check conformance (that is vibe audit).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, repoRoot, seams)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to diagnose")
	return cmd
}

// runDoctor loads the configuration, has doctor.Run check the machine,
// and prints the report. A load error gets the audit's own message. Only
// a FAIL makes it return an error, which exits 1: the workflow cannot
// run here.
func runDoctor(cmd *cobra.Command, repoRoot string, seams cliSeams) error {
	ctx := commandContext(cmd)
	env := doctor.System
	if seams.doctorEnv != nil {
		env = seams.doctorEnv
	}
	var loaded doctor.Loaded
	loaded.Config, loaded.Repo, loaded.Err = contract.Load(ctx, repoRoot, "", seams.openRepo)
	if loaded.Err != nil {
		loaded.Err = configLoadError(loaded.Err)
	}
	report := doctor.Run(ctx, env(), repoRoot, loaded)

	if err := report.Write(cmd.OutOrStdout()); err != nil {
		return err
	}
	if n := report.Count(doctor.Fail); n > 0 {
		noun := "check"
		if n > 1 {
			noun = "checks"
		}
		return fmt.Errorf("doctor: %d %s failed", n, noun)
	}
	return nil
}
