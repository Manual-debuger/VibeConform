package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// doctorEnv is a test seam: the CLI suite must assert on this code, not on
// what the machine running it has installed.
var doctorEnv = doctor.System

func newDoctorCmd(seams contractSeams) *cobra.Command {
	var repoRoot string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose whether this machine can run the repository's workflow",
		Long: "vibe doctor reads vibe.yaml version 2 and .claude/settings.json and checks,\n" +
			"one line each: git; the program of each entrypoint command (on PATH, or in\n" +
			"the repository when named by a path); the bash that runs the hook commands\n" +
			"(Git Bash on Windows), found as vibe audit finds it; and vibe-guard on PATH\n" +
			"when a PreToolUse hook runs it. It prints PASS, WARN, FAIL or UNVERIFIED\n" +
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

// runDoctor runs every check in a fixed order and prints the report. Only
// a FAIL makes it return an error, which exits 1: the workflow cannot run
// here. A configuration the audit would not read stops the checks that
// need it, with the audit's own message.
func runDoctor(cmd *cobra.Command, repoRoot string, seams contractSeams) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	env := doctorEnv()
	var report doctor.Report

	report.Add(doctor.Git(ctx, env, repoRoot))
	if cfg, repo, err := readDoctorConfig(ctx, repoRoot, seams); err != nil {
		report.Add(doctor.Result{Status: doctor.Fail, Name: configFileName, Detail: err.Error()})
	} else {
		report.Add(doctor.Result{Status: doctor.Pass, Name: configFileName,
			Detail: "version 2, contract " + cfg.Contract.Declared})
		report.Add(doctor.Entrypoints(env, repoRoot, cfg.Entrypoints)...)
		report.Add(hookChecks(env, repo)...)
	}

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

// readDoctorConfig reads and parses vibe.yaml as vibe audit does, with
// the audit's errors.
func readDoctorConfig(ctx context.Context, repoRoot string, seams contractSeams) (*contract.Config, *contract.Repo, error) {
	label, data, err := readV2Config(repoRoot, "")
	if err != nil {
		return nil, nil, err
	}
	openRepo := contract.OpenRepo
	if seams.openRepo != nil {
		openRepo = seams.openRepo
	}
	repo, err := openRepo(ctx, repoRoot)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := contract.Parse(label, data, repo)
	if err != nil {
		return nil, nil, err
	}
	return cfg, repo, nil
}

// hookChecks checks what the hooks of the tracked .claude/settings.json
// need: bash, and vibe-guard when a PreToolUse hook runs it. A file that
// is not tracked has no hooks; one that does not parse is A-01's to
// report.
func hookChecks(env doctor.Env, repo *contract.Repo) []doctor.Result {
	var hooks contract.ClaudeHooks
	if repo.HasFile(contract.ClaudeSettingsFile) {
		data, err := repo.ReadFile(contract.ClaudeSettingsFile)
		if err == nil {
			hooks, err = contract.ReadClaudeHooks(data)
		}
		if err != nil {
			return []doctor.Result{{Status: doctor.Unverified, Name: "hooks",
				Detail: contract.ClaudeSettingsFile + ": " + err.Error() + "; vibe audit reports it"}}
		}
	}
	return append([]doctor.Result{doctor.Bash(env, hooks)}, doctor.VibeGuard(env, hooks)...)
}
