package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// auditFlags are vibe audit's flags. Only --repo-root applies to the old
// audit; the others select or shape the contract audit (spec 0052,
// "Package and command").
type auditFlags struct {
	repoRoot string
	config   string
	static   bool
	json     bool
	explain  string
	// probeTimeout limits each entrypoint run; keepProbeCopy keeps the
	// probe copy (spec 0053, "Report and CLI").
	probeTimeout    time.Duration
	probeTimeoutSet bool
	keepProbeCopy   bool

	// seams are not flags: tests set them through newRootCmd.
	seams contractSeams
}

// contractSeams are the contract audit's test seams. The zero value is
// the real audit: the registry's rules, and no one watching the reports.
type contractSeams struct {
	// rules is the rule list the contract audit runs. Nil means the
	// registry; tests stand in for parts this vibe does not implement yet.
	rules []contract.Rule
	// seen, when set, sees every report the contract audit prints; tests
	// check every report with it.
	seen func(*contract.Report)
	// probeEnv is the environment the probe runs inherit; nil means this
	// process's. Tests narrow PATH with it.
	probeEnv []string
	// openRepo lists the audited repository's tracked files; nil means
	// contract.OpenRepo. Tests hand in the file list of a fixture they
	// built, so that a fixture needs no git.
	openRepo func(ctx context.Context, root string) (*contract.Repo, error)
}

// dispatchAudit picks the audit from the configuration it reads: a
// top-level contract key runs the contract audit, a top-level standard key
// runs the old audit exactly as before.
func dispatchAudit(cmd *cobra.Command, f auditFlags) error {
	if cmd.Flags().Changed("explain") {
		return runExplain(cmd, f)
	}

	path, label := f.config, f.config
	if path == "" {
		path, label = filepath.Join(f.repoRoot, manifestFileName), manifestFileName
	}
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CLI flag, same trust boundary as buildPlan's read
	if err != nil {
		if f.config == "" && !f.static && !f.json {
			// The old audit reports a missing vibe.yaml as it always has.
			return runAudit(cmd, f.repoRoot)
		}
		return fmt.Errorf("cannot read the configuration: %w", err)
	}

	schema, err := contract.DetectSchema(label, data)
	if err != nil {
		return err
	}
	if schema == contract.SchemaV1 {
		if err := v1FlagError(f, label); err != nil {
			return err
		}
		return runAudit(cmd, f.repoRoot)
	}
	return runContractAudit(cmd, f, label, data)
}

// v1FlagError rejects the contract audit's flags on a version 1
// configuration, which only the old audit reads.
func v1FlagError(f auditFlags, label string) error {
	var used []string
	if f.config != "" {
		used = append(used, "--config")
	}
	if f.static {
		used = append(used, "--static")
	}
	if f.json {
		used = append(used, "--json")
	}
	if f.probeTimeoutSet {
		used = append(used, "--probe-timeout")
	}
	if f.keepProbeCopy {
		used = append(used, "--keep-probe-copy")
	}
	if len(used) == 0 {
		return nil
	}
	return fmt.Errorf("%s needs a version 2 configuration (a top-level contract key); "+
		"%s is version 1 (a top-level standard key)", strings.Join(used, ", "), label)
}

// runContractAudit runs the contract audit on a version 2 configuration,
// prints the report and returns the error that sets the exit code. An
// invalid configuration prints no report.
func runContractAudit(cmd *cobra.Command, f auditFlags, label string, data []byte) error {
	if f.probeTimeout <= 0 {
		return fmt.Errorf("--probe-timeout must be a positive duration, such as 30m; got %s", f.probeTimeout)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	// An interrupt ends the runs in the probe copy, and the copy is still
	// deleted.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	openRepo := contract.OpenRepo
	if f.seams.openRepo != nil {
		openRepo = f.seams.openRepo
	}
	repo, err := openRepo(ctx, f.repoRoot)
	if err != nil {
		return err
	}
	cfg, err := contract.Parse(label, data, repo)
	if err != nil {
		return err
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
	rep := contract.Audit(ctx, cfg, repo, contract.Options{Static: f.static, Rules: f.seams.rules, Probe: probe})
	if f.seams.seen != nil {
		f.seams.seen(rep)
	}
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
// configuration, so any other audit flag is an error.
func runExplain(cmd *cobra.Command, f auditFlags) error {
	var others []string
	for _, name := range []string{"repo-root", "config", "static", "json", "probe-timeout", "keep-probe-copy"} {
		if cmd.Flags().Changed(name) {
			others = append(others, "--"+name)
		}
	}
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
