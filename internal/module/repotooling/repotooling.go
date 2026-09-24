// Package repotooling provides the repository's verification entry points:
// the Taskfile that CI, git hooks, and docs all call rather than duplicating
// command lists, and the lefthook configuration that runs fast checks before
// a commit. See docs/specs/0011-repo-tooling-module.md.
//
// It is separate from internal/module/gotooling because the two answer
// different questions: gotooling owns language-specific lint configuration,
// this module owns how a repository is verified at all.
package repotooling

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
//go:embed templates/guard.go
var guard []byte

type repotoolingModule struct{}

// New returns the repo-tooling module.
func New() module.Module {
	return repotoolingModule{}
}

func (repotoolingModule) Name() string {
	return "repo-tooling"
}

// RequiredTools reports the binaries this module's resources are
// instructions for: every program Taskfile.yml and lefthook.yml call
// (spec 0022). All are normally installed globally rather than per project
// — go install puts the Go tools on PATH — so their absence from PATH is a
// real finding rather than a false alarm about how the repository manages
// its dependencies. golangci-lint is go-tooling's, which configures it.
func (repotoolingModule) RequiredTools() []module.Tool {
	return []module.Tool{
		{Name: "task", Why: "every verification entry point Taskfile.yml defines"},
		{Name: "lefthook", Why: "the pre-commit hooks lefthook.yml describes, which vibe sync registers"},
		{Name: "go", Why: "every build, test, vet, and module task, and task hook:guard"},
		{Name: "goimports", Why: "task fmt, task fmt:check, and the pre-commit format check"},
		{Name: "govulncheck", Why: "task security"},
		{Name: "actionlint", Why: "task workflows:lint"},
	}
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
func (repotoolingModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
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
			Path:      ".claude/hooks/guard.go",
			Ownership: resource.Generated,
			Content:   guard,
		},
	}, nil
}
