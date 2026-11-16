package contract_test

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// Every finding of a failure names a file and a fix (spec 0052, "Report
// (#100)"): "A failure with no file named is a defect."
//
// The check walks every failing fixture there is, because it runs on
// every report a contract audit test produces: contracttest.Audit runs
// contracttest.Check on each report before it returns it, and every rule
// test reaches the audit through it (RunRuleCases included). A new
// rule's failing cases are checked as soon as they exist, with no list
// to keep. The check reads the report model, not the JSON, because only
// the model says which part a finding belongs to: a rule that fails
// statically may also carry an undecided part's "not implemented"
// finding, which names no file.

// Every fixture audited through contracttest.Audit is checked, listed or
// Git, static or not, so a new rule's failing cases are covered without
// being listed anywhere. Each audit here holds a real failing rule, whose
// findings pass the check, and a stand-in that fails with no file, which
// the check must report once per audit.
func TestFailingFixturesAreWatched(t *testing.T) {
	t.Parallel()
	noFile := failingStandIn(contract.Finding{Message: "stand-in", Fix: "do it"})
	audits := []struct {
		dir  string
		rule string
		opts contract.Options
	}{
		{contracttest.Fixture(t, contracttest.Baseline, contracttest.WithoutFile(".github/pull_request_template.md")),
			"R-03", contract.Options{Static: true}},
		{contracttest.GitFixture(t, contracttest.Baseline, contracttest.WithoutFile("AGENTS.md")),
			"R-02", contract.Options{Probe: contract.ProbeOptions{Env: contracttest.EmptyProbeEnv(t)}}},
	}
	rec := &errorRecorder{TB: t}
	for _, a := range audits {
		a.opts.Rules = append(contracttest.Rules(t, a.rule), noFile)
		rep := contracttest.Audit(rec, a.dir, a.opts)
		contracttest.AssertRule(t, rep, a.rule, contract.ResultFail)
	}
	if len(rec.errors) != len(audits) {
		t.Fatalf("the report check said %q, want one problem for each of %d reports", rec.errors, len(audits))
	}
	for _, e := range rec.errors {
		if !strings.Contains(e, "T-01 finding stand-in has no file") {
			t.Errorf("the report check said %q, want only the stand-in's missing file", e)
		}
	}
}

func TestFailureProblems(t *testing.T) {
	t.Parallel()
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
			got := contracttest.Problems(rep)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("Problems = %q, want %q", got, tc.want)
			}
		})
	}
}
