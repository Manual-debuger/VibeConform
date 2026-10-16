package cli

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// --explain prints what the registry knows about one rule (spec 0052,
// "Package and command"): ID, level, category, rule sentence, check
// kinds, source, rationale, the check as spec 0051 states it, when the
// rule is not applicable, and whether it can be waived.
func TestAuditExplainEveryRule(t *testing.T) {
	for _, rule := range contract.Rules() {
		t.Run(rule.ID, func(t *testing.T) {
			r := runVibeAudit(t, "--explain", rule.ID)
			assertExit(t, r, 0)
			if r.stderr != "" {
				t.Errorf("stderr not empty:\n%s", r.stderr)
			}
			if !strings.HasPrefix(r.stdout, rule.ID+": "+rule.Title+"\n") {
				t.Errorf("first line is not %q:\n%s", rule.ID+": "+rule.Title, r.stdout)
			}
			// Long text is wrapped, so compare with the whitespace folded.
			flat := strings.Join(strings.Fields(r.stdout), " ")
			for _, want := range []string{
				string(rule.Level), string(rule.Category) + ", " + rule.Category.Name(),
				rule.Source, rule.Rationale(), rule.Check,
			} {
				if !strings.Contains(flat, strings.Join(strings.Fields(want), " ")) {
					t.Errorf("--explain %s does not show %q:\n%s", rule.ID, want, r.stdout)
				}
			}
		})
	}
}

// The values below are spec 0051's, read from its rule table.
func TestAuditExplainContent(t *testing.T) {
	cases := []struct {
		id   string
		want []string
	}{
		{"A-01", []string{
			"Level: MUST", "Category: A, Agent guardrails", "Checks: static", "Source: spec 0021",
			"Not applicable: when `agents` does not include `claude`",
			"Waivable: no, a MUST rule cannot be waived",
			"The A rules cover Claude Code only.",
			"PreToolUse hooks match each of `Bash`, `PowerShell`, `Write`, `Edit` and `MultiEdit`",
		}},
		{"R-01", []string{
			"Level: MUST", "Category: R, Removability and repository hygiene", "Checks: static, execution",
			"Source: principle 3, spec 0017", "Not applicable: never", "Waivable: no",
		}},
		{"C-05", []string{
			"Level: SHOULD", "Category: C, CI", "Checks: static", "Source: principle 2",
			"Not applicable: when `platforms` does not include `windows`",
			"Waivable: yes, through a waiver in vibe.yaml",
			"Contract v1 supports GitHub Actions only (Q19).",
		}},
		{"V-01", []string{"Checks: execution, probe", "Rationale: Source: spec 0011, principle 2."}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			r := runVibeAudit(t, "--explain", tc.id)
			assertExit(t, r, 0)
			flat := strings.Join(strings.Fields(r.stdout), " ")
			for _, want := range tc.want {
				if !strings.Contains(flat, want) {
					t.Errorf("--explain %s does not show %q:\n%s", tc.id, want, r.stdout)
				}
			}
		})
	}
}
