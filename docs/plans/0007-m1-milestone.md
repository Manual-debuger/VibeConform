# Plan 0007: M1 — closing the reconciliation loop (milestone)

Milestone plan, not a feature plan. It defines what M1 means, sequences the
spec/plan pairs that deliver it, and records the cross-cutting risks that
only show up when the increments are viewed together. Each increment below
still gets its own `docs/specs/NNNN-*.md` + `docs/plans/NNNN-*.md` approved
before implementation, per `AGENTS.md`. Numbers 0008–0013 are reserved here
so the series stays predictable.

## What M1 is

`docs/specs/0001-v0-control-plane.md` scopes M1 as: *once `init`, `audit`,
`diff`, `sync` have real implementations against a manifest + standard +
resolver, migrate this repository's manually managed `.github/`, `.claude/`,
`.codex/`, `Taskfile.yml`, `lefthook.yml` under VibeConform.* That is steps
2–4 of `docs/plans/0001-bootstrap.md`'s sequencing: build the tool, migrate
this repo onto it, become the first dogfood repository.

## Where M0 left off

| Piece | State |
|---|---|
| `vibe init` | implemented (spec 0002) |
| `vibe audit` | implemented, reports a module count only (spec 0004) |
| `vibe diff` | implemented, `Generated` only, read-only (spec 0006) |
| `vibe sync` / `check` / `doctor` | stubs returning `errNotImplemented` |
| `internal/reconcile` | full truth table, pure `Decide` (spec 0006) |
| `internal/state` | reader only; nothing writes `.vibe/state.yaml` |
| modules in `production/v1` | one — `go-tooling` → `.golangci.yml` |
| this repo's own guardrails | hand-maintained, not managed by `vibe` |

The decision engine is done; the loop is open at both ends — nothing writes,
and nothing gates.

## Status: complete

All four exit criteria below are met. Delivered as 0008 (`sync`), 0009
(strict `audit`), 0010–0012 (the `github-ci`, `repo-tooling`, and
`agent-config` modules), and 0013 (dogfood). The acceptance test held: with
templates seeded from files hand-written months earlier, the first `vibe
sync` on this repository reported `0 created, 0 updated, 11 unchanged, 0
conflicts`.

Two things were found during implementation that the plan below did not
anticipate:

- **A CRLF/`go:embed` defect.** `go:embed` reads the working copy at build
  time, and 27 tracked files had gone stale as CRLF while their blobs stayed
  LF — `core.autocrlf=true` hid it from `git status`. Binaries built on
  Windows embedded different bytes than binaries built in CI, which for a
  content-hashing tool is a correctness bug. Fixed, and each module now
  asserts its templates contain no CR.
- **Adoption order is the reverse of what plan 0013 specified.** See that
  plan's correction note: with no prior state, editing templates before the
  first sync produces conflicts the tool refuses to resolve.

## Exit criteria

M1 is done when all four hold:

1. `vibe sync` writes `Generated` resources, records their hashes in
   `.vibe/state.yaml`, and refuses to overwrite a `Conflict`.
2. `vibe audit` is a strict, CI-usable gate: exit non-zero when the
   repository is not conformant.
3. `production/v1` composes modules covering this repository's manually
   managed guardrails: `.golangci.yml`, `.github/`, `Taskfile.yml`,
   `lefthook.yml`, `.codex/`, `.claude/`.
4. This repository has `vibe.yaml` and `.vibe/state.yaml` committed, and its
   own CI runs `vibe audit` against itself.

## Sequence

### 0008 — `vibe sync` v1 + state writer

First, because nothing else in M1 is reachable without a writer: strict
audit with no way to fix drift is a dead end, and every later module is
unverifiable until `sync` can apply it.

- `internal/state.Save(repoRoot string, s *State) error` — creates `.vibe/`,
  writes atomically (temp file + rename) so an interrupted run can't leave a
  truncated state file.
- Extract the resolve → hash → `reconcile.Decide` walk currently inline in
  `internal/cli/diff.go` into one helper both `diff` and `sync` call. Two
  copies of that walk will drift; `diff` must be exactly the preview of what
  `sync` does.
- Apply semantics to pin down in the spec: `Create`/`Overwrite` → write;
  `NoChange` → skip the write but still record the hash; `Conflict` → do not
  touch the file, report it, and exit non-zero once every other resource has
  been processed.
- `os.MkdirAll` for nested resource paths — no module needs it yet, 0010
  does.
- Partial failure: define whether state is saved for the resources that
  succeeded before the error. Recommendation: yes, save, then return the
  error — a half-applied repo that remembers what it applied is recoverable;
  one that forgets is not.
- Open questions for the spec: file mode for written resources (`init` uses
  `0o600`); whether `--force` exists in v1 (recommend no — deleting the file
  is the escape hatch, and `--force` deserves its own spec).

### 0009 — `vibe audit` v2 (strict gate)

Spec 0004 explicitly deferred "audit becomes strict" until reconciliation
existed; 0008 finishes that prerequisite.

- Reuse 0008's shared walk; report a decision per resource instead of a bare
  module count.
