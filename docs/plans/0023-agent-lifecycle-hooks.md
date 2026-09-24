# Plan 0023: Agent lifecycle hooks beyond the guard

See `docs/specs/0023-agent-lifecycle-hooks.md` for the accepted scope.
Accepted at spec review (2026-09-24), with the recommended option taken
for every resolved question.

Branch: `feature/agent-lifecycle-hooks`, off `main`, one pull request.

## Repository impact

| Area | Change |
|---|---|
| `internal/module/{repotooling,tsrepotooling,pyrepotooling}/templates/Taskfile.yml` | cache flags on existing tasks; new `verify:fast`, `hook:context`, `hook:format`, `hook:check`, `hook:done`; minimum Task version note if it rises |
| `internal/module/{repotooling,tsrepotooling,pyrepotooling}/*_test.go` | task shape tests, following `TestHookGuardTask` |
| `internal/module/agents/templates/claude/settings.json` | four more events |
| `internal/module/agents/templates/codex/hooks.json` | four more events, Codex matcher and `async` |
| `internal/module/agents/agents.go` | a command constant per event, next to `GuardCommand` |
| `internal/module/agents/agents_test.go` | every command is `task -x hook:<name>`; `asyncRewake` vs `async`; matchers |
| `internal/module/agents/testdata/` | new `stop_corpus.json` |
| `internal/module/agents/*_test.go` | `Stop` corpus run in-process through Task and end to end |
| `internal/module/hooks_test.go` (new) | cross-module invariants: the `hook:context` allowlist, cacheable `go test` |
| `internal/standard/wiring_test.go` | `TestAgentConfigWiring` covers every event, not only `hook:guard` |
| `.github/workflows/hook-guard.yml` | runs the new end-to-end tests on Linux and Windows |
| `.github/workflows/examples.yml` | runs `task verify:fast` and `task hook:context` in each example |
| Generated files (root, `examples/typescript`, `examples/python`) | re-synced, plus `.vibe/state.yaml` |
| Docs | `docs/usage.md`, `.codex/README.md`, `AGENTS.md`, `docs/architecture/principles.md`, spec 0023 status |

Dependency direction is unchanged. `agent-config` still only names tasks,
and the repo-tooling modules still define them. `internal/module` and
`internal/resource` gain no new imports, and nothing gains a dependency.

## Order of work

Six commits, each leaving the tree green. The spikes come first because
four of the resolved questions choose a mechanism the templates depend on.

0. **Spikes**, no commit: answer the checks in resolved questions 2, 3, 5,
   6 and the `Stop` pattern, and record the results below before writing
   templates.
1. **C1: caches and `verify:fast`.** No hook changes yet, so this commit is
   useful on its own.
2. **C2: the four hook tasks** in the three Taskfile templates.
3. **C3: agent-config registers the events.** It has to come after C2,
   because the wiring test fails when a config names a task that doesn't
   exist.
4. **C4: CI.** Extend `hook-guard.yml` and `examples.yml`.
5. **C5: docs.**
6. **C6: manual canaries, verification record, spec marked implemented.**

**Dogfooding hazard.** This repository is on `prod-go`. Once C3 is synced
at the root, the agent session doing the work picks up the new hooks,
possibly mid-session (as spec 0021 observed). From then on every stop runs
`verify:fast`. That is the intended behaviour, but a broken intermediate
state will block stops. Sync the root last within C3, after the examples
pass.

## Checklist

### 0. Spikes (record results under "Spike results")

Under Task v3.53.1 (the CI pin) and the minimum version the templates
declare, on Windows 11 and Linux:

- [x] **`xargs -r`** (question 2): Task's built-in `xargs` with empty
      stdin, with and without `-r`. Also a path containing a space. If
      `-r` is unsupported, settle the empty-list guard form.
- [x] **Template functions** (question 3): `splitLines`, `len`, slicing
      (`slice` or `until`/`range` with an index), and `base .ROOT_DIR`.
      Find the lowest Task version that has all of them.
