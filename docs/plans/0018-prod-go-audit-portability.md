# Plan 0018: Make `prod-go/v1`'s `task audit` work outside this repository

See `docs/specs/0018-prod-go-audit-portability.md` for the accepted scope.
Sequenced after spec 0017 (verify/audit decoupling), which made `task audit`
the only local conformance entrypoint and so turned this from a latent leak
into an adopter-facing break. Closes issue #20.

Approved at spec review: increment 4 removes `build`/`run` from the shipped
standard **and** adds a project-owned `Taskfile.local.yml` include seam to
all three repo-tooling templates; ADRs 0008 and 0009 are both written.

Issue #22 is deliberately *not* addressed here and is expected to be
spec 0019.

## Order of work

Three commits. The ordering is load-bearing, not cosmetic:

1. **C1 — ADRs 0008 and 0009.** Documentation only. Lands first so both
   constraints are reviewable before the templates that rely on them.
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

### C1 — ADRs 0008 and 0009

- [x] Establish the minimum Task version that supports `includes.flatten`
      and record it in ADR 0009. Confirm it is at or below the pinned
      `TASK_VERSION` (v3.53.1). Also confirm what an older Task does when
      it meets `flatten: true` — a clear error is acceptable, silently
      namespacing the tasks as `local:build` is a trap worth documenting.
      This gates the ADR's "Consequences" section; do not write a version
      number that has not been checked.

- [x] Write `docs/decisions/0008-self-hosting-probe-in-shipped-templates.md`
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

- [x] Write `docs/decisions/0009-managed-file-local-extension.md`, same
      format.
  - **Context**: `docs/decisions/0003-resource-ownership.md` defines
    `generated` as VibeConform owning the whole file, which leaves a
    repository nowhere to put tasks of its own. `prod-go/v1` papered over
    this by shipping `build`/`run` — commands that are repository-specific
    even within one language, and that only ever worked here.
    `docs/usage.md:452-455` already recorded the generalization failure,
    but filed it as a TS/PY omission rather than a Go defect.
  - **Decision**: a wholly-generated `Taskfile.yml` declares an optional,
    flattened include of a project-owned `Taskfile.local.yml`. VibeConform
    keeps owning the generated file completely; the local file is outside
    the managed set entirely. State plainly that the mechanism is Task's
    own `includes` — `vibe` gains no code, knows no such filename, and
    audits nothing about it.
  - **Consequences**: absent is a no-op (`optional: true`); `flatten: true`
    gives bare task names and `CLI_ARGS` pass-through; a local file
    redefining a managed task is a **hard error, exit 203**
    (`Found multiple tasks (verify) included by "local"`), so the seam
    cannot be used to neuter `verify` or `audit`. Record explicitly that
    this integrity property is Task's behaviour, not a VibeConform check —
    if Task ever changed it to last-wins, the standard would silently gain
    a hole, so it is worth re-confirming on a `TASK_VERSION` bump. Note the
    minimum Task version from the step above.

### C2 — increments 1, 2, 3, 4(a)

#### Increment 3 first: the regression test, watched failing

- [x] Add `TestAuditInvokesVibeFromPath` to
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

- [x] **Run it before changing any template and confirm it fails**, for
      `repotooling/templates/Taskfile.yml` only (TS and PY already comply,
      so they must pass from the start — a test that fails for all three
      would mean the predicate is wrong, not that the bug is worse than
      described). Paste the observed failure output into this plan under
      "Verification" rather than asserting it happened.

- [x] Confirm `TestVerifyNeverInvokesVibe` and `TestAuditStillInvokesVibe`
      are **unmodified** and still pass. `vibe audit --repo-root .`
      contains the substring `vibe`, so the latter keeps holding; this is
      the intended outcome, not something to work around.

#### Increment 1 — `audit` becomes `PATH`-resolved

- [x] `internal/module/repotooling/templates/Taskfile.yml`: change
      `audit`'s command from `go run ./cmd/vibe audit --repo-root .` to
      `vibe audit --repo-root .`. Name, `desc`, and argument unchanged;
      `audit` stays out of `verify`'s `cmds` per spec 0017.

