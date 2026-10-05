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
	// Policies maps each repository policy vibe.yaml selects to its value,
	// e.g. "line_endings": "lf" (docs/decisions/0014-managed-sections.md).
	Policies map[string]string
	// DocsDirs is where the docs layout puts each kind of document, with
	// any directory vibe.yaml adopts (spec 0034). Zero means the defaults.
	DocsDirs manifest.DocsDirs
	// Profiles lists the languages the repository declares: a
	// single-language standard's own, or its components', in
	// manifest.Profiles order.
	Profiles []manifest.Profile
	// Generated lists the files a generator owns, relative to the root of
	// the project a tooling module configures: vibe.yaml's top-level list
	// for a single-language standard, or one component's list, which
	// monotooling passes to each component's tooling (spec 0037). Empty
	// leaves every template unchanged.
	Generated []string
}

// GeneratedOf returns the context's generated paths; nil for a nil
// context.
func GeneratedOf(mctx *Context) []string {
	if mctx == nil {
		return nil
	}
	return mctx.Generated
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

// GraphifyIntegration is the code-intelligence integration whose graph
// update task, Git hook jobs, and context line core repo-tooling modules
// generate (docs/specs/0035-graphify.md).
const GraphifyIntegration = "graphify"

// Selected reports whether vibe.yaml explicitly selects the named
// integration. Unlike WantsAgentHooks it fails closed: an unknown
// selection (nil context, or nil Integrations) selects nothing, so a
// default-off integration never appears in a module resolved in
// isolation.
func Selected(mctx *Context, name string) bool {
	return mctx != nil && slices.Contains(mctx.Integrations, name)
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
	// Optional marks a tool whose absence degrades an optional capability
	// rather than stopping a required workflow: vibe doctor reports it as a
	// warning, not a failure, and Install says how to get it.
	Optional bool
	// Install is a one-line hint for obtaining an optional tool.
	Install string
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

// HookRuntime is implemented by the core modules that generate the agent
// hook surface (the hook:* tasks and the guard). It names the binaries
// those hook commands start, so vibe doctor can say whether the selected
// agent's hooks can run on this machine (docs/specs/0028-environment-doctor.md).
// Optional, like ToolRequirer, and only asked when an agent that runs the
// hooks is selected.
type HookRuntime interface {
	// HookBinaries lists the binaries every hook command needs: task,
	// then the guard's runtime.
	HookBinaries(mctx *Context) []string
}

// Retirer is implemented by a module that once generated a whole file it
// no longer does, such as the /spec command the spec skill replaced. Sync
// treats a recorded retired path as it treats a deselected option's file:
// removed if unchanged since sync, kept as a conflict if modified, never
// touched if unrecorded (docs/specs/0031-spec-skill.md §3).
type Retirer interface {
	// Retired maps each retired path, slash-separated, to the reason
	// messages give for removing it.
	Retired() map[string]string
}

// ConditionalSectioner is implemented by a module that resolves a managed
// section only for some vibe.yaml values, like ts-tooling's generated
// section of .prettierignore, which exists only while generated: lists
// paths. A recorded section with one of these IDs that the current plan
// does not resolve is removed under the rules for a deselected option's
// section (docs/specs/0037-generated-paths.md §5).
type ConditionalSectioner interface {
	// ConditionalSections describes each such section: its SectionID,
	// Markers and Placement. Path is not used.
	ConditionalSections() []resource.Resource
}

// SectionMover is implemented by a module whose section can move to
// another file when vibe.yaml changes, like the specs section, which
// follows development.specs_dir. A recorded section with one of these IDs,
// at a path the module no longer resolves it at, is removed from its old
// file under the rules for a deselected option's section
// (docs/specs/0034-adopt-docs-layout.md §3).
type SectionMover interface {
	MovableSections() []string
}

// SectionChecker is implemented by a module whose managed section can be
// defeated by text around it, such as a later .gitattributes rule that
// overrides the line-ending policy for every path. The check reads; it
// never edits (docs/decisions/0014-managed-sections.md §3).
type SectionChecker interface {
	// CheckSection sees the file as sync would leave it: the bytes before
	// and after r's section. Conflicts hold the file unwritten and fail
	// audit; warnings are reported and fail nothing.
	CheckSection(r resource.Resource, before, after []byte) (conflicts, warnings []string)
}

// ProjectFileGuard is implemented by a module that opens a seam for a
// project-owned file and limits what the file may hold, such as GitLab's
// .gitlab-ci.defaults.yml, which may set only default:. VibeConform never
// writes the file; commands read it and report each problem as a conflict
// (docs/specs/0040-ci-floor-environment-seam.md). Optional, like
// SectionChecker.
type ProjectFileGuard interface {
	// GuardedFiles lists the files to check under mctx. An absent file is
	// never checked.
	GuardedFiles(mctx *Context) []GuardedFile
}

// GuardedFile is one project-owned file a ProjectFileGuard checks.
type GuardedFile struct {
	// Path is relative to the repository root and slash-separated.
	Path string
	// Check returns what is wrong with content; none means conformant. It
	// reads; it never edits.
	Check func(content []byte) []string
}
