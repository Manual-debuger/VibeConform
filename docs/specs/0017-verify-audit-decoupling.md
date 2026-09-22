# Spec 0017: Decouple `task verify` from `vibe audit`

Status: accepted and implemented.

## Problem

`task verify`/`task verify-ci` are meant to be a generated repo's pure
language-verification entrypoint — fmt, lint, typecheck, test, and (for Go)
security/mod-verify checks. In practice they are not independent of
VibeConform: in all three per-language repo-tooling Taskfile templates,
`verify`'s step list includes `task: audit`, and `audit` shells out to
`vibe audit --repo-root .`:

- `internal/module/repotooling/templates/Taskfile.yml` (Go) — self-hosted
  form, `go run ./cmd/vibe audit --repo-root .`. This only works because
  *this* repository happens to vendor `cmd/vibe`; any other repository
  declaring `prod-go/v1` has no such package.
- `internal/module/tsrepotooling/templates/Taskfile.yml` (TS) and
  `internal/module/pyrepotooling/templates/Taskfile.yml` (PY) — bare
  `vibe audit --repo-root .`, assuming a `vibe` binary is resolvable on
  `PATH`.

The consequence is visible today in `.github/workflows/examples.yml`: its
`typescript` and `python` jobs build `vibe` from source and prepend it to
`$GITHUB_PATH` before running `task verify`, solely so the transitive
`audit` step doesn't fail — not because verifying TypeScript or Python
source has anything to do with VibeConform. A `task verify` that requires
a `vibe` binary to succeed is not "native tooling verification"; it is
VibeConform's own conformance check wearing `verify`'s name.

