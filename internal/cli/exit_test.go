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
		{"contract: not conformant", &contractVerdictError{verdict: contract.VerdictNotConformant, reason: "1 MUST rule fails"}, 2},
		{"contract: wrapped not conformant", fmt.Errorf("audit: %w", &contractVerdictError{verdict: contract.VerdictNotConformant, reason: "2 MUST rules fail"}), 2},
		{"contract: doubly wrapped not conformant", fmt.Errorf("outer: %w", fmt.Errorf("audit: %w", &contractVerdictError{verdict: contract.VerdictNotConformant, reason: "3 MUST rules fail"})), 2},
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

// A contract audit's verdict maps to its exit code and stderr line: the
// engine's tests in internal/contract decide the verdict and its reason,
// and this maps them (spec 0051, "The verdict"; spec 0063).
func TestVerdictErrorExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rep  contract.Report
		text string
		code int
	}{
		{"conformant exits 0", contract.Report{Verdict: contract.VerdictConformant}, "", 0},
		{"a failed MUST rule exits 2", contract.Report{Verdict: contract.VerdictNotConformant, FailingMust: 1},
			"not conformant: 1 MUST rule fails", 2},
		{"a failing required rule exits 2", contract.Report{Verdict: contract.VerdictNotConformant, FailingRequired: 1},
			"not conformant: 1 required SHOULD rule fails", 2},
		{"MUST and required failures are counted apart",
			contract.Report{Verdict: contract.VerdictNotConformant, FailingMust: 1, FailingRequired: 2},
			"not conformant: 1 MUST rule fails; 2 required SHOULD rules fail", 2},
		{"cannot decide exits 1", contract.Report{Verdict: contract.VerdictUndecided, Reason: "3 rules cannot be decided"},
			"cannot decide: 3 rules cannot be decided", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verdictError(&tc.rep)
			text := ""
			if err != nil {
				text = err.Error()
			}
			if text != tc.text || ExitCode(err) != tc.code {
				t.Errorf("verdictError = %q, exit %d; want %q, exit %d", text, ExitCode(err), tc.text, tc.code)
			}
		})
	}
}
