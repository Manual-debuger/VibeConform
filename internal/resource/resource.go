// Package resource defines the unit of managed repository state that
// modules resolve and the reconciler applies.
//
// A Resource represents one file or well-defined fragment of a file that
// VibeConform manages, along with the ownership mode that governs how
// reconciliation is allowed to touch it. See docs/decisions/0003-resource-ownership.md.
package resource

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
	// Path is the repository-relative path of the managed file.
	Path string
	// Ownership governs how reconciliation may modify Path.
	Ownership Ownership
	// Content is the desired content for Generated resources, or the
	// desired fragment for StructuredPatch/ManagedSection resources.
	Content []byte
}
