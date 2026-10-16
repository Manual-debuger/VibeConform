package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	if len(used) == 0 {
		return nil
	}
	return fmt.Errorf("%s needs a version 2 configuration (a top-level contract key); "+
		"%s is version 1 (a top-level standard key)", strings.Join(used, ", "), label)
}

// runContractAudit runs the contract audit on a version 2 configuration.
func runContractAudit(cmd *cobra.Command, f auditFlags, label string, data []byte) error {
	repo, err := contract.OpenRepo(cmd.Context(), f.repoRoot)
	if err != nil {
		return err
	}
	if _, err := contract.Parse(label, data, repo); err != nil {
		return err
	}
	return errors.New("cannot decide: rule engine not implemented (#99)")
}

// runExplain explains one rule. It reads no repository and no
// configuration, so any other audit flag is an error.
func runExplain(cmd *cobra.Command, f auditFlags) error {
	var others []string
	for _, name := range []string{"repo-root", "config", "static", "json"} {
		if cmd.Flags().Changed(name) {
			others = append(others, "--"+name)
		}
	}
	if len(others) > 0 {
		return fmt.Errorf("--explain reads no repository and no configuration; drop %s", strings.Join(others, ", "))
	}
	if _, ok := contract.LookupRule(f.explain); !ok {
		return fmt.Errorf("--explain: %q is not a rule of contract %s; rules: %s",
			f.explain, contract.Implemented, strings.Join(contract.RuleIDs(), " "))
	}
	return fmt.Errorf("--explain %s: not implemented in this vibe (#99)", f.explain)
}
