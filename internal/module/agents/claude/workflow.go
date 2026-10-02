package claude

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/workflow"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// The Claude Code adapter of spec 0030 §5, as revised by spec 0031: the
// spec skill, which is also /spec, and the import that makes Claude Code
// read AGENTS.md at all.
const (
	// SpecSkillPath is the project skill spec, invocable as /spec.
	SpecSkillPath = ".claude/skills/spec/SKILL.md"
	// SpecCommandPath is the /spec command spec 0030 generated, retired by
	// spec 0031 in favour of the skill.
	SpecCommandPath = ".claude/commands/spec.md"
	// ClaudeMDPath is the file Claude Code loads, where AGENTS.md does not
	// reach it.
	ClaudeMDPath = "CLAUDE.md"
	// ImportSectionID names CLAUDE.md's section.
	ImportSectionID = "agents"
)

// importLine is the documented CLAUDE.md import of AGENTS.md, padded so
// the section is Prettier-stable (workflow.MarkdownSection).
var importLine = workflow.MarkdownSection("@AGENTS.md\n")

// The front matter of .claude/skills/spec/SKILL.md. Under the SDD
// workflows Claude may invoke the skill by itself, in plan mode or when
// asked to plan a change. Under direct a spec is written only when asked,
// so the skill is the user's alone (spec 0031 §1).
const (
	skillModelInvoked = "---\n" +
		"name: spec\n" +
		"description: Write a lightweight spec (what must be true, with acceptance criteria) for a non-trivial change, then propose a plan and stop before implementing. Use it in plan mode, and whenever asked to plan or spec a change.\n" +
		"argument-hint: <feature or issue>\n" +
		"---\n"
	skillUserInvoked = "---\n" +
		"name: spec\n" +
		"description: Write a lightweight spec (what must be true, with acceptance criteria) for a change, then propose a plan and stop before implementing.\n" +
		"argument-hint: <feature or issue>\n" +
		"disable-model-invocation: true\n" +
		"---\n"
)

// skillBody follows the front matter. It is a constant for the same CRLF
// reason as the workflow section.
const skillBody = "\n" +
	"This is a planning context for: $ARGUMENTS\n" +
	"(If nothing follows the colon, it is for the change under discussion.)\n" +
	"\n" +
	"Do not change code, configuration or tests while using this skill.\n" +
	"\n" +
	"1. Read the specs, architecture docs and ADRs that apply (AGENTS.md says\n" +
	"   where they live), and the code involved.\n" +
	"2. List the constraints that apply and the assumptions you have not\n" +
	"   verified. Ask the user about the ones that would change the result.\n" +
	"3. If an approved spec already covers this, reuse it. Otherwise write one\n" +
	"   from the template below, where AGENTS.md says specs live\n" +
	"   (`docs/specs/<feature>.md` if it names no place). In a read-only plan\n" +
	"   mode, put it in the plan instead, and write it once it is approved.\n" +
	"   Keep it to WHAT must be true: no file-level steps, function design or\n" +
	"   sequencing.\n" +
	"4. Propose the implementation plan, the HOW, separately from the spec.\n" +
	"   Do not commit it unless the project asks for that.\n" +
	"5. Stop. Implement only after the user approves the spec and the plan.\n" +
	"\n" +
	"Template:\n" +
	"\n" +
	"```markdown\n" +
	workflow.SpecTemplate +
	"```\n"

// SpecSkill returns .claude/skills/spec/SKILL.md for one value of
// development.workflow.
func SpecSkill(mode string) string {
	if mode == workflow.Direct {
		return skillUserInvoked + skillBody
	}
	return skillModelInvoked + skillBody
}

// workflowResources returns the adapter's resources when vibe.yaml selects
// a development workflow, and none otherwise. Every workflow gets /spec:
// under direct it is how one asks for a spec.
func workflowResources(mctx *module.Context) []resource.Resource {
	if mctx == nil || mctx.Policies[manifest.DevelopmentWorkflow] == "" {
		return nil
	}
	rs := []resource.Resource{
		{
			Path:      SpecSkillPath,
			Ownership: resource.Generated,
			Content:   []byte(SpecSkill(mctx.Policies[manifest.DevelopmentWorkflow])),
		},
		importSection(ClaudeMDPath),
	}
	// Each component's CLAUDE.md imports its own AGENTS.md, whose section
	// links to the root (spec 0032 §2).
	for _, c := range module.ComponentsOf(mctx) {
		rs = append(rs, importSection(c.Path+"/"+ClaudeMDPath))
	}
	return rs
}

// importSection is the agents section of the CLAUDE.md at p.
func importSection(p string) resource.Resource {
	return resource.Resource{
		Path:      p,
		Ownership: resource.ManagedSection,
		SectionID: ImportSectionID,
		Markers:   resource.HTMLComment,
		Placement: resource.Top,
		Content:   []byte(importLine),
	}
}

// Retired names the command spec 0031 replaced with the skill, so that
// sync removes a recorded copy left by an older vibe.
func (claudeModule) Retired() map[string]string {
	return map[string]string{SpecCommandPath: "replaced by " + SpecSkillPath}
}

// CheckSection warns about a second import of AGENTS.md outside
// CLAUDE.md's section: harmless, but the project's line is now redundant.
// It never reports a conflict, and checks no file but a CLAUDE.md, the
// root's or a component's.
func (claudeModule) CheckSection(r resource.Resource, before, after []byte) (conflicts, warnings []string) {
	if path.Base(r.Path) != ClaudeMDPath {
		return nil, nil
	}
	line := 0
	scan := func(data []byte) {
		if len(data) == 0 {
			return
		}
		for _, l := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			line++
			switch strings.TrimSpace(l) {
			case "@AGENTS.md", "@./AGENTS.md":
				warnings = append(warnings, fmt.Sprintf("line %d: %q imports AGENTS.md again; the section already does, so remove this line",
					line, strings.TrimSpace(l)))
			}
		}
	}
	scan(before)
	line += bytes.Count(r.Content, []byte("\n")) + 2
	scan(after)
	return nil, warnings
}
