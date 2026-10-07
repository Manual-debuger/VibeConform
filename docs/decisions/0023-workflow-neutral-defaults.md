# ADR 0023: Workflow-neutral defaults; the CLAUDE.md import belongs to the harness

## Status

Accepted. Implemented per `docs/specs/0042-workflow-neutral-defaults.md`.
Amends ADR 0015 §3 and §5. Does not remove anything ADR 0015 added.

## Context

ADR 0015 made the workflow opt-in. The docs and examples did not treat
it as optional, though. They showed `plan-triggered-sdd` as the setting
to start from. Issue #50 then framed harness adapters as ways to route a
harness's planning mode into VibeConform's lightweight SDD.

VibeConform's value is the environment around the agent: tooling,
hooks, CI, repository conventions and harness integration. Teams
increasingly bring their own workflow as a skill pack (TDD, specs,
tickets, review). If VibeConform prefers a workflow, it gets in their
way.

ADR 0015 §3 also put the `CLAUDE.md` import of `AGENTS.md` in the
workflow adapter. The import is how Claude Code reads `AGENTS.md` at
all, so it is a harness capability. Tying it to the workflow left a
`claude` repository without a workflow with no import.

## Decision

1. **Absence is the recommended setting.** VibeConform chooses no
   development process unless `development.workflow` is selected. The
   three values remain as an optional, bundled workflow. `direct` is one
   of them, not a neutral mode.
2. **Harness adapters are workflow-neutral.** An agent module generates
   what its harness supports whatever the workflow. Workflow-specific
   entry points come only with a selected workflow: the `spec` skill and
   plan-mode routing in the `AGENTS.md` section. No harness's planning
   mode implies SDD unless an SDD workflow is selected.
3. **`claude-config` owns the root `CLAUDE.md` import whenever `claude` is
   selected.** Component imports stay with the workflow, because the
   component `AGENTS.md` sections exist only with one (spec 0032).

## Consequences

- A `claude` repository without a workflow gains one managed section in
  `CLAUDE.md` on its next sync. Until then `vibe audit` reports it as out
  of date.
- Removing the workflow no longer removes the root `CLAUDE.md` section.
  Deselecting `claude` does.
- ADR 0015 §5 now reads: `AGENTS.md` and `CLAUDE.md` are yours, except
  the workflow section in `AGENTS.md` and the import section in
  `CLAUDE.md`.
- Codex and OpenCode adapters (#50) carry no SDD routing by default.

## Alternatives considered

- **Make `direct` the default.** This was rejected because `direct`
  still generates the `AGENTS.md` section and the `spec` skill. It is a
  workflow.
- **Remove the workflow now.** This was rejected as premature. Removing
  it would break repositories that selected it, and #71 leaves its future
  to a later decision.
- **A `workflow.provider` abstraction.** This was rejected as design
  ahead of need. No second provider exists.
