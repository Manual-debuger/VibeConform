package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
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

// TestWorkflowClaudeAdapter: with claude selected, a workflow brings /spec
// and CLAUDE.md's import, and each leaves with whichever option goes.
func TestWorkflowClaudeAdapter(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goWorkflow(workflow.PlanTriggered))
	out := mustSync(t, dir)
	for _, want := range []string{".claude/skills/spec/SKILL.md: created", "CLAUDE.md (section agents): created"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	if got := readFile(t, dir, "CLAUDE.md"); got != "<!-- vibeconform:begin agents -->\n@AGENTS.md\n<!-- vibeconform:end agents -->\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
	mustConform(t, dir)

	// Deselecting claude removes the adapter, and AGENTS.md stops naming /spec.
	writeVibeYAML(t, dir, goWorkflow(workflow.PlanTriggered)+"integrations:\n  agents: [codex]\n")
	out = mustSync(t, dir)
	for _, want := range []string{
		".claude/skills/spec/SKILL.md: removed (claude deselected)",
		"CLAUDE.md (section agents): removed, and the file (claude deselected; nothing else was in it)",
		"AGENTS.md (section workflow): updated",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	if got := readFile(t, dir, "AGENTS.md"); strings.Contains(got, "`/spec`") {
		t.Errorf("AGENTS.md still names /spec:\n%s", got)
	}
	mustConform(t, dir)

	// Selecting claude again, then dropping the workflow, removes all three.
	writeVibeYAML(t, dir, goWorkflow(workflow.PlanTriggered))
	mustSync(t, dir)
	writeVibeYAML(t, dir, goDefaults)
	out = mustSync(t, dir)
	for _, want := range []string{
		"AGENTS.md (section workflow): removed",
		".claude/skills/spec/SKILL.md: removed (development.workflow deselected)",
		"CLAUDE.md (section agents): removed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md", ".claude/skills/spec/SKILL.md"} {
		if exists(t, dir, p) {
			t.Errorf("%s survived deselection", p)
		}
	}
	mustConform(t, dir)
}

// TestWorkflowComponents: in prod-mono, a workflow gives each component's
// AGENTS.md its section below the project's prose, and each component's
// CLAUDE.md the import; deselecting the workflow takes them all away
// again (spec 0032).
func TestWorkflowComponents(t *testing.T) {
	dir := t.TempDir()
	base := "standard: prod-mono\nversion: v1\ncomponents:\n" +
		"  - {id: api, path: services/api, profile: go}\n  - {id: web, path: web, profile: ts}\n"
	writeVibeYAML(t, dir, base+"development:\n  workflow: always-sdd\n")
	prose := "# API\n\nKeep handlers thin.\n"
	if err := os.MkdirAll(filepath.Join(dir, "services", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "services", "api", "AGENTS.md"), []byte(prose), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustSync(t, dir)
	for _, want := range []string{
		"services/api/AGENTS.md (section component): added",
		"web/AGENTS.md (section component): created",
		"services/api/CLAUDE.md (section agents): created",
		"web/CLAUDE.md (section agents): created",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	api := manifest.Component{ID: "api", Path: "services/api", Profile: manifest.ProfileGo}
	want := prose + "\n<!-- vibeconform:begin component -->\n" + workflow.ComponentContent(api) + "<!-- vibeconform:end component -->\n"
	if got := readFile(t, dir, "services/api/AGENTS.md"); got != want {
		t.Errorf("services/api/AGENTS.md = %q", got)
	}
	mustConform(t, dir)

	writeVibeYAML(t, dir, base)
	out = mustSync(t, dir)
	for _, want := range []string{
		"services/api/AGENTS.md (section component): removed (development.workflow deselected)",
		"web/AGENTS.md (section component): removed, and the file (development.workflow deselected; nothing else was in it)",
		"web/CLAUDE.md (section agents): removed, and the file",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync: missing %q\n%s", want, out)
		}
	}
	if got := readFile(t, dir, "services/api/AGENTS.md"); got != prose {
		t.Errorf("services/api/AGENTS.md = %q, want the project's prose alone", got)
	}
	for _, p := range []string{"web/AGENTS.md", "web/CLAUDE.md", "services/api/CLAUDE.md"} {
		if exists(t, dir, p) {
			t.Errorf("%s survived deselection", p)
		}
	}
	mustConform(t, dir)
}

// TestSpecCommandRetired: the /spec command spec 0030 generated is
// removed once the skill replaces it, if it is recorded and unchanged
// (spec 0031 §3). A modified one is kept with a conflict, and an
// unrecorded one is never touched.
func TestSpecCommandRetired(t *testing.T) {
	const command = ".claude/commands/spec.md"
	old := []byte("---\ndescription: the spec 0030 command\n---\n")
	writeCommand := func(t *testing.T, dir string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, ".claude", "commands"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".claude", "commands", "spec.md"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	setup := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		writeVibeYAML(t, dir, goWorkflow(workflow.PlanTriggered))
		mustSync(t, dir)
		writeCommand(t, dir, old)
		recordState(t, dir, command, sha256Hex(old))
		return dir
	}
	const why = "replaced by .claude/skills/spec/SKILL.md"

	t.Run("unchanged", func(t *testing.T) {
		dir := setup(t)
		if out := runDiffIn(t, dir); !strings.Contains(out, command+": would remove ("+why+")") {
			t.Errorf("diff:\n%s", out)
		}
		out, err := runAuditIn(t, dir)
		wantExit(t, err, 3, out)
		if !strings.Contains(out, command+": out of date ("+why+"; run vibe sync to remove)") {
			t.Errorf("audit:\n%s", out)
		}
		if out := mustSync(t, dir); !strings.Contains(out, command+": removed ("+why+")") {
			t.Errorf("sync:\n%s", out)
		}
		if exists(t, dir, command) || exists(t, dir, ".claude/commands") {
			t.Error("the retired command, or its empty directory, survived sync")
		}
		s, err := state.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Resources[command]; ok {
			t.Error("state still records the retired command")
		}
		mustConform(t, dir)
	})

	t.Run("modified", func(t *testing.T) {
		dir := setup(t)
		edited := []byte("---\ndescription: my own /spec\n---\n")
		writeCommand(t, dir, edited)
		out, err := runSyncIn(t, dir)
		if err == nil || !strings.Contains(out, command+": conflict: "+why+" but file modified since sync; kept (delete it by hand)") {
			t.Errorf("sync: %v\n%s", err, out)
		}
		if got := readFile(t, dir, command); got != string(edited) {
			t.Errorf("sync rewrote a modified retired command: %q", got)
		}
	})

	t.Run("unrecorded", func(t *testing.T) {
		dir := t.TempDir()
		writeVibeYAML(t, dir, goWorkflow(workflow.PlanTriggered))
		writeCommand(t, dir, old)
		if out := mustSync(t, dir); strings.Contains(out, command) {
			t.Errorf("sync touched an unrecorded command:\n%s", out)
		}
		if got := readFile(t, dir, command); got != string(old) {
			t.Errorf("command = %q", got)
		}
		mustConform(t, dir)
	})
}

// TestWorkflowDuplicateImport: a project CLAUDE.md that already imports
// AGENTS.md syncs, with a warning, and keeps its own line.
func TestWorkflowDuplicateImport(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goWorkflow(workflow.Direct))
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("@AGENTS.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubHookInstall(t)
	out, errOut, err := runSyncCapturing(t, dir)
	if err != nil || !strings.Contains(errOut, `warning: CLAUDE.md (section agents): line 5: "@AGENTS.md" imports AGENTS.md again`) {
		t.Errorf("sync: %v\n%s\n%s", err, out, errOut)
	}
	if got := readFile(t, dir, "CLAUDE.md"); !strings.HasSuffix(got, "<!-- vibeconform:end agents -->\n\n@AGENTS.md\n") {
		t.Errorf("CLAUDE.md = %q", got)
	}
	mustConform(t, dir)
}
