# Spec 0006: Reconciliation decision engine + `vibe diff` (v1)

Status: proposed.

## Problem

`internal/module.Module.Resolve` can produce `resource.Resource` values (one
today, from `internal/module/gotooling`), but nothing compares a resolved
resource against a target repository's actual filesystem state or against
what VibeConform previously applied. `docs/architecture/overview.md`'s
three-way reconciliation model (previous / current / target) and
`docs/decisions/0003-resource-ownership.md`'s ownership modes are documented
but unimplemented. `vibe diff` is currently a stub. Both spec 0004 and spec
0005 name the reconciliation engine as their follow-on trigger.

## Scope

Two new leaf packages plus one CLI command:

- `internal/state`: schema and reader for `.vibe/state.yaml`, the
  machine-owned record of what VibeConform last applied.
  - `type State struct { Resources map[string]ResourceState }` keyed by
    repository-relative resource path.
  - `type ResourceState struct { SHA256 string }` — a content hash, not the
    full content, to keep the state file small.
  - `Load(repoRoot string) (*State, error)`: returns an empty `State` (not
    an error) when `.vibe/state.yaml` does not exist; returns an error only
    on a malformed file.
- `internal/reconcile`: the three-way decision engine.
  - `type Decision int` with values `Create`, `NoChange`, `Overwrite`,
    `Conflict`.
  - `func Decide(previous *string, current *string, target string) Decision`
    — pure function, no I/O. `previous`/`current` are `nil` when absent
    (no recorded prior state / no file on disk); `target` is always
    present (a resolved resource always has content).
  - Only `resource.Generated` ownership is handled by a decision-taking
    path in this spec. `resource.StructuredPatch`, `resource.ManagedSection`,
    and `resource.ProjectOwned` are explicitly out of scope (see non-goals)
    — no registered module produces them today, so implementing decision
    logic for them now would be speculative.
