package cli

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/state"
	"github.com/Manual-debuger/VibeConform/internal/textregion"
)

// sectionIDPattern is what a managed section's ID must match: the same
// shape as an option name, so it reads the same in markers and in state.
var sectionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// sectionPlan is what reconciliation would do to one managed section.
// Exactly one of Decision (a section the selection resolves) or Removal (a
// recorded section a deselected option would produce) applies, as Pruned
// says.
type sectionPlan struct {
	ID         string
	Decision   reconcile.Decision
	Pruned     bool
	Removal    reconcile.Removal
	TargetHash string
	// Reasons explain a conflict that is not a hash disagreement: malformed
	// markers, or what the owning module's check found.
	Reasons []string
	// checker is the owning module's check, if it has one.
	checker module.SectionChecker
	// resource is what the module resolved.
	resource resource.Resource
}

// conflicted reports whether this section, on its own, conflicts.
func (sp *sectionPlan) conflicted() bool {
	if sp.Pruned {
		return sp.Removal == reconcile.RemoveConflict
	}
	return sp.Decision == reconcile.Conflict
}

// sectionFile is every managed section of one file, planned together:
// sections share the file's bytes, so they are decided against one read
// and written in one write (docs/specs/0029-text-policy.md §3).
type sectionFile struct {
	Path     string
	Sections []*sectionPlan
	// Content is the file sync would write; nil when nothing changes.
	Content []byte
	// Delete is true when sync would delete the file: VibeConform created
	// it and removing a section left it empty.
	Delete bool
	// Created is true when VibeConform created, or would create, the file.
	Created bool
	// Exists is whether the file is on disk.
	Exists bool
	// written is set once sync has applied the file.
	written bool
}

// conflicted reports whether any section conflicts. A conflicted file is
// not written at all, so every other section of it waits too.
func (f *sectionFile) conflicted() bool {
	return slices.ContainsFunc(f.Sections, (*sectionPlan).conflicted)
}

func sectionSyntax(m resource.MarkerSyntax) textregion.Syntax {
	if m == resource.HTMLComment {
		return textregion.HTML
	}
	return textregion.Hash
}

func sectionPlacement(p resource.Placement) textregion.Placement {
	if p == resource.Bottom {
		return textregion.Bottom
	}
	return textregion.Top
}

// checkSection refuses a managed-section resource a module got wrong.
func checkSection(r resource.Resource) error {
	if (r.Ownership == resource.ManagedSection) != (r.SectionID != "") {
		return fmt.Errorf("%s: a managed-section resource needs a SectionID, and only one may have it", r.Path)
	}
	if r.SectionID == "" {
		return nil
	}
	if !sectionIDPattern.MatchString(r.SectionID) {
		return fmt.Errorf("%s: section id %q must match %s", r.Path, r.SectionID, sectionIDPattern)
	}
	if len(r.Content) > 0 && r.Content[len(r.Content)-1] != '\n' {
		return fmt.Errorf("%s (section %s): content must end with a newline", r.Path, r.SectionID)
	}
	if strings.Contains(string(r.Content), "vibeconform:begin") || strings.Contains(string(r.Content), "vibeconform:end") {
		return fmt.Errorf("%s (section %s): content must not contain a marker", r.Path, r.SectionID)
	}
	return nil
}

// planSections groups every resolved and every pruned section by file and
// plans each file once.
func planSections(repoRoot string, p *repoPlan) error {
	files := map[string]*sectionFile{}
	var order []string
	file := func(path string) *sectionFile {
		f, ok := files[path]
		if !ok {
			f = &sectionFile{Path: path}
			files[path] = f
			order = append(order, path)
		}
		return f
	}
	seen := map[state.SectionKey]bool{}
	add := func(f *sectionFile, sp *sectionPlan) error {
		key := state.SectionKey{Path: f.Path, ID: sp.ID}
		if seen[key] {
			return fmt.Errorf("%s: section %s is resolved twice", f.Path, sp.ID)
		}
		seen[key] = true
		if len(f.Sections) > 0 && f.Sections[0].resource.Markers != sp.resource.Markers {
			return fmt.Errorf("%s: sections %s and %s use different marker syntax", f.Path, f.Sections[0].ID, sp.ID)
		}
		f.Sections = append(f.Sections, sp)
		return nil
	}
	for i := range p.Resources {
		rp := &p.Resources[i]
		if rp.Section == nil {
			continue
		}
		rp.SectionFile = file(stateKey(rp.Resource.Path))
		if err := add(rp.SectionFile, rp.Section); err != nil {
			return err
		}
	}
	for i := range p.Prunes {
		pp := &p.Prunes[i]
		if pp.Section == nil {
			continue
		}
		pp.SectionFile = file(pp.Path)
		if err := add(pp.SectionFile, pp.Section); err != nil {
			return err
		}
	}
	for _, path := range order {
		if err := planSectionFile(repoRoot, p, files[path]); err != nil {
			return err
		}
	}
	// The commands count by the plans' own decisions.
	for i := range p.Resources {
		if sp := p.Resources[i].Section; sp != nil {
			p.Resources[i].Decision, p.Resources[i].TargetHash = sp.Decision, sp.TargetHash
		}
	}
	for i := range p.Prunes {
		if sp := p.Prunes[i].Section; sp != nil {
			p.Prunes[i].Decision = sp.Removal
		}
	}
	return nil
}

