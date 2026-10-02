package cli

import (
	"bytes"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/intelligence/graphify"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

const (
	goGraphify = "standard: prod-go\nversion: v1\nintegrations:\n  intelligence: [graphify]\n"

	// graphifySection is the .gitignore section, byte for byte.
	graphifySection = "# vibeconform:begin graphify\n" +
		"# Managed by VibeConform: integrations.intelligence graphify in vibe.yaml.\n" +
		"graphify-out/\n" +
		"# vibeconform:end graphify\n"
)

// TestGraphifyKeepsUserIgnores pins spec 0035 acceptance criterion 3: the
// section goes below the project's own entries, which keep their bytes,
// CRLF included, and deselecting removes exactly the section.
func TestGraphifyKeepsUserIgnores(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goGraphify)
	user := "bin/\r\n*.log\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustSync(t, dir)
	for _, want := range []string{".gitignore (section graphify): added", graphify.SkillPath + ": created"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync lacks %q:\n%s", want, out)
		}
	}
	if got := readFile(t, dir, ".gitignore"); got != user+"\n"+graphifySection {
		t.Errorf(".gitignore = %q", got)
	}
	mustConform(t, dir)
	if again := mustSync(t, dir); !strings.Contains(again, "0 created, 0 updated, ") || strings.Contains(again, ": added") {
		t.Errorf("a second sync changed something:\n%s", again)
	}

	writeVibeYAML(t, dir, goDefaults)
	out = mustSync(t, dir)
	for _, want := range []string{
		".gitignore (section graphify): removed (graphify deselected)",
		graphify.SkillPath + ": removed (graphify deselected)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("deselecting sync lacks %q:\n%s", want, out)
		}
	}
	if got := readFile(t, dir, ".gitignore"); got != user {
		t.Errorf(".gitignore = %q, want the user's entries alone", got)
	}
	mustConform(t, dir)
}

// TestGraphifyIgnoresItsOutput runs git against the synced .gitignore:
// the graph is ignored, and the managed files stay committable.
func TestGraphifyIgnoresItsOutput(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	writeVibeYAML(t, dir, goGraphify)
	mustSync(t, dir)
	got, err := gitCheckIgnored(t.Context(), dir, []string{graphify.GraphPath, "graphify-out/cache/x.json", graphify.SkillPath, "Taskfile.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if !got[graphify.GraphPath] || !got["graphify-out/cache/x.json"] || len(got) != 2 {
		t.Errorf("ignored %v, want exactly the two graphify-out/ paths", got)
	}
}

// TestGraphifyDeselectKeepsModifiedSkillAndGraph pins acceptance
// criterion 7: an edited skill is someone's work and is kept as a
// conflict; graphify-out/ was never recorded, so it is never touched;
// a .gitignore that VibeConform created is deleted with its section.
func TestGraphifyDeselectKeepsModifiedSkillAndGraph(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goGraphify)
	mustSync(t, dir)
	graph := filepath.Join(dir, filepath.FromSlash(graphify.GraphPath))
	if err := os.MkdirAll(filepath.Dir(graph), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(graph, []byte(`{"built_at_commit":"abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(dir, filepath.FromSlash(graphify.SkillPath))
	if err := os.WriteFile(skill, []byte("# my notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeVibeYAML(t, dir, goDefaults)
	out, err := runSyncIn(t, dir)
	if err == nil || !strings.Contains(out, graphify.SkillPath+": conflict: graphify deselected but file modified since sync") {
		t.Fatalf("sync err %v; want a conflict for the modified skill:\n%s", err, out)
	}
	if data, _ := os.ReadFile(skill); string(data) != "# my notes\n" {
		t.Errorf("modified skill changed or removed: %q", data)
	}
	if !exists(t, dir, graphify.GraphPath) {
		t.Error("graphify-out/graph.json was removed; vibe never owned it")
	}
	if exists(t, dir, ".gitignore") {
		t.Error(".gitignore survived, though VibeConform created it and only its section was in it")
	}
	s, _ := state.Load(dir)
	if _, ok := s.Resources[graphify.SkillPath]; !ok {
		t.Error("the kept skill lost its state entry")
	}
}

// TestSyncWarnsAboutOptionalGraphify pins acceptance criterion 6: a
// missing graphify is a warning naming how to install it, and sync still
// succeeds.
func TestSyncWarnsAboutOptionalGraphify(t *testing.T) {
	stubLookPath(t, "go", "golangci-lint", "lefthook", "task", "gofmt", "goimports", "govulncheck", "actionlint", "zizmor", "git")
	dir := t.TempDir()
	writeVibeYAML(t, dir, goGraphify)

	root := NewRootCmd("test")
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"sync", "--repo-root", dir})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync failed: %v\n%s\n%s", err, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "warning: graphify not found on PATH (optional, for graphify:") ||
		!strings.Contains(errOut.String(), "install: uv tool install graphifyy") {
		t.Errorf("no optional-tool warning:\n%s", errOut.String())
	}
}

// TestDoctorGraphifyMissingExitsZero pins acceptance criterion 5 at the
// CLI: without graphify or a graph, doctor warns and still exits 0, and
// writes nothing.
func TestDoctorGraphifyMissingExitsZero(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goGraphify)
	mustSync(t, dir)
	before := snapshot(t, dir)

	stubDoctorEnv(t, "graphify")
	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor failed over an optional integration: %v\n%s", err, out)
	}
	for _, want := range []string{
		"WARN        graphify ",
		"WARN        graphify graph   no graph (graphify-out/graph.json absent); run task graph:update",
		"graphify ignore",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor lacks %q:\n%s", want, out)
		}
	}
	if after := snapshot(t, dir); !maps.Equal(before, after) {
		t.Error("doctor changed the repository")
	}
}

// TestDoctorWithoutGraphifyIsUnchanged: unselected, doctor says nothing
// about graphify.
func TestDoctorWithoutGraphifyIsUnchanged(t *testing.T) {
	dir := syncedRepo(t)
	stubDoctorEnv(t)
	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "graphify") {
		t.Errorf("doctor mentions graphify though it is not selected:\n%s", out)
	}
}
