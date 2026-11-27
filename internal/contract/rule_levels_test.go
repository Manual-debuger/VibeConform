package contract_test

import (
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// A rule's level depends on the audited contract (spec 0074, "A rule's
// level depends on the audited contract"): a rule that is MUST and
// becomes SHOULD in 2.1.0 counts in the verdict under 1.2.0 and does not
// under 2.1.0. The stand-in rule fails, so only its level decides the
// verdict.
func TestRuleLevelFollowsTheAuditedContract(t *testing.T) {
	t.Parallel()
	relaxed := contract.Rule{
		ID: "T-01", Level: contract.Must, Category: "V", Title: "a stand-in rule that SHOULD since 2.1.0",
		LevelChanges: []contract.LevelChange{{Since: "2.1.0", Level: contract.Should}},
		Parts: []contract.Part{{Kind: contract.Static, Run: func(*contract.Env) contract.PartResult {
			return contract.Fail(contract.Finding{File: "vibe.yaml", Message: "stand-in failure", Fix: "none"})
		}}},
	}
	for _, tc := range []struct {
		declared string
		level    contract.Level
		verdict  contract.Verdict
		failing  string
	}{
		{declared: "1.0.0", level: contract.Must, verdict: contract.VerdictNotConformant, failing: "1 MUST rule fails"},
		{declared: "2.0.0", level: contract.Should, verdict: contract.VerdictConformant},
		{declared: "2.1.0", level: contract.Should, verdict: contract.VerdictConformant},
	} {
		t.Run(tc.declared, func(t *testing.T) {
			t.Parallel()
			dir := contracttest.Fixture(t, contracttest.Baseline,
				contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: " + tc.declared + "\n"})))
			rep := contracttest.Audit(t, dir, contract.Options{Static: true, Rules: []contract.Rule{relaxed}})
			if len(rep.Rules) != 1 {
				t.Fatalf("report has %d rules, want 1", len(rep.Rules))
			}
			if got := rep.Rules[0].Level; got != tc.level {
				t.Errorf("T-01 level under %s = %s, want %s", rep.Contract.Audited, got, tc.level)
			}
			if got := rep.Count(tc.level, contract.ResultFail); got != 1 {
				t.Errorf("Count(%s, fail) = %d, want 1", tc.level, got)
			}
			engVerdict(t, rep, tc.verdict, tc.failing)
		})
	}
}

// LevelAt reads a rule's level history: the level it was added with,
// until the first change whose contract is not newer than the one asked.
func TestRuleLevelAt(t *testing.T) {
	t.Parallel()
	r := contract.Rule{ID: "T-01", Level: contract.Must, LevelChanges: []contract.LevelChange{{Since: "2.1.0", Level: contract.Should}}}
	for v, want := range map[string]contract.Level{
		"1.2.0":  contract.Must,
		"2.0.0":  contract.Must,
		"2.1.0":  contract.Should,
		"2.1.3":  contract.Should,
		"10.0.0": contract.Should,
	} {
		if got := r.LevelAt(v); got != want {
			t.Errorf("LevelAt(%s) = %s, want %s", v, got, want)
		}
	}
	if r.WaivableAt("1.2.0") || !r.WaivableAt("2.1.0") {
		t.Errorf("WaivableAt(1.2.0) = %v, WaivableAt(2.1.0) = %v; want false, true", r.WaivableAt("1.2.0"), r.WaivableAt("2.1.0"))
	}
}
