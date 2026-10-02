# Spec 0032: Component AGENTS.md and CLAUDE.md for `prod-mono`

Status: accepted and implemented. Tracks issue #49 ("Component-level
AGENTS.md"). Builds on spec 0025 (`prod-mono`), spec 0030 (the workflow
section) and spec 0031 (the spec skill).

## Problem

- In a `prod-mono` repository, an agent working inside a component may
  never read the root `AGENTS.md`. Codex and other AGENTS.md readers use
  the nearest file. Claude Code loads a subdirectory's `CLAUDE.md` when
  it works there.
- A component AGENTS.md written by hand tends to repeat the root's
  policy, and then drift from it.
- Nothing says which tasks verify one component on its own.

## Scope

### 1. The `component` section

When `development.workflow` is selected and the standard takes
components, each component's `<path>/AGENTS.md` gets a managed section
`component`, with HTML markers, at the bottom (like the root's
`workflow` section). It says:

- which component this is: its id and profile;
- that the root `AGENTS.md`, linked relatively, holds the workflow and
  the rules, and that they apply here unchanged;
- how to verify this component alone: `task verify:fast` and
  `task verify` from its directory, and `task <id>:verify` from the root.

It restates no policy. A test caps it at 80 words. The rest of the file
stays the project's, as with the root section.

### 2. The component `CLAUDE.md` import

When `claude` is also selected, each component's `<path>/CLAUDE.md` gets
the `agents` section (`@AGENTS.md`) at the top, as the root does
(spec 0030 §5). Claude Code resolves the import relative to that file,
so it reads the component's AGENTS.md, which links to the root. A
project import of `AGENTS.md` outside the section warns, as at the root.

### 3. Selection and removal

- Without a workflow, components get neither section.
- Deselecting the workflow removes every component section. Deselecting
  `claude` removes the component imports. Both follow the existing rules
  for removing sections (spec 0029 §5).
- Removing a component from `vibe.yaml` leaves its sections in place,
  just like its other generated files (its tooling config and Taskfile).
  Spec 0008 and ADR 0013 rule out orphan pruning, and this spec does not
  change that.

## Explicit non-goals

- Component-specific workflows, rules or overrides (spec 0025's non-goal
  stands).
- Pruning a removed component's files.
- Component sections for standards that take no components.
- Changing the root `workflow` section.

## Acceptance criteria

- [ ] The component section is pinned byte for byte for a nested and a shallow path, and is at most 80 words for every profile.
- [ ] Resolve emits one `component` section per component, plus one `CLAUDE.md` `agents` section per component when claude is selected, and none of either without a workflow.
- [ ] An existing component AGENTS.md keeps its prose, with the section appended at the bottom.
- [ ] Deselecting the workflow removes the component sections, and the files VibeConform created for them.
- [ ] `examples/monorepo` selects a workflow and is synced, so the example shows the result.
