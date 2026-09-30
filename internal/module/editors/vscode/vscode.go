// Package vscode provides the vscode editor integration: owned entries in
// .vscode/tasks.json, one per Task entry point, and in
// .vscode/extensions.json, the extensions for each declared language. Both
// files are shared with the repository's users, so VibeConform owns only
// its own elements (structured patch); every other task, recommendation,
// key, and comment is theirs. See docs/specs/0026-optional-integrations.md.
package vscode

import (
	"context"
	"fmt"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/editors"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// recommendations are the extensions each profile's tooling needs in VS
// Code: the language support and the configured linters and formatters.
var recommendations = map[manifest.Profile][]string{
	manifest.ProfileGo: {"golang.go"},
	manifest.ProfileTS: {"dbaeumer.vscode-eslint", "esbenp.prettier-vscode"},
	manifest.ProfilePy: {"charliermarsh.ruff", "ms-python.python"},
}

var (
	tasksSkeleton      = []byte("{\n  \"version\": \"2.0.0\",\n  \"tasks\": []\n}\n")
	extensionsSkeleton = []byte("{\n  \"recommendations\": []\n}\n")
)

type vscodeModule struct{}

// New returns the vscode-config module.
func New() module.Module {
	return vscodeModule{}
}

func (vscodeModule) Name() string {
	return "vscode-config"
}

// Resolve returns tasks.json, then extensions.json when any profile is
// declared. Elements are in a fixed order: tasks as editors.Tasks lists
// them, recommendations by profile in manifest.Profiles order.
func (vscodeModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	tasks := &resource.ArrayPatch{Array: "tasks", Skeleton: tasksSkeleton}
	for _, t := range editors.Tasks {
		tasks.Elements = append(tasks.Elements, resource.Element{
			ID: editors.Label(t),
			Value: fmt.Appendf(nil, `{"label": %q, "type": "shell", "command": "task", "args": [%q], "problemMatcher": []}`,
				editors.Label(t), t),
		})
	}
	resources := []resource.Resource{{Path: ".vscode/tasks.json", Ownership: resource.StructuredPatch, Patch: tasks}}

	extensions := &resource.ArrayPatch{Array: "recommendations", Skeleton: extensionsSkeleton}
	for _, p := range manifest.Profiles {
		if !declares(mctx, p) {
			continue
		}
		for _, id := range recommendations[p] {
			extensions.Elements = append(extensions.Elements, resource.Element{ID: id, Value: fmt.Appendf(nil, "%q", id)})
		}
	}
	if len(extensions.Elements) > 0 {
		resources = append(resources, resource.Resource{Path: ".vscode/extensions.json", Ownership: resource.StructuredPatch, Patch: extensions})
	}
	return resources, nil
}

func declares(mctx *module.Context, p manifest.Profile) bool {
	for _, d := range module.ProfilesOf(mctx) {
		if d == p {
			return true
		}
	}
	return false
}
