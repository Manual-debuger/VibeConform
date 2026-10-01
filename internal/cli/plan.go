package cli

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

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
	// Ignored is true when Git ignores this path and does not track it:
	// written, it would never be committed (spec 0026). An ignored
	// resource is an error in every command, whatever its Decision.
	Ignored bool
	// Patch is the element-level plan of a structured-patch resource.
	Patch *patchPlan
	// Section is the plan of a managed section, and SectionFile the plan
	// of the file it shares with the file's other sections.
	Section     *sectionPlan
	SectionFile *sectionFile
}

// prunePlan is one recorded path that the selection no longer produces
// and that a deselected integration would: a candidate for removal
// (docs/decisions/0013-optional-integrations.md).
type prunePlan struct {
	// Path is the state key, slash-separated.
	Path string
	// Option names the deselected option that produces it, as messages
	// show it: an integration's name, or a policy's key.
	Option string
	// Retired is set instead of Option for a path a module retired
	// (module.Retirer): the reason messages give, e.g. "replaced by …".
	Retired string
	// Decision is what sync would do about it.
	Decision reconcile.Removal
	// Resource and Patch are set for a structured-patch resource, which is
	// pruned element by element rather than deleted whole.
	Resource resource.Resource
	Patch    *patchPlan
	// Section and SectionFile are set for a managed section, which is
	// removed from its file rather than deleted with it.
	Section     *sectionPlan
	SectionFile *sectionFile
}

// repoPlan is everything a command needs to report or apply reconciliation
// for one repository.
type repoPlan struct {
	// Standard is the resolved standard vibe.yaml declares.
	Standard *standard.Standard
	// Context is what every module resolved against.
	Context *module.Context
	// Selection is what vibe.yaml selects from the standard's catalog.
	Selection standard.Selection
	// Modules are the core modules followed by the selected options'
	// modules, in the order they resolved.
	Modules []module.Module
	// Previous is the state VibeConform last recorded for this repository.
	Previous *state.State
	// Resources is one entry per resource the standard's modules resolve,
	// in module then resolution order.
	Resources []resourcePlan
	// Prunes are the recorded paths of deselected integrations, in catalog
	// then resolution order.
	Prunes []prunePlan
	// Warnings are findings that fail nothing, for stderr.
	Warnings []string
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
	if err := checkComponents(s, m); err != nil {
		return nil, err
	}
	if err := checkDocsDirs(repoRoot, m); err != nil {
		return nil, err
	}

	previous, err := state.Load(repoRoot)
	if err != nil {
		return nil, err
	}

	selected, err := s.Select(m)
	if err != nil {
		return nil, err
	}

	mctx := &module.Context{
		RepoRoot:     repoRoot,
		Components:   m.Components,
		Integrations: selected.Integrations,
		Policies:     selected.Policies,
		DocsDirs:     m.Development.DocsDirs(),
		Profiles:     profiles(s, m),
	}
	p := &repoPlan{Standard: s, Context: mctx, Selection: selected, Previous: previous, Modules: s.ModulesFor(selected)}
	for _, mod := range p.Modules {
		resources, err := mod.Resolve(context.Background(), mctx)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", mod.Name(), err)
		}
		for _, r := range resources {
			rp, err := planResource(repoRoot, previous, mod, r)
			if err != nil {
				return nil, err
			}
			p.Resources = append(p.Resources, rp)
		}
	}

	if err := planPrunes(repoRoot, p); err != nil {
		return nil, err
	}
	if err := planSections(repoRoot, p); err != nil {
		return nil, err
	}
	if err := markIgnored(repoRoot, p); err != nil {
		return nil, err
	}

	return p, nil
}

