package github

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "github-ci" {
		t.Errorf("Name() = %q, want %q", got, "github-ci")
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
	}
}

func TestResolvePathsAreSlashSeparated(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Resource paths double as .vibe/state.yaml keys. A backslash here would
	// make a Windows run record different state than a Unix run for the same
	// repository.
	for _, r := range resources {
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

	if len(first) != len(second) {
		t.Fatalf("resource counts differ across calls: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Path != second[i].Path {
			t.Errorf("resource %d path differs across calls: %q vs %q", i, first[i].Path, second[i].Path)
		}
		if !bytes.Equal(first[i].Content, second[i].Content) {
			t.Errorf("%s content differs across calls", first[i].Path)
		}
	}
}
