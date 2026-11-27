package contract

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/testkit"
)

// The registry quotes spec 0051 (spec 0052, "Rule registry and results"):
// each rule's sentence, check, level and source must match the spec's
// rule table. The check is derived from the parts (spec 0073), so this
// also holds each part's lead and check text to the spec. Every part of
// every rule has a check function. The table states the latest contract,
// so each rule is read as it is under Latest (spec 0074); a revision
// gives one check text for each part.
func TestRegistryQuotesSpec0051(t *testing.T) {
	rows := spec0051Rows(t)
	must, from0051 := 0, 0
	for _, r := range Rules() {
		for _, p := range r.Parts {
			if p.Run == nil && p.RunLast == nil {
				t.Errorf("%s %s part has no check", r.ID, p.Kind)
			}
		}
		for _, v := range r.Revisions {
			if len(v.Checks) != len(r.Parts) {
				t.Errorf("%s revision %s has %d checks for %d parts", r.ID, v.Since, len(v.Checks), len(r.Parts))
			}
		}
		r = r.At(Latest)
		if r.Since != "" {
			checkSpec0061Rule(t, r)
			continue
		}
		row, ok := rows[r.ID]
		if !ok {
			t.Errorf("%s is not in spec 0051 and has no Since", r.ID)
			continue
		}
		from0051++
		checkSpec0051Rule(t, r, row)
		if r.LevelAt(Latest) == Must {
			must++
		}
	}
	if must != 16 || from0051 != 25 {
		t.Errorf("%d rules of spec 0051, %d MUST; want 25 and 16", from0051, must)
	}
}

// spec0051Rows reads spec 0051's rule table: each row's cells (ID,
// sentence, check, level, source) by rule ID.
func spec0051Rows(t *testing.T) map[string][]string {
	t.Helper()
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
	return rows
}

// checkSpec0051Rule checks a rule of spec 0051's table against its row:
// its sentence, check, level, source and category, and that its check
// names the kind of each part.
func checkSpec0051Rule(t *testing.T, r Rule, row []string) {
	t.Helper()
	if r.Title != row[1] {
		t.Errorf("%s title = %q, spec says %q", r.ID, r.Title, row[1])
	}
	if r.Check() != row[2] {
		t.Errorf("%s check = %q, spec says %q", r.ID, r.Check(), row[2])
	}
	if string(r.LevelAt(Latest)) != row[3] {
		t.Errorf("%s level = %s, spec says %s", r.ID, r.LevelAt(Latest), row[3])
	}
	if r.Source != row[4] {
		t.Errorf("%s source = %q, spec says %q", r.ID, r.Source, row[4])
	}
	if string(r.Category) != r.ID[:1] {
		t.Errorf("%s category = %s", r.ID, r.Category)
	}
	if r.WaivableAt(Latest) != (r.LevelAt(Latest) == Should) {
		t.Errorf("%s waivable = %v", r.ID, r.WaivableAt(Latest))
	}
	for _, p := range r.Parts {
		if !strings.Contains(strings.ToLower(r.Check()), string(p.Kind)) {
			t.Errorf("%s has a %s part its check does not name", r.ID, p.Kind)
		}
	}
}

// spec0061Solution is the "Solution" section of spec 0061, the list of
// the rules contract 1.2.0 adds.
var spec0061Solution = func() string {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "0061-contract-1-2-quality-probes.md"))
	if err != nil {
		return ""
	}
	const heading = "\n## Solution\n"
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	i := strings.Index(s, heading)
	if i < 0 {
		return ""
	}
	s = s[i+len(heading):]
	if j := strings.Index(s, "\n## "); j >= 0 {
		s = s[:j]
	}
	return s
}()

// checkSpec0061Rule checks a rule marked with Since, a rule that spec
// 0051's table does not hold: it was added in contract 1.2.0, spec
// 0061's Solution lists it as "- **ID**:", it is a SHOULD rule (spec
// 0061 and ADR 0027: a MINOR adds SHOULD rules only), its source names
// spec 0061. Its check text is pinned by the explain goldens.
func checkSpec0061Rule(t *testing.T, r Rule) {
	t.Helper()
	if spec0061Solution == "" {
		t.Fatal("cannot read the Solution section of spec 0061")
	}
	if r.Since != "1.2.0" {
		t.Errorf("%s since = %q, want 1.2.0", r.ID, r.Since)
	}
	if !strings.Contains(spec0061Solution, "\n- **"+r.ID+"**: ") {
		t.Errorf("%s: spec 0061's Solution does not list it", r.ID)
	}
	if r.LevelAt(Latest) != Should || !r.WaivableAt(Latest) {
		t.Errorf("%s level = %s, ADR 0027 says SHOULD", r.ID, r.Level)
	}
	if !strings.Contains(r.Source, "spec 0061") {
		t.Errorf("%s source = %q, want spec 0061 named", r.ID, r.Source)
	}
	if string(r.Category) != r.ID[:1] {
		t.Errorf("%s category = %s", r.ID, r.Category)
	}
}

// The registry has one last part (spec 0071): R-01's execution part,
// which reads A-02's deny verdict and the vibe calls of every other
// part. A static audit of Rules() does not panic: the runner validates
// the rule list in every audit, --static included.
func TestRegistryLastPartIsR01Execution(t *testing.T) {
	var last []partKey
	for _, r := range Rules() {
		for _, p := range r.Parts {
			if p.RunLast != nil {
				last = append(last, partKey{rule: r.ID, kind: p.Kind})
			}
		}
	}
	if want := []partKey{{rule: "R-01", kind: Execution}}; !slices.Equal(last, want) {
		t.Errorf("the last parts are %v, want %v", last, want)
	}

	cfg, repo := staticFixture(t)
	rep := Audit(context.Background(), cfg, repo, Options{Static: true})
	if got, want := len(rep.Rules), len(Rules()); got != want {
		t.Errorf("the static audit decided %d rules, want %d", got, want)
	}
}

// staticFixture is a Git repository holding only a go.mod and a
// vibe.yaml, and its parsed configuration: enough for a static audit.
func staticFixture(t *testing.T) (*Config, *Repo) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	r := testkit.NewRepo(t, "repo")
	data := []byte("contract: 1.2.0\nplatforms: [linux]\nagents: [claude]\n")
	for name, content := range map[string][]byte{"vibe.yaml": data, "go.mod": []byte("module fixture\n\ngo 1.22\n")} {
		if err := os.WriteFile(filepath.Join(r.Dir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r.Init(t)
	r.Git(t, "add", "-A")
	ctx := context.Background()
	repo, err := OpenRepo(ctx, r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse("vibe.yaml", data, repo)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, repo
}
