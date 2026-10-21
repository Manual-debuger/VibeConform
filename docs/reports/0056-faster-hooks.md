# Spec 0056 report: short-mode diagnosis, results, and multi-agent friction

Spec: `docs/specs/0056-faster-agent-hooks.md` (amended 2026-10-20).
PR: #128. Machine: 24-core Windows 11, Go 1.27.0, Windows Defender
real-time protection on.

## Why the first plan missed 90 seconds

The spec first assumed that skipping five slow test groups would bring
`go test -short -count=1 ./...` under 90s. Measured on 2026-10-20 at
3aa5c30, on a quiet machine, with `-skip` standing in for the skips:

| Skip set | Short-mode wall |
|---|---|
| The five groups the spec named | 223s, 258s |
| Every test in the amended Desired Behavior 5 (22 tests) | 183s, 208s |
| Floor: every cli rule and config table also skipped | 90s |

`internal/cli` was the critical path, at about 190s on its own. Its short
run started about 7,000 git processes:
- about 900 table cases, each running `git init` and `git add` to build
  its fixture;
- five more git calls per case in `contract.OpenRepo`.

Starting git took 85% of each case's time. This machine starts about
20–130 processes per second, whatever the concurrency. `-parallel` 24, 8
and 4 all took about 190s, with the CPU at 20–30%. Running the tests in
parallel therefore could not help. Removing the process starts could.

## What changed

- **Desired Behavior 5:** 22 tests skip in short mode. They are the
  contract probes, the guard corpus run through subprocesses, and the
  Stop-hook matrix.
- **Desired Behavior 6:** the cli tables build their fixtures without git.
  The audit takes the fixture's file list through `contract.NewRepo` and
  `contractSeams.openRepo`. About 17 cases with untracked files, and the
  tests of default branches, git failures and golden reports, keep real
  git. 60 serial cli tests became parallel. About 100 tests stay serial,
  because `runSyncIn` swaps the package variable `installGitHooks`. The
  skill drift check builds each reference configuration once.
- **Desired Behavior 6a:** the Stop-hook matrix and the guard corpus run
  their cases in parallel, at most 4 at a time.

## Results (2026-10-21, 880ce12, quiet machine)