#### Increment 2 — self-hosting-aware `Install vibe`

- [x] `internal/module/ci/github/templates/ci.yml`: add an `Install vibe`
      step to the `conformance` job, positioned after `Install task` and
      before `task audit`, matching the YAML in spec 0018's increment 2.
      The `else` branch must be byte-identical to
      `internal/module/ci/githubts/templates/ci.yml`'s existing install
      step.
- [x] Leave `githubts`/`githubpy` templates untouched. Their generated
      `ci.yml` files under `examples/` must show **no diff** after sync. If
      they do, stop — something resolved that should not have.
- [x] Keep the existing `conformance:` job comment block (spec 0017's
      teardown instructions) intact; the new step sits below it.

#### Increment 4 — remove `build`/`run`, add the local include seam

- [x] `internal/module/repotooling/templates/Taskfile.yml`: delete the
      `build` and `run` tasks. Verified unreferenced: not in
      `lefthook.yml`, not in any CI template, not in any test (no test
      enumerates task names), and mentioned in no document except the
      template's own `desc` line and `docs/usage.md:452-455`.
- [x] Add the include block, **identically**, to all three repo-tooling
      templates, immediately below `version: "3"` and above `tasks:`:

      ```yaml
      includes:
        local:
          taskfile: ./Taskfile.local.yml
          optional: true
          flatten: true
      ```

      Identical is the requirement, not a preference — a per-language
      divergence here would be the same asymmetry this increment removes.
- [x] Create this repository's project-owned `Taskfile.local.yml`, holding
      the `build` and `run` tasks verbatim as they exist in the Go template
      today, including their `desc` lines and `{{.CLI_ARGS}}`. Commit it.
      It must **not** be gitignored — it is committed project configuration,
      not developer-local scratch, and the name refers to the repository,
      not the machine.
- [x] Confirm `vibe audit` does **not** list `Taskfile.local.yml`. It is
      unmanaged, like `AGENTS.md` and `.gitignore`; if it appears in the
      audit output something has gone wrong in the resource model.
- [x] Do **not** create `Taskfile.local.yml` in `examples/typescript` or
      `examples/python`. Their absence is the adopter default and is what
      the examples exist to exercise.

#### Rebuild, sync, verify

- [x] `go build -o bin/vibe ./cmd/vibe`
- [x] `./bin/vibe sync --repo-root .`
- [x] `./bin/vibe sync --repo-root examples/typescript`
- [x] `./bin/vibe sync --repo-root examples/python`
- [x] `./bin/vibe audit --repo-root .` → conformant
- [x] `./bin/vibe audit --repo-root examples/typescript` → conformant
- [x] `./bin/vibe audit --repo-root examples/python` → conformant
- [x] Inspect `git diff` before staging and confirm the changed set is
      exactly: the four templates (Go/TS/PY repo-tooling + `ci/github`),
      all three `Taskfile.yml` files (root, `examples/typescript`,
      `examples/python`), the root `.github/workflows/ci.yml`, the three
      `.vibe/state.yaml` files, and the new untracked
      `Taskfile.local.yml`.

      The examples' `.github/workflows/ci.yml` must be **unchanged** —
      `githubts`/`githubpy` are untouched and only increment 2 alters a CI
      template. A diff there means something resolved that should not
      have; stop rather than commit it.
- [x] `task verify` at the repository root (includes `task workflows:lint`,
      so `actionlint` validates the regenerated workflow).

### C3 — documentation sync

- [x] `docs/usage.md:452-455` — the standards-parity bullet currently reads
      that TS/PY expose the same targets as `prod-go/v1` "minus
      `build`/`run` — those build the `vibe` binary this repository ships,
      which doesn't generalize to an adopting repository." After increment
      4 that clause is false and its own reasoning now applies to Go.
      Rewrite so all three standards expose one identical target set, and
      say that repository-specific tasks belong in `Taskfile.local.yml`.
      This sentence is the best evidence the leak was already understood;
      the rewrite should read as finishing that thought, not reversing it.
