# Plan 0019: Tell local drift apart from a moved standard

See `docs/specs/0019-drift-classification.md` for the accepted scope.
Sequenced after spec 0018, whose verification surfaced the stale-binary
case this spec exists to make safe. Closes issue #22.

Approved at spec review: exit `3` for out-of-date-only and `1` for a stale
binary; ordering option (c); increments 1–4 in one spec.

## Order of work

Five commits. Increment 1 stands alone and is deliberately first — it is
the fix for the reported issue, needs no dependency and no schema change,
and would still be worth merging if everything after it stalled.

1. **C1 — split the decision.** `internal/reconcile` + every caller.
   No new dependency, no state change, no docs yet.
2. **C2 — ADR 0010 and the dependency.** `golang.org/x/mod` added to
   `go.mod`, ADR recorded, plus the `semver` helper and its tests
   (including the `IsValid` trap). No behaviour wired up yet.
3. **C3 — provenance.** State schema 2, written by `sync`, read by
   `audit`/`diff`. Compatibility both directions.
4. **C4 — guards.** Exit codes 3 and 1; `sync` refusing a backwards write;
   `--allow-downgrade`. Plus the `task build` pseudo-version stamp.
5. **C5 — docs.** `docs/usage.md`, `docs/architecture/overview.md`,
   `README.md` if anything became false.

C1 through C4 each leave the tree green. C2 before C3 so the dependency
and its ADR are reviewable on their own, rather than buried in a schema
change.

## Checklist

### C1 — split the `Overwrite` decision

- [x] `internal/reconcile/reconcile.go`: replace `Overwrite` with
      `LocalDrift` (`C != P`, `T == P`) and `OutOfDate` (`C == P`,
      `T != P`). Update `String()`.
- [x] Update the package doc's truth table and rationale. Record the
      property that makes the split sound: within the former `Overwrite`
      branch exactly one of the two conditions can hold, because if both
      held then `C == T` and `Decide` would have returned `NoChange`
      earlier. That sentence is the whole justification; write it down.
- [x] `internal/cli/audit.go`: distinct `auditLine` messages —
      `drifted (edited since last sync; run vibe sync to restore)` and
      `out of date (standard moved; run vibe sync to update)`.
- [x] `internal/cli/audit.go`: count `OutOfDate` separately from
      `drifted`. Decide the summary line's shape — spec 0019 flags this as
      an output-contract change. Proposal: `N resources checked, N drifted,
      N out of date, N conflicts`, always printing all four so the line's
      arity is stable for anything parsing it.
- [x] `internal/cli/diff.go` and `internal/cli/sync.go`: handle both values
      where `Overwrite` was handled. Both still mean "write the target" —
      the action is identical, only the explanation differs. Do **not**
      fork the plan walk; issue #22 names that as a thing to preserve.
- [x] Check whether `.golangci.yml` enables an exhaustiveness linter. If
      it does, every switch is found for free; if not, grep for
      `reconcile.` across `internal/` and confirm by hand that no site was
      missed.
- [x] Table-test `Decide` exhaustively over all eight rows, asserting the
      two former-`Overwrite` rows now return different values.

### C2 — ADR 0010 and the semver dependency

