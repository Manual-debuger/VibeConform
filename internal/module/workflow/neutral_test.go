package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// The knowledge section of spec 0043 §1, byte for byte: the default
// layout, both optional directories, and an adopted layout.
func TestKnowledgeContent(t *testing.T) {
	head := "\n## Repository documents\n" +
		"\n" +
		"Managed by VibeConform from `development.docs_layout` in `vibe.yaml`.\n" +
		"\n"
	tail := "- If an approved spec, an ADR and the code disagree, say so. Do not\n" +
		"  pick one silently.\n" +
		"\n"
	both := DefaultLayout()
	both.Development, both.Operations = "docs/development", "docs/operations"
	adopted := DefaultLayout()
	adopted.Specs, adopted.Decisions = "rfcs", "docs/adr"

	for name, tc := range map[string]struct {
		layout Layout
		want   string
	}{
		"default": {DefaultLayout(), head +
			"- Specs (what must be true) live in `docs/specs/`, architecture (how it\n" +
			"  works now) in `docs/architecture/`, decisions (ADRs) in\n" +
			"  `docs/decisions/`. Read the relevant ones before a non-trivial change.\n" +
			tail},
		"optional directories": {both, head +
			"- Specs (what must be true) live in `docs/specs/`, architecture (how it\n" +
			"  works now) in `docs/architecture/`, decisions (ADRs) in\n" +
			"  `docs/decisions/`. Read the relevant ones before a non-trivial change.\n" +
			"- Development guides (build, test, contribute) live in\n" +
			"  `docs/development/`, operations docs (deploy, run, incidents) in\n" +
			"  `docs/operations/`.\n" +
			tail},
		"adopted": {adopted, head +
			"- Specs (what must be true) live in `rfcs/`, architecture (how it\n" +
			"  works now) in `docs/architecture/`, decisions (ADRs) in\n" +
			"  `docs/adr/`. Read the relevant ones before a non-trivial change.\n" +
			tail},
	} {
		got := KnowledgeContent(tc.layout)
		if got != tc.want {
			t.Errorf("%s:\n%s", name, got)
		}
		if n := len(strings.Fields(got)); n > MaxNeutralWords {
			t.Errorf("%s: %d words, cap %d", name, n, MaxNeutralWords)
		}
	}
	if n := len(strings.Fields(IntelligenceContent())); n > MaxNeutralWords {
		t.Errorf("intelligence: %d words, cap %d", n, MaxNeutralWords)
	}
}

// TestNeutralSectionsNameNoProcess: neither section carries workflow text
// (spec 0042 §5).
func TestNeutralSectionsNameNoProcess(t *testing.T) {
	both := DefaultLayout()
	both.Development, both.Operations = "docs/development", "docs/operations"
	for _, s := range []string{KnowledgeContent(both), IntelligenceContent()} {
		for _, word := range []string{"Workflow", "plan mode", "Planning", "`/spec`", "Do not implement", "the user approves"} {
			if strings.Contains(s, word) {
				t.Errorf("section mentions %q:\n%s", word, s)
			}
		}
	}
}

// TestKnowledgeNeedsNoWorkflow: with a workflow, whose section names the
// layout, the docs layout resolves no AGENTS.md section, and it declares
// the section conditional so that sync removes a recorded copy (spec 0043
// §3, §4).
func TestKnowledgeNeedsNoWorkflow(t *testing.T) {
	for mode, want := range map[string]bool{"": true, Direct: false, PlanTriggered: false, AlwaysSDD: false} {
		mctx := &module.Context{Policies: map[string]string{"docs_layout": DocsLayoutStandard}}
		if mode != "" {
			mctx.Policies["workflow"] = mode
		}
		rs, err := NewDocsLayout().Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatal(err)
		}
		got := false
		for _, r := range rs {
			got = got || r.Path == AgentsPath
		}
		if got != want {
			t.Errorf("workflow %q: AGENTS.md section = %v, want %v", mode, got, want)
		}
	}
	cs := NewDocsLayout().(module.ConditionalSectioner).ConditionalSections()
	if len(cs) != 1 || cs[0].SectionID != KnowledgeSectionID || cs[0].Markers != resource.HTMLComment || cs[0].Placement != resource.Bottom {
		t.Errorf("ConditionalSections() = %+v", cs)
	}
}
