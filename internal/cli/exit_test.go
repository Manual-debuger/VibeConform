package cli

import (
	"errors"
	"fmt"
	"strings"
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

// TestExitCodeSeparatesOutOfDateFromDrift pins spec 0019's exit contract.
// Being behind the standard is non-zero — the repository is in fact behind
// — but distinct from 2, so a conformance job can decide for itself whether
// that should fail the build. Anything worse than out-of-date outranks it.
func TestExitCodeSeparatesOutOfDateFromDrift(t *testing.T) {
	tests := []struct {
		name string
		err  *nonConformantError
		want int
	}{
		{"only out of date", &nonConformantError{outOfDate: 2}, 3},
		{"drift outranks out of date", &nonConformantError{drifted: 1, outOfDate: 2}, 2},
		{"conflict outranks out of date", &nonConformantError{conflicts: 1, outOfDate: 2}, 2},
		{"only drift", &nonConformantError{drifted: 1}, 2},
		{"only conflict", &nonConformantError{conflicts: 1}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// TestStaleBinaryExitsAsCouldNotAnswer keeps the stale-binary case out of
// the conformance codes entirely. The repository may be perfectly fine; it
// is the install that is behind, and 2 or 3 would both blame the wrong
// thing.
func TestStaleBinaryExitsAsCouldNotAnswer(t *testing.T) {
	err := &staleBinaryError{recorded: "v0.3.0", running: "v0.2.0"}
	if got := ExitCode(err); got != 1 {
		t.Errorf("ExitCode = %d, want 1", got)
	}
	for _, want := range []string{"v0.3.0", "v0.2.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %v", want, err)
		}
	}
}
