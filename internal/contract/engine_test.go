package contract_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The engine's tests read the verdict, its reason and the rules' results
// from the report model. The CLI maps a verdict to its exit code and
// stderr line (TestVerdictErrorExitCode in internal/cli): conformant
// exits 0, not conformant exits 2 with "not conformant: " and
// FailingReason, cannot decide exits 1 with "cannot decide: " and
// Reason.

// engAudit runs the full audit of dir with rules in place of the registry
// (nil for the registry). The runs of the execution and probe parts find
// no program, so that a fixture's result does not depend on what this
// machine has installed.
func engAudit(t *testing.T, dir string, rules []contract.Rule) *contract.Report {
	t.Helper()
	return contracttest.Audit(t, dir, contract.Options{Rules: rules, Probe: contract.ProbeOptions{Env: contracttest.EmptyProbeEnv(t)}})
}

// engAuditStatic runs the static audit of dir with the registry.
func engAuditStatic(t *testing.T, dir string) *contract.Report {
	t.Helper()
	return contracttest.Audit(t, dir, contract.Options{Static: true})
}

// engVerdict checks a report's verdict and, for not conformant, which
// binding rules fail (the CLI's stderr line after "not conformant: ").
func engVerdict(t *testing.T, rep *contract.Report, want contract.Verdict, failing string) {
	t.Helper()
	if rep.Verdict != want || rep.FailingReason() != failing {
		t.Errorf("verdict = %s (%q, failing %q), want %s failing %q", rep.Verdict, rep.Reason, rep.FailingReason(), want, failing)
	}
}

func engWaiverConfig(rule, until string) contracttest.Edit {
	return contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "waivers",
		Body: "waivers:\n  - rule: " + rule + "\n    reason: known gap\n    until: " + until + "\n"}))
}

