package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The registry quotes spec 0051 (spec 0052, "Rule registry and results"):
// each rule's sentence, check, level and source must match the spec's
// rule table, and each part's check must be a piece of the whole check.
func TestRegistryQuotesSpec0051(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "0051-contract-v1.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][]string{}
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		cells := strings.Split(strings.Trim(line, "|"), " | ")
		if len(cells) != 5 || len(strings.TrimSpace(cells[0])) != 4 || strings.TrimSpace(cells[0])[1] != '-' {
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows[cells[0]] = cells
	}
	if len(rows) != 25 {
		t.Fatalf("found %d rule rows in spec 0051, want 25", len(rows))
	}

	must, from0051 := 0, 0
	for _, r := range Rules() {
		row, ok := rows[r.ID]
		if !ok {
			checkSpec0061Rule(t, r)
			continue
		}
		from0051++
		if r.Title != row[1] {
			t.Errorf("%s title = %q, spec says %q", r.ID, r.Title, row[1])
		}
		if r.Check != row[2] {
			t.Errorf("%s check = %q, spec says %q", r.ID, r.Check, row[2])
		}
		if string(r.Level) != row[3] {
			t.Errorf("%s level = %s, spec says %s", r.ID, r.Level, row[3])
		}
		if r.Source != row[4] {
			t.Errorf("%s source = %q, spec says %q", r.ID, r.Source, row[4])
		}
		if string(r.Category) != r.ID[:1] {
			t.Errorf("%s category = %s", r.ID, r.Category)
		}
		if r.Waivable() != (r.Level == Should) {
			t.Errorf("%s waivable = %v", r.ID, r.Waivable())
		}
		for _, p := range r.Parts {
			if !strings.Contains(r.Check, p.Check) {
				t.Errorf("%s %s part %q is not part of its check", r.ID, p.Kind, p.Check)
			}
			if !strings.Contains(strings.ToLower(r.Check), string(p.Kind)) {
				t.Errorf("%s has a %s part its check does not name", r.ID, p.Kind)
			}
			if p.Run == nil {
				t.Errorf("%s %s part has no check", r.ID, p.Kind)
			}
		}
		if r.Level == Must {
			must++
		}
	}
	if must != 16 || from0051 != 25 {
		t.Errorf("%d rules of spec 0051, %d MUST; want 25 and 16", from0051, must)
	}
}

// spec0061 is spec 0061's text with runs of white space made one space.
var spec0061 = func() string {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "0061-contract-1-2-quality-probes.md"))
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(data)), " ")
}()

// checkSpec0061Rule checks a rule of contract 1.2.0 that spec 0051 does
// not have: spec 0061's Solution lists it as "**ID**: title", it is a
// SHOULD rule (ADR 0027), and its parts are pieces of its check.
func checkSpec0061Rule(t *testing.T, r Rule) {
	t.Helper()
	if spec0061 == "" {
		t.Fatal("cannot read spec 0061")
	}
	bullet := strings.ToLower("- **" + r.ID + "**: " + r.Title)
	if !strings.Contains(strings.ToLower(spec0061), bullet) {
		t.Errorf("%s: spec 0061 has no bullet %q", r.ID, bullet)
	}
	if r.Level != Should || !r.Waivable() {
		t.Errorf("%s level = %s, ADR 0027 says SHOULD", r.ID, r.Level)
	}
	if !strings.Contains(r.Source, "spec 0061") {
		t.Errorf("%s source = %q, want spec 0061 named", r.ID, r.Source)
	}
	if string(r.Category) != r.ID[:1] {
		t.Errorf("%s category = %s", r.ID, r.Category)
	}
	for _, p := range r.Parts {
		if !strings.Contains(r.Check, p.Check) || p.Run == nil {
			t.Errorf("%s %s part %q is not part of its check, or has no check", r.ID, p.Kind, p.Check)
		}
	}
}
