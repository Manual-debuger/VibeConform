package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/Manual-debuger/VibeConform/internal/jsonarray"
	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// elementPlan is what reconciliation would do to one owned element of a
// structured-patch resource. Exactly one of Decision (an owned element the
// module resolves) or Removal (a recorded element it no longer resolves)
// applies, as Pruned says.
type elementPlan struct {
	ID         string
	Key        string
	Decision   reconcile.Decision
	Pruned     bool
	Removal    reconcile.Removal
	TargetHash string
	// Reason explains a conflict that is not a hash disagreement.
	Reason string
}

// patchPlan is the element-level plan for one structured-patch resource,
// and the file it would leave behind.
type patchPlan struct {
	Elements []elementPlan
	// Content is the file sync would write; nil when nothing changes.
	Content []byte
	// Delete is true when sync would delete the file: VibeConform created
	// it and nothing but its skeleton would remain.
	Delete bool
	// Created is true when VibeConform created, or would create, the file.
	Created bool
	// Exists is whether the file is on disk.
	Exists bool
	// Invalid is why the file could not be edited at all, if it could not.
	Invalid string
}

// conflicted reports whether any element, or the file itself, conflicts.
// A conflicted resource is not written at all: half a resource would
// record a state no single decision describes.
func (pp *patchPlan) conflicted() bool {
	if pp.Invalid != "" {
		return true
	}
	return slices.ContainsFunc(pp.Elements, func(e elementPlan) bool {
		return (!e.Pruned && e.Decision == reconcile.Conflict) || (e.Pruned && e.Removal == reconcile.RemoveConflict)
	})
}

// planPatch reconciles every owned element of r three-way against the
// element recorded in state and the element with the same identity on
// disk, and prunes recorded elements r no longer owns. It returns the
// plan and the resource-level decision the commands count by.
func planPatch(repoRoot string, recorded *state.ResourceState, r resource.Resource) (*patchPlan, reconcile.Decision, error) {
	patch := r.Patch
	data, err := os.ReadFile(resourcePath(repoRoot, r.Path)) // #nosec G304 -- repoRoot is an operator-supplied CLI flag; r.Path is a registered module's resource path
	pp := &patchPlan{Exists: err == nil, Created: recorded != nil && recorded.Created}
	switch {
	case errors.Is(err, os.ErrNotExist):
		data = patch.Skeleton
		pp.Created = true
	case err != nil:
		return nil, 0, err
	}

	doc, err := jsonarray.Parse(data, patch.Array)
	if err != nil {
		pp.Invalid = err.Error()
		return pp, reconcile.Conflict, nil
	}

	changed := false
	targets := map[string]bool{}
	for _, e := range patch.Elements {
		key := patch.StateKey(e.ID)
		targets[key] = true
		ep := elementPlan{ID: e.ID, Key: key}
		if ep.TargetHash, err = jsonarray.HashValue(e.Value); err != nil {
			return nil, 0, fmt.Errorf("%s: element %q: %w", r.Path, e.ID, err)
		}

		i, err := doc.Find(e.ID)
		if err != nil {
			ep.Decision, ep.Reason = reconcile.Conflict, err.Error()
			pp.Elements = append(pp.Elements, ep)
			continue
		}
		var current, previous *string
		if i >= 0 {
			h, err := doc.Hash(i)
			if err != nil {
				return nil, 0, fmt.Errorf("%s: element %q: %w", r.Path, e.ID, err)
			}
			current = &h
		}
		if recorded != nil {
			if es, ok := recorded.Elements[key]; ok {
				previous = &es.SHA256
			}
		}

		ep.Decision = reconcile.Decide(previous, current, ep.TargetHash)
		switch ep.Decision {
		case reconcile.Create:
			err = doc.Append(e.Value)
			changed = true
		case reconcile.LocalDrift, reconcile.OutOfDate:
			err = doc.Replace(i, e.Value)
			changed = true
		case reconcile.NoChange, reconcile.Conflict:
		}
		if err != nil {
			return nil, 0, fmt.Errorf("%s: element %q: %w", r.Path, e.ID, err)
		}
		pp.Elements = append(pp.Elements, ep)
	}

	// Recorded elements this resource no longer owns, in key order.
	if recorded != nil {
		keys := make([]string, 0, len(recorded.Elements))
		for key := range recorded.Elements {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			id, ok := patch.OwnsKey(key)
			if !ok || targets[key] {
				continue
			}
			ep := elementPlan{ID: id, Key: key, Pruned: true, Removal: reconcile.Forget}
			i, err := doc.Find(id)
			switch {
			case err != nil:
				ep.Removal, ep.Reason = reconcile.RemoveConflict, err.Error()
			case i >= 0:
				h, err := doc.Hash(i)
				if err != nil {
					return nil, 0, fmt.Errorf("%s: element %q: %w", r.Path, id, err)
				}
				ep.Removal = reconcile.DecideRemoval(recorded.Elements[key].SHA256, &h)
				if ep.Removal == reconcile.Remove {
					doc.Remove(i)
					changed = true
				}
			}
			pp.Elements = append(pp.Elements, ep)
		}
	}

	if changed && (pp.Exists || len(patch.Elements) > 0) {
		pp.Content = doc.Bytes()
	}
	if pp.Exists && pp.Created && len(patch.Elements) == 0 && pp.Content != nil {
		if same, err := sameJSON(pp.Content, patch.Skeleton); err == nil && same {
			pp.Content, pp.Delete = nil, true
		}
	}
	return pp, pp.decision(), nil
}

