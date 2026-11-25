package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// auditFlags are vibe audit's flags (spec 0052, "Package and command").
type auditFlags struct {
	repoRoot string
	config   string
	static   bool
	json     bool
	explain  string
	// probeTimeout limits each entrypoint run; keepProbeCopy keeps the
	// probe copy (spec 0053, "Report and CLI").
	probeTimeout  time.Duration
	keepProbeCopy bool

	// seams are not flags: tests set them through newRootCmd.
	seams cliSeams
}

// commandContext is cmd's context, or the background one when cmd runs
// without one.
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// configFileName is the configuration vibe audit reads at the repository
// root unless --config names another file.
const configFileName = contract.ConfigFile

// dispatchAudit loads the configuration and runs the contract audit.
func dispatchAudit(cmd *cobra.Command, f auditFlags) error {
	if cmd.Flags().Changed("explain") {
		return runExplain(cmd, f)
	}
	return runContractAudit(cmd, f)
}

// configLoadError adds what the user should do to contract.Load's two
// sentinel errors: write a vibe.yaml, or leave the template-sync system.
// vibe audit and vibe doctor share it, so they say the same.
func configLoadError(err error) error {
	switch {
	case errors.Is(err, contract.ErrNoConfig):
		return fmt.Errorf("%w: run `vibe init` to write one", err)
	case errors.Is(err, contract.ErrConfigV1):
		return fmt.Errorf("%w; %s", err, migrationPointer())
	}
	return err
}

// runContractAudit loads the configuration, runs the contract audit,
// prints the report and returns the error that sets the exit code. A
// configuration it cannot load prints no report.
func runContractAudit(cmd *cobra.Command, f auditFlags) error {
	if f.probeTimeout <= 0 {
		// A configuration that is missing or not version 2 is reported
		// before the flag, and one that does not parse after it, as
		// before contract.Load existed.
		if _, _, err := contract.ReadConfig(f.repoRoot, f.config); err != nil {
			return configLoadError(err)
		}
		return fmt.Errorf("--probe-timeout must be a positive duration, such as 30m; got %s", f.probeTimeout)
	}
	// An interrupt ends the runs in the probe copy, and the copy is still
	// deleted.
	ctx, stop := signal.NotifyContext(commandContext(cmd), os.Interrupt)
	defer stop()
	cfg, repo, err := contract.Load(ctx, f.repoRoot, f.config, f.seams.openRepo)
	if err != nil {
		return configLoadError(err)
	}

	probe := contract.ProbeOptions{
		Timeout:  f.probeTimeout,
		KeepCopy: f.keepProbeCopy,
		Stderr:   cmd.ErrOrStderr(),
		Env:      f.seams.probeEnv,
	}
	if !f.json {
		probe.Progress = cmd.ErrOrStderr()
	}
	rep := contract.Audit(ctx, cfg, repo, contract.Options{Static: f.static, Probe: probe})
	write := writeContractText
	if f.json {
		write = writeContractJSON
	}
	if err := write(cmd.OutOrStdout(), rep, runningVersion(cmd)); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return verdictError(rep)
}

// runExplain explains one rule. It reads no repository and no
// configuration, so any other flag the user set is an error. It visits
// the set flags, in cobra's order (sorted by name), and keeps no list of
// them, so that a flag added later is refused too.
func runExplain(cmd *cobra.Command, f auditFlags) error {
	var others []string
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		if flag.Name != "explain" {
			others = append(others, "--"+flag.Name)
		}
	})
	if len(others) > 0 {
		return fmt.Errorf("--explain reads no repository and no configuration; drop %s", strings.Join(others, ", "))
	}
	rule, ok := contract.LookupRule(f.explain)
	if !ok {
		return fmt.Errorf("--explain: %q is not a rule of contract %s; rules: %s",
			f.explain, contract.Implemented, strings.Join(contract.RuleIDs(), " "))
	}
	if err := writeExplain(cmd.OutOrStdout(), rule); err != nil {
		return fmt.Errorf("--explain: %w", err)
	}
	return nil
}
