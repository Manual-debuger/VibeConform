# Spec 0023: Agent Lifecycle Hooks Beyond the Guard

Status: accepted (2026-09-24). Not yet implemented.

## Problem

Every standard composes `agent-config`, and `agent-config` configures
exactly one hook event: `PreToolUse`, running `task -x hook:guard`
(spec 0021). An agent working in a conformant repository gets a safety
gate and nothing else:

1. **No dynamic context at session start.** The agent loads `AGENTS.md` on
   its own, but it doesn't learn the branch, the git state, what is
   already dirty, or whether the toolchain is even installed until it
   goes and asks.
2. **No formatting after an edit.** Unformatted code is caught at
   `task verify`, at `pre-commit`, or in CI, each later and more expensive
   than the edit itself.
3. **No early feedback.** Static checks and tests run only when the agent
   chooses to run them.
4. **No definition-of-done gate.** `AGENTS.md` says "run `task verify`
   before declaring work done", which is exactly the kind of rule
   principle 1 (`docs/architecture/principles.md`) says belongs in a tool,
   not a prompt.

The target set, for every standard and both agent runtimes:

| Event | Purpose |
|---|---|
| `SessionStart` | lightweight, dynamic project context |
| `PreToolUse` | fast safety/policy gate (exists, spec 0021) |
| `PostToolUse` | changed-file formatting and cheap fixes |
| `PostToolUse`, async | affected static checks and tests |
| `Stop` | affected definition-of-done verification |

## Design in one paragraph

