// Package workflow provides the opt-in development settings of spec 0030:
// development.workflow, which owns one routing section of AGENTS.md, and
// development.docs_layout. Every line of the AGENTS.md section is derived
// from what vibe.yaml selects, and the rest of the file stays the
// project's. See docs/specs/0030-plan-triggered-sdd.md and
// docs/decisions/0015-agents-md-workflow-section.md.
package workflow

import (
	"context"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// The values development.workflow accepts.
const (
	Direct        = "direct"
	PlanTriggered = "plan-triggered-sdd"
	AlwaysSDD     = "always-sdd"
)

const (
	// AgentsPath is the file the workflow section lives in.
	AgentsPath = "AGENTS.md"
	// SectionID names the section within it.
	SectionID = "workflow"
	// MaxWords bounds the section for every selection (spec 0030 §3).
	MaxWords = 300
)

// The section is composed from constants rather than embedded templates,
// so a CRLF working copy of a template file cannot change the bytes a
// build ships. {planningRule}, {directRule} and {specdir} are filled in
// by Content.
const (
	header = "## Repository workflow\n" +
		"\n" +
		"Managed by VibeConform from `development:` in `vibe.yaml`. This section\n" +
		"routes; the documents and tasks it names hold the detail.\n" +
		"\n"

	knowledgeNoLayout = "Knowledge:\n" +
		"- Read the specs, architecture docs and ADRs that apply before a\n" +
		"  non-trivial change.\n" +
		conflicts

	conflicts = "- If an approved spec, an ADR and the code disagree, say so. Do not\n" +
		"  pick one silently.\n" +
		"\n"

	direct = "Workflow: direct.\n" +
		"- Implement, then verify. Respect any spec that applies.\n" +
		"{directRule}" +
		"\n"

	planTriggered = "Workflow: plan-triggered lightweight SDD.\n" +
		"- Normal mode: implement, then verify. Respect any spec that applies.\n" +
		sddRules

	alwaysSDD = "Workflow: spec-driven.\n" +
		"- A non-trivial behavioural change needs an approved spec with\n" +
		"  acceptance criteria before it is planned, in any mode. A small fix\n" +
		"  may go straight to implement and verify.\n" +
		sddRules

	sddRules = "{planningRule}" +
		"- The spec says WHAT must be true; the plan says HOW to change the\n" +
		"  repository. Keep them apart. Reuse an approved spec when one exists.\n" +
		"- In a read-only plan mode, put the spec in the plan. Once it is\n" +
		"  approved, {specdir}.\n" +
		"- Do not implement until the user approves.\n" +
		"\n"

	verification = "Verification:\n" +
		"- `task verify:fast` while working; `task verify` before declaring\n" +
		"  done. Do not weaken a test, lint or type check to make a change pass.\n" +
		"- Finish with a ledger, one line per check: PASS, FAIL or UNVERIFIED.\n" +
		"  Unit tests, CI and a real integration are separate lines.\n"
)

// The phrasings Content fills in, by whether the spec skill (/spec)
// exists and whether the docs layout names a spec directory. With the
// skill, the planning rules name it (spec 0031 §2).
const (
	planningSpec = "- Planning context (the harness's plan mode, or `/spec`): use the\n" +
		"  `spec` skill. List the constraints that apply and the assumptions\n" +
		"  you have not verified, then write a lightweight spec with acceptance\n" +
		"  criteria. Plan only after that.\n"
	planningNoSpec = "- Planning context (the harness's plan mode, or a request for a spec): list the\n" +
		"  constraints that apply and the assumptions you have not verified,\n" +
		"  then write a lightweight spec with acceptance criteria. Plan only\n" +
		"  after that.\n"
	directSpec = "- A planning context (the harness's plan mode, or `/spec`) writes a\n" +
		"  lightweight spec with acceptance criteria when asked, with the `spec`\n" +
		"  skill. The spec says WHAT must be true; the plan says HOW to change\n" +
		"  the repository.\n"
	directNoSpec = "- A planning context (the harness's plan mode, or a request for a spec) writes a\n" +
		"  lightweight spec with acceptance criteria when asked. The spec says\n" +
		"  WHAT must be true; the plan says HOW to change the repository.\n"
	specdirNone = "write it where this project keeps specs first"
)

// Content returns the AGENTS.md section for mode: with spec when the
// harness offers /spec (the claude integration), and with docsLayout when
// development.docs_layout is selected, in its default layout.
func Content(mode string, spec, docsLayout bool) string {
	if docsLayout {
		l := DefaultLayout()
		return LayoutContent(mode, spec, &l)
	}
	return LayoutContent(mode, spec, nil)
}

// LayoutContent is Content for any layout; nil means no docs layout.
func LayoutContent(mode string, spec bool, layout *Layout) string {
	var b strings.Builder
	b.WriteString(header)
	if layout != nil {
		b.WriteString(layout.knowledge())
	} else {
		b.WriteString(knowledgeNoLayout)
	}
	switch mode {
	case Direct:
		b.WriteString(direct)
	case PlanTriggered:
		b.WriteString(planTriggered)
	case AlwaysSDD:
		b.WriteString(alwaysSDD)
	}
	b.WriteString(verification)

	planning, directRule, specdir := planningNoSpec, directNoSpec, specdirNone
	if spec {
		planning, directRule = planningSpec, directSpec
	}
	if layout != nil {
		specdir = "write it to `" + layout.Specs + "/` first"
	}
	return strings.NewReplacer("{planningRule}", planning, "{directRule}", directRule, "{specdir}", specdir).Replace(b.String())
}

type workflowModule struct {
	mode string
}

// New returns the development-workflow module for one value of
// development.workflow.
func New(mode string) module.Module {
	return workflowModule{mode: mode}
}

func (workflowModule) Name() string {
	return "development-workflow"
}

// Resolve returns the workflow section of AGENTS.md, placed at the bottom
// so the project's own title and introduction come first, then each
// component's section of its own AGENTS.md (spec 0032).
func (w workflowModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	rs := []resource.Resource{{
		Path:      AgentsPath,
		Ownership: resource.ManagedSection,
		SectionID: SectionID,
		Markers:   resource.HTMLComment,
		Placement: resource.Bottom,
		Content:   []byte(LayoutContent(w.mode, hasSpecCommand(mctx), layoutOf(mctx))),
	}}
	for _, c := range module.ComponentsOf(mctx) {
		rs = append(rs, resource.Resource{
			Path:      c.Path + "/" + AgentsPath,
			Ownership: resource.ManagedSection,
			SectionID: ComponentSectionID,
			Markers:   resource.HTMLComment,
			Placement: resource.Bottom,
			Content:   []byte(ComponentContent(c)),
		})
	}
	return rs, nil
}

const (
	// ComponentSectionID names the section of a component's AGENTS.md.
	ComponentSectionID = "component"
	// MaxComponentWords bounds that section (spec 0032 §1).
	MaxComponentWords = 80
)

// ComponentContent returns the section of c's AGENTS.md: which component
// this is, a link to the root AGENTS.md that governs it, and the tasks
// that verify it alone. It restates no policy.
func ComponentContent(c manifest.Component) string {
	root := strings.Repeat("../", strings.Count(c.Path, "/")+1) + AgentsPath
	return "## Component `" + c.ID + "`\n" +
		"\n" +
		"Managed by VibeConform from `components:` in `vibe.yaml`. This is the\n" +
		"`" + c.ID + "` component (profile `" + string(c.Profile) + "`). The root\n" +
		"[`AGENTS.md`](" + root + ") holds the workflow and the rules, and they\n" +
		"apply here unchanged.\n" +
		"\n" +
		"Verification: `task verify:fast` while working and `task verify` before\n" +
		"declaring done, from this directory; `task " + c.ID + ":verify` from the root.\n"
}

// hasSpecCommand reports whether the harness offers /spec: claude-config
// generates it whenever claude is selected with a workflow. An unknown
// selection counts as selected, as it does for the agent hooks.
func hasSpecCommand(mctx *module.Context) bool {
	return module.WantsAgentHooks(mctx)
}
