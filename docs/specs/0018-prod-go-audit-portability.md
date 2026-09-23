# Spec 0018: Make `prod-go/v1`'s `task audit` work outside this repository

Status: draft, awaiting approval.

Closes issue #20.

## Problem

`internal/module/repotooling/templates/Taskfile.yml`'s `audit` task is
self-hosted:

```yaml
  audit:
    desc: Check this repository against the standard its vibe.yaml declares.
    cmds:
      - go run ./cmd/vibe audit --repo-root .
```

`go run ./cmd/vibe` only resolves because *this* repository vendors the
`cmd/vibe` package. Every other repository declaring `prod-go/v1` gets that
same generated line and no such package, so its `task audit` fails
unconditionally — `go run` cannot find the package, before any audit logic
runs. The TypeScript and Python repo-tooling templates already do the right
thing (`vibe audit --repo-root .`, resolved from `PATH`), so this is a
Go-only defect and a parity gap, not a design question.

Three things make it worse than a stale line in a template.

**Spec 0017 made `task audit` the only local conformance entrypoint.**
Before it, `audit` was reachable transitively through `task verify`, so a
`prod-go` adopter hit the failure as part of their main verification gate.
Now `verify` is native-tooling-only by design, and `task audit` is the sole
way to check conformance locally. For `prod-go` adopters that entrypoint has
never worked.

**The generated CI `conformance` job runs `task audit`.** So the defect is
not merely a broken convenience task: `internal/module/ci/github/templates/
ci.yml`'s `conformance` job, which `gate` requires, fails on every run for
every external `prod-go` adopter. They inherit a red required check they
cannot fix without hand-editing a managed file — which is itself
non-conformant.

**Fixing the Taskfile alone moves the failure rather than removing it.**
Go's `conformance` job has no vibe-install step, because `go run
./cmd/vibe` never needed one. Its `githubts`/`githubpy` counterparts do:

```yaml
      - name: Install vibe
        run: go install github.com/Manual-debuger/VibeConform/cmd/vibe@latest
```

Change the Taskfile to a `PATH`-resolved `vibe` without touching the CI
template and every `prod-go` adopter's `conformance` job fails the same
number of times, just with `vibe: command not found` instead.

### The self-hosting chicken-and-egg

The obvious fix — copy `githubts`'s `Install vibe` step into
`ci/github`'s template — breaks *this* repository, and it cannot be patched
around locally, because `.github/workflows/ci.yml` is itself a managed
resource generated from that template. This repository's own `conformance`
job *is* the shipped template's output; there is no hand-written copy to
make differ.

`@latest` is wrong here for two independent reasons:

1. **No release exists yet.** This repository must be able to audit itself
   before its first tag. (Spec 0013 step B has gated on `task audit` since
   M1; that gate cannot depend on an artifact that does not exist.)
2. **Even after a release, it would be wrong.** A pull request that changes
   an embedded template changes the generated file alongside it, in the
   same commit, as `AGENTS.md` requires. An `@latest` binary carries the
   *released* templates, so it would compare this PR's regenerated files
   against the previous release's templates and report drift. Every
   template-change PR would go red — including this one. `@latest` makes
   the repository structurally unable to evolve its own standards.

So the install step must resolve `vibe` from the working tree when the
repository is VibeConform itself, and from a release otherwise. Spec 0016
established the constraint that governs how: *a template shipped to
adopters must not encode this repository's layout.*

### Related finding: `build` and `run` have the same defect

Scoping this surfaced two more tasks in the same Go template that hardcode
this repository's layout, and that TS/PY have no counterpart for:

```yaml
  build:
    desc: Build the vibe binary into ./bin.
    cmds:
      - go build -o bin/vibe ./cmd/vibe

  run:
    desc: 'Run the CLI, e.g. task run -- --help.'
    cmds:
      - go run ./cmd/vibe {{.CLI_ARGS}}
```

Every external `prod-go/v1` adopter receives a `task build` that claims to
build "the vibe binary" and a `task run` that runs VibeConform's CLI —
neither of which resolves in their repository. These are the same defect as
`audit` (this repository's developer tasks leaked into a shipped standard),
in the same file, fixed by the same sync cycle. They are not named in issue
#20, so increment 4 below carries them as an explicit approval decision
rather than folding them in silently.

## Scope

Four increments. Increment 4 is conditional on approval.

### 1. Make `prod-go/v1`'s `audit` task `PATH`-resolved

In `internal/module/repotooling/templates/Taskfile.yml`, change `audit`'s
command to match the TS/PY templates exactly:

```yaml
  audit:
    desc: Check this repository against the standard its vibe.yaml declares.
    cmds:
      - vibe audit --repo-root .
