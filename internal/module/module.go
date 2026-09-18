// Package module defines the composition unit that turns a standard's
// configuration into concrete resources, mirroring the projen-style
// standard -> modules -> resolved resources pipeline described in
// docs/architecture/overview.md.
package module

import (
	"context"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// Context carries whatever a Module needs to resolve resources: the target
// repository root and, eventually, the parsed vibe.yaml and prior lock
// state. It is intentionally minimal at bootstrap time.
type Context struct {
	// RepoRoot is the absolute path of the repository being reconciled.
	RepoRoot string
}

// Module resolves its slice of desired state into concrete resources.
// Implementations must be deterministic: the same Context must always
// produce the same resources.
type Module interface {
	// Name identifies the module for diagnostics and lock-file bookkeeping.
	Name() string
	// Resolve computes the resources this module contributes.
	Resolve(ctx context.Context, mctx *Context) ([]resource.Resource, error)
}
