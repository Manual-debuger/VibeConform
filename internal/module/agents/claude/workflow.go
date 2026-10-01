package claude

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/workflow"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// The Claude Code adapter of spec 0030 §5: a /spec command, and the
// import that makes Claude Code read AGENTS.md at all.
const (
	// SpecCommandPath is the project slash command /spec.
	SpecCommandPath = ".claude/commands/spec.md"
	// ClaudeMDPath is the file Claude Code loads, where AGENTS.md does not
	// reach it.
	ClaudeMDPath = "CLAUDE.md"
	// ImportSectionID names CLAUDE.md's section.
	ImportSectionID = "agents"
)

// importLine is the documented CLAUDE.md import of AGENTS.md.
const importLine = "@AGENTS.md\n"

// specCommand is .claude/commands/spec.md. It is a constant for the same
// CRLF reason as the workflow section.
const specCommand = "---\n" +
	"description: Write a lightweight spec for a change, then propose a plan; stop before implementing.\n" +
	"argument-hint: <feature or issue>\n" +
	"---\n" +
	"\n" +
	"This is a planning context for: $ARGUMENTS\n" +
	"\n" +
	"Do not change code, configuration or tests while running this command.\n" +
	"\n" +
	"1. Read the specs, architecture docs and ADRs that apply (AGENTS.md says\n" +
	"   where they live), and the code involved.\n" +
	"2. List the constraints that apply and the assumptions you have not\n" +
	"   verified. Ask the user about the ones that would change the result.\n" +
	"3. If an approved spec already covers this, reuse it. Otherwise write one\n" +
	"   from the template below, where AGENTS.md says specs live\n" +
	"   (`docs/specs/<feature>.md` if it names no place). Keep it to WHAT must\n" +
	"   be true: no file-level steps, function design or sequencing.\n" +
	"4. Propose the implementation plan, the HOW, separately in the\n" +
	"   conversation. Do not commit it unless the project asks for that.\n" +
	"5. Stop. Implement only after the user approves the spec and the plan.\n" +
	"\n" +
	"Template:\n" +
	"\n" +
	"```markdown\n" +
	workflow.SpecTemplate +
	"```\n"

// workflowResources returns the adapter's resources when vibe.yaml selects
// a development workflow, and none otherwise. Every workflow gets /spec:
// under direct it is how one asks for a spec.
func workflowResources(mctx *module.Context) []resource.Resource {
	if mctx == nil || mctx.Policies[manifest.DevelopmentWorkflow] == "" {
		return nil
	}
	return []resource.Resource{
		{
			Path:      SpecCommandPath,
			Ownership: resource.Generated,
			Content:   []byte(specCommand),
		},
		{
			Path:      ClaudeMDPath,
			Ownership: resource.ManagedSection,
			SectionID: ImportSectionID,
			Markers:   resource.HTMLComment,
			Placement: resource.Top,
			Content:   []byte(importLine),
		},
	}
}

// CheckSection warns about a second import of AGENTS.md outside
// CLAUDE.md's section: harmless, but the project's line is now redundant.
// It never reports a conflict, and checks no other file.
func (claudeModule) CheckSection(r resource.Resource, before, after []byte) (conflicts, warnings []string) {
	if r.Path != ClaudeMDPath {
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
