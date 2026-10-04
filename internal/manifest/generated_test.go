package manifest

import (
	"slices"
	"strings"
	"testing"
)

// TestParseGenerated pins spec 0037 acceptance criterion 2's valid forms,
// per component and at the top level.
func TestParseGenerated(t *testing.T) {
	valid := []string{"src/a/**", "src/**/gen/types.ts", "src/a.gen.ts", "src/a/b.py", "gen/contracts_v1.py"}
	var list strings.Builder
	for _, g := range valid {
		list.WriteString("      - " + g + "\n")
	}
	m, err := Parse([]byte("standard: prod-mono\nversion: v1\ncomponents:\n  - id: web\n    path: apps/web\n    profile: ts\n    generated:\n" + list.String()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !slices.Equal(m.Components[0].Generated, valid) {
		t.Errorf("component generated = %q, want %q", m.Components[0].Generated, valid)
	}

	m, err = Parse([]byte("standard: prod-py\nversion: v1\ngenerated:\n  - src/example/contracts/**\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !slices.Equal(m.Generated, []string{"src/example/contracts/**"}) {
		t.Errorf("generated = %q", m.Generated)
	}

	for _, doc := range []string{"standard: prod-py\nversion: v1\ngenerated: []\n", "standard: prod-py\nversion: v1\n"} {
		if m, err := Parse([]byte(doc)); err != nil || len(m.Generated) != 0 {
			t.Errorf("%q: generated = %q, err = %v", doc, m.Generated, err)
		}
	}
}

// TestParseRejectsGenerated pins spec 0037 acceptance criterion 2's
// rejections: every row of §2's table, a duplicate, and profile go. Each
// error names the component and the index of the offending entry.
func TestParseRejectsGenerated(t *testing.T) {
	rejected := []string{
		`""`,
		`'src\gen\x.py'`,
		"/src/gen",
		"C:/gen",
		"./src/gen/x.py",
		"src/gen/",
		"src//gen/x.py",
		"../shared/gen.py",
		"src/../x",
		".",
		"contracts.py",
		"'*/gen.py'",
		"'**/x.py'",
		"'**'",
		"'src/gen/*.py'",
		"'src/gen/[a-z].py'",
		"'src/{a,b}/x.ts'",
		"'!src/gen'",
		"'src/gen/**.py'",
		"'src/gen x/a.py'",
	}
	for _, g := range rejected {
		t.Run(g, func(t *testing.T) {
			doc := "standard: prod-mono\nversion: v1\ncomponents:\n  - id: web\n    path: apps/web\n    profile: ts\n    generated:\n      - src/ok/**\n      - " + g + "\n"
			_, err := Parse([]byte(doc))
			if err == nil {
				t.Fatal("Parse accepted it")
			}
			if !strings.Contains(err.Error(), "components[0] (web).generated[1]") {
				t.Errorf("error does not name the component and index: %v", err)
			}
		})
	}

	cases := map[string]struct{ doc, want string }{
		"duplicate":             {"standard: prod-py\nversion: v1\ngenerated:\n  - src/a/**\n  - src/a/**\n", "generated[1]: duplicate"},
		"top-level bad":         {"standard: prod-py\nversion: v1\ngenerated:\n  - x.py\n", "generated[0]"},
		"profile go":            {"standard: prod-mono\nversion: v1\ncomponents:\n  - id: api\n    path: services/api\n    profile: go\n    generated:\n      - internal/gen/**\n", "Code generated"},
		"unknown component key": {"standard: prod-mono\nversion: v1\ncomponents:\n  - id: web\n    path: apps/web\n    profile: ts\n    generate: [src/a/**]\n", "generate"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.doc))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to contain %q", err, c.want)
			}
		})
	}
}