- [x] **`Stop` pattern**: `case "$(cat)" in …` inside a `cmds` entry
      reads the hook's stdin on both OSes. Test with compact JSON, spaced
      JSON, and the field missing.
- [x] **Git state probes**: `git rev-parse --git-path MERGE_HEAD` and the
      rest, with the shell's `test -e`, in a worktree and a normal clone.
- [x] **`uv python find`** (question 5): on a machine or CI runner with no
      uv-managed Python and `UV_PYTHON_DOWNLOADS` unset, confirm it
      downloads nothing (watch `uv python dir` before and after).
- [x] **`hook:format` latency** (question 6): a warm run for a one-file
      change in each example and at the root, per step. Any step over the
      1-second budget moves to `hook:check`, and the spec's 4.2 table is
      amended to match.
- [x] **`git status` timing** (question 4): `hook:context` here, and on a
      synthetic checkout of about 100k files, against the 30-second
      timeout.

### C1: caches and `verify:fast`

Regression tests first, watched failing:

- [ ] Each repo-tooling test: `verify:fast` exists, has a `desc`, and runs
      exactly the tasks the spec's section 2 table lists, in `verify`'s
      order.
- [ ] `tsrepotooling_test.go`: `lint` passes `--cache --cache-location
      node_modules/.cache/eslint/`, `fmt` and `fmt:check` pass `--cache`,
      and `typecheck` passes `--incremental --tsBuildInfoFile
      node_modules/.cache/tsc/tsbuildinfo`. Extend
      `TestTaskfileUsesPnpm`'s neighbours, don't weaken it.
- [ ] `hooks_test.go`: every `go test` command in `prod-go`'s Taskfile
      passes a package argument and none of the uncacheable flags
      (`-count`, `-coverprofile`, …) except in `test:race`, which is not in
      `verify:fast`.

Then implementation:

- [ ] Cache flags in `tsrepotooling`'s `Taskfile.yml`. `lefthook.yml` is
      left alone: `pre-commit` runs on staged files only, and caches there
      buy little.
- [ ] `verify:fast` in all three templates. `verify` calls the same
      subtasks, and its `desc` gains a pointer to `verify:fast`.
- [ ] Rebuild `vibe` (fresh binary, per the CRLF / stale-embed hazard).
      Sync `examples/typescript`, `examples/python`, then the root. Run
      `task verify` and `task audit` in each. Run `task verify:fast` twice
      and confirm the second run is cached (`go test` prints `(cached)`;
      ESLint and `tsc` are faster).
- [ ] `git status` is clean after the runs: no cache file lands outside an
      ignored location.

### C2: the four hook tasks

Regression tests first:

- [ ] Each repo-tooling test, following `TestHookGuardTask`: `hook:context`,
      `hook:format`, `hook:check`, `hook:done` exist, have no `desc`, and
      are `silent`. `hook:check` and `hook:done` both run `verify:fast`.
      Every `hook:format` cmd ends in `|| exit 2`.
- [ ] `hooks_test.go`, context allowlist: every command word in
      `hook:context` (its cmds and its `sh:` variables, split on `|`, `;`,
      `&&`, `||`) is one of `git`, `go`, `node`, `pnpm`, `uv`, `task`,
      `test`, `echo`, `printf`. Otherwise the test fails. It also fails if
      `hook:context` calls another task.
- [ ] `hooks_test.go`, format scope: every `hook:format` cmd takes its
      files from `git diff --name-only --diff-filter=d HEAD` plus
      `git ls-files --others --exclude-standard`, with a pathspec that
      matches the extension set its `fmt` task uses.
- [ ] `stop_corpus.json` plus an in-process test: each case (field true,
      compact and spaced JSON, false, missing, invalid JSON) with a stub
      `verify:fast` that fails. Expected: exit 2 unless
      `stop_hook_active` is true, in which case exit 0 with the failure
      still printed.
