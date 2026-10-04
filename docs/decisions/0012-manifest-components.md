# ADR 0012: `components:` in `vibe.yaml`

## Status

Accepted. Implemented by `internal/manifest`, per
`docs/specs/0025-prod-mono.md`.

## Context

Through spec 0024 a repository declares exactly one standard, and every
standard assumes one language at the repository root: `go.mod`,
`package.json`, or `pyproject.toml` beside `Taskfile.yml`. Spec 0014
deferred "multiple profiles or components per repository" on purpose —
a new free-form standard name cost a registry entry, where a manifest
field cost a schema, a validation story, and a resolver.

A polyglot monorepo — Go services, a TypeScript app, a Python worker in
one repository — cannot be expressed by choosing a standard name. What
it needs to say is *where* each language lives, and no standard can know
that in advance. `docs/architecture/overview.md` and ADR 0004 have sketched
the shape since bootstrap:

```yaml
components:
  - id: web
    path: apps/web
    profile: ts
```

## Decision

`vibe.yaml` gains an optional `components:` list. Each entry has exactly
three fields (ADR 0018 later adds an optional fourth, `generated:`):

| Field | Rule |
|---|---|
| `id` | `^[a-z][a-z0-9-]*$`, unique, and not one of the names the generated root `Taskfile.yml` and `ci.yml` already use (`audit`, `fmt`, `gate`, `hook`, `lint`, `local`, `test`, `typecheck`, `verify`, `verify-ci`, `vibe`, `workflows`). It becomes a Task namespace (`task web:verify`) and a CI job key, so it is restricted to what both accept unquoted and cannot collide with either. |
| `path` | Slash-separated, relative, already clean (`path.Clean` leaves it unchanged), not `.`, no `..`. Unique, and no component may sit inside another's path. |
| `profile` | One of `go`, `ts`, `py` — the language half of the single-language standards' names. |

Decoding is strict: an unknown key anywhere in `vibe.yaml` is an error,
not ignored. With `components:` a typo (`component:`, `profle:`) would
otherwise silently resolve a standard with nothing in it.

A standard says whether it takes components. `prod-mono/v1` requires at
least one; `prod-go`, `prod-ts`, and `prod-py` accept none. Either mismatch
is a plan error before anything resolves, never a silently ignored field.

`depends_on`, from the overview's sketch, is **not** added. It exists for
the affected-component graph, which is not built; a field nothing reads is
a promise with no check behind it.

## Consequences

- `module.Context` carries the components, so modules resolve from the
  manifest instead of only from constants. Resolution stays deterministic:
  same `vibe.yaml`, same resources.
- `module.ToolRequirer` takes the context too: which binaries a monorepo
  needs depends on which profiles it declares, and warning about `uv` in a
  repository with no Python component is exactly the false alarm spec 0014
  warns against.
- Nested components are rejected rather than supported. A Go component at
  `.` would put every other component inside `gofmt -l .` and
  `go vet ./...`; forbidding overlap keeps each component's tools confined
  to its own directory without per-tool exclude lists.
- Strict decoding is a compatibility break only for a `vibe.yaml` carrying
  a key no version of `vibe` ever read. That file was already not doing
  what its author meant.
