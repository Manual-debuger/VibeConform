package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

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

// configFileName is the configuration vibe audit reads at the repository
// root unless --config names another file.
const configFileName = "vibe.yaml"

// dispatchAudit reads the configuration and runs the contract audit.
func dispatchAudit(cmd *cobra.Command, f auditFlags) error {
	if cmd.Flags().Changed("explain") {
		return runExplain(cmd, f)
	}
	label, data, err := readV2Config(f.repoRoot, f.config)
	if err != nil {
		return err
	}
	return runContractAudit(cmd, f, label, data)
}

// readV2Config reads the configuration at config, or vibe.yaml at the
// repository root when config is empty, and returns its label and
// content when it is version 2. A version 1 file (a top-level standard
// key) belongs to the template-sync system, which vibe no longer has: its
// error is the migration pointer. A missing vibe.yaml says to run vibe
// init.
func readV2Config(repoRoot, config string) (label string, data []byte, err error) {
	path, label := config, config
	if path == "" {
		path, label = filepath.Join(repoRoot, configFileName), configFileName
	}
	data, err = os.ReadFile(path) // #nosec G304 -- operator-supplied CLI flag
	if err != nil {
		if config == "" && errors.Is(err, fs.ErrNotExist) {
			return "", nil, fmt.Errorf("no %s in %s: run `vibe init` to write one", configFileName, repoRoot)
		}
		return "", nil, fmt.Errorf("cannot read the configuration: %w", err)
	}
	schema, err := contract.DetectSchema(label, data)
	if err != nil {
		return "", nil, err
	}
	if schema == contract.SchemaV1 {
		return "", nil, fmt.Errorf("%s is version 1 (a top-level standard key), which this vibe no longer reads; %s",
			label, migrationPointer())
	}
	return label, data, nil
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
