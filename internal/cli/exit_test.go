package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"non-conformance", &nonConformantError{drifted: 1}, 2},
		{"wrapped non-conformance", fmt.Errorf("audit: %w", &nonConformantError{conflicts: 2}), 2},
		{"doubly wrapped non-conformance", fmt.Errorf("outer: %w", fmt.Errorf("audit: %w", &nonConformantError{drifted: 3})), 2},
		{"other error", errors.New("open vibe.yaml: no such file"), 1},
		{"wrapped other error", fmt.Errorf("audit: %w", errors.New("boom")), 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

func TestNonConformantErrorMessage(t *testing.T) {
	err := &nonConformantError{drifted: 2, outOfDate: 3, conflicts: 1}
	if want := "not conformant: 2 drifted, 3 out of date, 1 conflicts"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
