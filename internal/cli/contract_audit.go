package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
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

// explainFlags are the flags --explain reads besides itself: the ones that
// find the configuration.
var explainFlags = []string{"explain", "repo-root", "config"}

// runExplain explains one rule as it is under the contract the
// configuration declares, or under the latest contract when the
// repository root has no vibe.yaml and --config names none (spec 0074).
// It reads only the configuration's contract field and no repository, so
// any flag but those of explainFlags is an error. It visits the set flags,
// in cobra's order (sorted by name), and keeps no list of the refused
// ones, so that a flag added later is refused too.
func runExplain(cmd *cobra.Command, f auditFlags) error {
	var others []string
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		if !slices.Contains(explainFlags, flag.Name) {
			others = append(others, "--"+flag.Name)
		}
	})
	if len(others) > 0 {
		return fmt.Errorf("--explain reads only the contract version of the configuration; drop %s", strings.Join(others, ", "))
	}
	versions, source, err := explainContract(f)
	if err != nil {
		return err
	}
	rule, ok := contract.LookupRule(f.explain)
	if !ok {
		return fmt.Errorf("--explain: %q is not a rule of contract %s; rules: %s",
			f.explain, versions.Audited, strings.Join(contract.RuleIDs(), " "))
	}
	if err := writeExplain(cmd.OutOrStdout(), rule, versions.Audited, source); err != nil {
		return fmt.Errorf("--explain: %w", err)
	}
	return nil
}

// explainContract is the contract --explain explains a rule under, and
// where that version comes from, for the explanation's Contract line.
func explainContract(f auditFlags) (contract.Versions, string, error) {
	file, data, err := contract.ReadConfig(f.repoRoot, f.config)
	if errors.Is(err, contract.ErrNoConfig) {
		return contract.Versions{Declared: contract.Latest, Audited: contract.Latest},
			fmt.Sprintf("%s, the latest (no %s)", contract.Latest, configFileName), nil
	}
	if err != nil {
		return contract.Versions{}, "", configLoadError(err)
	}
	versions, err := contract.ParseContract(file, data)
	if err != nil {
		return contract.Versions{}, "", err
	}
	return versions, fmt.Sprintf("%s (%s declares %s)", versions.Audited, file, versions.Declared), nil
}
