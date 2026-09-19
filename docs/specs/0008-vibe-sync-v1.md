# Spec 0008: `vibe sync` (v1)

Status: proposed.

## Problem

`internal/reconcile.Decide` produces a decision per resource and `vibe diff`
reports it, but nothing acts on it. Nothing writes a resolved resource to
disk, and nothing writes `.vibe/state.yaml` — `internal/state` is a reader
only, so `Decide`'s `previous` argument is always `nil` in practice and two
of its eight truth-table rows (`Overwrite`) can never be reached. The
reconciliation loop is open at the writing end.

Spec 0006 deferred this deliberately: mutating a user's repository is a
different risk profile than reading it, and deserves its own spec. This is
that spec, and the first increment of `docs/plans/0007-m1-milestone.md`.

## Scope

One new state function, one refactor, one CLI command.

- `internal/state.Save(repoRoot string, s *State) error`:
  - Creates `.vibe/` if absent (`0o750`).
  - Marshals `s` to YAML and writes `.vibe/state.yaml` atomically: write to
    a temp file in the same directory, `fsync`, then `os.Rename` over the
    destination. An interrupted run must never leave a truncated state file
    — a half-written state file is worse than no state file, because it
    makes every subsequent decision wrong rather than merely conservative.
  - File mode `0o600`, matching `internal/cli/init.go`'s manifest write.

- `internal/cli/plan.go` (new): the resolve → hash → `Decide` walk currently
  inline in `internal/cli/diff.go`, extracted so `diff` and `sync` share one
  implementation.

  ```go
  type resourcePlan struct {
      Resource   resource.Resource
      Decision   reconcile.Decision
      TargetHash string
      Supported  bool // false for non-Generated ownership
  }

  func buildPlan(repoRoot string) (*standard.Standard, []resourcePlan, error)
  ```

  `runDiff` becomes a reporter over `buildPlan`'s output with no behavior
  change. `diff` must remain exactly the preview of what `sync` does; two
  copies of this walk would drift silently.

- `vibe sync [--repo-root]` (same flag convention as `init`/`audit`/`diff`):
  1. `buildPlan(repoRoot)`.
  2. For each planned resource, in plan order:
     - `Create` / `Overwrite` → `os.MkdirAll` the parent directory, write
       the resource content atomically (same temp + rename as `Save`),
       record `TargetHash` in the new state.
     - `NoChange` → no write; still record `TargetHash` in the new state, so
       a repository that already matched becomes tracked and subsequent runs
       decide three-way instead of two-way.
     - `Conflict` → do not touch the file; report it; carry the resource's
       existing state entry through unchanged if it had one.
     - Unsupported ownership → do not touch the file; report it; no state
       entry.
  3. `state.Save` once, after the loop.
  4. Exit non-zero if any resource decided `Conflict`.

Example output against a fresh repository declaring `production/v1`:

```
$ vibe sync
standard: production/v1
.golangci.yml: created
1 created, 0 updated, 0 unchanged, 0 conflicts
```

And on a second run:

```
$ vibe sync
standard: production/v1
.golangci.yml: unchanged
0 created, 0 updated, 1 unchanged, 0 conflicts
```

## Behavior

### Decision → action

| Decision | File on disk | State entry | Exit |
|---|---|---|---|
| `Create` | written | recorded | 0 |
| `Overwrite` | written | recorded | 0 |
| `NoChange` | untouched | recorded | 0 |
| `Conflict` | untouched | prior entry preserved | non-zero |
| unsupported ownership | untouched | none | 0 |

### Partial failure

If writing a resource fails (permissions, full disk), `sync` stops
processing further resources, still calls `state.Save` with the entries
recorded so far, and returns the error. Recording what actually landed
before surfacing the failure keeps the repository recoverable: the next run
decides correctly about the resources that were written, rather than
treating them as unrecorded and reporting spurious conflicts.

A failure in `state.Save` itself is returned as-is. The files are already on
disk at that point; the next run sees them as unrecorded (`previous` nil)
and, since they match the target, decides `NoChange` — degraded to two-way
reconciliation, not corrupted.

### Exit codes

- `0` — every resource applied or already conformant.
- non-zero — at least one `Conflict`, or an I/O/parse error. v1 does not
  distinguish the two numerically; cobra returns 1 for any returned error.
  A distinct drift exit code is spec 0009's problem, where `audit` becomes
  a gate and CI needs to tell "non-conformant" from "the tool broke".

### State keys

State keys are repository-relative and slash-separated. `sync` normalizes
with `filepath.ToSlash` before recording, and joins with `filepath.Join`
only at the I/O boundary. No current resource path contains a separator, but
the CI module (plan 0007's increment 0010) introduces `.github/workflows/…`,
and without this a Windows run and a Linux run would write different state
files for the same repository.

## Explicit non-goals

- **No `--force`.** A `Conflict` is a hard stop in v1; the escape hatch is
  deleting the file or reconciling it by hand. An override flag changes what
  the tool is allowed to destroy and deserves its own spec rather than
  riding along with the first writer.
- **No `--dry-run`.** `vibe diff` already is the dry run, and after this
  spec's refactor they are provably the same walk.
- **No orphan pruning.** A resource recorded in `.vibe/state.yaml` but no
  longer produced by any module is left on disk and left in state. Deleting
  files a standard no longer claims is a separate, more dangerous behavior.
- **No backups of overwritten files.** `Generated` ownership means
  VibeConform owns the whole file; git is the recovery mechanism.
- **No `vibe audit` change** — spec 0009 makes `audit` strict once `sync`
  gives users a way to fix what it reports.
- **No `.vibe/lock.yaml`.** State alone closes the loop.
- **No `StructuredPatch` / `ManagedSection` / `ProjectOwned` application** —
  still no module producing them (spec 0006's non-goal, unchanged).
- **No locking or concurrency handling.** Single short-lived CLI process.

## Design notes

- Atomic write (temp + rename) is used for both resources and the state
  file, via one unexported helper. `os.Rename` replaces an existing
  destination on both POSIX and Windows in Go, so no platform branch is
  needed.
- Content is written as the module resolved it, byte for byte, with no
  line-ending translation. This repository pins `* text=auto eol=lf` in
  `.gitattributes`, so embedded LF templates round-trip through a Windows
  checkout unchanged; translating on write would produce a hash mismatch and
  perpetual drift on Windows.
- `state.Save` lives in `internal/state` beside `Load`, keeping the file
  format owned by one package. The resource-writing I/O stays in
  `internal/cli`, mirroring spec 0006's design note about keeping I/O out of
  the composed packages.
- `buildPlan` returns the `*standard.Standard` alongside the plan so both
  commands can print the `standard: <name>/<version>` header without
  re-resolving.
- `internal/cli/root_test.go`'s `TestSubcommandsNotYetImplemented` drops
  `sync`; `check`/`doctor` remain stubs.

## Follow-on work

- `vibe audit` v2 (plan 0007 increment 0009): reuse `buildPlan`, report per
  resource, exit non-zero when any resource is not `NoChange`, with an exit
  code distinguishable from a tool error.
- `--force` / conflict resolution UX, once there is evidence of what real
  conflicts look like — earliest signal is the dogfood increment (0013).
- Orphan pruning, once a standard has actually dropped a resource.