- [ ] End-to-end tests, gated by environment variables like
      `TestGuardCorpusEndToEnd`: the `Stop` corpus through
      `task -x hook:done`; `hook:format` on an empty changed set (exit 0,
      nothing run) and on one unformatted file (file comes back formatted);
      `hook:context` output has every spec line and is under 10,000
      characters with more than 20 dirty files.

Then implementation:

- [ ] The four tasks in each template, using the spike results.
- [ ] Rebuild and sync, examples first, then the root. `task verify` and
      `task audit` in each.

### C3: agent-config registers the events

Regression tests first:

- [ ] `agents_test.go`: every hook command in both configs is exactly
      `task -x hook:<name>`, extending `TestAgentConfigsCallTaskWithExitCode`.
      The `PostToolUse` matchers are `Write|Edit|MultiEdit|NotebookEdit`
      for Claude Code and `apply_patch|Edit|Write` for Codex. The
      `hook:check` handler has `asyncRewake: true` and no `async` for
      Claude Code, and `async: true` for Codex. The timeouts match the
      spec's section 1 table.
- [ ] `TestCodexHooksMatchDocumentedSchema` decodes every event strictly,
      not only `PreToolUse`.
- [ ] `wiring_test.go`: `TestAgentConfigWiring` walks every command in
      both configs and requires the named task in the standard's
      `Taskfile.yml`. The guard-specific checks (guard file and
      `policy.json` resolved) stay as they are.

Then implementation:

- [ ] Both templates, and the constants in `agents.go`.
- [ ] Rebuild; sync the examples, then the root last (dogfooding hazard).
      `task verify` and `task audit` in each.

### C4: CI

- [ ] `hook-guard.yml`: run the new end-to-end tests next to
      `TestGuardCorpusEndToEnd` on the existing standard × OS matrix. Keep
      the job and check names unchanged, so branch protection is
      untouched. Update the header comment to say the workflow now covers
      all agent hooks.
- [ ] `examples.yml`: `task verify:fast` and `task hook:context` in each
      example, after the existing `task verify`.
- [ ] `actionlint` passes (`task workflows:lint`).

### C5: docs

