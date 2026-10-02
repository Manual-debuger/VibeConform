package standard

import (
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/intelligence/graphify"
)

// graphifyTouches are the paths selecting graphify may change; every
// other resource must be byte-identical with and without it.
var graphifyTouches = []string{"Taskfile.yml", "lefthook.yml", graphify.IgnorePath, graphify.SkillPath}

// TestGraphifyFollowsSelection pins spec 0035 acceptance criteria 1 and 2
// for every standard: off by default and invisible when unselected, and,
// when selected, exactly its resources, with the Claude-only parts
// following claude.
func TestGraphifyFollowsSelection(t *testing.T) {
	for k, s := range registry {
		if len(s.Options) == 0 {
			continue
		}
		t.Run(k.name+"/"+k.version, func(t *testing.T) {
			if slices.Contains(s.Defaults().Integrations, "graphify") {
				t.Fatal("graphify is on by default")
			}
			for path, r := range resolveAll(t, s, s.Defaults().Integrations) {
				if strings.Contains(strings.ToLower(string(r.Content)), "graphify") {
					t.Errorf("default selection: %s mentions graphify", path)
				}
			}

			for _, agents := range [][]string{{"claude", "codex"}, {"codex"}, {}} {
				without := resolveAll(t, s, agents)
				got := resolveAll(t, s, append(slices.Clone(agents), "graphify"))
				claude := slices.Contains(agents, "claude")

				for path, want := range without {
					if slices.Contains(graphifyTouches, path) {
						continue
					}
					if r, ok := got[path]; !ok || string(r.Content) != string(want.Content) {
						t.Errorf("agents %v: %s changed when graphify was selected", agents, path)
					}
				}
				for path := range got {
					if _, ok := without[path]; !ok && path != graphify.IgnorePath && path != graphify.SkillPath {
						t.Errorf("agents %v: graphify added unexpected resource %s", agents, path)
					}
				}

				if _, ok := got[graphify.IgnorePath]; !ok {
					t.Errorf("agents %v: no .gitignore section", agents)
				}
				if _, ok := got[graphify.SkillPath]; ok != claude {
					t.Errorf("agents %v: skill present = %v, want %v", agents, ok, claude)
				}
				tasks := taskfileTasks(t, got["Taskfile.yml"])
				if _, ok := tasks["graph:update"]; !ok {
					t.Errorf("agents %v: Taskfile.yml has no graph:update", agents)
				}
				if line := strings.Contains(string(got["Taskfile.yml"].Content), "Graphify: "); line != claude {
					t.Errorf("agents %v: hook:context Graphify line present = %v, want %v", agents, line, claude)
				}
				lh := string(got["lefthook.yml"].Content)
				if !strings.Contains(lh, "post-commit:") || !strings.Contains(lh, "post-checkout:") || !strings.Contains(lh, "run: task graph:update") {
					t.Errorf("agents %v: lefthook.yml lacks the graph update jobs:\n%s", agents, lh)
				}
			}
		})
	}
}
