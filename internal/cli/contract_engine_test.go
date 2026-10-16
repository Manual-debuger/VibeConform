package cli

import (
	"encoding/json"
	"strings"
	"testing"
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
	Summary map[string]map[string]int `json:"summary"`
	Rules   []jsonRule                `json:"rules"`
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
	} `json:"waiver"`
}

// auditJSON runs vibe audit --json on a fixture and decodes the report.
func auditJSON(t *testing.T, dir string, args ...string) (jsonReport, auditResult) {
	t.Helper()
	r := runVibeAudit(t, append([]string{"--repo-root", dir, "--json"}, args...)...)
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

func TestContractAuditReportsEveryRule(t *testing.T) {
	dir := contractFixture(t)
	rep, r := auditJSON(t, dir)

	assertExit(t, r, 1)
	if rep.Schema != 1 {
		t.Errorf("schema = %d, want 1", rep.Schema)
	}
	if rep.Vibe != "test" {
		t.Errorf("vibe = %q, want the running version", rep.Vibe)
	}
	if rep.Contract.Declared != "1.0.0" || rep.Contract.Audited != "1.0.0" {
		t.Errorf("contract = %+v, want 1.0.0 declared and audited", rep.Contract)
	}
	want := []string{
		"V-01", "V-02", "V-03", "V-04", "H-01", "H-02", "C-01", "C-02", "C-03", "C-04", "C-05",
		"A-01", "A-02", "A-03", "A-04", "A-05", "A-06", "A-07", "A-08", "D-01", "D-02", "D-03",
		"R-01", "R-02", "R-03",
	}
	var got []string
	for _, rule := range rep.Rules {
		got = append(got, rule.ID)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("rules = %v, want all 25 in ID order", got)
	}
	c02 := rep.rule(t, "C-02")
	if c02.Level != "MUST" || c02.Category != "C" || c02.Title != "Toolchains and actions are pinned" {
		t.Errorf("C-02 = %+v", c02)
	}
}
