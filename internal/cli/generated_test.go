package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/state"
)

const (
	tsGenerated = "standard: prod-ts\nversion: v1\ngenerated:\n  - src/contracts/**\n"
	tsPlain     = "standard: prod-ts\nversion: v1\n"
	monoWeb     = "  - id: web\n    path: apps/web\n    profile: ts\n    generated:\n      - src/contracts/**\n"
	monoAPI     = "  - id: api\n    path: services/api\n    profile: go\n"
	monoHead    = "standard: prod-mono\nversion: v1\ncomponents:\n"
)

const generatedSection = "# vibeconform:begin generated\n" +
	"# Managed by VibeConform: generated: in vibe.yaml. Not formatted.\n" +
	"src/contracts/**\n" +
	"# vibeconform:end generated\n"

// TestGeneratedSection pins spec 0037 acceptance criterion 6, through a
// real prod-ts and prod-mono sync.
func TestGeneratedSection(t *testing.T) {
	t.Run("user lines kept", func(t *testing.T) {
		dir := t.TempDir()
		user := "# mine\nfixtures/\n"
		writeFile(t, dir, ".prettierignore", user)
		writeVibeYAML(t, dir, tsGenerated)
		mustSync(t, dir)
		got := readFile(t, dir, ".prettierignore")
		if !strings.HasPrefix(got, user) || !strings.Contains(got, generatedSection) {
			t.Errorf(".prettierignore = %q", got)
		}
		mustConform(t, dir)
		if out := mustSync(t, dir); !strings.Contains(out, "0 created, 0 updated") {
			t.Errorf("second sync changed something:\n%s", out)
		}

		writeVibeYAML(t, dir, tsPlain)
		out := mustSync(t, dir)
		if !strings.Contains(out, ".prettierignore (section generated): removed (no longer declared in vibe.yaml)") {
			t.Errorf("sync:\n%s", out)
		}
		if got := readFile(t, dir, ".prettierignore"); got != user {
			t.Errorf(".prettierignore = %q, want the user's lines only", got)
		}
		mustConform(t, dir)
	})

	t.Run("created file left empty is deleted", func(t *testing.T) {
		dir := t.TempDir()
		writeVibeYAML(t, dir, tsGenerated)
		mustSync(t, dir)
		if got := readFile(t, dir, ".prettierignore"); got != generatedSection {
			t.Errorf(".prettierignore = %q", got)
		}
		writeVibeYAML(t, dir, "standard: prod-ts\nversion: v1\ngenerated: []\n")
		if out := mustSync(t, dir); !strings.Contains(out, "removed, and the file") {
			t.Errorf("sync:\n%s", out)
		}
		if exists(t, dir, ".prettierignore") {
			t.Error(".prettierignore survived")
		}
		mustConform(t, dir)
	})

	t.Run("modified section is a conflict", func(t *testing.T) {
		dir := t.TempDir()
		writeVibeYAML(t, dir, tsGenerated)
		mustSync(t, dir)
		edited := strings.Replace(readFile(t, dir, ".prettierignore"), "src/contracts/**", "src/contracts/**\nsrc/legacy/**", 1)
		writeFile(t, dir, ".prettierignore", edited)
		writeVibeYAML(t, dir, tsPlain)
		out, err := runSyncIn(t, dir)
		if err == nil || !strings.Contains(out, "conflict") {
			t.Errorf("sync: %v\n%s", err, out)
		}
		if got := readFile(t, dir, ".prettierignore"); got != edited {
			t.Errorf("a modified section was changed: %q", got)
		}
	})

	t.Run("removed component", func(t *testing.T) {
		dir := t.TempDir()
		writeVibeYAML(t, dir, monoHead+monoAPI+monoWeb)
		mustSync(t, dir)
		if got := readFile(t, dir, "apps/web/.prettierignore"); got != generatedSection {
			t.Errorf("apps/web/.prettierignore = %q", got)
		}
		writeVibeYAML(t, dir, monoHead+monoAPI)
		out := mustSync(t, dir)
		if !strings.Contains(out, "apps/web/.prettierignore (section generated): removed") {
			t.Errorf("sync:\n%s", out)
		}
		if exists(t, dir, "apps/web/.prettierignore") {
			t.Error("apps/web/.prettierignore survived")
		}
		s, _ := state.Load(dir)
		if _, ok := s.Sections[state.SectionKey{Path: "apps/web/.prettierignore", ID: "generated"}]; ok {
			t.Error("the section is still recorded")
		}
	})
}

// TestCheckGenerated pins spec 0037 acceptance criterion 3: generated:
// where the standard cannot honour it is refused before anything resolves.
func TestCheckGenerated(t *testing.T) {
	for name, c := range map[string]struct{ doc, want string }{
		"prod-mono top-level": {monoHead + monoAPI + "generated:\n  - src/gen/**\n", "per component"},
		"prod-go top-level":   {"standard: prod-go\nversion: v1\ngenerated:\n  - internal/gen/**\n", "Code generated"},
		"profile go":          {monoHead + "  - id: api\n    path: services/api\n    profile: go\n    generated:\n      - internal/gen/**\n", "Code generated"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeVibeYAML(t, dir, c.doc)
			out, err := runSyncIn(t, dir)
			wantExit(t, err, 1, out)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to contain %q", err, c.want)
			}
			if exists(t, dir, ".vibe/state.yaml") {
				t.Error("sync wrote state")
			}
		})
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