- [x] `go get golang.org/x/mod`. Confirm it is the **only** module added to
      `go.mod`'s `require` blocks, and that `go list -deps
      golang.org/x/mod/semver` resolves to stdlib only.
- [x] Write `docs/decisions/0010-semver-comparison-dependency.md` in house
      format: the need, the measured footprint, the rejected hand-rolled
      alternative and why (the backwards `-42-g…` result is the argument —
      quote it), and that ordering is only ever consulted when both
      operands are valid.
- [x] Add a small internal helper — location to settle in implementation,
      likely `internal/state` or a new leaf — exposing something like
      `CompareWriters(recorded, running string) (order int, ok bool)`,
      where `ok` is false unless **both** are `semver.IsValid`. Callers get
      no way to accidentally use `Compare`'s `-1`-for-invalid.
- [x] **Regression test for the trap, watched failing first**: assert that
      `dev` against a released version returns `ok == false`, and write it
      against a deliberately naive implementation that calls
      `semver.Compare` directly, to confirm the test catches it. A test
      that cannot fail is worse than none.
- [x] Table-test ordering: pseudo-version > the release it descends from;
      two pseudo-versions by commit time; equal versions; both-invalid.

### C3 — provenance in `.vibe/state.yaml`

- [x] `internal/state`: add top-level `Schema int`, `VibeVersion string`,
      `Standard string`. `Load` must accept a file with none of them
      (schema 1) without error and without inventing values.
- [x] `Save` writes `schema: 2`, the running binary's version, and
      `<standard>/<version>`. Field order in the marshalled file should be
      stable so state diffs stay readable.
- [x] Thread the running version into `Save`'s callers. `cmd/vibe/main.go`
      already holds it (`var version = "dev"`, ldflags-overridden) and
      passes it to `cli.NewRootCmd`; it currently goes no further than
      cobra's `Version` field.
- [x] **No timestamp field.** Spec 0019 rules it out: it would churn the
      file on every sync.
- [x] `standard` is written but not yet read. Say so in the field's doc
      comment, so nobody assumes a check exists.
- [x] Backward compatibility test: the three committed state files in this
      repository (root, `examples/typescript`, `examples/python`) are
      schema 1 today. Assert `Load` handles a schema-1 fixture and that
      classification is unaffected — run the full audit against all three
      **before** any sync rewrites them.
- [x] Forward compatibility, verified by observation: a pre-0019 binary
      reads a schema-2 file unaffected. Already confirmed once by hand
      against a `v0.2.0-alpha.1` binary; redo it against the real
      schema-2 output rather than a hand-edited approximation.

### C4 — exit codes, guards, and the build stamp

- [x] `internal/cli/exit.go`: exit `3` when the only findings are
      `OutOfDate` (no `LocalDrift`, no `Conflict`). Mixed findings keep
      exit `2` — the more serious wins.
- [x] `audit`: when the running binary is orderably **older** than the
      recorded writer, do not report repository non-conformance. Exit `1`
      (could-not-answer) with a message naming both versions.
- [x] `audit`: on any `OutOfDate` resource where provenance exists, print
      the recorded and running versions, whether or not they order. Making
      the mismatch visible is the part that works in every case.
- [x] `sync`: refuse a backwards write, with the message drafted in spec
      0019's increment 4. Add `--allow-downgrade` to override.
- [x] Regression test, watched failing: sync refuses when running < recorded,
      and proceeds with `--allow-downgrade`.
- [x] **Decide the churn question before stamping.** As of C3 every local
      build reports `dev`, so `vibe_version` is identical for everyone and
      the field never changes. Stamping makes it per-build: two
      contributors, or one contributor across two commits, write different
      pseudo-versions, so any `vibe sync` rewrites the line even when no
      managed content changed — a committed diff with no substance, and a
      merge-conflict surface on a file three roots share.

      **Decided: accept it.** The churn is provenance doing its job, and it
      mostly rides along with commits that already rewrite `state.yaml`.
      Observed consequence worth knowing: a binary built mid-change stamps
      `+dirty`, so the committed provenance usually carries it. That is
      accurate rather than untidy, and semver ignores build metadata in
      precedence, so it does not affect ordering. Fallback if the noise
      proves annoying is to write `vibe_version` only when something else
      in the state changed.

      Mostly it coincides with commits that already rewrite `state.yaml`
      (a template change moves the hashes anyway), so the marginal cost is
      one line. The exception is a no-op sync. Options: accept it; write
      `vibe_version` only when something else in the state actually
      changed; or leave local builds unstamped and accept no local
      ordering. Raise it rather than picking silently — it trades the
      protection C4 exists for against diff noise in a committed file.
- [x] `Taskfile.local.yml` (this repository's own, project-owned): stamp
      `task build` with a Go-style pseudo-version,
      `vX.Y.(Z+1)-0.<commit-time-UTC>-<12-char-sha>`, derived from git.
      Handle the no-tag and dirty-tree cases so the task never fails on a
      fresh clone.
- [x] Verify by building: a stamped local binary compares **greater** than
      the release it descends from, so the motivating case — a stale
      `~/go/bin` release run against state written by a local build — is
      detected. Check the format by running the comparison, not by reading
      it.

### C5 — documentation

- [x] `docs/usage.md`, `vibe audit` section: document the three outcomes
      (`drifted`, `out of date`, `conflict`), the exit codes including the
      new `3` and the stale-binary `1`, and what each one means for a
      reader who did not change anything.
- [x] `docs/usage.md`: document `.vibe/state.yaml` schema 2 and that a
      schema-1 file keeps working until the next sync.
- [x] `docs/usage.md`: the stale-`~/go/bin` warning added in spec 0018 can
      now point at the real guard rather than only warning.
- [x] `docs/architecture/overview.md`: update the three-way reconciliation
      section, which still states the collapsed model
      (`current == previous` → safe replacement).
- [x] `docs/specs/0009-vibe-audit-v2.md` — it is accepted-and-implemented,
      and its output examples and decision table (`| Overwrite | drifted
      (run vibe sync) |`) are now superseded. Do **not** rewrite it; it is
      a record of what was decided then. Add a short forward-pointer to
      spec 0019 at the top, the way an amended decision record should read.
- [x] `README.md` — review; expected no change beyond the `vibe audit`
      one-liner's exit-code parenthetical if it overstates.

## Verification

Recorded as run, with observed output, not as intent.

### The guard caught a bug in its own tooling

Worth recording because it was not staged. After C5's doc pass, `vibe
audit` refused to judge this repository:

```
Error: audit: this vibe (v0.2.0-alpha.1.0.20260923051543-c4cd4c284bd2+dirty)
is older than the one that last synced this repository
(v0.2.0-alpha.1.0.20260923120009-f2c001260e4b+dirty)
```

`c4cd4c2` is the child of `f2c0012`, so a later commit had stamped an
earlier timestamp. Cause: the `task build` stamp used git's
`--date=format-local`, which renders in whatever `TZ` the shell carries.
One build rendered local time (`120009`, UTC+08:00) and a later one
rendered UTC (`051543`), so the two were not comparable. Go pseudo-version
timestamps are UTC by definition.

Fixed by forcing `TZ=UTC` in the stamp; verified the output is now
identical under `TZ=Asia/Tokyo` and `TZ=America/New_York`. Resetting the
recorded provenance to a UTC-based stamp needed one deliberate
`--allow-downgrade` sync — the first real use of the escape hatch, and the
case it exists for.

Two things this demonstrates that a unit test could not: the guard fires on
a genuine inconsistency nobody constructed, and refusing rather than
reverting left the repository untouched while the tooling was fixed.

- [ ] Every regression test above watched failing before it is relied on;
      outputs pasted here.
- [ ] `task verify` green at the repository root.
- [ ] `vibe audit` conformant on all three roots against a freshly rebuilt
      binary, **before** and **after** the first schema-2 sync.
- [ ] The full scenario from spec 0019's problem statement, replayed on a
      scratch export: a stale binary against an untouched tree must now
      report something other than `drifted`, and `sync` must refuse to
      revert. This is the whole point of the spec; demonstrate it end to
      end rather than trusting unit tests.
- [ ] An adopter-shaped scenario: untouched repository, newer binary,
      changed template → `out of date`, exit 3, message names no fault of
      the user's.
- [ ] `go.mod`/`go.sum` diff reviewed: `golang.org/x/mod` only.

## Explicitly still deferred

- **Pinning `vibe` in the CI templates.** Would reduce how often adopters
  meet `OutOfDate`; does nothing about the ambiguity. Its own change.
- **Per-resource template digests.** A top-level writer version answers the
  question hashes cannot; per-resource provenance would be written and
  never read.
- **Reading the recorded `standard`.** Written in C3, unused. A mismatch
  between it and `vibe.yaml` is a real condition worth detecting later.
- **`StructuredPatch`/`ManagedSection`/`ProjectOwned` decisions** — still
  no module produces them.
- **The Windows `bin/vibe` extension fix** — one line in
  `Taskfile.local.yml`, unrelated to this spec's logic. Fold it into C4's
  Taskfile commit only if it stays a one-liner; otherwise leave it.
