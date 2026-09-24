// Package codex provides the Codex CLI configuration a conformant
// repository gets: .codex/config.toml and .codex/hooks.json. See
// docs/specs/0012-agent-config-module.md and
// docs/specs/0024-claude-first-agent-modules.md.
//
// Codex's hook contract differs from Claude Code's (spec 0024's canary):
// on Windows it ignores exit code 2, so neither a PreToolUse deny nor a
// Stop block takes effect, and its background hooks report only through
// JSON additionalContext, never stderr. Hooks written for Claude Code's
// contract therefore don't carry over, and this module is its own module
// rather than a second half of Claude's.
package codex

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/config.toml
var config []byte

//go:embed templates/hooks.json
var hooks []byte

type codexModule struct{}

// New returns the codex-config module.
func New() module.Module {
	return codexModule{}
}

func (codexModule) Name() string {
	return "codex-config"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (codexModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      ".codex/config.toml",
			Ownership: resource.Generated,
			Content:   config,
		},
		{
			Path:      ".codex/hooks.json",
			Ownership: resource.Generated,
			Content:   hooks,
		},
	}, nil
}
