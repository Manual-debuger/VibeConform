# Plan 0009: `vibe audit` (v2 — strict conformance gate)

See `docs/specs/0009-vibe-audit-v2.md` for the accepted scope, and
`docs/plans/0007-m1-milestone.md` for where this sits in M1.

## Checklist

- [x] `internal/cli/exit.go` (new): unexported `nonConformantError{drifted,
      conflicts int}` implementing `error`, plus
      `ExitCode(err error) int` — `0` for nil, `2` when `errors.As` matches
      the non-conformance type, `1` otherwise. Exported doc comment
      explains the contract for `main`.
- [x] `internal/cli/exit_test.go`: nil → 0; non-conformance error → 2;
      wrapped non-conformance error (`fmt.Errorf("audit: %w", …)`) → 2;
      any other error → 1.
- [x] `cmd/vibe/main.go`: `os.Exit(cli.ExitCode(err))` in place of the
      unconditional `os.Exit(1)`.
- [x] `internal/cli/audit.go`: `runAudit` rewritten over `buildPlan` —
      header, one line per resource per the spec's table, summary
      `<N> resource(s) checked, <N> drifted, <N> conflicts`, then
      `conformant` / `not conformant`. Returns the non-conformance error
      when anything drifted or conflicted; returns a plain wrapped error for
      manifest/standard/state/I-O failures. Drops the manifest and
      `standard.Lookup` calls it duplicated from `diff`.
- [x] `internal/cli/audit_test.go`: rewrite the module-count case (that
      output no longer exists) into:
      - conformant repo (file matches target) → exit 0, `ok`, `conformant`;
      - missing file → `missing`, `ExitCode` 2;
      - drifted file with recorded state → `drifted`, `ExitCode` 2;
      - conflicted file, no recorded state → `conflict`, `ExitCode` 2;
      - missing `vibe.yaml` → `ExitCode` 1;
      - unknown standard → `ExitCode` 1.
      Assert through `ExitCode(err)`, not just `err != nil`, or the 1-vs-2
      distinction is untested.
- [x] `docs/usage.md`: rewrite the `vibe audit` section — new output, the
      decision→line table, the three exit codes and what each means, and
      the "use `audit` in CI, `diff` for a preview" split.
- [x] `README.md`: status banner — `audit` is a strict conformance gate,
      not a module-count report.
- [x] `task verify` clean.

## Notes

- `audit`, `diff`, and `sync` now share `buildPlan` and differ only in what
  they do with the decisions: `audit` judges, `diff` describes, `sync`
  applies. Any future change to how a decision is reached belongs in
  `plan.go`, not in one of the three commands.
- Wording differs deliberately between commands: `diff` says "would
  update", `audit` says "drifted". `diff` describes a hypothetical action;
  `audit` describes the repository's present state.

## Explicitly still deferred

Per `docs/specs/0009-vibe-audit-v2.md`: no `--fix`, no `--strict` (strict is
the behavior), no per-module grouping, no `--json`, no orphan detection, no
change to `diff`/`sync`/`reconcile.Decide`. Unchanged from
`docs/plans/0008-vibe-sync-v1.md`: no `--force`, no `--dry-run`, no orphan
pruning, no `.vibe/lock.yaml`, no locking/concurrency handling, no decision
or apply logic for `StructuredPatch`/`ManagedSection`/`ProjectOwned`
ownership, no affected-component graph, no `check`/`doctor`
implementations, no dynamic/config-driven standard loading, no
`vibe.yaml`-driven content customization.
