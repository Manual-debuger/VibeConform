package workflow

import (
	"context"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// On is the one value development.docs_development and
// development.docs_operations accept (spec 0033).
const On = "on"

// Layout is the docs layout development: selects: where each kind of
// document lives, as slash-separated paths from the repository root.
// Development and Operations are empty unless selected (spec 0033).
type Layout struct {
	Specs, Architecture, Decisions string
	Development, Operations        string
}

// DefaultLayout is spec 0030 §4's layout, with neither optional directory.
func DefaultLayout() Layout {
	return Layout{Specs: "docs/specs", Architecture: "docs/architecture", Decisions: "docs/decisions"}
}

// layoutOf returns the layout mctx selects, or nil when
// development.docs_layout is not selected.
func layoutOf(mctx *module.Context) *Layout {
	if mctx == nil || mctx.Policies[manifest.DevelopmentDocsLayout] == "" {
		return nil
	}
	l := DefaultLayout()
	if mctx.Policies[manifest.DevelopmentDocsDevelopment] == On {
		l.Development = "docs/development"
	}
	if mctx.Policies[manifest.DevelopmentDocsOperations] == On {
		l.Operations = "docs/operations"
	}
	return &l
}

// knowledge is the AGENTS.md section's knowledge rule for l.
func (l Layout) knowledge() string {
	s := "Knowledge:\n" +
		"- Specs (what must be true) live in `" + l.Specs + "/`, architecture (how it\n" +
		"  works now) in `" + l.Architecture + "/`, decisions (ADRs) in\n" +
		"  `" + l.Decisions + "/`. Read the relevant ones before a non-trivial change.\n"
	switch {
	case l.Development != "" && l.Operations != "":
		s += "- Development guides (build, test, contribute) live in\n" +
			"  `" + l.Development + "/`, operations docs (deploy, run, incidents) in\n" +
			"  `" + l.Operations + "/`.\n"
	case l.Development != "":
		s += "- Development guides (build, test, contribute) live in\n" +
			"  `" + l.Development + "/`.\n"
	case l.Operations != "":
		s += "- Operations docs (deploy, run, incidents) live in\n" +
			"  `" + l.Operations + "/`.\n"
	}
	return s + conflicts
}

// index is the docs section of docs/README.md for l: one entry per
// directory, linked relative to docs/.
func (l Layout) index() string {
	var b strings.Builder
	b.WriteString("## Layout\n\n")
	entry := func(dir, text string) {
		b.WriteString("- [`" + docsRel(dir) + "/`](" + docsRel(dir) + "/): " + text)
	}
	entry(l.Specs, "what must be true. Problem, constraints, desired\n"+
		"  behavior and acceptance criteria, one file per feature.\n")
	entry(l.Architecture, "how the system works now.\n")
	entry(l.Decisions, "decision records (ADRs), why a significant\n"+
		"  choice was made and what it costs.\n")
	if l.Development != "" {
		entry(l.Development, "building, testing and contributing\n"+
			"  locally.\n")
	}
	if l.Operations != "" {
		entry(l.Operations, "deploying, running and handling\n"+
			"  incidents.\n")
	}
	return b.String()
}

// docsRel is dir relative to docs/, where the docs index lives.
func docsRel(dir string) string {
	if rest, ok := strings.CutPrefix(dir, "docs/"); ok {
		return rest
	}
	return "../" + dir
}

type docsDir struct {
	name string
}

// NewDocsDir returns the module of an optional docs directory. It
// resolves nothing: selecting it changes the docs-layout and
// development-workflow sections, which read the selection.
func NewDocsDir(name string) module.Module {
	return docsDir{name: name}
}

func (d docsDir) Name() string {
	return d.name
}

func (docsDir) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return nil, nil
}