// A repository on contract 1.1.0 keeps its verdict under a vibe that
// implements 1.2.0, even when every rule 1.2.0 added fails (spec 0061,
// acceptance criterion 3; user story 9).
func TestContractAuditOlderMinorKeepsVerdictWhenNewRulesFail(t *testing.T) {
	t.Parallel()
	added := []string{"V-05", "V-06", "V-07", "V-08", "A-09"}
	rules := contracttest.StandIn(nil, contract.ResultPass, contracttest.RuleIDs(contract.Must, "V-04")...)
	rules = contracttest.StandIn(rules, contract.ResultFail, added...)
	dir := contracttest.Fixture(t, contracttest.Baseline,
		contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.1.0\n"})))
	rep := engAudit(t, dir, rules)
	if rep.Verdict != contract.VerdictConformant || rep.Contract.Declared != "1.1.0" || rep.Contract.Audited != "1.2.0" {
		t.Errorf("verdict = %s, contract = %+v, want conformant, 1.1.0 declared, 1.2.0 audited", rep.Verdict, rep.Contract)
	}
	// V-05 is not applicable: the fixture has no Python component.
	for _, id := range added[1:] {
		contracttest.AssertRule(t, rep, id, contract.ResultFail)
	}
}

func TestContractAuditVerdicts(t *testing.T) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	t.Parallel()
	t.Run("conformant exits 0", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(nil, contract.ResultPass, contracttest.RuleIDs(contract.Must, "V-04")...)
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline), rules)
		engVerdict(t, rep, contract.VerdictConformant, "")
		if rep.Reason != "" {
			t.Errorf("reason = %q, want none", rep.Reason)
		}
	})
	t.Run("a failed SHOULD rule does not change it", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(nil, contract.ResultPass, contracttest.RuleIDs(contract.Must, "V-04")...)
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline, contracttest.WithoutFile(".github/pull_request_template.md")), rules)
		engVerdict(t, rep, contract.VerdictConformant, "")
		contracttest.AssertRule(t, rep, "R-03", contract.ResultFail)
	})
	t.Run("an undecided SHOULD rule does not change it", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(nil, contract.ResultPass, contracttest.RuleIDs(contract.Must, "V-04")...)
		rules = contracttest.StandIn(rules, contract.ResultUndecided, "H-01")
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline), rules)
		engVerdict(t, rep, contract.VerdictConformant, "")
		contracttest.AssertRule(t, rep, "H-01", contract.ResultUndecided)
	})
	t.Run("a failed MUST rule exits 2", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(nil, contract.ResultPass, contracttest.RuleIDs(contract.Must, "V-04")...)
		dir := contracttest.Fixture(t, contracttest.Baseline,
			contracttest.Entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n"))
		rep := engAudit(t, dir, rules)
		engVerdict(t, rep, contract.VerdictNotConformant, "1 MUST rule fails")
		if got := rep.Count(contract.Must, contract.ResultFail); got != 1 {
			t.Errorf("summary MUST fail = %d, want 1", got)
		}
	})
	t.Run("cannot decide exits 1", func(t *testing.T) {
		t.Parallel()
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline), nil)
		if rep.Verdict != contract.VerdictUndecided || rep.Static || !strings.Contains(rep.Reason, "rules cannot be decided") {
			t.Errorf("verdict = %s (%q), static %v; want undecided, rules cannot be decided, not static", rep.Verdict, rep.Reason, rep.Static)
		}
		contracttest.AssertRule(t, rep, "V-01", contract.ResultUndecided, "`task` is not installed on this machine")
		contracttest.AssertRule(t, rep, "A-08", contract.ResultUndecided, "no bash found")
		contracttest.AssertRule(t, rep, "R-01", contract.ResultUndecided, "`task` is not installed on this machine")
	})
	t.Run("a failed MUST rule beside an undecided one exits 1", func(t *testing.T) {
		t.Parallel()
		// Without fmt V-04 fails; fmt-check, which V-03 needs, stays.
		dir := contracttest.Fixture(t, contracttest.Baseline,
			contracttest.Entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n"))
		rep := engAudit(t, dir, nil)
		contracttest.AssertRule(t, rep, "V-04", contract.ResultFail)
		if rep.Verdict != contract.VerdictUndecided || !strings.Contains(rep.Reason, "1 MUST rule already fails") {
			t.Errorf("verdict = %s (%q), want undecided saying 1 MUST rule already fails", rep.Verdict, rep.Reason)
		}
	})
	t.Run("--static never reaches a verdict", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(nil, contract.ResultPass, contracttest.RuleIDs("", "V-04", "R-02", "R-03")...)
		rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline), contract.Options{Static: true, Rules: rules})
		// Every rule with an execution or probe part does not run, unless
		// it is not applicable: V-05 has no Python component here.
		contracttest.AssertRule(t, rep, "V-05", contract.ResultNotApplicable)
		if !rep.Static || rep.Verdict != contract.VerdictUndecided || !strings.Contains(rep.Reason, "14 rules did not run (--static)") {
			t.Errorf("static %v, verdict %s (%q); want undecided, 14 rules did not run", rep.Static, rep.Verdict, rep.Reason)
		}
		wantNotRun := []string{"V-01", "V-02", "V-03", "V-06", "V-07", "V-08", "A-02", "A-03", "A-05", "A-07", "A-08", "A-09", "D-01", "R-01"}
		var notRun []string
		for _, rr := range rep.Rules {
			if rr.Result == contract.ResultNotRun {
				notRun = append(notRun, rr.Rule.ID)
			}
		}
		if !slices.Equal(notRun, wantNotRun) {
			t.Errorf("rules not run under --static: %v, want %v", notRun, wantNotRun)
		}
		a05 := contracttest.RuleResult(t, rep, "A-05")
		if p := a05.Parts; len(p) != 2 || p[0].Kind != contract.Static || p[0].Result != contract.ResultPass ||
			p[1].Kind != contract.Probe || p[1].Result != contract.ResultNotRun {
			t.Errorf("A-05 parts = %+v, want static pass and probe not_run", p)
		}
	})
}

func engRequireConfig(ids string, more ...contracttest.Section) contracttest.Edit {
	return contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(append(more, contracttest.Section{Key: "require", Body: "require: [" + ids + "]\n"})...))
}

