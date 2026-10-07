package graphify

import (
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func resolve(t *testing.T, mctx *module.Context) []resource.Resource {
	t.Helper()
	rs, err := New().Resolve(context.Background(), mctx)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

// TestResolve pins spec 0035 §2: the ignore section always, the skill
// only with Claude Code selected.
func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mctx   *module.Context
		skills bool
	}{
		{"unknown selection", nil, false},
		{"claude", &module.Context{Integrations: []string{"claude", "graphify"}}, true},
		{"codex only", &module.Context{Integrations: []string{"codex", "graphify"}}, false},
		{"no agents", &module.Context{Integrations: []string{"graphify"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := resolve(t, tc.mctx)
			ignore := rs[0]
			if ignore.Path != ".gitignore" || ignore.Ownership != resource.ManagedSection ||
				ignore.SectionID != "graphify" || ignore.Markers != resource.HashComment || ignore.Placement != resource.Bottom {
				t.Errorf("first resource is %+v, want the bottom hash-comment graphify section of .gitignore", ignore)
			}
			if !strings.Contains(string(ignore.Content), "\ngraphify-out/\n") {
				t.Errorf("section does not ignore graphify-out/:\n%s", ignore.Content)
			}
			if got := len(rs) > 1 && rs[1].Path == SkillPath && rs[1].Ownership == resource.Generated; got != tc.skills {
				t.Errorf("skill generated = %v, want %v (%+v)", got, tc.skills, rs)
			}
			// No workflow in any of these contexts: AGENTS.md gets the
			// intelligence section, last (spec 0043 §2).
			if last := rs[len(rs)-1]; last.Path != "AGENTS.md" || last.SectionID != "intelligence" || last.Placement != resource.Bottom ||
				string(last.Content) != wantIntelligence {
				t.Errorf("last resource %+v, want the intelligence section of AGENTS.md", last)
			}
		})
	}
}

// TestNoSectionWithAWorkflow: the workflow section names the graph, so the
// intelligence section is not generated, and it is declared conditional so
// that sync removes a recorded copy (spec 0043 §3, §4).
func TestNoSectionWithAWorkflow(t *testing.T) {
	for _, r := range resolve(t, &module.Context{Integrations: []string{"graphify"}, Policies: map[string]string{"workflow": "direct"}}) {
		if r.Path == "AGENTS.md" {
			t.Errorf("resolved %+v with a workflow", r)
		}
	}
	cs := New().(module.ConditionalSectioner).ConditionalSections()
	if len(cs) != 1 || cs[0].SectionID != "intelligence" || cs[0].Markers != resource.HTMLComment {
		t.Errorf("ConditionalSections() = %+v", cs)
	}
}

const wantIntelligence = "\n## Repository intelligence\n" +
	"\n" +
	"Managed by VibeConform from `integrations.intelligence` in `vibe.yaml`.\n" +
	"\n" +
	"- Graphify keeps a knowledge graph in `graphify-out/`. Use it only when\n" +
	"  `built_at_commit` in `graph.json` matches HEAD (`task graph:update`\n" +
	"  rebuilds it); otherwise, or when it is absent, use search, the\n" +
	"  compiler and tests. It never replaces verification.\n" +
	"\n"

// TestSkill keeps the skill's promises: freshness before use, a fallback,
// no empty-result success, and no substitute for verification.
func TestSkill(t *testing.T) {
	for _, want := range []string{
		"name: graphify\n", "built_at_commit", "git rev-parse HEAD", "task graph:update",
		"Fall back", "not evidence that something does not exist", "never replaces or skips",
	} {
		if !strings.Contains(Skill, want) {
			t.Errorf("skill lacks %q", want)
		}
	}
	if strings.Contains(Skill, "\r") || !strings.HasSuffix(Skill, "\n") {
		t.Error("skill must be LF-only with a final newline")
	}
}

func TestRequiredToolIsOptional(t *testing.T) {
	tools := New().(module.ToolRequirer).RequiredTools(nil)
	if len(tools) != 1 || tools[0].Name != "graphify" || !tools[0].Optional || tools[0].Install == "" || len(tools[0].Version) == 0 {
		t.Errorf("tools = %+v, want graphify, optional, with an install hint and a version probe", tools)
	}
}
