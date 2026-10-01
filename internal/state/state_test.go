package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsEmptyState(t *testing.T) {
	dir := t.TempDir()

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s == nil || s.Resources == nil {
		t.Fatal("Load: expected non-nil State with non-nil Resources map")
	}
	if len(s.Resources) != 0 {
		t.Fatalf("Load: expected empty Resources, got %v", s.Resources)
	}
}

func TestLoadMalformedFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o755); err != nil {
		t.Fatalf("seeding .vibe dir: %v", err)
	}
	path := filepath.Join(dir, ".vibe", "state.yaml")
	if err := os.WriteFile(path, []byte("resources: [not, a, map]"), 0o644); err != nil {
		t.Fatalf("seeding state.yaml: %v", err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("Load: expected error for malformed state.yaml, got nil")
	}
}

func TestLoadValidFileParsesResources(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o755); err != nil {
		t.Fatalf("seeding .vibe dir: %v", err)
	}
	path := filepath.Join(dir, ".vibe", "state.yaml")
	content := "resources:\n  .golangci.yml:\n    sha256: deadbeef\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seeding state.yaml: %v", err)
	}

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := s.Resources[".golangci.yml"]
	if !ok {
		t.Fatalf("Load: expected .golangci.yml entry, got %v", s.Resources)
	}
	if got.SHA256 != "deadbeef" {
		t.Errorf("Load: SHA256 = %q, want %q", got.SHA256, "deadbeef")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := &State{Resources: map[string]ResourceState{
		".golangci.yml":            {SHA256: "aaa"},
		".github/workflows/ci.yml": {SHA256: "bbb"},
		".vscode/tasks.json": {Created: true, Elements: map[string]ElementState{
			"tasks/task verify": {SHA256: "ccc"},
		}},
	}}

	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Resources) != len(want.Resources) {
		t.Fatalf("Load: got %v, want %v", got.Resources, want.Resources)
	}
	for path, rs := range want.Resources {
		if !reflect.DeepEqual(got.Resources[path], rs) {
			t.Errorf("Load: %s = %v, want %v", path, got.Resources[path], rs)
		}
	}
}

