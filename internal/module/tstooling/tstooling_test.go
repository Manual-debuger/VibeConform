package tstooling

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "ts-tooling" {
		t.Errorf("Name() = %q, want %q", got, "ts-tooling")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{"eslint.config.js", ".prettierrc.json", "tsconfig.base.json"}
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
		t.Fatal("ts-tooling should declare node; it does not implement ToolRequirer")
	}

	tools := requirer.RequiredTools()
	if len(tools) != 1 || tools[0].Name != "node" {
		t.Fatalf("RequiredTools() = %+v, want node only", tools)
	}

	// The reason this is one entry and not four: eslint, prettier, and tsc
	// are project-local, so requiring them on PATH would warn on every
	// correctly configured repository. The package manager is not
	// project-local — pnpm is on PATH, and ts-repo-tooling declares it,
	// because its commands are what run pnpm (spec 0020). It stays off this
	// list because this module's lint configuration never invokes a package
	// manager.
	for _, notRequiredHere := range []string{"eslint", "prettier", "tsc", "npm", "pnpm"} {
		for _, tool := range tools {
			if tool.Name == notRequiredHere {
				t.Errorf("ts-tooling requires %s; only node runs its configuration", notRequiredHere)
			}
		}
	}
}

// TestTypedFilesGlobIsNotTheSeedRepositoryLayout guards the one edit made to
// a seeded template that no other test can see. The seed's glob was scoped
// to a pnpm workspace; a config that type-checks nothing still exits 0, so
// this is the failure that would otherwise ship silently.
func TestTypedFilesGlobIsNotTheSeedRepositoryLayout(t *testing.T) {
	if !bytes.Contains(eslintConfig, []byte("const typedFiles = ['**/*.ts']")) {
		t.Error("typedFiles is not the generalized glob; see docs/specs/0014-m2-milestone.md")
	}
	if bytes.Contains(eslintConfig, []byte("packages/")) {
		t.Error("eslint.config.js still names the seed repository's directory layout")
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{
		"eslint.config.js":   eslintConfig,
		"prettierrc.json":    prettierConfig,
		"tsconfig.base.json": tsconfigBase,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}