// planSectionFile decides every section of f against the file on disk and
// what state recorded, then builds the bytes sync would write.
func planSectionFile(repoRoot string, p *repoPlan, f *sectionFile) error {
	data, err := os.ReadFile(resourcePath(repoRoot, f.Path)) // #nosec G304 -- repoRoot is an operator-supplied CLI flag; f.Path is a registered module's resource path
	switch {
	case errors.Is(err, os.ErrNotExist):
		data = nil
	case err != nil:
		return err
	default:
		f.Exists = true
	}
	f.Created = !f.Exists
	for _, sp := range f.Sections {
		if rec, ok := p.Previous.Sections[state.SectionKey{Path: f.Path, ID: sp.ID}]; ok && rec.Created {
			f.Created = true
		}
	}

	syntax := sectionSyntax(f.Sections[0].resource.Markers)
	doc := textregion.Parse(data, syntax)
	work := data
	edit := func(sp *sectionPlan, fn func(span textregion.Span, ok bool) []byte) {
		span, ok, _ := textregion.Parse(work, syntax).Find(sp.ID)
		work = fn(span, ok)
	}

	for _, sp := range f.Sections {
		r := sp.resource
		recorded, wasRecorded := p.Previous.Sections[state.SectionKey{Path: f.Path, ID: sp.ID}]
		span, ok, ferr := doc.Find(sp.ID)

		if sp.Pruned {
			switch {
			case ferr != nil:
				sp.Removal, sp.Reasons = reconcile.RemoveConflict, []string{ferr.Error()}
			case !ok:
				sp.Removal = reconcile.Forget
			default:
				h := hashHex(doc.Inner(span))
				sp.Removal = reconcile.DecideRemoval(recorded.SHA256, &h)
			}
			if sp.Removal == reconcile.Remove {
				edit(sp, func(span textregion.Span, _ bool) []byte {
					return textregion.Remove(work, span, sectionPlacement(r.Placement))
				})
			}
			continue
		}

		sp.TargetHash = hashHex(r.Content)
		if ferr != nil {
			sp.Decision, sp.Reasons = reconcile.Conflict, []string{ferr.Error()}
			continue
		}
		var current, previous *string
		if ok {
			h := hashHex(doc.Inner(span))
			current = &h
		}
		if wasRecorded {
			previous = &recorded.SHA256
		}
		sp.Decision = reconcile.Decide(previous, current, sp.TargetHash)
		switch sp.Decision {
		case reconcile.Create:
			work = textregion.Insert(work, syntax, sp.ID, r.Content, sectionPlacement(r.Placement))
		case reconcile.LocalDrift, reconcile.OutOfDate:
			edit(sp, func(span textregion.Span, _ bool) []byte {
				return textregion.Replace(work, span, r.Content)
			})
		case reconcile.NoChange, reconcile.Conflict:
		}
	}

	// Each owning module checks the file as sync would leave it.
	final := textregion.Parse(work, syntax)
	for _, sp := range f.Sections {
		if sp.Pruned || sp.checker == nil {
			continue
		}
		span, ok, err := final.Find(sp.ID)
		if err != nil || !ok {
			continue
		}
		conflicts, warnings := sp.checker.CheckSection(sp.resource, work[:span.Start], work[span.End:])
		if len(conflicts) > 0 {
			sp.Decision = reconcile.Conflict
			sp.Reasons = append(sp.Reasons, conflicts...)
		}
		for _, w := range warnings {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s: %s", sectionLabel(f.Path, sp.ID), w))
		}
	}

	switch {
	case f.Exists && f.Created && len(work) == 0:
		f.Delete = true
	case string(work) != string(data):
		f.Content = work
	}
	return nil
}

// sectionLabel is how every report names a section.
func sectionLabel(path, id string) string {
	return fmt.Sprintf("%s (section %s)", path, id)
}

// held reports whether sp would change the file but cannot, because
// another section of it conflicts.
func held(f *sectionFile, sp *sectionPlan) bool {
	if !f.conflicted() || sp.conflicted() {
		return false
	}
	if sp.Pruned {
		return sp.Removal == reconcile.Remove
	}
	return sp.Decision.Writes()
}

const heldNote = " (held: another section of this file conflicts)"

func conflictWhy(sp *sectionPlan, fallback string) string {
	if len(sp.Reasons) > 0 {
		return strings.Join(sp.Reasons, "; ")
	}
	return fallback
}

