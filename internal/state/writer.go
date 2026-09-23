package state

import "golang.org/x/mod/semver"

// WriterOrder is how the running vibe binary's version relates to the one
// recorded as having last written .vibe/state.yaml.
//
// It is an enum rather than a comparison integer on purpose. The question
// callers ask — "would syncing revert this repository?" — is a direction,
// and a bare -1/0/+1 invites getting that direction backwards at the call
// site, in code whose entire job is to prevent a destructive revert. The
// zero value is WriterUnknown, so a caller that forgets to assign one gets
// "no claim" rather than a confident wrong one.
type WriterOrder int

const (
	// WriterUnknown means the two versions cannot be ordered: one or both
	// is absent, or is not valid semver (a locally built binary reports
	// "dev"). Never an error — it means no direction may be claimed.
	WriterUnknown WriterOrder = iota
	// WriterSame means both versions are valid and equal.
	WriterSame
	// WriterRunningNewer means the running binary is newer than the one
	// that last wrote the state: the repository is behind the standard.
	WriterRunningNewer
	// WriterRunningOlder means the running binary is older than the one
	// that last wrote the state. Syncing would revert managed files to
	// older templates, and audit cannot judge conformance against a
	// standard definition older than the repository's.
	WriterRunningOlder
)

// String returns a human-readable label for o.
func (o WriterOrder) String() string {
	switch o {
	case WriterUnknown:
		return "Unknown"
	case WriterSame:
		return "Same"
	case WriterRunningNewer:
		return "RunningNewer"
	case WriterRunningOlder:
		return "RunningOlder"
	default:
		return "Unknown"
	}
}

// CompareWriters reports how the running binary's version relates to the
// version recorded in .vibe/state.yaml.
//
// Both versions must be valid semver for any direction to be claimed.
// This is not defensive tidiness: semver.Compare returns -1 for invalid
// input, so comparing the default "dev" version of a locally built binary
// against a released one reads as "older" and would brand every unstamped
// developer build stale. Gating on IsValid for both operands first is the
// only thing standing between this package and that bug, which is why
// callers are given no path to semver.Compare of their own.
func CompareWriters(recorded, running string) WriterOrder {
	if !semver.IsValid(recorded) || !semver.IsValid(running) {
		return WriterUnknown
	}
	switch semver.Compare(running, recorded) {
	case 0:
		return WriterSame
	case 1:
		return WriterRunningNewer
	default:
		return WriterRunningOlder
	}
}
