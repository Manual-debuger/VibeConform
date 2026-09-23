# Plan 0018: Make `prod-go/v1`'s `task audit` work outside this repository

See `docs/specs/0018-prod-go-audit-portability.md` for the accepted scope.
Sequenced after spec 0017 (verify/audit decoupling), which made `task audit`
the only local conformance entrypoint and so turned this from a latent leak
into an adopter-facing break. Closes issue #20.

Approved at spec review: increment 4 proceeds as **(a)** (`build`/`run`
removed), and ADR 0008 is written.

Issue #22 is deliberately *not* addressed here and is expected to be
spec 0019.

## Order of work

Three commits. The ordering is load-bearing, not cosmetic:

1. **C1 — ADR 0008.** Documentation only. Lands first so the constraint it
   records is reviewable before the template that relies on it.
2. **C2 — the template change.** Increments 1, 2, 3 and 4(a) together,
   with the rebuild, the three syncs, and every regenerated artifact. This
   *must* be one commit: between increments 1 and 2 there is a state where
   the Taskfile wants a `PATH`-resolved `vibe` and the CI template provides
   none, and committing that state would leave a revision whose
   `conformance` job cannot pass.
3. **C3 — documentation sync.** `docs/usage.md`,
   `docs/architecture/overview.md`. Separated from C2 so the behavioural
   diff is reviewable without prose noise.

Within C2, per `AGENTS.md`: write the test and watch it fail → edit
templates → **rebuild** → sync all three roots → audit all three roots →
stage templates, regenerated files, and all three `.vibe/state.yaml`
together. Skipping the rebuild makes `vibe sync` propagate the *old*
embedded templates and everything reports conformant against stale content.

## Checklist

### C1 — ADR 0008

