# Spec 0019: Tell local drift apart from a moved standard

Status: accepted. Implementation plan:
`docs/plans/0019-drift-classification.md`.

Closes issue #22.

Decisions taken at approval:

- **Exit codes as recommended below**: `3` for out-of-date-only, `1` when
  the running binary is older than the one that wrote the state.
- **Option (c) for ordering**: stamp local builds *and* never claim a
  direction that cannot be established. See "The ordering problem", which
  has been revised since approval — the stamp format the recommendation
  assumed does not order correctly, and the corrected design is recorded
  there.
- **One spec, not two**: increments 1–4 land together rather than splitting
  the decision-engine fix from the provenance work.

## Problem

`vibe audit` prints `drifted (run vibe sync)` for two situations that are
not the same thing, and for a third that nobody has named yet.

`reconcile.Decide` compares three content hashes — P (recorded in
`.vibe/state.yaml`), C (the file on disk), T (what the module resolves to)
— and collapses two distinct rows onto one `Overwrite` decision:

| row | condition | what actually happened |
|---|---|---|
| 6 | `C != P`, `T == P` | Someone edited a managed file. The repository is wrong. |
| 7 | `C == P`, `T != P` | The repository is untouched. The *standard* moved. |

`internal/cli/audit.go`'s `auditLine` renders both as
`drifted (run vibe sync)`, counts both as `drifted`, and exits 2. So an
adopter who changed nothing sees a red required check telling them a file
they never touched has drifted. Because `github-ci-ts`/`github-ci-py`
install `vibe@latest` unpinned, this arrives on a run they did not trigger,
and spec 0013 forbids `vibe sync` in CI, so the job cannot self-heal.

It fires on every change to an embedded template, so it is a standing tax
on maintaining the standards. Spec 0018 is a worked example: it changed
`Taskfile.yml` in all three repo-tooling modules and `ci.yml` in
`ci/github`, so every existing adopter goes non-conformant on whatever
release carries it, with no action of their own.

### The information is already there

The two rows are already distinguishable from data VibeConform has today.
Within `Decide`'s `Overwrite` branch, **exactly one** of `C == P` and
`T == P` holds: if both held, then `C == T`, and `Decide` would have
returned `NoChange` two lines earlier. The engine computes the distinction
and then throws it away by mapping both to one enum value.

This matters for scope. Issue #22 proposes recording provenance in
`.vibe/state.yaml` to separate the cases. For the headline complaint —
"you edited this" versus "the standard moved" — no provenance, no schema
change, and no migration are needed. It is a decision-engine fix that works
on every existing state file, including every adopter's, the moment they
upgrade.

### The third case: a stale binary, and a silent revert

Provenance is still needed, for a case the issue does not name and which is
worse than a confusing message.

Row 7 says the standard moved. It does not say *which way*. A newer binary
whose templates have advanced produces row 7 — and so does an **older**
binary whose templates are behind. Hashes cannot be ordered, so `audit`
cannot tell "your repository is out of date" from "your binary is out of
date," and it reports both as drift.

Following its advice in the second case is destructive. Verified against a
scratch export of `main` at `e70c7c0`, using a `vibe@latest` binary
(`v0.2.0-alpha.1`, whose templates predate spec 0017):

```console
$ vibe audit --repo-root <scratch>      # stale binary, untouched repo
.github/workflows/ci.yml: drifted (run vibe sync)
Taskfile.yml: drifted (run vibe sync)
not conformant

$ vibe sync --repo-root <scratch>       # obeying that advice
```

The sync silently reverted, in one command:

- `audit` back to `go run ./cmd/vibe audit --repo-root .` — reintroducing
  issue #20, closed hours earlier.
- `- task: audit` back into `verify`'s `cmds` — reverting spec 0017.
- `build` and `run` back into the shipped Go template, and the
  `Taskfile.local.yml` include seam removed — reverting spec 0018.
- The `Install vibe` step removed from `.github/workflows/ci.yml`.

It then rewrote `.vibe/state.yaml` to bless the result, after which the
same stale binary reports:

```console
$ vibe audit --repo-root <scratch>
11 resources checked, 0 drifted, 0 conflicts
conformant
```

The repository is now *certified conformant* while carrying three specs'
worth of reverted content, and the two binaries confidently disagree about
the same tree. Committed, it would read as an intentional regeneration; the
commit message a maintainer would write is "ran vibe sync," which is true.

