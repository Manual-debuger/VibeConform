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

## Open: contract rule A-05 cannot be decided

`vibe audit --config vibe.v2.yaml` ran twice: at 880ce12 under load, and
at 88b2093 on a quiet machine. Both runs ended "cannot decide". Every
other MUST rule passes. The five SHOULD findings (H-02, C-05, A-06, D-03,
R-02) are the same as on the baseline, 2055da5.

A-05 runs the Stop hook in a probe copy with a planted failing test
(`cmd/vibe/vibe_probe_test.go`). The hook command times out at the hook's
own 600s. Reproduced in a fresh worktree of 88b2093: `go test -short
./...` took 634s, and `internal/cli` took 620s of that.
- The cli tests themselves finished in 53s: the `ok … 53.450s` line was
  printed at 02:28:00.
- The package's `pass` event came 9m27s later, with no other test events
  in between.

`go test` waited for something that outlived the test binary, most likely
a child process that inherited its output pipe. It did not reproduce when
`internal/cli` ran alone in the same worktree (60s). It does not happen in
the main checkout (53–55s for the whole suite). Suspects, none confirmed:
- git's detached auto-gc: a linked worktree shares the object store,
  which held 6,216 loose objects against gc.auto's default of 6,700;
- another detached process that a cli test starts in a fresh checkout.

Next step: find the process. List the processes (Win32_Process) while the
gap is open, in a fresh copy and with the whole `./...` run. Before this
branch, `verify:fast` ran the full suite, so A-05 may not have been
decidable on main either. That is not checked.

## Multi-agent friction this work exposed

The session ran up to five implementer agents in parallel worktrees. What
got in their way:

| # | Friction | Status after spec 0056 |
|---|---|---|
| 1 | golangci-lint takes one machine-wide lock; a second run exited 3 | Fixed: `allow-serial-runners` |
| 2 | A sub-agent's async `hook:check` failure (`asyncRewake`) wakes the parent session. TDD red phases make these routine; the orchestrator got about 20 of them. | Open |
| 3 | Every edit in every worktree runs typecheck, the full lint and the short tests | Reduced: path filter and short mode. Still the full set for any edit inside a repository. |
| 4 | The orchestrator's Stop gate (`verify:fast`) competes with the agents for the machine and spoiled two timing runs | Open |
| 5 | Every sync rewrites `vibe_version` (a timestamp and a commit) in `.vibe/state.yaml`, so parallel branches always conflict there | Reduced: `task sync` re-syncs, and restores a change to `vibe_version` alone |
| 6 | A `vibe` on PATH that is older than the build that synced the branch refuses an audit verdict (spec 0019). Every agent had to put its own `bin/` first. | Open |
| 7 | vibe-guard refuses improvised rebuild-and-sync commands (`dynamic-command`) | Fixed: `task sync` |
| 8 | A pre-push from a linked worktree leaks `GIT_DIR` into tests that use git | Open (worked around by pushing only from the main checkout) |
| 9 | A sync from a dirty tree records a `+dirty` `vibe_version`, which CI cannot install | Open (corrected by hand twice in this PR) |
| 10 | A worktree that was just created failed git's identity check once, under load | Not reproduced |

Items 2, 4, 6, 8 and 9 are the input for the next spec. That spec decides
what hooks, pre-push and CI each run, and when.
