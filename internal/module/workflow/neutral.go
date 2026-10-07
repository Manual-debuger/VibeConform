package workflow

import (
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// The AGENTS.md sections of spec 0043: the routing that the workflow
// section carries, for a repository that selects no workflow. Each names
// documents or tools and no process, and neither exists with a workflow,
// whose section already says the same.
const (
	// KnowledgeSectionID names the docs layout's section of AGENTS.md.
	KnowledgeSectionID = "knowledge"
	// IntelligenceSectionID names Graphify's section of AGENTS.md.
	IntelligenceSectionID = "intelligence"
	// MaxNeutralWords bounds each of them (spec 0043 §5).
	MaxNeutralWords = 120
)

// KnowledgeContent returns the knowledge section for l.
func KnowledgeContent(l Layout) string {
	return MarkdownSection("## Repository documents\n" +
		"\n" +
		"Managed by VibeConform from `development.docs_layout` in `vibe.yaml`.\n" +
		"\n" +
		strings.TrimSuffix(l.knowledgeRules(), "\n"))
}

// IntelligenceContent returns the intelligence section.
func IntelligenceContent() string {
	return MarkdownSection("## Repository intelligence\n" +
		"\n" +
		"Managed by VibeConform from `integrations.intelligence` in `vibe.yaml`.\n" +
		"\n" +
		strings.TrimSuffix(graphifyRule, "\n"))
}

// Selected reports whether vibe.yaml selects a development
// workflow. An isolated module, with no context, has none.
func Selected(mctx *module.Context) bool {
	return mctx != nil && mctx.Policies[manifest.DevelopmentWorkflow] != ""
}

// AgentsSection returns a section of AGENTS.md at its bottom, below the
// project's own text.
func AgentsSection(id, content string) resource.Resource {
	return resource.Resource{
		Path:      AgentsPath,
		Ownership: resource.ManagedSection,
		SectionID: id,
		Markers:   resource.HTMLComment,
		Placement: resource.Bottom,
		Content:   []byte(content),
	}
}