This is not hypothetical, and it is not exotic. A `vibe` left in `~/go/bin`
by an earlier `go install` wins the `PATH` lookup that spec 0018 introduced
for `task audit`, so the stale binary is the one a contributor gets by
default. It happened while verifying spec 0018.

The root cause of all three symptoms is one missing fact: **`.vibe/state.yaml`
records what the content was, never what produced it.**

## Scope

Four increments. 1 is self-contained and delivers the headline fix; 2 adds
the minimum provenance needed for the case hashes cannot answer.

### 1. Split the `Overwrite` decision

Replace `reconcile.Overwrite` with two values, so the distinction the
engine already computes survives to the caller:

- `reconcile.LocalDrift` — row 6 (`C != P`, `T == P`). The managed file was
  edited. The repository is wrong; `vibe sync` restores it.
- `reconcile.OutOfDate` — row 7 (`C == P`, `T != P`). The repository is
  exactly as VibeConform last wrote it; the standard moved.

`Create`, `NoChange`, and `Conflict` are unchanged. Removing `Overwrite`
rather than keeping it alongside a reason field is deliberate: it makes the
compiler enumerate every site that must now decide which case it means,
instead of leaving a default that silently keeps today's behaviour.

`audit` gains distinct messages:

```text
Taskfile.yml: drifted (edited since last sync; run vibe sync to restore)
Taskfile.yml: out of date (standard moved; run vibe sync to update)
```

`diff` and `sync` keep treating both as "write the target" — the action is
the same, only the explanation differs — so `internal/cli/plan.go`'s single
shared walk is preserved exactly as issue #22 requires.

No state schema change. No migration. Works on every existing
`.vibe/state.yaml`, including adopters', with no sync required first.

### 2. Record what wrote the state

Add top-level provenance to `.vibe/state.yaml`, written by `vibe sync`:

```yaml
schema: 2
vibe_version: v0.3.0
standard: prod-go/v1
resources:
  Taskfile.yml:
    sha256: ...
```

Top-level, not per-resource: `vibe sync` resolves and writes every resource
in the standard in one pass with one binary, so a per-resource writer
version would record the same value N times.

`audit` then reports, on an `OutOfDate` resource, which binary wrote the
state and which is running — turning an invisible mismatch into a visible
one — and, where the two versions are orderable and the running binary is
**older**, refuses to present the situation as repository drift at all.

Compatibility, both directions, is mandatory:

- **Reading a schema-1 file** (no `schema`, no `vibe_version`) must not
  fail and must not guess. Unknown provenance means no direction claim:
  increment 1's classification still applies in full, and the extra
  reporting is simply absent. One `vibe sync` after upgrading populates it.
- **Old binaries reading a schema-2 file** already work. `state.Load` uses
  `yaml.Unmarshal` without `KnownFields`, so unknown top-level keys are
  ignored. Verified: a `v0.2.0-alpha.1`-era binary reads a state file
  carrying `schema`, `vibe_version`, and `standard` and reports
  `conformant` unaffected.

### 3. Exit codes — a policy decision to take explicitly

Issue #22 flags this as a genuine call rather than an implementation
detail, so it is raised here rather than settled silently. Current
behaviour: 0 conformant, 1 could-not-answer, 2 drift or conflict.

Recommended:

| situation | exit | rationale |
|---|---|---|
| conformant | 0 | unchanged |
| any `LocalDrift` or `Conflict` | 2 | unchanged; someone edited a managed file |
| `OutOfDate` only | **3** | still non-zero, so `gate` stays red by default — the repository *is* out of date — but distinguishable, so a `conformance` job that wants to tolerate it can |
| running binary older than the one that wrote the state | **1** | `audit` cannot judge conformance against a standard definition older than the repository's. The repository is probably fine; claiming non-conformance would be the same false accusation in a new costume |

The last row is the substantive change: a stale binary currently produces a
confident, wrong, and actionable-in-the-wrong-direction verdict.

### 4. Refuse to sync backwards

`vibe sync`, when the running binary is orderably older than the one that
wrote the state, must refuse rather than revert, and say why:

```text
Error: sync: this binary (v0.2.0-alpha.1) is older than the one that last
synced this repository (v0.3.0). Syncing would revert managed files to
older templates. Upgrade vibe, or pass --allow-downgrade if you mean it.
```

An escape hatch is needed — deliberately reverting is legitimate — but it
must be typed, not stumbled into.

## The ordering problem, stated plainly

