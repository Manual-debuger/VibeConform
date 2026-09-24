package manifest

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	m, err := Parse([]byte("standard: prod-go\nversion: v1\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if m.Standard != "prod-go" || m.Version != "v1" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestParseMissingStandard(t *testing.T) {
	if _, err := Parse([]byte("version: v1\n")); err == nil {
		t.Fatal("expected error for missing standard, got nil")
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Fatal("expected error for empty manifest, got nil")
	}
}

func TestParseInvalidYAML(t *testing.T) {
	if _, err := Parse([]byte("standard: [unterminated\n")); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestNewMissingStandard(t *testing.T) {
	if _, err := New("", "v1"); err == nil {
		t.Fatal("expected error for missing standard, got nil")
	}
}

func TestNewMarshalParseRoundTrip(t *testing.T) {
	m, err := New("prod-go", "v1")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := m.Marshal()
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, m)
	}
}

func TestParseComponents(t *testing.T) {
	m, err := Parse([]byte(`standard: prod-mono
version: v1
components:
  - id: api
    path: services/api
    profile: go
  - id: web
    path: apps/web
    profile: ts
  - id: worker-2
    path: services/worker
    profile: py
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	want := []Component{
		{ID: "api", Path: "services/api", Profile: ProfileGo},
		{ID: "web", Path: "apps/web", Profile: ProfileTS},
		{ID: "worker-2", Path: "services/worker", Profile: ProfilePy},
	}
	if !reflect.DeepEqual(m.Components, want) {
		t.Fatalf("components = %+v, want %+v", m.Components, want)
	}
}

func TestParseRejects(t *testing.T) {
	const head = "standard: prod-mono\nversion: v1\ncomponents:\n"
	comp := func(id, path, profile string) string {
		return "  - id: " + id + "\n    path: '" + path + "'\n    profile: " + profile + "\n"
	}
	cases := map[string]struct {
		doc, want string
	}{
		"unknown top-level key": {"standard: prod-go\nrevision: v1\n", "revision"},
		"unknown component key": {head + comp("api", "a", "go") + "    depends_on: [web]\n", "depends_on"},
		"bad id":                {head + comp("Api", "a", "go"), "id"},
		"id with underscore":    {head + comp("my_api", "a", "go"), "id"},
		"reserved id":           {head + comp("verify", "a", "go"), "reserved"},
		"duplicate id":          {head + comp("api", "a", "go") + comp("api", "b", "go"), "duplicate id"},
		"unknown profile":       {head + comp("api", "a", "rust"), "profile"},
		"missing path":          {head + "  - id: api\n    profile: go\n", "path is required"},
		"root path":             {head + comp("api", ".", "go"), "repository root"},
		"parent path":           {head + comp("api", "../x", "go"), "outside"},
		"absolute path":         {head + comp("api", "/x", "go"), "relative"},
		"drive path":            {head + comp("api", "C:/x", "go"), "relative"},
		"backslash path":        {head + comp("api", `a\b`, "go"), "forward slashes"},
		"unclean path":          {head + comp("api", "a/./b", "go"), "clean"},
		"trailing slash":        {head + comp("api", "a/", "go"), "clean"},
		"same path":             {head + comp("api", "a", "go") + comp("web", "a", "ts"), "overlaps"},
		"nested path":           {head + comp("api", "a", "go") + comp("web", "a/b", "ts"), "overlaps"},
		"enclosing path":        {head + comp("web", "a/b", "ts") + comp("api", "a", "go"), "overlaps"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.doc))
			if err == nil {
				t.Fatalf("Parse accepted:\n%s", tc.doc)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// A sibling whose name merely starts with another component's path is not
// inside it.
func TestParseSiblingPrefixIsNotOverlap(t *testing.T) {
	_, err := Parse([]byte("standard: prod-mono\nversion: v1\ncomponents:\n" +
		"  - {id: a, path: svc/api, profile: go}\n  - {id: b, path: svc/api2, profile: go}\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
}
