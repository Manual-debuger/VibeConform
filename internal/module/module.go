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

// Tool is an external binary a module's resources depend on: configuration
// for a program that is not installed is a file the repository cannot act
// on. See docs/specs/0014-m2-milestone.md.
type Tool struct {
	// Name is the binary as it must appear on PATH.
	Name string
	// Why names what stops working without it, for the warning text.
	Why string
}

// ToolRequirer is implemented by modules whose resources are inert without
// an external binary on PATH. A module that does not implement it requires
// nothing.
//
// Deliberately optional rather than a Module method: making every module
// return nil to satisfy one caller costs more than a type assertion at the
// call site. Declare only binaries a correctly configured repository would
// genuinely have on PATH — a warning that fires on a healthy repository
// teaches people to ignore the ones that matter.
type ToolRequirer interface {
	// RequiredTools lists the binaries this module's resources need.
	RequiredTools() []Tool
}