Separately, adopters' generated CI (`internal/module/ci/github/templates/
ci.yml` and its `githubts`/`githubpy` counterparts) already runs `task
audit` as its own `conformance` job — structurally distinct from the
language job already — but nothing documents that this job is a VibeConform-
specific, optional, and separable piece of the pipeline, nor how a
repository that stops using VibeConform would remove it cleanly.

Finally, `vibe check` — a future command to accelerate validation by
running only the checks affected by a change set — has been referenced as
a placeholder across `README.md`, `docs/usage.md`, and
`docs/architecture/overview.md` since M0, with no behavioral contract
beyond "not implemented yet." The machinery it would need
(`internal/affected`, `internal/validation`) does not exist. Before that
machinery is built, its safe-fallback behavior should be specified, so the
eventual implementation is constrained correctly from day one rather than
retrofitted after a partial-validation bug ships.

## Scope

Three increments.

### 1. Remove `audit` from `verify`'s dependency chain

In each of the three repo-tooling Taskfile templates:

- `internal/module/repotooling/templates/Taskfile.yml`
- `internal/module/tsrepotooling/templates/Taskfile.yml`
- `internal/module/pyrepotooling/templates/Taskfile.yml`

Remove the `task: audit` step from `verify`'s `cmds` list. `audit` remains
defined exactly as today and remains independently runnable via
`task audit`; nothing about `audit`'s own behavior changes. `verify-ci`
keeps aliasing `verify` (`cmds: [task: verify]`), so no separate edit is
needed there — removing `audit` from `verify` removes it from `verify-ci`
for free.

Effect: `task verify`/`task verify-ci` in a generated repository now only
exercises native language tooling (fmt:check, lint, typecheck, test,
security, mod:verify, workflows:lint — whichever apply per language) and
has zero transitive dependency on a `vibe` binary or, for Go, on
`cmd/vibe`.

Per `AGENTS.md`, `Taskfile.yml` is itself a managed resource
(`resource.Generated`), resolved via `go:embed` at build time. Landing this
requires, together in one change: edit the three templates, rebuild the
`vibe` binary, run `vibe sync` against this repository, `examples/
typescript`, and `examples/python`, and commit the regenerated
`Taskfile.yml` files alongside the updated `.vibe/state.yaml` hashes (the
same sequencing spec 0016's increments already followed).

### 2. Make the CI conformance job an explicit, separable integration

The `conformance` job already exists as a distinct job (not a step folded
into the language job) in:

- `internal/module/ci/github/templates/ci.yml` (Go)
- `internal/module/ci/githubts/templates/ci.yml` (TS)
- `internal/module/ci/githubpy/templates/ci.yml` (PY)

Structurally it is already separable — `gate.needs` lists it alongside the
language jobs rather than nesting it inside one. What's missing is making
that separability legible and documented:

- Add a comment on each template's `conformance:` job identifying it as
  VibeConform's own conformance check, independent of language
  verification, and safe to delete (along with its `gate.needs` entry) if
  the repository stops using VibeConform. Exact wording decided in the
  implementation plan.
- Add a "Removing VibeConform" section to `docs/usage.md` enumerating the
  concrete teardown steps for an adopter: delete `.vibe/`, delete
  `vibe.yaml`, remove the `conformance` job and its `gate.needs` entry from
  `.github/workflows/ci.yml`, and (optionally, since it's now decoupled
  from `verify` per increment 1) drop the now-inert `audit` task from
  `Taskfile.yml`. Add a short pointer to it from `README.md`.
- Update `.github/workflows/examples.yml` to demonstrate the split
  concretely for this repository's own TS/PY fixtures: run `task verify`
  with no `vibe` on `PATH` (proving increment 1's independence end to end
  in CI, not just locally), and keep building `vibe` + running `task audit`
  as its own separately-labeled step — showing, in this repo's own
  dogfood CI, the shape of an optional integration rather than an implicit
  one.

### 3. Document the `vibe check` safe-fallback contract

Add a short subsection — to `docs/architecture/overview.md`'s "canonical
verification interface" section, or a new short entry under
`docs/decisions/`, whichever the implementation plan judges to fit better
— stating the contract a future `vibe check` implementation must satisfy:

- `vibe check` may skip validation for components it determines are
  unaffected by the current change set, once the affected-component graph
  (`internal/affected`) and validation runner (`internal/validation`)
  exist.
- It must instead run the full equivalent of `task verify` — never report
  success while having skipped checks — whenever any of the following
  holds: the `vibe` binary/context it needs cannot be resolved; the
  affected-component scope cannot be determined with confidence (for
  example, no prior state to diff against, or a graph-resolution error);
  or a change touches something with unbounded blast radius that isn't
  representable as a single component (`Taskfile.yml` itself, lint/tooling
  configuration, dependency manifests, or the `vibe.yaml` standard
  declaration).
- This is consistent with, and should be written to extend rather than
  contradict, two precedents already in the codebase: `docs/usage.md`'s
  existing `vibe check`/`vibe doctor` placeholder, which already fails
  loudly (`Error: check: not implemented yet ...`) instead of silently
  no-oping or exiting 0; and the non-fatal-but-visible `ToolRequirer`
  warning pattern from spec 0014 (`vibe sync` prints a warning, doesn't
  fail, when a required tool is missing from `PATH`) — `vibe check`'s
  fallback is stricter than that pattern (it must still run full
  verification, not just warn), and the spec should say so explicitly to
  avoid the two being conflated later.

This increment is documentation only. `internal/affected` and
`internal/validation` remain unbuilt, per every milestone spec since M0
that has deferred them.

## Acceptance criteria

- In `examples/typescript` and `examples/python`, with `vibe` absent from
  `PATH`: `task verify` and `task verify-ci` succeed on the clean fixture;
  a deliberately introduced lint or test failure still fails them.
- In this repository itself (its own `Taskfile.yml`, generated from
  `repotooling`, is the de facto Go example — there is no `examples/go`):
  `task verify`/`task verify-ci` succeed without invoking
  `go run ./cmd/vibe` at all.
- `task audit`, called explicitly, without a working `vibe`/`cmd/vibe`
  resolvable, still fails clearly (non-zero exit, readable error) — this is
  already today's behavior and must not regress.
- This repository's own CI `conformance` job (`.github/workflows/ci.yml`)
  continues to run `task audit` and continues to gate `gate` on drift —
  unaffected by increment 1, since it already invokes `task audit` directly
  rather than through `task verify`.

## Explicit non-goals

- No implementation of `vibe check` itself, or of `internal/affected`/
  `internal/validation` — increment 3 is a documented contract only.
- No fix to Go's `repotooling` `audit` task using `go run ./cmd/vibe`
  instead of a `PATH`-resolved `vibe` for external adopters. This is a
  pre-existing bug (adopters of `prod-go/v1` outside this repository have
  no local `cmd/vibe`, so their `task audit` already fails today,
  independent of this spec) — orthogonal to decoupling `verify` from
  `audit`, and tracked separately as issue #20 rather than folded in here.
- No change to spec 0013's "`vibe sync` never runs in CI" rule —
  conformance stays audit-only and read-only in CI.
- No new standards, and no renaming of existing modules or packages.

## Design notes

- The mechanical change in increment 1 is small — one line removed from
  each of three embedded Taskfile templates — but the correctness payoff
  (every generated repo's `verify` becomes truly tool-independent) is
  large relative to that size; the implementation plan should size review
  effort to the acceptance criteria, not the diff.
- Per `AGENTS.md`, any edit to `internal/module/**/templates/Taskfile.yml`
  requires, in the same change: a `vibe` rebuild (`go:embed` resolves at
  build time, so a stale binary re-syncs stale content), `vibe sync` run
  against this repository and both example fixtures, and the resulting
  `Taskfile.yml` + `.vibe/state.yaml` diffs committed together.
- Increment 2's CI template comments and `examples.yml` change should
  follow the same "unmanaged workflow file, not a module resource" design
  constraint spec 0016 already established for `examples.yml` — it must
  not leak this repository's own layout into a standard shipped to every
  adopter.
