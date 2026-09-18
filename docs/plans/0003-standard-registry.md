# Plan 0003: Standard Registry (`internal/standard`)

See `docs/specs/0003-standard-registry.md` for the accepted scope.

## Checklist

- [x] `internal/standard.Standard` type (`Name`, `Version`, `Modules
      []module.Module`).
- [x] `internal/standard.Register` (panics on duplicate `(name, version)`)
      and `internal/standard.Lookup` (returns an error on not found).
- [x] Seed `production`/`v1` registered via package `init()`, zero modules.
- [x] Tests: lookup hit, lookup miss (error, not panic), duplicate
      registration panics.
- [x] `task verify` clean.

## Explicitly still deferred

Unchanged from `docs/plans/0002-vibe-init.md`: standard/module resolution,
`.vibe/lock.yaml` / `.vibe/state.yaml`, reconciliation, affected-component
graph, `audit`/`diff`/`sync`/`check` real implementations. Additionally, per
`docs/specs/0003-standard-registry.md`: no concrete `module.Module`
implementations, no CLI wiring of `standard.Lookup`, no dynamic/config-
driven standard loading.