// planPrunes finds every path a deselected option would produce —
// through its own module, or through a core module it switches on, like
// the guard for claude — that the selection does not, and that state
// records. Each is resolved as if that one option were added to the
// selection. Requires and Excludes are not checked: the trial selection
// is never applied, only asked what it would write.
//
// Only recorded paths qualify, so a file VibeConform never wrote is never
// a candidate, and orphans left by an older standard are not either: no
// option of this standard produces them.
func planPrunes(repoRoot string, p *repoPlan) error {
	resolved := map[string]bool{}
	sections := map[state.SectionKey]bool{}
	for _, rp := range p.Resources {
		resolved[stateKey(rp.Resource.Path)] = true
		if rp.Section != nil {
			sections[state.SectionKey{Path: stateKey(rp.Resource.Path), ID: rp.Section.ID}] = true
		}
	}

	seen := map[string]bool{}
	for _, o := range p.Standard.Options {
		if p.Selection.Has(o) {
			continue
		}
		trial := p.Standard.With(p.Selection, o)
		tctx := *p.Context
		tctx.Integrations, tctx.Policies = trial.Integrations, trial.Policies
		for _, mod := range p.Standard.ModulesFor(trial) {
			resources, err := mod.Resolve(context.Background(), &tctx)
			if err != nil {
				return fmt.Errorf("resolve %s with %s selected: %w", mod.Name(), o.Label(), err)
			}
			for _, r := range resources {
				key := stateKey(r.Path)
				if r.Ownership == resource.ManagedSection {
					// A section is owned apart from its file: prune it if
					// state records it, whatever else the file holds.
					skey := state.SectionKey{Path: key, ID: r.SectionID}
					if _, ok := p.Previous.Sections[skey]; ok && !sections[skey] {
						sections[skey] = true
						p.Prunes = append(p.Prunes, prunePlan{Path: key, Option: o.Label(), Resource: r,
							Section: &sectionPlan{ID: r.SectionID, Pruned: true, resource: r}})
					}
					continue
				}
				recorded, ok := p.Previous.Resources[key]
				if resolved[key] || seen[key] || !ok {
					continue
				}
				seen[key] = true
				if r.Patch != nil {
					// A structured patch owns elements, not the file: prune
					// every recorded element by planning the file as if
					// this resource owned none.
					pp, err := prunePatch(repoRoot, recorded, r, o.Label())
					if err != nil {
						return err
					}
					p.Prunes = append(p.Prunes, pp)
					continue
				}
				decision, err := decideRemoval(repoRoot, key, recorded)
				if err != nil {
					return err
				}
				p.Prunes = append(p.Prunes, prunePlan{Path: key, Option: o.Label(), Decision: decision})
			}
		}
	}
	if err := planRetired(repoRoot, p, resolved, seen); err != nil {
		return err
	}
	planMoved(p, sections)
	return nil
}

// planMoved adds a prune for every recorded section that a selected
// module's section has moved away from: same ID, another file.
func planMoved(p *repoPlan, sections map[state.SectionKey]bool) {
	for _, mod := range p.Modules {
		mover, ok := mod.(module.SectionMover)
		if !ok {
			continue
		}
		for _, id := range mover.MovableSections() {
			var to *resource.Resource
			for _, rp := range p.Resources {
				if rp.Section != nil && rp.Section.ID == id {
					to = &rp.Resource
					break
				}
			}
			if to == nil {
				continue
			}
			for _, key := range slices.SortedFunc(maps.Keys(p.Previous.Sections), compareSectionKeys) {
				if key.ID != id || sections[key] {
					continue
				}
				sections[key] = true
				old := resource.Resource{Path: key.Path, Ownership: resource.ManagedSection, SectionID: id,
					Markers: to.Markers, Placement: to.Placement}
				p.Prunes = append(p.Prunes, prunePlan{Path: key.Path, Retired: "moved to " + stateKey(to.Path), Resource: old,
					Section: &sectionPlan{ID: id, Pruned: true, resource: old}})
			}
		}
	}
}

func compareSectionKeys(a, b state.SectionKey) int {
	return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.ID, b.ID))
}

// planRetired adds a prune for every recorded path that a module of the
// standard, selected or not, has retired and nothing still resolves.
func planRetired(repoRoot string, p *repoPlan, resolved, seen map[string]bool) error {
	mods := slices.Clone(p.Standard.Modules)
	for _, o := range p.Standard.Options {
		mods = append(mods, o.Module)
	}
	for _, mod := range mods {
		r, ok := mod.(module.Retirer)
		if !ok {
			continue
		}
		retired := r.Retired()
		for _, path := range slices.Sorted(maps.Keys(retired)) {
			key := stateKey(path)
			recorded, ok := p.Previous.Resources[key]
			if resolved[key] || seen[key] || !ok {
				continue
			}
			seen[key] = true
			decision, err := decideRemoval(repoRoot, key, recorded)
			if err != nil {
				return err
			}
			p.Prunes = append(p.Prunes, prunePlan{Path: key, Retired: retired[path], Decision: decision})
		}
	}
	return nil
}

// cause is why pp's path goes, as messages show it.
func (pp prunePlan) cause() string {
	if pp.Retired != "" {
		return pp.Retired
	}
	return pp.Option + " deselected"
}

func prunePatch(repoRoot string, recorded state.ResourceState, r resource.Resource, option string) (prunePlan, error) {
	none := r
	patch := *r.Patch
	patch.Elements = nil
	none.Patch = &patch
	pp, _, err := planPatch(repoRoot, &recorded, none)
	if err != nil {
		return prunePlan{}, err
	}
	decision := reconcile.Remove
	switch {
	case pp.conflicted():
		decision = reconcile.RemoveConflict
	case !pp.Exists:
		decision = reconcile.Forget
	}
	return prunePlan{Path: stateKey(r.Path), Option: option, Decision: decision, Resource: none, Patch: pp}, nil
}

