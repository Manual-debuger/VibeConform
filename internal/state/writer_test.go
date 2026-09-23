package state

import "testing"

func TestCompareWriters(t *testing.T) {
	// A Go-style pseudo-version, the form Taskfile.local.yml stamps onto a
	// locally built binary so it is orderable against releases.
	const (
		release     = "v0.2.0-alpha.1"
		nextRelease = "v0.3.0"
		pseudoNew   = "v0.2.1-0.20260923031702-1c7666bdead1"
		pseudoOld   = "v0.2.1-0.20260101000000-aaaaaaaaaaaa"
	)

	tests := []struct {
		name              string
		recorded, running string
		want              WriterOrder
	}{
		{"equal releases", release, release, WriterSame},
		{"running is a later release", release, nextRelease, WriterRunningNewer},
		{"running is an earlier release", nextRelease, release, WriterRunningOlder},

		// The motivating case: a local build outranks the release it
		// descends from, so running a stale ~/go/bin release against state
		// written by that local build is detected as a downgrade.
		{"pseudo-version outranks the release it descends from", release, pseudoNew, WriterRunningNewer},
		{"stale release against a local build", pseudoNew, release, WriterRunningOlder},
		{"pseudo-versions order by commit time", pseudoOld, pseudoNew, WriterRunningNewer},
		{"older pseudo-version", pseudoNew, pseudoOld, WriterRunningOlder},

		{"no provenance recorded", "", release, WriterUnknown},
		{"both absent", "", "", WriterUnknown},
		{"garbage", release, "not-a-version", WriterUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareWriters(tt.recorded, tt.running); got != tt.want {
				t.Errorf("CompareWriters(%q, %q) = %v, want %v",
					tt.recorded, tt.running, got, tt.want)
			}
		})
	}
}

// TestCompareWritersNeverCallsStaleOnDevBuild pins the specific trap this
// package exists to avoid. semver.Compare("dev", "v0.2.0") returns -1,
// because it returns -1 for any invalid input rather than reporting that it
// could not compare. cmd/vibe sets version = "dev" for every build that is
// not stamped by GoReleaser, so an implementation that compared first and
// checked validity second — or never — would declare every locally built
// binary older than the recorded writer, and refuse to sync on the most
// common developer path there is.
//
// Kept separate from the table above so the failure names the cause rather
// than reading as one row among ten.
func TestCompareWritersNeverCallsStaleOnDevBuild(t *testing.T) {
	for _, running := range []string{"dev", "", "v", "1.2.3", "devel+abc"} {
		t.Run(running, func(t *testing.T) {
			got := CompareWriters("v0.2.0-alpha.1", running)
			if got == WriterRunningOlder {
				t.Errorf("CompareWriters(%q, %q) = %v; an unorderable version "+
					"must never be reported as older — semver.Compare returns -1 "+
					"for invalid input, and acting on that would refuse to sync "+
					"on every unstamped local build",
					"v0.2.0-alpha.1", running, got)
			}
			if got != WriterUnknown {
				t.Errorf("CompareWriters(%q, %q) = %v, want Unknown",
					"v0.2.0-alpha.1", running, got)
			}
		})
	}
}
