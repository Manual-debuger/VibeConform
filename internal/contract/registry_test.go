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

	must := 0
	for _, r := range Rules() {
		row, ok := rows[r.ID]
		if spec0061Rules[r.ID] {
			// Contract 1.2.0's rules are spec 0061's, all SHOULD.
			row, ok = []string{r.ID, r.Title, r.Check, string(Should), "spec 0061, ADR 0027"}, true
		}
		if !ok {
			t.Errorf("%s is not in spec 0051", r.ID)
			continue
		}
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
	if want := 25 + len(spec0061Rules); must != 16 || len(Rules()) != want {
		t.Errorf("%d rules, %d MUST; want %d and 16", len(Rules()), must, want)
	}
}

// spec0061Rules are the rules contract 1.2.0 adds (spec 0061). They are
// not in spec 0051's table.
var spec0061Rules = map[string]bool{
	"V-07": true,
}
