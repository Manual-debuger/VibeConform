---
name: vibeconform
description: How this repository's VibeConform-managed tooling works and how to change it. Use before editing a file VibeConform manages (Taskfile.yml, lefthook.yml, CI configuration, lint or format configuration, agent settings) or vibe.yaml, when adding tasks or Git hooks, when task audit or the conformance check fails, or when asked how this repository's tooling is set up.
---

# VibeConform

This repository declares a VibeConform standard in `vibe.yaml`.
`vibe sync` generates part of its tooling from that standard and
records what it wrote in `.vibe/state.yaml`. CI checks that the files
still match.

## Managed files

- A managed file is generated whole. A managed section is a block
  between VibeConform markers in a file that is otherwise the
  project's, such as `.gitignore`, `AGENTS.md` or an editor's task file.
- `vibe audit` lists every managed file and section. Do not guess the
  list. Run the command.
- Do not edit managed content by hand. The edit is drift: `vibe audit`
  and CI's conformance check fail, and the next `vibe sync` restores
  the file or reports a conflict.

## Changing what is managed

1. Edit `vibe.yaml`. Editing it by hand is expected.
2. Run `vibe diff` to preview the change. It writes nothing.
3. Run `vibe sync`. It writes the files and `.vibe/state.yaml`.
4. Commit the changed files and `.vibe/state.yaml` together.

If the standard cannot express the change, tell the user. Do not edit
a managed file to work around it.

Changes that belong to the project go where VibeConform manages
nothing:

- `Taskfile.local.yml`: the project's own tasks, such as build, run or
  deploy. `Taskfile.yml` includes it. A task with the name of a
  generated task is an error, so it cannot redefine `verify`, `lint`,
  `test` or `audit`.
- `lefthook.local.yml`: the project's own Git hooks. `lefthook.yml`
  extends it.
- Anything outside the markers of a managed section.

## Commands

- `vibe audit`: a read-only check. Exit 0: conformant. Exit 3: only out
  of date (the standard moved and nothing was edited). Exit 2: not
  conformant (drifted, missing or a conflict). Exit 1: it could not
  answer, for example because this `vibe` is older than the one that
  last synced the repository.
- `vibe diff`: a preview of `vibe sync`. It exits 0 whatever it finds.
- `vibe sync`: the only command that writes. It never overwrites a file
  that was edited by hand. It reports a conflict and exits non-zero.
  Move the edit to a place the project owns, restore the file with
  `git checkout <file>`, and sync again.
- `vibe doctor`: checks whether this machine can run the workflow: the
  tools on `PATH` and the agent hooks. It does not check conformance.
- `task audit`: runs `vibe audit`. Without `vibe` on `PATH`, it
  installs the `vibe_version` that `.vibe/state.yaml` records, so CI
  uses the `vibe` that last synced. To upgrade, sync with a newer
  `vibe` and commit the new `vibe_version`.

Use `task verify:fast` while you work and `task verify` before you say
the work is done. Both use the language tools only and never need
`vibe`. The `hook:*` tasks are for the agent hooks. Do not copy them.

## `vibe.yaml`

Every standard accepts these keys:

- `integrations:` with `editors` (`vscode`, `zed`), `agents`
  (`claude`, `codex`, both on by default) and `intelligence` (code
  intelligence, off by default). A category that is not given takes
  its defaults. `[]` means none.
- `policy: {line_endings: lf}`: a managed section of `.gitattributes`.
- `development:` with `workflow` (`direct`, `plan-triggered-sdd` or
  `always-sdd`) and `docs_layout: standard`.

VibeConform decodes the file strictly: an unknown key is an error.

## Standard: `prod-mono/v1`

- `components:` lists each project with an `id`, a `path` and a
  `profile` (`go`, `ts` or `py`). Each component is a complete
  single-language project at its path, with its own `Taskfile.yml`.
- In a component's directory, `task verify` and the other tasks work as
  in a single-language repository. From the root, `task <id>:verify`
  runs one component, and `task verify`, `task verify:fast` and
  `task fmt` run every component.
- A component's `generated:` list names code that a generator writes,
  relative to the component. Format and lint checks skip it. Type
  checks still run. A `go` component does not accept it.
- `ci: {provider: ...}` selects the CI system: `github` (the default),
  `gitlab` or `none`. This repository uses `github`.
- CI: `.github/workflows/ci.yml` runs one job per component and ends
  in the `CI / gate` check. `.github/workflows/conformance.yml` runs
  `task audit` as `Conformance / audit`. Both must be required checks.

## Removing VibeConform

Delete these. The other generated files keep working as ordinary
configuration.

- `vibe.yaml` and `.vibe/`
- `Taskfile.vibe.yml`
- `.github/workflows/conformance.yml`. Also remove `Conformance / audit`
  from the required checks.
- the `vibeconform` skill in `.claude/skills/` and `.agents/skills/`
