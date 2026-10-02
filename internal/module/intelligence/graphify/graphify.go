// Package graphify provides the opt-in Graphify integration: an ignore
// entry for the graph it writes, and, for Claude Code, a skill that says
// how to use the graph and when not to trust it. The graph update task,
// the Git hook jobs, and the session-context line belong to the core
// repo-tooling modules, which read the selection (module.Selected). See
// docs/specs/0035-graphify.md.
package graphify

import (
	"context"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

const (
	// IgnorePath is the file the ignore section lives in.
	IgnorePath = ".gitignore"
	// SectionID names the section within it.
	SectionID = "graphify"
	// SkillPath is the Claude Code project skill.
	SkillPath = ".claude/skills/graphify/SKILL.md"
	// OutputDir is where graphify writes the graph, relative to the
	// repository root.
	OutputDir = "graphify-out"
	// GraphPath is the graph file vibe doctor reads.
	GraphPath = OutputDir + "/graph.json"
)

// ignoreContent is the section, between its markers. Every file under
// graphify-out/ is derived and machine-local, so none of it is committed.
const ignoreContent = "# Managed by VibeConform: integrations.intelligence graphify in vibe.yaml.\n" +
	OutputDir + "/\n"

// Skill is .claude/skills/graphify/SKILL.md. It is a constant rather than
// an embedded template, so a CRLF working copy cannot change the bytes a
// build ships.
const Skill = "---\n" +
	"name: graphify\n" +
	"description: Query the repository's Graphify knowledge graph (graphify-out/) for architecture and cross-file relationships, after checking that the graph is present and current. Use it before broad searches of an unfamiliar area.\n" +
	"---\n" +
	"\n" +
	"This repository keeps a Graphify knowledge graph in `graphify-out/`. It is\n" +
	"derived output, ignored by Git, and rebuilt by `task graph:update`, which\n" +
	"the post-commit and post-checkout Git hooks run.\n" +
	"\n" +
	"1. Check that it can be trusted before using it:\n" +
	"   - `graphify --version` works. If not, graphify is not installed: say so.\n" +
	"   - `graphify-out/graph.json` exists. If not, run `task graph:update`.\n" +
	"   - Its top-level `built_at_commit` equals `git rev-parse HEAD`. If not,\n" +
	"     the graph is stale: run `task graph:update`, or say that it is stale.\n" +
	"     Uncommitted changes are never in the graph.\n" +
	"2. Ask it focused questions: `graphify query \"<question>\"`,\n" +
	"   `graphify path \"<A>\" \"<B>\"`, `graphify explain \"<concept>\"`. Read\n" +
	"   `graphify-out/GRAPH_REPORT.md` only for a broad overview.\n" +
	"3. Fall back to search, the compiler, the language server and the tests\n" +
	"   whenever the graph is missing, stale, or does not answer. An empty\n" +
	"   graph result is not evidence that something does not exist.\n" +
	"4. The graph is static analysis. It never replaces or skips the\n" +
	"   repository's verification: run the checks AGENTS.md requires.\n"

type graphifyModule struct{}

// New returns the Graphify integration module.
func New() module.Module {
	return graphifyModule{}
}

func (graphifyModule) Name() string {
	return "graphify"
}

// Resolve returns the ignore section, at the bottom of .gitignore so the
// project's own entries keep their place, and the skill when Claude Code
// is selected. Codex reads AGENTS.md, whose workflow section names the
// graph, and gets nothing of its own (spec 0035 §2).
func (graphifyModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	rs := []resource.Resource{{
		Path:      IgnorePath,
		Ownership: resource.ManagedSection,
		SectionID: SectionID,
		Markers:   resource.HashComment,
		Placement: resource.Bottom,
		Content:   []byte(ignoreContent),
	}}
	if module.Selected(mctx, module.AgentHookIntegration) {
		rs = append(rs, resource.Resource{
			Path:      SkillPath,
			Ownership: resource.Generated,
			Content:   []byte(Skill),
		})
	}
	return rs, nil
}

// RequiredTools declares graphify as optional: without it the Git hooks
// skip the graph update, and nothing the repository requires stops.
func (graphifyModule) RequiredTools(*module.Context) []module.Tool {
	return []module.Tool{{
		Name:     "graphify",
		Why:      "task graph:update, run by the post-commit and post-checkout hooks",
		Version:  []string{"--version"},
		Optional: true,
		Install:  "uv tool install graphifyy, or pip install graphifyy",
	}}
}
