package pythontooling

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "python-tooling" {
		t.Errorf("Name() = %q, want %q", got, "python-tooling")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{"ruff.toml", "pyrightconfig.json"}
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

// TestDeclaresNoRequiredTools pins a decision rather than an accident. ruff
// and pyright are normally pinned per project and run through uv or a
// virtualenv, so checking PATH for them would warn on a healthy repository —
// and a warning that fires on correct setups trains people to ignore the
// ones that matter.
func TestDeclaresNoRequiredTools(t *testing.T) {
	if _, ok := New().(module.ToolRequirer); ok {
		t.Error("python-tooling declares required tools; ruff and pyright are project-local, " +
			"see docs/specs/0014-m2-milestone.md before adding them")
	}
}

// TestPyrightConfigIsValidJSON matters more here than for a template copied
// byte for byte: this one was transcribed out of pyproject.toml's
// [tool.pyright] table, so a syntax error would be ours rather than the
// seed's.
func TestPyrightConfigIsValidJSON(t *testing.T) {
	var parsed map[string]any
	if err := json.Unmarshal(pyrightConfig, &parsed); err != nil {
		t.Fatalf("pyrightconfig.json is not valid JSON: %v", err)
	}

	if _, ok := parsed["include"]; ok {
		t.Error("include names the seed repository's package directory; " +
			"without it pyright checks the project directory, which is the right default for a standard")
	}
	for _, key := range []string{"pythonVersion", "typeCheckingMode"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("pyrightconfig.json is missing %q", key)
		}
	}
}

// TestRuffConfigIsRerootedForStandaloneUse guards the transcription: in
// pyproject.toml these are [tool.ruff.*] tables, and in ruff.toml they are
// top-level. A stray "tool." prefix would leave ruff silently applying its
// defaults.
func TestRuffConfigIsRerootedForStandaloneUse(t *testing.T) {
	// Table headers only. The file's own header comment explains why it is
	// not a pyproject.toml section, so a plain substring search for
	// "[tool.ruff" matches the prose saying exactly that.
	for line := range strings.SplitSeq(string(ruffConfig), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[tool.") {
			t.Errorf("ruff.toml still uses pyproject.toml's table names (%s); "+
				"standalone config puts lint settings under [lint]", line)
		}
	}
	for _, want := range []string{"[lint]", "[lint.per-file-ignores]", "target-version"} {
		if !bytes.Contains(ruffConfig, []byte(want)) {
			t.Errorf("ruff.toml is missing %q", want)
		}
	}
	// app/routes/*.py was a FastAPI-specific ignore in the seed repository.
	if bytes.Contains(ruffConfig, []byte("app/routes")) {
		t.Error("ruff.toml still carries the seed repository's FastAPI-specific per-file ignore")
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{
		"ruff.toml":          ruffConfig,
		"pyrightconfig.json": pyrightConfig,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}
