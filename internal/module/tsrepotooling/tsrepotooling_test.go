package tsrepotooling

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "ts-repo-tooling" {
		t.Errorf("Name() = %q, want %q", got, "ts-repo-tooling")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{"Taskfile.yml", "lefthook.yml", ".claude/hooks/guard.mjs"}
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
		t.Fatal("ts-repo-tooling should declare task, lefthook, and pnpm; it does not implement ToolRequirer")
	}

	tools := requirer.RequiredTools()
	if len(tools) != 3 {
		t.Fatalf("RequiredTools() = %+v, want 3 entries", tools)
	}
	// pnpm is required on PATH here, not in ts-tooling: every command this
	// module generates runs through it (spec 0020).
	for _, want := range []string{"task", "lefthook", "pnpm"} {
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

// TestTaskfileUsesPnpm guards spec 0020: every entry point runs through
// pnpm, and through pnpm exec specifically, so a missing devDependency
// fails instead of being fetched.
func TestTaskfileUsesPnpm(t *testing.T) {
	assertNoNpm(t, "Taskfile.yml", taskfile)
	for _, want := range []string{
		"pnpm exec prettier --write",
		"pnpm exec prettier --check",
		"pnpm exec eslint .",
		"pnpm exec tsc --noEmit",
		"pnpm test",
	} {
		if !bytes.Contains(taskfile, []byte(want)) {
			t.Errorf("Taskfile.yml does not run %q", want)
		}
	}
}

// TestLefthookUsesPnpm is TestTaskfileUsesPnpm for the git hooks.
func TestLefthookUsesPnpm(t *testing.T) {
	assertNoNpm(t, "lefthook.yml", lefthookConfig)
	for _, want := range []string{
		"pnpm exec prettier --check {staged_files}",
		"pnpm exec eslint {staged_files}",
		"pnpm exec tsc --noEmit",
		"pnpm test",
	} {
		if !bytes.Contains(lefthookConfig, []byte(want)) {
			t.Errorf("lefthook.yml does not run %q", want)
		}
	}
}

// npmOrNpx matches npm or npx as whole words, so "pnpm" does not count.
var npmOrNpx = regexp.MustCompile(`\bnp[mx]\b`)

// assertNoNpm fails if content invokes npm or npx anywhere.
func assertNoNpm(t *testing.T, name string, content []byte) {
	t.Helper()
	for _, found := range npmOrNpx.FindAll(content, -1) {
		t.Errorf("%s still runs %s; prod-ts uses pnpm (spec 0020)", name, found)
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{"Taskfile.yml": taskfile, "lefthook.yml": lefthookConfig, "guard.mjs": guard} {
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
	want := "node .claude/hooks/guard.mjs .claude/hooks/policy.json || exit 2"
	if len(task.Cmds) != 1 || task.Cmds[0] != want {
		t.Errorf("hook:guard cmds = %v, want [%q]", task.Cmds, want)
	}
}
