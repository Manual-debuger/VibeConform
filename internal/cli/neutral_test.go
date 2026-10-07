package cli

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/workflow"
)

const goNeutral = "standard: prod-go\nversion: v1\n" +
	"integrations:\n  intelligence: [graphify]\n" +
	"development:\n  docs_layout: standard\n"

// TestNeutralRouting: without a workflow, the docs layout and Graphify each
// route agents through their own AGENTS.md section. A workflow takes over,
// and they come back when it goes; each leaves with its option (spec 0043).
func TestNeutralRouting(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goNeutral)
	out := mustSync(t, dir)
	for _, want := range []string{"AGENTS.md (section knowledge): created", "AGENTS.md (section intelligence): created"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	// The order is the standard's module order, graphify before the docs
	// layout, the same in every sync.
	neutral := "<!-- vibeconform:begin intelligence -->\n" + workflow.IntelligenceContent() + "<!-- vibeconform:end intelligence -->\n" +
		"\n" +
		"<!-- vibeconform:begin knowledge -->\n" + workflow.KnowledgeContent(workflow.DefaultLayout()) + "<!-- vibeconform:end knowledge -->\n"
	if got := readFile(t, dir, "AGENTS.md"); got != neutral {
		t.Errorf("AGENTS.md = %q", got)
	}
	mustConform(t, dir)
	if out := mustSync(t, dir); strings.Contains(out, "AGENTS.md (section knowledge): created") || !strings.Contains(out, "0 created, 0 updated") {
		t.Errorf("second sync changed something:\n%s", out)
	}

	// A workflow takes over: its section replaces both.
	writeVibeYAML(t, dir, goNeutral+"  workflow: direct\n")
	out = mustSync(t, dir)
	for _, want := range []string{
		"AGENTS.md (section knowledge): removed",
		"AGENTS.md (section intelligence): removed",
		"AGENTS.md (section workflow): added",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	if got := readFile(t, dir, "AGENTS.md"); strings.Contains(got, "begin knowledge") || strings.Contains(got, "begin intelligence") {
		t.Errorf("AGENTS.md keeps a neutral section beside the workflow:\n%s", got)
	}
	mustConform(t, dir)

	// Dropping the workflow brings them back.
	writeVibeYAML(t, dir, goNeutral)
	mustSync(t, dir)
	if got := readFile(t, dir, "AGENTS.md"); got != neutral {
		t.Errorf("AGENTS.md = %q", got)
	}
	mustConform(t, dir)

	// Each leaves with its option, and the file with the last of them.
	writeVibeYAML(t, dir, "standard: prod-go\nversion: v1\nintegrations:\n  intelligence: [graphify]\n")
	if out := mustSync(t, dir); !strings.Contains(out, "AGENTS.md (section knowledge): removed (development.docs_layout deselected)") {
		t.Errorf("sync:\n%s", out)
	}
	writeVibeYAML(t, dir, goDefaults)
	if out := mustSync(t, dir); !strings.Contains(out, "AGENTS.md (section intelligence): removed, and the file (graphify deselected; nothing else was in it)") {
		t.Errorf("sync:\n%s", out)
	}
	if exists(t, dir, "AGENTS.md") {
		t.Error("AGENTS.md survived, though VibeConform created it and nothing else is in it")
	}
	mustConform(t, dir)
}