Increments 2–4 rest on comparing two version strings, and one of them is
frequently not a version. `cmd/vibe/main.go` sets `var version = "dev"`,
overridden by ldflags only in a GoReleaser build. So every locally built
binary — including every one this repository's own contributors use, and
the one `task build` produces — reports `dev`, which cannot be ordered
against anything.

Consequences, honestly: the downgrade guard protects the case where both
versions are released semver (an adopter pinning an old `vibe`, or
`@latest` lagging a fast-moving standard). It does **not** protect a
contributor here whose `~/go/bin/vibe` is a stale `go install` of a
release, if their working binary reports `dev` — the exact case that
prompted this spec.

Option (c) is approved: stamp local builds, and never claim a direction
that cannot be established. The *format* of the stamp, however, is not what
the recommendation assumed. Measured against `golang.org/x/mod/semver`:

```text
v0.2.0-alpha.1-42-gAAAAAAA  vs  v0.2.0-alpha.1-9-gBBBBBBB   -> -1
v0.2.1-0.20260923031702-1c7666bdead1  vs  v0.2.0-alpha.1    -> +1
v0.2.1-0.20260923031702-1c7666bdead1
                    vs  v0.2.1-0.20260101000000-aaaaaaaaaa  -> +1
"dev"  ->  IsValid == false;  Compare("dev", anything) == -1
```

Two corrections follow, and both are load-bearing:

**`git describe` output does not order correctly.** Semver compares
prerelease identifiers lexically, so `-42-g…` sorts *below* `-9-g…`: a
build 42 commits past the tag is reported as older than one 9 commits past
it. Stamping raw `git describe` would not merely fail to help — it would
produce confidently wrong direction claims, which is worse than none.

**The stamp is a Go-style pseudo-version instead**:
`vX.Y.(Z+1)-0.<commit-time-UTC>-<12-char-sha>`. It is a defined, orderable
format, it sorts above the release it descends from, and two of them sort
by commit time. That gives exactly the protection the motivating case
needs: a contributor's locally built binary outranks a stale
`~/go/bin` release, so running the stale one against state written by the
local build is detected as a downgrade.

**Validity must be checked before comparing, not inferred from the
result.** `semver.Compare` returns `-1` for invalid input, so comparing a
`dev` version against a release yields "older" — silently branding every
unstamped local binary stale. Every comparison site must gate on
`semver.IsValid` for *both* operands first, and treat "not both valid" as
unorderable. A regression test must pin this specific trap.

The resulting rule:

| recorded | running | outcome |
|---|---|---|
| absent (schema 1) | any | unorderable — no direction claim |
| either invalid (`dev`) | — | unorderable — no direction claim |
| either `v0.0.0-<ts>-<rev>` | — | unorderable — no tag was visible to the build |
| both valid, running > recorded | | repository out of date; `sync` proceeds |
| both valid, equal | | templates differ within one version — unorderable in the direction sense; report both, no claim |
| both valid, running < recorded | | **stale binary**; `audit` exits 1, `sync` refuses without `--allow-downgrade` |

"Unorderable" is never an error and never blocks: it reports both versions
and falls through to increment 1's classification, which needs no
provenance at all.

The `v0.0.0-…` row was added after CI rejected this spec's own pull
request. `actions/checkout` fetches depth 1 and no tags, so a binary built
in CI from the newest possible source has Go derive `v0.0.0-<ts>-<rev>` —
there is a commit to name but no tag to build on. That sorts below every
real tag, so the guard declared the freshest binary stale and refused to
audit: the same false accusation this spec exists to remove, reappearing
one layer down. A `v0.0.0-` pseudo-version means *no version information
was available*, not *version zero*, and must not be ordered. Binaries with
real version information are unaffected — a release reports its tag,
`go install` reports the module version it resolved, and the
`Taskfile.local.yml` stamp bases its pseudo-version on the newest tag.

## A new production dependency: `golang.org/x/mod/semver`

Ordering versions correctly requires a semver comparator, and this
repository has none — `go.mod` requires only `cobra` and `yaml.v3`.

Hand-rolling one is the obvious alternative and is rejected. The evidence
above is precisely that semver precedence is subtle enough to surprise:
prerelease identifiers compare lexically, numeric and alphanumeric
identifiers rank differently, and an invalid input returns a valid-looking
answer. A hand-rolled comparator would be ~60 lines implementing exactly
the rules that just produced a backwards result in testing, guarding a
feature whose entire job is to prevent a destructive revert.

