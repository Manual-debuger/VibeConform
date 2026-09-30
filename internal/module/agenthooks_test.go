package module

import (
	"strings"
	"testing"
)

const hookFixture = `tasks:
  lint:
    cmds:
      - go vet ./...

  # Called by Claude Code, not by people.
  hook:guard:
    cmds:
      - guard

  # The other agent hooks.
  hook:done:
    cmds:
      - |
        done

  # Not a hook.
  verify:
    cmds:
      - task: lint
`

func TestStripAgentHooks(t *testing.T) {
	got, err := StripAgentHooks([]byte(hookFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := "tasks:\n  lint:\n    cmds:\n      - go vet ./...\n\n  # Not a hook.\n  verify:\n    cmds:\n      - task: lint\n"
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestStripAgentHooksCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(hookFixture, "\n", "\r\n")
	got, err := StripAgentHooks([]byte(crlf))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "hook:") || !strings.Contains(string(got), "  verify:\r\n") {
		t.Errorf("CRLF strip:\n%q", got)
	}
}

func TestStripAgentHooksRefusesAmbiguousTemplates(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"no block":   {"tasks:\n  lint:\n    cmds: [x]\n", "no agent-hook block"},
		"no hooks":   {"tasks:\n  # Called by Claude Code.\n  lint:\n    cmds: [x]\n", "defines no hook:* task"},
		"two blocks": {hookFixture + "  # Called by Claude Code again.\n  hook:x:\n    cmds: [x]\n", "two agent-hook blocks"},
		"interloper": {
			"tasks:\n  # Called by Claude Code.\n  hook:a:\n    cmds: [x]\n  lint:\n    cmds: [x]\n  hook:b:\n    cmds: [x]\n",
			"task lint inside the agent-hook block",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := StripAgentHooks([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestWantsAgentHooksFailsSafe(t *testing.T) {
	for name, tc := range map[string]struct {
		mctx *Context
		want bool
	}{
		"nil context":       {nil, true},
		"unknown selection": {&Context{}, true},
		"claude selected":   {&Context{Integrations: []string{"claude", "codex"}}, true},
		"no agents":         {&Context{Integrations: []string{}}, false},
		"codex only":        {&Context{Integrations: []string{"codex"}}, false},
	} {
		if got := WantsAgentHooks(tc.mctx); got != tc.want {
			t.Errorf("%s: WantsAgentHooks = %v, want %v", name, got, tc.want)
		}
	}
}
