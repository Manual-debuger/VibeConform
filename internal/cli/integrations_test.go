package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// planFor writes body as vibe.yaml into a fresh directory and plans it.
func planFor(t *testing.T, body string) (*repoPlan, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return buildPlan(dir)
}

func planPaths(p *repoPlan) []string {
	var paths []string
	for _, rp := range p.Resources {
		paths = append(paths, rp.Resource.Path)
	}
	return paths
}

// TestPlanSelectsIntegrations: the manifest's selection reaches the plan's
// context and module list, and an explicit selection drops a deselected
// agent's resources.
func TestPlanSelectsIntegrations(t *testing.T) {
	p, err := planFor(t, "standard: prod-go\nversion: v1\nintegrations:\n  agents: [claude]\n")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Context.Integrations, []string{"claude"}) {
		t.Errorf("context integrations %v, want [claude]", p.Context.Integrations)
	}
	paths := planPaths(p)
	if !slices.Contains(paths, ".claude/settings.json") {
		t.Errorf("claude selected, but no .claude/settings.json in %v", paths)
	}
	for _, path := range paths {
		if strings.HasPrefix(path, ".codex/") {
			t.Errorf("codex deselected, but %s still resolves", path)
		}
	}
}

// TestPlanDefaultsKeepAgentsAndHooks is the regression the goal of spec
// 0026's C1/C2 split rests on: a vibe.yaml without integrations: must keep
// both agents, the guard, and every hook:* task, for every standard.
func TestPlanDefaultsKeepAgentsAndHooks(t *testing.T) {
	manifests := map[string]string{
		"prod-go":   "standard: prod-go\nversion: v1\n",
		"prod-ts":   "standard: prod-ts\nversion: v1\n",
		"prod-py":   "standard: prod-py\nversion: v1\n",
		"prod-mono": "standard: prod-mono\nversion: v1\ncomponents:\n  - {id: api, path: api, profile: go}\n",
	}
	for name, body := range manifests {
		t.Run(name, func(t *testing.T) {
			p, err := planFor(t, body)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(p.Context.Integrations, []string{"claude", "codex"}) {
				t.Errorf("default integrations %v", p.Context.Integrations)
			}
			var taskfile string
			guard := false
			for _, rp := range p.Resources {
				switch {
				case rp.Resource.Path == "Taskfile.yml":
					taskfile = string(rp.Resource.Content)
				case strings.HasPrefix(rp.Resource.Path, ".claude/hooks/guard."):
					guard = true
				}
			}
			if !guard {
				t.Error("defaults resolve no guard program")
			}
			for _, task := range []string{"hook:guard:", "hook:context:", "hook:format:", "hook:check:", "hook:done:"} {
				if !strings.Contains(taskfile, "\n  "+task+"\n") {
					t.Errorf("defaults' Taskfile.yml has no %s task", task)
				}
			}
			for _, want := range []string{".claude/settings.json", ".claude/hooks/policy.json", ".codex/config.toml", ".codex/hooks.json"} {
				if !slices.Contains(planPaths(p), want) {
					t.Errorf("defaults resolve no %s", want)
				}
			}
		})
	}
}

func TestPlanRejectsUnknownIntegration(t *testing.T) {
	_, err := planFor(t, "standard: prod-go\nversion: v1\nintegrations:\n  agents: [cursor]\n")
	if err == nil || !strings.Contains(err.Error(), "unknown agents integration (valid: claude, codex)") {
		t.Fatalf("error %v", err)
	}
}

func TestPlanProfiles(t *testing.T) {
	p, err := planFor(t, "standard: prod-ts\nversion: v1\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Context.Profiles) != 1 || p.Context.Profiles[0] != "ts" {
		t.Errorf("prod-ts profiles %v", p.Context.Profiles)
	}
	p, err = planFor(t, "standard: prod-mono\nversion: v1\ncomponents:\n"+
		"  - {id: w, path: w, profile: py}\n  - {id: a, path: a, profile: go}\n  - {id: b, path: b, profile: go}\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Context.Profiles; len(got) != 2 || got[0] != "go" || got[1] != "py" {
		t.Errorf("prod-mono profiles %v, want [go py]", got)
	}
}
