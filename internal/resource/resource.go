// Package resource defines the unit of managed repository state that
// modules resolve and the reconciler applies.
//
// A Resource represents one file or well-defined fragment of a file that
// VibeConform manages, along with the ownership mode that governs how
// reconciliation is allowed to touch it. See docs/decisions/0003-resource-ownership.md.
package resource

import (
	"os"
	"strings"
)

// Ownership describes how much authority VibeConform has over a Resource.
type Ownership int

const (
	// Generated resources are fully owned by VibeConform and may be
	// overwritten wholesale.
	Generated Ownership = iota
	// StructuredPatch resources are owned by VibeConform for specific
	// structured fields (e.g. keys in a YAML/JSON document) while leaving
	// the rest of the document untouched.
	StructuredPatch
	// ManagedSection resources are project-owned files containing a
	// delimited section that VibeConform manages.
	ManagedSection
	// ProjectOwned resources are never written by VibeConform; they are
	// read for context only.
	ProjectOwned
)

// Resource is a single piece of desired state resolved by a Module.
type Resource struct {
	// Path is the repository-relative path of the managed file, always
	// slash-separated: it identifies the resource and keys .vibe/state.yaml,
	// so it must not vary by host platform.
	Path string
	// Ownership governs how reconciliation may modify Path.
	Ownership Ownership
	// Content is the desired content for Generated resources, or the
	// desired fragment for StructuredPatch/ManagedSection resources.
	Content []byte
	// Mode is the file mode to write Path with. The zero value means the
	// default for this resource's Ownership; a module only sets it when it
	// needs something else, such as 0o755 for a hook script. Mode is applied
	// on write but does not participate in reconciliation — see
	// docs/decisions/0006-resource-file-mode.md.
	Mode os.FileMode
	// Patch describes a StructuredPatch resource: the elements VibeConform
	// owns in one array of a JSON(C) file it shares with its users
	// (docs/decisions/0013-optional-integrations.md). Nil for every other
	// ownership; Content is unused when it is set.
	Patch *ArrayPatch
}

// ArrayPatch is a structured patch over one array of a JSON(C) document.
type ArrayPatch struct {
	// Array is the array's member name in the root object, or "" when the
	// root is the array.
	Array string
	// Skeleton is the file written when there is none: the array, empty,
	// in its document. Owned elements are then added to it.
	Skeleton []byte
	// Elements are the owned elements, in the order they are added.
	Elements []Element
}

// Element is one owned array element.
type Element struct {
	// ID is its identity: an object's "label", or a string's value.
	ID string
	// Value is its JSON text, exactly as written into the file.
	Value []byte
}

// StateKey is how .vibe/state.yaml records an element of p:
// "<array>/<id>", with "[]" naming a root array.
func (p *ArrayPatch) StateKey(id string) string {
	if p.Array == "" {
		return "[]/" + id
	}
	return p.Array + "/" + id
}

// OwnsKey reports whether a recorded element key belongs to p's array.
func (p *ArrayPatch) OwnsKey(key string) (id string, ok bool) {
	return strings.CutPrefix(key, p.StateKey(""))
}

// DefaultMode is the file mode used for resources that do not declare one.
// Generated files are ordinary repository files meant to be read by other
// tools and other users, so they are world-readable rather than owner-only.
func DefaultMode(_ Ownership) os.FileMode {
	return 0o644
}

// ModeOrDefault returns the mode r should be written with.
func (r Resource) ModeOrDefault() os.FileMode {
	if r.Mode == 0 {
		return DefaultMode(r.Ownership)
	}
	return r.Mode
}
