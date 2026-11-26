package contract_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// The rerun (spec 0072, "The rerun"): under 2.0.0, a deny case that the
// guard let through and an exception matches runs again with the
// matching exceptions removed from the probe copy's policy.json. The
// exception counts only when the guard refuses the case without it.

const (
	rerunSettings = ".claude/settings.json"
	rerunPolicy   = ".claude/hooks/policy.json"
	rerunFix      = "make the guard refuse it; an exception counts only when the guard refuses the case without it"
)

// rerunProbe skips a rerun test where the probes cannot run, and returns
// the directory of the vibe-guard the test built.
func rerunProbe(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	requireBash(t)
	return builtVibeGuard(t)
}

// rmBuildUses is the uses of probeRmBuildException by cases.
func rmBuildUses(cases ...string) []contract.GuardExceptionUse {
	var out []contract.GuardExceptionUse
	for _, c := range cases {
		out = append(out, contract.GuardExceptionUse{Rule: "rm-rf", Match: "rm -r -f build", Case: c})
	}
	return out
}

// A-02's verdict by the rerun, under each contract.
func TestGuardProbeRerunA02(t *testing.T) {
	bin := rerunProbe(t)

	t.Run("a guard that refuses the case without the exception passes and the exception is listed", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, map[string]string{
			rerunSettings: guardSettings(honouringGuard(`*'"rm -r -f build"'*`, `rm -r -f build`)),
			rerunPolicy:   probeRmBuildException,
		}, contract.Contract2)), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
		if want := rmBuildUses(probeRmBuildCases...); !slices.Equal(rep.GuardExceptionUses, want) {
			t.Errorf("GuardExceptionUses = %+v, want %+v", rep.GuardExceptionUses, want)
		}
	})

	// brokenGuard lets rm -r -f build through whatever policy.json says.
	broken := map[string]string{rerunSettings: guardSettings(brokenGuard), rerunPolicy: probeRmBuildException}
	t.Run("a guard that lets the case through without the exception fails under 2.0.0", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, broken, contract.Contract2)), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail,
			`.claude/settings.json: corpus case "rm -r -f (spec 0047)" (Bash "rm -r -f build") should be refused `+
				`(guard rule rm-rf) without the exception rm-rf "rm -r -f build" in .claude/hooks/policy.json; hook "input=$(cat); `,
			`exited 0 Fix: `+rerunFix)
		if got := casesNamed(t, rep); !slices.Equal(got, []string{"rm -r -f (spec 0047)"}) {
			t.Errorf("A-02 names %q, want only rm -r -f (spec 0047)", got)
		}
		if want := rmBuildUses(probeRmBuildCases[1:]...); !slices.Equal(rep.GuardExceptionUses, want) {
			t.Errorf("GuardExceptionUses = %+v, want %+v", rep.GuardExceptionUses, want)
		}
	})
	t.Run("a guard that lets the case through without the exception passes under 1.2.0", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, broken, contract.Contract1)), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
		if want := rmBuildUses(probeRmBuildCases...); !slices.Equal(rep.GuardExceptionUses, want) {
			t.Errorf("GuardExceptionUses = %+v, want %+v", rep.GuardExceptionUses, want)
		}
	})
}

// After the rerun, a later part sees the original policy.json.
func TestGuardProbeRerunRestoresPolicy(t *testing.T) {
	bin := rerunProbe(t)
	// A-02's rerun removes the rm-rf exception. During A-09 the hook lets
	// git commit --no-verify through unless the exception is back.
	hook := `input=$(cat); case "$input" in *'--no-verify -m wip'*) ` +
		`grep -qF 'rm -r -f build' "$CLAUDE_PROJECT_DIR/.claude/hooks/policy.json" || exit 0;; esac; ` +
		`printf '%s' "$input" | vibe-guard hook`
	var rules []contract.Rule
	for _, id := range []string{"A-02", "A-09"} {
		r, ok := contract.LookupRule(id)
		if !ok {
			t.Fatalf("no rule %s", id)
		}
		rules = append(rules, r)
	}
	rep := auditRules(t, guardFixture(t, withContract(t, map[string]string{
		rerunSettings: guardSettings(hook),
		rerunPolicy:   probeRmBuildException,
	}, contract.Contract2)), rules, bin)
	assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
	assertPart(t, rep, "A-09", contract.Probe, contract.ResultPass)
}

