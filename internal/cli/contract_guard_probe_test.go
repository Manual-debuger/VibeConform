package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// How vibe audit reports the guard probes A-02 and A-03 (spec 0053). The
// fixtures with real guards run hundreds of hook processes each; they
// are in internal/contract (guardprobe_test.go), which has a test-time
// budget of its own. The fixtures here run no hook: without a
// PreToolUse hook every deny case is let through, unless an exception
// lets it through.

// The exception the tests below declare, and the corpus cases it matches.
const (
	rmBuildException = `{"version": 2, "exceptions": [` +
		`{"rule": "rm-rf", "match": "rm -r -f build", "reason": "the build directory is disposable"}]}` + "\n"
)

var rmBuildCases = []string{
	"rm -r -f (spec 0047)",
	"rm with extra blanks",
	"variant (#105): quoted flags",
	"variant (#105): sudo with options, by full paths",
	"variant (#105): sh -c with separate flags",
}

func TestRuleA02Reporting(t *testing.T) {
	t.Parallel()
	t.Run("no PreToolUse hook fails A-02 with one finding per tool", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t), ids("A-02"))
		r := rep.rule(t, "A-02")
		if r.Result != "fail" {
			t.Errorf("A-02 = %s, want fail", r.Result)
		}
		text := r.findingText()
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
		dir := probeFixture(t, withFile(".claude/hooks/policy.json", rmBuildException))

		a02 := contractSeams{rules: registryRules(t, "A-02")}
		r := runVibeAuditSeams(t, a02, "--repo-root", dir, "--json")
		var out struct {
			Uses []struct {
				Rule  string `json:"rule"`
				Match string `json:"match"`
				Case  string `json:"case"`
			} `json:"guard_exception_uses"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
			t.Fatalf("no JSON: %v\n%s", err, r.stderr)
		}
		var cases []string
		for _, u := range out.Uses {
			if u.Rule != "rm-rf" || u.Match != "rm -r -f build" {
				t.Errorf("a use names exception %s %q", u.Rule, u.Match)
			}
			cases = append(cases, u.Case)
		}
		if !slices.Equal(cases, rmBuildCases) {
			t.Errorf("guard_exception_uses cases = %q, want %q", cases, rmBuildCases)
		}

		text := runVibeAuditSeams(t, a02, "--repo-root", dir).stdout
		want := "Guard exceptions\n" +
			"  rm-rf \"rm -r -f build\" (the build directory is disposable)\n"
		for _, c := range rmBuildCases {
			want += "    let through: " + c + "\n"
		}
		if !strings.Contains(text, want) {
			t.Errorf("the report lacks\n%s\nstdout:\n%s", want, text)
		}
	})
	t.Run("without exceptions guard_exception_uses is an empty list", func(t *testing.T) {
		t.Parallel()
		r := runVibeAuditRules(t, registryRules(t, "A-02"), "--repo-root", contractFixture(t), "--json")
		if !strings.Contains(r.stdout, `"guard_exception_uses": []`) {
			t.Errorf("--json lacks an empty guard_exception_uses:\n%s", r.stdout)
		}
	})
}

// auditGuardRules runs only the rules ids with --json in this process's
// environment, so a test pays for no other probe.
func auditGuardRules(t *testing.T, dir string, ids ...string) jsonReport {
	t.Helper()
	var rules []contract.Rule
	for _, r := range contract.Rules() {
		if slices.Contains(ids, r.ID) {
			rules = append(rules, r)
		}
	}
	r := runVibeAuditSeams(t, contractSeams{rules: rules}, "--repo-root", dir, "--json")
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, r.stdout, r.stderr)
	}
	return rep
}

// How vibe audit reports A-08 (spec 0053, "A-08 (#106)") when no hook
// runs. The fixtures with real guards and Stop hooks are in
// internal/contract (guardprobe_test.go).
func TestRuleA08Reporting(t *testing.T) {
	t.Parallel()
	t.Run("no hooks fail A-08: its cases are not refused and no Stop hook relocks", func(t *testing.T) {
		t.Parallel()
		r := auditGuardRules(t, probeFixture(t), "A-08").rule(t, "A-08")
		if r.Result != "fail" {
			t.Errorf("A-08 = %s, want fail", r.Result)
		}
		text := r.findingText()
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
		rep := auditGuardRules(t, probeFixture(t, withFile(".claude/hooks/policy.json", policy)), "A-02", "A-08")
		if text := rep.rule(t, "A-02").findingText(); strings.Contains(text, "A-08 (#106): Write settings.json, no unlock") {
			t.Errorf("A-02 names a case the exception lets through:\n%s", text)
		}
		text := rep.rule(t, "A-08").findingText()
		want := `corpus case "A-08 (#106): Write settings.json, no unlock" (Write ".claude/settings.json") should be refused ` +
			`with no unlock marker; no PreToolUse hook matches Write; the exception guardrail-settings ".claude/**" in ` +
			`.claude/hooks/policy.json does not count: guard exceptions do not satisfy A-08`
		if !strings.Contains(text, want) {
			t.Errorf("A-08 findings lack %q:\n%s", want, text)
		}
	})
}