- [ ] Write `docs/decisions/0008-self-hosting-probe-in-shipped-templates.md`
      in the house format (`## Status` / `## Context` / `## Decision` /
      `## Consequences`, per ADR 0007).
  - **Context**: spec 0016 established that a template shipped to adopters
    must not encode this repository's layout. Spec 0018's `conformance` job
    needs `vibe` resolvable, and this repository cannot use the published
    release that adopters use, because auditing a template-change PR
    against a released binary compares new files to old templates.
  - **Decision**: a shipped template may reference a path only this
    repository populates **when the reference is a probe with a generic
    fallback** — the adopter path must be the `else` branch, must be the
    behaviour for any repository lacking the probed path, and must be
    byte-equivalent to what the sibling templates ship. An unconditional
    dependency on this repository's layout remains forbidden.
  - **Consequences**: record that nothing enforces this (it is a rule for
    humans reviewing template diffs, like ADR 0007's tag naming); that the
    probe is Go-only today and adding one to `githubts`/`githubpy` would
    be dead code; and that a repository coincidentally containing
    `cmd/vibe` takes the `if` branch and fails loudly at `go build`, which
    is the accepted trade.

### C2 — increments 1, 2, 3, 4(a)

#### Increment 3 first: the regression test, watched failing

- [ ] Add `TestAuditInvokesVibeFromPath` to
      `internal/module/verify_independence_test.go`, reusing the existing
      `taskfileDoc`/`shellClosure` helpers and the `repoToolingTaskfiles`
      list.

      Predicate: for each template, for every command in `audit`'s shell
      closure that mentions `vibe`, the command's **first whitespace-
      separated token must be exactly `vibe`**. This rejects `go run
      ./cmd/vibe audit …`, `go build …`, and any other
      repository-local invocation, while accepting the `vibe audit
      --repo-root .` form all three templates will share.

      Chosen over matching `go run`/`./cmd/` substrings because it states
      the requirement positively — *audit resolves vibe from `PATH`* —
      rather than enumerating today's known-bad forms.

- [ ] **Run it before changing any template and confirm it fails**, for
      `repotooling/templates/Taskfile.yml` only (TS and PY already comply,
      so they must pass from the start — a test that fails for all three
      would mean the predicate is wrong, not that the bug is worse than
      described). Paste the observed failure output into this plan under
      "Verification" rather than asserting it happened.

- [ ] Confirm `TestVerifyNeverInvokesVibe` and `TestAuditStillInvokesVibe`
      are **unmodified** and still pass. `vibe audit --repo-root .`
      contains the substring `vibe`, so the latter keeps holding; this is
      the intended outcome, not something to work around.

#### Increment 1 — `audit` becomes `PATH`-resolved

- [ ] `internal/module/repotooling/templates/Taskfile.yml`: change
      `audit`'s command from `go run ./cmd/vibe audit --repo-root .` to
      `vibe audit --repo-root .`. Name, `desc`, and argument unchanged;
      `audit` stays out of `verify`'s `cmds` per spec 0017.

#### Increment 2 — self-hosting-aware `Install vibe`

- [ ] `internal/module/ci/github/templates/ci.yml`: add an `Install vibe`
      step to the `conformance` job, positioned after `Install task` and
      before `task audit`, matching the YAML in spec 0018's increment 2.
      The `else` branch must be byte-identical to
      `internal/module/ci/githubts/templates/ci.yml`'s existing install
      step.
- [ ] Leave `githubts`/`githubpy` templates untouched. Their generated
      `ci.yml` files under `examples/` must show **no diff** after sync. If
      they do, stop — something resolved that should not have.
- [ ] Keep the existing `conformance:` job comment block (spec 0017's
      teardown instructions) intact; the new step sits below it.

#### Increment 4(a) — remove `build` and `run`

- [ ] `internal/module/repotooling/templates/Taskfile.yml`: delete the
      `build` and `run` tasks. Verified unreferenced: not in
      `lefthook.yml`, not in any CI template, not in any test (no test
      enumerates task names), and mentioned in no document except the
      template's own `desc` line.

#### Rebuild, sync, verify

- [ ] `go build -o bin/vibe ./cmd/vibe`
- [ ] `./bin/vibe sync --repo-root .`
- [ ] `./bin/vibe sync --repo-root examples/typescript`
- [ ] `./bin/vibe sync --repo-root examples/python`
- [ ] `./bin/vibe audit --repo-root .` → conformant
- [ ] `./bin/vibe audit --repo-root examples/typescript` → conformant
- [ ] `./bin/vibe audit --repo-root examples/python` → conformant
- [ ] Inspect `git diff` before staging and confirm the changed set is
      exactly: the two templates, the root `Taskfile.yml`, the root
      `.github/workflows/ci.yml`, and the three `.vibe/state.yaml` files.
      The examples' `Taskfile.yml` and `ci.yml` should be **unchanged** —
      they resolve from the TS/PY modules, which this spec does not touch.
- [ ] `task verify` at the repository root (includes `task workflows:lint`,
      so `actionlint` validates the regenerated workflow).

### C3 — documentation sync

- [ ] `docs/usage.md:452-455` — the standards-parity bullet currently reads
      that TS/PY expose the same targets as `prod-go/v1` "minus
      `build`/`run` — those build the `vibe` binary this repository ships,
      which doesn't generalize to an adopting repository." After increment
      4(a) that clause is false and its own reasoning now applies to Go.
      Rewrite so all three standards expose one identical target set, and
      note that the Go template previously carried two tasks that did not
      generalize.
- [ ] `docs/usage.md`, "VibeConform manages itself" — add the concrete
      `go build -o bin/vibe ./cmd/vibe` command to the managed-file cycle
      prose, discharging the obligation increment 4(a) creates. This is the
      only remaining written home for that command once `task build` is
      gone.
- [ ] `docs/usage.md` — state that `prod-go`'s `task audit` now requires
      `vibe` on `PATH`, like TS/PY, and that the Go `conformance` job
      installs it (from source when the repository provides `cmd/vibe`,
      from `@latest` otherwise).
- [ ] `docs/usage.md:553` — the "Removing VibeConform" teardown step for
      the `audit` task. Check whether its wording still holds now that
      `audit` shells to a `PATH` binary rather than `go run`; update the
      failure mode it describes if not.
- [ ] `docs/architecture/overview.md`, "Canonical verification interface" —
      record that `task audit` resolves `vibe` from `PATH` in all three
      standards, and point at ADR 0008 for why the Go `conformance` job's
      install step is conditional.
- [ ] `README.md` — review only. No statement in it currently mentions
      `task build`/`task run` or how `audit` resolves `vibe`, so the
      expected outcome is **no change**. If that turns out to be wrong,
      make the minimal edit and say so.

## Verification

Recorded as run, with observed output, not as intent.

- [ ] Regression test observed failing before the fix (paste output).
- [ ] Regression test passing after; full `go test ./...` green.
- [ ] `vibe audit` conformant on all three roots against a binary rebuilt
      *after* the template edits.
- [ ] `task verify` green at the repository root.
- [ ] `task audit` at the root with `bin/` on `PATH`: succeeds.
- [ ] `task audit` at the root with `vibe` **not** on `PATH`: fails with a
      legible `vibe: command not found`-class error and a non-zero exit.
      This is the adopter-visible behaviour when `vibe` is not installed;
      it must be a clear error, not a silent pass.
- [ ] **The `@latest` claim, confirmed empirically.** Install the published
      binary into a throwaway `GOBIN` and audit current `main` with it:

      ```bash
      GOBIN=$(mktemp -d) go install github.com/Manual-debuger/VibeConform/cmd/vibe@latest
      ```

      Expected: it resolves `v0.2.0-alpha.1` (only prereleases are tagged,
      so Go falls back to the newest one) and reports
      `.github/workflows/ci.yml` and `Taskfile.yml` as drifted against
      `main`, because that tag predates spec 0017. Record the actual
      output. If it does *not* drift, the argument for the `if` branch is
      weaker than the spec states and the spec needs correcting before
      this lands.
- [ ] This repository's own `conformance` job green on the pull request.
      This is the end-to-end proof of increment 2 and the only one that
      exercises the new install step for real — the job builds `vibe` from
      the PR's working tree and audits the PR's own regenerated files
      against the PR's own templates. Do not call the work done before it
      has run.

## Pull request

- [ ] Branch `feature/prod-go-audit-portability` off `main`, merge commit
      (not squash), opened as a PR — not merged. Merging is the
      maintainer's call.
- [ ] Fill in `.github/pull_request_template.md` truthfully, including
      `Closes #20` so the issue closes on merge. Tick the regression-test
      box only on the strength of the observed failure above; if any
      checkbox does not apply, write why instead of ticking it. (PR #21
      shipped with an unfilled template and no closing keyword, and its
      issue had to be closed by hand.)

## Explicitly still deferred

- **Issue #22** — `.vibe/state.yaml` records no provenance, so `vibe audit`
  reports a template upgrade as local drift. This spec's own sync will
  trigger exactly that for existing adopters on the next release. Expected
  to be spec 0019, with its own ADR and a migration story for existing
  state files.
- **Pinning `vibe` in the CI templates.** `githubts`/`githubpy` keep
  `@latest`, and the Go template's `else` branch matches them. Raised in
  #22 as a possible split-out; decided there.
- **A release tag.** Merging does not ship this. When a tag is eventually
  cut, its notes must tell adopters to run `vibe sync` — both this spec and
  #22 change managed content, and `github-ci-ts`/`github-ci-py` install
  `vibe@latest` unpinned, so the upgrade reaches adopters' CI on its next
  run whether or not they act.
- **`vibe check`/`vibe doctor`, `internal/affected`, `internal/validation`**
  — unchanged from plan 0017's deferral.
- **No new standards, no module renames, no manifest format change.**
