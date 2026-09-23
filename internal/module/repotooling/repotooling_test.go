package repotooling

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "repo-tooling" {
		t.Errorf("Name() = %q, want %q", got, "repo-tooling")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{"Taskfile.yml", "lefthook.yml", ".claude/hooks/guard.go"}
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

// TestTemplatesMatchLiveFiles is the drift alarm; it matters more here than
// anywhere else in the standard, because a broken Taskfile.yml breaks every
// CI job and every local "task verify" at once.
func TestTemplatesMatchLiveFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	for _, tc := range []struct {
		live     string
		embedded []byte
	}{
		{"Taskfile.yml", taskfile},
		{"lefthook.yml", lefthookConfig},
		{filepath.Join(".claude", "hooks", "guard.go"), guard},
	} {
		t.Run(tc.live, func(t *testing.T) {
			live, err := os.ReadFile(filepath.Join(repoRoot, tc.live))
			if err != nil {
				t.Fatalf("reading live file: %v", err)
			}
			if !bytes.Equal(live, tc.embedded) {
				t.Errorf("embedded template has drifted from %s — "+
					"decide which copy is right rather than re-seeding blindly", tc.live)
			}
		})
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{"Taskfile.yml": taskfile, "lefthook.yml": lefthookConfig, "guard.go": guard} {
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
	want := "go run .claude/hooks/guard.go .claude/hooks/policy.json || exit 2"
	if len(task.Cmds) != 1 || task.Cmds[0] != want {
		t.Errorf("hook:guard cmds = %v, want [%q]", task.Cmds, want)
	}
}
