package gotooling

import (
	"context"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "go-tooling" {
		t.Fatalf("Name() = %q, want %q", got, "go-tooling")
	}
}

func TestResolve(t *testing.T) {
	resources, err := New().Resolve(context.Background(), &module.Context{RepoRoot: "/anywhere"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("Resolve returned %d resources, want 1", len(resources))
	}

	r := resources[0]
	if r.Path != ".golangci.yml" {
		t.Errorf("Path = %q, want %q", r.Path, ".golangci.yml")
	}
	if r.Ownership != resource.Generated {
		t.Errorf("Ownership = %v, want %v", r.Ownership, resource.Generated)
	}
	if len(r.Content) == 0 {
		t.Error("Content is empty, want embedded golangci config")
	}
}

func TestResolveDeterministic(t *testing.T) {
	m := New()
	first, err := m.Resolve(context.Background(), &module.Context{})
	if err != nil {
		t.Fatalf("first Resolve returned error: %v", err)
	}
	second, err := m.Resolve(context.Background(), &module.Context{})
	if err != nil {
		t.Fatalf("second Resolve returned error: %v", err)
	}

	if string(first[0].Content) != string(second[0].Content) {
		t.Error("Resolve produced different content across calls")
	}
}
