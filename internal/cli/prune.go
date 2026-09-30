package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// ignoredLine is every command's report for a managed path Git ignores.
func ignoredLine(p string) string {
	return fmt.Sprintf("error: ignored by git, so it would never be committed; add !%s to .gitignore", p)
}

// printWarnings writes a plan's warnings to w. Write errors are dropped:
// a warning that could not be printed must not fail the command.
func printWarnings(w io.Writer, p *repoPlan) {
	for _, warning := range p.Warnings {
		_, _ = fmt.Fprintf(w, "warning: %s\n", warning)
	}
}

func diffPruneLine(pp prunePlan) string {
	switch pp.Decision {
	case reconcile.Forget:
		return fmt.Sprintf("would forget (%s deselected; already removed)", pp.Integration)
	case reconcile.Remove:
		return fmt.Sprintf("would remove (%s deselected)", pp.Integration)
	case reconcile.RemoveConflict:
		return fmt.Sprintf("conflict: %s deselected but file modified since sync; kept", pp.Integration)
	default:
		return pp.Decision.String()
	}
}

func auditPruneLine(pp prunePlan) string {
	switch pp.Decision {
	case reconcile.Forget:
		return fmt.Sprintf("out of date (%s deselected, already removed; run vibe sync to forget it)", pp.Integration)
	case reconcile.Remove:
		return fmt.Sprintf("out of date (%s deselected; run vibe sync to remove)", pp.Integration)
	case reconcile.RemoveConflict:
		return fmt.Sprintf("conflict: %s deselected but file modified since sync", pp.Integration)
	default:
		return pp.Decision.String()
	}
}

// applyPrune carries out one removal and returns the line describing it.
func applyPrune(repoRoot string, pp prunePlan, next *state.State, counts *syncCounts) (string, error) {
	switch pp.Decision {
	case reconcile.Forget:
		delete(next.Resources, pp.Path)
		counts.removed++
		return fmt.Sprintf("forgotten (%s deselected; already removed)", pp.Integration), nil
	case reconcile.Remove:
		if err := removeResource(repoRoot, pp.Path); err != nil {
			return "", err
		}
		delete(next.Resources, pp.Path)
		counts.removed++
		return fmt.Sprintf("removed (%s deselected)", pp.Integration), nil
	case reconcile.RemoveConflict:
		counts.conflicts++
		return fmt.Sprintf("conflict: %s deselected but file modified since sync; kept (delete it by hand, or select %s again)",
			pp.Integration, pp.Integration), nil
	default:
		return "", fmt.Errorf("unknown removal %v", pp.Decision)
	}
}

// removeResource deletes one managed file, then every parent directory
// the deletion left empty, up to but never including repoRoot. A
// directory with anything else in it stays.
func removeResource(repoRoot, key string) error {
	if err := os.Remove(resourcePath(repoRoot, key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for dir := path.Dir(key); dir != "." && dir != "/"; dir = path.Dir(dir) {
		entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(dir)))
		if err != nil || len(entries) > 0 {
			return nil //nolint:nilerr // a directory we cannot read is one we leave alone
		}
		if err := os.Remove(filepath.Join(repoRoot, filepath.FromSlash(dir))); err != nil {
			return nil //nolint:nilerr // likewise: the file is gone, which is what was asked
		}
	}
	return nil
}
