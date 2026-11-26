package contract_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// errorRecorder is a testing.TB whose Errorf records the message instead
// of failing the test, so that a test can read what the report check
// said. Every other method is the real test's.
type errorRecorder struct {
	testing.TB
	errors []string
}

func (r *errorRecorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

// failingStandIn is a rule of one static part that fails with finding.
func failingStandIn(finding contract.Finding) contract.Rule {
	return contract.Rule{ID: "T-01", Parts: []contract.Part{{
		Kind: contract.Static,
		Run: func(*contract.Env) contract.PartResult {
			return contract.Fail(finding)
		},
	}}}
}

// The audit helper checks every report it returns (spec 0065, ADR 0028):
// a failure whose finding names no file is reported, and one that names
// a file and a fix is not. No test has to ask for the check.
func TestAuditHelperRunsTheReportCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		finding contract.Finding
		want    []string
	}{
		{"a finding with no file is reported",
			contract.Finding{Message: "stand-in", Fix: "do it"},
			[]string{"T-01 finding stand-in has no file"}},
		{"a finding with a file and a fix is not",
			contract.Finding{File: "vibe.yaml", Message: "stand-in", Fix: "do it"},
			nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := contracttest.Fixture(t, contracttest.Baseline)
			rec := &errorRecorder{TB: t}
			rep := contracttest.Audit(rec, dir, contract.Options{Static: true, Rules: []contract.Rule{failingStandIn(tc.finding)}})
			if len(rep.Rules) != 1 || rep.Rules[0].Result != contract.ResultFail {
				t.Fatalf("the stand-in rule did not fail: %+v", rep.Rules)
			}
			if len(rec.errors) != len(tc.want) {
				t.Fatalf("the report check said %q, want %d messages naming %q", rec.errors, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if !strings.Contains(rec.errors[i], w) {
					t.Errorf("the report check said %q, want it to name %q", rec.errors[i], w)
				}
			}
		})
	}
}
