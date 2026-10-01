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
		return fmt.Sprintf("would forget (%s; already removed)", pp.cause())
	case reconcile.Remove:
		return fmt.Sprintf("would remove (%s)", pp.cause())
	case reconcile.RemoveConflict:
		return fmt.Sprintf("conflict: %s but file modified since sync; kept", pp.cause())
	default:
		return pp.Decision.String()
	}
}

func auditPruneLine(pp prunePlan) string {
	switch pp.Decision {
	case reconcile.Forget:
		return fmt.Sprintf("out of date (%s, already removed; run vibe sync to forget it)", pp.cause())
	case reconcile.Remove:
		return fmt.Sprintf("out of date (%s; run vibe sync to remove)", pp.cause())
	case reconcile.RemoveConflict:
		return fmt.Sprintf("conflict: %s but file modified since sync", pp.cause())
	default:
		return pp.Decision.String()
	}
}

// applyPrune carries out one removal and returns the lines describing it.
func applyPrune(repoRoot string, pp prunePlan, next *state.State, counts *syncCounts) ([]string, error) {
	if pp.Patch != nil {
		return applyPatchPrune(repoRoot, pp, next, counts)
	}
	switch pp.Decision {
	case reconcile.Forget:
		delete(next.Resources, pp.Path)
		counts.removed++
		return []string{fmt.Sprintf("forgotten (%s; already removed)", pp.cause())}, nil
	case reconcile.Remove:
		if err := removeResource(repoRoot, pp.Path); err != nil {
			return nil, err
		}
		delete(next.Resources, pp.Path)
		counts.removed++
		return []string{fmt.Sprintf("removed (%s)", pp.cause())}, nil
	case reconcile.RemoveConflict:
		counts.conflicts++
		if pp.Retired != "" {
			return []string{fmt.Sprintf("conflict: %s but file modified since sync; kept (delete it by hand)", pp.Retired)}, nil
		}
		return []string{fmt.Sprintf("conflict: %s deselected but file modified since sync; kept (delete it by hand, or select %s again)",
			pp.Option, pp.Option)}, nil
	default:
		return nil, fmt.Errorf("unknown removal %v", pp.Decision)
	}
}

// applyPatchPrune removes a deselected option's elements from a
// file it shares, and the file itself only if VibeConform created it and
// nothing else is left in it.
func applyPatchPrune(repoRoot string, pp prunePlan, next *state.State, counts *syncCounts) ([]string, error) {
	lines := deselected(elementLines(pp.Patch, syncElement), pp.Option)
	if pp.Decision == reconcile.RemoveConflict {
		counts.conflicts++
		return lines, nil
	}
	if err := applyPatch(repoRoot, pp.Resource, pp.Patch); err != nil {
		return nil, err
	}
	delete(next.Resources, pp.Path)
	counts.removed++
	if pp.Patch.Delete {
		lines = append(lines, fmt.Sprintf("removed (%s deselected; nothing else was in it)", pp.Option))
	}
	return lines, nil
}

// deselected suffixes each line with the option that caused it.
func deselected(lines []string, option string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = fmt.Sprintf("%s (%s deselected)", l, option)
	}
	return out
}

// patchPruneLines are diff's and audit's lines for a structured-patch
// prune.
func patchPruneLines(pp prunePlan, verb func(elementPlan) string) []string {
	lines := deselected(elementLines(pp.Patch, verb), pp.Option)
	if pp.Patch.Delete {
		lines = append(lines, fmt.Sprintf("would remove the file (%s deselected; nothing else is in it)", pp.Option))
	}
	return lines
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
