# Plan 0008: `vibe sync` (v1)

See `docs/specs/0008-vibe-sync-v1.md` for the accepted scope, and
`docs/plans/0007-m1-milestone.md` for where this sits in M1.

## Checklist

- [ ] `internal/atomicfile/atomicfile.go` (new leaf package):
      `Write(path string, data []byte, perm os.FileMode) error` — temp file
      in the destination's directory, `Sync()`, `Close()`, `os.Rename` over
      the destination, removing the temp file on any failure path. Depends
      on the standard library only.
      *Refinement of the spec's "one unexported helper": `state.Save` and
      the resource write live in different packages, so the helper has to
      be exported from somewhere. A leaf package beats duplicating the
      temp+rename dance in two places or exporting a file utility from
      `internal/state`.*
- [ ] `internal/atomicfile/atomicfile_test.go`: writes new file with the
      requested mode; replaces existing file's content; leaves no temp file
      behind on success; leaves the original intact when the destination
      directory doesn't exist (error path).
- [ ] `internal/state/state.go`: `Save(repoRoot string, s *State) error` —
      `os.MkdirAll(filepath.Join(repoRoot, ".vibe"), 0o750)`, `yaml.Marshal`,
      `atomicfile.Write(..., 0o600)`. Package doc updated: the "only a
      reader exists so far" sentence is now false.
- [ ] `internal/state/state_test.go`: `Save` → `Load` round-trip preserves
      the path→hash map; `Save` creates `.vibe/` when absent; `Save` over an
      existing state file replaces it; saving an empty `State` then loading
      yields an empty non-nil map.
- [ ] `internal/cli/plan.go` (new): `resourcePlan{Resource, Decision,
      TargetHash, Supported}` and `buildPlan(repoRoot string)
      (*standard.Standard, []resourcePlan, error)`, holding the manifest
      read/parse, `standard.Lookup`, `state.Load`, per-module `Resolve`,
      hashing, and `reconcile.Decide` currently inline in `diff.go`.
      `hashHex` moves here. Error strings stay unprefixed; each command
      wraps with its own `"<cmd>: %w"`.
- [ ] `internal/cli/diff.go`: `runDiff` becomes a reporter over
      `buildPlan` — resolve, then one `diffLine` per plan entry, plus the
      `"not yet supported by diff"` line where `Supported` is false.
      `diffLine`'s text is unchanged.
- [ ] `internal/cli/diff_test.go`: **unmodified**. The existing five cases
      passing untouched is the evidence that the extraction changed no
      behavior; if a test needs editing, the refactor was not a refactor.
- [ ] `internal/cli/sync.go` (new): `newSyncCmd` + `runSync(cmd
      *cobra.Command, repoRoot string) error`, mirroring `diff.go`'s shape
      and `--repo-root` flag:
      1. `buildPlan(repoRoot)`; print `standard: <name>/<version>`.
      2. Per plan entry, in order: `Create`/`Overwrite` → `os.MkdirAll` the
         parent dir (`0o750`), `atomicfile.Write(content, 0o600)`, record
         `filepath.ToSlash(path)` → `TargetHash`; `NoChange` → record only;
         `Conflict` → no write, carry any prior state entry through
         unchanged; unsupported → no write, no entry.
      3. A write error stops the loop, still calls `state.Save` with what
         was recorded, and returns the write error.
      4. `state.Save` once after the loop.
      5. Per-resource lines (`created` / `updated` / `unchanged` /
         `conflict: manual changes detected, resolve before sync` /
         `not yet supported by sync`), then the summary
         `N created, N updated, N unchanged, N conflicts`.
      6. Return a non-nil error when any entry decided `Conflict`, after
         all entries are processed and state is saved.
- [ ] `internal/cli/commands.go`: remove `newSyncCmd` stub (moves to
      `sync.go`); `check`/`doctor` stubs unchanged.
- [ ] `internal/cli/root_test.go`: drop `"sync"` from
      `TestSubcommandsNotYetImplemented`'s list, leaving `check`/`doctor`.
- [ ] `internal/cli/sync_test.go`, following `diff_test.go`'s `t.TempDir()`
      + `root.Execute()` pattern:
      - fresh repo → `.golangci.yml` written with the module's exact bytes,
        `.vibe/state.yaml` records its hash, exit 0;
      - second run over the first's output → `unchanged`, file mtime/content
        stable, state unchanged, exit 0 (idempotence);
      - pre-existing differing file, no state → `conflict`, non-zero exit,
        **file content unchanged on disk** (the assertion that matters);
      - pre-existing file plus a state entry recording that same file's hash
        → `Overwrite` → file replaced with target, state updated, exit 0.
        This is the first test anywhere to reach an `Overwrite` row;
      - missing `vibe.yaml` → error; unknown standard → error.
- [ ] `docs/usage.md`: `vibe sync` moves out of the "not implemented"
      section into its own section (flags, example output for first and
      second run, decision table, exit codes, `Conflict` guidance). The
      remaining "not implemented" section covers `check`/`doctor` only.
- [ ] `README.md`: factual status update — `sync` applies `Generated`
      resources and writes `.vibe/state.yaml`; the "nothing writes that
      file yet" claim in the `vibe diff` sentence is now false.
- [ ] `task verify` clean.

## Notes

- Written resources use mode `0o600`, consistent with `init.go`'s manifest
  write. Git tracks only the executable bit, so this does not affect
  checkouts; the exec-bit question arrives with the agent-config module
  (`.claude/hooks/*.sh`) in plan 0007's increment 0012, not here.
- No test covers nested-directory creation, because no module produces a
  nested path yet. `os.MkdirAll` is implemented now so increment 0010's CI
  module doesn't have to change `sync`; 0010 brings the test with it.

## Explicitly still deferred

Per `docs/specs/0008-vibe-sync-v1.md`: no `--force`, no `--dry-run` (`diff`
is the dry run), no orphan pruning, no backups of overwritten files, no
`vibe audit` change, no `.vibe/lock.yaml`, no locking/concurrency handling,
no distinct exit code for drift vs. tool error. Unchanged from
`docs/plans/0006-reconcile-diff-v1.md`: no decision or apply logic for
`StructuredPatch`/`ManagedSection`/`ProjectOwned` ownership, no
affected-component graph, no `check`/`doctor` implementations, no
dynamic/config-driven standard loading, no `vibe.yaml`-driven content
customization, no second module.
