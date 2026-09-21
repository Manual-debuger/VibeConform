# Spec 0015: Standard Naming (`prod-go`/`prod-ts`/`prod-py`)

Status: accepted and implemented.

## Problem

`production`, `production-typescript`, and `production-python` are
inconsistent: the Go standard's name does not say it is Go, while the other
two spell their language out in full. `docs/specs/0014-m2-milestone.md`
already flagged this as a known asymmetry, deliberately deferred to M3:

> `production/v1` is the Go standard but is not called `production-go/v1`,
> because renaming it would break this repository's own `vibe.yaml` and
> `.vibe/state.yaml` for cosmetic gain. M3's profile/component work is where
> naming gets settled.

M3 is that point. This spec settles the naming, nothing else.

## Scope

Hard rename, no alias, in `internal/standard/standard.go`:

- `production/v1` → `prod-go/v1`
- `production-typescript/v1` → `prod-ts/v1`
- `production-python/v1` → `prod-py/v1`

Module composition is unchanged for all three — this is a name-only change.

Touches:

- This repository's own `vibe.yaml` (`standard: prod-go`).
- `examples/typescript/vibe.yaml` → `standard: prod-ts`.
- `examples/python/vibe.yaml` → `standard: prod-py`.
- `README.md`, `docs/usage.md`, and any other prose referencing the old
  names.
- Every test asserting on the old literal names, including but not limited
  to `internal/standard/standard_test.go` and `internal/cli/*_test.go`
  (`audit_test.go`, `diff_test.go`, `sync_test.go`, `init_test.go`,
  `examples_test.go`, `tools_test.go`).

`.vibe/state.yaml` is unaffected by this change — it is keyed by resource
path, not standard name (see the resource keys in that file: they are
paths like `.golangci.yml`, not the standard). `vibe audit` is re-run after
the rename to confirm this repository stays clean.

## Behavior

`vibe init prod-go v1` (and `prod-ts`/`prod-py` equivalents) resolves. The
old names — `production`, `production-typescript`, `production-python` —
become unknown standards: `standard.Lookup` returns the same not-found
error it already returns for any typo. No special deprecation message, no
redirect.

## Explicit non-goals

- No back-compat alias for the old names. Pre-1.0, breaking the name is
  cheaper than carrying a permanent alias in the registry.
- No change to what modules each standard composes — that is spec 0016.
- No general standard-migration CLI feature. Nothing rewrites an adopting
  repository's `vibe.yaml` on rename automatically; this repository's own
  file is hand-migrated as part of this change, same as any other adopter
  would have to do.
- No broader "profile/component" naming scheme. `docs/specs/0014-m2-
  milestone.md`'s follow-on work mentions this separately; it is not part
  of this spec.
- No version bump semantics change — all three standards stay at `v1`;
  this is a name change, not a behavior change requiring a new version.

## Design notes

- Names read uniformly as `prod-<lang>`, closing the asymmetry called out
  in spec 0014. `go` is spelled out rather than left implicit, since
  leaving it implicit is exactly the asymmetry being fixed.
- This repository's `vibe.yaml` is migrated by hand rather than through
  tooling, because no tooling for this exists yet (see non-goals) and
  building it would be scope creep for a naming change.

## Follow-on work

- Spec 0016: TS/PY tooling parity, sequenced after this rename so it can
  use the new names (`prod-ts`, `prod-py`) directly.
