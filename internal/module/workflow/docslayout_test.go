package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// TestDocsLayoutResources pins both sections byte for byte, and where
// each one goes in its file.
func TestDocsLayoutResources(t *testing.T) {
	rs, err := NewDocsLayout().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// The two docs sections, and, with no workflow, AGENTS.md's knowledge
	// section (spec 0043 §1).
	if len(rs) != 3 {
		t.Fatalf("%d resources, want 3", len(rs))
	}
	if k := rs[2]; k.Path != "AGENTS.md" || k.SectionID != "knowledge" || k.Markers != resource.HTMLComment ||
		k.Placement != resource.Bottom || string(k.Content) != KnowledgeContent(DefaultLayout()) {
		t.Errorf("knowledge resource %+v", k)
	}

	index := rs[0]
	if index.Path != "docs/README.md" || index.SectionID != "docs" || index.Ownership != resource.ManagedSection ||
		index.Markers != resource.HTMLComment || index.Placement != resource.Bottom {
		t.Errorf("index resource %+v", index)
	}
	wantIndex := "\n## Layout\n" +
		"\n" +
		"- [`specs/`](specs/): what must be true. Problem, constraints, desired\n" +
		"  behavior and acceptance criteria, one file per feature.\n" +
		"- [`architecture/`](architecture/): how the system works now.\n" +
		"- [`decisions/`](decisions/): decision records (ADRs), why a significant\n" +
		"  choice was made and what it costs.\n" +
		"\n"
	if string(index.Content) != wantIndex {
		t.Errorf("docs/README.md section:\n%s", index.Content)
	}

	specs := rs[1]
	if specs.Path != "docs/specs/README.md" || specs.SectionID != "specs" || specs.Ownership != resource.ManagedSection ||
		specs.Markers != resource.HTMLComment || specs.Placement != resource.Top {
		t.Errorf("specs resource %+v", specs)
	}
	wantSpecs := "\n# Specs\n" +
		"\n" +
		"A spec says what must be true; it does not say how to change the code.\n" +
		"Write one file per feature, `docs/specs/<feature>.md` (a numeric prefix\n" +
		"is fine), starting with a Status line: draft, accepted, implemented or\n" +
		"superseded. A spec is a living document: update it when the behavior it\n" +
		"describes changes.\n" +
		"\n" +
		"Template:\n" +
		"\n" +
		"```markdown\n" +
		wantTemplate +
		"```\n" +
		"\n"
	if string(specs.Content) != wantSpecs {
		t.Errorf("docs/specs/README.md section:\n%s", specs.Content)
	}
}

// wantTemplate is spec 0030 §5's template with plan 0030's prompts.
const wantTemplate = "# Feature: <name>\n" +
	"\n" +
	"Status: draft\n" +
	"\n" +
	"## Problem\n\nWhat is wrong or missing, and for whom.\n\n" +
	"## Constraints\n\nArchitecture, compatibility and product rules this must not break.\n\n" +
	"## Assumptions\n\nWhat this takes as true without evidence from the repository or the user.\n\n" +
	"## Desired Behavior\n\nWhat must be true when this is done, observable from outside.\n\n" +
	"## Non-goals\n\nWhat this deliberately leaves out.\n\n" +
	"## Acceptance Criteria\n\nCheckable statements; each is verified before the work is called done.\n\n" +
	"- [ ] ...\n"

func TestSpecTemplate(t *testing.T) {
	if SpecTemplate != wantTemplate {
		t.Errorf("SpecTemplate:\n%s", SpecTemplate)
	}
	// The template sits inside a fenced block in docs/specs/README.md.
	if strings.Contains(SpecTemplate, "```") {
		t.Error("SpecTemplate contains a code fence")
	}
}
