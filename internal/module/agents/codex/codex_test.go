package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "codex-config" {
		t.Errorf("Name() = %q, want %q", got, "codex-config")
	}
}

func resolve(t *testing.T) []resource.Resource {
	t.Helper()
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return resources
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources := resolve(t)

	want := []string{".codex/config.toml", ".codex/hooks.json"}
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
		if r.Mode != 0 {
			t.Errorf("%s sets mode %v; agent config must work at the default mode", r.Path, r.Mode)
		}
	}
}

func TestResolveDeterministic(t *testing.T) {
	first, second := resolve(t), resolve(t)
	for i := range first {
		if first[i].Path != second[i].Path || !bytes.Equal(first[i].Content, second[i].Content) {
			t.Errorf("resource %d differs across calls", i)
		}
	}
}

// TestCodexHooksSuspended pins spec 0024. Codex on Windows ignores exit
// code 2, and its background hooks report only through JSON, so the hooks
// written for Claude Code's contract are suspended rather than shipped
// half-working. hooks.json stays a managed file with no hooks, because
// vibe sync never deletes a file a standard stops producing: an empty file
// is what removes the old hooks from existing repositories. Lifting the
// suspension is a new spec, not an edit to make this test pass.
func TestCodexHooksSuspended(t *testing.T) {
	var cfg struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	dec := json.NewDecoder(bytes.NewReader(hooks))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf(".codex/hooks.json does not decode as {\"hooks\": {}}: %v", err)
	}
	if cfg.Hooks == nil {
		t.Error(".codex/hooks.json has no hooks object; it must be present and empty (spec 0024)")
	}
	if len(cfg.Hooks) != 0 {
		t.Errorf(".codex/hooks.json registers %d events; Codex hooks are suspended by spec 0024", len(cfg.Hooks))
	}

	// No hooks key: neither "hooks = true", which would say VibeConform
	// relies on hooks, nor "hooks = false", which would also turn off hooks
	// a user configured in ~/.codex/.
	for line := range strings.Lines(string(config)) {
		key, _, _ := strings.Cut(strings.TrimSpace(line), "=")
		key = strings.TrimSpace(key)
		if key == "[features]" || key == "hooks" || key == "codex_hooks" || key == "features.hooks" {
			t.Errorf(".codex/config.toml sets %q; spec 0024 leaves the hooks feature flag alone", strings.TrimSpace(line))
		}
	}
}

func TestTemplatesMatchLiveFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..", "..")

	for _, tc := range []struct {
		live     string
		embedded []byte
	}{
		{filepath.Join(".codex", "config.toml"), config},
		{filepath.Join(".codex", "hooks.json"), hooks},
	} {
		t.Run(tc.live, func(t *testing.T) {
			live, err := os.ReadFile(filepath.Join(repoRoot, tc.live))
			if err != nil {
				t.Fatalf("reading live file: %v", err)
			}
			if !bytes.Equal(live, tc.embedded) {
				t.Errorf("embedded template has drifted from %s — read the difference before resolving it", tc.live)
			}
		})
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{
		"config.toml": config,
		"hooks.json":  hooks,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}