- [ ] `docs/usage.md`:
  - "The agent guard" becomes "Agent hooks", with the guard as one
    subsection. Add one subsection per new hook: what it runs, when it
    blocks, and how to try it by hand, like the guard's `echo … | task -x
    hook:guard` example.
  - `verify:fast` in the task lists and in "What `prod-go/v1` manages" and
    its TS/Python counterpart.
  - The cache locations table.
  - A note that Vitest or Jest users can make `pnpm test` changed-only in
    their own `test` script.
  - `core.fsmonitor` / `core.untrackedCache` for large repositories.
  - Known limitations from the spec.
  - The manual canary for each hook.
- [ ] `.codex/README.md`: the new events, `async` delivery, and any gap
      the Codex canary finds.
- [ ] `AGENTS.md`: the "run `task verify` and `task audit`" rule stays,
      but gains a sentence saying the `Stop` hook now runs `verify:fast`
      and that `audit` is still manual. The Guardrails section mentions
      the other hooks.
- [ ] `docs/architecture/principles.md`: add a principle-1 row for the
      `Stop` gate, noting its once-per-stop soft spot.
- [ ] Spec 0023 status: accepted and implemented. Amend it wherever the
      spikes changed a mechanism, with an "*Added during implementation:*"
      note, as spec 0021 did.

### C6: verification

- [ ] `go test ./...`, `task verify` and `task audit` at the root and in
      both examples.
- [ ] **Claude Code canaries** in a fresh session in this repository:
      session start shows the context; an edit that leaves a file
      unformatted comes back formatted; a test broken on purpose wakes the
      session through `asyncRewake`; stopping with that test still broken
      is blocked once, then let through.
- [ ] **Codex canary** (resolved question 1): the same broken test
      reaches the model through the async hook. If it doesn't, apply the
      fallback the spec names and record which one.
- [ ] Push. `CI / gate`, `Conformance / audit`, `Examples / gate` and
      `Hook guard / gate` are green.

## Spike results (2026-09-24)

Run on Windows 11 (Task v3.53.1, Git for Windows) and on Linux (WSL
Ubuntu, Task v3.53.1 and v3.39.0 built into a scratch `GOBIN`).

- **`xargs`** (question 2). *Windows:* Task's built-in `xargs` (u-root)
  wins over Git's `xargs.exe` on `PATH`. It has no `-r` ("flag provided but
  not defined: -r", exit 1). It runs nothing on empty input, splits on
  whitespace, and supports `-0`. *Linux:* Task has no built-in core
  utilities there, so GNU `xargs` runs, and it **runs the command once on
  empty input** (`RAN[]`). `-r` therefore can't be used on Windows, and
  isn't implied on Linux. **Use the fallback:** each cmd skips when the
  git list is empty, then pipes `git … -z` into `xargs -0` so paths with
  spaces survive. Spec 4.2 is amended.
- **Template functions** (question 3). `base .ROOT_DIR`, `splitLines`,
  `trim`, `len`, `slice`, `join`, `range` with an index, and `sub` all work
  on both OSes, and on Task v3.39.0. **The minimum Task version stays
  v3.39.0.**
- **`Stop` pattern.** `case "$(cat)" in …` inside a cmd reads the hook's
  stdin on both OSes and on v3.39.0. Compact JSON, `": true"` with a space,
  and pretty-printed JSON across lines match. `false`, a missing field, and
  invalid JSON don't.
- **Git state.** `git rev-parse --git-path <name>` plus `[ -e … ]` detects
  a merge in progress in a normal clone and in a linked worktree, and
  clears after `git merge --abort`.
- **`uv python find`** (question 5). With an empty `HOME` and a `PATH`
  holding only `uv` (Linux, uv 0.12.10) it exits 2 with "No interpreter
  found in virtual environments, managed installations, or search path",
  and the managed-Python directory is never created. **It doesn't
  download.** On Windows the same isolation isn't possible, because uv
  also finds interpreters through the registry (PEP 514), which is
  lookup, not download.
- **`hook:format` latency** (question 6), warm, one file, milliseconds:

  | Step | Time |
  |---|---|
  | Task startup | ~75 |
  | git changed-file list (`diff` + `ls-files`) | ~100 |
  | `goimports -l` / `gofmt -l` | ~70 / ~45 |
  | `uv run ruff format --check` / `uv run ruff check --fix --diff` | ~70 / ~70 |
  | `pnpm exec node -e 0` (**pnpm overhead alone**) | ~1030 |
  | `pnpm exec prettier --cache --check` | ~1100 |
  | `node …/prettier.cjs --cache --check` (no pnpm) | ~105 |
  | `pnpm exec eslint --cache --fix-dry-run`, cache hit / miss | ~1800 / ~2200 |
  | `pnpm exec tsc --noEmit --incremental` (for `hook:check`) | ~1400 |

  `prod-go` totals about 300 ms and `prod-py` about 320 ms, both within
  the budget. **`prod-ts` misses it** on `pnpm exec`'s startup alone.
  `node_modules/.bin/prettier` doesn't run in Task's shell on Windows (the
  shim is a shell script, exit 201), so there's no portable way to skip
  pnpm. **Decision needed. See "Open after spikes".**
- **`git status`** (question 4). On a synthetic 100,000-file repository:
  `git status --short` ~84 ms with Git for Windows' default
  `core.fscache`, ~315 ms with it off; `git diff --dirstat` ~66 ms. This
  repository: ~50 ms. Far under the 30-second timeout, so no flag is
  added. `docs/usage.md` still names `core.fsmonitor` and
  `core.untrackedCache`.

## Open after spikes

- **`prod-ts` `hook:format` is over the 1-second budget** because of
  `pnpm exec`'s ~1 s startup: Prettier ~1.1 s, and ESLint `--fix` a further
  ~1.8–2.2 s. Under the rule the spec set, both would move to `hook:check`.
  That would leave TS with no synchronous formatting at all.

## Verification record

*(Filled in during C6.)*