`golang.org/x/mod/semver` is maintained by the Go team and its build
footprint is stdlib only — `go list -deps golang.org/x/mod/semver` resolves
to `slices`, `io`, and `strings`. (`go list -m all` also shows
`golang.org/x/tools` in the module graph; it is a graph entry of `x/mod`,
not a build dependency of the `semver` package.)

Per `AGENTS.md`, a production dependency needs its rationale recorded:
**`docs/decisions/0010-semver-comparison-dependency.md`**, short, covering
the footprint, the rejected alternative, and the fact that `x/mod` is the
only new module in `go.mod`.

## Acceptance criteria

- `reconcile.Decide` returns `LocalDrift` and `OutOfDate` in place of
  `Overwrite`, with the truth table and rationale updated in the package
  doc and exhaustively table-tested, including that exactly one of the two
  can hold.
- `audit` prints distinct messages, and an adopter with an untouched
  repository and a newer binary sees `out of date`, never `drifted`.
- `diff` and `sync` still act identically on both, through the one shared
  plan walk in `internal/cli/plan.go`. No forked logic.
- A schema-1 `.vibe/state.yaml` loads without error, and classification is
  fully correct for it — proven against the three committed state files in
  this repository before any sync.
- A schema-2 file is read without error by a pre-0019 binary. Verified by
  observation, not assumed.
- `vibe sync` refuses to revert when the running binary is orderably older,
  and `--allow-downgrade` overrides it.
- Version comparison gates on `semver.IsValid` for both operands. A `dev`
  binary against a released recorded version is reported as
  **unorderable**, never as stale — pinned by a regression test written
  specifically against `Compare`'s `-1`-for-invalid behaviour, since that
  is the failure this design would otherwise walk into.
- `task build` stamps a Go-style pseudo-version, and a binary so built
  compares **greater** than the release it descends from. Verified by
  building and comparing, not by reading the format.
- `golang.org/x/mod` is the only module added to `go.mod`, recorded in
  ADR 0010.
- Regression tests, each watched failing first: one pinning that row 6 and
  row 7 classify differently; one pinning that a schema-1 state file still
  classifies correctly; one pinning that sync refuses a backwards write;
  one pinning the `IsValid` trap above.
- `task verify` and `vibe audit` on all three roots against a freshly
  rebuilt binary.

## Explicit non-goals

- **No versioning of standards on content change.** Issue #22 rejects
  forcing `prod-go/v2` for every template edit; not reopened.
- **No `vibe sync` in CI.** Spec 0013 stands. This is better reporting, not
  auto-repair.
- **No pinning of `vibe` in the CI templates.** Issue #22 raises it as a
  possible split-out; it would reduce how often adopters meet `OutOfDate`
  but does nothing about the ambiguity, and belongs in its own change.
- **No per-resource provenance, no template digests.** A top-level writer
  version answers the one question hashes cannot. Per-resource template
  revisions would be recorded but unread.
- **No timestamps in `.vibe/state.yaml`.** A `synced_at` field would change
  on every sync, churning the file and its diff for no reconciliation
  benefit. State stays content-derived and deterministic.
- **No change to `Conflict` handling**, and no decision logic for
  `StructuredPatch`/`ManagedSection`/`ProjectOwned` — still no module
  produces them.

## Design notes

- Increment 1 is worth landing even if 2–4 are deferred: it is the fix for
  the reported issue, it needs no migration, and it helps every existing
  adopter on upgrade. Increments 2–4 address the case found while verifying
  spec 0018, which is rarer but destructive rather than merely confusing.
- The `Decision` enum is internal, so splitting it is a compile-time
  change with no external contract. `golangci-lint`'s exhaustiveness check
  should be relied on to find every switch.
- `audit`'s summary line (`N resources checked, N drifted, N conflicts`) is
  parsed by nothing in this repository, but changing its shape is still an
  output-contract change; the plan should decide whether to add an
  `out of date` count to it or leave the line alone.
- Recording `standard: prod-go/v1` in state alongside the writer version is
  cheap and makes a mismatch between `vibe.yaml` and what was last applied
  detectable later. It is written but not yet read; the plan should say so
  explicitly rather than implying a check exists.

## Related, not in scope

Spec 0018's verification surfaced a second, smaller hazard worth fixing
separately: on Windows, `go build -o bin/vibe` produces an extensionless
file that `PATH` lookup will not find as `vibe`, so putting `bin/` on
`PATH` silently falls through to whatever else is installed — which is how
the stale `~/go/bin` binary got used in the first place. That is a
one-line fix in this repository's own `Taskfile.local.yml` (build to
`vibe.exe` on Windows) and needs no spec.
