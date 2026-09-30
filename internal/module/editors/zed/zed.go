// Package zed provides the zed editor integration: owned entries in
// .zed/tasks.json, one per Task entry point. The file is shared with the
// repository's users, so VibeConform owns only its own tasks (structured
// patch). See docs/specs/0026-optional-integrations.md.
package zed

import (
	"context"
	"fmt"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/editors"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// skeleton is Zed's tasks file with no tasks: a top-level array.
var skeleton = []byte("[]\n")

type zedModule struct{}

// New returns the zed-config module.
func New() module.Module {
	return zedModule{}
}

func (zedModule) Name() string {
	return "zed-config"
}

// Resolve returns .zed/tasks.json with a task per editors.Tasks entry.
func (zedModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	tasks := &resource.ArrayPatch{Skeleton: skeleton}
	for _, t := range editors.Tasks {
		tasks.Elements = append(tasks.Elements, resource.Element{
			ID:    editors.Label(t),
			Value: fmt.Appendf(nil, `{"label": %q, "command": "task", "args": [%q]}`, editors.Label(t), t),
		})
	}
	return []resource.Resource{{Path: ".zed/tasks.json", Ownership: resource.StructuredPatch, Patch: tasks}}, nil
}
