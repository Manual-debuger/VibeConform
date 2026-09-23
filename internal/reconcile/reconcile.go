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
// Truth table (spec 0006, refined by spec 0019):
//
//	P absent? | C absent? | C==T | C==P | T==P | Decision
//	yes       | yes       | –    | –    | –    | Create
//	yes       | no        | yes  | –    | –    | NoChange
//	yes       | no        | no   | –    | –    | Conflict
//	no        | yes       | –    | –    | –    | Create
//	no        | no        | yes  | –    | –    | NoChange
//	no        | no        | no   | yes  | no   | OutOfDate
//	no        | no        | no   | no   | yes  | LocalDrift
//	no        | no        | no   | no   | no   | Conflict
//
// The two "no/no/no" rows split from ADR-0002's literal "current !=
// previous and target != previous -> conflict" wording, and spec 0019
// splits them from each other:
//
//   - C != P, T == P: the desired state hasn't changed since the last
//     apply, but the file on disk has drifted from it (e.g. a hand edit of
//     a Generated file). Generated ownership means VibeConform owns the
//     whole file, so this is drift correction, not a genuine conflict.
//     Decision: LocalDrift.
//   - C == P, T != P: the file on disk still matches what was last
//     applied, and the target has simply moved on (e.g. the module's
//     template changed). This is the textbook "safe replacement" case.
//     Decision: OutOfDate.
//
// Both mean "write the target", which is why spec 0006 collapsed them into
// a single Overwrite decision. They differ in whose fault it is, and that
// difference is what a reader of vibe audit needs: LocalDrift says someone
// edited a managed file, OutOfDate says the repository is untouched and
// the standard moved. Reporting the second as the first accuses an adopter
// of editing a file they never opened (issue #22).
//
// Splitting them needs no extra recorded state, because the two conditions
// are mutually exclusive wherever both are reachable: if C == P and T == P
// both held, then C == T, and Decide would have returned NoChange before
// either was tested. So exactly one of the two applies, and which one is
// already determined by the three hashes.
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
	// LocalDrift means the managed file was changed since it was last
	// applied, while the desired state stayed put: the repository is wrong
	// and syncing restores it.
	LocalDrift
	// OutOfDate means the managed file is exactly what was last applied and
	// the desired state has moved on: the repository is untouched and
	// syncing updates it. Nobody edited anything.
	OutOfDate
	// Conflict means current and target have both diverged from previous
	// and disagree with each other; surfacing to the user rather than
	// silently overwriting.
	Conflict
)

// Writes reports whether d means sync should write the target to disk.
// LocalDrift and OutOfDate differ in what they say about how the
// repository got here, not in what reconciliation does about it.
func (d Decision) Writes() bool {
	return d == Create || d == LocalDrift || d == OutOfDate
}

// String returns a human-readable label for d.
func (d Decision) String() string {
	switch d {
	case Create:
		return "Create"
	case NoChange:
		return "NoChange"
	case LocalDrift:
		return "LocalDrift"
	case OutOfDate:
		return "OutOfDate"
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
	// Past this point C != T and P exists, so the two branches below are
	// mutually exclusive: T == P forces C != P, and testing it first means
	// the C == P branch can only be the untouched-repository case.
	if target == *previous {
		return LocalDrift
	}
	if *current == *previous {
		return OutOfDate
	}
	return Conflict
}
