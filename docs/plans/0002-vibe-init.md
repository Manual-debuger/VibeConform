# Plan 0002: `vibe init` (v1)

See `docs/specs/0002-vibe-init.md` for the accepted scope.

## Checklist

- [x] `internal/manifest.New` / `internal/manifest.Manifest.Marshal`, with
      round-trip test against `Parse`.
- [x] `vibe init <standard> <version> [--repo-root]` writes `vibe.yaml`,
      refuses to overwrite an existing one.
- [x] Tests: happy path, refuse-to-overwrite, missing required argument.
- [x] `README.md` updated to reflect `init` no longer being a stub.
- [x] `task verify` clean.

## Explicitly still deferred

Unchanged from `docs/plans/0001-bootstrap.md`: standard/module resolution,
`.vibe/lock.yaml` / `.vibe/state.yaml`, reconciliation, affected-component
graph, `audit`/`diff`/`sync`/`check` real implementations.
