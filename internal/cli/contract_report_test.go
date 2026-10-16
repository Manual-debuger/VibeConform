package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// reportOf builds a report the way the audit would: every rule of the
// registry, passing, unless results says otherwise.
func reportOf(t *testing.T, static bool, results map[string]contract.RuleResult) *contract.Report {
	t.Helper()
	rep := &contract.Report{Contract: contract.Versions{Declared: "1.0.0", Audited: "1.0.0"}, Static: static}
	for _, rule := range contract.Rules() {
		rr, ok := results[rule.ID]
		if !ok {
			rr = contract.RuleResult{Result: contract.ResultPass}
			for _, p := range rule.Parts {
				rr.Parts = append(rr.Parts, contract.PartOutcome{Kind: p.Kind, Result: contract.ResultPass})
			}
		}
		rr.Rule = rule
		rep.Rules = append(rep.Rules, rr)
	}
	return rep
}

func part(kind contract.PartKind, result contract.Result, findings ...contract.Finding) contract.PartOutcome {
	return contract.PartOutcome{Kind: kind, Result: result, Findings: findings}
}

func outcome(result contract.Result, parts ...contract.PartOutcome) contract.RuleResult {
	return contract.RuleResult{Result: result, Parts: parts}
}

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func assertText(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("text report differs\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func renderText(t *testing.T, rep *contract.Report, version string) string {
	t.Helper()
	var b strings.Builder
	if err := writeContractText(&b, rep, version); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The report spec 0052 shows under "Report (#100)", rendered from the
// report model. Two differences from that illustrative sample are
// deliberate: the R category has spec 0051's long name, "Removability
// and repository hygiene", which widens the name column; and the
// "Cannot decide" section is always printed.
func TestContractTextReportSpecSample(t *testing.T) {
	notRun := func(kind contract.PartKind) contract.PartOutcome { return part(kind, contract.ResultNotRun) }
	pass := func(kind contract.PartKind) contract.PartOutcome { return part(kind, contract.ResultPass) }
	waived := outcome(contract.ResultWaived, part(contract.Static, contract.ResultWaived))
	waived.Waiver = &contract.WaiverStatus{Reason: "lint is platform-independent", Until: day(t, "2026-12-31")}

	rep := reportOf(t, true, map[string]contract.RuleResult{
		"V-01": outcome(contract.ResultNotRun, notRun(contract.Execution), notRun(contract.Probe)),
		"V-02": outcome(contract.ResultNotRun, notRun(contract.Probe)),
		"V-03": outcome(contract.ResultNotRun, notRun(contract.Execution)),
		"H-02": outcome(contract.ResultFail, part(contract.Static, contract.ResultFail, contract.Finding{
			File:    "lefthook.yml",
			Message: "pre-push calls neither verify-fast (task verify:fast) nor verify.",
			Fix:     "make pre-push run `task verify:fast`.",
		})),
		"C-05": waived,
		"A-01": outcome(contract.ResultFail, part(contract.Static, contract.ResultFail, contract.Finding{
			File:    ".claude/settings.json",
			Message: "no PreToolUse hook matches MultiEdit.",
			Fix:     "add MultiEdit to the matcher of the hook that runs the guard.",
		})),
		"A-02": outcome(contract.ResultNotRun, notRun(contract.Probe)),
		"A-03": outcome(contract.ResultNotRun, notRun(contract.Probe)),
		"A-05": outcome(contract.ResultNotRun, pass(contract.Static), notRun(contract.Probe)),
		"A-07": outcome(contract.ResultNotRun, pass(contract.Static), notRun(contract.Execution)),
		"A-08": outcome(contract.ResultNotRun, notRun(contract.Probe)),
		"D-01": outcome(contract.ResultNotRun, notRun(contract.Execution)),
		"R-01": outcome(contract.ResultNotRun, pass(contract.Static), notRun(contract.Execution)),
	})
	rep.Verdict = contract.VerdictUndecided
	rep.Reason = "10 rules did not run (--static); 1 MUST rule already fails"
	rep.LocalAdjustments = []contract.LocalAdjustment{{
		File: ".claude/hooks/policy.local.json", Kind: "exception", Rule: "git-push-force",
		Pattern: "^git push --force origin feat/", Reason: "rebasing feature branches",
	}}

	assertText(t, renderText(t, rep, "v0.12.0"), `vibe audit: contract 1.0.0 (vibe v0.12.0), static checks only

  V  Verification entrypoints              1 pass, 3 not run
  H  Local Git hooks                       1 pass, 1 fail
  C  CI                                    4 pass, 1 waived
  A  Agent guardrails                      2 pass, 1 FAIL, 5 not run
  D  Dependencies and supply chain         2 pass, 1 not run
  R  Removability and repository hygiene   2 pass, 1 not run

Failed
  A-01  MUST    Shell commands and file edits pass through the guard
        .claude/settings.json: no PreToolUse hook matches MultiEdit.
        Fix: add MultiEdit to the matcher of the hook that runs the guard.
  H-02  SHOULD  pre-commit checks formatting; pre-push runs verification
        lefthook.yml: pre-push calls neither verify-fast (task verify:fast) nor verify.
        Fix: make pre-push run `+"`task verify:fast`"+`.

Waived
  C-05  SHOULD  until 2026-12-31: lint is platform-independent

Not applicable
  (none)

Cannot decide
  (none)

Not run (--static)
  V-01 V-02 V-03 A-02 A-03 A-05 (probe) A-07 (execution) A-08 D-01 R-01 (execution)

Guard exceptions
  (none)

Local guardrail adjustments
  .claude/hooks/policy.local.json: exception git-push-force "^git push --force origin feat/" (rebasing feature branches)

Verdict: cannot decide. 10 rules did not run (--static); 1 MUST rule already fails.
`)
}

// Every kind of entry the report has, on a full audit (no --static).
func TestContractTextReportEverySection(t *testing.T) {
	notApplicable := func(why string) contract.RuleResult {
		rr := outcome(contract.ResultNotApplicable, part(contract.Static, contract.ResultNotApplicable,
			contract.Finding{File: "vibe.yaml", Message: "not applicable: " + why}))
		rr.NotApplicable = why
		return rr
	}
	results := map[string]contract.RuleResult{
		"V-01": outcome(contract.ResultUndecided,
			part(contract.Execution, contract.ResultUndecided, contract.Finding{Message: "the execution check is not implemented in this vibe; #104 adds it"}),
			part(contract.Probe, contract.ResultUndecided, contract.Finding{Message: "the probe check is not implemented in this vibe; #104 adds it"})),
		"V-04": outcome(contract.ResultFail, part(contract.Static, contract.ResultFail,
			contract.Finding{File: "vibe.yaml", Message: "entrypoints.fmt is not declared", Fix: "declare entrypoints.fmt"})),
		"R-01": outcome(contract.ResultFail,
			part(contract.Static, contract.ResultFail,
				contract.Finding{File: ".github/workflows/ci.yml", Message: "job build calls vibe sync", Fix: "call the tool vibe sync runs"},
				contract.Finding{File: "lefthook.yml", Message: "pre-push calls vibe audit", Fix: "remove it"}),
			part(contract.Execution, contract.ResultUndecided, contract.Finding{Message: "the execution check is not implemented in this vibe; #102 adds it"})),
		"R-02": outcome(contract.ResultFail, part(contract.Static, contract.ResultFail,
			contract.Finding{File: "AGENTS.md", Message: `does not name the verify command "task verify"`, Fix: "tell agents in AGENTS.md to run `task verify`"})),
		"C-05": notApplicable("platforms does not include windows"),
	}
	results["R-02"] = withWaiver(results["R-02"], &contract.WaiverStatus{Reason: "rewriting AGENTS.md", Until: day(t, "2026-01-31"), Expired: true})
	results["C-05"] = withWaiver(results["C-05"], &contract.WaiverStatus{Reason: "lint is platform-independent", Until: day(t, "2026-12-31"), Unused: true})
	for _, id := range []string{"A-01", "A-02", "A-03", "A-04", "A-05", "A-06", "A-07", "A-08"} {
		results[id] = notApplicable("agents does not include claude")
	}
	rep := reportOf(t, false, results)
	rep.Contract.Declared = "1.0.3"
	rep.Verdict = contract.VerdictUndecided
	rep.Reason = "1 rule cannot be decided; 2 MUST rules already fail"
	rep.GuardExceptions = []contract.GuardException{{Rule: "git-push-force", Match: "git push --force origin feat/", Reason: "rebasing feature branches"}}
	rep.LocalAdjustments = []contract.LocalAdjustment{{File: ".claude/hooks/policy.local.json", Kind: "rule", Rule: "no-curl", Pattern: `\bcurl\b`}}
	rep.Notes = []string{"no origin/HEAD in this clone; assumed the default branch is main"}

	assertText(t, renderText(t, rep, "test"), `vibe audit: contract 1.0.0 (declared 1.0.3; vibe test)

  V  Verification entrypoints              2 pass, 1 FAIL, 1 cannot decide
  H  Local Git hooks                       2 pass
  C  CI                                    4 pass, 1 not applicable
  A  Agent guardrails                      8 not applicable
  D  Dependencies and supply chain         3 pass
  R  Removability and repository hygiene   1 pass, 1 FAIL, 1 fail

Failed
  V-04  MUST    The entrypoints are declared and used
        vibe.yaml: entrypoints.fmt is not declared
        Fix: declare entrypoints.fmt
  R-01  MUST    Everything but the audit works without vibe
        .github/workflows/ci.yml: job build calls vibe sync
        Fix: call the tool vibe sync runs
        lefthook.yml: pre-push calls vibe audit
        Fix: remove it
  R-02  SHOULD  Agents are told the verification entrypoints
        AGENTS.md: does not name the verify command "task verify"
        Fix: tell agents in AGENTS.md to run `+"`task verify`"+`

Waived
  C-05  SHOULD  waiver unused (not applicable) until 2026-12-31: lint is platform-independent
  R-02  SHOULD  waiver expired 2026-01-31: rewriting AGENTS.md (checked: fail)

Not applicable
  C-05  SHOULD  platforms does not include windows
  A-01  MUST    agents does not include claude
  A-02  MUST    agents does not include claude
  A-03  MUST    agents does not include claude
  A-04  MUST    agents does not include claude
  A-05  MUST    agents does not include claude
  A-06  SHOULD  agents does not include claude
  A-07  SHOULD  agents does not include claude
  A-08  MUST    agents does not include claude

Cannot decide
  V-01  MUST    `+"`verify`"+` covers formatting, static analysis and tests
        the execution check is not implemented in this vibe; #104 adds it
        the probe check is not implemented in this vibe; #104 adds it

Guard exceptions
  git-push-force "git push --force origin feat/" (rebasing feature branches)

Local guardrail adjustments
  .claude/hooks/policy.local.json: extra rule no-curl "\bcurl\b"

Note: no origin/HEAD in this clone; assumed the default branch is main

Verdict: cannot decide. 1 rule cannot be decided; 2 MUST rules already fail.
`)
}

func withWaiver(rr contract.RuleResult, w *contract.WaiverStatus) contract.RuleResult {
	rr.Waiver = w
	return rr
}

func TestContractTextReportVerdicts(t *testing.T) {
	for _, tc := range []struct {
		verdict contract.Verdict
		want    string
	}{
		{contract.VerdictConformant, "\nVerdict: conformant\n"},
		{contract.VerdictNotConformant, "\nVerdict: not conformant\n"},
	} {
		rep := reportOf(t, false, nil)
		rep.Verdict = tc.verdict
		if got := renderText(t, rep, "test"); !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s report ends:\n%s\nwant it to end with %q", tc.verdict, got, tc.want)
		}
	}
}
