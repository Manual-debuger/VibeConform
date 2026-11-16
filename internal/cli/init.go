package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

func newInitCmd(seams contractSeams) *cobra.Command {
	var (
		repoRoot string
		skill    bool
	)

	cmd := &cobra.Command{
		Use:   "init [--skill]",
		Short: "Write a vibe.yaml version 2, or the setup skill",
		Long: "vibe init writes a vibe.yaml version 2 at the repository root: contract " + contract.Implemented + ",\n" +
			"platforms linux and windows, agent claude, the components it finds in the\n" +
			"tracked files (" + contract.ManifestPhrase() + "),\n" +
			"and the four required entrypoints as comments to fill in. Until they\n" +
			"are declared, vibe audit fails V-04. It refuses to overwrite an existing\n" +
			"vibe.yaml, version 1 included.\n\n" +
			"vibe init --skill writes the setup skill, " + setupskill.Name + ", to " +
			strings.Join(setupskill.Dirs, " and ") + ",\n" +
			"replacing its files and removing files it no longer has. It writes nothing else.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if skill {
				return runInitSkill(cmd, repoRoot)
			}
			return runInitConfig(cmd, repoRoot, seams)
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository root to initialize")
	cmd.Flags().BoolVar(&skill, "skill", false, "write the setup skill ("+setupskill.Name+") instead of vibe.yaml")

	return cmd
}

// runInitSkill writes the setup skill into the repository (spec 0054,
// "vibe init --skill"). It reads no vibe.yaml, so it works with a
// version 1 configuration, a version 2 one or none.
func runInitSkill(cmd *cobra.Command, repoRoot string) error {
	res, err := setupskill.Install(repoRoot, runningVersion(cmd))
	if err != nil {
		return fmt.Errorf("init --skill: %w", err)
	}
	w := cmd.OutOrStdout()
	for _, p := range res.Removed {
		if _, err := fmt.Fprintf(w, "removed %s\n", p); err != nil {
			return fmt.Errorf("init --skill: %w", err)
		}
	}
	for _, p := range res.Written {
		if _, err := fmt.Fprintf(w, "wrote %s\n", p); err != nil {
			return fmt.Errorf("init --skill: %w", err)
		}
	}
	return nil
}

// runInitConfig writes a vibe.yaml version 2 (spec 0059, Desired
// Behavior 10). It checks what it writes with the audit's own parser, so
// it never writes a file vibe audit cannot read. The contract detects the
// components (spec 0066); init only renders them.
func runInitConfig(cmd *cobra.Command, repoRoot string, seams contractSeams) error {
	if err := refuseExisting(repoRoot); err != nil {
		return err
	}
	repo, err := seams.openRepo.Open(commandContext(cmd), repoRoot)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	detection := contract.DetectComponents(repo.Files())
	if len(detection.Components) == 0 {
		return fmt.Errorf("init: found no component in the tracked files: no %s within %d directories of the root; "+
			"commit the project's manifest and run vibe init again", contract.ManifestPhrase(), contract.DetectionDepth)
	}

	// Without a components key when the audit detects the same components.
	declared := detection.Components
	if detection.AuditDetectsSame {
		declared = nil
	}
	data := renderConfig(declared)
	if _, err := contract.Parse(configFileName, data, repo); err != nil {
		return fmt.Errorf("init: the vibe.yaml it would write does not parse, which is a bug in vibe: %w", err)
	}
	if err := createConfig(repoRoot, data); err != nil {
		return err
	}
	if err := printDetection(cmd.OutOrStdout(), detection); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return nil
}

// createConfig writes data as repoRoot's vibe.yaml. The create is
// exclusive: a vibe.yaml that appeared since the check is not
// overwritten.
func createConfig(repoRoot string, data []byte) error {
	file := filepath.Join(repoRoot, configFileName)
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302 G304 -- vibe.yaml at the operator's repository root
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return createRefused(repoRoot)
		}
		return fmt.Errorf("init: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("init: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return nil
}

// printDetection prints what init wrote and found: the file, each
// component, each nested manifest and the next step.
func printDetection(w io.Writer, detection contract.Detection) error {
	lines := []string{"wrote " + configFileName}
	for _, c := range detection.Components {
		lines = append(lines, fmt.Sprintf("component %s (%s)", c.Path, c.Language))
	}
	for _, n := range detection.Nested {
		lines = append(lines, fmt.Sprintf("skipped %s: inside the component %s (%s); components do not nest",
			n.Dir, n.In.Path, n.In.Language))
	}
	lines = append(lines, "next: declare the entrypoints in "+configFileName+"; until then vibe audit fails V-04")
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

// refuseExisting returns an error when repoRoot has a vibe.yaml: init
// never replaces a configuration, and says which kind it found. It reads
// the file with the audit's own read-and-detect step.
func refuseExisting(repoRoot string) error {
	_, _, err := contract.ReadConfig(repoRoot, "")
	var readErr *fs.PathError
	switch {
	case errors.Is(err, contract.ErrNoConfig):
		return nil
	case errors.Is(err, contract.ErrConfigV1):
		return fmt.Errorf("init: %s exists and is version 1 (a top-level standard key); vibe init does not overwrite it. %s",
			configFileName, migrationPointer())
	case errors.As(err, &readErr):
		return fmt.Errorf("init: %s exists and cannot be read (%w); vibe init does not overwrite it", configFileName, readErr)
	}
	return fmt.Errorf("init: %s exists; vibe init does not overwrite it. Edit it, or remove it and run vibe init again", configFileName)
}

// createRefused is the error when the exclusive create of repoRoot's
// vibe.yaml found one. It is never nil: a file that is gone again by the
// time init reads it still means init wrote nothing.
func createRefused(repoRoot string) error {
	if err := refuseExisting(repoRoot); err != nil {
		return err
	}
	return fmt.Errorf("init: %s appeared while vibe init ran and is gone again; nothing was written. Run vibe init again", configFileName)
}

// renderConfig writes the vibe.yaml vibe init writes, declaring
// components. With none it writes no components key, so the audit
// detects the root component.
func renderConfig(components []contract.Component) []byte {
	var b strings.Builder
	b.WriteString("# vibe.yaml version 2, written by vibe init. vibe audit reads it and\n")
	b.WriteString("# checks the repository against the VibeConform contract.\n")
	fmt.Fprintf(&b, "contract: %s\n", contract.Implemented)
	b.WriteString("platforms: [linux, windows]\n")
	b.WriteString("agents: [claude]\n")
	if len(components) > 0 {
		b.WriteString("components:\n")
		for _, c := range components {
			fmt.Fprintf(&b, "  - path: %q\n    language: %s\n", c.Path, c.Language)
		}
	}
	b.WriteString("# Declare the four entrypoints: one command each, or a list of commands.\n")
	b.WriteString("# Until they are declared, vibe audit fails V-04. The setup skill\n")
	b.WriteString("# (vibe init --skill) can fill them in. For example:\n")
	b.WriteString("# entrypoints:\n")
	b.WriteString("#   verify: task verify\n")
	b.WriteString("#   verify-fast: task verify:fast\n")
	b.WriteString("#   fmt: task fmt\n")
	b.WriteString("#   fmt-check: task fmt:check\n")
	return []byte(b.String())
}
