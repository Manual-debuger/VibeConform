package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// doctorEnv is a test seam: the CLI suite must assert on this code, not on
// what the machine running it has installed.
var doctorEnv = doctor.System

func newDoctorCmd() *cobra.Command {
	var repoRoot string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose whether this machine can run the repository's workflow",
		Long: "Checks git and the repository's vibe.yaml. Prints PASS, WARN, FAIL, or\n" +
			"UNVERIFIED per check and exits 1 if any required check fails. Read-only:\n" +
			"installs nothing, writes nothing, and does not check conformance (that is\n" +
			"vibe audit).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, repoRoot)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to diagnose")
	return cmd
}

// runDoctor runs every check in a fixed order and prints the report. Only
// a FAIL makes it return an error, which exits 1: the workflow cannot run
// here, so there is no usable verdict.
func runDoctor(cmd *cobra.Command, repoRoot string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	env := doctorEnv()
	var report doctor.Report

	git := doctor.Git(ctx, env, repoRoot)
	gitOK := git.Status == doctor.Pass
	report.Add(git)

	if label, _, err := readV2Config(repoRoot, ""); err != nil {
		report.Add(doctor.Result{Status: doctor.Fail, Name: configFileName, Detail: err.Error()})
	} else {
		report.Add(doctor.Result{Status: doctor.Pass, Name: configFileName, Detail: label + " is version 2"})
	}

	report.Add(doctor.Runtime(env))
	if gitOK {
		report.Add(doctor.Worktree(ctx, env, repoRoot))
	} else {
		report.Add(doctor.Result{Status: doctor.Unverified, Name: "worktree", Detail: "needs git"})
	}

	if err := report.Write(cmd.OutOrStdout()); err != nil {
		return err
	}
	if n := report.Count(doctor.Fail); n > 0 {
		noun := "check"
		if n > 1 {
			noun = "checks"
		}
		return fmt.Errorf("doctor: %d required %s failed", n, noun)
	}
	return nil
}
