// Package agents provides the AI-agent configuration a conformant
// repository gets: Claude Code and Codex settings, plus the pre-tool-use
// hook scripts both of them invoke to block destructive shell commands.
// See docs/specs/0012-agent-config-module.md.
//
// Claude and Codex are one module rather than two because they share the
// hook scripts: a settings file naming a hook that was never written is
// worse than neither file.
package agents

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/claude/settings.json
var claudeSettings []byte

//go:embed templates/claude/hooks/block-dangerous.sh
var blockDangerousHook []byte

//go:embed templates/claude/hooks/block-secret-files.sh
var blockSecretFilesHook []byte

//go:embed templates/codex/config.toml
var codexConfig []byte

//go:embed templates/codex/hooks.json
var codexHooks []byte

// hookMode makes the hook scripts executable. A hook script that is not
// executable does not run, and it fails open — the guardrail silently stops
// guarding. See docs/decisions/0006-resource-file-mode.md.
const hookMode = 0o755

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
func (agentsModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      ".claude/settings.json",
			Ownership: resource.Generated,
			Content:   claudeSettings,
		},
		{
			Path:      ".claude/hooks/block-dangerous.sh",
			Ownership: resource.Generated,
			Content:   blockDangerousHook,
			Mode:      hookMode,
		},
		{
			Path:      ".claude/hooks/block-secret-files.sh",
			Ownership: resource.Generated,
			Content:   blockSecretFilesHook,
			Mode:      hookMode,
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
