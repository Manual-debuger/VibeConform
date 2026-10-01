package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goDocsLayout = "standard: prod-go\nversion: v1\ndevelopment:\n  docs_layout: standard\n"

// TestDocsLayoutOnBareRepository: both seed files are created, and both
// leave again when the layout is deselected.
func TestDocsLayoutOnBareRepository(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDocsLayout)
	out := mustSync(t, dir)
	for _, want := range []string{
		"docs/README.md (section docs): created",
		"docs/specs/README.md (section specs): created",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	if got := readFile(t, dir, "docs/README.md"); !strings.HasPrefix(got, "<!-- vibeconform:begin docs -->\n## Layout\n") {
		t.Errorf("docs/README.md = %q", got)
	}
	mustConform(t, dir)

	writeVibeYAML(t, dir, goDefaults)
	out = mustSync(t, dir)
	if !strings.Contains(out, "docs/specs/README.md (section specs): removed, and the file (development.docs_layout deselected; nothing else was in it)") {
		t.Errorf("sync:\n%s", out)
	}
	for _, p := range []string{"docs/README.md", "docs/specs/README.md"} {
		if exists(t, dir, p) {
			t.Errorf("%s survived, though VibeConform created it and nothing else is in it", p)
		}
	}
	mustConform(t, dir)
}

// TestDocsOptionalDirs: docs_operations: on, written in vibe.yaml as YAML
// reads it, adds operations/ to the docs index and to AGENTS.md; turning
// it off again updates both sections back (spec 0033).
func TestDocsOptionalDirs(t *testing.T) {
	dir := t.TempDir()
	base := "standard: prod-go\nversion: v1\ndevelopment:\n  workflow: plan-triggered-sdd\n  docs_layout: standard\n"
	writeVibeYAML(t, dir, base+"  docs_operations: on\n")
	mustSync(t, dir)
	if got := readFile(t, dir, "docs/README.md"); !strings.Contains(got, "- [`operations/`](operations/): deploying, running and handling\n") {
		t.Errorf("docs/README.md = %q", got)
	}
	if got := readFile(t, dir, "AGENTS.md"); !strings.Contains(got, "`docs/operations/`") {
		t.Errorf("AGENTS.md does not name docs/operations/:\n%s", got)
	}
	if exists(t, dir, "docs/operations") {
		t.Error("sync created docs/operations/")
	}
	mustConform(t, dir)

	writeVibeYAML(t, dir, base)
	out := mustSync(t, dir)
	for _, want := range []string{"docs/README.md (section docs): updated", "AGENTS.md (section workflow): updated"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "deselected") {
		t.Errorf("turning operations off removed something:\n%s", out)
	}
	if got := readFile(t, dir, "docs/README.md"); strings.Contains(got, "operations/") {
		t.Errorf("docs/README.md still lists operations/: %q", got)
	}
	mustConform(t, dir)

	writeVibeYAML(t, dir, "standard: prod-go\nversion: v1\ndevelopment:\n  docs_operations: on\n")
	out, err := runSyncIn(t, dir)
	wantExit(t, err, 1, out)
	if want := "development.docs_operations: on requires development.docs_layout: standard, which is not selected"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %v, want it to contain %q", err, want)
	}
}

func mkdirs(t *testing.T, dir string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(p)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDocsAdoptedDir: an adopted directory must exist; once it does, the
// docs index and AGENTS.md name it, and nothing is created in it
// (spec 0034).
func TestDocsAdoptedDir(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDocsLayout+"  workflow: always-sdd\n  decisions_dir: docs/adr\n")
	out, err := runSyncIn(t, dir)
	wantExit(t, err, 1, out)
	if want := "development.decisions_dir (docs/adr): not a directory in this repository"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error %v, want it to contain %q", err, want)
	}
	if exists(t, dir, "docs") || exists(t, dir, "AGENTS.md") {
		t.Error("sync wrote files though an adopted directory is missing")
	}

	mkdirs(t, dir, "docs/adr")
	mustSync(t, dir)
	if got := readFile(t, dir, "docs/README.md"); !strings.Contains(got, "- [`adr/`](adr/): decision records") {
		t.Errorf("docs/README.md = %q", got)
	}
	if got := readFile(t, dir, "AGENTS.md"); !strings.Contains(got, "`docs/adr/`") || strings.Contains(got, "docs/decisions") {
		t.Errorf("AGENTS.md:\n%s", got)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "docs", "adr")); len(entries) != 0 {
		t.Errorf("sync wrote into the adopted directory: %v", entries)
	}
	mustConform(t, dir)
}

