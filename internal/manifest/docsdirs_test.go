package manifest

import (
	"strings"
	"testing"
)

const layoutBase = "standard: prod-go\nversion: v1\ndevelopment:\n  docs_layout: standard\n"

// TestDocsDirs: an adopted path replaces its default; the optional
// directories are in the layout only when selected (spec 0034 §1).
func TestDocsDirs(t *testing.T) {
	m, err := Parse([]byte(layoutBase + "  decisions_dir: docs/adr\n  specs_dir: rfcs\n  docs_operations: on\n  operations_dir: ops\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := DocsDirs{Specs: "rfcs", Architecture: "docs/architecture", Decisions: "docs/adr", Operations: "ops"}
	if got := m.Development.DocsDirs(); got != want {
		t.Errorf("DocsDirs() = %+v, want %+v", got, want)
	}
	var keys []string
	for _, k := range m.Development.AdoptedDirs() {
		keys = append(keys, k.Key+"="+m.Development.DocsDir(k.Key))
	}
	if got := strings.Join(keys, " "); got != "specs_dir=rfcs decisions_dir=docs/adr operations_dir=ops" {
		t.Errorf("adopted %s", got)
	}
	if got := (*Development)(nil).DocsDirs(); got != (DocsDirs{Specs: "docs/specs", Architecture: "docs/architecture", Decisions: "docs/decisions"}) {
		t.Errorf("defaults %+v", got)
	}
}

func TestDocsDirsInvalid(t *testing.T) {
	for name, tc := range map[string]struct{ yaml, want string }{
		"backslash": {layoutBase + "  decisions_dir: docs\\adr\n",
			`development.decisions_dir (docs\adr): path "docs\\adr" must use forward slashes`},
		"escapes": {layoutBase + "  specs_dir: ../specs\n",
			`development.specs_dir (../specs): path "../specs" is outside the repository`},
		"unclean": {layoutBase + "  specs_dir: docs//specs\n",
			`development.specs_dir (docs//specs): path "docs//specs" must be clean ("docs/specs")`},
		"root": {layoutBase + "  specs_dir: .\n",
			"development.specs_dir (.): the repository root cannot be a docs directory"},
		"no layout": {"standard: prod-go\nversion: v1\ndevelopment:\n  decisions_dir: docs/adr\n",
			"development.decisions_dir (docs/adr): requires development.docs_layout"},
		"no docs_operations": {layoutBase + "  operations_dir: ops\n",
			"development.operations_dir (ops): requires development.docs_operations: on"},
		"same as a default": {layoutBase + "  specs_dir: docs/decisions\n",
			"development.decisions_dir (docs/decisions): overlaps development.specs_dir (docs/decisions)"},
		"nested": {layoutBase + "  architecture_dir: docs/specs/arch\n",
			"development.architecture_dir (docs/specs/arch): overlaps development.specs_dir (docs/specs)"},
	} {
		_, err := Parse([]byte(tc.yaml))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to contain %q", name, err, tc.want)
		}
	}
	// An unselected optional directory's default overlaps nothing.
	if _, err := Parse([]byte(layoutBase + "  specs_dir: docs/operations\n")); err != nil {
		t.Errorf("docs/operations is free while docs_operations is off: %v", err)
	}
}
