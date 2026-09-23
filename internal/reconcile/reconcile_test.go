package reconcile

import "testing"

func TestDecide(t *testing.T) {
	// Distinct stand-ins for content hashes; declared as vars (not consts)
	// so they're addressable for the *string previous/current fields below.
	a, b, c := "content-a", "content-b", "content-c"

	tests := []struct {
		name     string
		previous *string
		current  *string
		target   string
		want     Decision
	}{
		{"P absent, C absent", nil, nil, a, Create},
		{"P absent, C present, C==T", nil, &a, a, NoChange},
		{"P absent, C present, C!=T", nil, &a, b, Conflict},
		{"P present, C absent", &a, nil, b, Create},
		{"P present, C present, C==T", &a, &b, b, NoChange},
		// The two rows spec 0019 split apart. Before it, both returned
		// Overwrite and audit reported both as "drifted", which accused an
		// adopter of editing a file they never opened (issue #22).
		{"P present, C present, C!=T, C==P, T!=P", &a, &a, b, OutOfDate},
		{"P present, C present, C!=T, C!=P, T==P", &a, &b, a, LocalDrift},
		{"P present, C present, C!=T, C!=P, T!=P", &a, &b, c, Conflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.previous, tt.current, tt.target)
			if got != tt.want {
				t.Errorf("Decide(%v, %v, %q) = %v, want %v", tt.previous, tt.current, tt.target, got, tt.want)
			}
		})
	}
}

func TestDecisionString(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{Create, "Create"},
		{NoChange, "NoChange"},
		{LocalDrift, "LocalDrift"},
		{OutOfDate, "OutOfDate"},
		{Conflict, "Conflict"},
		{Decision(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Decision(%d).String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// TestLocalDriftAndOutOfDateAreExclusive pins the property that lets spec
// 0019 split the two cases without recording anything beyond the hashes
// spec 0006 already stored: C==P and T==P can never both hold by the time
// Decide reaches them, because that would make C==T and return NoChange
// first. If a future edit reorders Decide so both can be true, one of these
// two rows silently starts shadowing the other; this test fails instead.
func TestLocalDriftAndOutOfDateAreExclusive(t *testing.T) {
	same := "content-a"

	if got := Decide(&same, &same, same); got != NoChange {
		t.Fatalf("Decide with P==C==T = %v, want NoChange — the exclusivity "+
			"argument for splitting LocalDrift from OutOfDate depends on this", got)
	}
}

func TestWrites(t *testing.T) {
	tests := []struct {
		d    Decision
		want bool
	}{
		{Create, true},
		{LocalDrift, true},
		{OutOfDate, true},
		{NoChange, false},
		{Conflict, false},
	}
	for _, tt := range tests {
		if got := tt.d.Writes(); got != tt.want {
			t.Errorf("%v.Writes() = %v, want %v", tt.d, got, tt.want)
		}
	}
}
