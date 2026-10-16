package cli

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// Every finding of a failure names a file and a fix (spec 0052, "Report
// (#100)"): "A failure with no file named is a defect."
//
// The check walks every failing fixture there is, because it runs on
// every report a contract audit test produces: runVibeAudit installs it
// through the contractReportSeen hook, and every fixture test reaches the
// audit through runVibeAudit (auditJSON and runRuleCases included). A
// new rule's failing cases are checked as soon as they exist, with no
// list to keep. The check reads the report model, not the JSON, because
// only the model says which part a finding belongs to: a rule that fails
// statically may also carry an undecided part's "not implemented"
// finding, which names no file.

// reportsWatched counts the reports watchReports has checked.
var reportsWatched int

// watchReports makes every contract audit report printed during one
// runVibeAudit call checked for failures without a file or a fix.
func watchReports(t *testing.T) func() {
	t.Helper()
	contractReportSeen = func(rep *contract.Report) {
		t.Helper()
		reportsWatched++
		for _, p := range failureProblems(rep) {
			t.Errorf("a failure must name a file and a fix: %s", p)
		}
	}
	return func() { contractReportSeen = nil }
}

// failureProblems lists the failed parts in a report that have no
// finding, and the findings of failed parts that name no file or no fix.
func failureProblems(rep *contract.Report) []string {
	var out []string
	for _, rr := range rep.Rules {
		for _, p := range rr.Parts {
			if p.Result != contract.ResultFail {
				continue
			}
			if len(p.Findings) == 0 {
				out = append(out, rr.Rule.ID+" "+string(p.Kind)+" fails with no finding")
			}
			for _, f := range p.Findings {
				var missing []string
				if f.File == "" {
					missing = append(missing, "file")
				}
				if f.Fix == "" {
					missing = append(missing, "fix")
				}
				if len(missing) > 0 {
					out = append(out, rr.Rule.ID+" finding "+f.Message+" has no "+strings.Join(missing, " and no "))
				}
			}
		}
	}
	return out
}

// Every fixture run through runVibeAudit is checked, so a new rule's
// failing cases are covered without being listed anywhere.
func TestFailingFixturesAreWatched(t *testing.T) {
	before := reportsWatched
	runVibeAudit(t, "--repo-root", contractFixture(t, withoutFile(".github/pull_request_template.md")), "--static")
	auditJSON(t, contractFixture(t, withoutFile("AGENTS.md")), "--static")
	if got := reportsWatched - before; got != 2 {
		t.Errorf("%d reports were checked for failures without a file or a fix, want 2", got)
	}
}

func TestFailureProblems(t *testing.T) {
	rule, _ := contract.LookupRule("R-01")
	fail := func(findings ...contract.Finding) contract.PartOutcome {
		return contract.PartOutcome{Kind: contract.Static, Result: contract.ResultFail, Findings: findings}
	}
	undecided := contract.PartOutcome{Kind: contract.Execution, Result: contract.ResultUndecided,
		Findings: []contract.Finding{{Message: "the execution check is not implemented in this vibe; #102 adds it"}}}
	cases := []struct {
		name  string
		parts []contract.PartOutcome
		want  []string
	}{
		{"file and fix", []contract.PartOutcome{fail(contract.Finding{File: "vibe.yaml", Message: "m", Fix: "f"}), undecided}, nil},
		{"no file", []contract.PartOutcome{fail(contract.Finding{Message: "m", Fix: "f"})}, []string{"R-01 finding m has no file"}},
		{"no fix", []contract.PartOutcome{fail(contract.Finding{File: "vibe.yaml", Message: "m"})}, []string{"R-01 finding m has no fix"}},
		{"neither", []contract.PartOutcome{fail(contract.Finding{Message: "m"})}, []string{"R-01 finding m has no file and no fix"}},
		{"no finding", []contract.PartOutcome{fail()}, []string{"R-01 static fails with no finding"}},
		{"undecided needs neither", []contract.PartOutcome{undecided}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := &contract.Report{Rules: []contract.RuleResult{{Rule: rule, Result: contract.ResultFail, Parts: tc.parts}}}
			got := failureProblems(rep)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("failureProblems = %q, want %q", got, tc.want)
			}
		})
	}
}
