package tsrepotooling

import (
	"bytes"
	"context"
	"strings"
	"testing"

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

	want := []string{"Taskfile.yml", "lefthook.yml"}
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
		t.Fatal("ts-repo-tooling should declare task and lefthook; it does not implement ToolRequirer")
	}

	tools := requirer.RequiredTools()
	if len(tools) != 2 {
		t.Fatalf("RequiredTools() = %+v, want 2 entries", tools)
	}
	for _, want := range []string{"task", "lefthook"} {
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
	for name, content := range map[string][]byte{"Taskfile.yml": taskfile, "lefthook.yml": lefthookConfig} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}
