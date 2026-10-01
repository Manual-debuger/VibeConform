package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/workflow"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

var workflowKey = state.SectionKey{Path: "AGENTS.md", ID: "workflow"}

func goWorkflow(mode string) string {
	return "standard: prod-go\nversion: v1\ndevelopment:\n  workflow: " + mode + "\n"
}

// workflowSection is the whole section as it lands in AGENTS.md.
func workflowSection(mode string) string {
	return "<!-- vibeconform:begin workflow -->\n" +
		workflow.Content(mode, true, false) +
		"<!-- vibeconform:end workflow -->\n"
}

func writeAgents(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWorkflowIsOptIn: without development:, nothing touches AGENTS.md.
func TestWorkflowIsOptIn(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDefaults)
	if out := runDiffIn(t, dir); strings.Contains(out, "AGENTS.md") {
		t.Errorf("diff mentions AGENTS.md:\n%s", out)
	}
	mustSync(t, dir)
	if exists(t, dir, "AGENTS.md") {
		t.Error("sync wrote AGENTS.md")
	}
}

func TestWorkflowCreatesAgents(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goWorkflow(workflow.PlanTriggered))
	if out := mustSync(t, dir); !strings.Contains(out, "AGENTS.md (section workflow): created") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, "AGENTS.md"); got != workflowSection(workflow.PlanTriggered) {
		t.Errorf("AGENTS.md = %q", got)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Sections[workflowKey].Created {
		t.Errorf("state %+v, want created", s.Sections[workflowKey])
	}
	mustConform(t, dir)

	// Removing development: deletes the file VibeConform created.
	writeVibeYAML(t, dir, goDefaults)
	if out := mustSync(t, dir); !strings.Contains(out, "AGENTS.md (section workflow): removed, and the file (development.workflow deselected; nothing else was in it)") {
		t.Errorf("sync:\n%s", out)
	}
	if exists(t, dir, "AGENTS.md") {
		t.Error("AGENTS.md survived, though VibeConform created it and nothing else is in it")
	}
	mustConform(t, dir)
}

// TestWorkflowKeepsProjectProse: the section goes after the project's
// text, which is never rewritten, and leaves with nothing else.
func TestWorkflowKeepsProjectProse(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goWorkflow(workflow.Direct))
	prose := "# Agent Instructions\r\n\r\nProject rules stay here.\r\n"
	writeAgents(t, dir, prose)
	mustSync(t, dir)
	if got := readFile(t, dir, "AGENTS.md"); got != prose+"\n"+workflowSection(workflow.Direct) {
		t.Errorf("AGENTS.md = %q", got)
	}
	mustConform(t, dir)

	// Switching workflow is an update of the section, not a removal.
	writeVibeYAML(t, dir, goWorkflow(workflow.AlwaysSDD))
	out := mustSync(t, dir)
	if !strings.Contains(out, "AGENTS.md (section workflow): updated") || strings.Contains(out, "deselected") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, "AGENTS.md"); got != prose+"\n"+workflowSection(workflow.AlwaysSDD) {
		t.Errorf("AGENTS.md = %q", got)
	}
	mustConform(t, dir)

	writeVibeYAML(t, dir, goDefaults)
	if out := mustSync(t, dir); !strings.Contains(out, "AGENTS.md (section workflow): removed (development.workflow deselected)") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, "AGENTS.md"); got != prose {
		t.Errorf("AGENTS.md = %q, want the project's prose alone", got)
	}
	mustConform(t, dir)
}

// TestWorkflowHandEdit: the section is managed like any other resource.
// An edit inside it is drift, which sync restores; an edit while the
// standard also changed the section is a conflict, which sync refuses.
func TestWorkflowHandEdit(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goWorkflow(workflow.PlanTriggered))
	mustSync(t, dir)

	edited := strings.Replace(workflowSection(workflow.PlanTriggered), "Do not implement until the user approves.", "Implement whenever.", 1)
	writeAgents(t, dir, edited)
	out, err := runAuditIn(t, dir)
	wantExit(t, err, 2, out)
	if !strings.Contains(out, "AGENTS.md (section workflow): drifted") {
		t.Errorf("audit:\n%s", out)
	}
	mustSync(t, dir)
	if got := readFile(t, dir, "AGENTS.md"); got != workflowSection(workflow.PlanTriggered) {
		t.Errorf("sync did not restore the section: %q", got)
	}

	writeAgents(t, dir, edited)
	writeVibeYAML(t, dir, goWorkflow(workflow.AlwaysSDD))
	out, err = runAuditIn(t, dir)
	wantExit(t, err, 2, out)
	if !strings.Contains(out, "AGENTS.md (section workflow): conflict") {
		t.Errorf("audit:\n%s", out)
	}
	out, err = runSyncIn(t, dir)
	if err == nil || !strings.Contains(out, "AGENTS.md (section workflow): conflict") {
		t.Errorf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, dir, "AGENTS.md"); got != edited {
		t.Errorf("sync rewrote an edited section: %q", got)
	}
}

func TestWorkflowUnknownValue(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goWorkflow("sdd"))
	out, err := runSyncIn(t, dir)
	wantExit(t, err, 1, out)
	if want := "development.workflow (sdd): unknown value (valid: direct, plan-triggered-sdd, always-sdd)"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %v, want it to contain %q", err, want)
	}
}
