// Package module defines the composition unit that turns a standard's
// configuration into concrete resources, mirroring the projen-style
// standard -> modules -> resolved resources pipeline described in
// docs/architecture/overview.md.
package module

import (
	"context"
	"slices"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// Context carries whatever a Module needs to resolve resources: the target
// repository root and the parts of vibe.yaml a module may depend on. It is
// intentionally minimal; fields are added when a module needs them.
//
// A nil *Context is valid and means "no repository in particular": a
// module that reads nothing from it must accept nil, which is how tests
// resolve a standard in isolation.
type Context struct {
	// RepoRoot is the absolute path of the repository being reconciled.
	RepoRoot string
	// Components is vibe.yaml's component list, in declaration order.
	// Empty for every standard that takes none; see
	// docs/decisions/0012-manifest-components.md.
	Components []manifest.Component
	// Integrations names the optional integrations vibe.yaml selects, in
	// catalog order. nil means the selection is unknown — a test resolving
	// a module in isolation — and never "none", which is an empty slice
	// (docs/decisions/0013-optional-integrations.md).
	Integrations []string
	// Profiles lists the languages the repository declares: a
	// single-language standard's own, or its components', in
	// manifest.Profiles order.
	Profiles []manifest.Profile
}

// AgentHookIntegration is the integration whose settings run the hook:*
// tasks and the guard that core repo-tooling modules generate.
const AgentHookIntegration = "claude"

// WantsAgentHooks reports whether a core module should generate the agent
// hook surface: the hook:* tasks and the guard program. It fails safe: an
// unknown selection (nil context, or nil Integrations) keeps them, so
// nothing but an explicit vibe.yaml selection without claude can switch a
// guard off.
func WantsAgentHooks(mctx *Context) bool {
	if mctx == nil || mctx.Integrations == nil {
		return true
	}
	return slices.Contains(mctx.Integrations, AgentHookIntegration)
}

// ProfilesOf returns mctx's profiles, or none for a nil context.
func ProfilesOf(mctx *Context) []manifest.Profile {
	if mctx == nil {
		return nil
	}
	return mctx.Profiles
}

// ComponentsOf returns mctx's components, or none for a nil context.
func ComponentsOf(mctx *Context) []manifest.Component {
	if mctx == nil {
		return nil
	}
	return mctx.Components
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
	// Version is the arguments that make the binary print its version on
	// the first line of stdout, for vibe doctor. nil when it has none.
	Version []string
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
	// RequiredTools lists the binaries this module's resources need. It
	// takes the same context as Resolve, since what a module writes (and
	// so what it needs) may depend on vibe.yaml.
	RequiredTools(mctx *Context) []Tool
}
