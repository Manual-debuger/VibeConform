package contract_test

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The CI findings goldens (spec 0067, "Characterization first"): every
// case of the C-01 to C-05, D-03 and R-01 tables of both providers
// compares the rule's full findings (file, message and fix, in report
// order) with a golden, so that a refactor of the CI rules cannot change
// their wording or order unless a test fails.
//
// One golden file holds one table: testdata/ci/<provider>/<rule>.golden,
// such as testdata/ci/github/C-02.golden. In it, each case is a section
// that starts with the line "== <case name>", in table order, followed
// by the case's findings, or by "no findings". The -update flag
// (explain_golden_test.go) rewrites the sections of the cases that ran;
// review the diff before committing it. A refactor that changes a golden
// is a regression, not a reason to run -update.

// The providers, as the directories under testdata/ci name them.
const (
	goldenGitHub = "github"
	goldenGitLab = "gitlab"
)

// ciRuleCases is contracttest.RunRuleCases with each case's findings
// compared with its section of the table's golden.
func ciRuleCases(t *testing.T, provider, id string, cases []contracttest.RuleCase) {
	t.Helper()
	contracttest.RunRuleCases(t, id, cases, ciGolden(t, provider, id, cases))
}

// ciPartCases is contracttest.RunPartCases with each case's findings
// compared with its section of the table's golden.
func ciPartCases(t *testing.T, provider, id string, kind contract.PartKind, cases []contracttest.RuleCase) {
	t.Helper()
	contracttest.RunPartCases(t, id, kind, cases, ciGolden(t, provider, id, cases))
}

// ciGolden reads the golden of one table and returns the check that
// compares a case's findings with its section. A section with no case in
// the table fails t. With -update, the file is rewritten once every case
// has run: the cases that ran get their findings, the others keep their
// section, and sections with no case are dropped.
func ciGolden(t *testing.T, provider, id string, cases []contracttest.RuleCase) contracttest.CaseCheck {
	t.Helper()
	path := filepath.Join("testdata", "ci", provider, id+".golden")
	var names []string
	for _, tc := range cases {
		if slices.Contains(names, tc.Name) {
			t.Fatalf("%s: two cases are named %q", path, tc.Name)
		}
		names = append(names, tc.Name)
	}
	want := readGoldenSections(t, path)
	var mu sync.Mutex
	got := map[string]string{}
	if *updateGoldens {
		// Cleanup runs after the parallel case subtests have finished.
		t.Cleanup(func() { writeGoldenSections(t, path, names, want, got) })
	} else {
		for name := range want {
			if !slices.Contains(names, name) {
				t.Errorf("%s has a section %q, which no case of the table has; delete it, or rewrite the file with -update", path, name)
			}
		}
	}
	return func(t *testing.T, tc contracttest.RuleCase, rep *contract.Report) {
		t.Helper()
		text := findingsGolden(t, contracttest.RuleResult(t, rep, id))
		if *updateGoldens {
			mu.Lock()
			got[tc.Name] = text
			mu.Unlock()
			return
		}
		w, ok := want[tc.Name]
		switch {
		case !ok:
			t.Errorf("%s has no section %q (rewrite it with -update, then review the diff)", path, tc.Name)
		case w != text:
			t.Errorf("%s, section %q, differs\ngot:\n%s\nwant:\n%s", path, tc.Name, text, w)
		}
	}
}

// findingsGolden is a rule's findings as a golden section's body: each
// finding as its file, message and fix lines, a blank line between two
// findings, or "no findings".
func findingsGolden(t *testing.T, rr contract.RuleResult) string {
	t.Helper()
	findings := rr.Findings()
	if len(findings) == 0 {
		return "no findings\n"
	}
	blocks := make([]string, 0, len(findings))
	for _, f := range findings {
		blocks = append(blocks, "file: "+f.File+"\nmessage: "+f.Message+"\nfix: "+f.Fix+"\n")
	}
	body := strings.Join(blocks, "\n")
	if strings.HasPrefix(body, sectionMark) || strings.Contains(body, "\n"+sectionMark) {
		t.Fatalf("a finding of %s has a line that starts with %q, which the golden reads as a section", rr.Rule.ID, sectionMark)
	}
	return body
}

// sectionMark starts the line that names a golden section's case.
const sectionMark = "== "

// readGoldenSections reads a golden's sections by case name. A missing
// file has no sections.
func readGoldenSections(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}
	}
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("\r\n")) {
		t.Fatalf("%s has CRLF line endings; it must be checked out LF (.gitattributes)", path)
	}
	text := strings.TrimSuffix(string(data), "\n")
	if !strings.HasPrefix(text, sectionMark) {
		t.Fatalf("%s does not start with a %q line", path, sectionMark)
	}
	sections := map[string]string{}
	for _, s := range strings.Split(strings.TrimPrefix(text, sectionMark), "\n\n"+sectionMark) {
		name, body, _ := strings.Cut(s, "\n")
		if _, dup := sections[name]; dup {
			t.Fatalf("%s has two sections %q", path, name)
		}
		sections[name] = body + "\n"
	}
	return sections
}

// writeGoldenSections writes a golden with one section per case, in
// table order: got's findings for the cases that ran, old's section for
// the others. A case with neither has no section.
func writeGoldenSections(t *testing.T, path string, names []string, old, got map[string]string) {
	t.Helper()
	sections := maps.Clone(old)
	maps.Copy(sections, got)
	var parts []string
	for _, name := range names {
		if body, ok := sections[name]; ok {
			parts = append(parts, sectionMark+name+"\n"+body)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(parts, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}
