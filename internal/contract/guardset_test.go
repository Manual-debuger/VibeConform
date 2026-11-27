package contract_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// guardSetRequire is the require field that lists the guard set.
const guardSetRequire = "require: [A-01, A-02, A-03, A-04, A-08]\n"

// unguardedSettings is the baseline's .claude/settings.json without its
// PreToolUse entry: the Stop, PostToolUse and SessionStart hooks stay,
// so A-05 to A-07 can still pass.
const unguardedSettings = `{
  "hooks": {
    "SessionStart": [
      {"matcher": "startup|resume|clear", "hooks": [{"type": "command", "command": "task -x hook:context"}]}
    ],
    "PostToolUse": [
      {"matcher": "Write|Edit|MultiEdit", "hooks": [{"type": "command", "command": "task fmt"}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "task verify:fast 1>&2 || exit 2"}]}
    ]
  }
}
`

// guardSetConfig is the baseline vibe.yaml declaring contract version,
// with require when it is not empty.
func guardSetConfig(version, require string) contracttest.Edit {
	sections := []contracttest.Section{{Key: "contract", Body: "contract: " + version + "\n"}}
	if require != "" {
		sections = append(sections, contracttest.Section{Key: "require", Body: require})
	}
	return contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(sections...))
}

// guardSetRules is the registry with every rule outside the guard set
// and A-09 standing in as passing, except V-04, R-02 and R-03, which the
// baseline passes as it is. The guard set and A-09 run their own checks.
func guardSetRules() []contract.Rule {
	var ids []string
	for _, id := range contracttest.RuleIDs("", "V-04", "R-02", "R-03", "A-09") {
		if !slices.Contains(contract.GuardSet(), id) {
			ids = append(ids, id)
		}
	}
	return contracttest.StandIn(nil, contract.ResultPass, ids...)
}

// Under contract 2.1.0 the guard set is SHOULD: a repository with no
// guard is conformant unless it requires the guard set. Under 1.2.0 it
// is MUST (spec 0074, "Testing Decisions": guard set levels).
func TestGuardSetVerdicts(t *testing.T) {
	t.Parallel()
	unguarded := func(t *testing.T, version, require string) *contract.Report {
		t.Helper()
		dir := contracttest.Fixture(t, contracttest.Baseline, guardSetConfig(version, require),
			contracttest.WithFile(".claude/settings.json", unguardedSettings))
		return engAudit(t, dir, guardSetRules())
	}
	level := func(t *testing.T, rep *contract.Report, id string, want contract.Level, required bool) {
		t.Helper()
		rr := contracttest.RuleResult(t, rep, id)
		if rr.Level != want || rr.Required != required {
			t.Errorf("%s level = %s, required %v; want %s, required %v", id, rr.Level, rr.Required, want, required)
		}
	}
	t.Run("2.1.0 with no guard and no require is conformant", func(t *testing.T) {
		t.Parallel()
		rep := unguarded(t, "2.1.0", "")
		contracttest.AssertRule(t, rep, "A-01", contract.ResultFail, "no PreToolUse command hook matches")
		for _, id := range contract.GuardSet() {
			level(t, rep, id, contract.Should, false)
		}
		engVerdict(t, rep, contract.VerdictConformant, "")
	})
	t.Run("2.0.0 is audited as 2.1.0", func(t *testing.T) {
		t.Parallel()
		rep := unguarded(t, "2.0.0", "")
		level(t, rep, "A-01", contract.Should, false)
		engVerdict(t, rep, contract.VerdictConformant, "")
	})
	t.Run("2.1.0 with no guard and the guard set required is not conformant", func(t *testing.T) {
		t.Parallel()
		rep := unguarded(t, "2.1.0", guardSetRequire)
		contracttest.AssertRule(t, rep, "A-01", contract.ResultFail, "no PreToolUse command hook matches")
		for _, id := range contract.GuardSet() {
			level(t, rep, id, contract.Should, true)
		}
		if rep.Verdict == contract.VerdictConformant || !strings.Contains(rep.Reason+rep.FailingReason(), "required SHOULD rule") {
			t.Errorf("verdict = %s (%q, failing %q), want no conformant verdict, with required SHOULD rules failing",
				rep.Verdict, rep.Reason, rep.FailingReason())
		}
	})
	t.Run("1.2.0 with no guard is not conformant", func(t *testing.T) {
		t.Parallel()
		rep := unguarded(t, "1.2.0", "")
		contracttest.AssertRule(t, rep, "A-01", contract.ResultFail, "no PreToolUse command hook matches")
		for _, id := range contract.GuardSet() {
			level(t, rep, id, contract.Must, false)
		}
		if rep.Verdict == contract.VerdictConformant || !strings.Contains(rep.Reason+rep.FailingReason(), "MUST rule") {
			t.Errorf("verdict = %s (%q, failing %q), want no conformant verdict, with MUST rules failing",
				rep.Verdict, rep.Reason, rep.FailingReason())
		}
	})
}