// TestDocsSpecsDirMoves: moving specs_dir writes the specs section in the
// new directory and removes the old one, by the rules for a deselected
// section: unchanged goes, with a file only it was in; modified stays as
// a conflict (spec 0034 §3).
func TestDocsSpecsDirMoves(t *testing.T) {
	const why = "moved to rfcs/README.md"
	t.Run("unchanged", func(t *testing.T) {
		dir := t.TempDir()
		writeVibeYAML(t, dir, goDocsLayout)
		mustSync(t, dir)
		mkdirs(t, dir, "rfcs")
		writeVibeYAML(t, dir, goDocsLayout+"  specs_dir: rfcs\n")
		if out := runDiffIn(t, dir); !strings.Contains(out, "docs/specs/README.md (section specs): would remove, and the file ("+why+"; nothing else is in it)") {
			t.Errorf("diff:\n%s", out)
		}
		out := mustSync(t, dir)
		for _, want := range []string{
			"rfcs/README.md (section specs): created",
			"docs/specs/README.md (section specs): removed, and the file (" + why + "; nothing else was in it)",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("sync: missing %q\n%s", want, out)
			}
		}
		if exists(t, dir, "docs/specs") {
			t.Error("docs/specs/ survived, though only the moved section was in it")
		}
		mustConform(t, dir)
	})
	t.Run("modified", func(t *testing.T) {
		dir := t.TempDir()
		writeVibeYAML(t, dir, goDocsLayout)
		mustSync(t, dir)
		edited := strings.Replace(readFile(t, dir, "docs/specs/README.md"), "a numeric prefix", "a date prefix", 1)
		if err := os.WriteFile(filepath.Join(dir, "docs", "specs", "README.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		mkdirs(t, dir, "rfcs")
		writeVibeYAML(t, dir, goDocsLayout+"  specs_dir: rfcs\n")
		out, err := runSyncIn(t, dir)
		if err == nil || !strings.Contains(out, "docs/specs/README.md (section specs): conflict: "+why+" but section modified since sync; kept (remove it by hand)") {
			t.Errorf("sync: %v\n%s", err, out)
		}
		if got := readFile(t, dir, "docs/specs/README.md"); got != edited {
			t.Errorf("sync rewrote a modified moved section: %q", got)
		}
	})
}

// TestDocsLayoutKeepsProjectIndex: an existing docs/README.md keeps its
// own text above the section, and keeps it when the layout leaves.
func TestDocsLayoutKeepsProjectIndex(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDocsLayout)
	prose := "# Documentation\n\nPlans live in plans/.\n"
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "README.md"), []byte(prose), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSync(t, dir)
	if got := readFile(t, dir, "docs/README.md"); !strings.HasPrefix(got, prose+"\n<!-- vibeconform:begin docs -->\n") {
		t.Errorf("docs/README.md = %q", got)
	}
	mustConform(t, dir)

	writeVibeYAML(t, dir, goDefaults)
	mustSync(t, dir)
	if got := readFile(t, dir, "docs/README.md"); got != prose {
		t.Errorf("docs/README.md = %q, want the project's text alone", got)
	}
}

// TestDocsLayoutRoutesAgents: the AGENTS.md section names the layout's
// directories only while the layout is selected.
func TestDocsLayoutRoutesAgents(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goWorkflow("plan-triggered-sdd"))
	mustSync(t, dir)
	if got := readFile(t, dir, "AGENTS.md"); strings.Contains(got, "docs/specs/") {
		t.Errorf("AGENTS.md names docs/specs/ without the layout:\n%s", got)
	}

	writeVibeYAML(t, dir, goWorkflow("plan-triggered-sdd")+"  docs_layout: standard\n")
	if out := mustSync(t, dir); !strings.Contains(out, "AGENTS.md (section workflow): updated") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, "AGENTS.md"); !strings.Contains(got, "live in `docs/specs/`") {
		t.Errorf("AGENTS.md does not route to the layout:\n%s", got)
	}
	mustConform(t, dir)
}
