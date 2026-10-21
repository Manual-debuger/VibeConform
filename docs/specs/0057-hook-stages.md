# Spec 0057: What each hook stage runs

Status: accepted (2026-10-21). Issue #130. Scope settled with the user
in a grilling session on 2026-10-21 (decisions Q1–Q16 below). Builds on
spec 0056 (#127, PR #128) and amends spec 0023 (agent lifecycle hooks),
spec 0019 (the audit's vibe version) and ADR 0008. The `hook:format`
subdirectory bug found on the way is #129, outside this spec.

## Problem

Spec 0056 made the checks fast enough to run. It did not decide where
each check belongs. Running up to five agents in parallel worktrees
(docs/reports/0056-faster-hooks.md, "Multi-agent friction") left these
open:

- **A sub-agent's failures wake its parent.** `hook:check` runs after
  every edit with `asyncRewake`. When a sub-agent's edit fails the check,
  the parent session is woken, not the sub-agent. TDD red phases fail on
  purpose, so the orchestrator got about 20 such wake-ups in one session
  (friction item 2).
- **Tests run after every edit.** `hook:check` runs typecheck, lint and
  tests: `test:short` for Go, and the *whole* `pytest` / `pnpm test`
  suite for Python and TypeScript. An edit in the middle of a change
  rarely passes the tests, and five agents doing this compete for the
  machine.
- **The Stop gate runs even when nothing changed.** An orchestrator turn
  that only dispatched agents or read their results still runs
  `verify:fast`, which competes with the agents for CPU and spoiled two
  timing runs (item 4).
- **This repository cannot audit itself with an older vibe on PATH.**
  `vibe audit` refuses a binary older than the one that last synced
  (spec 0019), which is right for adopters. Here, every agent has to put
  the branch's `bin/` first on PATH (item 6).
- **pre-push leaks Git's repository variables.** A pre-push hook started
  from a linked worktree exports `GIT_DIR`. Tests that run git then act
  on the real repository. Every test package here clears the variables
  in its own `TestMain` (seven copies); an adopter's tests do not (item
  8).
- **A sync from a dirty tree breaks CI.** `task build` stamps
  `+dirty` on a build from uncommitted changes, and sync records that as
  `vibe_version`. The setup action validates the version before it
  decides to build from the checkout, so it refuses `+dirty` even in
  VibeConform's own repository, where the version is not used (item 9).
- **pre-push runs the full suite** (376s here, over 10 minutes before
  #125), which CI runs anyway on Linux and Windows.
- **The hooks name tools, not entrypoints.** pre-commit runs `gofmt`,
  `goimports` and `go vet` on staged files; pre-push runs `golangci-lint`
  and `task test`. The contract audit therefore reports SHOULD rule H-02
  ("pre-commit checks formatting; pre-push runs verification") as
  failing.

## Constraints

- **Contract rules stay satisfied.** A-05 (MUST): the Stop hook command
  calls `verify-fast`, and with a planted failing test it exits 2 for
  `stop_hook_active: false` and 0 for `true`. V-02 (MUST): `verify-fast`
  catches every planted defect, so it keeps the tests. The audit matches
  a call word for word (`task verify:fast`), so a prefix such as
  `env -u …` before it does not count as a call.
- **No contract change.** Contract 1.1.0 stays as it is; H-02 moves from
  fail to pass because the generated hooks change, not the rule.
- **Managed files change only through templates** under
  `internal/module/`, then `task sync` (AGENTS.md). The setup skill's
  reference Taskfiles change with them.
- **Adopters keep spec 0019's safety.** An adopter's `task audit` still
  reaches vibe only from PATH or from the version pinned in
  `.vibe/state.yaml`, and still refuses an older binary.
- **Claude Code facts** (hooks reference, 2026-10-21): the hook input
  carries `agent_id` and `agent_type` when the hook fires inside a
  sub-agent. `SubagentStop` exists, and exit 2 there keeps the sub-agent
  from stopping. The docs do not say which session an `asyncRewake`
  wakes; the parent being woken is observed, not documented.

## Assumptions

- A payload with an `agent_id` comes from a sub-agent, and a main-session
  payload has none. Not verified for teammates or `--agent` sessions,
  which `agent_type` also marks.
- lefthook runs a command's `run:` with `sh` on every platform, so
  `unset` works there, including on Windows with Git for Windows.
- Go's test cache stays on (Q7): `test:short` keeps no `-count=1`.

## Desired Behavior

### After each edit: `hook:check` (Q2, Q13)

1. `hook:check` runs typecheck and lint only, in every standard: build
   and `golangci-lint` for Go, `lint` and `typecheck` for Python and
   TypeScript, and the monorepo components' equivalents. It runs no
   tests.
2. When the payload has a non-empty `agent_id`, `hook:check` exits 0 and
   runs nothing. No `SubagentStop` hook is added.
3. Spec 0056's path filter stays: an edit outside the repository or of an
   ignored file runs nothing.

### When the agent stops: the Stop gate (Q3, Q4, Q12)

4. The Stop gate still runs `verify:fast`, tests included.
5. After a green `verify:fast`, the gate records a **green stamp**: a
   hash of HEAD, the tracked changes (staged or not), and the names and
   contents of untracked files git does not ignore. The stamp lives in
   the worktree's own Git directory, so each linked worktree has its own.
6. When the tree's hash equals the green stamp, the gate exits 0 without
   running `verify:fast`. A failing run never records a stamp, so a
   failure always runs again. Outside a Git repository there is no stamp
   and the gate always runs.
7. The block-once behavior is unchanged: a failing `verify:fast` exits 2
   for `stop_hook_active: false` and 0 for `true`. The Stop command still
   contains the literal call `task verify:fast` (A-05's static part).

### Git hooks (Q5, Q9, Q11, Q15)

8. pre-commit runs `task fmt:check`.
9. pre-push clears `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE` and
   `GIT_COMMON_DIR` with `unset`, then runs `task verify:fast`. It always
   runs; it does not read the green stamp.
10. The full suite moves out of pre-push: CI's test jobs remain the full
    gate, and `task test` stays available by hand.
11. Lint and test steps that pre-push and pre-commit ran directly are
    gone; repository-specific additions still go in
    `lefthook.local.yml`.

### VibeConform's own repository (Q8, Q14)

12. The generated `audit` task first checks whether the repository *is*
    VibeConform: `cmd/vibe` exists and `go.mod` declares VibeConform's
    module path (the probe ADR 0008 allows). If so, it runs the
    checkout's vibe from source. Otherwise it behaves as today: vibe from
    PATH, else the pinned version.
13. The setup action decides its source before it validates the version.
    When the source is the workspace (VibeConform's own checkout), the
    recorded version is information only and any value is accepted,
    `dev` and `+dirty` included. For every other source, validation is
    unchanged.
14. `task sync` keeps restoring a `.vibe/state.yaml` whose only change is
    `vibe_version`. Syncing from a dirty tree is no longer a CI hazard in
    this repository, and the rule "sync from a clean commit only" is
    dropped from the docs.

### Documentation

15. `docs/usage.md` ("Agent hooks", "Git hooks") states what each stage
    runs, the green stamp, the sub-agent skip, and the Go test cache
    hazard: without `-count=1`, go test hashes each package's test log,
    and a package that stats hundreds of thousands of paths pays minutes
    for it, on a cache hit too (report 0056).
16. ADR 0008's Consequences agree with its spec 0054 update (the module
    path is checked), and `warnUnpinnable`'s comment says what CI does.
17. The report's friction table marks items 2, 4, 6, 8 and 9 resolved by
    this spec.

## User Stories

1. As an orchestrating agent, I want my sub-agents' red phases not to
   wake me, so that I see their results, not their intermediate failures.
2. As a sub-agent doing TDD, I want my edits not to start test runs, so
   that a deliberately failing test does not cost CPU or noise.
3. As an agent editing code, I want typecheck and lint feedback after
   each edit, so that I catch a broken build early.
4. As an agent ending a turn, I want the tests to run before I stop, so
   that I do not hand back a failing tree.
5. As an orchestrator whose turn changed nothing, I want my Stop gate to
   return at once, so that it does not compete with my agents.
6. As an agent whose previous stop was green, I want a stop with the same
   tree to skip the checks, so that reading or planning turns cost
   nothing.
7. As an agent whose last run failed, I want the next stop to run the
   checks again, so that a failure is never skipped.
8. As an agent in a linked worktree, I want my own green stamp, so that
   another worktree's result never stands for mine.
9. As a developer pushing, I want pre-push to finish in a minute or two,
   so that I push often; CI runs the full suite.
10. As a developer pushing from a linked worktree, I want my tests to
    see no `GIT_DIR` from the hook, so that they cannot touch my real
    repository.
11. As an adopter, I want my hooks to call my entrypoints, so that the
    contract audit recognizes them and H-02 passes.
12. As a VibeConform contributor, I want `task audit` to judge the
    checkout with the checkout's own vibe, so that I need no PATH tricks.
13. As an adopter, I want `task audit` to keep refusing an older vibe,
    so that a stale binary never gives me a verdict.
14. As a VibeConform contributor, I want a sync from uncommitted
    template changes to pass CI, so that I can sync and commit in one
    step.
15. As an adopter, I want the setup action to keep refusing versions it
    cannot fetch, so that my CI fails clearly rather than oddly.
16. As a contract auditor, I want A-05 and V-02 to stay conformant for
    every language fixture, so that faster hooks never weaken the gate.
17. As an adopter with Python or TypeScript, I want my per-edit hook to
    stop running my whole test suite, so that editing stays responsive.
18. As a reader of the docs, I want each stage's checks in one place,
    with the test-cache hazard, so that I can reason about hook timings.

## Implementation Decisions

- **Templates, not local overrides** (Q1). Every change is made in the
  generated templates for every standard (Go, Python, TypeScript, the
  monorepo and its components), the Claude settings template, the
  lefthook templates, the conformance `audit` template and the setup
  skill's reference Taskfiles. This repository picks them up through
  `task sync`, and the examples are re-synced.
- **`hook:check` shape.** Its command keeps reading the payload once. A
  non-empty `agent_id` (both JSON spacings, as the path filter already
  handles) exits 0 before the path filter. The checks it runs become the
  standard's typecheck and lint tasks; the `test:short` / `test` call is
  removed. `asyncRewake` stays: in the main session, a broken build
  should still wake it.
- **Green stamp as two small tasks.** One task exits 0 when the current
  tree hash equals the stored stamp, non-zero otherwise (and when there
  is no Git repository or no stamp). The other writes the hash and always
  exits 0. The hash is computed by git (`hash-object`) over HEAD, the
  binary diff against HEAD, and the untracked non-ignored files' names
  and contents. The stamp file sits at a `git rev-parse --git-path` path,
  so it never appears in `git status` and linked worktrees keep separate
  stamps.
- **Stop command.** The settings template's Stop command becomes, in
  outline: the stamp check, else (`verify:fast`, then record the stamp),
  else `hook:done`. `task verify:fast 1>&2 </dev/null` stays a literal
  simple command, so A-05's static part finds it. The payload on stdin
  still reaches `hook:done`; the stamp tasks read no stdin. This
  repository's `vibe-guard relock` prefix stays where it is.
- **lefthook.** pre-commit: one command, `task fmt:check`. pre-push: one
  command, `unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR;
  task verify:fast`, with the comment that explains why. `task test`
  keeps its 30-minute timeout for people and CI.
- **The `audit` task's self-build branch** (Q8) runs before the PATH
  branch, guarded by both the `cmd/vibe` directory and the module line in
  `go.mod`. It runs vibe with `go run ./cmd/vibe`, so the binary always
  matches the checkout. This deliberately amends spec 0019's invariant
  "the audit reaches vibe only from PATH or the pinned module": the third
  path exists only in VibeConform's own repository. ADR 0008 is the
  precedent; its text gains this case.
- **Setup action** (Q14). The source is resolved first. In the workspace
  case the version check is skipped and the version is printed as
  information. Release and go-install sources keep today's checks.
- **Shared `GIT_*` clearing in tests stays per package.** The seven
  `TestMain`s keep clearing the variables; `go test` also runs outside
  hooks. No shared helper is added in this spec.
- **No contract or ADR beyond ADR 0008's update.** No new production
  dependency.

## Testing Decisions

A good test here runs the generated file the way its runner does and
asserts on what happened (exit code, output, whether a check ran), not
on the template's text, except where the shape *is* the contract (the
literal `task verify:fast` call).

- **Generated hook tasks** (Go, Python, TypeScript, monorepo): run with
  `task` in fixture repositories, payloads on stdin. `hook:check` with
  an `agent_id` runs nothing; without one it runs typecheck and lint and
  no tests (a fixture test that would fail proves it is not run).
  Prior art: `TestHookCheckSkipsEditsThatCannotMatter`,
  `TestHookTasksShape`.
- **Green stamp and Stop gate**: in a fixture repository, a green Stop
  run records a stamp and a second Stop runs nothing; editing, staging,
  adding an untracked file, or committing makes the next Stop run;
  a failing run records nothing; block-once still holds; two linked
  worktrees keep separate stamps. Prior art: `TestHookStopBlocksOnce`,
  the Stop matrix in `internal/module/agents/claude`.
- **lefthook**: shape test of the generated `lefthook.yml` for every
  standard; and, in full mode only, `lefthook run pre-push` from a linked
  worktree of a fixture whose `verify:fast` prints whether `GIT_DIR` is
  set. Prior art: `TestLefthookLocalMerge`, `TestHookContextLinkedWorktree`.
- **Audit task**: the conformance template tests. A fixture with
  VibeConform's module path and a `cmd/vibe` runs the source build; a
  fixture with another module path, or without `cmd/vibe`, never does.
  `TestAuditReachesVibeOnlyFromPathOrPinnedModule` is amended to allow
  exactly this branch. Prior art: `conformance_test.go`.
- **Setup action**: the CI job "Setup action / workspace and refusals"
  gains two cases: the workspace source with `dev` and with a `+dirty`
  version succeeds; a non-workspace source with `+dirty` is still
  refused. CI only; no local seam.
- **Contract end to end**: the contract probe tests for the Go, Python
  and TypeScript fixtures (`internal/contract/probetest`) keep A-05 and
  V-02 conformant, and `vibe audit --config vibe.v2.yaml` on this
  repository reports H-02 as a pass.

## Non-goals

- The `hook:format` subdirectory bug (#129).
- Changing contract rules, or making the Stop gate build + lint only
  (that would fail A-05 and need a contract MAJOR change).
- A short-mode convention for Python or TypeScript (Q10).
- Turning off Go's test cache (Q7).
- Lowering the gate's process priority, or skipping it while sub-agents
  run (Q12).
- Codex hooks, which stay suspended (spec 0024).
- A shared test helper for clearing `GIT_*`.

## Acceptance Criteria

- [ ] In every standard and setup skill reference, `hook:check` runs
      typecheck and lint and no tests, and exits 0 without running
      anything for a payload with a non-empty `agent_id`. Covered by
      tests that run the generated task.
- [ ] The Stop gate skips `verify:fast` when the tree matches the green
      stamp, runs it after any tracked, staged, untracked or committed
      change, never records a stamp for a failing run, keeps separate
      stamps per linked worktree, and still blocks exactly once.
- [ ] The generated pre-commit runs `task fmt:check`; pre-push clears
      the four `GIT_*` variables and runs `task verify:fast`. A full-mode
      test shows `GIT_DIR` unset inside pre-push from a linked worktree.
- [ ] `vibe audit --config vibe.v2.yaml` on this repository: conformant,
      nothing undecided, H-02 passes; the other SHOULD findings are no
      worse than at 6b88b8e. The probe tests keep A-05 and V-02
      conformant for the Go, Python and TypeScript fixtures.
- [ ] With an older `vibe` first on PATH, `task audit` in this repository
      reaches a verdict; in a fixture with another module path it still
      refuses the older binary.
- [ ] The setup action accepts `dev` and a `+dirty` version for the
      workspace source and refuses `+dirty` for the others (CI job
      green).
- [ ] `docs/usage.md`, ADR 0008, `warnUnpinnable`'s comment and the 0056
      report's friction table are updated as in Desired Behavior 15–17.
- [ ] `task verify`, `task audit` and CI pass.

## Decisions (grilling, 2026-10-21)

| Q | Decision |
|---|---|
| Q1 | The generated templates, not this repository alone |
| Q2 | `hook:check` runs typecheck + lint, no tests |
| Q3 | The Stop gate keeps short tests (A-05) |
| Q4 | The Stop gate skips when the tree equals the last green stamp |
| Q5 | pre-push runs short tests, not the full suite |
| Q6 | The `hook:format` bug is a separate issue (#129) |
| Q7 | Go's test cache stays on; the hazard is documented |
| Q8 | VibeConform's `task audit` runs the checkout's vibe |
| Q9 | pre-push clears `GIT_DIR` and friends |
| Q10 | Python and TypeScript `verify:fast` keep their full tests |
| Q11 | pre-push does not read the green stamp |
| Q12 | No further CPU measures for the Stop gate |
| Q13 | Sub-agents skip `hook:check`; no `SubagentStop` gate |
| Q14 | The setup action resolves the source before validating the version |
| Q15 | The hooks call `task fmt:check` and `task verify:fast` (H-02) |
| Q16 | One spec and one PR |
