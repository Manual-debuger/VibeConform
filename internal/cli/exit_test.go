package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

func TestExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"other error", errors.New("open vibe.yaml: no such file"), 1},
		{"wrapped other error", fmt.Errorf("audit: %w", errors.New("boom")), 1},
		{"contract: not conformant", &contractVerdictError{verdict: contract.VerdictNotConformant, failing: 1}, 2},
		{"contract: wrapped not conformant", fmt.Errorf("audit: %w", &contractVerdictError{verdict: contract.VerdictNotConformant, failing: 2}), 2},
		{"contract: doubly wrapped not conformant", fmt.Errorf("outer: %w", fmt.Errorf("audit: %w", &contractVerdictError{verdict: contract.VerdictNotConformant, failing: 3})), 2},
		{"contract: cannot decide", &contractVerdictError{verdict: contract.VerdictUndecided, reason: "1 rule did not run (--static)"}, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
