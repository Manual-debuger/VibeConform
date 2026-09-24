package standard

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/module/agents"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// TestAgentConfigWiring checks the one dependency between modules that the
// module model cannot express (spec 0021). agent-config's settings run
// "task -x hook:guard", but the task and the guard it runs come from the
// standard's repo-tooling module. A standard composing agent-config without
// them would ship agent configs calling a task that does not exist; Task
// would exit 200, and both agents treat that as allow. Catch it at build
// time rather than in an agent session.
func TestAgentConfigWiring(t *testing.T) {
	for k, s := range registry {
		t.Run(k.name+"/"+k.version, func(t *testing.T) {
			resources := map[string]resource.Resource{}
			composesAgentConfig := false
			for _, m := range s.Modules {
				if m.Name() == agents.New().Name() {
					composesAgentConfig = true
				}
				rs, err := m.Resolve(context.Background(), nil)
				if err != nil {
					t.Fatalf("resolving %s: %v", m.Name(), err)
				}
				for _, r := range rs {
					resources[r.Path] = r
				}
			}
			if !composesAgentConfig {
				t.Skip("does not compose agent-config")
			}

			taskfile, ok := resources["Taskfile.yml"]
			if !ok {
				t.Fatal("composes agent-config but resolves no Taskfile.yml to define the hook tasks")
			}
			var tf struct {
				Tasks map[string]struct {
					Cmds []any `yaml:"cmds"`
				} `yaml:"tasks"`
			}
			if err := yaml.Unmarshal(taskfile.Content, &tf); err != nil {
				t.Fatalf("parsing Taskfile.yml: %v", err)
			}

			// Every hook either agent runs must name a task this standard's
			// Taskfile defines (spec 0023 extends this from hook:guard to
			// every event).
			for _, config := range []string{".claude/settings.json", ".codex/hooks.json"} {
				for _, command := range configCommands(t, resources, config) {
					name, ok := strings.CutPrefix(command, "task -x ")
					if !ok {
						t.Errorf("%s runs %q, which is not a task -x command", config, command)
						continue
					}
					if _, ok := tf.Tasks[name]; !ok {
						t.Errorf("%s runs %q, but Taskfile.yml defines no %s", config, command, name)
					}
				}
			}

			task, ok := tf.Tasks["hook:guard"]
			if !ok {
				t.Fatalf("Taskfile.yml defines no hook:guard, which %q calls", agents.GuardCommand)
			}
			if len(task.Cmds) != 1 {
				t.Fatalf("hook:guard has %d commands, want 1", len(task.Cmds))
			}
			cmd, ok := task.Cmds[0].(string)
			if !ok {
				t.Fatalf("hook:guard command %v is not a string", task.Cmds[0])
			}

			// Every repository file the command names must be a resource
			// the standard writes: the guard and its policy.
			named := 0
			for arg := range strings.FieldsSeq(cmd) {
				if !strings.HasPrefix(arg, ".claude/") {
					continue
				}
				named++
				if _, ok := resources[arg]; !ok {
					t.Errorf("hook:guard runs %s, which no module in this standard resolves", arg)
				}
			}
			if named != 2 {
				t.Errorf("hook:guard names %d repository files in %q, want 2 (the guard and policy.json)", named, cmd)
			}
		})
	}
}

// configCommands returns every hook command in an agent config resource.
func configCommands(t *testing.T, resources map[string]resource.Resource, path string) []string {
	t.Helper()
	r, ok := resources[path]
	if !ok {
		t.Fatalf("agent-config resolves no %s", path)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(r.Content, &cfg); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var commands []string
	for _, entries := range cfg.Hooks {
		for _, e := range entries {
			for _, h := range e.Hooks {
				commands = append(commands, h.Command)
			}
		}
	}
	if len(commands) == 0 {
		t.Fatalf("%s runs no hook commands", path)
	}
	return commands
}

// TestEveryStandardComposesConformance pins spec 0022: the conformance job
// and task audit no longer live in any language module, so a standard
// without vibe-conformance would silently lose its conformance check.
func TestEveryStandardComposesConformance(t *testing.T) {
	for k, s := range registry {
		found := false
		for _, m := range s.Modules {
			found = found || m.Name() == "vibe-conformance"
		}
		if !found {
			t.Errorf("%s/%s does not compose vibe-conformance", k.name, k.version)
		}
	}
}
