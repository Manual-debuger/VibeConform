# Spec 0031: `/spec` as a Claude Code Skill

Status: accepted and implemented. Tracks issue #49 ("Claude Skill
follow-up", and the unverified `/spec` checks of spec 0030). Revises spec
0030 §5: the Claude Code adapter generates a skill instead of a command.

## Problem

- Spec 0030 delivered `/spec` as a project command,
  `.claude/commands/spec.md`. Claude Code runs a command only when the
  user types it. The intended normal trigger for lightweight SDD is the
  harness's plan mode, so in practice the spec step relied on the agent
  reading AGENTS.md and choosing to follow it.
- Claude Code also loads Agent Skills (`.claude/skills/<name>/SKILL.md`).
  Claude can invoke a skill by itself when the skill's description fits
  the situation, and the user can still invoke it as `/<name>`. A skill
  named `spec` gives both triggers from one file.
- Spec 0030 verified `/spec` only through `claude -p`. Plan mode and
  interactive sessions were left UNVERIFIED.

## Scope

### 1. The skill replaces the command

When `development.workflow` is selected and so is the `claude`
integration, `claude-config` generates `.claude/skills/spec/SKILL.md`
(ownership `generated`). It no longer generates `.claude/commands/spec.md`.

- The front matter carries `name: spec`, a `description` and
  `argument-hint: <feature or issue>`.
- The description says what the skill does, which is to write a
  lightweight spec, propose a plan and stop. It names its triggers:
  plan mode, and a request to plan or spec a non-trivial change.
- **Under `workflow: direct` the skill is user-invoked only**
  (`disable-model-invocation: true`, and a description without the plan
  mode trigger). Under `direct`, a planning context writes a spec only
  "when asked" (spec 0030 §3), and an automatic spec in every plan mode
  would contradict that. Under the two SDD workflows, Claude may invoke
  it.
- The body is the command's body from spec 0030 §5, with three changes:
  - it works with no arguments, the case when Claude invokes it, by
    taking the change under discussion;
  - it says that in a read-only plan mode the spec goes in the plan
    until it is approved, as the AGENTS.md section already does;
  - it calls itself a skill rather than a command.
- The template is still `workflow.SpecTemplate`, which is shared with
  `docs/specs/README.md`.

### 2. The AGENTS.md section names the skill

When `/spec` exists (claude selected), the planning rule in the
`workflow` section says to use the `spec` skill. Without claude, the
section is unchanged. The 300-word cap of spec 0030 §3 still holds for
every selection.

### 3. Retiring a file a module no longer generates

Upgrading leaves a recorded `.claude/commands/spec.md` behind. Today,
nothing removes it: pruning (ADR 0013) covers only paths that a
deselected option would still produce.

- A module may declare retired paths: whole files it once generated and
  no longer does, each with a reason shown in messages, e.g. "replaced
  by .claude/skills/spec/SKILL.md".
- A retired path is handled exactly like a deselected option's file:
  - recorded and unchanged since sync: `diff` says it would remove it,
    `audit` reports it out of date, and `sync` removes it;
  - recorded but already gone: sync forgets it;
  - recorded and modified: a conflict; kept;
  - not recorded: never touched.
- A path that a selected module still resolves is never retired.
- Retirement is checked for every module of the standard, selected or
  not, so the file goes even if `claude` is deselected in the same
  change.

## Behavior

| Before | `vibe.yaml` | After `vibe sync` |
|---|---|---|
| 0030 state, unmodified command | workflow + claude | skill created; command removed (replaced by the skill) |
| 0030 state, edited command | workflow + claude | skill created; command kept with a conflict |
| nothing | workflow + claude | skill created |
| skill | claude deselected, or workflow removed | skill removed, as for any deselected option |

## Verification of the trigger

There is no hook. Whether Claude invokes the skill by itself in plan mode
is measured, and the result is recorded in plan 0031:

- A. Five `claude -p --permission-mode plan` runs in a scratch repository
  with `always-sdd`. Each run gets a non-trivial change prompt that does
  not mention `/spec`. The record says how many runs invoked the skill.
- B. One `claude -p` run with an explicit `/spec <change>`.
- C. Interactive plan mode, and interactive `/spec`. A person runs these;
  until then the ledger keeps them UNVERIFIED.

If A shows that self-invocation is unreliable, a new issue proposes the
plan-mode reminder hook of issue #49. It is not part of this spec.

## Explicit non-goals

- A hook that detects plan mode (spec 0030's non-goal stands).
- Skills for other harnesses (#50).
- Any change to the SDD semantics, or to `docs/specs/README.md`.
- A general mechanism for retiring sections or structured-patch elements.
  Retirement here is for whole generated files, which is what this
  change needs.

## Acceptance criteria

- [ ] With a workflow and claude, sync writes `.claude/skills/spec/SKILL.md`, pinned byte for byte by a test for each workflow, and no command file.
- [ ] Under `direct` the skill has `disable-model-invocation: true`. Under the SDD workflows it does not.
- [ ] A recorded, unmodified `.claude/commands/spec.md` is removed by sync, shown by diff and reported by audit, with the reason "replaced by .claude/skills/spec/SKILL.md". A modified one is a conflict and is kept. An unrecorded one is never touched.
- [ ] The AGENTS.md section names the `spec` skill when claude is selected. The golden tests are updated, and the word cap holds for every selection.
- [ ] Checks A and B are run and recorded in plan 0031. Check C is recorded as UNVERIFIED unless a person runs it.
