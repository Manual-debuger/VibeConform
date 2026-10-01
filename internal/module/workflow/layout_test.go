package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

// docsPolicies selects the docs layout and, by name, the optional
// directories of spec 0033.
func docsPolicies(optional ...string) map[string]string {
	p := map[string]string{"workflow": AlwaysSDD, "docs_layout": DocsLayoutStandard}
	for _, k := range optional {
		p[k] = On
	}
	return p
}

// TestOptionalDocsDirs pins what each selection of spec 0033 adds to the
// docs index and to the AGENTS.md knowledge rule.
func TestOptionalDocsDirs(t *testing.T) {
	devEntry := "- [`development/`](development/): building, testing and contributing\n  locally.\n"
	opsEntry := "- [`operations/`](operations/): deploying, running and handling\n  incidents.\n"
	for name, tc := range map[string]struct {
		optional       []string
		index, agents  []string
		noIndex, noAgt []string
	}{
		"neither": {
			noIndex: []string{"development/", "operations/"},
			noAgt:   []string{"docs/development/", "docs/operations/"},
		},
		"development": {
			optional: []string{"docs_development"},
			index:    []string{devEntry},
			agents:   []string{"- Development guides (build, test, contribute) live in\n  `docs/development/`.\n"},
			noIndex:  []string{"operations/"},
			noAgt:    []string{"docs/operations/"},
		},
		"operations": {
			optional: []string{"docs_operations"},
			index:    []string{opsEntry},
			agents:   []string{"- Operations docs (deploy, run, incidents) live in\n  `docs/operations/`.\n"},
			noIndex:  []string{"development/"},
			noAgt:    []string{"docs/development/"},
		},
		"both": {
			optional: []string{"docs_development", "docs_operations"},
			index:    []string{devEntry + opsEntry},
			agents: []string{"- Development guides (build, test, contribute) live in\n" +
				"  `docs/development/`, operations docs (deploy, run, incidents) in\n" +
				"  `docs/operations/`.\n- If an approved spec"},
		},
	} {
		mctx := &module.Context{Integrations: []string{"claude"}, Policies: docsPolicies(tc.optional...)}
		docs, err := NewDocsLayout().Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatal(err)
		}
		agents, err := New(AlwaysSDD).Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatal(err)
		}
		index, section := string(docs[0].Content), string(agents[0].Content)
		check := func(what, got string, want, gone []string) {
			for _, w := range want {
				if !strings.Contains(got, w) {
					t.Errorf("%s: %s lacks %q:\n%s", name, what, w, got)
				}
			}
			for _, g := range gone {
				if strings.Contains(got, g) {
					t.Errorf("%s: %s has %q", name, what, g)
				}
			}
		}
		check("docs index", index, tc.index, tc.noIndex)
		check("AGENTS.md section", section, tc.agents, tc.noAgt)
		if words := len(strings.Fields(section)); words > MaxWords {
			t.Errorf("%s: %d words, more than %d", name, words, MaxWords)
		}
	}
}

// TestAdoptedLayout pins spec 0034 §2: adopted paths in the AGENTS.md
// knowledge rule and spec-directory phrase, in the docs index (a path
// outside docs/ linked with ../), and as the specs README's directory.
func TestAdoptedLayout(t *testing.T) {
	mctx := &module.Context{
		Integrations: []string{"claude"},
		Policies:     docsPolicies("docs_operations"),
		DocsDirs: manifest.DocsDirs{Specs: "rfcs", Architecture: "docs/architecture", Decisions: "docs/adr",
			Operations: "runbooks"},
	}
	docs, err := NewDocsLayout().Resolve(context.Background(), mctx)
	if err != nil {
		t.Fatal(err)
	}
	wantIndex := "## Layout\n" +
		"\n" +
		"- [`../rfcs/`](../rfcs/): what must be true. Problem, constraints, desired\n" +
		"  behavior and acceptance criteria, one file per feature.\n" +
		"- [`architecture/`](architecture/): how the system works now.\n" +
		"- [`adr/`](adr/): decision records (ADRs), why a significant\n" +
		"  choice was made and what it costs.\n" +
		"- [`../runbooks/`](../runbooks/): deploying, running and handling\n" +
		"  incidents.\n"
	if got := string(docs[0].Content); got != wantIndex {
		t.Errorf("docs index:\n%s", got)
	}
	if docs[1].Path != "rfcs/README.md" || !strings.Contains(string(docs[1].Content), "`rfcs/<feature>.md`") {
		t.Errorf("specs README %s:\n%s", docs[1].Path, docs[1].Content)
	}

	agents, err := New(AlwaysSDD).Resolve(context.Background(), mctx)
	if err != nil {
		t.Fatal(err)
	}
	section := string(agents[0].Content)
	for _, want := range []string{
		"- Specs (what must be true) live in `rfcs/`, architecture (how it\n" +
			"  works now) in `docs/architecture/`, decisions (ADRs) in\n" +
			"  `docs/adr/`. Read the relevant ones before a non-trivial change.\n" +
			"- Operations docs (deploy, run, incidents) live in\n  `runbooks/`.\n",
		"  approved, write it to `rfcs/` first.\n",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("AGENTS.md section lacks %q:\n%s", want, section)
		}
	}
	if strings.Contains(section, "docs/specs/") || strings.Contains(section, "docs/decisions/") {
		t.Errorf("AGENTS.md section still names a default it replaced:\n%s", section)
	}
}

// TestDocsDirResolvesNothing: the optional directories' own modules write
// nothing; the layout's and the workflow's sections carry them.
func TestDocsDirResolvesNothing(t *testing.T) {
	rs, err := NewDocsDir("docs-operations").Resolve(context.Background(), nil)
	if err != nil || len(rs) != 0 {
		t.Errorf("resources %v, err %v", rs, err)
	}
}