func decideRemoval(repoRoot, key string, recorded state.ResourceState) (reconcile.Removal, error) {
	data, err := os.ReadFile(resourcePath(repoRoot, key)) // #nosec G304 -- repoRoot is an operator-supplied CLI flag; key is a registered module's resource path
	switch {
	case errors.Is(err, os.ErrNotExist):
		return reconcile.DecideRemoval(recorded.SHA256, nil), nil
	case err != nil:
		return 0, err
	}
	h := hashHex(data)
	return reconcile.DecideRemoval(recorded.SHA256, &h), nil
}

// markIgnored flags every resolved resource Git would ignore. Outside a
// work tree, or without git, the check is skipped with one warning.
func markIgnored(repoRoot string, p *repoPlan) error {
	paths := make([]string, 0, len(p.Resources))
	for _, rp := range p.Resources {
		paths = append(paths, stateKey(rp.Resource.Path))
	}
	ignored, err := checkIgnored(context.Background(), repoRoot, paths)
	switch {
	case errors.Is(err, errNoGit):
		p.Warnings = append(p.Warnings, fmt.Sprintf("%v: %s is not a git work tree, or git is not on PATH", err, repoRoot))
		return nil
	case err != nil:
		return fmt.Errorf("git check-ignore: %w", err)
	}
	for i := range p.Resources {
		p.Resources[i].Ignored = ignored[stateKey(p.Resources[i].Resource.Path)]
	}
	return nil
}

// checkComponents refuses a manifest whose components the standard would
// ignore, or a component standard with nothing to compose: either way the
// resolved resources would not be what vibe.yaml says
// (docs/decisions/0012-manifest-components.md).
func checkComponents(s *standard.Standard, m *manifest.Manifest) error {
	switch {
	case s.TakesComponents && len(m.Components) == 0:
		return fmt.Errorf("%s/%s needs at least one component: add a components: list to %s "+
			"(each entry an id, a path, and a profile: go, ts, or py)", s.Name, s.Version, manifestFileName)
	case !s.TakesComponents && len(m.Components) > 0:
		return fmt.Errorf("%s/%s takes no components, but %s declares %d; "+
			"use prod-mono for a repository with components", s.Name, s.Version, manifestFileName, len(m.Components))
	}
	return nil
}

// checkDocsDirs fails unless every docs directory vibe.yaml adopts is a
// directory in the repository: adopting means using what is there, and
// nothing is created or guessed (spec 0034 §1).
func checkDocsDirs(repoRoot string, m *manifest.Manifest) error {
	for _, k := range m.Development.AdoptedDirs() {
		p := m.Development.DocsDir(k.Key)
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(p))) // #nosec G304 G703 -- manifest.Parse admits only clean relative paths inside the repository
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%s.%s (%s): not a directory in this repository; an adopted directory must exist", k.Map, k.Key, p)
		}
	}
	return nil
}

// profiles lists the languages a repository declares: a single-language
// standard's own, or its components' in manifest.Profiles order.
func profiles(s *standard.Standard, m *manifest.Manifest) []manifest.Profile {
	if s.Profile != "" {
		return []manifest.Profile{s.Profile}
	}
	var used []manifest.Profile
	for _, p := range manifest.Profiles {
		for _, c := range m.Components {
			if c.Profile == p {
				used = append(used, p)
				break
			}
		}
	}
	return used
}

// planResource decides the outcome for a single resource by comparing the
// hash recorded in state, the file on disk, and the resolved content.
func planResource(repoRoot string, previous *state.State, mod module.Module, r resource.Resource) (resourcePlan, error) {
	if (r.Ownership == resource.StructuredPatch) != (r.Patch != nil) {
		return resourcePlan{}, fmt.Errorf("%s: a structured-patch resource needs a Patch, and only one may have it", r.Path)
	}
	if err := checkSection(r); err != nil {
		return resourcePlan{}, err
	}
	if r.Ownership == resource.ManagedSection {
		// Decided in planSections, with the file's other sections.
		checker, _ := mod.(module.SectionChecker)
		return resourcePlan{Resource: r, Supported: true, Section: &sectionPlan{ID: r.SectionID, checker: checker, resource: r}}, nil
	}
	if r.Patch != nil {
		var recorded *state.ResourceState
		if rs, ok := previous.Resources[stateKey(r.Path)]; ok {
			recorded = &rs
		}
		pp, decision, err := planPatch(repoRoot, recorded, r)
		if err != nil {
			return resourcePlan{}, err
		}
		return resourcePlan{Resource: r, Decision: decision, Supported: true, Patch: pp}, nil
	}
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