```

Nothing else about the task changes: same name, same description, same
`--repo-root .` argument, still absent from `verify`'s dependency chain per
spec 0017. All three repo-tooling templates then express `audit`
identically.

Effect for this repository: `task audit` begins requiring a `vibe` on
`PATH` rather than compiling one on demand. Locally that is already the
documented workflow (`AGENTS.md`'s managed-file cycle builds
`bin/vibe` explicitly, precisely so `go:embed` is resolved at a known
point); in CI it is what increment 2 provides.

### 2. Give the Go CI template a self-hosting-aware `Install vibe` step

Add an `Install vibe` step to `internal/module/ci/github/templates/ci.yml`'s
`conformance` job, before the `task audit` step, that resolves `vibe` from
the working tree when the repository provides `cmd/vibe` and from a release
otherwise:

```yaml
      - name: Install vibe
        run: |
          if [ -d ./cmd/vibe ]; then
            # Self-hosting: this repository provides cmd/vibe, so audit
            # against the working tree's embedded templates. Installing a
            # published release instead would compare a pull request's
            # regenerated files against the previous release's templates
            # and report drift on every template change.
            mkdir -p "$RUNNER_TEMP/bin"
            go build -o "$RUNNER_TEMP/bin/vibe" ./cmd/vibe
            echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"
          else
            go install github.com/Manual-debuger/VibeConform/cmd/vibe@latest
          fi