| Run | Before | After |
|---|---|---|
| `go test -short -count=1 ./...` | 289s (skips simulated) | **53s, 55s** |
| `go test -count=1 ./...` | 5m17s (317s) | **214s** |
| `internal/cli` short, alone | 114–164s (in pairs, under load) | 64–66s |
| Stop matrix, full mode (under load) | 13.9s | 4.8s |
| Guard corpus, full mode (under load) | 40.1s | 12.3s |
| Pre-push (lint + full tests, under load) | 712s, which hit the 10m timeout (#125) | 376s |

Full-mode test names, compared through `go test -json`:
- every one of the 3,480 baseline names still passes;
- 137 names are new tests;
- four tests changed from skip to pass, because the baseline worktree did
  not have the examples' tools installed;
- five subtests are named after random temporary directories and so match
  only by count.

## Contract rule A-05: the test cache, not a process

`vibe audit --config vibe.v2.yaml` ran twice: at 880ce12 under load, and
at 88b2093 on a quiet machine. Both runs ended "cannot decide" on A-05.
Every other MUST rule passed. The five SHOULD findings (H-02, C-05, A-06,
D-03, R-02) are the same as on the baseline, 2055da5.

A-05 runs the Stop hook in a probe copy with a planted failing test
(`cmd/vibe/vibe_probe_test.go`), and the hook timed out at its own 600s.
In `internal/cli` the `ok … 53s` line came at once and the package's
`pass` event 9m27s later. The first guess was a child process holding
the output pipe. It was not one. Diagnosed on 2026-10-21:
- **Nothing outlived the test binary.** With Win32_Process listed during
  the gap, `go test` had no children and there were no orphans. cmd/go
  gives a test binary's pipes 60s (`WaitDelay`, a tenth of `-timeout`)
  and measures the time it prints after they close. A held pipe would
  have shown as "Test I/O incomplete", and the printed time would include
  the wait.
- **The test cache.** Without `-count=1`, cmd/go reads the binary's test
  log after the run (`saveOutput`) and before a cache lookup
  (`tryCache`), and hashes every entry. For each `stat` or `open` outside
  the module it calls `search.InDir`, which calls `filepath.EvalSymlinks`
  on the path and on the module root. `test:short` runs `go test -short
  ./...` with no `-count`, so the Stop hook pays this; the timings above
  used `-count=1` and did not.
- **The cli tests wrote 661,037 entries,** 604,507 of them `stat`. Every
  sync warns about missing tools (`warnMissingTools`), and each search on
  Windows stats all 59 PATH directories with every PATHEXT extension.
  Hundreds of syncs repeated the same ten searches.
- **The module root's depth sets the price.** The same `go test -short
  ./internal/cli`, with the cache on: 73s from `ok` to `pass` in
  `D:\VibeConform`, 10m08s in a probe copy 11 directories deep under
  `%TEMP%`. A cache hit pays it too, because the lookup hashes the stored
  log again.

The fix is in the tests: the cli `TestMain` memoizes `lookPath`, so each
tool is searched once per binary. The log fell to 98,895 entries in short
mode (113,017 in full mode), and the cli run from 61s to 45s. The gap fell
to 16s in the main checkout. The Stop hook in a probe copy took 119s,
against 636s, and still exited 2. As a regression test, the cli
`TestMain` fails the package when its test log holds more than 150,000
entries (`checkTestLog`). It checks only when go test passes a log, which
is the cached mode that the Stop hook uses.

Main had the same problem, and a larger one. Its `verify:fast` ran the
full suite, and main's cli tests (a365ed6) write 818,572 entries in full
mode. That is inferred from the log size; the A-05 probe was not run
against main.

## Multi-agent friction this work exposed

The session ran up to five implementer agents in parallel worktrees. What
got in their way:

Items 2, 4, 6, 8 and 9 were left open by spec 0056 and are resolved by
spec 0057 (#130), which decides what each hook stage runs.

| # | Friction | Status |
|---|---|---|
| 1 | golangci-lint takes one machine-wide lock; a second run exited 3 | Fixed: `allow-serial-runners` |
| 2 | A sub-agent's async `hook:check` failure (`asyncRewake`) wakes the parent session. TDD red phases make these routine; the orchestrator got about 20 of them. | Resolved by spec 0057: `hook:check` runs nothing for a payload with an `agent_id`, and runs no tests at all |
| 3 | Every edit in every worktree runs typecheck, the full lint and the short tests | Reduced: path filter and short mode. Still the full set for any edit inside a repository. |
| 4 | The orchestrator's Stop gate (`verify:fast`) competes with the agents for the machine and spoiled two timing runs | Resolved by spec 0057: the Stop gate skips `verify:fast` when the tree equals the last green run (the green stamp) |
| 5 | Every sync rewrites `vibe_version` (a timestamp and a commit) in `.vibe/state.yaml`, so parallel branches always conflict there | Reduced: `task sync` re-syncs, and restores a change to `vibe_version` alone |
| 6 | A `vibe` on PATH that is older than the build that synced the branch refuses an audit verdict (spec 0019). Every agent had to put its own `bin/` first. | Resolved by spec 0057: in VibeConform's own repository, `task audit` runs the checkout's vibe from source |
| 7 | vibe-guard refuses improvised rebuild-and-sync commands (`dynamic-command`) | Fixed: `task sync` |
| 8 | A pre-push from a linked worktree leaks `GIT_DIR` into tests that use git | Resolved by spec 0057: pre-push unsets `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE` and `GIT_COMMON_DIR` before `task verify:fast` |
| 9 | A sync from a dirty tree records a `+dirty` `vibe_version`, which CI cannot install | Resolved by spec 0057: when the setup action builds from VibeConform's own workspace, the recorded version is information only |
| 10 | A worktree that was just created failed git's identity check once, under load | Not reproduced |

Items 2, 4, 6, 8 and 9 are the input for the next spec. That spec decides
what hooks, pre-push and CI each run, and when.
