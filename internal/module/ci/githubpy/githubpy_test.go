package githubpy

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "github-ci-py" {
		t.Errorf("Name() = %q, want %q", got, "github-ci-py")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{
		".github/workflows/ci.yml",
		".github/dependabot.yml",
		".github/pull_request_template.md",
	}
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

// TestDependabotIsPipNotGoMod guards the reason this package exists rather
// than reusing internal/module/ci/github's dependabot.yml as-is: that one
// hardcodes package-ecosystem: gomod, which is Go-specific despite the
// file's generic name.
func TestDependabotIsPipNotGoMod(t *testing.T) {
	if !bytes.Contains(dependabotConfig, []byte("package-ecosystem: pip")) {
		t.Error("dependabot.yml does not declare the pip ecosystem")
	}
	if bytes.Contains(dependabotConfig, []byte("gomod")) {
		t.Error("dependabot.yml still names the Go ecosystem")
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{
		"ci.yml":                   ciWorkflow,
		"dependabot.yml":           dependabotConfig,
		"pull_request_template.md": pullRequestTemplate,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}