```

The `else` branch is byte-identical to what `githubts`/`githubpy` already
ship. The from-source branch reuses the pattern
`.github/workflows/examples.yml` already uses and `docs/usage.md:501`
already documents, so it introduces no new mechanism.

On spec 0016's constraint: this is a capability probe with a generic
fallback, not a layout assumption. The template never *requires*
`cmd/vibe`; it asks whether the repository has one and degrades to the
published-release path when it does not. An adopter's repository takes the
`else` branch and renders behaviourally identical to `githubts`'s. That
said, it does name a path that only this repository populates, which is a
real qualification of a standing constraint rather than an application of
it — see "ADR" below.

Go only. `githubts` and `githubpy` keep their unconditional `@latest`
install: a TypeScript or Python repository never provides a Go `cmd/vibe`
package, so the probe would be dead code whose `if` branch could only fire
on a coincidence. The three CI templates already differ in their language
jobs; this keeps each one's install step the simplest form that is correct
for its language.

Edge case, accepted: an adopter whose Go repository happens to contain an
unrelated `cmd/vibe` package takes the from-source branch and fails at
`go build` with a compiler error. This is loud, immediate, and rare, and
the alternative (probing for VibeConform's module path) buys precision at a
cost in template complexity that the risk does not justify.

### 3. Regression test

Extend `internal/module/verify_independence_test.go` — the existing home
for invariants about what the repo-tooling templates' tasks may invoke —
with a test asserting that no repo-tooling template's `audit` closure
shells out to a repository-local path.

The existing `TestAuditStillInvokesVibe` does not cover this: it matches
the substring `vibe`, which both `go run ./cmd/vibe audit` and `vibe audit`
satisfy. That is intended — spec 0017's invariant is about *whether* audit
reaches vibe, not *how* — so the new test is additive and neither existing
test changes.

The new assertion is that `audit`'s shell closure invokes `vibe` as a
`PATH`-resolved binary: no `go run`, no `./cmd/`, no `go build`. Exact
predicate decided in the implementation plan.

Per `AGENTS.md`, this test must be watched failing against the current
`go run ./cmd/vibe audit --repo-root .` before it is relied on, and the
plan records that observation explicitly.

The per-module byte-equality drift tests cannot catch this class of
regression, for the reason spec 0017 already documented: template and live
file move together under `vibe sync`, so reintroducing the self-hosted form
and re-syncing leaves all three repositories reporting conformant.

### 4. `build` and `run` — decision required at approval

Two options, to be settled before implementation begins:

**(a) Fix in this spec.** Remove `build` and `run` from
`internal/module/repotooling/templates/Taskfile.yml`. They are developer
tasks for VibeConform's own CLI, not part of what `prod-go/v1` means, and
neither TS nor PY ships an equivalent. This repository keeps them by a
means that does not ship to adopters — decided in the plan; the likely
shape is that `AGENTS.md`'s managed-file cycle already spells out
`go build -o bin/vibe ./cmd/vibe` directly, so the `build` task may simply
have no replacement worth adding.

**(b) Defer to a follow-up issue.** Keep this spec to exactly what #20
names, and file the `build`/`run` leak separately.

Recommendation: **(a)**. The cost of deferring is not zero. Every
template change forces a regenerated `Taskfile.yml` in three repositories
plus three `.vibe/state.yaml` updates, and — per issue #22 — pushes every
existing adopter into a `drifted` report they did not cause. Doing both
fixes in one sync cycle pays that cost once. The risk is also low and
one-directional: removing a task that cannot work for any adopter cannot
break an adopter, and for this repository it is a local convenience with a
documented one-line equivalent.

Against (a): it widens a spec whose value is being small, and `task run`
may be in a maintainer's muscle memory. If that matters, (b) is a
legitimate call and this spec proceeds with increments 1–3 unchanged.

## ADR

Increment 2 qualifies a constraint spec 0016 set — it permits a shipped
template to name a path that only this repository populates, provided the
reference is a probe with a generic fallback. Future template work will
need to know where that line is, and a spec's design notes are not where
someone looks for a standing rule.

Recommendation: a short `docs/decisions/0008-self-hosting-in-shipped-
templates.md` recording the rule and its limits (probe with fallback, yes;
unconditional layout dependency, no). Slug and exact wording in the plan.

Spec 0017's increment 3 decided *against* an ADR on the grounds that its
content extended a paragraph already present in
`docs/architecture/overview.md`. This is the opposite case: there is no
existing paragraph, and the content is a constraint on future decisions
rather than a description of current behaviour.

If the maintainer prefers this live in the spec alone, increment 2 is
unaffected — the ADR records the decision, it does not make it.

## Acceptance criteria

- `internal/module/repotooling/templates/Taskfile.yml`'s `audit` task is
  `vibe audit --repo-root .`, byte-identical in form to the TS and PY
  templates' `audit` tasks.
- Against a freshly rebuilt `bin/vibe`, `vibe audit` reports **conformant**
  on all three roots: `.`, `examples/typescript`, `examples/python`. The
  regenerated `Taskfile.yml` files, the regenerated
  `.github/workflows/ci.yml`, and all three updated `.vibe/state.yaml`
  files are committed in the same change as the template edits.
- `task verify` passes at the repository root.
- The new regression test in `internal/module/verify_independence_test.go`
  fails against the pre-fix `go run ./cmd/vibe audit --repo-root .` — a
  failure observed and recorded in the plan, not assumed — and passes
  after.
- `TestVerifyNeverInvokesVibe` and `TestAuditStillInvokesVibe` both still
  pass, unmodified. (`vibe audit --repo-root .` keeps satisfying the
  latter; this is the intended outcome, not a coincidence to work around.)
- `task audit` at the repository root succeeds with `bin/` on `PATH` and
  fails clearly — non-zero exit, readable `vibe: command not found` —
  without it. The second half is the adopter-visible behaviour when `vibe`
  is not installed, and it must be a legible error rather than a silent
  pass.
- `actionlint` accepts the regenerated `.github/workflows/ci.yml`
  (`task workflows:lint`, already part of `task verify`).
- This repository's own `conformance` job passes on the pull request — the
  end-to-end proof of increment 2, since the job runs the new install step
  and audits the PR's own regenerated files against the PR's own templates.
- `docs/usage.md` and `docs/architecture/overview.md` reflect that
  `prod-go`'s `task audit` requires `vibe` on `PATH` like TS/PY, and that
  the Go `conformance` job installs it. `README.md` updated only if a
  statement in it became false.

## Explicit non-goals

- **No fix for issue #22.** `vibe audit` still reports a template upgrade
  as local drift, and this spec's own sync will trigger exactly that for
  existing adopters. That is the standing tax #22 exists to remove; it
  needs a state-format change and its own ADR, and is sequenced after this
  spec deliberately so a one-line portability fix does not wait behind it.
- **No change to `verify`'s dependency chain.** Spec 0017's invariant
  stands: `verify` reaches native language tooling only. This spec changes
  how `audit` resolves `vibe`, never who calls `audit`.
- **No change to spec 0013's "`vibe sync` never runs in CI" rule.** The
  `conformance` job stays read-only; increment 2 adds an install step, not
  a repair step.
- **No version pinning of `vibe` in the CI templates.** `githubts`/
  `githubpy` keep `@latest`, and the Go template's `else` branch matches
  them. Pinning is raised in issue #22 as a possible split-out and is
  decided there, not here.
- **No new standards, no module renames, no manifest format change.** The
  self-hosting distinction is made by the template at runtime, not by a new
  `vibe.yaml` field.
- **No release tag.** Merging this does not ship it. Adopters reach it when
  a tag is cut, and that release's notes must tell them to run `vibe sync`.

## Design notes

- The mechanical change in increment 1 is one line, and increments 2–4 are
  small; as with spec 0017, review effort should be sized to the acceptance
  criteria rather than the diff. The load-bearing part is that this
  repository's `conformance` job keeps working across the change, and the
  only honest proof of that is the job running green on the pull request
  itself.
- Sequencing within the change matters and is a rebuild hazard, not a
  style preference. Increments 1 and 2 must land in one commit: between
  them there is a state where the Taskfile wants a `PATH`-resolved `vibe`
  and the CI template does not provide one. Per `AGENTS.md`, `go:embed`
  resolves at build time, so the order is edit templates → rebuild →
  sync all three roots → audit all three roots → commit templates,
  regenerated files, and all three `.vibe/state.yaml` together.
- `.github/workflows/examples.yml` needs no change. It is unmanaged, it
  already builds `vibe` from source into `$RUNNER_TEMP/bin` before running
  `task audit` against the TS/PY fixtures, and increment 1 does not alter
  those fixtures' `audit` task.
- The examples' own generated `.github/workflows/ci.yml` files are inert —
  GitHub only runs workflows under the repository root's
  `.github/workflows/` — so they are drift alarms for the `githubts`/
  `githubpy` templates, not live pipelines. Increment 2 does not touch
  those templates, so those files should not change under sync. If they
  do, something is wrong and the plan should stop rather than commit it.
