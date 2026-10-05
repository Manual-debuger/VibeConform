package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Manual-debuger/VibeConform/internal/module"
)

// guardPlan is one project-owned file a module guards, as found on disk.
// No command writes it: each problem is a conflict only a person can fix
// (docs/specs/0040-ci-floor-environment-seam.md).
type guardPlan struct {
	// Path is slash-separated, relative to the repository root.
	Path string
	// Problems is what the module's check found; none means conformant.
	Problems []string
}

// planGuards checks every file a module guards and that exists. An absent
// file is not a finding, so a repository that never adds one reports
// exactly as before.
func planGuards(repoRoot string, p *repoPlan) error {
	for _, mod := range p.Modules {
		g, ok := mod.(module.ProjectFileGuard)
		if !ok {
			continue
		}
		for _, f := range g.GuardedFiles(p.Context) {
			content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(f.Path))) // #nosec G304 -- a fixed path a module names, under the operator-supplied repo root
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("%s: %w", f.Path, err)
			}
			p.Guards = append(p.Guards, guardPlan{Path: f.Path, Problems: f.Check(content)})
		}
	}
	return nil
}

// printGuard writes "path: ok", or one conflict line per problem. Every
// command reports a guarded file the same way: there is no action to
// preview, only a state to describe.
func printGuard(w io.Writer, g guardPlan) error {
	if len(g.Problems) == 0 {
		_, err := fmt.Fprintf(w, "%s: ok (project-owned)\n", g.Path)
		return err
	}
	for _, problem := range g.Problems {
		if _, err := fmt.Fprintf(w, "%s: conflict: %s (project-owned; fix it by hand)\n", g.Path, problem); err != nil {
			return err
		}
	}
	return nil
}
