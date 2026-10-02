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
	"github.com/Manual-debuger/VibeConform/internal/module/editors"
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
func (tsrepotoolingModule) RequiredTools(_ *module.Context) []module.Tool {
	return []module.Tool{
		{Name: "task", Why: "every verification entry point Taskfile.yml defines", Version: []string{"--version"}},
		{Name: "lefthook", Why: "the pre-commit hooks lefthook.yml describes, which vibe sync registers", Version: []string{"version"}},
		{Name: "pnpm", Why: "every Taskfile and lefthook command, which run through pnpm exec", Version: []string{"--version"}},
	}
}

// HookBinaries reports task, which every hook command runs through, and
// the runtime hook:guard starts.
func (tsrepotoolingModule) HookBinaries(_ *module.Context) []string {
	return []string{"task", "node"}
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (tsrepotoolingModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	// Without an agent running them, the hook:* tasks and the guard are
	// integration-specific content in core files; see
	// docs/specs/0026-optional-integrations.md. An unknown selection keeps
	// them (module.WantsAgentHooks).
	hooks := module.WantsAgentHooks(mctx)
	taskfile := taskfile
	if !hooks {
		stripped, err := module.StripAgentHooks(taskfile)
		if err != nil {
			return nil, err
		}
		taskfile = stripped
	}
	// Prettier rewrites the layout of the elements an editor integration
	// owns, so fmt, fmt:check, lefthook, and hook:format leave those files
	// alone (spec 0026 §10).
	owned := editors.OwnedPathsOf(integrationsOf(mctx))
	taskfile, err := excludeFromPrettierTaskfile(taskfile, owned, hooks)
	if err != nil {
		return nil, err
	}
	lefthook, err := excludeFromPrettierLefthook(lefthookConfig, owned)
	if err != nil {
		return nil, err
	}
	if module.Selected(mctx, module.GraphifyIntegration) {
		if taskfile, lefthook, err = module.AddGraphify(taskfile, lefthook, hooks); err != nil {
			return nil, err
		}
	}

	resources := []resource.Resource{
		{
			Path:      "Taskfile.yml",
			Ownership: resource.Generated,
			Content:   taskfile,
		},
		{
			Path:      "lefthook.yml",
			Ownership: resource.Generated,
			Content:   lefthook,
		},
	}
	if !hooks {
		return resources, nil
	}
	return append(resources, resource.Resource{
		// Default mode: it is run through an interpreter, never executed
		// directly, so no mode bit can switch it off.
		Path:      ".claude/hooks/guard.mjs",
		Ownership: resource.Generated,
		Content:   guard,
	}), nil
}

func integrationsOf(mctx *module.Context) []string {
	if mctx == nil {
		return nil
	}
	return mctx.Integrations
}
