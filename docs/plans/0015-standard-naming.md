# Plan 0015: Standard Naming (`prod-go`/`prod-ts`/`prod-py`)

See `docs/specs/0015-standard-naming.md` for the accepted scope.

## Checklist

- [x] `internal/standard/standard.go`: rename the three `Register` calls'
      `Name` fields (`production` → `prod-go`, `production-typescript` →
      `prod-ts`, `production-python` → `prod-py`). Update the package
      comment and the inline comments explaining the naming asymmetry
      (now resolved).
- [x] `internal/standard/standard_test.go`: update literal names.
- [x] `internal/cli/*_test.go` referencing standard names (`audit_test.go`,
      `diff_test.go`, `sync_test.go`, `init_test.go`, `examples_test.go`,
      `tools_test.go`, `manifest_test.go` if applicable): update literals.
- [x] This repository's own `vibe.yaml`: `standard: prod-go`.
- [x] `examples/typescript/vibe.yaml`: `standard: prod-ts`.
- [x] `examples/python/vibe.yaml`: `standard: prod-py`.
- [x] `README.md` and `docs/usage.md`: update every reference to the old
      names.
- [x] Sweep remaining docs (`docs/specs/*`, `docs/plans/*`,
      `docs/decisions/*`) for prose that names the old standards as
      *current* (historical specs describing what was true at the time
      keep their original wording — this is a naming change, not a
      rewrite of history).
- [x] Run `vibe audit --repo-root .` (or `task audit`) after the rename to
      confirm `.vibe/state.yaml` still reports clean (expected: unaffected,
      since it's keyed by resource path). Confirmed: `conformant`, 11
      resources checked, 0 drifted. Both examples also re-audited clean
      under `prod-ts`/`prod-py`.
- [x] `task verify` clean.

## Explicitly still deferred

Per `docs/specs/0015-standard-naming.md`: no alias for the old names, no
module composition changes (that's plan 0016), no standard-migration
tooling.
