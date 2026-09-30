package reconcile

// Removal is the outcome for a path VibeConform recorded and no longer
// wants: a deselected integration's resource
// (docs/decisions/0013-optional-integrations.md). It is decided two-way —
// there is no target — between the recorded hash and the file on disk.
type Removal int

const (
	// Forget means the file is already gone: only the state entry remains
	// to drop.
	Forget Removal = iota
	// Remove means the file is exactly what VibeConform last wrote, so
	// deleting it loses nothing.
	Remove
	// RemoveConflict means the file changed since it was recorded. Someone
	// edited it; deleting it would lose that edit, so it is kept and
	// surfaced instead.
	RemoveConflict
)

// String returns a human-readable label for r.
func (r Removal) String() string {
	switch r {
	case Forget:
		return "Forget"
	case Remove:
		return "Remove"
	case RemoveConflict:
		return "RemoveConflict"
	default:
		return "Unknown"
	}
}

// DecideRemoval compares the recorded hash of a no-longer-wanted path with
// the file on disk; current is nil when there is no file.
func DecideRemoval(previous string, current *string) Removal {
	switch {
	case current == nil:
		return Forget
	case *current == previous:
		return Remove
	default:
		return RemoveConflict
	}
}
