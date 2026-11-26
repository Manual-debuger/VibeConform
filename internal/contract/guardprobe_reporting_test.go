package contract_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// What the guard probes A-02 and A-08 report (spec 0053) when no hook
// runs. The fixtures with real guards are in guardprobe_test.go; without
// a PreToolUse hook every deny case is let through, unless an exception
// lets it through. How vibe audit renders guard_exception_uses and the
// "Guard exceptions" section is tested in internal/cli.

// The exception the tests below declare, and the corpus cases it matches.
const (
	probeRmBuildException = `{"version": 2, "exceptions": [` +
		`{"rule": "rm-rf", "match": "rm -r -f build", "reason": "the build directory is disposable"}]}` + "\n"
)

var probeRmBuildCases = []string{
	"rm -r -f (spec 0047)",
	"rm with extra blanks",
	"variant (#105): quoted flags",
	"variant (#105): sudo with options, by full paths",
	"variant (#105): sh -c with separate flags",
}

// probeAuditRules runs only the rules ids in this process's environment,
// so a test pays for no other probe.
func probeAuditRules(t *testing.T, dir string, ids ...string) *contract.Report {
	t.Helper()
	return contracttest.Audit(t, dir, contract.Options{Rules: contracttest.Rules(t, ids...)})
}

func TestRuleA02Reporting(t *testing.T) {
	t.Parallel()
	t.Run("no PreToolUse hook fails A-02 with one finding per tool", func(t *testing.T) {
		t.Parallel()
		r := contracttest.RuleResult(t, probeAuditRules(t, probeFixture(t), "A-02"), "A-02")
		if r.Result != contract.ResultFail {
			t.Errorf("A-02 = %s, want fail", r.Result)
		}
		text := contracttest.FindingText(r)
		for _, want := range []string{
			`.claude/settings.json: no PreToolUse hook matches Bash, so its `,
			`deny cases are not refused: "`, `, "rm -r -f (spec 0047)", `,
			"no PreToolUse hook matches PowerShell", "no PreToolUse hook matches Write", "no PreToolUse hook matches Edit",
			"Fix: add a PreToolUse hook whose matcher matches Bash and runs the guard",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("A-02 findings lack %q:\n%s", want, text)
			}
		}
	})
	t.Run("an exception's cases are listed in JSON and text", func(t *testing.T) {
		t.Parallel()
		dir := probeFixture(t, contracttest.WithFile(".claude/hooks/policy.json", probeRmBuildException))
		rep := probeAuditRules(t, dir, "A-02")
		var cases []string
		for _, u := range rep.GuardExceptionUses {
			if u.Rule != "rm-rf" || u.Match != "rm -r -f build" {
				t.Errorf("a use names exception %s %q", u.Rule, u.Match)
			}
			cases = append(cases, u.Case)
		}
		if !slices.Equal(cases, probeRmBuildCases) {
			t.Errorf("GuardExceptionUses cases = %q, want %q", cases, probeRmBuildCases)
		}
		want := []contract.GuardException{{Rule: "rm-rf", Match: "rm -r -f build", Reason: "the build directory is disposable"}}
		if !slices.Equal(rep.GuardExceptions, want) {
			t.Errorf("GuardExceptions = %+v, want %+v", rep.GuardExceptions, want)
		}
	})
	t.Run("without exceptions guard_exception_uses is an empty list", func(t *testing.T) {
		t.Parallel()
		rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline), contract.Options{
			Rules: contracttest.Rules(t, "A-02"),
			Probe: contract.ProbeOptions{Env: contracttest.EmptyProbeEnv(t)},
		})
		if len(rep.GuardExceptionUses) != 0 {
			t.Errorf("GuardExceptionUses = %+v, want none", rep.GuardExceptionUses)
		}
	})
}

