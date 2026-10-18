package repotooling

import (
	"context"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

// TestAgentGuardVibeGuard pins spec 0053's opt-in: with
// policy.agent_guard vibe-guard the Taskfile loses hook:guard and nothing
// else, guard.go is not resolved, and the hooks need vibe-guard instead
// of go. The default and an explicit runtime resolve what they always did.
func TestAgentGuardVibeGuard(t *testing.T) {
	resolve := func(mctx *module.Context) map[string][]byte {
		t.Helper()
		rs, err := New().Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		out := map[string][]byte{}
		for _, r := range rs {
			out[r.Path] = r.Content
		}
		return out
	}
	policies := func(v string) *module.Context {
		return &module.Context{Policies: map[string]string{manifest.PolicyAgentGuard: v}}
	}
	base := resolve(nil)
	runtime := resolve(policies(module.AgentGuardRuntime))
	for path, want := range base {
		if string(runtime[path]) != string(want) {
			t.Errorf("agent_guard runtime: %s differs from the default", path)
		}
	}
	if len(runtime) != len(base) {
		t.Errorf("agent_guard runtime resolves %d files, the default %d", len(runtime), len(base))
	}

	got := resolve(policies(module.AgentGuardVibeGuard))
	if _, ok := got[".claude/hooks/guard.go"]; ok {
		t.Error("agent_guard vibe-guard still resolves .claude/hooks/guard.go")
	}
	if string(got["lefthook.yml"]) != string(base["lefthook.yml"]) {
		t.Error("agent_guard vibe-guard changed lefthook.yml")
	}
	tasks := func(content []byte) map[string]any {
		var tf struct {
			Tasks map[string]any `yaml:"tasks"`
		}
		if err := yaml.Unmarshal(content, &tf); err != nil {
			t.Fatalf("parsing Taskfile.yml: %v", err)
		}
		return tf.Tasks
	}
	before, after := tasks(base["Taskfile.yml"]), tasks(got["Taskfile.yml"])
	if _, ok := after["hook:guard"]; ok {
		t.Error("agent_guard vibe-guard still defines hook:guard")
	}
	delete(before, "hook:guard")
	for name, task := range before {
		if a, ok := after[name]; !ok || !yamlEqual(t, a, task) {
			t.Errorf("agent_guard vibe-guard changed or dropped task %s", name)
		}
	}

	if hb := (repotoolingModule{}).HookBinaries(policies(module.AgentGuardVibeGuard)); !slices.Equal(hb, []string{"task", "vibe-guard"}) {
		t.Errorf("HookBinaries = %v, want [task vibe-guard]", hb)
	}
	if hb := (repotoolingModule{}).HookBinaries(nil); !slices.Equal(hb, []string{"task", "go"}) {
		t.Errorf("HookBinaries(nil) = %v, want [task go]", hb)
	}
}

func yamlEqual(t *testing.T, a, b any) bool {
	t.Helper()
	x, err := yaml.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := yaml.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(x) == string(y)
}
