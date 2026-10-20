# Spec 0056: Faster agent hooks and Go tests

Status: accepted (2026-10-20). Absorbs issue #125. Amends spec 0023
(agent lifecycle hooks), sections 4.3 and 4.4 and its "Known
limitations". Scope settled with the user on 2026-10-20.

## Problem

On Windows, an agent session in this repository spends 40 minutes and
more in its hooks. The cause is the test suite, run again and again:

- **The suite is slow.** One uncached `go test -count=1 ./...` took
  5m17s on a 24-core Windows 11 machine at rest (2026-10-20). Five
  tests account for most of it:

  | Test | Package | Elapsed |
  |---|---|---|
  | `TestHookStopBlocksOnce` | `internal/module` | 236s, serial |
  | `TestGuardProbesOldGuards`, `TestGuardProbesVibeGuard` | `internal/contract` | ~236s |
  | `TestDefectProbes*`, `TestRuleV01OtherLanguages` | `internal/contract/probetest` | ~220s |
  | `TestContractAuditNeverWrites` | `internal/cli` | 210s |
  | `TestGuardCorpusGo` (+ Node, Python) | `internal/module/agents/claude` | 194s (+57s), serial |

  The other ~2,900 tests and subtests take seconds. The probe tests run
  `task verify`, `golangci-lint` and `go test` inside `go test`, so they
  compete for the same CPUs.
