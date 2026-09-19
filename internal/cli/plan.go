package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/standard"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// resourcePlan is one resolved resource together with what reconciliation
// would do to it.
type resourcePlan struct {
	// Resource is the desired state a module resolved.
	Resource resource.Resource
	// Decision is meaningful only when Supported is true.
	Decision reconcile.Decision
	// TargetHash is the content hash of Resource.Content, recorded in
	// .vibe/state.yaml once the resource is applied.
	TargetHash string
	// Supported is false for ownership modes no command handles yet.
	Supported bool
}

// repoPlan is everything a command needs to report or apply reconciliation
// for one repository.
type repoPlan struct {
	// Standard is the resolved standard vibe.yaml declares.
	Standard *standard.Standard
	// Previous is the state VibeConform last recorded for this repository.
	Previous *state.State
	// Resources is one entry per resource the standard's modules resolve,
	// in module then resolution order.
	Resources []resourcePlan
}

// buildPlan reads repoRoot's manifest, resolves the standard it declares,
// and decides what would happen to every resource that standard composes.
//
// It holds all of the command-side I/O — manifest, recorded state, and
// current file contents — so that diff and sync cannot drift apart: diff is
// exactly the preview of what sync applies. Callers wrap the returned error
// with their own command name.
func buildPlan(repoRoot string) (*repoPlan, error) {
	path := filepath.Join(repoRoot, manifestFileName)
	data, err := os.ReadFile(path) // #nosec G304 -- repoRoot is an operator-supplied CLI flag, same trust boundary as init.go's WriteFile target
	if err != nil {
		return nil, err
	}

	m, err := manifest.Parse(data)
	if err != nil {
		return nil, err
	}

	s, err := standard.Lookup(m.Standard, m.Version)
	if err != nil {
		return nil, err
	}

	previous, err := state.Load(repoRoot)
	if err != nil {
		return nil, err
	}

	p := &repoPlan{Standard: s, Previous: previous}
	mctx := &module.Context{RepoRoot: repoRoot}
	for _, mod := range s.Modules {
		resources, err := mod.Resolve(context.Background(), mctx)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", mod.Name(), err)
		}
		for _, r := range resources {
			rp, err := planResource(repoRoot, previous, r)
			if err != nil {
				return nil, err
			}
			p.Resources = append(p.Resources, rp)
		}
	}

	return p, nil
}

// planResource decides the outcome for a single resource by comparing the
// hash recorded in state, the file on disk, and the resolved content.
func planResource(repoRoot string, previous *state.State, r resource.Resource) (resourcePlan, error) {
	if r.Ownership != resource.Generated {
		return resourcePlan{Resource: r}, nil
	}

	target := hashHex(r.Content)

	var current *string
	currentData, err := os.ReadFile(resourcePath(repoRoot, r.Path)) // #nosec G304 -- repoRoot/r.Path come from an operator-supplied CLI flag and a registered module's fixed resource path
	switch {
	case errors.Is(err, os.ErrNotExist):
		current = nil
	case err != nil:
		return resourcePlan{}, err
	default:
		h := hashHex(currentData)
		current = &h
	}

	var recorded *string
	if rs, ok := previous.Resources[stateKey(r.Path)]; ok {
		recorded = &rs.SHA256
	}

	return resourcePlan{
		Resource:   r,
		Decision:   reconcile.Decide(recorded, current, target),
		TargetHash: target,
		Supported:  true,
	}, nil
}

// resourcePath turns a repository-relative resource path into a path for
// this platform's filesystem.
func resourcePath(repoRoot, path string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(path))
}

// stateKey normalizes a resource path into its .vibe/state.yaml key.
// Keys are always slash-separated so a Windows run and a Unix run record
// the same state for the same repository.
func stateKey(path string) string {
	return filepath.ToSlash(path)
}

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