func TestSaveCreatesStateDirectory(t *testing.T) {
	dir := t.TempDir()

	if err := Save(dir, &State{Resources: map[string]ResourceState{".golangci.yml": {SHA256: "abc"}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".vibe", "state.yaml")); err != nil {
		t.Fatalf("expected .vibe/state.yaml to exist: %v", err)
	}
}

func TestSaveReplacesExistingState(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, &State{Resources: map[string]ResourceState{"old.yml": {SHA256: "old"}}}); err != nil {
		t.Fatalf("seeding Save: %v", err)
	}

	if err := Save(dir, &State{Resources: map[string]ResourceState{"new.yml": {SHA256: "new"}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, stale := got.Resources["old.yml"]; stale {
		t.Errorf("Save: stale entry survived a rewrite: %v", got.Resources)
	}
	if got.Resources["new.yml"].SHA256 != "new" {
		t.Errorf("Save: new.yml = %v, want sha256 %q", got.Resources["new.yml"], "new")
	}
}

func TestSaveEmptyStateLoadsAsEmptyMap(t *testing.T) {
	dir := t.TempDir()

	if err := Save(dir, &State{Resources: map[string]ResourceState{}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Resources == nil {
		t.Fatal("Load: Resources map should be non-nil after saving an empty state")
	}
	if len(got.Resources) != 0 {
		t.Errorf("Load: got %v, want empty", got.Resources)
	}
}

// TestLoadSchema1FileLeavesProvenanceZero pins backward compatibility with
// every .vibe/state.yaml written before spec 0019 — which is to say, every
// one that exists today, in this repository and in every adopter's.
//
// The fields must come back zero rather than defaulted. An empty
// VibeVersion means "nobody recorded this", and CompareWriters is built to
// refuse to order it; a plausible-looking default here would travel all the
// way to a confident verdict about a repository nothing is known about.
func TestLoadSchema1FileLeavesProvenanceZero(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o750); err != nil {
		t.Fatalf("seeding .vibe dir: %v", err)
	}
	// Exactly the shape spec 0006 wrote: a bare resources map.
	schema1 := "resources:\n    .golangci.yml:\n        sha256: abc123\n"
	path := filepath.Join(dir, ".vibe", "state.yaml")
	if err := os.WriteFile(path, []byte(schema1), 0o600); err != nil {
		t.Fatalf("seeding state: %v", err)
	}

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load of a schema-1 file must not fail: %v", err)
	}
	if s.Schema != 0 {
		t.Errorf("Schema = %d, want 0 (absent means pre-0019, not version 0)", s.Schema)
	}
	if s.VibeVersion != "" {
		t.Errorf("VibeVersion = %q, want empty", s.VibeVersion)
	}
	if s.Standard != "" {
		t.Errorf("Standard = %q, want empty", s.Standard)
	}
	if got := s.Resources[".golangci.yml"].SHA256; got != "abc123" {
		t.Errorf("resource hash = %q, want abc123 — the resources map must "+
			"still parse exactly as before", got)
	}

	// The whole point: an unknown writer is unorderable, so no direction is
	// claimed about a repository whose state predates provenance.
	if got := CompareWriters(s.VibeVersion, "v0.3.0"); got != WriterUnknown {
		t.Errorf("CompareWriters(%q, v0.3.0) = %v, want Unknown", s.VibeVersion, got)
	}
}

// TestSaveRecordsProvenance is the counterpart: a file this version writes
// carries the schema and the writer, so the next run can tell whether it is
// older or newer than whatever produced the repository.
func TestSaveRecordsProvenance(t *testing.T) {
	dir := t.TempDir()
	in := &State{
		Schema:      SchemaVersion,
		VibeVersion: "v0.3.0",
		Standard:    "prod-go/v1",
		Resources:   map[string]ResourceState{".golangci.yml": {SHA256: "abc123"}},
	}
	if err := Save(dir, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Schema != SchemaVersion {
		t.Errorf("Schema = %d, want %d", out.Schema, SchemaVersion)
	}
	if out.VibeVersion != "v0.3.0" {
		t.Errorf("VibeVersion = %q, want v0.3.0", out.VibeVersion)
	}
	if out.Standard != "prod-go/v1" {
		t.Errorf("Standard = %q, want prod-go/v1", out.Standard)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	// No timestamp: spec 0019 rules one out because it would rewrite the
	// file on every sync and churn its diff for no reconciliation benefit.
	for _, banned := range []string{"synced_at", "timestamp", "written_at"} {
		if strings.Contains(string(raw), banned) {
			t.Errorf("state file contains %q; state must stay content-derived "+
				"and deterministic:\n%s", banned, raw)
		}
	}
}

// TestSchema4Layout pins spec 0029 §8: one list of records, sorted by
// path then section ID, each naming its ownership, with only the fields
// that ownership needs.
func TestSchema4Layout(t *testing.T) {
	dir := t.TempDir()
	in := &State{
		Schema: SchemaVersion,
		Resources: map[string]ResourceState{
			"Taskfile.yml": {SHA256: "abc"},
			".vscode/tasks.json": {Created: true, Elements: map[string]ElementState{
				"tasks/task fmt": {SHA256: "def"},
			}},
		},
		Sections: map[SectionKey]SectionState{
			{Path: "AGENTS.md", ID: "workflow"}:          {SHA256: "ghi"},
			{Path: ".gitattributes", ID: "line-endings"}: {SHA256: "jkl", Created: true},
		},
	}
	if err := Save(dir, in); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := `schema: 4
resources:
    - path: .gitattributes
      section_id: line-endings
      ownership: managed-section
      created: true
      sha256: jkl
    - path: .vscode/tasks.json
      ownership: structured-patch
      created: true
      elements:
        tasks/task fmt:
            sha256: def
    - path: AGENTS.md
      section_id: workflow
      ownership: managed-section
      sha256: ghi
    - path: Taskfile.yml
      ownership: generated
      sha256: abc
`
	if string(data) != want {
		t.Errorf("state file\n%s\nwant\n%s", data, want)
	}

	out, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Resources, in.Resources) || !reflect.DeepEqual(out.Sections, in.Sections) {
		t.Errorf("round trip\n%+v %+v\nwant\n%+v %+v", out.Resources, out.Sections, in.Resources, in.Sections)
	}
	if err := Save(dir, out); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(data) {
		t.Errorf("a second save changed the file:\n%s", again)
	}
}

// TestLoadMigratesEarlierSchemas: every layout written before spec 0029
// loads into the same in-memory state, which then saves as schema 4.
func TestLoadMigratesEarlierSchemas(t *testing.T) {
	for name, data := range map[string]string{
		"schema 1": "resources:\n    Taskfile.yml:\n        sha256: abc\n",
		"schema 2": "schema: 2\nvibe_version: v0.3.0\nstandard: prod-go/v1\nresources:\n    Taskfile.yml:\n        sha256: abc\n",
		"schema 3": "schema: 3\nresources:\n    Taskfile.yml:\n        sha256: abc\n" +
			"    .vscode/tasks.json:\n        created: true\n        elements:\n            tasks/task fmt:\n                sha256: def\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := seed(t, data)
			s, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Resources["Taskfile.yml"]; got.SHA256 != "abc" || got.Elements != nil {
				t.Errorf("Taskfile.yml = %+v, want a generated entry with sha256 abc", got)
			}
			if name == "schema 3" {
				want := ResourceState{Created: true, Elements: map[string]ElementState{"tasks/task fmt": {SHA256: "def"}}}
				if got := s.Resources[".vscode/tasks.json"]; !reflect.DeepEqual(got, want) {
					t.Errorf(".vscode/tasks.json = %+v, want %+v", got, want)
				}
			}
			if len(s.Sections) != 0 {
				t.Errorf("sections %v, want none", s.Sections)
			}

			s.Schema = SchemaVersion
			if err := Save(dir, s); err != nil {
				t.Fatal(err)
			}
			out, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), "schema: 4\n") ||
				!strings.Contains(string(out), "    - path: Taskfile.yml\n      ownership: generated\n      sha256: abc\n") {
				t.Errorf("saved as\n%s\nwant schema 4 records", out)
			}
		})
	}
}

