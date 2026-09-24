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

// TestCodexHooksMatchDocumentedSchema fixes the bug spec 0021 found: the
// shipped .codex/hooks.json was a flat list with no matcher, no inner hooks
// array, and no type, which Codex's documented schema does not describe.
func TestCodexHooksMatchDocumentedSchema(t *testing.T) {
	var cfg struct {
		Hooks map[string][]struct {
			Matcher *string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
				Async   bool   `json:"async"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	dec := json.NewDecoder(bytes.NewReader(hooks))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf(".codex/hooks.json is not in the documented hooks shape: %v", err)
	}
	for event, entries := range cfg.Hooks {
		for _, e := range entries {
			if len(e.Hooks) == 0 {
				t.Errorf("%s entry has no hooks array; this is the flat shape Codex does not read", event)
			}
			for _, h := range e.Hooks {
				if h.Type != "command" {
					t.Errorf("%s hook type = %q, want command", event, h.Type)
				}
				if !strings.HasPrefix(h.Command, "task -x hook:") {
					t.Errorf("%s runs %q, want task -x hook:<name>", event, h.Command)
				}
			}
		}
	}
	if bytes.Contains(hooks, []byte("commandWindows")) {
		t.Error(".codex/hooks.json sets commandWindows; every hook command is the same on every OS")
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
