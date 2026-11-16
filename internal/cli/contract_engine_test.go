package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// jsonReport is the --json report as a consumer reads it (spec 0052,
// "Report (#100)").
type jsonReport struct {
	Schema     int    `json:"schema"`
	Vibe       string `json:"vibe"`
	StaticOnly bool   `json:"static_only"`
	Verdict    string `json:"verdict"`
	Reason     string `json:"reason"`
	Contract   struct {
		Declared string `json:"declared"`
		Audited  string `json:"audited"`
	} `json:"contract"`
	Summary    map[string]map[string]int `json:"summary"`
	Rules      []jsonRule                `json:"rules"`
	CIProvider string                    `json:"ci_provider"`
}

type jsonRule struct {
	ID       string `json:"id"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Result   string `json:"result"`
	Parts    []struct {
		Check  string `json:"check"`
		Result string `json:"result"`
	} `json:"parts"`
	Findings []struct {
		File    string `json:"file"`
		Message string `json:"message"`
		Fix     string `json:"fix"`
	} `json:"findings"`
	Waiver *struct {
		Reason  string `json:"reason"`
		Until   string `json:"until"`
		Expired bool   `json:"expired"`
		// Unused is a pointer so that a missing key shows.
		Unused *bool `json:"unused"`
	} `json:"waiver"`
	// Required is a pointer so that a missing key shows.
	Required *bool `json:"required"`
}

// auditJSON runs vibe audit --json on a fixture and decodes the report.
func auditJSON(t *testing.T, dir string, args ...string) (jsonReport, auditResult) {
	t.Helper()
	return auditJSONRules(t, nil, dir, args...)
}

// auditJSONRules is auditJSON with rules in place of the registry.
func auditJSONRules(t *testing.T, rules []contract.Rule, dir string, args ...string) (jsonReport, auditResult) {
	t.Helper()
	r := runVibeAuditRules(t, rules, append([]string{"--repo-root", dir, "--json"}, args...)...)
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, r.stdout, r.stderr)
	}
	return rep, r
}

// rule returns the result of one rule from a report.
func (rep jsonReport) rule(t *testing.T, id string) jsonRule {
	t.Helper()
	for _, r := range rep.Rules {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("report has no rule %s", id)
	return jsonRule{}
}

// findingText joins a rule's findings for substring assertions.
func (r jsonRule) findingText() string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(f.File + ": " + f.Message + " Fix: " + f.Fix + "\n")
	}
	return b.String()
}

func assertRule(t *testing.T, rep jsonReport, id, want string, findings ...string) {
	t.Helper()
	r := rep.rule(t, id)
	if r.Result != want {
		t.Errorf("%s = %s, want %s\nfindings:\n%s", id, r.Result, want, r.findingText())
	}
	text := r.findingText()
	for _, f := range findings {
		if !strings.Contains(text, f) {
			t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
		}
	}
}

// standIn returns rules (nil for the registry) with the listed rules
// deciding every part, execution and probe included, as result. Phase 1
// implements no execution or probe part, so only stand-ins can show the
// verdicts conformant and not conformant through the seam.
func standIn(rules []contract.Rule, result contract.Result, ids ...string) []contract.Rule {
	if rules == nil {
		rules = contract.Rules()
	}
	for i := range rules {
		if !slices.Contains(ids, rules[i].ID) {
			continue
		}
		for j := range rules[i].Parts {
			rules[i].Parts[j].Run = func(*contract.Env, contract.Needed) contract.PartResult {
				return contract.PartResult{Result: result, Findings: []contract.Finding{
					{File: "stand-in.txt", Message: "stand-in " + string(result), Fix: "none"},
				}}
			}
		}
	}
	return rules
}

// mustRules lists every MUST rule except those named.
func mustRules(except ...string) []string { return ruleIDs(contract.Must, except...) }

// ruleIDs lists the rules of a level ("" for every level) except those
// named.
func ruleIDs(level contract.Level, except ...string) []string {
	var ids []string
	for _, r := range contract.Rules() {
		if (level == "" || r.Level == level) && !slices.Contains(except, r.ID) {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

func waiverConfig(rule, until string) fixtureEdit {
	return withFile("vibe.yaml", configWith(section{"waivers",
		"waivers:\n  - rule: " + rule + "\n    reason: known gap\n    until: " + until + "\n"}))
}

// A repository on contract 1.1.0 keeps its verdict under a vibe that
// implements 1.2.0, even when every rule 1.2.0 added fails (spec 0061,
// acceptance criterion 3; user story 9).
func TestContractAuditOlderMinorKeepsVerdictWhenNewRulesFail(t *testing.T) {
	t.Parallel()
	added := []string{"V-05", "V-06", "V-07", "V-08", "A-09"}
	rules := standIn(nil, contract.ResultPass, mustRules("V-04")...)
	rules = standIn(rules, contract.ResultFail, added...)
	dir := contractFixture(t, withFile("vibe.yaml", configWith(section{"contract", "contract: 1.1.0\n"})))
	rep, r := auditJSONRules(t, rules, dir)
	assertExit(t, r, 0)
	if rep.Verdict != "conformant" || rep.Contract.Declared != "1.1.0" || rep.Contract.Audited != "1.2.0" {
		t.Errorf("verdict = %s, contract = %+v, want conformant, 1.1.0 declared, 1.2.0 audited", rep.Verdict, rep.Contract)
	}
	// V-05 is not applicable: the fixture has no Python component.
	for _, id := range added[1:] {
		assertRule(t, rep, id, "fail")
	}
}

func TestContractAuditVerdicts(t *testing.T) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	t.Parallel()
	t.Run("conformant exits 0", func(t *testing.T) {
		rules := standIn(nil, contract.ResultPass, mustRules("V-04")...)
		rep, r := auditJSONRules(t, rules, contractFixture(t))
		assertExit(t, r, 0)
		if rep.Verdict != "conformant" || rep.Reason != "" {
			t.Errorf("verdict = %s (%q), want conformant", rep.Verdict, rep.Reason)
		}
		if r.stderr != "" {
			t.Errorf("stderr not empty:\n%s", r.stderr)
		}
	})
	t.Run("a failed SHOULD rule does not change it", func(t *testing.T) {
		rules := standIn(nil, contract.ResultPass, mustRules("V-04")...)
		rep, r := auditJSONRules(t, rules, contractFixture(t, withoutFile(".github/pull_request_template.md")))
		assertExit(t, r, 0)
		assertRule(t, rep, "R-03", "fail")
	})
	t.Run("an undecided SHOULD rule does not change it", func(t *testing.T) {
		rules := standIn(nil, contract.ResultPass, mustRules("V-04")...)
		rules = standIn(rules, contract.ResultUndecided, "H-01")
		rep, r := auditJSONRules(t, rules, contractFixture(t))
		assertExit(t, r, 0)
		assertRule(t, rep, "H-01", "undecided")
	})
	t.Run("a failed MUST rule exits 2", func(t *testing.T) {
		rules := standIn(nil, contract.ResultPass, mustRules("V-04")...)
		dir := contractFixture(t, entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n"))
		rep, r := auditJSONRules(t, rules, dir)
		assertExit(t, r, 2)
		assertStderr(t, r, "not conformant: 1 MUST rule fails")
		if rep.Verdict != "not_conformant" {
			t.Errorf("verdict = %s, want not_conformant", rep.Verdict)
		}
		if got := rep.Summary["MUST"]["fail"]; got != 1 {
			t.Errorf("summary MUST fail = %d, want 1", got)
		}
	})
	t.Run("cannot decide exits 1", func(t *testing.T) {
		rep, r := auditJSON(t, contractFixture(t))
		assertExit(t, r, 1)
		assertStderr(t, r, "cannot decide: ", "rules cannot be decided")
		if rep.Verdict != "undecided" || rep.StaticOnly {
			t.Errorf("verdict = %s, static_only %v; want undecided, false", rep.Verdict, rep.StaticOnly)
		}
		assertRule(t, rep, "V-01", "undecided", "`task` is not installed on this machine")
		assertRule(t, rep, "A-08", "undecided", "no bash found")
		assertRule(t, rep, "R-01", "undecided", "`task` is not installed on this machine")
	})
	t.Run("a failed MUST rule beside an undecided one exits 1", func(t *testing.T) {
		// Without fmt V-04 fails; fmt-check, which V-03 needs, stays.
		dir := contractFixture(t, entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n"))
		rep, r := auditJSON(t, dir)
		assertExit(t, r, 1)
		assertRule(t, rep, "V-04", "fail")
		if rep.Verdict != "undecided" || !strings.Contains(rep.Reason, "1 MUST rule already fails") {
			t.Errorf("verdict = %s (%q), want undecided saying 1 MUST rule already fails", rep.Verdict, rep.Reason)
		}
	})
	t.Run("--static never reaches a verdict", func(t *testing.T) {
		rules := standIn(nil, contract.ResultPass, ruleIDs("", "V-04", "R-02", "R-03")...)
		rep, r := auditJSONRules(t, rules, contractFixture(t), "--static")
		assertExit(t, r, 1)
		// Every rule with an execution or probe part does not run, unless
		// it is not applicable: V-05 has no Python component here.
		assertRule(t, rep, "V-05", "not_applicable")
		if !rep.StaticOnly || rep.Verdict != "undecided" || !strings.Contains(rep.Reason, "14 rules did not run (--static)") {
			t.Errorf("static_only %v, verdict %s (%q); want undecided, 14 rules did not run", rep.StaticOnly, rep.Verdict, rep.Reason)
		}
		wantNotRun := []string{"V-01", "V-02", "V-03", "V-06", "V-07", "V-08", "A-02", "A-03", "A-05", "A-07", "A-08", "A-09", "D-01", "R-01"}
		var notRun []string
		for _, rule := range rep.Rules {
			if rule.Result == "not_run" {
				notRun = append(notRun, rule.ID)
			}
		}
		if !slices.Equal(notRun, wantNotRun) {
			t.Errorf("rules not run under --static: %v, want %v", notRun, wantNotRun)
		}
		a05 := rep.rule(t, "A-05")
		if len(a05.Parts) != 2 || a05.Parts[0].Check != "static" || a05.Parts[0].Result != "pass" ||
			a05.Parts[1].Check != "probe" || a05.Parts[1].Result != "not_run" {
			t.Errorf("A-05 parts = %+v, want static pass and probe not_run", a05.Parts)
		}
	})
}

func requireConfig(ids string, more ...section) fixtureEdit {
	return withFile("vibe.yaml", configWith(append(more, section{"require", "require: [" + ids + "]\n"})...))
}

// A required SHOULD rule counts in the verdict like a MUST rule (spec
// 0063, "A repository can require SHOULD rules").
func TestContractAuditRequiredRules(t *testing.T) { //nolint:gocognit // one subtest per verdict
	t.Parallel()
	// Every rule decides, so that the reason counts only the rules named.
	passing := func() []contract.Rule {
		return standIn(nil, contract.ResultPass, ruleIDs("", "V-04", "R-02", "R-03")...)
	}
	t.Run("a failing required rule exits 2", func(t *testing.T) {
		rules := standIn(passing(), contract.ResultFail, "V-06")
		rep, r := auditJSONRules(t, rules, contractFixture(t, requireConfig("V-06")))
		assertExit(t, r, 2)
		assertStderr(t, r, "not conformant: 1 required SHOULD rule fails")
		if strings.Contains(r.stderr, "MUST") {
			t.Errorf("stderr counts a MUST rule:\n%s", r.stderr)
		}
		if rep.Verdict != "not_conformant" {
			t.Errorf("verdict = %s, want not_conformant", rep.Verdict)
		}
		if v06 := rep.rule(t, "V-06"); v06.Required == nil || !*v06.Required {
			t.Errorf("V-06 required = %v, want true", v06.Required)
		}
		if v07 := rep.rule(t, "V-07"); v07.Required == nil || *v07.Required {
			t.Errorf("V-07 required = %v, want false", v07.Required)
		}
		if c02 := rep.rule(t, "C-02"); c02.Required == nil || *c02.Required {
			t.Errorf("C-02 required = %v, want false: MUST rules are not listed in require", c02.Required)
		}
	})
	t.Run("MUST and required failures are counted apart", func(t *testing.T) {
		rules := standIn(passing(), contract.ResultFail, "V-06", "V-07")
		dir := contractFixture(t, requireConfig("V-06, V-07",
			section{"entrypoints", "entrypoints:\n  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n"}))
		_, r := auditJSONRules(t, rules, dir)
		assertExit(t, r, 2)
		assertStderr(t, r, "not conformant: 1 MUST rule fails; 2 required SHOULD rules fail")
	})
	t.Run("an undecided required rule exits 1", func(t *testing.T) {
		rules := standIn(passing(), contract.ResultUndecided, "V-06")
		rep, r := auditJSONRules(t, rules, contractFixture(t, requireConfig("V-06")))
		assertExit(t, r, 1)
		if rep.Verdict != "undecided" || !strings.Contains(rep.Reason, "1 rule cannot be decided") {
			t.Errorf("verdict = %s (%q), want undecided, 1 rule cannot be decided", rep.Verdict, rep.Reason)
		}
	})
	t.Run("a required rule that already fails is named in the reason", func(t *testing.T) {
		rules := standIn(passing(), contract.ResultFail, "V-06")
		rules = standIn(rules, contract.ResultUndecided, "V-01")
		rep, r := auditJSONRules(t, rules, contractFixture(t, requireConfig("V-06")))
		assertExit(t, r, 1)
		if want := "1 rule cannot be decided; 1 required SHOULD rule already fails"; rep.Reason != want {
			t.Errorf("reason = %q, want %q", rep.Reason, want)
		}
	})
	t.Run("a required rule that is not applicable is conformant", func(t *testing.T) {
		// V-05 is not applicable: the fixture has no Python component.
		rep, r := auditJSONRules(t, passing(), contractFixture(t, requireConfig("V-05")))
		assertExit(t, r, 0)
		assertRule(t, rep, "V-05", "not_applicable")
		if rep.Verdict != "conformant" {
			t.Errorf("verdict = %s, want conformant", rep.Verdict)
		}
	})
	t.Run("a failing SHOULD rule that is not required is conformant", func(t *testing.T) {
		rules := standIn(passing(), contract.ResultFail, "V-07")
		rules = standIn(rules, contract.ResultPass, "V-06")
		rep, r := auditJSONRules(t, rules, contractFixture(t, requireConfig("V-06")))
		assertExit(t, r, 0)
		assertRule(t, rep, "V-07", "fail")
		if rep.Verdict != "conformant" {
			t.Errorf("verdict = %s, want conformant", rep.Verdict)
		}
	})
}

// Every rule object of the JSON report has a boolean required key, also
// without require (spec 0063).
func TestContractJSONRuleHasRequiredKey(t *testing.T) {
	t.Parallel()
	r := runVibeAudit(t, "--repo-root", contractFixture(t), "--json", "--static")
	var rep struct {
		Rules []map[string]json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("no JSON report: %v\n%s", err, r.stdout)
	}
	for _, rule := range rep.Rules {
		if got := string(rule["required"]); got != "false" {
			t.Errorf("rule %s: required = %q, want false", rule["id"], got)
		}
	}
}

func TestContractAuditApplicabilityAndWaivers(t *testing.T) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	t.Parallel()
	t.Run("C-05 not applicable without windows", func(t *testing.T) {
		rep, _ := auditJSON(t, contractFixture(t,
			withFile("vibe.yaml", configWith(section{"platforms", "platforms: [linux]\n"}))), "--static")
		assertRule(t, rep, "C-05", "not_applicable", "platforms does not include windows")
		if p := rep.rule(t, "C-05").Parts; len(p) != 1 || p[0].Result != "not_applicable" {
			t.Errorf("C-05 parts = %+v, want one not_applicable part", p)
		}
	})
	t.Run("A rules not applicable without claude", func(t *testing.T) {
		rep, _ := auditJSON(t, contractFixture(t, agentsConfig("[codex]")), "--static")
		for _, id := range []string{"A-01", "A-02", "A-03", "A-04", "A-05", "A-06", "A-07", "A-08"} {
			assertRule(t, rep, id, "not_applicable", "agents does not include claude")
		}
	})
	t.Run("waived", func(t *testing.T) {
		rep, _ := auditJSON(t, contractFixture(t,
			waiverConfig("R-03", "2999-12-31"), withoutFile(".github/pull_request_template.md")), "--static")
		assertRule(t, rep, "R-03", "waived")
		w := rep.rule(t, "R-03").Waiver
		if w == nil || w.Reason != "known gap" || w.Until != "2999-12-31" || w.Expired {
			t.Errorf("R-03 waiver = %+v, want in force until 2999-12-31", w)
		}
		if w != nil && (w.Unused == nil || *w.Unused) {
			t.Errorf("R-03 waiver unused = %v, want false: the waiver applied", w.Unused)
		}
		if got := rep.Summary["SHOULD"]["waived"]; got != 1 {
			t.Errorf("summary SHOULD waived = %d, want 1", got)
		}
	})
	t.Run("expired waiver is checked again", func(t *testing.T) {
		rep, _ := auditJSON(t, contractFixture(t,
			waiverConfig("R-03", "2000-01-01"), withoutFile(".github/pull_request_template.md")), "--static")
		assertRule(t, rep, "R-03", "fail")
		if w := rep.rule(t, "R-03").Waiver; w == nil || !w.Expired {
			t.Errorf("R-03 waiver = %+v, want expired", w)
		}
		rep, _ = auditJSON(t, contractFixture(t, waiverConfig("R-03", "2000-01-01")), "--static")
		assertRule(t, rep, "R-03", "pass")
	})
	t.Run("waiver for a rule that is not applicable is unused", func(t *testing.T) {
		rep, _ := auditJSON(t, contractFixture(t, withFile("vibe.yaml", configWith(
			section{"platforms", "platforms: [linux]\n"},
			section{"waivers", "waivers:\n  - rule: C-05\n    reason: known gap\n    until: 2999-12-31\n"}))), "--static")
		assertRule(t, rep, "C-05", "not_applicable")
		if w := rep.rule(t, "C-05").Waiver; w == nil || w.Expired || w.Unused == nil || !*w.Unused {
			t.Errorf("C-05 waiver = %+v, want the unused waiver listed with unused true", w)
		}
		if rep.Schema != 1 {
			t.Errorf("schema = %d, want 1: unused is a new key", rep.Schema)
		}
		r := runVibeAudit(t, "--repo-root", contractFixture(t, withFile("vibe.yaml", configWith(
			section{"platforms", "platforms: [linux]\n"},
			section{"waivers", "waivers:\n  - rule: C-05\n    reason: known gap\n    until: 2999-12-31\n"}))), "--static")
		if !strings.Contains(r.stdout, "waiver unused") {
			t.Errorf("text report does not say the waiver is unused:\n%s", r.stdout)
		}
	})
	t.Run("newer PATCH audited as 1.2.0", func(t *testing.T) {
		rep, _ := auditJSON(t, contractFixture(t,
			withFile("vibe.yaml", configWith(section{"contract", "contract: 1.2.7\n"}))), "--static")
		if rep.Contract.Declared != "1.2.7" || rep.Contract.Audited != "1.2.0" {
			t.Errorf("contract = %+v, want 1.2.7 declared, 1.2.0 audited", rep.Contract)
		}
	})
	// A vibe audits every repository that declares its MAJOR with the
	// latest MINOR it implements (spec 0051, "Versioning"; spec 0055;
	// spec 0061, "Contract version").
	for _, declared := range []string{"1.0.0", "1.0.1", "1.1.0", "1.1.3"} {
		t.Run("older MINOR "+declared+" audited as 1.2.0", func(t *testing.T) {
			rep, _ := auditJSON(t, contractFixture(t,
				withFile("vibe.yaml", configWith(section{"contract", "contract: " + declared + "\n"}))), "--static")
			if rep.Contract.Declared != declared || rep.Contract.Audited != "1.2.0" {
				t.Errorf("contract = %+v, want %s declared, 1.2.0 audited", rep.Contract, declared)
			}
		})
	}
}

func TestContractAuditTextReport(t *testing.T) {
	t.Parallel()
	r := runVibeAudit(t, "--repo-root", contractFixture(t), "--static")
	assertExit(t, r, 1)
	if !strings.HasPrefix(r.stdout, "vibe audit: contract 1.2.0 (vibe test), static checks only\n") {
		t.Errorf("header:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "\nVerdict: cannot decide. ") {
		t.Errorf("no verdict line:\n%s", r.stdout)
	}
}

func TestContractAuditReportsEveryRule(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t)
	rep, r := auditJSON(t, dir)

	assertExit(t, r, 1)
	if rep.Schema != 1 {
		t.Errorf("schema = %d, want 1", rep.Schema)
	}
	if rep.Vibe != "test" {
		t.Errorf("vibe = %q, want the running version", rep.Vibe)
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
	for _, rule := range rep.Rules {
		got = append(got, rule.ID)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("rules = %v, want every rule in ID order", got)
	}
	c02 := rep.rule(t, "C-02")
	if c02.Level != "MUST" || c02.Category != "C" || c02.Title != "Toolchains and actions are pinned" {
		t.Errorf("C-02 = %+v", c02)
	}
}
