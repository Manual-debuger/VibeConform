package contracttest

import (
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// Section is one top-level field of a fixture vibe.yaml, with its text:
// the key and the lines that hold it, such as
// Section{"agents", "agents: [claude]\n"}.
type Section struct {
	Key  string
	Body string
}

// baseConfig is the baseline fixture's vibe.yaml, field by field.
var baseConfig = []Section{
	{"contract", "contract: 1.2.0\n"},
	{"platforms", "platforms: [linux, windows]\n"},
	{"agents", "agents: [claude]\n"},
	{"entrypoints", "entrypoints:\n  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"},
}

// ConfigWith is the baseline vibe.yaml with fields replaced in place (an
// empty body removes the field) and new fields appended in order.
func ConfigWith(changes ...Section) string {
	var b strings.Builder
	used := map[string]bool{}
	for _, s := range baseConfig {
		body := s.Body
		for _, c := range changes {
			if c.Key == s.Key {
				body = c.Body
				used[c.Key] = true
			}
		}
		b.WriteString(body)
	}
	for _, c := range changes {
		if !used[c.Key] {
			b.WriteString(c.Body)
		}
	}
	return b.String()
}

// Entrypoints is the baseline vibe.yaml with its entrypoints replaced by
// body, the lines under "entrypoints:".
func Entrypoints(body string) Edit {
	return WithFile("vibe.yaml", ConfigWith(Section{"entrypoints", "entrypoints:\n" + body}))
}

// AgentsConfig is the baseline vibe.yaml with its agents set to list, a
// YAML flow list such as "[codex]" or "[]".
func AgentsConfig(list string) Edit {
	return WithFile("vibe.yaml", ConfigWith(Section{"agents", "agents: " + list + "\n"}))
}

// Dependabot writes .github/dependabot.yml with one updates entry for
// each of entries, "ecosystem directory-key directory-value", such as
// "gomod directory /" or "npm directories [/web, /apps/*]".
func Dependabot(entries ...string) Edit {
	var b strings.Builder
	b.WriteString("version: 2\nupdates:\n")
	for _, e := range entries {
		f := strings.SplitN(e, " ", 3)
		b.WriteString("  - package-ecosystem: " + f[0] + "\n    " + f[1] + ": " + f[2] + "\n    schedule:\n      interval: weekly\n")
	}
	return WithFile(".github/dependabot.yml", b.String())
}

// RuleResult returns one rule's result from a report, or fails the test.
func RuleResult(t testing.TB, rep *contract.Report, id string) contract.RuleResult {
	t.Helper()
	for _, rr := range rep.Rules {
		if rr.Rule.ID == id {
			return rr
		}
	}
	t.Fatalf("the report has no rule %s", id)
	return contract.RuleResult{}
}

// FindingText is every finding of a rule, one per line, as
// "<file>: <message> Fix: <fix>", for substring assertions.
func FindingText(rr contract.RuleResult) string {
	var b strings.Builder
	for _, f := range rr.Findings() {
		b.WriteString(f.File + ": " + f.Message + " Fix: " + f.Fix + "\n")
	}
	return b.String()
}

// AssertRule checks one rule's result and that its findings hold each
// of findings.
func AssertRule(t testing.TB, rep *contract.Report, id string, want contract.Result, findings ...string) {
	t.Helper()
	rr := RuleResult(t, rep, id)
	text := FindingText(rr)
	if rr.Result != want {
		t.Errorf("%s = %s, want %s\nfindings:\n%s", id, rr.Result, want, text)
	}
	assertFindings(t, id, text, findings)
}

// AssertPart checks the result of one part of a rule and that the rule's
// findings hold each of findings.
func AssertPart(t testing.TB, rep *contract.Report, id string, kind contract.PartKind, want contract.Result, findings ...string) {
	t.Helper()
	rr := RuleResult(t, rep, id)
	i := slices.IndexFunc(rr.Parts, func(p contract.PartOutcome) bool { return p.Kind == kind })
	if i < 0 {
		t.Fatalf("%s has no %s part: %+v", id, kind, rr.Parts)
	}
	text := FindingText(rr)
	if got := rr.Parts[i].Result; got != want {
		t.Errorf("%s %s = %s, want %s\nfindings:\n%s", id, kind, got, want, text)
	}
	assertFindings(t, id, text, findings)
}

func assertFindings(t testing.TB, id, text string, findings []string) {
	t.Helper()
	for _, f := range findings {
		if !strings.Contains(text, f) {
			t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
		}
	}
}

// RuleCase is one fixture for one rule: the baseline plus Edits, and the
// result and findings they must give.
type RuleCase struct {
	Name     string
	Edits    []Edit
	Want     contract.Result
	Findings []string
}

// RunRuleCases runs each case as a parallel subtest: a static audit of
// the baseline with the case's edits, through Audit, so the report check
// runs too.
func RunRuleCases(t *testing.T, id string, cases []RuleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			rep := Audit(t, Fixture(t, Baseline, tc.Edits...), contract.Options{Static: true})
			AssertRule(t, rep, id, tc.Want, tc.Findings...)
		})
	}
}

// RunPartCases is RunRuleCases for one part of a rule that has other
// parts: a static audit runs only the static parts, so each case's Want
// is the result of the part of kind, and the findings are the rule's. A
// case whose part fails must fail the rule too; a passing static part
// leaves the rule not run, since its other parts did not run.
func RunPartCases(t *testing.T, id string, kind contract.PartKind, cases []RuleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			rep := Audit(t, Fixture(t, Baseline, tc.Edits...), contract.Options{Static: true})
			AssertPart(t, rep, id, kind, tc.Want, tc.Findings...)
			if r := RuleResult(t, rep, id); tc.Want == contract.ResultFail && r.Result != contract.ResultFail {
				t.Errorf("%s = %s, want fail", id, r.Result)
			}
		})
	}
}
