package standard

import (
	"context"
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
				t.Fatal("composes agent-config but resolves no Taskfile.yml to define hook:guard")
			}
			var tf struct {
				Tasks map[string]struct {
					Cmds []any `yaml:"cmds"`
				} `yaml:"tasks"`
			}
			if err := yaml.Unmarshal(taskfile.Content, &tf); err != nil {
				t.Fatalf("parsing Taskfile.yml: %v", err)
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