// A hook that does not end during the rerun leaves the case undecided.
func TestGuardProbeRerunUndecided(t *testing.T) {
	bin := rerunProbe(t)
	// The first hook hangs on rm -r -f build while policy.json lacks the
	// exception and lets every other call through at once; its timeout
	// leaves room for a slow bash on a loaded machine. brokenGuard, the
	// second, lets rm -r -f build through.
	hang := `input=$(cat); case "$input" in *'"rm -r -f build"'*) ` +
		`grep -qF 'rm -r -f build' "$CLAUDE_PROJECT_DIR/.claude/hooks/policy.json" || sleep 120;; esac; exit 0`
	data, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{
		"PreToolUse": []any{map[string]any{"matcher": guard.ReferencePreToolUseMatcher,
			"hooks": []any{
				map[string]any{"type": "command", "command": hang, "timeout": 15},
				map[string]any{"type": "command", "command": brokenGuard},
			}}},
	}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rep := audit(t, guardFixture(t, withContract(t, map[string]string{
		rerunSettings: string(data) + "\n",
		rerunPolicy:   probeRmBuildException,
	}, contract.Contract2)), []string{"A-02"}, bin)
	assertPart(t, rep, "A-02", contract.Probe, contract.ResultUndecided,
		`cannot decide: corpus case "rm -r -f (spec 0047)" (Bash "rm -r -f build") without the exception rm-rf `+
			`"rm -r -f build" in .claude/hooks/policy.json: `)
}

// A-09 reruns as A-02 does.
func TestGuardProbeRerunA09(t *testing.T) {
	bin := rerunProbe(t)
	// corpus110Guard lets every git commit through whatever policy.json says.
	a09 := map[string]string{
		rerunSettings: guardSettings(corpus110Guard),
		rerunPolicy: `{"version": 2, "exceptions": [{"rule": "git-skip-hooks", "match": "git commit --no-verify -m wip", ` +
			`"reason": "a hook that needs the network"}]}` + "\n",
	}
	excepted := []string{"A-09: git commit --no-verify", "A-09: git commit --no-verify in PowerShell"}
	t.Run("under 2.0.0 the cases fail without the exception", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, a09, contract.Contract2)), []string{"A-09"}, bin)
		assertPart(t, rep, "A-09", contract.Probe, contract.ResultFail,
			`corpus case "A-09: git commit --no-verify" (Bash "git commit --no-verify -m wip") should be refused `+
				`(guard rule git-skip-hooks) without the exception git-skip-hooks "git commit --no-verify -m wip" in .claude/hooks/policy.json`,
			rerunFix)
		got := casesNamedBy(t, rep, "A-09")
		for _, c := range excepted {
			if !slices.Contains(got, c) {
				t.Errorf("A-09 names %q, want %q among them", got, c)
			}
		}
		if len(rep.GuardExceptionUses) != 0 {
			t.Errorf("GuardExceptionUses = %+v, want none", rep.GuardExceptionUses)
		}
	})
	t.Run("under 1.2.0 the exception counts", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, a09, contract.Contract1)), []string{"A-09"}, bin)
		got := casesNamedBy(t, rep, "A-09")
		for _, c := range excepted {
			if slices.Contains(got, c) {
				t.Errorf("A-09 names %q, which the exception lets through under 1.2.0", c)
			}
		}
		if len(rep.GuardExceptionUses) != len(excepted) {
			t.Errorf("GuardExceptionUses = %+v, want the %d excepted cases", rep.GuardExceptionUses, len(excepted))
		}
	})
}
