# ADR 0002: Desired-State Configuration Model

## Status

Accepted (interfaces only; resolver not yet implemented — see
`docs/specs/0001-v0-control-plane.md`).

## Context

VibeConform needs a model for "what should this repository look like" that
scales from a single CLI skeleton to mixed-language, multi-component
repositories, without repeating the pitfalls of pure template generators
(drift once generated, no reconciliation, no ownership boundaries).

## Decision

Adopt a three-layer model:

1. **`vibe.yaml`** (human-owned): declares which standard and version a
   repository conforms to, plus overrides. Not yet required to exist —
   `internal/manifest` parses `standard`/`version` only.
2. **Standard + modules** (`internal/module`): a named, versioned standard
   is a composition of modules. Each module implements
   `Resolve(ctx, *Context) ([]resource.Resource, error)` and must be
   deterministic. This mirrors projen's `standard -> modules -> resolved
   resources` pipeline rather than Yeoman/Cookiecutter-style one-shot
   generation.
3. **Machine-owned state** (`.vibe/lock.yaml`, `.vibe/state.yaml`, not yet
   created): records what was last resolved and applied, enabling
   three-way reconciliation (previous / current / target) instead of
   naive overwrite-or-skip logic.

## Consequences

- The `Module` interface is intentionally minimal and will grow (e.g. to
  accept component-scoped context) as the resolver is built. It is not
  frozen.
- Because no resolver exists yet, `.vibe/lock.yaml` and `.vibe/state.yaml`
  are not created in M0 — creating them without a reader/writer would be a
  fabricated artifact.
- `vibe.yaml` is deliberately small at this stage (two fields) to avoid
  speculative schema design ahead of the resolver.
