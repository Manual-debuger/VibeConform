package agents

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
	if got := New().Name(); got != "agent-config" {
		t.Errorf("Name() = %q, want %q", got, "agent-config")
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

	want := []string{
		".claude/settings.json",
		".claude/hooks/policy.json",
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

// TestNoResourceDependsOnModeBit pins what spec 0021 removed: the bash
// hooks needed 0755, and a chmod -x silently disabled them. Every guard is
// now run through an interpreter, so nothing here may need a mode bit.
func TestNoResourceDependsOnModeBit(t *testing.T) {
	for _, r := range resolve(t) {
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

// hookCommands returns every command a Claude Code or Codex hooks config
// runs, decoding the documented nested shape. It fails the test if the
// config is not in that shape, which is what the flat Codex config shipped
// before spec 0021 was.
func hookCommands(t *testing.T, name string, data []byte) (matchers, commands []string) {
	t.Helper()
	var cfg struct {
		Hooks map[string][]struct {
			Matcher *string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("%s is not in the documented hooks shape: %v", name, err)
	}

	entries := cfg.Hooks["PreToolUse"]
	if len(entries) == 0 {
		t.Fatalf("%s has no PreToolUse entries", name)
	}
	for _, e := range entries {
		if e.Matcher == nil {
			t.Errorf("%s has a PreToolUse entry with no matcher", name)
		} else {
			matchers = append(matchers, *e.Matcher)
		}
		if len(e.Hooks) == 0 {
			t.Errorf("%s has a PreToolUse entry with no hooks array; this is the flat shape Codex does not read", name)
		}
		for _, h := range e.Hooks {
			if h.Type != "command" {
				t.Errorf("%s hook type = %q, want command", name, h.Type)
			}
			commands = append(commands, h.Command)
		}
	}
	return matchers, commands
}

// TestAgentConfigsCallTaskWithExitCode is the -x regression test. Without
// -x, Task exits 201 when the guard denies, and both agents treat 201 as
// allow: every guardrail would silently stop guarding.
func TestAgentConfigsCallTaskWithExitCode(t *testing.T) {
	for name, data := range map[string][]byte{
		".claude/settings.json": claudeSettings,
		".codex/hooks.json":     codexHooks,
	} {
		_, commands := hookCommands(t, name, data)
		for _, c := range commands {
			if c != GuardCommand {
				t.Errorf("%s runs %q, want exactly %q", name, c, GuardCommand)
			}
		}
	}
}

// TestClaudeMatcherCoversCommandsAndEdits checks that Claude Code routes
// every tool kind the policy has rules for to the guard.
func TestClaudeMatcherCoversCommandsAndEdits(t *testing.T) {
	matchers, _ := hookCommands(t, ".claude/settings.json", claudeSettings)
	joined := strings.Join(matchers, "|")
	for _, tools := range policy.Tools {
		for _, tool := range tools {
			if tool == "MultiEdit" {
				continue // matched by "Edit": Claude Code matchers are regular expressions
			}
			if !strings.Contains("|"+joined+"|", "|"+tool+"|") {
				t.Errorf("no .claude/settings.json matcher routes %s to the guard", tool)
			}
		}
	}
}

// TestCodexHooksMatchDocumentedSchema fixes the bug spec 0021 found: the
// shipped .codex/hooks.json was a flat list with no matcher, no inner hooks
// array, and no type, which Codex's documented schema does not describe.
func TestCodexHooksMatchDocumentedSchema(t *testing.T) {
	matchers, commands := hookCommands(t, ".codex/hooks.json", codexHooks)
	if len(matchers) != 1 || matchers[0] != "Bash" {
		t.Errorf(".codex/hooks.json matchers = %v, want [Bash]", matchers)
	}
	if len(commands) != 1 {
		t.Errorf(".codex/hooks.json runs %d commands, want 1", len(commands))
	}
	if bytes.Contains(codexHooks, []byte("commandWindows")) {
		t.Error(".codex/hooks.json sets commandWindows; the guard command is the same on every OS")
	}
}

func TestTemplatesMatchLiveFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	policyJSON, err := renderPolicy(policy)
	if err != nil {
		t.Fatalf("renderPolicy: %v", err)
	}

	for _, tc := range []struct {
		live     string
		embedded []byte
	}{
		{filepath.Join(".claude", "settings.json"), claudeSettings},
		{filepath.Join(".claude", "hooks", "policy.json"), policyJSON},
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

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{
		"settings.json": claudeSettings,
		"config.toml":   codexConfig,
		"hooks.json":    codexHooks,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}
