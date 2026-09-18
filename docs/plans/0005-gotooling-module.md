# Plan 0005: Go-tooling module (`internal/module/gotooling`)

See `docs/specs/0005-gotooling-module.md` for the accepted scope.

## Checklist

- [ ] `internal/module/gotooling/gotooling.go`: unexported struct
      satisfying `module.Module`, `New() module.Module` constructor,
      `Name()` returns `"go-tooling"`.
- [ ] `internal/module/gotooling/golangci.yml` (embedded template, seeded
      as a copy of this repo's own `.golangci.yml`) + `//go:embed`
      directive wiring it into `Resolve`.
- [ ] `Resolve(ctx, mctx)` returns `[]resource.Resource{{Path:
      ".golangci.yml", Ownership: resource.Generated, Content: <embedded
      bytes>}}, nil`, ignoring `mctx`.
- [ ] `internal/module/gotooling/gotooling_test.go`: `Name()` value,
      `Resolve` returns exactly one resource with the expected path/
      ownership/content, two calls produce byte-identical output
      (determinism).
- [ ] `internal/standard/standard.go`: register `gotooling.New()` into
      `production`/`v1`'s `Modules`, replacing the empty slice.
- [ ] `internal/standard/standard_test.go`: update/extend to assert
      `production`/`v1` now has exactly one module named `"go-tooling"`.
- [ ] `internal/cli/audit_test.go`: rename
      `TestAuditCmdReportsZeroModules` to reflect the new count and update
      its assertion from `"0 modules configured"` to
      `"1 modules configured"`.
- [ ] `docs/usage.md`: update the `vibe audit` example output to
      `1 modules configured, nothing to check`, and correct the "every
      registered standard has zero modules today" claim now that
      `production`/`v1` has one.
- [ ] `README.md`: status banner note that `production`/`v1` now composes
      one module (`go-tooling`, a `.golangci.yml` resource) — factual
      update only, no scope claims beyond what's implemented.
- [ ] `task verify` clean.

## Explicitly still deferred

Unchanged from `docs/plans/0004-vibe-audit-v1.md`: standard/module
resolution beyond `Lookup`/`Resolve` returning in-memory values,
`.vibe/lock.yaml` / `.vibe/state.yaml`, reconciliation, affected-component
graph, `diff`/`sync`/`check`/`doctor` real implementations, dynamic/config-
driven standard loading, `--strict`/severity flags. Additionally, per
`docs/specs/0005-gotooling-module.md`: no resource-writing/reconciliation
for `go-tooling`'s resource, no second module, no `module.Module`/
`module.Context` interface changes, no `vibe.yaml`-driven content
customization, no change to `vibe audit`'s report format beyond the module
count changing from 0 to 1.