- `vibe diff [--repo-root]` (same flag convention as `init`/`audit`):
  1. Parse `vibe.yaml` and look up the standard, reusing
     `internal/manifest.Parse` / `internal/standard.Lookup` exactly as
     `runAudit` does.
  2. Load `.vibe/state.yaml` via `internal/state.Load`.
  3. For each module, call `Resolve`, then for each `resource.Generated`
     resource: read the current file from `--repo-root` (absent if it
     doesn't exist), look up its recorded hash in state (absent if not
     recorded), and call `reconcile.Decide`.
  4. Print one line per resource with its path and decision.
  5. Any non-`Generated` resource is printed with a fixed
     "not yet supported by diff" line rather than silently skipped or
     erroring the whole command.
  6. Always exits 0 — `diff` is a preview, not a compliance gate (mirrors
     spec 0004's exit-code scoping for `audit`).

Example output against `production/v1` (one `Generated` resource,
`.golangci.yml`, no prior state, no existing file):

```
$ vibe diff
standard: production/v1
.golangci.yml: create (no file on disk)
```

## Behavior — `reconcile.Decide` truth table

Let P = previous recorded hash (nil if unrecorded), C = current file hash
(nil if the file doesn't exist), T = target hash (always present). All
comparisons are by content hash, not raw bytes, since `state.ResourceState`
only stores a hash.

| P absent? | C absent? | C == T | C == P | T == P | Decision |
|---|---|---|---|---|---|
| yes | yes | – | – | – | `Create` |
| yes | no | yes | – | – | `NoChange` |
| yes | no | no | – | – | `Conflict` |
| no | yes | – | – | – | `Create` |
| no | no | yes | – | – | `NoChange` |
| no | no | no | yes | no | `Overwrite` |
| no | no | no | no | yes | `Overwrite` |
| no | no | no | no | no | `Conflict` |

Rationale for the two `no/no/no` rows that split from ADR-0002's literal
"current != previous and target != previous → conflict" wording:

- `C != P`, `T == P` (row 6): the desired state hasn't changed since the
  last apply, but the file on disk has drifted from it — e.g. a hand edit
  of a `Generated` file. `Generated` ownership means VibeConform owns the
  whole file, so this is drift correction, not a genuine conflict between
  two independent changes. Decision: `Overwrite`.
- `C == P`, `T != P` (row 7): the file on disk still matches what was last
  applied, and the target has simply moved on (e.g. the module's template
  changed). This is the textbook "safe replacement" case from
  `docs/architecture/overview.md`. Decision: `Overwrite`.
- Only when *both* current and target have diverged from previous, and
  they disagree with each other (row 8), is it a genuine `Conflict`.

`internal/reconcile`'s package doc records this table and its rationale so
the future `Overwrite`/`Conflict` handling in `vibe sync` (not this spec)
has a single source of truth to cite rather than re-deriving it.

## Explicit non-goals

- No `vibe sync` — this spec produces the decision engine and a read-only
  reporter. Writing files to disk, and writing `.vibe/state.yaml`, is
  follow-on work with its own spec: mutating a user's repository is a
  materially different risk profile than reading it.
- No writer for `.vibe/state.yaml` — `internal/state.Load` only reads. The
  file will never exist in practice until `vibe sync` lands, so `diff`'s
  output in this spec is effectively two-way (`Create`/`NoChange`/
  `Conflict` only; `Overwrite`'s rows never trigger without a state file to
  read). The full table and `Overwrite` decision are implemented now anyway
  so `vibe sync` doesn't need to change `reconcile.Decide`'s behavior later.
- No decision logic for `StructuredPatch`, `ManagedSection`, or
  `ProjectOwned` ownership — no module produces them yet.
- No change to `vibe audit` — spec 0004 already deferred "audit becomes
  strict" to once reconciliation exists; that is itself follow-on work
  from this spec, not part of it.
- No `--strict`/exit-code-on-conflict flag for `diff` — it is a preview
  tool, matching `git diff`'s default (non-gating) behavior.
- No locking/concurrency handling for `.vibe/state.yaml` reads (single
  short-lived CLI process, no concurrent writers exist yet).

## Design notes

- `internal/reconcile` depends on nothing beyond the standard library —
  `Decide` takes hashes/pointers, not `resource.Resource` or filesystem
  paths, keeping it a pure, exhaustively-table-tested function. Hashing and
  file I/O live in `internal/cli/diff.go`, mirroring how `runAudit` keeps
  I/O in the CLI package rather than the packages it composes.
- `internal/state` depends only on the standard library and a YAML
  decoder (already a dependency via `internal/manifest`); it does not
  depend on `internal/resource` or `internal/module`, preserving the
  dependency direction in `docs/architecture/overview.md` (state is a leaf
  package other packages read, not one that reaches back into them).
- `vibe diff` follows `init.go`/`audit.go`'s established shape: a
  `newDiffCmd` constructor plus a `runDiff(cmd *cobra.Command, repoRoot
  string) error` that writes to `cmd.OutOrStdout()`.
- `internal/cli/root_test.go`'s `TestSubcommandsNotYetImplemented` drops
  `diff` from its stub list, mirroring how spec 0004 moved `audit` out;
  `sync`/`check`/`doctor` remain stubs.

## Follow-on work

- `vibe sync`: writes `Create`/`Overwrite` decisions to disk and records
  the new content hash in `.vibe/state.yaml`; surfaces `Conflict` as a
  refusal-to-write with guidance, not a silent overwrite. Needs its own
  spec (write semantics, `--force`/override behavior, partial-failure
  handling across multiple resources).
- `vibe audit` v2: once `vibe sync` exists and `.vibe/state.yaml` is
  actually populated, `audit` can call `reconcile.Decide` per resource and
  exit non-zero on `Conflict` (spec 0004's deferred follow-on).
- `StructuredPatch`/`ManagedSection`/`ProjectOwned` decision logic, once a
  module that produces one of those ownership modes exists.
