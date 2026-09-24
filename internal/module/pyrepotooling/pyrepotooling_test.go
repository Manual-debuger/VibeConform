package pyrepotooling

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "py-repo-tooling" {
		t.Errorf("Name() = %q, want %q", got, "py-repo-tooling")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{"Taskfile.yml", "lefthook.yml", ".claude/hooks/guard.py"}
	if len(resources) != len(want) {
		t.Fatalf("Resolve returned %d resources, want %d", len(resources), len(want))
	}

	for i, r := range resources {
		if r.Path != want[i] {
			t.Errorf("resource %d path = %q, want %q", i, r.Path, want[i])
		}
		if r.Ownership != resource.Generated {
			t.Errorf("%s ownership = %v, want Generated", r.Path, r.Ownership)
		}
		if len(r.Content) == 0 {
			t.Errorf("%s has empty content", r.Path)
		}
		if strings.Contains(r.Path, "\\") {
			t.Errorf("path %q contains a backslash; paths must be slash-separated", r.Path)
		}
	}
}

func TestResolveDeterministic(t *testing.T) {
	first, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	second, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for i := range first {
		if first[i].Path != second[i].Path || !bytes.Equal(first[i].Content, second[i].Content) {
			t.Errorf("resource %d differs across calls", i)
		}
	}
}

func TestRequiredTools(t *testing.T) {
	requirer, ok := New().(module.ToolRequirer)
	if !ok {
		t.Fatal("py-repo-tooling should declare task, lefthook, and uv; it does not implement ToolRequirer")
	}

	tools := requirer.RequiredTools()
	if len(tools) != 3 {
		t.Fatalf("RequiredTools() = %+v, want 3 entries", tools)
	}
	// uv since spec 0022: every Taskfile and lefthook command runs through it.
	for _, want := range []string{"task", "lefthook", "uv"} {
		found := false
		for _, tool := range tools {
			if tool.Name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("RequiredTools() missing %q", want)
		}
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{"Taskfile.yml": taskfile, "lefthook.yml": lefthookConfig, "guard.py": guard} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}

// TestHookGuardTask pins the contract the agent configs depend on (spec
// 0021): a hook:guard task, hidden from task --list, that runs this
// module's guard on its own runtime against the shared policy file.
func TestHookGuardTask(t *testing.T) {
	var tf struct {
		Tasks map[string]struct {
			Desc   string `yaml:"desc"`
			Silent bool   `yaml:"silent"`
			Cmds   []any  `yaml:"cmds"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &tf); err != nil {
		t.Fatalf("parsing Taskfile.yml: %v", err)
	}

	task, ok := tf.Tasks["hook:guard"]
	if !ok {
		t.Fatal("Taskfile.yml defines no hook:guard task; the agent configs call one")
	}
	if task.Desc != "" {
		t.Errorf("hook:guard has desc %q; agents call it, people don't, so it stays out of task --list", task.Desc)
	}
	if !task.Silent {
		t.Error("hook:guard is not silent; Task would echo the command into the agent's hook output")
	}
	want := "uv run --no-project python .claude/hooks/guard.py .claude/hooks/policy.json || exit 2"
	if len(task.Cmds) != 1 || task.Cmds[0] != want {
		t.Errorf("hook:guard cmds = %v, want [%q]", task.Cmds, want)
	}
}
