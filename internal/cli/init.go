package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
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
			"tracked files (go.mod, pyproject.toml, package.json with tsconfig.json), and\n" +
			"the four required entrypoints as comments to fill in. Until they are\n" +
			"declared, vibe audit fails V-04. It refuses to overwrite an existing\n" +
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
// it never writes a file vibe audit cannot read.
func runInitConfig(cmd *cobra.Command, repoRoot string, seams contractSeams) error {
	file := filepath.Join(repoRoot, configFileName)
	if err := refuseExisting(file); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	openRepo := contract.OpenRepo
	if seams.openRepo != nil {
		openRepo = seams.openRepo
	}
	repo, err := openRepo(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	components := detectComponents(repo.Files())
	if len(components) == 0 {
		return fmt.Errorf("init: found no component in the tracked files: no go.mod, pyproject.toml, "+
			"or package.json beside a tsconfig.json within %d directories of the root; "+
			"commit the project's manifest and run vibe init again", initDepth)
	}

	data := renderConfig(components, false)
	cfg, err := contract.Parse(configFileName, data, repo)
	if err != nil || !slices.Equal(cfg.Components, components) {
		// The audit detects the root's language only from a single root
		// manifest, so a root component it would read otherwise is
		// declared.
		data = renderConfig(components, true)
		if _, err := contract.Parse(configFileName, data, repo); err != nil {
			return fmt.Errorf("init: the vibe.yaml it would write does not parse, which is a bug in vibe: %w", err)
		}
	}

	// O_EXCL: a vibe.yaml that appeared since the check is not overwritten.
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302 G304 -- vibe.yaml at the operator's repository root
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return refuseExisting(file)
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

	w := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(w, "wrote %s\n", configFileName); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	for _, c := range components {
		if _, err := fmt.Fprintf(w, "component %s (%s)\n", c.Path, c.Language); err != nil {
			return fmt.Errorf("init: %w", err)
		}
	}
	_, err = fmt.Fprintf(w, "next: declare the entrypoints in %s; until then vibe audit fails V-04\n", configFileName)
	return err
}

// refuseExisting returns an error when file exists: init never replaces a
// configuration, and says which kind it found.
func refuseExisting(file string) error {
	data, err := os.ReadFile(file) // #nosec G304 -- vibe.yaml at the operator's repository root
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("init: %s exists and cannot be read (%w); vibe init does not overwrite it", configFileName, err)
	}
	if schema, err := contract.DetectSchema(configFileName, data); err == nil && schema == contract.SchemaV1 {
		return fmt.Errorf("init: %s exists and is version 1 (a top-level standard key); vibe init does not overwrite it. %s",
			configFileName, migrationPointer())
	}
	return fmt.Errorf("init: %s exists; vibe init does not overwrite it. Edit it, or remove it and run vibe init again", configFileName)
}

// initDepth is how many directories below the root vibe init looks for a
// component's manifest.
const initDepth = 3

// skippedDirs are directory names whose manifests belong to dependencies,
// not to the repository's components.
var skippedDirs = []string{"node_modules", "vendor"}

// detectComponents finds the components among tracked files: each
// directory up to initDepth below the root with a go.mod, a
// pyproject.toml, or a package.json beside a tsconfig.json, in that order
// when a directory has more than one. Components do not nest, so a
// directory inside another component is not one. The result is sorted by
// path.
func detectComponents(files []string) []contract.Component {
	has := map[string]bool{}
	for _, f := range files {
		has[f] = true
	}
	var found []contract.Component
	for _, f := range files {
		dir, name := path.Split(f)
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" {
			dir = "."
		}
		if !componentDir(dir) {
			continue
		}
		var language string
		switch name {
		case "go.mod":
			language = contract.LanguageGo
		case "pyproject.toml":
			if !has[path.Join(dir, "go.mod")] {
				language = contract.LanguagePython
			}
		case "package.json":
			if has[path.Join(dir, "tsconfig.json")] && !has[path.Join(dir, "go.mod")] && !has[path.Join(dir, "pyproject.toml")] {
				language = contract.LanguageTypeScript
			}
		}
		if language != "" {
			found = append(found, contract.Component{Path: dir, Language: language})
		}
	}
	slices.SortFunc(found, func(a, b contract.Component) int { return strings.Compare(a.Path, b.Path) })

	var out []contract.Component
	for _, c := range found {
		if !slices.ContainsFunc(out, func(o contract.Component) bool { return insideDir(c.Path, o.Path) }) {
			out = append(out, c)
		}
	}
	return out
}

// componentDir reports whether dir may hold a component: within initDepth
// of the root and not under a dependency directory.
func componentDir(dir string) bool {
	if dir == "." {
		return true
	}
	parts := strings.Split(dir, "/")
	if len(parts) > initDepth {
		return false
	}
	for _, p := range parts {
		if slices.Contains(skippedDirs, p) {
			return false
		}
	}
	return true
}

// insideDir reports whether dir lies strictly inside parent.
func insideDir(dir, parent string) bool {
	if parent == "." {
		return dir != "."
	}
	return strings.HasPrefix(dir, parent+"/")
}

// renderConfig writes the vibe.yaml vibe init writes. A single component
// at the root goes without a components key unless declare is set.
func renderConfig(components []contract.Component, declare bool) []byte {
	var b strings.Builder
	b.WriteString("# vibe.yaml version 2, written by vibe init. vibe audit reads it and\n")
	b.WriteString("# checks the repository against the VibeConform contract.\n")
	fmt.Fprintf(&b, "contract: %s\n", contract.Implemented)
	b.WriteString("platforms: [linux, windows]\n")
	b.WriteString("agents: [claude]\n")
	if declare || len(components) != 1 || components[0].Path != "." {
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
