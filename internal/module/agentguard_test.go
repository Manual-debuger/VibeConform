package module

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
)

func TestStripGuardTask(t *testing.T) {
	got, err := StripGuardTask([]byte(hookFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := "tasks:\n  lint:\n    cmds:\n      - go vet ./...\n\n" + guardTaskNote +
		"  # The other agent hooks.\n  hook:done:\n    cmds:\n      - |\n        done\n\n" +
		"  # Not a hook.\n  verify:\n    cmds:\n      - task: lint\n"
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// The block still starts as StripAgentHooks expects.
	if _, err := StripAgentHooks(got); err != nil {
		t.Errorf("StripAgentHooks after StripGuardTask: %v", err)
	}
}

func TestStripGuardTaskRefusesAmbiguousTemplates(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"no block":    {"tasks:\n  lint:\n    cmds: [x]\n", "no agent-hook block"},
		"no task":     {"tasks:\n  # Called by Claude Code.\n", "no hook:guard task"},
		"other first": {"tasks:\n  # Called by Claude Code.\n  hook:done:\n    cmds: [x]\n", "does not start with hook:guard"},
		"two blocks":  {hookFixture + "  # Called by Claude Code again.\n  hook:x:\n    cmds: [x]\n", "two agent-hook blocks"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := StripGuardTask([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestVibeGuard(t *testing.T) {
	for name, tc := range map[string]struct {
		mctx *Context
		want bool
	}{
		"nil context": {nil, false},
		"absent":      {&Context{Policies: map[string]string{}}, false},
		"runtime":     {&Context{Policies: map[string]string{manifest.PolicyAgentGuard: AgentGuardRuntime}}, false},
		"vibe-guard":  {&Context{Policies: map[string]string{manifest.PolicyAgentGuard: AgentGuardVibeGuard}}, true},
	} {
		if got := VibeGuard(tc.mctx); got != tc.want {
			t.Errorf("%s: VibeGuard = %v, want %v", name, got, tc.want)
		}
	}
}