- [x] `docs/usage.md` — document the `Taskfile.local.yml` seam for
      adopters: what it is for, that it is optional and project-owned, that
      `vibe` never touches it, and that redefining a managed task is a hard
      Task error rather than an override. Include the exit-203 message so
      someone who hits it can search for it.
- [x] `docs/usage.md`, "VibeConform manages itself" — add the concrete
      `go build -o bin/vibe ./cmd/vibe` command to the managed-file cycle
      prose, and add `Taskfile.local.yml` to the "Still hand-maintained
      here" list alongside `AGENTS.md` and `.gitignore`.
- [x] `docs/usage.md` — state that `prod-go`'s `task audit` now requires
      `vibe` on `PATH`, like TS/PY, and that the Go `conformance` job
      installs it (from source when the repository provides `cmd/vibe`,
      from `@latest` otherwise).
- [x] `docs/usage.md:553` — the "Removing VibeConform" teardown step for
      the `audit` task. Check whether its wording still holds now that
      `audit` shells to a `PATH` binary rather than `go run`; update the
      failure mode it describes if not.
- [x] `docs/architecture/overview.md`, "Canonical verification interface" —
      record that `task audit` resolves `vibe` from `PATH` in all three
      standards; that the managed target set is fixed and extended (never
      overridden) via `Taskfile.local.yml`; and point at ADR 0008 for the
      conditional install step and ADR 0009 for the seam.
- [x] `docs/architecture/overview.md`, "Resource ownership" — the four
      ownership modes are listed there with no mention that a `generated`
      file can carry a documented extension point. Add a sentence pointing
      at ADR 0009, so someone reading the ownership model does not conclude
      that `generated` leaves a repository no recourse.
- [x] `README.md` — review only. No statement in it currently mentions
      `task build`/`task run` or how `audit` resolves `vibe`, so the
      expected outcome is **no change**. If that turns out to be wrong,
      make the minimal edit and say so.

## Verification

Recorded as run, with observed output, not as intent.

- [x] Regression test observed failing before the fix (paste output).
- [x] Regression test passing after; full `go test ./...` green.
- [x] `vibe audit` conformant on all three roots against a binary rebuilt
      *after* the template edits.
- [x] `task verify` green at the repository root.
- [x] `task audit` at the root with `bin/` on `PATH`: succeeds.
- [x] `task audit` at the root with `vibe` **not** on `PATH`: fails with a
      legible `vibe: command not found`-class error and a non-zero exit.
      This is the adopter-visible behaviour when `vibe` is not installed;
      it must be a clear error, not a silent pass.
- [x] **The seam, all three states, in the real repository** — not only in
      the scratch fixture the design was prototyped against:
  - [x] `task build` and `task run -- --help` at the repository root behave
        as they did before the change, now served by `Taskfile.local.yml`.
        `task --list` shows them beside the managed tasks.
  - [x] `task verify` in `examples/typescript` and `examples/python`, which
        declare the include and have no `Taskfile.local.yml`, passes with
        no `vibe` on `PATH`. Absent must cost nothing.
  - [x] Temporarily add a `verify` task to this repository's
        `Taskfile.local.yml`, confirm `task verify` fails with exit 203 and
        `Found multiple tasks (verify) included by "local"`, then revert
        and confirm `git status` is clean. The standard's integrity now
        rests on this; assert it against the real file, and record the
        output.
- [x] **The `@latest` claim, confirmed empirically.** Install the published
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
- [x] This repository's own `conformance` job green on the pull request.
      Confirmed on PR #24: the job took the `if` branch, built `vibe` from
      the PR's working tree, and `task audit` reported
      `11 resources checked, 0 drifted, 0 conflicts / conformant`. All 13
      checks pass.
      This is the end-to-end proof of increment 2 and the only one that
      exercises the new install step for real — the job builds `vibe` from
      the PR's working tree and audits the PR's own regenerated files
      against the PR's own templates. Do not call the work done before it
      has run.

