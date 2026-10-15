package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// seedAction is what reconciliation does with a seeded file: one the
// repository owns, which VibeConform writes only when it is missing and
// otherwise checks for the name of its base
// (docs/decisions/0024-floor-and-content-seams.md).
type seedAction int

const (
	// seedOK: the file exists and names its base.
	seedOK seedAction = iota
	// seedCreate: the file is missing; sync writes the seed.
	seedCreate
	// seedConvert: VibeConform used to own the whole file and it is
	// unchanged since; sync replaces it with the seed (spec 0049 D9).
	seedConvert
	// seedAdopt: VibeConform used to own the file, and the repository has
	// since rewritten it to name its base; sync only forgets the record.
	seedAdopt
	// seedEdited: VibeConform used to own the file, the repository edited
	// it, and it does not name its base (spec 0049 D10).
	seedEdited
	// seedNoBase: the repository's file does not name its base (D5).
	seedNoBase
)

// seedPlan is the plan of a seeded resource.
type seedPlan struct {
	Action seedAction
	// Extended is true when an existing file differs from the seed.
	Extended bool
	// Base is the file the seeded file must name.
	Base string
}

// planSeed decides what happens to a seeded resource. State only ever
// holds a record for its path from a version that owned the whole file.
func planSeed(repoRoot string, previous *state.State, r resource.Resource) (resourcePlan, error) {
	if r.Base == "" {
		return resourcePlan{}, fmt.Errorf("%s: a seeded resource needs a Base", r.Path)
	}
	sp := &seedPlan{Base: r.Base}
	rp := resourcePlan{Resource: r, Supported: true, Seed: sp}

	recorded, wasManaged := previous.Resources[stateKey(r.Path)]
	content, err := os.ReadFile(resourcePath(repoRoot, r.Path)) // #nosec G304 -- repoRoot is an operator-supplied CLI flag; r.Path is a registered module's resource path
	switch {
	case errors.Is(err, os.ErrNotExist):
		sp.Action = seedCreate
		return rp, nil
	case err != nil:
		return resourcePlan{}, err
	}

	namesBase := bytes.Contains(content, []byte(r.Base))
	sp.Extended = !bytes.Equal(content, r.Content)
	switch {
	case wasManaged && hashHex(content) == recorded.SHA256:
		sp.Action = seedConvert
	case wasManaged && namesBase:
		sp.Action = seedAdopt
	case wasManaged:
		sp.Action = seedEdited
	case !namesBase:
		sp.Action = seedNoBase
	default:
		sp.Action = seedOK
	}
	return rp, nil
}

// basePath is the path of the base config next to a seeded resource.
func basePath(r resource.Resource) string {
	return path.Join(path.Dir(stateKey(r.Path)), r.Base)
}

// holdBases keeps the base config from being written next to a file it
// replaces whose edits are still to be resolved: the edited file is still
// a standalone config, and would never read it (spec 0049 D10).
func holdBases(p *repoPlan) {
	for _, rp := range p.Resources {
		if rp.Seed == nil || rp.Seed.Action != seedEdited {
			continue
		}
		base := basePath(rp.Resource)
		for i := range p.Resources {
			if stateKey(p.Resources[i].Resource.Path) == base {
				p.Resources[i].HeldBy = stateKey(rp.Resource.Path)
			}
		}
	}
}

// heldLine is what sync and diff say about a held base config.
func heldLine(seeded string) string {
	return fmt.Sprintf("held: not written until %s is resolved", seeded)
}

// seedConflictLine is the conflict every command reports for sp, or "".
func seedConflictLine(sp *seedPlan) string {
	switch sp.Action {
	case seedEdited:
		return fmt.Sprintf("conflict: edited since vibe last wrote it; it is now yours and must name %s. "+
			"Move your changes after the base (docs/usage.md, \"Base configs\"), or delete the file and run vibe sync", sp.Base)
	case seedNoBase:
		return fmt.Sprintf("conflict: does not name %s, so the standard's rules no longer apply "+
			"(project-owned; fix it by hand, or delete it and run vibe sync)", sp.Base)
	default:
		return ""
	}
}

// seedStateLine describes an existing, conformant seeded file.
func seedStateLine(sp *seedPlan) string {
	if sp.Extended {
		return "extended (project-owned; differs from the seed)"
	}
	return "ok (project-owned)"
}

// auditSeedLine describes a seeded file's present state.
func auditSeedLine(sp *seedPlan) string {
	switch sp.Action {
	case seedCreate:
		return "missing (run vibe sync to create it)"
	case seedConvert:
		return fmt.Sprintf("out of date (now project-owned: run vibe sync to replace it with a file extending %s)", sp.Base)
	case seedAdopt:
		return fmt.Sprintf("out of date (now project-owned and extends %s: run vibe sync to stop tracking it)", sp.Base)
	case seedEdited, seedNoBase:
		return seedConflictLine(sp)
	default:
		return seedStateLine(sp)
	}
}

// diffSeedLine describes what sync would do to a seeded file.
func diffSeedLine(sp *seedPlan) string {
	switch sp.Action {
	case seedCreate:
		return "create (no file on disk; project-owned once written)"
	case seedConvert:
		return fmt.Sprintf("would replace with a file extending %s (unchanged since vibe wrote it; project-owned from now on)", sp.Base)
	case seedAdopt:
		return fmt.Sprintf("would stop tracking (extends %s; project-owned)", sp.Base)
	case seedEdited, seedNoBase:
		return seedConflictLine(sp)
	default:
		return seedStateLine(sp)
	}
}

// applySeed carries out rp's seed plan. A seeded file is never recorded
// in state, so every outcome that touches it drops a record left by a
// version that owned the whole file.
func applySeed(repoRoot string, rp resourcePlan, next *state.State, counts *syncCounts) (string, error) {
	sp := rp.Seed
	key := stateKey(rp.Resource.Path)
	switch sp.Action {
	case seedCreate:
		if err := writeResource(repoRoot, rp.Resource); err != nil {
			return "", err
		}
		delete(next.Resources, key)
		counts.created++
		return "created (project-owned from now on)", nil
	case seedConvert:
		if err := writeResource(repoRoot, rp.Resource); err != nil {
			return "", err
		}
		delete(next.Resources, key)
		counts.updated++
		return fmt.Sprintf("converted: now extends %s (project-owned)", sp.Base), nil
	case seedAdopt:
		delete(next.Resources, key)
		counts.updated++
		return fmt.Sprintf("no longer tracked: extends %s (project-owned)", sp.Base), nil
	case seedEdited, seedNoBase:
		counts.conflicts++
		return seedConflictLine(sp), nil
	default:
		counts.unchanged++
		if sp.Extended {
			return seedStateLine(sp), nil
		}
		return "unchanged (project-owned)", nil
	}
}
