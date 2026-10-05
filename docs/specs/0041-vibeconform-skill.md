# Spec 0041: A `vibeconform` skill for adopter agents

Status: approved 2026-10-05. Addresses part of #50 (skills for other
harnesses), for this one skill only. AGENTS.md and CLAUDE.md are not
changed.

## Problem

- An adopter's coding agent learns almost nothing about VibeConform
  from what `vibe sync` generates. The AGENTS.md `workflow` section
  (spec 0030) names no `vibe` command, no managed files, no
  `.vibe/state.yaml` and no `Taskfile.local.yml`. It exists only when
  `development.workflow` is selected.
- The rule "do not hand-edit managed files" is written only in this
  repository's own AGENTS.md. Its CI check (`Conformance / audit`)
  catches the edit, but only after the agent has made it.
- So the agent either edits a managed file and CI fails, or it reads
  VibeConform's repository and `docs/usage.md` to find out how the
  repository works, again in every session.

## Constraints

- `docs/architecture/principles.md`: agent instructions are for
  judgment. The skill points to `vibe audit` and `task audit` as the
  enforcement. It does not replace them.
- Principle 3 ("Removing VibeConform"): removal stays "delete files,
  edit none". The skill names `vibe`, so it is VibeConform-specific, and
  each copy joins the list of files to delete.
- Content is Go constants, not embedded templates (the CRLF hazard of
  spec 0030 §3), and tests pin it byte for byte.
- Spec 0024: one module per agent runtime. Each agent module owns its
  own harness's files. The skill text is shared, and each module chooses
  only the path.

## Assumptions

- Codex discovers repository skills in `.agents/skills/<name>/SKILL.md`
  in every directory from the working directory up to the repository
  root. `name` and `description` are the required front matter, and
  Codex may load a skill when the task matches its `description`.
  Checked against the Codex skills documentation on 2026-10-05; not
  verified in a Codex session.
- Codex loads skills while its hooks are suspended (ADR 0011). Skills
  are not hooks. Not verified.
- Claude Code loads a third project skill beside `spec` and `graphify`
  with no conflict. Not verified.

## Scope

### 1. Where it is written

| Selected agent | Module | Path |
|---|---|---|
| `claude` | `claude-config` | `.claude/skills/vibeconform/SKILL.md` |
| `codex` | `codex-config` | `.agents/skills/vibeconform/SKILL.md` |

- Each copy is ownership `generated`, whatever `development:` selects.
- The two copies are byte-identical. Each harness decides by itself, from
  the `description`, when to load the skill. Neither copy sets
  `disable-model-invocation`.

### 2. What it says

The front matter carries `name: vibeconform` and a `description`. The
description names the triggers:

- editing a file VibeConform manages, or `vibe.yaml`;
- adding tasks or Git hooks;
- a failing `task audit` or conformance check;
- a question about how the repository's tooling is set up.

The body has a part that is the same in every standard:

- What a managed file is: a whole file, or a managed section between
  markers. `vibe audit` lists them. Do not guess the list.
- Do not edit managed content by hand. To change it, edit `vibe.yaml`,
  run `vibe diff`, then `vibe sync`, and commit the files with
  `.vibe/state.yaml`. If the standard cannot express the change, tell
  the user. Do not work around it.
- Where project-owned changes go: `Taskfile.local.yml`,
  `lefthook.local.yml`, and the unmanaged parts of shared files.
- The commands `audit`, `diff`, `sync` and `doctor`, and the exit
  codes of `audit`: 0 conformant, 3 out of date only, 2 not
  conformant, 1 could not answer.
- That `task audit` runs the `vibe` version that `.vibe/state.yaml`
  records, and that an upgrade is a sync with a newer `vibe`.
- The `vibe.yaml` keys that every standard accepts: `integrations`,
  `policy`, `development`.
- How to remove VibeConform: the files to delete.

The rest of the body depends on the standard, and for `prod-mono` on
`ci.provider`:

- the tasks that the standard guarantees, and the toolchain that they call;
- the CI checks to make required;
- the `vibe.yaml` keys only this standard accepts (`generated:`,
  `components:`, `ci:`);
- for `prod-mono`: components, and `task <id>:verify`.

The skill holds nothing that the standard and `ci.provider` do not
determine. It changes only when they change or when `vibe` changes. It is
at most 900 words for every standard and provider.

## Behaviour

| Selection | Before | After |
|---|---|---|
| `claude` and `codex` (the default) | no VibeConform guidance | both copies |
| one agent | none | that agent's copy |
| no agents | none | none |
| an agent deselected later | its copy | sync removes it (`<agent> deselected`) |

## Explicit non-goals

- Any change to AGENTS.md or CLAUDE.md.
- Listing the repository's managed files in the skill. `vibe audit` is
  the source.
- Codex copies of the `spec` and `graphify` skills (the rest of #50).

## Acceptance criteria

- [ ] For `prod-go`, `prod-ts`, `prod-py` and `prod-mono` under `github`,
      `gitlab` and `none`, a test pins the skill byte for byte.
- [ ] With `claude` selected, sync creates
      `.claude/skills/vibeconform/SKILL.md`. With `codex` selected, it
      creates `.agents/skills/vibeconform/SKILL.md`. The two are
      byte-identical.
- [ ] Both are generated with no `development:` key.
- [ ] The 900-word cap holds for every standard and provider.
- [ ] A second sync changes nothing. Deselecting one agent removes only
      its copy.
- [ ] `docs/usage.md` lists both copies in what each standard manages,
      and "Removing VibeConform" lists them as files to delete. Removal
      still needs no edits.
- [ ] This repository and `examples/*` are synced, and
      `TestExamplesAreConformant` passes.
- [ ] `task verify` and `task audit` pass.
- [ ] May stay UNVERIFIED: in a new adopter repository, Claude Code and
      Codex each load the skill when asked to change `lefthook.yml`.
