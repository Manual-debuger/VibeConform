# Spec 0043: AGENTS.md routing without a workflow

Status: draft. Follow-up to spec 0042 (#71).

## Problem

- Spec 0042 made the workflow-neutral setting the recommended one: no
  `development.workflow`.
- Two selections only reach agents through the workflow section of
  `AGENTS.md`:
  - `development.docs_layout` (where specs, architecture docs and ADRs
    live);
  - Graphify (when the graph may be used).
- So in a workflow-neutral repository nothing tells agents about either.
  `examples/python` lost its Graphify paragraph this way when it dropped
  its workflow.

## Constraints

- ADR 0015: VibeConform owns delimited sections of `AGENTS.md`, never the
  whole file. Every line is derived from `vibe.yaml`. Each section routes
  and does not explain.
- No workflow text without a workflow (spec 0042 §5). The new sections
  name no process: no planning context, no spec-before-plan, no approval
  rule.
- Repositories that select a workflow see no change. Their workflow
  section already carries this routing, and its bytes stay the same.
- Deselection uses the ordinary pruning rules: ADR 0013, and
  `module.ConditionalSectioner` (spec 0037 §5). No new mechanism.
- Content is Go constants, pinned by tests (the CRLF hazard, spec 0030
  §3).

## Assumptions

- Two `Bottom` sections from different modules in one `AGENTS.md` are
  ordered deterministically, as `.gitignore`'s `claude` and `graphify`
  sections already are. Not yet checked for `AGENTS.md`.
- The guidance in the "Knowledge" block is neutral, not process. That
  covers where the documents live, reading them before a non-trivial
  change, and saying so when spec, ADR and code disagree.

## Desired behaviour

1. **Docs layout.** With `docs_layout` selected and no `workflow`,
   `AGENTS.md` gets a `knowledge` section at the bottom. It holds a
   heading, a "Managed by VibeConform from `development.docs_layout`"
   line, and the same Knowledge block the workflow section uses for that
   layout. That includes the optional directories (spec 0033) and
   adopted directories (spec 0034).
2. **Graphify.** With Graphify selected and no `workflow`, `AGENTS.md`
   gets an `intelligence` section at the bottom. It holds a heading, a
   "Managed by VibeConform from `integrations.intelligence`" line, and the
   same Graphify paragraph the workflow section uses.
3. **With a workflow,** neither section is generated. The workflow
   section is unchanged.
4. **Transitions** go through `vibe sync`:
   - selecting a workflow removes both sections, and the workflow section
     takes over;
   - removing the workflow brings them back;
   - deselecting `docs_layout` or Graphify removes its section.

   A modified section is kept as a conflict. If VibeConform created
   `AGENTS.md` and nothing else is left in it, the file is deleted.
5. **Bounds.** Each section is at most 120 words for every layout.
6. `examples/python` (docs layout and Graphify, no workflow) gets both
   sections. `docs/usage.md` drops the "known gap" note of spec 0042.

## Non-goals

- Routing for verification tasks (`task verify`) without a workflow.
  The `vibeconform` skill already names them.
- Component `AGENTS.md` sections without a workflow.
- Changing the workflow section's text or splitting it.
- Codex or OpenCode adapters.

## Acceptance criteria

- [ ] Golden tests pin the `knowledge` section for the default layout,
      the optional directories and an adopted layout. A golden test pins
      the `intelligence` section.
- [ ] A test proves that neither section resolves when a workflow is
      selected.
- [ ] A CLI test covers the transitions in Desired behaviour §4: no
      workflow → workflow → none, and deselecting `docs_layout` and
      Graphify. A second sync changes nothing.
- [ ] The 120-word cap holds for every layout variant.
- [ ] `examples/python` is synced and `TestExamplesAreConformant` passes.
      This repository's `AGENTS.md` is unchanged, since it selects a
      workflow.
- [ ] `docs/usage.md` (development workflow, Graphify) describes both
      sections.
- [ ] `task verify` and `task audit` pass.
