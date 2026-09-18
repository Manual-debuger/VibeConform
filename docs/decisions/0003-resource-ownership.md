# ADR 0003: Explicit Resource Ownership

## Status

Accepted (model only; reconciler not yet implemented).

## Context

Tools that generate repository files typically pick one of two failure
modes: they either own a file completely (any manual edit is silently
clobbered on the next run) or they rely on fragile text markers
(`# BEGIN VIBE`, `<!-- managed -->`) as the *primary* mechanism for
deciding what they may touch, which breaks silently when markers are
edited, reformatted, or omitted.

## Decision

Every `resource.Resource` carries an explicit `Ownership` mode:

- `generated` — VibeConform owns the whole file; safe to overwrite when the
  file matches the previously resolved state (see ADR-0002 three-way
  reconciliation).
- `structured-patch` — VibeConform owns specific structured fields inside a
  document (e.g. specific YAML/JSON keys) and must merge rather than
  overwrite.
- `managed-section` — VibeConform owns a delimited section within an
  otherwise project-owned file. Text markers are used here, but only as a
  *scoping* mechanism inside a resource whose ownership is already known
  from the module that produced it — not as the thing that determines
  ownership in the first place.
- `project-owned` — read-only context; the reconciler must never write it.

Ownership is a property the *module* declares when it produces a resource,
not something inferred by scanning file contents at reconciliation time.

## Consequences

- The reconciler (not yet implemented) can apply the three-way logic from
  ADR-0002 per-resource using `Ownership` to decide whether "conflict"
  should even be possible for a given file.
- Modules must be honest about ownership; a module that emits
  `project-owned` resources is documenting expected repository state for
  audit purposes only.
- This is a model decision, not an API freeze — the `Ownership` enum may
  grow (e.g. a future `append-only` mode) as real modules are built.
