// Package claude provides the Claude Code configuration a conformant
// repository gets: .claude/settings.json, which runs the standard's hook
// tasks at each lifecycle event, and the policy the PreToolUse guard
// enforces. See docs/specs/0012-agent-config-module.md,
// docs/specs/0021-agent-hooks-task-interface.md,
// docs/specs/0023-agent-lifecycle-hooks.md, and
// docs/specs/0024-claude-first-agent-modules.md.
//
// Claude Code's hook contract, which the tasks are written for: exit 2
// blocks a PreToolUse call or a Stop and shows stderr to the model; exit 2
// from a PostToolUse hook shows stderr to the model; SessionStart stdout
// becomes context; and an asyncRewake hook runs in the background and wakes
// the model on exit 2. Any other exit code is a non-blocking error.
//
// Settings call the same commands in every standard ("task -x hook:<name>"),
// so this module is language-neutral. The guard program and the tasks
// belong to each standard's repo-tooling module, which already knows the
// language; internal/standard's wiring test checks that every standard
// composing this module also defines them.
//
// Each agent runtime has its own module (spec 0024), and nothing requires
// another runtime's module to call these commands.
package claude

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// GuardCommand is what the settings run before a tool call. The -x matters:
// without it Task exits 201 when the guard denies, and Claude Code treats
// any exit other than 2 as allow.
const GuardCommand = "task -x hook:guard"

// The other hooks the settings run (spec 0023), each a task the standard's
// repo-tooling module defines. The -x matters for the same reason as for
// GuardCommand: a failed check exits 2 only if Task passes the code through.
const (
	// ContextCommand runs at session start; its stdout becomes context.
	ContextCommand = "task -x hook:context"
	// FormatCommand runs after each file edit.
	FormatCommand = "task -x hook:format"
	// CheckCommand runs after each file edit, in the background.
	CheckCommand = "task -x hook:check"
	// DoneCommand runs when the agent ends its turn, as a gate.
	DoneCommand = "task -x hook:done"
)

//go:embed templates/settings.json
var settings []byte

type claudeModule struct{}

// New returns the claude-config module.
func New() module.Module {
	return claudeModule{}
}

func (claudeModule) Name() string {
	return "claude-config"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
//
// policy.json's content is computed rather than embedded. It is still
// deterministic, because renderPolicy is.
func (claudeModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	policyJSON, err := renderPolicy(policy)
	if err != nil {
		return nil, err
	}

	return []resource.Resource{
		{
			Path:      ".claude/settings.json",
			Ownership: resource.Generated,
			Content:   settings,
		},
		{
			Path:      ".claude/hooks/policy.json",
			Ownership: resource.Generated,
			Content:   policyJSON,
		},
	}, nil
}