Keep spec 0021's shape: agents call **one fixed command per event, the same
in every standard** (`task -x hook:<name>`), and the managed `Taskfile.yml`
defines each task. **Nothing new is built to detect what is affected.**
Changed files come from `git`, and each check is made incremental by the
cache of the tool that already runs it (`go test`'s test cache,
golangci-lint's cache, ESLint and Prettier `--cache`, `tsc --incremental`,
Ruff's cache). There is no new program, no new resource beyond the config
files, and no new dependency. One new public task, `verify:fast`, is what
the async check and the `Stop` gate run, and what a person can run by hand.

## Scope

### 1. `agent-config` registers four more events

`.claude/settings.json`:

| Event | Matcher | Command | Options |
|---|---|---|---|
| `SessionStart` | `startup\|resume\|clear` | `task -x hook:context` | `timeout: 30` |
| `PreToolUse` | `Bash\|PowerShell\|Write\|Edit` | `task -x hook:guard` | unchanged |
| `PostToolUse` | `Write\|Edit\|MultiEdit\|NotebookEdit` | `task -x hook:format` | `timeout: 60` |
| `PostToolUse` | `Write\|Edit\|MultiEdit\|NotebookEdit` | `task -x hook:check` | `asyncRewake: true`, `timeout: 300` |
| `Stop` | none | `task -x hook:done` | `timeout: 600` |

`.codex/hooks.json` gets the same five events in Codex's nested schema.
The `PostToolUse` matcher is `apply_patch|Edit|Write` (Codex reports
`tool_name: "apply_patch"` and accepts the other two as aliases), and
`hook:check` gets `async: true`. See 4.2 for why the two runtimes use
different async options.

`agent-config` gains no new files. This repository's hand-maintained
`.codex/README.md` (not a resource) is updated to list the new events.

### 2. The tasks

Each repo-tooling module (`repo-tooling`, `ts-repo-tooling`,
`py-repo-tooling`) defines `hook:context`, `hook:format`, `hook:check`,
`hook:done`, and `verify:fast`. The `hook:*` tasks have no `desc` and are
`silent`, like `hook:guard`. `verify:fast` has a `desc`, because people
run it too. All five names become reserved (ADR-0009).

`verify:fast` is the part of `verify` that is cheap and incremental:

| Standard | `verify:fast` | Left in `verify` only |
|---|---|---|
| `prod-go` | `fmt:check`, `typecheck`, `lint`, `test` | `security` (network), `mod:verify`, `workflows:lint` |
| `prod-ts` | same as `verify` | none |
| `prod-py` | same as `verify` | none |

`verify` stays the full gate, and CI keeps running it.

### 3. Cache flags on the existing tasks

To make "run the task" mean "redo only what changed", the existing tasks
gain their tools' cache flags. This repository doesn't manage `.gitignore`,
so every cache is written somewhere already ignored:

| Tool | Flag | Cache location |
|---|---|---|
| ESLint | `--cache --cache-location node_modules/.cache/eslint/` | under `node_modules` |
| Prettier | `--cache` | `node_modules/.cache/prettier/` (default) |
| `tsc` | `--incremental --tsBuildInfoFile node_modules/.cache/tsc/tsbuildinfo` | under `node_modules` |
| Ruff | none, cached by default | `.ruff_cache/`, which writes its own `.gitignore` |
| golangci-lint | none, cached by default | user cache directory |
| `go test` | none; results cache when given package arguments (`./...`) | Go build cache |

`go vet` has no result cache (golang/go#53014), but it reuses the build
cache and is fast. `go test` must keep its `./...` argument and never gain
`-count=1` or another uncacheable flag. A test pins that.

### 4. The hooks

#### 4.1 `SessionStart` → lightweight context (`hook:context`)

**What it prints.** Plain text on stdout, which both agents add to the
model's context (Claude Code as context, Codex as developer context). A
handful of lines:

```text
Repo: VibeConform
Branch: feature/m4
State: dirty, rebase in progress
Go: go1.27.0
Task: 3.53.1
Dirty files: 3
 M internal/module/agents/agents.go
 M docs/specs/0023-agent-lifecycle-hooks.md
?? internal/module/agents/templates/claude/new.json
Affected components:
  66.6% internal/module/agents/
  33.3% docs/specs/
```

- **Repo**: `{{base .ROOT_DIR}}`.
- **Branch**: `git branch --show-current`, or `detached at <short sha>`.
- **State** (the repository mode): `clean` or `dirty`, plus any
  operation in progress: rebase, merge, cherry-pick, revert, or bisect.
  Detected by testing the paths `git rev-parse --git-path` reports
  (`rebase-merge`, `rebase-apply`, `MERGE_HEAD`, `CHERRY_PICK_HEAD`,
  `REVERT_HEAD`, `BISECT_LOG`), using the shell's built-in `test`.
- **Toolchain health**: each runtime the standard requires, with its
  version or `missing`:
  - `prod-go`: `go env GOVERSION`, `task --version`
  - `prod-ts`: `node --version`, `pnpm --version`, `task --version`
  - `prod-py`: `uv --version`, the version of the interpreter
    `uv python find` reports (never `uv run`, which can download a Python),
    `task --version`
- **Dirty files**: the count, then `git status --short` itself, capped at
  20 lines with a `… and N more` line.
- **Affected components**: `git diff --dirstat=files,0 HEAD`, native git.
  Untracked files are already in the status list above.

**What it never does.** The context is facts, not work. `hook:context`
runs no dependency installation, no `docker compose up`, no tests, no
database migrations, and no dependency-graph rebuild. Those stay explicit
tasks. It doesn't repeat `AGENTS.md`, which both agents load themselves,
and it doesn't call `vibe` or read `vibe.yaml`, so it keeps working after
VibeConform is removed (principle 3).

**How it's built.** Pure Task: dynamic `sh:` variables (which need no
stdin, so spec 0021's dynamic-variable finding doesn't apply) and one
`echo` of a template. Every probe ends in `|| echo missing`, so a missing
tool shows up as a line, never as a failed session start. It exits 0
unless Task itself can't load.

#### 4.2 `PostToolUse` → changed-file formatting and cheap fixes (`hook:format`)

**Trigger.** After every file edit: Claude Code
`Write|Edit|MultiEdit|NotebookEdit`, Codex `apply_patch|Edit|Write`.
Synchronous, `timeout: 60`.

**Which files.** The working tree's changed files, asked of git, with
git's own pathspec filtering by extension:

```sh
{ git diff --name-only --diff-filter=d HEAD -- '*.go'
  git ls-files --others --exclude-standard -- '*.go'; } | xargs -r goimports -w
```

`-r` skips the run on an empty list (resolved question 2).

`--diff-filter=d` drops deleted files. Task's built-in core utilities
include `xargs` on Windows, so this runs the same everywhere
(principle 2). The file the agent just edited is always in this set. So
is anything else uncommitted, including the user's own edits. That is
accepted, because formatters are idempotent and the repository's
`fmt:check` would demand it anyway. No hook payload is parsed.

**What runs:**

| Standard | Formatting | Cheap fix |
|---|---|---|
| `prod-go` | `gofmt -w` | `goimports -w` (adds and removes imports) |
| `prod-ts` | `prettier --cache --write` (the `fmt` extension set) | `eslint --cache --fix` |
| `prod-py` | `ruff format` | `ruff check --fix` |

**Behaviour.** The edit has already happened, so this can't block it. On
failure (a syntax error the formatter rejects, a missing tool) the task
exits 2 and both agents show stderr to the model. Each cmd ends in
`|| exit 2`, as `hook:guard`'s does. With no changed files of that
language, nothing runs and it exits 0.

#### 4.3 async `PostToolUse` → affected static checks and tests (`hook:check`)

**Trigger.** Same as 4.2, but in the background. Neither agent waits for
it. This was checked against both runtimes' documentation on 2026-09-24:

- **Claude Code: `asyncRewake: true`, not `async: true`.** A hook with
  `async: true` runs in the background, but Claude Code discards its
  output and exit code, so the model would never see a failure.
  `asyncRewake: true` also runs in the background, and when the hook exits
  2 it wakes Claude with the hook's stderr (or stdout) as a system
  reminder. Unlike `async`, its `timeout` is enforced, hence
  `timeout: 300`.
- **Codex: `async: true`.** Codex runs the hook in the background and
  delivers its output at the next safe point: after the current model
  request and tool calls finish, or at the next user turn if no turn is
  active. It runs at most eight background hooks per session.

**What runs.** `task verify:fast`. "Affected" comes from each tool's
cache:

| Standard | Check | Scope |
|---|---|---|
| `prod-go` | `go build ./...`, `go vet ./...` | whole module, from the build cache |
| | `golangci-lint run` | cached; unchanged packages are fast |
| | `go test ./...` | **affected only, reverse dependencies included**: a package's cached result is reused unless it or anything it imports changed |
| `prod-ts` | `eslint --cache .` | changed files only |
| | `tsc --noEmit --incremental` | whole project, incrementally |
| | `pnpm test` | whatever the repository's `test` script runs |
| `prod-py` | `ruff check .` | cached |
| | `pyright`, `pytest` | whole (see 5) |

**Behaviour.** Advisory. It never blocks anything, and exits 2 on failure
so the result reaches the model.

#### 4.4 `Stop` → affected definition-of-done verification (`hook:done`)

**Trigger.** When the agent ends its turn. Synchronous, `timeout: 600`.

**What runs.** `task verify:fast`, the same scope as 4.3, including
`fmt:check`. This is the definition of done for one turn. `task verify`
(with `security`, `mod:verify`, `workflows:lint` on `prod-go`) stays the
full gate that CI runs. When nothing changed, every step hits a warm cache
and the gate is fast.

**Behaviour.** On failure it exits 2, which blocks the stop and shows the
output to the model. In Claude Code the model sees the reason and keeps
working. In Codex the turn continues with the reason as an automatic
prompt.

**It blocks once, not forever.** To avoid a turn that never ends on a
failure the agent can't fix (a pre-existing flake, a missing tool),
`hook:done` checks the payload's `stop_hook_active` field. When it is true,
this stop was already blocked once. The hook still runs `verify:fast` but
exits 0. Without a helper program, this reads stdin with Task's built-in
`cat` inside a command (not a dynamic variable) and matches it with a
shell `case` pattern that tolerates JSON whitespace:

```yaml
  hook:done:
    silent: true
    cmds:
      - |
        case "$(cat)" in
          *'"stop_hook_active":true'*|*'"stop_hook_active": true'*) task verify:fast || true ;;
          *) task verify:fast || exit 2 ;;
        esac
```

This matters more for Codex than for Claude Code. Claude Code overrides a
`Stop` hook after it blocks eight times in a row without progress. Codex
documents no cap.

This is a deliberate soft spot against principle 1, and it is documented
as one: a second stop is let through with failures printed. CI remains the
authoritative gate.

### 5. How "affected" is chosen

Tools are preferred in this order: tools the standards already run, then
popular tools, then our own code.

| Tier | Choice | Verdict |
|---|---|---|
| 1. Already used | `git` for changed files; `go test` cache; golangci-lint cache; ESLint/Prettier `--cache`; `tsc --incremental`; Ruff cache | **Adopted.** Everything in section 4. |
| 1. Already used | `lefthook run <custom hook>` with `files:`, `glob:`, `{files}` | Considered. It does file selection natively, but its commands run through `sh`, which isn't confirmed on Windows without Git Bash, and Task is the interface principle 2 names. |
| 1. Already used | Task `sources:` checksums, to skip `hook:done` when nothing changed | Considered. Task writes checksums under `.task/`, which would show up as untracked files, since `.gitignore` isn't managed. The tool caches already make an unchanged run fast. |
| 2. Popular | Nx `affected`, Turborepo `--affected` | Rejected: they need adopting a JS monorepo build system, for single-package repositories. |
| 2. Popular | Bazel target-determinator, Pants `--changed-since` | Rejected: they need the whole repository to build under Bazel or Pants. |
| 2. Popular | `pytest-testmon` | Deferred: a new dependency, beta status. Needs an ADR. |
| 3. Our own | a hook helper parsing payloads and mapping files to packages | **Not built.** Tier 1 covers it. |

What tier 1 doesn't cover:

- **`pyright` and `pytest` run whole.** Pyright has no command-line cache,
  and pytest has no built-in "tests for changed code".
- **`pnpm test` is opaque.** `prod-ts` doesn't choose a test runner. A
  repository using Vitest (`vitest related`, `--changed`) or Jest
  (`--findRelatedTests`, `--changedSince`) can put that in its own `test`
  script. `docs/usage.md` says so.

### 6. Tests

- The `internal/standard` wiring test extends to every event: each command
  in `.claude/settings.json` and `.codex/hooks.json` must name a task that
  the standard's `Taskfile.yml` defines.
- Every generated hook command is exactly `task -x hook:<name>`. The `-x`
  reasoning from spec 0021 applies to every event.
- `hook:check` uses `asyncRewake` in `.claude/settings.json` and never
  `async`; `.codex/hooks.json` uses `async`.
- `hook:context` runs only an allowlisted set of commands (`git`, version
  probes, `test`, `echo`). An install, test, or service command in it fails
  the build.
- `go test` in `prod-go`'s tasks keeps `./...` and no uncacheable flag.
- A corpus of `Stop` payloads (`stop_hook_active` true or false, compact
  and spaced JSON, field missing) run through `task -x hook:done` in each
  example.
- `examples.yml` runs `task hook:context` and `task verify:fast` in each
  example, and runs `hook:format` after touching a source file to show that
  the file comes back formatted.

## Behavior

After upgrading `vibe`, in a repository on any standard:

- `vibe audit` reports `.claude/settings.json`, `.codex/hooks.json`, and
  `Taskfile.yml` out of date. `vibe sync` writes them.
- The next agent session gets a short context at start, formatting after
  each edit, background check results, and a `verify:fast` gate at each
  stop.
- **This repository dogfoods `prod-go`**, so its own sessions get all of
  this.
- Removing VibeConform leaves all of it working: `task`, `git`, the
  toolchain, and plain config files.

## Known limitations

- **Formatting covers every uncommitted change**, including the user's own
  edits, not only the file just edited.
- **`pyright`, `pytest`, and `pnpm test` run whole** (section 5).
- **ESLint's cache doesn't track cross-file dependencies**, so a
  type-aware or import rule can show a stale result. `verify` in CI starts
  cold and is unaffected.
- **Overlapping async runs.** Quick successive edits start overlapping
  `hook:check` runs. Codex caps them at eight per session. This spec
  doesn't debounce them.
- **The nearest Taskfile wins**, as for the guard (spec 0021).
- **A file is formatted under the agent.** After `hook:format` the agent's
  copy is stale until it re-reads the file.
- **A repository with no commits** has no `HEAD`. The `git diff … HEAD`
  steps print nothing, so only untracked files are formatted.
- **`stop_hook_active` lets a second stop through** (4.4).

## Explicit non-goals

- **No heavy work at session start**: no dependency installation,
  `docker compose up`, full tests, database migrations, or dependency-graph
  rebuilds. Those are explicit tasks.
- **No repetition of `AGENTS.md`** in the session context.
- **No `vibe audit` in any hook** (principle 3). The hand-edited-managed-file
  case stays CI's `Conformance / audit` job.
- **No monorepo build system**, and no test-impact plugin without an ADR.
- **No custom affected-file detection.** No hook payload is parsed except
  for the one `stop_hook_active` match.
- **No change to the guard** or its policy.
- **No `UserPromptSubmit`, `PreCompact`, or `SubagentStop`** hooks.

## Resolved questions

Each was settled by choosing a default, with a check the plan must run and
a fallback if the check fails. None of the fallbacks adds a helper program
or a dependency.

1. **Codex async failures.** *Decision:* `hook:check` exits 2 with the
   failure on stderr, the same as for Claude Code. *Check:* a manual
   canary in a Codex session (break a test, edit a file, expect the failure
   to reach the model), recorded in the plan like the guard's canary in
   `docs/usage.md`. CI can't observe delivery. *Fallback:* print the
   failure as Codex's documented JSON context output if that can be done
   in Task's shell. Otherwise leave the async hook out of
   `.codex/hooks.json` and rely on the `Stop` gate there, with the gap in
   `.codex/README.md`.
2. **`xargs` on empty input.** *Decision:* `xargs -r`. *Check:* run each
   `hook:format` cmd with an empty changed set on Windows through Task's
   built-in `xargs`, and on Linux. *Fallback:* the same cmd first tests that
   the list is non-empty and skips if it isn't. A Task variable holding the
   list is rejected, because Task would split paths containing spaces.
3. **Capping the status list and the repo name.** *Decision:* Task
   template functions (`splitLines`, `len`, slicing, `base .ROOT_DIR`). The
   minimum Task version that provides them is recorded next to the existing
   "Requires Task v3.39.0 or newer" note in each `Taskfile.yml`. *Check:*
   run `hook:context` on that minimum version and the pinned CI version,
   with more than 20 dirty files. *Fallback:* drop the list and print the
   count plus the dirstat summary.
4. **`git status` on large repositories.** *Decision:* plain
   `git status --short`, no speed flags. `docs/usage.md` points adopters with
   large repositories to git's own speed-ups (`core.fsmonitor`,
   `core.untrackedCache`) rather than VibeConform configuring them. *Check:*
   time `hook:context` on this repository and on a checkout with about
   100k files, against the 30-second hook timeout. *Fallback:* none
   planned unless the measurement demands it.
5. **Python version without a download.** *Decision:* `uv python find`,
   then that interpreter's `--version`. *Check:* run it on a machine with no
   uv-managed Python and confirm nothing is downloaded. *Fallback:* report
   `uv --version` only.
6. **`hook:format` latency.** *Decision:* a budget. A warm run of
   `hook:format` must take under 1 second for a one-file change on each
   standard. *Check:* measure each formatter and fixer on the examples.
   *Fallback:* a step that misses the budget moves from `hook:format` to
   `hook:check`, with the measurement recorded in the plan. `goimports` and
   `ruff check --fix` are expected to stay synchronous; `eslint --fix` with
   type-aware rules is the likeliest to move.

## Follow-on work

- `vibe-conformance` contributing `task audit` to the `Stop` gate, if that
  can be done without making `hook:done` depend on `vibe`.
- Debouncing async checks.
- `pytest-testmon` behind an ADR, if `pytest` run whole proves too slow.
