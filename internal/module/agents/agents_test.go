package agents

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "agent-config" {
		t.Errorf("Name() = %q, want %q", got, "agent-config")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{
		".claude/settings.json",
		".claude/hooks/block-dangerous.sh",
		".claude/hooks/block-secret-files.sh",
		".codex/config.toml",
		".codex/hooks.json",
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

// TestHookScriptsAreExecutable pins the reason this module needed a file
// mode at all: a hook script that is not executable fails open, so the
// guardrail stops guarding without anything reporting an error.
func TestHookScriptsAreExecutable(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, r := range resources {
		isScript := strings.HasSuffix(r.Path, ".sh")
		mode := r.ModeOrDefault()
		switch {
		case isScript && mode != 0o755:
			t.Errorf("%s mode = %v, want 0755 — a non-executable hook silently does not run", r.Path, mode)
		case !isScript && mode != 0o644:
			t.Errorf("%s mode = %v, want the 0644 default", r.Path, mode)
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

func TestTemplatesMatchLiveFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	for _, tc := range []struct {
		live     string
		embedded []byte
	}{
		{filepath.Join(".claude", "settings.json"), claudeSettings},
		{filepath.Join(".claude", "hooks", "block-dangerous.sh"), blockDangerousHook},
		{filepath.Join(".claude", "hooks", "block-secret-files.sh"), blockSecretFilesHook},
		{filepath.Join(".codex", "config.toml"), codexConfig},
		{filepath.Join(".codex", "hooks.json"), codexHooks},
	} {
		t.Run(tc.live, func(t *testing.T) {
			live, err := os.ReadFile(filepath.Join(repoRoot, tc.live))
			if err != nil {
				t.Fatalf("reading live file: %v", err)
			}
			if !bytes.Equal(live, tc.embedded) {
				t.Errorf("embedded template has drifted from %s — "+
					"read the difference before resolving it; these files are guardrails", tc.live)
			}
		})
	}
}

// TestHookScriptsAreLF matters beyond the usual cross-platform hashing
// concern: these scripts are run through bash, including on Windows via
// .codex/hooks.json's commandWindows, and CRLF breaks a shebang script
// under bash.
func TestHookScriptsAreLF(t *testing.T) {
	for name, content := range map[string][]byte{
		"block-dangerous.sh":    blockDangerousHook,
		"block-secret-files.sh": blockSecretFilesHook,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; bash will not run it correctly", name)
		}
	}
}