- Conformance rule: conformant iff every resource decides `NoChange`.
- Exit-code policy must be explicit and documented — including how a drift
  exit is distinguished from a usage/IO error, since cobra returns 1 for
  both today.
- Retires the "N modules configured, nothing to check" string and the
  pluralization nit from spec 0005's follow-on list.

### 0010 — GitHub CI module

First module with multiple resources and nested paths — the shape the
remaining modules reuse.

- `.github/workflows/ci.yml`, `.github/workflows/release.yml`,
  `.github/dependabot.yml` as `Generated` resources, templates seeded
  **verbatim** from the files in this repo today.
- Resource paths are repository-relative and slash-separated; they are also
  `.vibe/state.yaml` keys. Normalize on `/` and join with `filepath.Join`
  only at the I/O boundary, or Windows and Linux will produce different
  state files for the same repository.
- Output ordering must be deterministic across modules and resources, or
  `audit`/`diff` output churns between runs.

### 0011 — repo-tooling module

`Taskfile.yml`, `lefthook.yml`. Same shape as 0010, no new mechanics —
small on purpose, and it can land in parallel with 0012 once 0010 sets the
pattern.

### 0012 — agent-config module

`.codex/config.toml`, `.codex/hooks.json`, `.claude/settings.json`.

- `.claude/settings.local.json` is gitignored user-local config and stays
  `ProjectOwned` — never written.
- New mechanic: `.claude/hooks/*.sh` need the executable bit, and
  `resource.Resource` has no file-mode field. Either add one (ADR-worthy:
  it changes a core type) or defer the hook scripts to a later increment and
  manage only the JSON/TOML here. Decide in the spec, not during
  implementation.
- These files encode the guardrails that constrain the agents working in
  this repo. Review the generated output against the current files by hand
  before merging; a sync that quietly relaxes a block pattern is the worst
  failure mode in this milestone.

### 0013 — dogfood this repository

The closing move: steps 3–4 of plan 0001's sequencing.

- `vibe.yaml` declaring `production/v1`, committed.
- Run `vibe sync`; commit the resulting `.vibe/state.yaml`. The diff at this
  point should be all `NoChange` — if it isn't, a template drifted from the
  file it was seeded from, and that difference is the review.
- Wire `vibe audit` into `Taskfile.yml`'s `verify` and into CI **after**
  `go build`, in two steps: non-gating first (one merged PR observing real
  output), then gating. A gating self-audit introduced in the same PR as the
  module it validates can lock the repo out of its own CI.

### Housekeeping (fold into whichever PR touches the file)

- `docs/specs/0004`, `0005`, `0006` still read `Status: proposed` though all
  three are implemented and merged. Update to `accepted and implemented`,
  matching 0002/0003.
- `docs/plans/0001-bootstrap.md`'s "Explicitly deferred to M1+" list should
  point here for what M1 actually absorbed.
- `internal/cli/commands.go`'s `errNotImplemented` cites
  `docs/plans/0001-bootstrap.md`; re-point it as commands graduate.

## Explicitly deferred past M1

Carried from `docs/plans/0001-bootstrap.md`'s "deferred to M1+" list — these
are M2 or later, not part of this milestone.

M2 (`docs/plans/0014-m2-milestone.md`) absorbed none of the items below. It
took hook registration, tool-availability warnings, and TypeScript/Python
support instead, and explicitly deferred `vibe check`, `vibe doctor`, the
affected-component graph, and multi-component repositories again.

- Affected-component graph and `vibe check`.
- `vibe doctor`.
- `.vibe/lock.yaml` (state alone is enough to close the loop; the lock file
  earns its place when standards resolve dynamically).
- `StructuredPatch` / `ManagedSection` decision and apply logic — still no
  module that produces them, unless 0012 forces the question.
- GitNexus and Skills Manager provider adapters.
- `vibe new` / `vibe eject`.
- `vibe.yaml` overrides, component graph, and dynamic/config-driven standard
  loading. `production/v1` stays a static Go-registered composition through
  M1.

## Cross-cutting risks

- **Self-management bootstrap.** From 0010 on, VibeConform generates the
  files that verify VibeConform. Mitigation is structural, not procedural:
  seed every template verbatim from the live file, so the first sync is a
  no-op, and keep CI non-gating until one PR has observed real audit output.
- **Silent template drift.** Between 0010 and 0013 each managed file exists
  twice — live, and embedded in a module. This is already true for
  `.golangci.yml` today. It is tolerable only because 0013 closes it; if the
  milestone stalls, that duplication is the first thing to reconcile.
- **Line endings.** `.gitattributes` pins `* text=auto eol=lf`, so content
  hashes stay stable across Windows checkouts. `sync` must write LF
  explicitly rather than inheriting a platform default, or every Windows
  run reports drift.
- **Conflict UX.** M1 ships `Conflict` as a hard stop with no override. If
  that proves unworkable during 0013, the answer is a spec for `--force`,
  not a quiet loosening of `reconcile.Decide`.

## Verification

Unchanged: `task verify` clean at every increment, CI reproducing it
independently. From 0013, `vibe audit` against this repository joins that
set.