func TestLoadRejectsBadRecords(t *testing.T) {
	for name, tc := range map[string]struct{ data, want string }{
		"duplicate": {
			"schema: 4\nresources:\n  - {path: a, ownership: generated, sha256: x}\n  - {path: a, ownership: generated, sha256: y}\n",
			"resources[1] (a): recorded twice",
		},
		"duplicate section": {
			"schema: 4\nresources:\n  - {path: a, section_id: s, ownership: managed-section, sha256: x}\n" +
				"  - {path: a, section_id: s, ownership: managed-section, sha256: y}\n",
			"resources[1] (a): recorded twice",
		},
		"unknown ownership": {
			"schema: 4\nresources:\n  - {path: a, ownership: owned, sha256: x}\n",
			`resources[0] (a): unknown ownership "owned"`,
		},
		"section id on a file": {
			"schema: 4\nresources:\n  - {path: a, section_id: s, ownership: generated, sha256: x}\n",
			"resources[0] (a): section_id on a generated entry",
		},
		"section without id": {
			"schema: 4\nresources:\n  - {path: a, ownership: managed-section, sha256: x}\n",
			"resources[0] (a): a managed-section entry needs a section_id",
		},
		"no path": {
			"schema: 4\nresources:\n  - {ownership: generated, sha256: x}\n",
			"resources[0]: path is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(seed(t, tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestSectionsShareAFile: two sections of one file are two entries, told
// apart by their IDs.
func TestSectionsShareAFile(t *testing.T) {
	dir := t.TempDir()
	in := &State{Resources: map[string]ResourceState{}, Sections: map[SectionKey]SectionState{
		{Path: "AGENTS.md", ID: "workflow"}: {SHA256: "a"},
		{Path: "AGENTS.md", ID: "docs"}:     {SHA256: "b"},
	}}
	if err := Save(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Sections, in.Sections) {
		t.Errorf("sections %v, want %v", out.Sections, in.Sections)
	}
}

func seed(t *testing.T, data string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vibe", "state.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
