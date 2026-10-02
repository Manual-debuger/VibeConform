# Plan 0034: Adopting an existing docs layout

Implements `docs/specs/0034-adopt-docs-layout.md`. Tracks issue #49.
Accepted at review (2026-10-01), as part of the #49 plan.

## Changes

- `internal/manifest/docsdirs.go`:
  - the five path keys and `DocsDirs`, the effective directories;
  - `validateDevelopment`, which reuses the component path rules,
    checks each key's prerequisite, and rejects overlapping directories;
  - `AdoptedDirs`, the keys that must exist.
- `module.Context.DocsDirs` carries the directories to the modules as a
  typed struct, so free-form paths are never confused with option names.
- `internal/cli/plan.go`:
  - `checkDocsDirs` stops `diff`, `audit` and `sync` when an adopted
    directory is missing;
  - `planMoved` prunes a recorded section whose file moved
    (`module.SectionMover`, implemented by docs-layout for `specs`).
- Section prune messages take a cause, as the file prunes of spec 0031
  do.
- `internal/module/workflow`: `Layout` is `manifest.DocsDirs`. The index
  links relative to `docs/`, with `../` for a path outside it.
- Tests: manifest validation, the adopted golden index and section, and
  CLI tests of a missing directory, adoption, and moving `specs_dir`
  (both unchanged and modified).
- Docs: `docs/usage.md`, and the architecture overview's removal rules.

## Found during implementation

- **Moving the specs section needed its own prune.** Changing a path is
  not an option deselection, so ADR 0013's prune never saw the old
  section. `module.SectionMover` covers that case and nothing else: only
  sections a module declares movable, and only when the module still
  resolves the same ID elsewhere.
- No ADR: `Retirer` and `SectionMover` are optional interfaces that
  extend the removal rules of ADR 0013 without changing them, and the
  architecture overview records them.

## Verification

| Check | Result |
|---|---|
| Unit and CLI tests, lint | PASS |
| `task verify`, `task audit` | PASS |
| Without path keys, generated output unchanged (goldens, examples) | PASS |
