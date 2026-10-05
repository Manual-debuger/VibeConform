package standard

import (
	"context"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/agents/vibeskill"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

const (
	claudeVibeSkill = ".claude/skills/vibeconform/SKILL.md"
	codexVibeSkill  = ".agents/skills/vibeconform/SKILL.md"
)

// TestVibeSkillFollowsAgents pins spec 0041 §1 for every standard: each
// selected agent gets the skill at its own path, with no development:
// selection, and the copies are byte-identical.
func TestVibeSkillFollowsAgents(t *testing.T) {
	for k, s := range registry {
		if len(s.Options) == 0 {
			continue
		}
		t.Run(k.name+"/"+k.version, func(t *testing.T) {
			for _, c := range []struct {
				agents        []string
				claude, codex bool
			}{
				{[]string{"claude", "codex"}, true, true},
				{[]string{"claude"}, true, false},
				{[]string{"codex"}, false, true},
				{[]string{}, false, false},
			} {
				rs := resolveSkills(t, s, c.agents)
				cl, hasClaude := rs[claudeVibeSkill]
				cx, hasCodex := rs[codexVibeSkill]
				if hasClaude != c.claude || hasCodex != c.codex {
					t.Errorf("agents %v: claude copy %v, codex copy %v", c.agents, hasClaude, hasCodex)
				}
				for _, r := range []struct {
					ok   bool
					path string
					body []byte
				}{{hasClaude, claudeVibeSkill, cl.Content}, {hasCodex, codexVibeSkill, cx.Content}} {
					if r.ok && len(r.body) == 0 {
						t.Errorf("agents %v: %s is empty", c.agents, r.path)
					}
				}
				if hasClaude && hasCodex && string(cl.Content) != string(cx.Content) {
					t.Errorf("agents %v: the two copies differ", c.agents)
				}
			}
			want := vibeskill.Kind(k.name)
			if s.TakesComponents {
				want = vibeskill.MonoGitHub
			}
			if got := resolveSkills(t, s, []string{"claude"})[claudeVibeSkill]; string(got.Content) != vibeskill.Content(want) {
				t.Errorf("content is not the %s skill", want)
			}
		})
	}
}

// resolveSkills is resolveAll with the profiles buildPlan sets: the
// standard's own, or the components'.
func resolveSkills(t *testing.T, s Standard, selected []string) map[string]resource.Resource {
	t.Helper()
	mctx := sampleContext(s)
	if mctx == nil {
		mctx = &module.Context{Profiles: []manifest.Profile{s.Profile}}
	} else {
		mctx.Profiles = manifest.Profiles
	}
	mctx.Integrations = selected
	resources := map[string]resource.Resource{}
	for _, m := range s.ModulesFor(Selection{Integrations: selected}) {
		rs, err := m.Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatalf("resolving %s: %v", m.Name(), err)
		}
		for _, r := range rs {
			resources[r.Path] = r
		}
	}
	return resources
}
