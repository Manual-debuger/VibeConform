// Package tsrepotooling provides the TypeScript verification entry
// points: the Taskfile that CI, git hooks, and docs all call rather than
// duplicating command lists, and the lefthook configuration that runs fast
// checks before a commit. Mirrors internal/module/repotooling's shape for
// prod-go/v1. See docs/specs/0016-ts-py-tooling-parity.md. Every command
// runs through pnpm; see docs/specs/0020-prod-ts-pnpm.md.
package tsrepotooling

import (
	"context"
	_ "embed"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/Taskfile.yml
var taskfile []byte

//go:embed templates/lefthook.yml
var lefthookConfig []byte

// guard is the PreToolUse guard the hook:guard task runs; see
// docs/specs/0021-agent-hooks-task-interface.md.
//
//go:embed templates/guard.mjs
var guard []byte

type tsrepotoolingModule struct{}

// New returns the ts-repo-tooling module.
func New() module.Module {
	return tsrepotoolingModule{}
}

func (tsrepotoolingModule) Name() string {
	return "ts-repo-tooling"
}

// RequiredTools reports the binaries this module's two resources are
// instructions for. All three are normally installed globally rather than
// per project, so their absence from PATH is a real finding rather than a
// false alarm about how the repository manages its dependencies. pnpm is
// the one that runs the project-local tools (spec 0020); eslint, prettier,
// and tsc themselves stay undeclared, as ts-tooling explains.
func (tsrepotoolingModule) RequiredTools() []module.Tool {
	return []module.Tool{
		{Name: "task", Why: "every verification entry point Taskfile.yml defines"},
		{Name: "lefthook", Why: "the pre-commit hooks lefthook.yml describes, which vibe sync registers"},
		{Name: "pnpm", Why: "every Taskfile and lefthook command, which run through pnpm exec"},
	}
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (tsrepotoolingModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{
			Path:      "Taskfile.yml",
			Ownership: resource.Generated,
			Content:   taskfile,
		},
		{
			Path:      "lefthook.yml",
			Ownership: resource.Generated,
			Content:   lefthookConfig,
		},
		{
			// Default mode: it is run through an interpreter, never executed
			// directly, so no mode bit can switch it off.
			Path:      ".claude/hooks/guard.mjs",
			Ownership: resource.Generated,
			Content:   guard,
		},
	}, nil
}