// decision folds the element decisions into one for the resource, the
// most serious first: a conflict, then an edit, then something missing,
// then the standard having moved.
func (pp *patchPlan) decision() reconcile.Decision {
	if pp.conflicted() {
		return reconcile.Conflict
	}
	if !pp.Exists && pp.Content != nil {
		return reconcile.Create
	}
	has := func(d reconcile.Decision) bool {
		return slices.ContainsFunc(pp.Elements, func(e elementPlan) bool { return !e.Pruned && e.Decision == d })
	}
	switch {
	case has(reconcile.LocalDrift):
		return reconcile.LocalDrift
	case has(reconcile.Create):
		return reconcile.Create
	case has(reconcile.OutOfDate), slices.ContainsFunc(pp.Elements, func(e elementPlan) bool { return e.Pruned }):
		return reconcile.OutOfDate
	}
	return reconcile.NoChange
}

// nextState is what .vibe/state.yaml records for the resource once pp is
// applied; nil means no entry at all.
func (pp *patchPlan) nextState() *state.ResourceState {
	elements := map[string]state.ElementState{}
	for _, e := range pp.Elements {
		if !e.Pruned {
			elements[e.Key] = state.ElementState{SHA256: e.TargetHash}
		}
	}
	if pp.Delete || len(elements) == 0 {
		return nil
	}
	return &state.ResourceState{Created: pp.Created, Elements: elements}
}

// applyPatch writes or deletes the file per pp. The caller has already
// checked it is not conflicted.
func applyPatch(repoRoot string, r resource.Resource, pp *patchPlan) error {
	switch {
	case pp.Delete:
		return removeResource(repoRoot, stateKey(r.Path))
	case pp.Content != nil:
		return writeResource(repoRoot, resource.Resource{Path: r.Path, Content: pp.Content, Mode: r.Mode})
	}
	return nil
}

// elementLines describe, one line each, the elements that are not
// already as wanted; verb picks the tense.
func elementLines(pp *patchPlan, verb func(elementPlan) string) []string {
	if pp.Invalid != "" {
		return []string{"conflict: " + pp.Invalid + "; fix the file by hand"}
	}
	var lines []string
	for _, e := range pp.Elements {
		if !e.Pruned && e.Decision == reconcile.NoChange {
			continue
		}
		lines = append(lines, verb(e))
	}
	return lines
}

func diffElement(e elementPlan) string {
	if e.Pruned {
		switch e.Removal {
		case reconcile.Forget:
			return fmt.Sprintf("would forget %q (already removed)", e.ID)
		case reconcile.Remove:
			return fmt.Sprintf("would remove %q", e.ID)
		default:
			return conflictElement(e, "modified since sync; kept")
		}
	}
	switch e.Decision {
	case reconcile.Create:
		return fmt.Sprintf("would add %q", e.ID)
	case reconcile.LocalDrift:
		return fmt.Sprintf("would update %q (edited since last applied state)", e.ID)
	case reconcile.OutOfDate:
		return fmt.Sprintf("would update %q (standard moved since last applied state)", e.ID)
	default:
		return conflictElement(e, "an element with that identity is not VibeConform's")
	}
}

func auditElement(e elementPlan) string {
	if e.Pruned {
		switch e.Removal {
		case reconcile.Forget:
			return fmt.Sprintf("out of date (%q no longer owned, already removed; run vibe sync to forget it)", e.ID)
		case reconcile.Remove:
			return fmt.Sprintf("out of date (%q no longer owned; run vibe sync to remove)", e.ID)
		default:
			return conflictElement(e, "no longer owned but modified since sync")
		}
	}
	switch e.Decision {
	case reconcile.Create:
		return fmt.Sprintf("missing %q (run vibe sync)", e.ID)
	case reconcile.LocalDrift:
		return fmt.Sprintf("drifted %q (edited since last sync; run vibe sync to restore)", e.ID)
	case reconcile.OutOfDate:
		return fmt.Sprintf("out of date %q (standard moved; run vibe sync to update)", e.ID)
	default:
		return conflictElement(e, "an element with that identity is not VibeConform's")
	}
}

func syncElement(e elementPlan) string {
	if e.Pruned {
		switch e.Removal {
		case reconcile.Forget:
			return fmt.Sprintf("forgot %q (already removed)", e.ID)
		case reconcile.Remove:
			return fmt.Sprintf("removed %q", e.ID)
		default:
			return conflictElement(e, "no longer owned but modified since sync; kept")
		}
	}
	switch e.Decision {
	case reconcile.Create:
		return fmt.Sprintf("added %q", e.ID)
	case reconcile.LocalDrift, reconcile.OutOfDate:
		return fmt.Sprintf("updated %q", e.ID)
	default:
		return conflictElement(e, "an element with that identity is not VibeConform's; rename or remove it")
	}
}

func conflictElement(e elementPlan, why string) string {
	if e.Reason != "" {
		why = e.Reason
	}
	return fmt.Sprintf("conflict: %q: %s", e.ID, why)
}

// printElementLines writes each line under path, or "path: fallback"
// when there is nothing to say about any element.
func printElementLines(w io.Writer, path string, lines []string, fallback string) error {
	if len(lines) == 0 {
		lines = []string{fallback}
	}
	for _, l := range lines {
		if _, err := fmt.Fprintf(w, "%s: %s\n", path, l); err != nil {
			return err
		}
	}
	return nil
}

// sameJSON reports whether a and b are the same JSON value.
func sameJSON(a, b []byte) (bool, error) {
	ca, err := jsonarray.Canonical(a)
	if err != nil {
		return false, err
	}
	cb, err := jsonarray.Canonical(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ca, cb), nil
}
