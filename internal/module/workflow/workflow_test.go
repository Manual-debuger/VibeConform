package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// reference is spec 0030 §3's example, byte for byte: plan-triggered-sdd
// with docs_layout and claude selected.
const reference = "## Repository workflow\n" +
	"\n" +
	"Managed by VibeConform from `development:` in `vibe.yaml`. This section\n" +
	"routes; the documents and tasks it names hold the detail.\n" +
	"\n" +
	"Knowledge:\n" +
	"- Specs (what must be true) live in `docs/specs/`, architecture (how it\n" +
	"  works now) in `docs/architecture/`, decisions (ADRs) in\n" +
	"  `docs/decisions/`. Read the relevant ones before a non-trivial change.\n" +
	"- If an approved spec, an ADR and the code disagree, say so. Do not\n" +
	"  pick one silently.\n" +
	"\n" +
	"Workflow: plan-triggered lightweight SDD.\n" +
	"- Normal mode: implement, then verify. Respect any spec that applies.\n" +
	"- Planning context (the harness's plan mode, or `/spec`): list the\n" +
	"  constraints that apply and the assumptions you have not verified,\n" +
	"  then write a lightweight spec with acceptance criteria. Plan only\n" +
	"  after that.\n" +
	"- The spec says WHAT must be true; the plan says HOW to change the\n" +
	"  repository. Keep them apart. Reuse an approved spec when one exists.\n" +
	"- In a read-only plan mode, put the spec in the plan. Once it is\n" +
	"  approved, write it to `docs/specs/` first.\n" +
	"- Do not implement until the user approves.\n" +
	"\n" +
	"Verification:\n" +
	"- `task verify:fast` while working; `task verify` before declaring\n" +
	"  done. Do not weaken a test, lint or type check to make a change pass.\n" +
	"- Finish with a ledger, one line per check: PASS, FAIL or UNVERIFIED.\n" +
	"  Unit tests, CI and a real integration are separate lines.\n"

func TestReferenceSection(t *testing.T) {
	if got := Content(PlanTriggered, true, true); got != reference {
		t.Errorf("section differs from spec 0030 §3:\n%s", got)
	}
}

// TestVariants pins what each selection changes (spec 0030 §3's table),
// as literal text in the section.
func TestVariants(t *testing.T) {
	noLayout := "Knowledge:\n" +
		"- Read the specs, architecture docs and ADRs that apply before a\n" +
		"  non-trivial change.\n" +
		"- If an approved spec"
	directBlock := "Workflow: direct.\n" +
		"- Implement, then verify. Respect any spec that applies.\n" +
		"- A planning context (the harness's plan mode, or `/spec`) writes a\n" +
		"  lightweight spec with acceptance criteria when asked. The spec says\n" +
		"  WHAT must be true; the plan says HOW to change the repository.\n" +
		"\nVerification:\n"
	alwaysBlock := "Workflow: spec-driven.\n" +
		"- A non-trivial behavioural change needs an approved spec with\n" +
		"  acceptance criteria before it is planned, in any mode. A small fix\n" +
		"  may go straight to implement and verify.\n" +
		"- Planning context (the harness's plan mode, or `/spec`): list the\n"

	for name, tc := range map[string]struct {
		got        string
		want, gone []string
	}{
		"no docs layout": {
			got:  Content(PlanTriggered, true, false),
			want: []string{noLayout, "  approved, write it where this project keeps specs first.\n"},
			gone: []string{"docs/specs/", "docs/architecture/", "docs/decisions/"},
		},
		"no claude": {
			got:  Content(PlanTriggered, false, true),
			want: []string{"- Planning context (the harness's plan mode, or a request for a spec): list the\n"},
			gone: []string{"`/spec`"},
		},
		"direct": {
			got:  Content(Direct, true, true),
			want: []string{directBlock},
			gone: []string{"Normal mode", "Do not implement until", "read-only plan mode"},
		},
		"always-sdd": {
			got:  Content(AlwaysSDD, true, true),
			want: []string{alwaysBlock, "- Do not implement until the user approves.\n"},
			gone: []string{"Normal mode"},
		},
	} {
		for _, w := range tc.want {
			if !strings.Contains(tc.got, w) {
				t.Errorf("%s: missing %q in\n%s", name, w, tc.got)
			}
		}
		for _, g := range tc.gone {
			if strings.Contains(tc.got, g) {
				t.Errorf("%s: still has %q", name, g)
			}
		}
	}
}

// TestSectionStaysSmall is spec 0030 §3's invariant: at most 300 words
// for every selection. It logs each size against the 150–250 word,
// 20–35 line design target, which it does not enforce.
func TestSectionStaysSmall(t *testing.T) {
	for _, mode := range []string{Direct, PlanTriggered, AlwaysSDD} {
		for _, spec := range []bool{false, true} {
			for _, docs := range []bool{false, true} {
				c := Content(mode, spec, docs)
				words, lines := len(strings.Fields(c)), strings.Count(c, "\n")
				t.Logf("%s spec=%v docs=%v: %d lines, %d words", mode, spec, docs, lines, words)
				if words > MaxWords {
					t.Errorf("%s spec=%v docs=%v: %d words, more than %d", mode, spec, docs, words, MaxWords)
				}
				if strings.Contains(c, "{") || !strings.HasSuffix(c, "\n") {
					t.Errorf("%s spec=%v docs=%v: unfilled placeholder or no final newline", mode, spec, docs)
				}
			}
		}
	}
}

func TestResolve(t *testing.T) {
	selected := &module.Context{Integrations: []string{}, Policies: map[string]string{"workflow": AlwaysSDD}}
	for name, tc := range map[string]struct {
		mctx *module.Context
		want string
	}{
		"isolated":  {nil, Content(AlwaysSDD, true, false)},
		"no claude": {selected, Content(AlwaysSDD, false, false)},
		"claude and docs": {&module.Context{
			Integrations: []string{"claude"},
			Policies:     map[string]string{"workflow": AlwaysSDD, "docs_layout": "standard"},
		}, Content(AlwaysSDD, true, true)},
	} {
		rs, err := New(AlwaysSDD).Resolve(context.Background(), tc.mctx)
		if err != nil {
			t.Fatal(err)
		}
		want := resource.Resource{
			Path: "AGENTS.md", Ownership: resource.ManagedSection, SectionID: "workflow",
			Markers: resource.HTMLComment, Placement: resource.Bottom,
		}
		if len(rs) != 1 {
			t.Fatalf("%s: %d resources", name, len(rs))
		}
		r := rs[0]
		if string(r.Content) != tc.want {
			t.Errorf("%s: content\n%s", name, r.Content)
		}
		r.Content = nil
		if r.Path != want.Path || r.Ownership != want.Ownership || r.SectionID != want.SectionID ||
			r.Markers != want.Markers || r.Placement != want.Placement {
			t.Errorf("%s: resource %+v", name, r)
		}
	}
}
