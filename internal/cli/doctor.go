package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// doctorEnv is a test seam, like lookPath: the CLI suite must assert on
// this code, not on what the machine running it has installed.
var doctorEnv = doctor.System

func newDoctorCmd() *cobra.Command {
	var repoRoot string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose whether this machine can run the repository's workflow",
		Long: "Checks git, the standard's required tools and their versions, and whether\n" +
			"Taskfile.yml loads. Prints PASS, WARN, FAIL, or UNVERIFIED per check and\n" +
			"exits 1 if any required check fails. Read-only: installs nothing, writes\n" +
			"nothing, and does not check conformance (that is vibe audit).",
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
// here, so there is no usable verdict (docs/specs/0028-environment-doctor.md).
func runDoctor(cmd *cobra.Command, repoRoot string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	env := doctorEnv()
	var report doctor.Report

	report.Add(doctor.Git(ctx, env, repoRoot))

	p, err := buildPlan(repoRoot)
	if err != nil {
		report.Add(
			doctor.Result{Status: doctor.Fail, Name: "manifest", Detail: err.Error()},
			doctor.Result{Status: doctor.Unverified, Name: "tools", Detail: "needs a valid vibe.yaml"},
		)
	} else {
		integrations := "none"
		if len(p.Context.Integrations) > 0 {
			integrations = strings.Join(p.Context.Integrations, ", ")
		}
		report.Add(doctor.Result{
			Status: doctor.Pass,
			Name:   "manifest",
			Detail: fmt.Sprintf("vibe.yaml resolves %s/%s (integrations: %s)", p.Standard.Name, p.Standard.Version, integrations),
		})

		var tools []doctor.Tool
		for _, rt := range requiredTools(p.Standard, p.Context) {
			tools = append(tools, doctor.Tool{Name: rt.tool.Name, Module: rt.module, Why: rt.tool.Why, Version: rt.tool.Version})
		}
		report.Add(doctor.Tools(ctx, env, repoRoot, tools)...)
	}

	out := cmd.OutOrStdout()
	if p != nil {
		if _, err := fmt.Fprintf(out, "standard: %s/%s\n", p.Standard.Name, p.Standard.Version); err != nil {
			return err
		}
	}
	if err := report.Write(out); err != nil {
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
