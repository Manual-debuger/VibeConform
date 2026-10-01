# Plan 0032: Component AGENTS.md and CLAUDE.md for `prod-mono`

Implements `docs/specs/0032-component-agents.md`. Tracks issue #49.
Accepted at review (2026-10-01), as part of the #49 plan.

## Changes

- `internal/module/workflow/workflow.go`: `ComponentContent(c)` and one
  `component` section per component in `Resolve`. The link to the root
  climbs one `../` per path segment.
- `internal/module/agents/claude/workflow.go`: `importSection(p)` for the
  root's and each component's `CLAUDE.md`. `CheckSection` now checks any
  `CLAUDE.md`, matching by base name.
- Tests: the golden section for a nested and a shallow path, the
  80-word cap for every profile, resource counts with and without a
  workflow, and a CLI test of project prose, sync output and deselection.
- `examples/monorepo` selects `plan-triggered-sdd` and is synced. Docs:
  the prod-mono part of `docs/usage.md`.

## Found during implementation

- **Removing a component does not prune its sections.** The approved
  plan's criterion said it would. Spec 0008 and ADR 0013 rule out orphan
  pruning, and a component's other generated files (its tooling config
  and Taskfile) are not pruned either. Spec 0032 §3 keeps that rule
  rather than making component sections the one exception. Deselecting
  the workflow, or `claude`, does remove them, because that is an option
  deselection.
- **Every generated Markdown section failed Prettier.** CI's
  `examples/monorepo` job failed `web:fmt:check` on the new
  `apps/web/AGENTS.md` and `CLAUDE.md`. A scratch `prod-ts` repository
  showed the bug dates from spec 0030: `fmt:check` runs Prettier over the
  whole repository, and Prettier rewrote the `AGENTS.md`, `CLAUDE.md`,
  `docs/README.md` and `docs/specs/README.md` sections. It wanted a blank
  line after the opening marker, before the closing one, and between a
  label such as `Knowledge:` and its list. `workflow.MarkdownSection` now
  pads every Markdown section, and the labels are followed by a blank
  line. The text is unchanged, and every existing section is updated
  once on the next sync. The regression test is
  `TestSectionsArePrettierStable`. Prettier 3.9.8 passed every regenerated
  file, in a scratch `prod-ts` repository and in the monorepo.
- Spec 0032's assumption about tasks holds: every profile's component
  Taskfile defines `verify` and `verify:fast`, and the root includes it
  as `<id>`.

## Verification

| Check | Result |
|---|---|
| Unit and CLI tests, lint | PASS |
| `task verify`, `task audit` | PASS |
| `examples/monorepo` synced with vibe `10cdc1c`; three component sections and imports created | PASS |
| Claude Code loading a component's `CLAUDE.md` in an interactive session | UNVERIFIED |
