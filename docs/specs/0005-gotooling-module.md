# Spec 0005: Go-tooling module (`internal/module/gotooling`)

Status: accepted and implemented.

## Problem

`internal/module.Module` and `internal/resource.Resource` exist, but
`production`/`v1` is registered with an empty `Modules` slice — an explicit
placeholder per `docs/specs/0003-standard-registry.md`. There is no concrete
module implementation anywhere in the codebase, so `vibe audit`'s report
cannot grow past a bare module count, and nothing exercises the
`Module.Resolve` -> `resource.Resource` path end-to-end.

## Scope

A new package, `internal/module/gotooling`, provides one module:

- `Module.Name()` returns `"go-tooling"`.
- `Module.Resolve` returns exactly one `resource.Resource`:
  - `Path`: `.golangci.yml`
  - `Ownership`: `resource.Generated`
  - `Content`: a fixed golangci-lint v2 configuration, embedded into the
    binary via `go:embed` from a template file checked into the package
    (seeded as a copy of this repo's own `.golangci.yml`).

`production`/`v1` in `internal/standard` registers this module, replacing
the zero-modules placeholder:

```go
Register(Standard{
    Name:    "production",
    Version: "v1",
    Modules: []module.Module{gotooling.New()},
})
```

## Behavior

- `Resolve` ignores `mctx.RepoRoot` and any other `Context` field — the
  produced content is fixed for v1, not computed from the target
  repository. It always returns the same single resource and a `nil` error;
  there is no failure mode yet (embedded content can't fail to read at
  runtime).
- `vibe audit` against a `vibe.yaml` declaring `production`/`v1` will now
  print `1 modules configured, nothing to check` (count changes from 0 to
  1). The message text itself is unchanged — see non-goals.

## Explicit non-goals

- No resource-writing/reconciliation: `Resolve` only returns the
  `resource.Resource` value in memory. Nothing applies it to disk —
  `internal/reconcile` does not exist yet.
- No lock/state files (`.vibe/lock.yaml`, `.vibe/state.yaml`).
- No second module — `go-tooling` is the only module in `production`/`v1`
  for this change.
- No change to the `module.Module` or `module.Context` interfaces — this
  module's needs (a fixed, repo-root-independent resource) don't require
  anything beyond what already exists.
- No change to `vibe audit`'s output format or exit-code policy beyond the
  module count naturally changing from 0 to 1 (spec 0004 already documents
  the count-based report, and already lists "per-module compliance
  results" as separate follow-on work once a real module exists — this
  spec is that trigger, not that follow-on). In particular, the existing
  "N modules configured" string is not pluralization-aware (it will read
  "1 modules configured"); fixing that wording is out of scope here.
- No content customization via `vibe.yaml` overrides — the golangci config
  is a fixed template for v1, not parameterized.
- No validation that a target repository's actual `.golangci.yml` matches
  the resolved resource — that is reconciliation/diff behavior, not this
  module's or audit's concern yet.

## Design notes

- `internal/module/gotooling` depends on `internal/module` and
  `internal/resource`, preserving the dependency direction in
  `docs/architecture/overview.md` — not the reverse.
- The template is embedded (`go:embed`) rather than a Go string literal so
  it stays reviewable/diffable as YAML, and rather than reading this
  repo's live `.golangci.yml` at runtime so the module's desired state
  isn't accidentally coupled to VibeConform's own dev tooling drifting.
- `gotooling.New()` returns a value satisfying `module.Module`; no exported
  struct fields, mirroring `internal/standard`'s preference for small,
  predictable constructors over configuration structs until a real need
  for options appears.

## Follow-on work

- Once `internal/reconcile` exists, actually write/compare
  `.golangci.yml` against the resolved resource.
- Once a second module exists, revisit whether `vibe audit`'s report
  should enumerate module names/results instead of a bare count (spec
  0004's deferred follow-on).
- Consider whether the "N modules configured" string should be
  pluralization-aware — cosmetic, owned by spec 0004 if picked up.