// A deny case that no PreToolUse hook runs is not one an exception let
// through under 2.0.0 (spec 0072, "The rerun"): the "no PreToolUse hook
// matches" finding names the case and the exception that did not count.
// Under 1.2.0 the exception counts, as before.
func TestRuleA02UnmatchedCaseWithAnException(t *testing.T) {
	t.Parallel()
	report := func(t *testing.T, version string) *contract.Report {
		t.Helper()
		var edits []contracttest.Edit
		for p, content := range withContract(t, map[string]string{".claude/hooks/policy.json": probeRmBuildException}, version) {
			edits = append(edits, contracttest.WithFile(p, content))
		}
		return probeAuditRules(t, probeFixture(t, edits...), "A-02")
	}
	t.Run("2.0.0", func(t *testing.T) {
		t.Parallel()
		rep := report(t, contract.Latest)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail,
			`no PreToolUse hook matches Bash, so its `,
			`"rm -r -f (spec 0047)" (the exception rm-rf "rm -r -f build" in .claude/hooks/policy.json does not count: `+
				`no hook ran the case)`)
		if len(rep.GuardExceptionUses) != 0 {
			t.Errorf("GuardExceptionUses = %+v, want none", rep.GuardExceptionUses)
		}
	})
	t.Run("1.2.0", func(t *testing.T) {
		t.Parallel()
		rep := report(t, contract.Contract1)
		if text := findingText(part(t, rep, "A-02", contract.Probe)); strings.Contains(text, `"rm -r -f (spec 0047)"`) {
			t.Errorf("A-02 names a case the exception lets through:\n%s", text)
		}
		if want := rmBuildUses(probeRmBuildCases...); !slices.Equal(rep.GuardExceptionUses, want) {
			t.Errorf("GuardExceptionUses = %+v, want %+v", rep.GuardExceptionUses, want)
		}
	})
}

// What A-08 reports (spec 0053, "A-08 (#106)") when no hook runs. The
// fixtures with real guards and Stop hooks are in guardprobe_test.go.
func TestRuleA08Reporting(t *testing.T) {
	t.Parallel()
	t.Run("no hooks fail A-08: its cases are not refused and no Stop hook relocks", func(t *testing.T) {
		t.Parallel()
		r := contracttest.RuleResult(t, probeAuditRules(t, probeFixture(t), "A-08"), "A-08")
		if r.Result != contract.ResultFail {
			t.Errorf("A-08 = %s, want fail", r.Result)
		}
		text := contracttest.FindingText(r)
		for _, want := range []string{
			`.claude/settings.json: corpus case "A-08 (#106): Write settings.json, no unlock" (Write ".claude/settings.json") ` +
				`should be refused with no unlock marker; no PreToolUse hook matches Write`,
			`corpus case "A-08 (#106): Bash vibe unlock, valid unlock" (Bash "vibe unlock --minutes 240") should be refused with a valid unlock marker`,
			`corpus case "A-08 (#106): Write settings.json, expired unlock" (Write ".claude/settings.json") ` +
				`should be refused with an expired unlock marker`,
			"no Stop hook removes the unlock marker",
			"Fix: add a Stop hook that runs `vibe-guard relock`",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("A-08 findings lack %q:\n%s", want, text)
			}
		}
	})
	t.Run("a guardrail-settings exception passes A-02's case and does not satisfy A-08", func(t *testing.T) {
		t.Parallel()
		policy := `{"version": 2, "exceptions": [` +
			`{"rule": "guardrail-settings", "match": ".claude/**", "reason": "the agent maintains its hooks"}]}` + "\n"
		rep := probeAuditRules(t, probeFixture(t, contracttest.WithFile(".claude/hooks/policy.json", policy)), "A-02", "A-08")
		if text := contracttest.FindingText(contracttest.RuleResult(t, rep, "A-02")); strings.Contains(text, "A-08 (#106): Write settings.json, no unlock") {
			t.Errorf("A-02 names a case the exception lets through:\n%s", text)
		}
		text := contracttest.FindingText(contracttest.RuleResult(t, rep, "A-08"))
		want := `corpus case "A-08 (#106): Write settings.json, no unlock" (Write ".claude/settings.json") should be refused ` +
			`with no unlock marker; no PreToolUse hook matches Write; the exception guardrail-settings ".claude/**" in ` +
			`.claude/hooks/policy.json does not count: guard exceptions do not satisfy A-08`
		if !strings.Contains(text, want) {
			t.Errorf("A-08 findings lack %q:\n%s", want, text)
		}
	})
}
