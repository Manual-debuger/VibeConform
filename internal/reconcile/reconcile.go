// Package reconcile implements the three-way reconciliation decision
// described in docs/architecture/overview.md and
// docs/decisions/0002-desired-state.md, scoped to resource.Generated
// ownership only (see docs/specs/0006-reconcile-diff-v1.md).
//
// Decide compares three content hashes:
//
//   - P (previous): what was last recorded in .vibe/state.yaml for this
//     path, nil if never recorded.
//   - C (current): the file on disk right now, nil if it doesn't exist.
//   - T (target): the resource a module resolves to. Always present.
//
// Truth table (spec 0006):
//
//	P absent? | C absent? | C==T | C==P | T==P | Decision
//	yes       | yes       | –    | –    | –    | Create
//	yes       | no        | yes  | –    | –    | NoChange
//	yes       | no        | no   | –    | –    | Conflict
//	no        | yes       | –    | –    | –    | Create
//	no        | no        | yes  | –    | –    | NoChange
//	no        | no        | no   | yes  | no   | Overwrite
//	no        | no        | no   | no   | yes  | Overwrite
//	no        | no        | no   | no   | no   | Conflict
//
// The two "no/no/no" rows split from ADR-0002's literal "current !=
// previous and target != previous -> conflict" wording:
//
//   - C != P, T == P: the desired state hasn't changed since the last
//     apply, but the file on disk has drifted from it (e.g. a hand edit of
//     a Generated file). Generated ownership means VibeConform owns the
//     whole file, so this is drift correction, not a genuine conflict.
//     Decision: Overwrite.
//   - C == P, T != P: the file on disk still matches what was last
//     applied, and the target has simply moved on (e.g. the module's
//     template changed). This is the textbook "safe replacement" case.
//     Decision: Overwrite.
//
// Only when both current and target have diverged from previous, and they
// disagree with each other, is it a genuine Conflict.
package reconcile

// Decision is the outcome of comparing previous/current/target content for
// one resource.
type Decision int

const (
	// Create means the file doesn't exist yet and should be written.
	Create Decision = iota
	// NoChange means the current file already matches the target.
	NoChange
	// Overwrite means the current file should be replaced with the target
	// (safe replacement or drift correction).
	Overwrite
	// Conflict means current and target have both diverged from previous
	// and disagree with each other; surfacing to the user rather than
	// silently overwriting.
	Conflict
)

// String returns a human-readable label for d.
func (d Decision) String() string {
	switch d {
	case Create:
		return "Create"
	case NoChange:
		return "NoChange"
	case Overwrite:
		return "Overwrite"
	case Conflict:
		return "Conflict"
	default:
		return "Unknown"
	}
}

// Decide compares previous, current, and target content hashes for one
// resource.Generated resource and returns the reconciliation Decision. See
// the package doc for the full truth table and rationale. previous and
// current are nil when absent (no recorded prior state / no file on disk);
// target is always present.
func Decide(previous, current *string, target string) Decision {
	if current == nil {
		return Create
	}
	if *current == target {
		return NoChange
	}
	if previous == nil {
		return Conflict
	}
	if *current == *previous || target == *previous {
		return Overwrite
	}
	return Conflict
}
