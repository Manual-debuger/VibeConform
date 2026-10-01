package workflow

import (
	"context"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// DocsLayoutStandard is the one value development.docs_layout accepts.
const DocsLayoutStandard = "standard"

const (
	// DocsIndexPath is the docs index; its docs section lists the layout.
	DocsIndexPath = "docs/README.md"
	// SpecsIndexPath is the specs directory's README; its specs section
	// says how a spec is named and written.
	SpecsIndexPath = "docs/specs/README.md"
)

// SpecTemplate is the lightweight spec template of spec 0030 §5. Both
// docs/specs/README.md and Claude Code's /spec command carry it, so the
// two can never disagree.
const SpecTemplate = "# Feature: <name>\n" +
	"\n" +
	"Status: draft\n" +
	"\n" +
	"## Problem\n" +
	"\n" +
	"What is wrong or missing, and for whom.\n" +
	"\n" +
	"## Constraints\n" +
	"\n" +
	"Architecture, compatibility and product rules this must not break.\n" +
	"\n" +
	"## Assumptions\n" +
	"\n" +
	"What this takes as true without evidence from the repository or the user.\n" +
	"\n" +
	"## Desired Behavior\n" +
	"\n" +
	"What must be true when this is done, observable from outside.\n" +
	"\n" +
	"## Non-goals\n" +
	"\n" +
	"What this deliberately leaves out.\n" +
	"\n" +
	"## Acceptance Criteria\n" +
	"\n" +
	"Checkable statements; each is verified before the work is called done.\n" +
	"\n" +
	"- [ ] ...\n"

// specsIndex is the specs section of the README in specs, the layout's
// spec directory.
func specsIndex(specs string) string {
	return "# Specs\n" +
		"\n" +
		"A spec says what must be true; it does not say how to change the code.\n" +
		"Write one file per feature, `" + specs + "/<feature>.md` (a numeric prefix\n" +
		"is fine), starting with a Status line: draft, accepted, implemented or\n" +
		"superseded. A spec is a living document: update it when the behavior it\n" +
		"describes changes.\n" +
		"\n" +
		"Template:\n" +
		"\n" +
		"```markdown\n" +
		SpecTemplate +
		"```\n"
}

type docsLayout struct{}

// NewDocsLayout returns the docs-layout module.
func NewDocsLayout() module.Module {
	return docsLayout{}
}

func (docsLayout) Name() string {
	return "docs-layout"
}

// MovableSections: the specs section follows development.specs_dir, so a
// recorded copy elsewhere leaves when the directory moves (spec 0034 §3).
func (docsLayout) MovableSections() []string {
	return []string{"specs"}
}

// Resolve returns the two sections that make the layout's directories
// canonical. The docs index goes below the project's own introduction;
// the specs README is VibeConform's from the top, and the project may add
// its own conventions below it. architecture/ and decisions/ get no file:
// they appear with the project's first document there.
func (docsLayout) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	l := DefaultLayout()
	if selected := layoutOf(mctx); selected != nil {
		l = *selected
	}
	return []resource.Resource{
		{
			Path:      DocsIndexPath,
			Ownership: resource.ManagedSection,
			SectionID: "docs",
			Markers:   resource.HTMLComment,
			Placement: resource.Bottom,
			Content:   []byte(l.index()),
		},
		{
			Path:      l.Specs + "/README.md",
			Ownership: resource.ManagedSection,
			SectionID: "specs",
			Markers:   resource.HTMLComment,
			Placement: resource.Top,
			Content:   []byte(specsIndex(l.Specs)),
		},
	}, nil
}