// A required SHOULD rule counts in the verdict like a MUST rule (spec
// 0063, "A repository can require SHOULD rules").
func TestContractAuditRequiredRules(t *testing.T) {
	t.Parallel()
	// Every rule decides, so that the reason counts only the rules named.
	passing := func() []contract.Rule {
		return contracttest.StandIn(nil, contract.ResultPass, contracttest.RuleIDs("", "V-04", "R-02", "R-03")...)
	}
	required := func(t *testing.T, rep *contract.Report, id string, want bool) {
		t.Helper()
		if got := contracttest.RuleResult(t, rep, id).Required; got != want {
			t.Errorf("%s required = %v, want %v", id, got, want)
		}
	}
	t.Run("a failing required rule exits 2", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(passing(), contract.ResultFail, "V-06")
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline, engRequireConfig("V-06")), rules)
		engVerdict(t, rep, contract.VerdictNotConformant, "1 required SHOULD rule fails")
		required(t, rep, "V-06", true)
		required(t, rep, "V-07", false)
		// MUST rules are not listed in require.
		required(t, rep, "C-02", false)
	})
	t.Run("MUST and required failures are counted apart", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(passing(), contract.ResultFail, "V-06", "V-07")
		dir := contracttest.Fixture(t, contracttest.Baseline, engRequireConfig("V-06, V-07",
			contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n"}))
		rep := engAudit(t, dir, rules)
		engVerdict(t, rep, contract.VerdictNotConformant, "1 MUST rule fails; 2 required SHOULD rules fail")
	})
	t.Run("an undecided required rule exits 1", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(passing(), contract.ResultUndecided, "V-06")
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline, engRequireConfig("V-06")), rules)
		if rep.Verdict != contract.VerdictUndecided || !strings.Contains(rep.Reason, "1 rule cannot be decided") {
			t.Errorf("verdict = %s (%q), want undecided, 1 rule cannot be decided", rep.Verdict, rep.Reason)
		}
	})
	t.Run("a required rule that already fails is named in the reason", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(passing(), contract.ResultFail, "V-06")
		rules = contracttest.StandIn(rules, contract.ResultUndecided, "V-01")
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline, engRequireConfig("V-06")), rules)
		if want := "1 rule cannot be decided; 1 required SHOULD rule already fails"; rep.Verdict != contract.VerdictUndecided || rep.Reason != want {
			t.Errorf("verdict = %s, reason = %q, want undecided, %q", rep.Verdict, rep.Reason, want)
		}
	})
	t.Run("a required rule that is not applicable is conformant", func(t *testing.T) {
		t.Parallel()
		// V-05 is not applicable: the fixture has no Python component.
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline, engRequireConfig("V-05")), passing())
		contracttest.AssertRule(t, rep, "V-05", contract.ResultNotApplicable)
		engVerdict(t, rep, contract.VerdictConformant, "")
	})
	t.Run("a failing SHOULD rule that is not required is conformant", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(passing(), contract.ResultFail, "V-07")
		rules = contracttest.StandIn(rules, contract.ResultPass, "V-06")
		rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline, engRequireConfig("V-06")), rules)
		contracttest.AssertRule(t, rep, "V-07", contract.ResultFail)
		engVerdict(t, rep, contract.VerdictConformant, "")
	})
}

func TestContractAuditApplicabilityAndWaivers(t *testing.T) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	t.Parallel()
	t.Run("C-05 not applicable without windows", func(t *testing.T) {
		t.Parallel()
		rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline,
			contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: [linux]\n"}))))
		contracttest.AssertRule(t, rep, "C-05", contract.ResultNotApplicable, "platforms does not include windows")
		if p := contracttest.RuleResult(t, rep, "C-05").Parts; len(p) != 1 || p[0].Result != contract.ResultNotApplicable {
			t.Errorf("C-05 parts = %+v, want one not_applicable part", p)
		}
	})
	// The report carries each rule as it reads under the audited contract
	// (spec 0074): C-05's title and applicability change in 2.1.0.
	t.Run("C-05 as the audited contract states it", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ declared, title string }{
			{"1.2.0", "Tests run on Linux and Windows"},
			{"2.0.0", "Tests run on every declared platform"},
		} {
			rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline,
				contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: " + tc.declared + "\n"}))))
			if got := contracttest.RuleResult(t, rep, "C-05").Rule.Title; got != tc.title {
				t.Errorf("contract %s: C-05 title = %q, want %q", tc.declared, got, tc.title)
			}
		}
	})
	t.Run("A rules not applicable without claude", func(t *testing.T) {
		t.Parallel()
		rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline, contracttest.AgentsConfig("[codex]")))
		for _, id := range []string{"A-01", "A-02", "A-03", "A-04", "A-05", "A-06", "A-07", "A-08"} {
			contracttest.AssertRule(t, rep, id, contract.ResultNotApplicable, "agents does not include claude")
		}
	})
	t.Run("waived", func(t *testing.T) {
		t.Parallel()
		rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline,
			engWaiverConfig("R-03", "2999-12-31"), contracttest.WithoutFile(".github/pull_request_template.md")))
		contracttest.AssertRule(t, rep, "R-03", contract.ResultWaived)
		w := contracttest.RuleResult(t, rep, "R-03").Waiver
		if w == nil || w.Reason != "known gap" || w.Until.Format(time.DateOnly) != "2999-12-31" || w.Expired {
			t.Errorf("R-03 waiver = %+v, want in force until 2999-12-31", w)
		}
		if w != nil && w.Unused {
			t.Errorf("R-03 waiver unused = true, want false: the waiver applied")
		}
		if got := rep.Count(contract.Should, contract.ResultWaived); got != 1 {
			t.Errorf("summary SHOULD waived = %d, want 1", got)
		}
	})
	t.Run("expired waiver is checked again", func(t *testing.T) {
		t.Parallel()
		rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline,
			engWaiverConfig("R-03", "2000-01-01"), contracttest.WithoutFile(".github/pull_request_template.md")))
		contracttest.AssertRule(t, rep, "R-03", contract.ResultFail)
		if w := contracttest.RuleResult(t, rep, "R-03").Waiver; w == nil || !w.Expired {
			t.Errorf("R-03 waiver = %+v, want expired", w)
		}
		rep = engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline, engWaiverConfig("R-03", "2000-01-01")))
		contracttest.AssertRule(t, rep, "R-03", contract.ResultPass)
	})
	t.Run("waiver for a rule that is not applicable is unused", func(t *testing.T) {
		t.Parallel()
		// The JSON key and the text report's "waiver unused" are the
		// CLI's: TestContractJSONReportFromTheModel and
		// TestContractTextReportEverySection in internal/cli.
		rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(
			contracttest.Section{Key: "platforms", Body: "platforms: [linux]\n"},
			contracttest.Section{Key: "waivers", Body: "waivers:\n  - rule: C-05\n    reason: known gap\n    until: 2999-12-31\n"}))))
		contracttest.AssertRule(t, rep, "C-05", contract.ResultNotApplicable)
		if w := contracttest.RuleResult(t, rep, "C-05").Waiver; w == nil || w.Expired || !w.Unused {
			t.Errorf("C-05 waiver = %+v, want the unused waiver listed with unused true", w)
		}
	})
	t.Run("newer PATCH audited as 1.2.0", func(t *testing.T) {
		t.Parallel()
		rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline,
			contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.2.7\n"}))))
		if rep.Contract.Declared != "1.2.7" || rep.Contract.Audited != "1.2.0" {
			t.Errorf("contract = %+v, want 1.2.7 declared, 1.2.0 audited", rep.Contract)
		}
	})
	// A vibe audits every repository that declares its MAJOR with the
	// latest MINOR it implements (spec 0051, "Versioning"; spec 0055;
	// spec 0061, "Contract version").
	for _, declared := range []string{"1.0.0", "1.0.1", "1.1.0", "1.1.3"} {
		t.Run("older MINOR "+declared+" audited as 1.2.0", func(t *testing.T) {
			t.Parallel()
			rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline,
				contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: " + declared + "\n"}))))
			if rep.Contract.Declared != declared || rep.Contract.Audited != "1.2.0" {
				t.Errorf("contract = %+v, want %s declared, 1.2.0 audited", rep.Contract, declared)
			}
		})
	}
}

