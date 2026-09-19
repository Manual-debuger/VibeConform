# Plan 0006: Reconciliation decision engine + `vibe diff` (v1)

See `docs/specs/0006-reconcile-diff-v1.md` for the accepted scope.

## Checklist

- [x] `internal/state/state.go`: `State{Resources map[string]ResourceState}`,
      `ResourceState{SHA256 string}`, `Load(repoRoot string) (*State, error)`
      reading `.vibe/state.yaml`; returns `&State{}` (empty map, nil error)
      when the file doesn't exist, an error only on malformed YAML.
- [x] `internal/state/state_test.go`: missing file → empty non-nil `State`,
      no error; malformed YAML → error; valid file → parsed
      path→hash map matches.
- [x] `internal/reconcile/reconcile.go`: `Decision` enum
      (`Create`/`NoChange`/`Overwrite`/`Conflict`) with a `String()` method
      for CLI output, and `Decide(previous, current *string, target string)
      Decision`. Package doc comment reproduces spec 0006's truth table and
      the row-6/row-7 rationale verbatim (single source of truth, cited by
      `vibe sync` later rather than re-derived).
- [x] `internal/reconcile/reconcile_test.go`: table-driven test with one
      case per row of the spec's truth table (8 rows), asserting the exact
      `Decision` returned.
- [x] `internal/cli/diff.go` (new file): `newDiffCmd` + `runDiff(cmd
      *cobra.Command, repoRoot string) error`, mirroring `audit.go`'s
      shape:
      1. Parse `vibe.yaml`, look up standard (same two calls as
         `runAudit`).
      2. `state.Load(repoRoot)`.
      3. For each module's resolved resources: if `Ownership !=
         resource.Generated`, print `<path>: not yet supported by diff` and
         continue; otherwise hash `Content` (target), read+hash the file at
         `filepath.Join(repoRoot, path)` if it exists (current), look up
         `st.Resources[path].SHA256` if present (previous), call
         `reconcile.Decide`, print `<path>: <decision-specific line>`.
      4. Decision → line text: `Create` → `create (no file on disk)`;
         `NoChange` → `no change`; `Overwrite` → `would update (drift from
         last applied state)`; `Conflict` → `conflict: manual changes
         detected, review before sync`.
      5. Always returns `nil` on a successful run regardless of decisions
         found (no exit-code gating, per spec's non-goals).
- [x] `internal/cli/commands.go`: remove `newDiffCmd` (moves to `diff.go`);
      `sync`/`check`/`doctor` stubs unchanged.
- [x] `internal/cli/diff_test.go`: cases for a fresh repo (`Create`), an
      up-to-date repo (`NoChange`), a repo with an untracked pre-existing
      `.golangci.yml` differing from target (`Conflict`), manifest missing
      (error), unknown standard (error). Mirrors `audit_test.go`'s
      `t.TempDir()` + `root.Execute()` pattern.
- [x] `internal/cli/root_test.go`: drop `"diff"` from
      `TestSubcommandsNotYetImplemented`'s stub list (mirrors spec 0004's
      plan removing `"audit"`); `TestRootCmdHelp`'s command-name assertions
      are unaffected (already lists `diff`).
- [x] `docs/usage.md`: split `vibe diff` out of the "not implemented" bullet
      into its own documented section (flags, example output, behavior —
      matching the `vibe audit` section's structure); the remaining
      "not implemented" section covers only `sync`/`check`/`doctor`.
- [x] `README.md`: factual status update — `vibe diff` now previews
      reconciliation for the one `Generated` resource that exists
      (`.golangci.yml`); `.vibe/state.yaml` is read but nothing writes it
      yet.
- [x] `task verify` clean.

## Explicitly still deferred

Unchanged from `docs/plans/0005-gotooling-module.md` except reconciliation
itself now partially exists. Per `docs/specs/0006-reconcile-diff-v1.md`:
no `vibe sync` (no writer for `.vibe/state.yaml`, no filesystem mutation),
no decision logic for `StructuredPatch`/`ManagedSection`/`ProjectOwned`
ownership, no `vibe audit` change, no `--strict`/exit-code-on-conflict flag
for `diff`, no concurrency handling for `.vibe/state.yaml`. Also still
deferred: affected-component graph, `check`/`doctor` real implementations,
dynamic/config-driven standard loading, `vibe.yaml`-driven content
customization, second module.
