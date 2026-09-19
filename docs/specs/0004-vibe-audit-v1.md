# Spec 0004: `vibe audit` (v1)

Status: accepted and implemented.

## Problem

`vibe audit` is currently a stub (`errNotImplemented`). `internal/manifest`
can parse `vibe.yaml` and `internal/standard` can resolve a declared
standard/version to a registered `Standard`, but nothing connects the two —
there is no command that reads a repository's desired state and reports
anything about it. Per `docs/architecture/overview.md`, `audit` is meant to
be the read-only compliance/drift check, safe to run in CI.

## Scope

`vibe audit [--repo-root]` (same flag convention as `vibe init`):

1. Reads `vibe.yaml` from `--repo-root` (default `.`) via
   `internal/manifest.Parse`.
2. Looks up the declared `(standard, version)` via `internal/standard.Lookup`.
3. Reports against `Standard.Modules` trivially — since every registered
   standard has zero modules today, the only honest report is "N modules
   configured, nothing to check" (N == `len(Standard.Modules)`).
4. Exits non-zero only on a real failure: `vibe.yaml` missing/unreadable, or
   the declared standard/version not found in the registry. Exits 0 when the
   manifest and standard both resolve, regardless of module count.

Example output:

```
$ vibe audit
standard: production/v1
0 modules configured, nothing to check
```

## Explicit non-goals

- No `diff`/`sync` — this is read-only reporting, not reconciliation.
- No `.vibe/lock.yaml` / `.vibe/state.yaml` reads or writes — nothing to
  compare against yet.
- No real module compliance checks — `Standard.Modules` is empty for every
  registered standard today; audit reports the count honestly rather than
  fabricating checks against modules that don't exist.
- No exit-code policy beyond "unknown standard or missing manifest = failure."
  In particular, "0 modules configured" is NOT a compliance failure and must
  not produce a non-zero exit — a repo has nothing to be non-compliant
  against yet.
- No `--strict` or severity-level flags — those only make sense once there
  are real checks to grade.

## Design notes

- Mirrors `vibe init`'s `--repo-root` flag and its `internal/cli/init.go`
  structure: a `newAuditCmd` constructor plus a `runAudit` function that
  takes `cmd *cobra.Command` and writes to `cmd.OutOrStdout()`, for the same
  testability reasons (see `internal/cli/init_test.go`).
- Reuses `internal/manifest.Parse` and `internal/standard.Lookup` directly;
  no new package is introduced for this v1 — there is no orchestration logic
  complex enough yet to justify `internal/audit/` from
  `docs/architecture/overview.md`'s future package layout.
- `internal/cli/root_test.go`'s `TestSubcommandsNotYetImplemented` currently
  asserts `audit` returns the not-implemented error; that assertion is
  removed for `audit` (it moves to a dedicated `audit_test.go`, mirroring
  `init_test.go`), while `diff`/`sync`/`check`/`doctor` remain in the stub
  list.

## Follow-on work

- Once `internal/reconcile` exists, `audit` becomes strict: non-zero exit on
  real drift, not just "manifest didn't resolve."
- Once a first real module is registered into `production`/`v1`, audit's
  report grows from a module count into per-module compliance results.
- `.vibe/lock.yaml` / `.vibe/state.yaml` reads, once `internal/state` exists,
  to report drift against previously-applied state rather than only against
  the standard's current definition.
