// Package cli wires together the vibe command tree.
package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// NewRootCmd constructs the root "vibe" command with all subcommands attached.
func NewRootCmd(version string) *cobra.Command {
	return newRootCmd(version, cliSeams{})
}

// cliSeams are the CLI suite's test seams (ADR 0028), handed to every
// command by newRootCmd. The zero value is the real command:
// contract.OpenRepo, this process's environment for the probe runs,
// doctor.System and the wall clock. The rule tests run contract.Audit in
// internal/contract, so the CLI holds no rule list and no report hook.
type cliSeams struct {
	// probeEnv is the environment the probe runs inherit; nil means this
	// process's. The CLI suite's full audits (the golden reports among
	// them) narrow PATH with it, so that what they print does not depend
	// on the tools this machine has installed. The audit's own git calls
	// keep this process's PATH.
	probeEnv []string
	// openRepo lists the audited repository's tracked files; nil means
	// contract.OpenRepo. Tests hand in the file list of a fixture they
	// built, so that a fixture needs no git.
	openRepo contract.RepoOpener
	// doctorEnv is the machine vibe doctor checks; nil means
	// doctor.System. Tests stand in a machine, so that the CLI suite
	// asserts on this code, not on what the machine running it has
	// installed.
	doctorEnv func() doctor.Env
	// now is the clock vibe unlock reads; nil means the wall clock (spec
	// 0070, "vibe unlock's clock"). Tests fix it, so that they check the
	// exact expiry in the unlock marker.
	now func() time.Time
}

// clock is the seams' clock: now, or time.Now when it is nil.
func (s cliSeams) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// newRootCmd is NewRootCmd with the CLI's test seams.
func newRootCmd(version string, seams cliSeams) *cobra.Command {
	root := &cobra.Command{
		Use:   "vibe",
		Short: "VibeConform: audit a repository against the VibeConform contract",
		Long: "VibeConform (vibe) audits a repository against the VibeConform contract\n" +
			"(contract " + contract.Implemented + "). The repository declares its entrypoints, components,\n" +
			"platforms and agents in vibe.yaml version 2 and owns every file; vibe audit\n" +
			"reads them and reports, rule by rule, whether the repository conforms.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       version,
	}
	// vibe --version names the contract version vibe implements (spec
	// 0054); the setup skill's upgrade flow reads it.
	root.SetVersionTemplate("vibe version {{.Version}} (contract " + contract.Implemented + ")\n")

	root.AddCommand(
		newInitCmd(seams),
		newAuditCmd(seams),
		newDoctorCmd(seams),
		newUnlockCmd(seams),
	)
	for _, name := range []string{"sync", "diff", "check"} {
		root.AddCommand(newRemovedCmd(name))
	}

	return root
}

// runningVersion returns the version string of the binary executing cmd.
//
// It reads cobra's own root Version rather than threading a parameter
// through every command constructor: main already passes the ldflags-set
// version to NewRootCmd, and a second copy of it would be a second thing to
// keep in sync. The audit report and the setup skill name this version, and
// must agree with what "vibe --version" prints.
//
// It is "dev" for any build GoReleaser did not stamp, and whatever a test
// passed to NewRootCmd under test.
func runningVersion(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return cmd.Root().Version
}
