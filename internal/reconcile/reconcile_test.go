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
		{"P present, C present, C!=T, C==P, T!=P", &a, &a, b, Overwrite},
		{"P present, C present, C!=T, C!=P, T==P", &a, &b, a, Overwrite},
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
		{Overwrite, "Overwrite"},
		{Conflict, "Conflict"},
		{Decision(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Decision(%d).String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}
