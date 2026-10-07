# Plan 0043: AGENTS.md routing without a workflow

Implements `docs/specs/0043-neutral-agents-routing.md`, a follow-up of
#71 (spec 0042). The spec and plan were accepted together at review
(2026-10-07). It is tracked under #71, with no issue of its own.

Branch: `feat/0043-neutral-agents-routing`, off `main` after PR #72.

## Changes

- `internal/module/workflow`:
  - `neutral.go`: `KnowledgeContent(layout)`, `IntelligenceContent()`,
    `Selected(mctx)` and `AgentsSection(id, content)`.
  - The workflow section's knowledge list and Graphify bullet are now
    `knowledgeRules()` and `graphifyRule`. The new sections share them,
    and the workflow section's bytes do not change.
  - `docslayout.go`: without a workflow, the docs layout also resolves the
    `knowledge` section. It is declared through `ConditionalSections`.
- `internal/module/intelligence/graphify`: the same for the
  `intelligence` section.
- `internal/textregion`: the bug fix described below.
- Tests:
  - golden tests for both sections, including the 120-word cap and the
    absence of workflow text;
  - neither section is resolved with any workflow value;
  - `TestNeutralRouting`, which covers no workflow → workflow → none,
    deselecting each option, and that a second sync changes nothing;
  - `TestRemoveFirstBottomSection`;
  - updated `TestDocsLayoutResources`, graphify `TestResolve` and
    `TestGraphifyFollowsSelection`.
- `examples/python` was synced and gained both sections. The other
  examples and this repository were synced with no change to their files,
  so `AGENTS.md` here is unchanged because it selects a workflow.
- Docs:
  - `docs/usage.md`: the docs layout, Graphify, deselection, the
    integrations table and the non-goals list;
  - status lines in specs 0042 and 0043, and in plan 0042.

## Found during implementation

- **Section order.** Within one `AGENTS.md`, `intelligence` comes before
  `knowledge`, because the standard resolves graphify before the docs
  layout. The order is deterministic, and `TestNeutralRouting` pins it.
  This confirms the spec's assumption.
- **A textregion bug, fixed with a regression test.**
  - The scenario: a file holds two `Bottom` sections, inserted into an
    empty file, and the first is removed.
  - What went wrong: `textregion.Remove` left behind the empty line that
    the second section's insert had put after the first. The file then
    never became empty, so it was not deleted when the last section left.
  - This predates spec 0043. It could already hit `.gitignore`, which
    holds `claude` and `graphify` sections. The workflow → none transition
    made it visible.
  - The fix: when a `Bottom` section starts the file, removing it also
    removes the empty line after it.
- **Prune message.** When a workflow takes over, `sync` reports the
  neutral sections as "removed (no longer declared in vibe.yaml)". That
  is the shared wording of `ConditionalSectioner`, left unchanged as the
  plan said.