// A repository that requires the guard set under 2.1.0 gets the verdict
// it had when the guard set was MUST (spec 0074, user story 21).
func TestGuardSetRequiredKeepsTheVerdict(t *testing.T) {
	t.Parallel()
	guarded := func(t *testing.T, version, require string, a02 contract.Result) *contract.Report {
		t.Helper()
		rules := contracttest.StandIn(guardSetRules(), contract.ResultPass, append(contract.GuardSet(), "A-09")...)
		rules = contracttest.StandIn(rules, a02, "A-02")
		return engAudit(t, contracttest.Fixture(t, contracttest.Baseline, guardSetConfig(version, require)), rules)
	}
	for _, tc := range []struct {
		name    string
		a02     contract.Result
		verdict contract.Verdict
		v1, v2  string
	}{
		{"a passing guard", contract.ResultPass, contract.VerdictConformant, "", ""},
		{"a failing guard", contract.ResultFail, contract.VerdictNotConformant, "1 MUST rule fails", "1 required SHOULD rule fails"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			engVerdict(t, guarded(t, "1.2.0", "", tc.a02), tc.verdict, tc.v1)
			engVerdict(t, guarded(t, "2.1.0", guardSetRequire, tc.a02), tc.verdict, tc.v2)
		})
	}
	t.Run("a failing guard that is not required", func(t *testing.T) {
		t.Parallel()
		rep := guarded(t, "2.1.0", "", contract.ResultFail)
		contracttest.AssertRule(t, rep, "A-02", contract.ResultFail)
		engVerdict(t, rep, contract.VerdictConformant, "")
	})
	t.Run("a waived guard rule", func(t *testing.T) {
		t.Parallel()
		rules := contracttest.StandIn(guardSetRules(), contract.ResultFail, "A-02")
		dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(
			contracttest.Section{Key: "contract", Body: "contract: 2.1.0\n"},
			contracttest.Section{Key: "waivers", Body: "waivers:\n  - rule: A-02\n    reason: guard under review\n    until: 2999-12-31\n"})))
		rep := engAudit(t, dir, rules)
		contracttest.AssertRule(t, rep, "A-02", contract.ResultWaived)
	})
}

// R-01's guard half holds the A-02 deny cases to the guard only where the
// guard binds the verdict: under 1.x, or under 2.1.0 when the guard set
// is required. Otherwise a MUST rule would require the guard that the
// guard set leaves to the adopter (spec 0074).
func TestGuardSetR01GuardHalf(t *testing.T) {
	t.Parallel()
	const noHook = "no PreToolUse hook matches Bash"
	r01 := func(t *testing.T, version, require string) contract.RuleResult {
		t.Helper()
		dir := probeFixture(t, contracttest.WithFile("vibe.yaml", probeGoConfig(t, version, require)))
		rep := contracttest.Audit(t, dir, contract.Options{
			Rules: contracttest.Rules(t, "A-02", "R-01"),
			Probe: contract.ProbeOptions{Env: contracttest.EmptyProbeEnv(t)},
		})
		return contracttest.RuleResult(t, rep, "R-01")
	}
	t.Run("2.1.0 without the guard set", func(t *testing.T) {
		t.Parallel()
		if text := contracttest.FindingText(r01(t, "2.1.0", "")); strings.Contains(text, noHook) {
			t.Errorf("R-01 holds the repository to A-02's deny cases:\n%s", text)
		}
	})
	t.Run("2.1.0 with the guard set", func(t *testing.T) {
		t.Parallel()
		rr := r01(t, "2.1.0", guardSetRequire)
		if rr.Result != contract.ResultFail || !strings.Contains(contracttest.FindingText(rr), noHook) {
			t.Errorf("R-01 = %s, want fail with %q:\n%s", rr.Result, noHook, contracttest.FindingText(rr))
		}
	})
	t.Run("1.2.0", func(t *testing.T) {
		t.Parallel()
		rr := r01(t, "1.2.0", "")
		if rr.Result != contract.ResultFail || !strings.Contains(contracttest.FindingText(rr), noHook) {
			t.Errorf("R-01 = %s, want fail with %q:\n%s", rr.Result, noHook, contracttest.FindingText(rr))
		}
	})
}

// probeGoConfig is the probe fixture's vibe.yaml declaring contract
// version, with require appended when it is not empty.
func probeGoConfig(t *testing.T, version, require string) string {
	t.Helper()
	return withContract(t, map[string]string{}, version)["vibe.yaml"] + require
}
