# Spec 0042: Workflow-neutral defaults

Status: approved 2026-10-07. Addresses #71. Amends specs 0030–0032
(the CLAUDE.md import) and ADR 0015 (ADR 0023). Spec 0043 closes the
routing gap of §6: without a workflow, the docs layout and Graphify each
get their own section of `AGENTS.md`.

## Problem

- VibeConform's docs, the starter sample in `docs/adopting.md` and the
  `python` and `monorepo` examples present the bundled lightweight-SDD
  workflow as the normal setup.
- The open harness-adapter issue (#50) assumes that a harness's planning
  mode routes into that workflow.
- Both give the wrong signal to people who bring their own workflow: an
  external TDD, spec, ticketing or review skill pack.
- The Claude adapter ties a pure harness capability to the workflow. The
  `CLAUDE.md` import that makes Claude Code read `AGENTS.md` exists only
  when `development.workflow` is selected.

## Constraints

- `development.workflow` stays in the schema with its three values. Each
  keeps its current output (specs 0030, 0031, 0032), apart from the root
  `CLAUDE.md` import of §4, which every value already generates.
- `vibe init` still writes no `development:` map.
- Deselection goes through the ordinary ownership and pruning rules
  (ADR 0013). No new migration mechanism.
- Dependency direction (`docs/architecture/overview.md`): agent modules
  depend on `module`, `manifest` and `resource`, not on each other.
- Codex hooks stay suspended (spec 0024, ADR 0011).

## Assumptions

- A1: Claude Code tolerates an `@AGENTS.md` import when `AGENTS.md` does
  not exist. See "Found during implementation" in the plan for what was
  checked.
- A2: An existing repository that selects `claude` and no workflow
  accepts one new managed section in `CLAUDE.md` on its next sync. It is
  additive, and `vibe audit` reports it as out of date until then.

## Desired behaviour

1. **The recommended configuration omits `development.workflow`.** The
   README, `docs/usage.md`, `docs/adopting.md`,
   `docs/architecture/overview.md` and the generated `vibeconform` skill
   say that VibeConform chooses no development process unless a workflow
   is selected, and that leaving it out is the recommended setting. The
   three values are an optional, bundled workflow for repositories that
   choose it.
2. **`direct` is not neutral.** Where the docs list the values, they say
   that `direct` is a VibeConform-managed workflow: it still generates
   the `AGENTS.md` section and a user-invoked `spec` skill. Nothing calls
   it the default or the neutral mode.
3. **Explicit values keep working.** `direct`, `plan-triggered-sdd` and
   `always-sdd` resolve as before, apart from §4. Plan-mode SDD (the
   model-invoked `spec` skill and the "planning context" rule) stays
   available only under the two SDD values.
4. **The import is a harness capability.** With `claude` selected, the
   root `CLAUDE.md` gets the managed `agents` section (`@AGENTS.md`)
   whatever `development.workflow` says. Component `CLAUDE.md` imports
   stay tied to a workflow, because the component `AGENTS.md` sections
   they reach exist only with one (spec 0032).
5. **No harness implies SDD.** A selected harness never generates the
   `spec` skill, plan-mode routing or `/spec` text without a workflow.
   Harness adapter docs describe what the harness supports (instructions,
   skills, hooks, planning modes, commands) apart from workflow selection.
6. **`docs_layout` stands alone.** The docs say it works without
   `workflow`. One gap is recorded as follow-up: with no workflow, nothing
   in `AGENTS.md` routes agents to the layout.
7. **The examples are neutral.** No example selects a workflow.
   `examples/python` keeps `docs_layout: standard`. Tests cover the
   workflow.
8. **The issues are rescoped.** #49 (closed) gets a comment: Plan Mode →
   SDD applies only under an explicitly selected SDD workflow. #50 is
   rescoped to workflow-neutral Codex and OpenCode adapters. Spec 0041's
   deferral of Codex `spec` skill copies becomes conditional on a
   selected workflow.

## Non-goals

- Removing `development.workflow`, the workflow module, the `spec` skill
  or the SDD docs.
- A generic `workflow.provider` abstraction, or any third-party workflow.
- Removing `docs_layout`, or coupling it to `workflow`.
- Changing what `direct` generates.
- Deciding the long-term future of lightweight SDD.
- Building Codex or OpenCode adapters.

## Acceptance criteria

- [ ] `vibe init` writes no `development:` map (existing test kept).
- [ ] No `examples/*/vibe.yaml` selects `development.workflow`, and
      `TestExamplesAreConformant` passes.
- [ ] The README, `docs/usage.md`, `docs/adopting.md`,
      `docs/architecture/overview.md` and the `vibeconform` skill say:
      workflow-neutral unless selected, absence recommended, `direct` is
      not neutral. No doc recommends a workflow value as the default.
- [ ] A test proves the root `CLAUDE.md` import with `claude` and no
      workflow. With a workflow, the output is unchanged apart from that
      section.
- [ ] Component `CLAUDE.md` imports still need a workflow (existing test
      kept).
- [ ] A test proves that `claude` without a workflow generates no
      `.claude/skills/spec/SKILL.md` and no `AGENTS.md` workflow section.
- [ ] The deselection tests pass. Those that expected `CLAUDE.md` to go
      with the workflow now expect the import to stay while `claude` is
      selected and to go when `claude` is deselected.
- [ ] `TestDocsLayoutOnBareRepository` still proves `docs_layout` without
      `workflow`.
- [ ] #49 has the scoping comment, #50 is rescoped, and spec 0041's
      deferral is amended.
- [ ] ADR 0023 is written. The stale "this repository selects
      `always-sdd`" mentions now say `plan-triggered-sdd`.
- [ ] `task verify` and `task audit` pass.
- [ ] May stay UNVERIFIED: an interactive Claude Code session in a
      repository without `AGENTS.md` shows no error for the import.
