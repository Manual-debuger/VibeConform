package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// --explain prints what the registry knows about one rule (spec 0052,
// "Package and command"): ID, level, category, rule sentence, check
// kinds, source, rationale, the check as spec 0051 states it, when the
// rule is not applicable, and whether it can be waived.
func TestAuditExplainEveryRule(t *testing.T) {
	t.Parallel()
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
				string(rule.LevelAt(contract.Latest)), string(rule.Category) + ", " + rule.Category.Name(),
				rule.Source, rule.Rationale(), rule.Check(),
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
	t.Parallel()
	cases := []struct {
		id   string
		want []string
	}{
		{"A-01", []string{
			"Level: SHOULD", "Category: A, Agent guardrails", "Checks: static", "Source: spec 0021",
			"Not applicable: when `agents` does not include `claude`",
			"Waivable: yes, through a waiver in vibe.yaml",
			"The A rules cover Claude Code only.",
			"PreToolUse hooks match each of `Bash`, `PowerShell`, `Write`, `Edit` and `MultiEdit`",
		}},
		{"R-01", []string{
			"Level: MUST", "Category: R, Removability and repository hygiene", "Checks: static, execution",
			"Source: principle 3, spec 0017", "Not applicable: never", "Waivable: no",
			// spec 0072, "R-01 and C-04".
			"Under contract 2.x, a command calls `vibe` when the last path element of its first word, after `/` or `\\`, is the literal `vibe` or `vibe.exe`",
			"Under contract 1.x, its first word must be the literal `vibe`.",
		}},
		// spec 0072, "The audit's reading under 2.0.0" and "The rerun";
		// spec 0074, "Applicability of the guard-testing rules".
		{"A-02", []string{
			"Not applicable: when `agents` does not include `claude`; under contract 2.1.0, also when `require` " +
				"does not list the guard set and no PreToolUse hook in `.claude/settings.json` matches `Bash`",
			"Under contract 2.x, an exception counts for a deny case only when",
			"the guard refuses the case when every exception that matches it is removed from `.claude/hooks/policy.json`",
			"A command exception must prefix the simple command the case's rule refuses, and no exception matches a fixed case",
		}},
		{"C-05", []string{
			"Level: SHOULD", "Category: C, CI", "Checks: static", "Source: principle 2",
			"Not applicable: when `platforms` does not include `windows`",
			"Waivable: yes, through a waiver in vibe.yaml",
			"The C rules read GitHub Actions or GitLab CI (GitLab CI since 1.1.0).",
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

// --explain V-07 prints the secret sample once, since every language
// has the same one, says where it is planted and what satisfies the rule
// (spec 0061, "`vibe explain` and the report").
func TestAuditExplainV07(t *testing.T) {
	t.Parallel()
	r := runVibeAudit(t, "--explain", "V-07")
	assertExit(t, r, 0)
	flat := strings.Join(strings.Fields(r.stdout), " ")
	for _, want := range []string{
		"Level: SHOULD", "Checks: probe", "Source: spec 0061, ADR 0027",
		"Every language, secret: vibe_probe_secret.txt",
		"    Planted by vibe audit: a PEM private-key block (the secret defect). It is not a key.\n",
		"BEGIN PRIVATE KEY", "END PRIVATE KEY",
	} {
		if !strings.Contains(r.stdout, want) && !strings.Contains(flat, want) {
			t.Errorf("--explain V-07 does not show %q:\n%s", want, r.stdout)
		}
	}
	for _, want := range []string{
		"Once per repository, in the first component in configuration order that has tests",
		"not committed, so the scanner must read the working tree's files, not the Git history",
		"Any secret scanner that reports it satisfies the rule",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("--explain V-07 does not say %q:\n%s", want, r.stdout)
		}
	}
	if n := strings.Count(r.stdout, "vibe_probe_secret.txt"); n != 1 {
		t.Errorf("--explain V-07 names the sample %d times, want once:\n%s", n, r.stdout)
	}
}

// --explain V-01 and V-02 print the planted defect samples exactly
// (spec 0053, "Planted defects"), so that a probe can be reproduced by
// hand.
func TestAuditExplainDefectSamples(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"V-01", "V-02"} {
		t.Run(id, func(t *testing.T) {
			r := runVibeAudit(t, "--explain", id)
			assertExit(t, r, 0)
			for _, want := range []string{
				"Planted defects:",
				"Go, format: vibe_probe_format_test.go",
				"Go, static analysis: vibe_probe_static_analysis_test.go",
				"Go, test: vibe_probe_test.go",
				"Python, format: vibe_probe_format.py",
				"Python, static analysis: vibe_probe_static_analysis.py",
				"Python, test: vibe_probe_test.py",
				"TypeScript, format: vibe_probe_format.ts",
				"TypeScript, static analysis: vibe_probe_static_analysis.ts",
				"TypeScript, test: vibe_probe.test.ts",
				"    // Planted by vibe audit: a test that fails (the test defect).\n" +
					"\n" +
					"    package {{package}}\n" +
					"\n" +
					"    import \"testing\"\n" +
					"\n" +
					"    func TestVibeProbe(t *testing.T) { t.Fatal(\"planted by vibe audit: this test fails\") }\n",
				"    # Planted by vibe audit: a reference to an undefined name (the static-analysis defect).\n" +
					"    def vibe_probe() -> int:\n" +
					"        return vibe_probe_undefined_name\n",
			} {
				if !strings.Contains(r.stdout, want) {
					t.Errorf("--explain %s does not show %q:\n%s", id, want, r.stdout)
				}
			}
		})
	}
}

// --explain A-09 lists A-09's own corpus cases, each with its expected
// decision, tool and command (spec 0061, "`vibe explain` and the
// report").
func TestAuditExplainA09ListsItsCases(t *testing.T) {
	t.Parallel()
	r := runVibeAudit(t, "--explain", "A-09")
	assertExit(t, r, 0)
	cases := contract.GuardCorpusFor(contract.ProbeA09)
	if len(cases) != 13 {
		t.Fatalf("A-09 has %d corpus cases, want the 13 of spec 0061", len(cases))
	}
	for _, c := range cases {
		want := fmt.Sprintf("\n  %-5s  %-10s  %s\n", c.Want, c.Payload.ToolName, c.Target().Text())
		if !strings.Contains(r.stdout, want) {
			t.Errorf("--explain A-09 lacks %q:\n%s", want, r.stdout)
		}
	}
	for _, w := range []string{"git-skip-hooks", "LEFTHOOK=0", "HUSKY=0"} {
		if !strings.Contains(r.stdout, w) {
			t.Errorf("--explain A-09 does not name %s:\n%s", w, r.stdout)
		}
	}
}

// --explain V-05 prints the type-error sample exactly, Python's only
// (spec 0061); V-01 does not plant it, so it does not print it.
func TestAuditExplainTypeErrorSample(t *testing.T) {
	t.Parallel()
	sample := "  Python, type error: vibe_probe_type_error.py\n" +
		"    # Planted by vibe audit: a function annotated -> int that returns a string (the type-error defect).\n" +
		"    def vibe_probe_type_error() -> int:\n" +
		"        return \"vibe audit\"\n"
	r := runVibeAudit(t, "--explain", "V-05")
	assertExit(t, r, 0)
	for _, want := range []string{"Not applicable:  when no component is Python", "Planted defects:", sample, "Python components only"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("--explain V-05 does not show %q:\n%s", want, r.stdout)
		}
	}
	for _, unwanted := range []string{"Go, type error", "TypeScript, type error", "Python, format"} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("--explain V-05 shows %q:\n%s", unwanted, r.stdout)
		}
	}
	if v01 := runVibeAudit(t, "--explain", "V-01"); strings.Contains(v01.stdout, "type error") {
		t.Errorf("--explain V-01 shows the type-error sample:\n%s", v01.stdout)
	}
}

