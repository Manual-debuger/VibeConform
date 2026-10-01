package claude

import (
	"context"
	"strings"
	"testing"

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
		if len(rs) != 2 || rs[SpecCommandPath].Path != "" || rs[ClaudeMDPath].Path != "" {
			t.Errorf("%s: resources %v", name, rs)
		}
	}
}

func TestAdapterResources(t *testing.T) {
	for _, mode := range []string{workflow.Direct, workflow.PlanTriggered, workflow.AlwaysSDD} {
		rs := resolvePaths(t, &module.Context{Integrations: []string{"claude"}, Policies: map[string]string{"workflow": mode}})
		cmd, ok := rs[SpecCommandPath]
		if !ok || cmd.Ownership != resource.Generated {
			t.Fatalf("%s: /spec %+v", mode, cmd)
		}
		if string(cmd.Content) != wantSpecCommand {
			t.Errorf("%s: /spec content:\n%s", mode, cmd.Content)
		}
		imp := rs[ClaudeMDPath]
		if imp.Ownership != resource.ManagedSection || imp.SectionID != "agents" || imp.Markers != resource.HTMLComment ||
			imp.Placement != resource.Top || string(imp.Content) != "@AGENTS.md\n" {
			t.Errorf("%s: CLAUDE.md section %+v", mode, imp)
		}
	}
}

// wantSpecCommand pins .claude/commands/spec.md byte for byte. Claude Code
// reads description and argument-hint from the front matter, and fills
// $ARGUMENTS with what follows /spec.
const wantSpecCommand = "---\n" +
	"description: Write a lightweight spec for a change, then propose a plan; stop before implementing.\n" +
	"argument-hint: <feature or issue>\n" +
	"---\n" +
	"\n" +
	"This is a planning context for: $ARGUMENTS\n" +
	"\n" +
	"Do not change code, configuration or tests while running this command.\n" +
	"\n" +
	"1. Read the specs, architecture docs and ADRs that apply (AGENTS.md says\n" +
	"   where they live), and the code involved.\n" +
	"2. List the constraints that apply and the assumptions you have not\n" +
	"   verified. Ask the user about the ones that would change the result.\n" +
	"3. If an approved spec already covers this, reuse it. Otherwise write one\n" +
	"   from the template below, where AGENTS.md says specs live\n" +
	"   (`docs/specs/<feature>.md` if it names no place). Keep it to WHAT must\n" +
	"   be true: no file-level steps, function design or sequencing.\n" +
	"4. Propose the implementation plan, the HOW, separately in the\n" +
	"   conversation. Do not commit it unless the project asks for that.\n" +
	"5. Stop. Implement only after the user approves the spec and the plan.\n" +
	"\n" +
	"Template:\n" +
	"\n" +
	"```markdown\n" +
	workflow.SpecTemplate +
	"```\n"

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
	if !strings.HasSuffix(wantSpecCommand, "```\n") {
		t.Error("spec command does not end its fence")
	}
}