### Observed output

**Regression test, before any template was touched** — fails for Go only,
TS/PY pass, which is what confirms the predicate is right rather than
merely strict:

```text
--- FAIL: TestAuditInvokesVibeFromPath (0.00s)
    verify_independence_test.go:189: task audit runs "go run ./cmd/vibe audit --repo-root ."; vibe must be
    invoked as a PATH-resolved binary (`vibe audit --repo-root .`), not built or run out of the repository
    under audit (spec 0018) — an adopting repository has no cmd/vibe package
    --- FAIL: TestAuditInvokesVibeFromPath/repotooling/templates/Taskfile.yml (0.00s)
    --- PASS: TestAuditInvokesVibeFromPath/tsrepotooling/templates/Taskfile.yml (0.00s)
    --- PASS: TestAuditInvokesVibeFromPath/pyrepotooling/templates/Taskfile.yml (0.00s)
```

`TestVerifyNeverInvokesVibe` and `TestAuditStillInvokesVibe` passed
throughout, unmodified, before and after.

**Sync counts** — root 2 updated (`Taskfile.yml`, `.github/workflows/
ci.yml`), each example 1 (`Taskfile.yml`). The examples' `ci.yml` did not
change, as required. All three roots then audited `0 drifted, 0 conflicts,
conformant`.

**Collision, against the real `Taskfile.local.yml`:**

```console
$ task verify   # with a verify: task added to Taskfile.local.yml
task: Found multiple tasks (verify) included by "local"
$ echo $?
203
```

Reverted afterwards; `git status` clean.

**`task audit` with no `vibe` on `PATH`:**

```console
$ task audit
task: [audit] vibe audit --repo-root .
"vibe": executable file not found in $PATH
task: Failed to run task "audit": exit status 127
```

Task itself exits 201. Same shape plan 0017 recorded for TS/PY, so Go is
now consistent with them.

**The `@latest` claim — confirmed, and stronger than the spec assumed:**

```console
$ GOBIN=<tmp> go install github.com/Manual-debuger/VibeConform/cmd/vibe@latest
$ go version -m <tmp>/vibe.exe | grep '\smod\s'
        mod     github.com/Manual-debuger/VibeConform  v0.2.0-alpha.1
$ <tmp>/vibe.exe audit --repo-root .
.github/workflows/ci.yml: drifted (run vibe sync)
Taskfile.yml: drifted (run vibe sync)
11 resources checked, 2 drifted, 0 conflicts
not conformant
```

So a plain `@latest` install step would have failed the `conformance` job
on this very pull request, reporting two files as drifted that were
regenerated correctly. The `if` branch is load-bearing, and this is issue
#22's ambiguity seen from the inside: the binary is stale, the repository
is not, and `audit` has no vocabulary for the difference.

### Two findings worth carrying forward

Neither blocks this work; both were found while verifying it.

1. **A stale `vibe` in `~/go/bin` silently wins.** Now that `task audit`
   resolves from `PATH`, a `vibe` left behind by an earlier `go install`
   is used in preference to a freshly built one, and reports just-synced
   files as drifted. This bit during verification here. Documented in
   `docs/usage.md`; it is the local-development twin of the `@latest`
   problem above and a further argument for #22.
2. **On Windows, `go build -o bin/vibe` produces an extensionless file**
   that `PATH` lookup will not find as `vibe`, so putting `bin/` on `PATH`
   does not make `task audit` work — it silently falls through to whatever
   else is on `PATH` (see finding 1). Use `./bin/vibe` directly, or build
   to `vibe.exe`. Documented; not changed, since the `build` task's output
   path is now this repository's own business.

## Pull request

- [x] Branch `feature/prod-go-audit-portability` off `main`, merge commit
      (not squash), opened as a PR — not merged. Merging is the
      maintainer's call.
- [x] Fill in `.github/pull_request_template.md` truthfully, including
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