func diffSectionLine(f *sectionFile, sp *sectionPlan) string {
	line := ""
	switch sp.Decision {
	case reconcile.Create:
		line = "would add"
	case reconcile.NoChange:
		return "no change"
	case reconcile.LocalDrift:
		line = "would update (edited since last applied state)"
	case reconcile.OutOfDate:
		line = "would update (standard moved since last applied state)"
	default:
		return "conflict: " + conflictWhy(sp, "the section differs from what VibeConform recorded") + ", review before sync"
	}
	if held(f, sp) {
		line += heldNote
	}
	return line
}

func auditSectionLine(sp *sectionPlan) string {
	if sp.Decision == reconcile.Conflict {
		return "conflict: " + conflictWhy(sp, "manual changes detected")
	}
	return auditLine(sp.Decision)
}

// diffSectionPruneLine and auditSectionPruneLine take the prune's cause,
// as prunePlan.cause gives it: "<option> deselected", or why a moved
// section leaves its old file.
func diffSectionPruneLine(f *sectionFile, sp *sectionPlan, cause string) string {
	switch sp.Removal {
	case reconcile.Forget:
		return fmt.Sprintf("would forget (%s; already removed)", cause)
	case reconcile.Remove:
		line := fmt.Sprintf("would remove (%s)", cause)
		if f.Delete {
			line = fmt.Sprintf("would remove, and the file (%s; nothing else is in it)", cause)
		}
		if held(f, sp) {
			line += heldNote
		}
		return line
	default:
		return fmt.Sprintf("conflict: %s but %s; kept", cause, conflictWhy(sp, "section modified since sync"))
	}
}

func auditSectionPruneLine(sp *sectionPlan, cause string) string {
	switch sp.Removal {
	case reconcile.Forget:
		return fmt.Sprintf("out of date (%s, already removed; run vibe sync to forget it)", cause)
	case reconcile.Remove:
		return fmt.Sprintf("out of date (%s; run vibe sync to remove)", cause)
	default:
		return fmt.Sprintf("conflict: %s but %s", cause, conflictWhy(sp, "section modified since sync"))
	}
}

// applySectionFile writes or deletes f, once, however many of its
// sections reach sync. The caller has checked it is not conflicted.
func applySectionFile(repoRoot string, f *sectionFile, mode resource.Resource) error {
	if f.written {
		return nil
	}
	f.written = true
	switch {
	case f.Delete:
		return removeResource(repoRoot, f.Path)
	case f.Content != nil:
		return writeResource(repoRoot, resource.Resource{Path: f.Path, Content: f.Content, Mode: mode.Mode})
	}
	return nil
}

// applySection applies one resolved section and returns its sync line.
func applySection(repoRoot string, rp resourcePlan, next *state.State, counts *syncCounts) (string, error) {
	f, sp := rp.SectionFile, rp.Section
	switch {
	case sp.conflicted():
		counts.conflicts++
		return "conflict: " + conflictWhy(sp, "the section differs from what VibeConform recorded") + "; resolve by hand", nil
	case f.conflicted():
		return "not written: another section of this file conflicts", nil
	}
	existed := f.Exists
	if err := applySectionFile(repoRoot, f, rp.Resource); err != nil {
		return "", err
	}
	next.Sections[state.SectionKey{Path: f.Path, ID: sp.ID}] = state.SectionState{SHA256: sp.TargetHash, Created: f.Created}
	switch sp.Decision {
	case reconcile.Create:
		counts.created++
		if !existed {
			return "created", nil
		}
		return "added", nil
	case reconcile.NoChange:
		counts.unchanged++
		return "unchanged", nil
	default:
		counts.updated++
		return "updated", nil
	}
}

// applySectionPrune removes one deselected section and returns its sync
// line.
func applySectionPrune(repoRoot string, pp prunePlan, next *state.State, counts *syncCounts) (string, error) {
	f, sp := pp.SectionFile, pp.Section
	switch {
	case sp.conflicted():
		counts.conflicts++
		if pp.Retired != "" {
			return fmt.Sprintf("conflict: %s but %s; kept (remove it by hand)",
				pp.Retired, conflictWhy(sp, "section modified since sync")), nil
		}
		return fmt.Sprintf("conflict: %s deselected but %s; kept (remove it by hand, or select %s again)",
			pp.Option, conflictWhy(sp, "section modified since sync"), pp.Option), nil
	case f.conflicted():
		return "not removed: another section of this file conflicts", nil
	}
	if err := applySectionFile(repoRoot, f, pp.Resource); err != nil {
		return "", err
	}
	delete(next.Sections, state.SectionKey{Path: f.Path, ID: sp.ID})
	counts.removed++
	switch {
	case sp.Removal == reconcile.Forget:
		return fmt.Sprintf("forgotten (%s; already removed)", pp.cause()), nil
	case f.Delete:
		return fmt.Sprintf("removed, and the file (%s; nothing else was in it)", pp.cause()), nil
	default:
		return fmt.Sprintf("removed (%s)", pp.cause()), nil
	}
}