- **Every edit starts the whole suite in the background.** `hook:check`
  runs after every Write and Edit, including edits of files outside the
  repository (the agent's memory, its scratchpad) and of ignored files.
  Quick successive edits start overlapping runs (spec 0023, "Known
  limitations").
- **The Stop hook runs it once more** through `verify:fast`.
- **`go test ./...` outgrew Go's default 10-minute timeout** in the
  pre-push hook (#125). Pushes need `GOFLAGS=-timeout=30m`.

Adopters get the same hook design from the generated templates and from
the setup skill's reference files, so a Go repository with slow tests has
the same problem.

## Constraints

- Contract rules V-01, V-02 and A-05 (spec 0051) still pass: `verify`
  and `verify-fast` fail on each planted defect, and the Stop hook still
  calls `verify-fast`. A planted failing test does not call
  `testing.Short()`, so short mode still runs it.
- `task verify`, `task test`, the pre-push hook and CI keep running the
  full suite. Short mode is for `verify:fast` and `hook:check` only.
- Spec 0023's interface stays: the same `task -x hook:<name>` commands,
  the same events, matchers and hook timeouts in `.claude/settings.json`.
- Hook tasks use only Task's built-in shell, git and the tools the
  hooks already use. No `jq`, no new dependency (AGENTS.md).
- Managed files change through their templates (`internal/module/...`),
  then a rebuild and `vibe sync` (AGENTS.md). The setup skill's reference
  files (`internal/setupskill/skill/reference/`) change to match.
- No test is weakened. A test that short mode skips still runs in full
  mode, and CI runs full mode.

## Assumptions

- Claude Code passes the hook payload on stdin to `task -x hook:check`,
  with the edited path in `tool_input.file_path` (Write, Edit, MultiEdit)
  or `tool_input.notebook_path` (NotebookEdit), JSON-escaped (Windows
  backslashes doubled). `hook:done` already reads stdin the same way.
- `git check-ignore` tells the three cases apart: an ignored path
  (exit 0), a path in the repository that is not ignored (exit 1), and a
  path outside the repository (exit 128, "outside repository"), on
  Windows paths too. Not yet checked on Windows.
- Without the five slow tests, `go test -short -count=1 ./...` takes
  under 90 seconds on the machine above. Not yet measured.
- 30 minutes covers the slowest full run seen (1,020s under load, #125).

## Desired Behavior

### Generated standards and setup skill references

1. **`hook:check` skips edits that cannot affect the checks.** It reads
   the edited path from the payload. If the path is outside the
   repository or ignored by git, it exits 0 without running a check.
   Otherwise it runs as before. If it cannot read a path from the payload
   (no path, a malformed payload, a tool without a path such as Codex's
   `apply_patch`), it runs the checks. This holds for every standard: Go,
   TypeScript, Python and the monorepo.
2. **Go: `verify:fast` and `hook:check` run tests in short mode**
   (`go test -short`). For a Go monorepo component, the same applies to
   its tasks.
3. **Go: full test runs have an explicit 30-minute timeout.** This covers
   `task test`, `task test:race` and this repository's pre-push `go test`.
   Nobody needs `GOFLAGS` to push.
4. `task verify` and CI behave as before: full mode, every test.

### This repository's tests

5. The tests that run the contract probes, the guard corpus through
   subprocesses, or the Stop-hook matrix skip in short mode with a
   message that names `task verify` as the place they run. Each skip is
   one `testing.Short()` check at the top of the test or its fixture
   helper.
6. Serial table tests that start one subprocess per case,
   `TestHookStopBlocksOnce` and the guard corpus tests, run their cases
   in parallel, each with its own temporary directory.
7. Full mode runs exactly the tests it runs today. Only their order and
   parallelism change.

### Documentation

8. Spec 0023 gets a dated note pointing here for 4.3, 4.4 and the
   overlapping-runs limitation. `docs/usage.md` ("Agent hooks") says what
   `hook:check` skips and that `verify:fast` runs Go tests in short mode.

## Non-goals

- **Debouncing overlapping `hook:check` runs.** Path filtering and short
  mode cut the cost first. A lock on Windows needs stale-lock handling
  after a hook timeout. Revisit if measurements still show overlap
  hurting.
- **Filtering by file type** (`*.go` only). This repository's tests read
  embedded templates, testdata and `.claude/hooks/policy.json`, so a type
  list would skip edits that change results.
- **Short mode outside Go.** pytest and the TypeScript test runners have
  no common equivalent. Their `hook:check` gets the path filter only.
- **Making the probe tests themselves faster** (sharing fixtures, fewer
  nested `task verify` runs). A later spec, if full mode is still too
  slow for pre-push.
- A contract change. V-02 does not measure speed, and no rule reads
  `hook:check`.

## Acceptance Criteria

- [ ] `hook:check` exits 0 without running a check for a payload whose
      path is outside the repository, and for one whose path is ignored
      by git. It runs the checks for a tracked path, for an untracked
      path that is not ignored, and for a payload with no path. Covered
      by tests for every standard's template and each setup skill
      reference Taskfile, with Windows-style escaped paths and a path
      with a space.
- [ ] In the Go standard, the monorepo Go component, and both Go
      reference Taskfiles, `verify:fast` and `hook:check` run
      `go test -short`, and `test` and `test:race` run with
      `-timeout 30m`. This repository's pre-push `go test` has the same
      timeout.
- [ ] Contract probes still pass on the reference fixtures: V-01, V-02
      and A-05 are conformant for the Go, Python and TypeScript probe
      fixtures.
- [ ] `go test -short ./...` skips the tests listed in Desired Behavior
      5. `go test ./...` runs the same set of tests as before this change
      (compared by test name lists from `go test -json`, before and
      after).
- [ ] On the machine above, at rest: `go test -short -count=1 ./...`
      takes under 90s, and `go test -count=1 ./...` takes no longer than
      the 5m17s baseline. Both numbers are recorded in the PR.
- [ ] Editing a file outside the repository (agent memory) in a live
      Claude Code session starts no test run. Checked by hand on Windows.
- [ ] `vibe sync` leaves this repository conformant: `task audit` passes,
      `vibe audit` (contract) is conformant, `task verify` passes.
- [ ] Spec 0023 and `docs/usage.md` are updated as in Desired Behavior 8.
- [ ] #125 is closed by the PR.
