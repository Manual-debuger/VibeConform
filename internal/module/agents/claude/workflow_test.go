package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/workflow"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func resolvePaths(t *testing.T, mctx *module.Context) map[string]resource.Resource {
	t.Helper()
	rs, err := New().Resolve(context.Background(), mctx)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]resource.Resource{}
	for _, r := range rs {
		out[r.Path] = r
	}
	return out
}

// TestAdapterNeedsAWorkflow: without development.workflow the module
// resolves exactly what it did before spec 0030.
func TestAdapterNeedsAWorkflow(t *testing.T) {
	for name, mctx := range map[string]*module.Context{
		"isolated":    nil,
		"no workflow": {Integrations: []string{"claude"}, Policies: map[string]string{"line_endings": "lf"}},
	} {
		rs := resolvePaths(t, mctx)
		if len(rs) != 2 || rs[SpecSkillPath].Path != "" || rs[ClaudeMDPath].Path != "" {
			t.Errorf("%s: resources %v", name, rs)
		}
	}
}

func TestAdapterResources(t *testing.T) {
	for mode, want := range map[string]string{
		workflow.Direct:        wantSkillUserInvoked + wantSkillBody,
		workflow.PlanTriggered: wantSkillModelInvoked + wantSkillBody,
		workflow.AlwaysSDD:     wantSkillModelInvoked + wantSkillBody,
	} {
		rs := resolvePaths(t, &module.Context{Integrations: []string{"claude"}, Policies: map[string]string{"workflow": mode}})
		skill, ok := rs[SpecSkillPath]
		if !ok || skill.Ownership != resource.Generated {
			t.Fatalf("%s: spec skill %+v", mode, skill)
		}
		if string(skill.Content) != want {
			t.Errorf("%s: spec skill content:\n%s", mode, skill.Content)
		}
		if _, ok := rs[SpecCommandPath]; ok {
			t.Errorf("%s: still generates %s", mode, SpecCommandPath)
		}
		imp := rs[ClaudeMDPath]
		if imp.Ownership != resource.ManagedSection || imp.SectionID != "agents" || imp.Markers != resource.HTMLComment ||
			imp.Placement != resource.Top || string(imp.Content) != "@AGENTS.md\n" {
			t.Errorf("%s: CLAUDE.md section %+v", mode, imp)
		}
	}
}

// TestComponentImports: each component's CLAUDE.md imports its own
// AGENTS.md (spec 0032 §2), and a duplicate import there warns too.
func TestComponentImports(t *testing.T) {
	comps := []manifest.Component{
		{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
		{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
	}
	rs := resolvePaths(t, &module.Context{Components: comps, Integrations: []string{"claude"}, Policies: map[string]string{"workflow": workflow.AlwaysSDD}})
	for _, c := range comps {
		imp, ok := rs[c.Path+"/CLAUDE.md"]
		if !ok || imp.SectionID != "agents" || imp.Placement != resource.Top || string(imp.Content) != "@AGENTS.md\n" {
			t.Errorf("%s: CLAUDE.md section %+v", c.ID, imp)
		}
	}
	if none := resolvePaths(t, &module.Context{Components: comps, Integrations: []string{"claude"}, Policies: map[string]string{}}); len(none) != 2 {
		t.Errorf("without a workflow: %v", none)
	}
	r := resource.Resource{Path: "services/api/CLAUDE.md", Content: []byte("@AGENTS.md\n")}
	if _, w := (claudeModule{}).CheckSection(r, nil, []byte("@AGENTS.md\n")); len(w) != 1 {
		t.Errorf("component duplicate import: warnings %v", w)
	}
}

// TestRetiresSpecCommand: the command spec 0030 generated is retired in
// favour of the skill (spec 0031 §3).
func TestRetiresSpecCommand(t *testing.T) {
	var r module.Retirer = claudeModule{}
	got := r.Retired()
	if len(got) != 1 || got[".claude/commands/spec.md"] != "replaced by .claude/skills/spec/SKILL.md" {
		t.Errorf("Retired() = %v", got)
	}
}

// The spec skill, .claude/skills/spec/SKILL.md, byte for byte. Claude Code
// reads name, description, argument-hint and disable-model-invocation from
// the front matter, and fills $ARGUMENTS with what follows /spec. Under
// direct the skill is the user's alone (spec 0031 §1).
const (
	wantSkillModelInvoked = "---\n" +
		"name: spec\n" +
		"description: Write a lightweight spec (what must be true, with acceptance criteria) for a non-trivial change, then propose a plan and stop before implementing. Use it in plan mode, and whenever asked to plan or spec a change.\n" +
		"argument-hint: <feature or issue>\n" +
		"---\n"
	wantSkillUserInvoked = "---\n" +
		"name: spec\n" +
		"description: Write a lightweight spec (what must be true, with acceptance criteria) for a change, then propose a plan and stop before implementing.\n" +
		"argument-hint: <feature or issue>\n" +
		"disable-model-invocation: true\n" +
		"---\n"
	wantSkillBody = "\n" +
		"This is a planning context for: $ARGUMENTS\n" +
		"(If nothing follows the colon, it is for the change under discussion.)\n" +
		"\n" +
		"Do not change code, configuration or tests while using this skill.\n" +
		"\n" +
		"1. Read the specs, architecture docs and ADRs that apply (AGENTS.md says\n" +
		"   where they live), and the code involved.\n" +
		"2. List the constraints that apply and the assumptions you have not\n" +
		"   verified. Ask the user about the ones that would change the result.\n" +
		"3. If an approved spec already covers this, reuse it. Otherwise write one\n" +
		"   from the template below, where AGENTS.md says specs live\n" +
		"   (`docs/specs/<feature>.md` if it names no place). In a read-only plan\n" +
		"   mode, put it in the plan instead, and write it once it is approved.\n" +
		"   Keep it to WHAT must be true: no file-level steps, function design or\n" +
		"   sequencing.\n" +
		"4. Propose the implementation plan, the HOW, separately from the spec.\n" +
		"   Do not commit it unless the project asks for that.\n" +
		"5. Stop. Implement only after the user approves the spec and the plan.\n" +
		"\n" +
		"Template:\n" +
		"\n" +
		"```markdown\n" +
		workflow.SpecTemplate +
		"```\n"
)

func TestDuplicateImportWarns(t *testing.T) {
	r := resource.Resource{Path: ClaudeMDPath, Content: []byte("@AGENTS.md\n")}
	conflicts, warnings := claudeModule{}.CheckSection(r, nil, []byte("\n# Project\n  @./AGENTS.md\nsee @AGENTS.md here\n"))
	if len(conflicts) != 0 {
		t.Errorf("conflicts %v", conflicts)
	}
	// The section is lines 1-3, so the project's import is line 6.
	want := `line 6: "@./AGENTS.md" imports AGENTS.md again; the section already does, so remove this line`
	if len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings %q, want [%q]", warnings, want)
	}

	if _, w := (claudeModule{}).CheckSection(r, nil, []byte("# Project\n")); len(w) != 0 {
		t.Errorf("warned with no duplicate: %v", w)
	}
	other := resource.Resource{Path: "AGENTS.md", Content: []byte("x\n")}
	if c, w := (claudeModule{}).CheckSection(other, []byte("@AGENTS.md\n"), nil); len(c)+len(w) != 0 {
		t.Errorf("checked %s: %v %v", other.Path, c, w)
	}
	if !strings.HasSuffix(wantSkillBody, "```\n") {
		t.Error("spec skill does not end its fence")
	}
}
