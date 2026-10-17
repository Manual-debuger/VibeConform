package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
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
		rep := auditProbe(t, probeFixture(t))
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

		r := runVibeAuditProbe(t, "--repo-root", dir, "--json")
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

		text := runVibeAuditProbe(t, "--repo-root", dir).stdout
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
		r := runVibeAudit(t, "--repo-root", contractFixture(t), "--json")
		if !strings.Contains(r.stdout, `"guard_exception_uses": []`) {
			t.Errorf("--json lacks an empty guard_exception_uses:\n%s", r.stdout)
		}
	})
}
