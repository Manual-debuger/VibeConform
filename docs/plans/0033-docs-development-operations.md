# Plan 0033: Optional `docs/development/` and `docs/operations/`

Implements `docs/specs/0033-docs-development-operations.md`. Tracks
issue #49. Accepted at review (2026-10-01), as part of the #49 plan.

## Changes

- `internal/manifest`: `docs_development` and `docs_operations` added to
  `Development` and to `ScalarKeys`.
- `internal/standard`:
  - two catalog groups, each with an `on` option that requires
    `docs_layout`;
  - a requirement on a setting is now named by its key and value
    (`development.docs_layout: standard`), not by the bare option name.
- `internal/module/workflow/layout.go`: `Layout`, which renders both the
  docs index and the `AGENTS.md` knowledge rule. `NewDocsDir` is the
  options' module, which resolves nothing. `Content` keeps its signature.
  `LayoutContent` takes a layout.
- Tests: every selection's index and knowledge rule, the word cap, the
  `Requires` error and `(valid: on)`, and a CLI test that writes `on` in
  a real `vibe.yaml` and turns it off again.
- Docs: `docs/usage.md`'s development section.

## Found during implementation

- With neither key set, the output is byte-identical to spec 0030: the
  existing golden tests and the synced examples are unchanged.
- This repository does not opt in.

## Verification

| Check | Result |
|---|---|
| Unit and CLI tests, lint | PASS |
| `task verify`, `task audit` | PASS |
