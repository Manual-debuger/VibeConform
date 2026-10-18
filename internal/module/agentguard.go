package module

import (
	"bytes"
	"errors"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
)

// The values of vibe.yaml's policy.agent_guard (spec 0053, "This
// repository"). AgentGuardRuntime is the standard's runtime guard, run
// through "task -x hook:guard"; it is what an absent key selects.
// AgentGuardVibeGuard runs the reference guard, vibe-guard, instead.
const (
	AgentGuardRuntime   = "runtime"
	AgentGuardVibeGuard = "vibe-guard"
)

// VibeGuard reports whether vibe.yaml selects vibe-guard as the agent
// guard. A nil context or an absent key is the runtime guard, so every
// repository that does not opt in resolves exactly what it did before.
func VibeGuard(mctx *Context) bool {
	return mctx != nil && mctx.Policies[manifest.PolicyAgentGuard] == AgentGuardVibeGuard
}

// guardTaskNote replaces the hook:guard task and its comment when
// vibe-guard is the agent guard. It keeps the agent-hook block's first
// line, so StripAgentHooks still finds the block.
const guardTaskNote = "  # Called by Claude Code, not by people. The PreToolUse guard is vibe-guard\n" +
	"  # (policy.agent_guard in vibe.yaml), which .claude/settings.json runs\n" +
	"  # directly, so there is no hook:guard task. See\n" +
	"  # docs/specs/0053-contract-probes-and-guard.md.\n" +
	"\n"

// StripGuardTask removes the hook:guard task, and the comment above it
// that opens the agent-hook block, from a repo-tooling Taskfile, for a
// repository whose agent guard is vibe-guard. The other hook:* tasks
// stay. It is an error for the block not to start with hook:guard, so a
// template edit that moves the task fails the module's tests.
func StripGuardTask(taskfile []byte) ([]byte, error) {
	lines := bytes.SplitAfter(taskfile, []byte("\n"))
	start := -1
	for i, l := range lines {
		if hookBlockStart.Match(l) {
			if start >= 0 {
				return nil, errors.New("strip guard task: two agent-hook blocks")
			}
			start = i
		}
	}
	if start < 0 {
		return nil, errors.New("strip guard task: no agent-hook block")
	}
	key := -1
	for i := start; i < len(lines); i++ {
		if m := taskKey.FindSubmatch(trimEOL(lines[i])); m != nil {
			if string(m[1]) != "hook:guard" {
				return nil, errors.New("strip guard task: the agent-hook block does not start with hook:guard")
			}
			key = i
			break
		}
	}
	if key < 0 {
		return nil, errors.New("strip guard task: no hook:guard task")
	}
	// The task's body ends at the next tasks-level line; the blank lines
	// before that line go with it.
	end := len(lines)
	for i := key + 1; i < len(lines); i++ {
		if indentTwo.Match(trimEOL(lines[i])) {
			end = i
			break
		}
	}
	out := bytes.Join(lines[:start], nil)
	out = append(out, guardTaskNote...)
	out = append(out, bytes.Join(lines[end:], nil)...)
	return out, nil
}
