// Package agents provides the AI-agent configuration a conformant
// repository gets: Claude Code and Codex settings that run a PreToolUse
// guard before shell commands and file edits, and the policy that guard
// enforces. See docs/specs/0012-agent-config-module.md and
// docs/specs/0021-agent-hooks-task-interface.md.
//
// Both agents call the same command in every standard, "task -x
// hook:guard", so this module is language-neutral. The guard program and
// the task that runs it belong to each standard's repo-tooling module,
// which already knows the language; internal/standard's wiring test checks
// that every standard composing this module also provides them.
//
// Claude and Codex are one module rather than two because they share the
// guard: a settings file naming a hook that was never written is worse
// than neither file.
package agents

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// GuardCommand is what both agents' configs run before a tool call. The -x
// matters: without it Task exits 201 when the guard denies, and both agents
// treat any exit other than 2 as allow.
const GuardCommand = "task -x hook:guard"

//go:embed templates/claude/settings.json
var claudeSettings []byte

//go:embed templates/codex/config.toml
var codexConfig []byte

//go:embed templates/codex/hooks.json
var codexHooks []byte

type agentsModule struct{}

// New returns the agent-config module.
func New() module.Module {
	return agentsModule{}
}

func (agentsModule) Name() string {
	return "agent-config"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
//
// policy.json is the first resource anywhere whose content is computed
// rather than embedded. It is still deterministic, because renderPolicy is.
func (agentsModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	policyJSON, err := renderPolicy(policy)
	if err != nil {
		return nil, err
	}

	return []resource.Resource{
		{
			Path:      ".claude/settings.json",
			Ownership: resource.Generated,
			Content:   claudeSettings,
		},
		{
			Path:      ".claude/hooks/policy.json",
			Ownership: resource.Generated,
			Content:   policyJSON,
		},
		{
			Path:      ".codex/config.toml",
			Ownership: resource.Generated,
			Content:   codexConfig,
		},
		{
			Path:      ".codex/hooks.json",
			Ownership: resource.Generated,
			Content:   codexHooks,
		},
	}, nil
}