// A vibe implements contracts 1.2.0 and 2.1.0 side by side: a declared
// 1.y.z is audited as 1.2.0 and a declared 2.0.z or 2.1.z as 2.1.0 (spec
// 0072, "Contract versions"; spec 0074, "Contract versions").
func TestContractAuditedVersion(t *testing.T) {
	t.Parallel()
	for declared, audited := range map[string]string{
		"1.0.0": "1.2.0",
		"1.2.0": "1.2.0",
		"1.2.5": "1.2.0",
		"2.0.0": "2.1.0",
		"2.0.3": "2.1.0",
		"2.1.0": "2.1.0",
		"2.1.4": "2.1.0",
	} {
		t.Run(declared, func(t *testing.T) {
			t.Parallel()
			rep := engAuditStatic(t, contracttest.Fixture(t, contracttest.Baseline,
				contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: " + declared + "\n"}))))
			if rep.Contract.Declared != declared || rep.Contract.Audited != audited {
				t.Errorf("contract = %+v, want %s declared, %s audited", rep.Contract, declared, audited)
			}
		})
	}
}

// The schema and the running vibe's version are the JSON report's
// (TestContractJSONReportFromTheModel in internal/cli).
func TestContractAuditReportsEveryRule(t *testing.T) {
	t.Parallel()
	rep := engAudit(t, contracttest.Fixture(t, contracttest.Baseline), nil)
	if rep.Verdict != contract.VerdictUndecided {
		t.Errorf("verdict = %s, want undecided", rep.Verdict)
	}
	if rep.Contract.Declared != "1.2.0" || rep.Contract.Audited != "1.2.0" {
		t.Errorf("contract = %+v, want 1.2.0 declared and audited", rep.Contract)
	}
	want := []string{
		"V-01", "V-02", "V-03", "V-04", "V-05", "V-06", "V-07", "V-08", "H-01", "H-02", "C-01", "C-02", "C-03", "C-04", "C-05",
		"A-01", "A-02", "A-03", "A-04", "A-05", "A-06", "A-07", "A-08", "A-09", "D-01", "D-02", "D-03",
		"R-01", "R-02", "R-03",
	}
	var got []string
	for _, rr := range rep.Rules {
		got = append(got, rr.Rule.ID)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("rules = %v, want every rule in ID order", got)
	}
	c02 := contracttest.RuleResult(t, rep, "C-02").Rule
	if c02.Level != contract.Must || c02.Category != "C" || c02.Title != "Toolchains and actions are pinned" {
		t.Errorf("C-02 = %s %s %q", c02.Level, c02.Category, c02.Title)
	}
}