// --explain A-02 and A-03 print the guard corpus's counts by guard rule
// ID (spec 0053, "Report and CLI"). The expected counts are taken from
// the corpus data.
func TestAuditExplainGuardCorpusCounts(t *testing.T) { //nolint:gocognit // baseline: pre-existing complexity
	t.Parallel()
	deny, allow := 0, 0
	byRule := map[string]int{}
	for _, c := range contract.GuardCorpusFor("") {
		if c.Want == contract.WantDeny {
			deny++
			byRule[c.Rule]++
		} else {
			allow++
		}
	}
	for _, id := range []string{"A-02", "A-03"} {
		t.Run(id, func(t *testing.T) {
			r := runVibeAudit(t, "--explain", id)
			assertExit(t, r, 0)
			want := []string{fmt.Sprintf("\nGuard corpus:\n  %d cases: %d deny and %d allow. Deny cases by guard rule ID:\n",
				deny+allow, deny, allow)}
			for _, rule := range guardrule.IDs {
				if byRule[rule] > 0 {
					want = append(want, fmt.Sprintf("\n  %-26s %3d\n", rule, byRule[rule]))
				}
			}
			for _, w := range want {
				if !strings.Contains(r.stdout, w) {
					t.Errorf("--explain %s lacks %q:\n%s", id, w, r.stdout)
				}
			}
			if strings.Contains(r.stdout, guardrule.GitSkipHooks) {
				t.Errorf("--explain %s counts A-09's cases:\n%s", id, r.stdout)
			}
		})
	}
	r := runVibeAudit(t, "--explain", "A-04")
	if strings.Contains(r.stdout, "Guard corpus") {
		t.Errorf("--explain A-04 prints the corpus:\n%s", r.stdout)
	}
}
