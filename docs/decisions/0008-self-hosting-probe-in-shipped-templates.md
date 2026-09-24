# ADR 0008: Self-Hosting Probes in Shipped Templates

## Status

Accepted. Implemented by `internal/module/ci/github/templates/ci.yml`'s
`conformance` job, per `docs/specs/0018-prod-go-audit-portability.md`.
Nothing enforces this rule in code — like ADR 0007's tag naming, it governs
what a human may write into a template, and is checked at review time.

## Context

Spec 0016 established that a template shipped to adopters must not encode
this repository's layout: `internal/module/**/templates/` renders into every
adopting repository, so a path that only exists here becomes a broken line
everywhere else. Issue #20 is that rule violated —
`repotooling/templates/Taskfile.yml`'s `audit` task ran
`go run ./cmd/vibe audit --repo-root .`, which resolves only because this
repository vendors `cmd/vibe`.

Fixing it to a `PATH`-resolved `vibe` requires the generated `conformance`
job to put a `vibe` on `PATH`. `githubts`/`githubpy` already do, with
`go install github.com/Manual-debuger/VibeConform/cmd/vibe@latest`. Copying
that into `ci/github` breaks this repository, and — because
`.github/workflows/ci.yml` is itself a `resource.Generated` file produced by
that same template — there is no hand-written copy here to make differ. This
repository's `conformance` job *is* the shipped template's output.

`@latest` is not merely unavailable here; it is wrong. Two prerelease tags
exist (`v0.1.0-alpha.1`, `v0.2.0-alpha.1`) and Go falls back to the newest
prerelease when no release version is tagged, so the install would succeed
and produce a binary carrying *that tag's* embedded templates. A pull
request that changes a template changes the generated file alongside it, in
the same commit, as `AGENTS.md` requires. Auditing that PR with a released
binary compares new files against old templates and reports drift — on a
file the author regenerated correctly. The repository would be structurally
unable to evolve its own standards, and would fail confusingly rather than
loudly.

So the template must resolve `vibe` differently here than at an adopter,
while remaining one template.

## Decision

A shipped template may reference a path that only this repository
populates, **when the reference is a probe with a generic fallback**. All
three of these must hold:

- The adopter behaviour is the fallback branch, and is what any repository
  lacking the probed path gets.
- The fallback is byte-equivalent to what the sibling templates ship, so
  the probe adds a branch rather than a dialect.
- The probe fails loudly, not silently, when it guesses wrong.

An unconditional dependency on this repository's layout remains forbidden;
spec 0016's rule is qualified, not withdrawn.

The `conformance` job's `Install vibe` step is the first and currently only
application: it builds `./cmd/vibe` into `$RUNNER_TEMP/bin` when that
directory exists, and otherwise runs the same `go install …@latest` its
`githubts`/`githubpy` siblings already ship.

**Update (spec 0022).** The probe moved to the shared
`.github/workflows/conformance.yml`, owned by the language-neutral
`vibe-conformance` module, so all three standards now ship it. The
fallback is no longer `go install …@latest` in the workflow. The step
simply does nothing when `cmd/vibe` is absent, and `task audit` installs
the `vibe_version` recorded in `.vibe/state.yaml`. The "Go-only"
consequence below describes the situation before that change. In a
TypeScript or Python repository the branch is now inert, not absent: it
costs one `if` and saves keeping three workflow templates.

## Consequences

- The probe is Go-only. A TypeScript or Python repository never provides a
  Go `cmd/vibe` package, so adding the same branch to `githubts`/`githubpy`
  would be dead code whose `if` could only fire by coincidence. The three
  CI templates already differ in their language jobs; each install step is
  the simplest form correct for its language.
- An adopter whose Go repository happens to contain an unrelated
  `cmd/vibe` package takes the from-source branch and fails at `go build`
  with a compiler error. This is the accepted trade: loud, immediate, and
  rare. Probing for VibeConform's module path instead would buy precision
  at a cost in template complexity the risk does not justify.
- Nothing prevents a future template from taking the licence here and
  omitting the fallback. The rule is a review-time agreement; the thing to
  look for in a template diff is a repository-local path with no `else`.
- This repository's `conformance` job is the only place the `if` branch is
  ever exercised, which means it is also the only proof the branch works.
  A change to that step is not verified by tests — it is verified by the
  job running green on the pull request that makes it.
