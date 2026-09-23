# Spec 0021: Agent Hooks Behind a Task Interface, Run on the Language Runtime

Status: accepted and implemented.

## Problem

`agent-config` ships two bash scripts, `.claude/hooks/block-dangerous.sh`
and `.claude/hooks/block-secret-files.sh`. Claude Code runs them as
`PreToolUse` hooks, and Codex runs `block-dangerous.sh` through
`.codex/hooks.json`. Six things are wrong with that:

1. **They need bash and `grep`.** Neither is part of a stock Windows
   install, and principle 2 (`docs/architecture/principles.md`) treats
   Windows as first-class. Worse, a missing `grep` fails open: the `if`
   condition is false, so the hook exits 0 and allows the command (see
   Design notes).
2. **They match text, not commands.** The scripts `grep -E` the raw
   tool-call JSON, so a pattern anywhere in the payload blocks the call,
   including in a Bash call's `description` field. Filing issue #26 hit a
   related case: a `gh issue create` whose body quoted a PowerShell
   recursive delete was denied.

   *Correction, found while implementing:* the issue #26 case is not fixed
   by this spec. The quoted body is part of `tool_input.command`, so
   matching the parsed command still denies it. What parsing does fix is
   matching fields other than the command. The shared corpus records both
   cases; see "Known limitations".
3. **The policy is untested.** Nothing runs the patterns against known
   allow/deny cases. A regex edit that stops matching a destructive
   command ships green, and a hook that allows everything looks exactly
   like one that works.
4. **They fail open when not executable.** A bash script runs only with its
   mode bit set, and `vibe audit` doesn't check mode
   (`docs/decisions/0006-resource-file-mode.md`). A `chmod -x` quietly
   disables the guardrail.
5. **The launch command depends on the shell.** `.claude/settings.json`
   uses `${CLAUDE_PROJECT_DIR}/…`, which only bash expands. Codex uses a
   path relative to a working directory it doesn't guarantee.
6. **The Codex hook config doesn't match Codex's schema.** Codex's
   documented `hooks.json` nests handlers under a matcher:

   ```json
   {"hooks": {"PreToolUse": [{"matcher": "…", "hooks": [{"type": "command", "command": "…"}]}]}}
   ```

   `agent-config` ships a flat entry instead, with no `matcher`, no inner
   `hooks` array, and no `type`:

   ```json
   {"hooks": {"PreToolUse": [{"command": "…", "commandWindows": "…"}]}}
   ```

   Codex may never have run the guard at all. That is a separate bug from
   the other five, and this spec fixes it because it rewrites the same file.

## Design in one paragraph

Agents call **one command, the same in every standard**: `task -x
hook:guard`. The managed `Taskfile.yml` defines `hook:guard`, and that task
runs a small guard program on the standard's own language runtime (`go`,
`node`, or `uv`). The guard reads the tool call from stdin, matches it
against one shared policy file, and denies by exiting 2. Task stays the
single interface for humans, CI, git hooks, and now agents. The language
runtime does the parsing and matching, which a shell can't do portably.

## Scope

### 1. `agent-config` stays one module, and calls Task

`agent-config` remains a single language-neutral module, composed by all
three standards. Its resources:

| Resource | Change |
|---|---|
| `.claude/settings.json` | One `PreToolUse` entry, matcher `Bash\|PowerShell\|Write\|Edit`, runs `task -x hook:guard` |
| `.codex/hooks.json` | Rewritten to Codex's documented schema (problem 6): one `PreToolUse` entry with `matcher: "Bash"` and a `hooks` array holding `{"type": "command", "command": "task -x hook:guard"}`. No `commandWindows`, since the command is identical on every OS. |
| `.codex/config.toml` | unchanged |
| `.claude/hooks/policy.json` | **new**: the shared policy (see 3) |
| `.claude/hooks/block-dangerous.sh`, `block-secret-files.sh` | **dropped** from the module |

One guard handles both kinds of tool call. It reads `tool_name` from the
payload and applies only the policy entries for that kind of tool, so
there is one task and one guard, not two.

`task -x hook:guard` parses the same way in bash, `sh`, `pwsh`,
`powershell.exe`, and `cmd`: no variable expansion, no shell-specific
quoting, no path. That removes problem 5.

### 2. Each repo-tooling module provides `hook:guard` and the guard

The language-specific part moves to the module that is already
language-specific:

| Module | Guard resource | `hook:guard` runs |
|---|---|---|
| `repo-tooling` (`prod-go`) | `.claude/hooks/guard.go` | `go run .claude/hooks/guard.go` |
| `ts-repo-tooling` (`prod-ts`) | `.claude/hooks/guard.mjs` | `node .claude/hooks/guard.mjs` |
| `py-repo-tooling` (`prod-py`) | `.claude/hooks/guard.py` | `uv run --no-project python .claude/hooks/guard.py` |

The generated task:

```yaml
  hook:guard:
    # No desc: agents call this, people don't, so it stays out of task --list.
    silent: true
    cmds:
      - node .claude/hooks/guard.mjs
```

Each runtime is one the standard already requires (`go`; `node` via
`ts-tooling`; `uv` via `py-repo-tooling`), so nothing new is installed
(principle 3). The Go guard lives under a dot-prefixed directory, which
`go build ./...`, `go vet ./...`, and golangci-lint all skip, so it doesn't
become part of the adopting repository's module.

`hook:guard` becomes a name the managed Taskfile reserves, like `verify`
and `audit` today (ADR-0009): a `Taskfile.local.yml` can't redefine it.

### 3. One policy, one regular-expression subset

The patterns and deny messages live in one Go table inside
`internal/module/agents`. Each entry has a regular expression, which tool
kind it applies to (`command` for Bash/PowerShell, `file_path` for
Write/Edit), and a message. `agent-config` emits the table as
`.claude/hooks/policy.json`. Each guard is a small generic interpreter of
that file, so a pattern change is one edit to one file, and the guards
rarely change.

Patterns are restricted to the subset that Go RE2, Python `re`, and
JavaScript `RegExp` interpret identically: no lookaround, no
backreferences, no possessive quantifiers, and no engine-specific classes.
A Go test rejects any table entry outside that subset.

The policy is a **port** of the two existing scripts, pattern for pattern.

### 4. Guard behavior

Each guard:

- reads the tool-call JSON from stdin and extracts `tool_input.command` or
  `tool_input.file_path`, depending on the policy entry's tool kind;
- if stdin isn't valid JSON, or the field is missing, falls back to
  matching the raw payload, which is today's behavior. It is never less
  strict than the script it replaces;
- on a match, writes the deny message to stderr and **exits 2**;
- otherwise exits 0 with no output.

Exit 2 is the deny signal because it is the one that survives Task: with
`-x`, Task exits with the guard's own code. Without `-x`, Task exits 201
for any failing command, and the agent treats 201 as a non-blocking hook
error, **allowing** the command. `-x` is therefore part of the contract, not
a style choice, and a test asserts that every generated agent config calls
`task -x hook:guard` exactly (principle 1).

*Added during implementation:* every `hook:guard` command ends in
`|| exit 2`. Running the corpus through Task showed that `go run` does not
pass its program's exit code through: the guard's `2` comes out as `1`,
and `prod-go`'s guard would have allowed everything it was meant to
block. The suffix fixes that, and it also makes any other failure after
the task starts a deny: a compile error, a missing `node` or `uv`, an
unreadable policy. That is deliberately stricter than the rest of this
spec, which accepted failing open when a runtime is missing. A test pins
`go run`'s behavior, so the reason for the workaround stays visible.

The guard takes the policy path as its only argument
(`.claude/hooks/policy.json`, relative to the Taskfile, which is where Task
runs it) rather than locating it beside its own source file. That is
simpler in all three runtimes, and it survives `go run`, which compiles
the guard to a temporary directory.

### 5. Wiring is checked, not assumed

`agent-config` references a task that a *different* module defines. A test
in `internal/standard` asserts that every registered standard composing
`agent-config` also resolves a `Taskfile.yml` defining `hook:guard`, plus
the guard file that task runs. A standard that forgot one fails the build,
not an agent session.

The Codex fix gets the same treatment. A test decodes `.codex/hooks.json`
into the documented nested shape and fails on the flat one, so a
regression to it cannot ship green.

### 6. A shared test corpus, run through the real command

A fixture file of allow/deny cases lives beside the policy table and is
*not* a resource. It covers each pattern, the known false positive (a
command that only quotes a dangerous one, recorded as *allow*), and
malformed input (recorded as *falls back to raw matching*).

- A Go test runs every case through `guard.go`'s logic in-process.
- `.github/workflows/examples.yml` runs the same corpus in the TS and
  Python examples **through `task -x hook:guard`**, the exact string the
  agent configs contain. That exercises the Taskfile wiring, the `-x` exit
  code passthrough, and the guard in one step.
- A case that disagrees across runtimes fails CI.

### 7. File mode no longer matters

The guards are launched through an interpreter, so they are written with
the default `0644`, and `hookMode` is removed. ADR-0006's unaudited-mode
gap no longer applies to any shipped resource. The ADR gets a note saying
so rather than being rewritten.

## Behavior

For a repository on any standard, after upgrading `vibe`:

- `vibe audit` reports `.claude/settings.json`, `.codex/hooks.json`, and
  `Taskfile.yml` as **out of date** (exit 3), and the guard and
  `policy.json` as **missing**.
- `vibe sync` writes them.
- The old `block-dangerous.sh` and `block-secret-files.sh` **stay on disk**.
  `sync` never deletes a resource a standard stopped resolving
  (`docs/usage.md`). They are inert, since nothing references them any
  more, but misleading, and `docs/usage.md` tells adopters to delete them.
  This repository deletes its own in the same commit.
- Removing VibeConform leaves a working guard: `task`, the language
  runtime, and three plain files. Nothing in the path calls `vibe`
  (principle 3).

This repository's `AGENTS.md` Guardrails section is rewritten to describe
the new mechanism and its limitations. It is hand-maintained, not a
resource.

## Known limitations

Recorded in `docs/usage.md` and in a comment in each template:

- **A Taskfile that fails to load turns the guard off.** A YAML error in
  `Taskfile.yml` or `Taskfile.local.yml` (Task exits 109), or a task-name
  collision with the managed file (exit 203), stops Task before the guard
  runs. Any exit other than 2 is treated as allow. `task verify` and every
  other task break at the same moment, so this is rarely silent for long,
  but it is the failure mode this design adds.
- **The nearest Taskfile wins.** Task searches upward from the agent's
  working directory. In a subdirectory with its own `Taskfile.yml` (such as
  `examples/typescript` in this repository), that file's `hook:guard`
  runs. Harmless when it is a conformant Taskfile. A nested Taskfile with
  no `hook:guard` fails with exit 200, which allows the command.
- **Codex on Windows** fires no `PreToolUse` hook for shell commands
  ([codex#24453](https://github.com/openai/codex/issues/24453), open), so
  Codex has no command guard on Windows.
- **Codex has no file-edit hook**, so `.codex/` has no secret-file guard on
  any platform (already recorded in `.codex/README.md`).
- **Claude Code on Windows without Git Bash** launches hook commands
  through `/bin/sh`, or `pwsh` when `shell: "powershell"` is set. When
  neither launches, the hook never runs and the command is allowed
  ([claude-code#90077](https://github.com/anthropics/claude-code/issues/90077)).
  This applies to `task` exactly as it did to bash.
- **Quoted content still matches.** Matching the parsed command still
  blocks a heredoc or string argument whose *content* contains a dangerous
  pattern, such as the issue #26 `gh issue create` body. Parsing removes
  only matches in *other* fields, such as a Bash call's `description`.
- **Canary.** No automated check can observe an agent's own hook dispatch.
  The manual test is to ask the agent to run a denied command and expect a
  denial.

## Explicit non-goals

- **No fix for Codex on Windows, or for Claude Code without Git Bash.**
  Both are upstream. This spec documents them rather than papering over
  them.
- **No shell parsing of commands** (tokenizing, stripping heredocs). It
  would cut the remaining false positives, but a shell parser in three
  languages is a much larger policy surface. Follow-on at most.
- **No new patterns.** Changing what is blocked is a separate decision.
- **No `vibe` at hook time** (principle 3).
- **No `shell: "powershell"` in generated settings.** It hard-requires
  `pwsh`, which is no more guaranteed than Git Bash, and would break the
  common working case to help the uncommon one.
- **No multi-language repositories yet.** See Design notes for why this
  design is the one that extends to them.

## Design notes

### Alternatives considered, with measurements

Measured with Task v3.53.1 on Windows 11.

| Design | Result |
|---|---|
| Task alone: template functions (`regexMatch`, `fromJson`) on a dynamic variable reading stdin | **Fails closed by accident.** Dynamic variables run without stdin; Task's built-in `cat` (u-root) panics on a nil reader and exits 2, which agents read as deny. Every tool call would be blocked by a crash. |
| Task's shell plus `grep` (today's scripts, moved into a task) | **Fails open silently.** With Git's tools off `PATH`: `"grep": executable file not found in $PATH`, the condition is false, exit 0, allowed. Task's built-in utilities include `cat`, not `grep`. |
| Runtime called directly from agent configs (`node .claude/hooks/guard.mjs`) | Works, but the command differs per standard, so `agent-config` must split into three modules. In a multi-language repository those variants would all generate `.claude/settings.json`, with no way to choose. Also leaves problem 5 (which directory hooks run from) open. |
| **Task as the interface, runtime does the work** (this spec) | stdin reaches the runtime. With `-x`, deny exits 2 and passes the guard's output through. Allow exits 0 silently. Called from a subdirectory, Task found the Taskfile upward and ran the guard relative to it. About 119 ms per call through `node`. |

### Why this design extends to multi-language repositories

The guard policy covers the whole repository, not one component, so a
repository needs exactly one guard runtime. This design puts that choice in
`Taskfile.yml` and keeps the agent configs identical everywhere. A
multi-language repository must already settle on a single top-level
`Taskfile.yml` (today every repo-tooling module generates one), so choosing
the guard runtime there adds no new conflict. Calling the runtime directly
would have added one, in `.claude/settings.json`.

### Why a data file plus small interpreters, not generated code

Three generated guards would each embed the table, so every pattern change
would regenerate three files, and the corpus test would be the only thing
keeping them honest. A policy file keeps the guards stable and puts every
change in one place that is easy to diff.

### Cross-module coupling is deliberate

`agent-config` naming a task that `repo-tooling` defines is a dependency
between modules, which the module model doesn't express. It is accepted
because every standard composes both, and it is enforced by the test in
Scope 5 rather than by convention.

## Open questions for the plan

- **Codex's deny protocol.** Does Codex honor exit 2 plus stderr from a
  `PreToolUse` hook, like Claude Code? If it needs
  `{"decision":"block"}` on stdout, the guard emits both. Verify against
  current Codex documentation.
- **Task's stderr on deny.** Task appends `task: Failed to run task
  "hook:guard": exit status 2` after the guard's own message. Claude Code
  shows stderr to the model as the reason. Confirm that the guard's message
  comes first and still reads well.
- **Latency on Go and Python.** Only `node` through Task was measured
  (~119 ms). Record `go run` (including the first, compiling run) and `uv
  run --no-project` through Task, and decide whether they are acceptable on
  every matched tool call.
- **`uv run --no-project`** inside a uv project: confirm it neither syncs
  nor creates `.venv`, and that it works before any Python is installed (uv
  may download one on first use).

## Follow-on work

- Shell-aware command matching, if the remaining false positives turn out
  to matter.
- `vibe doctor` checking that `task hook:guard` launches, once `doctor`
  exists.
- A single top-level Taskfile for multi-language repositories, which this
  design is ready for.
- Revisit Codex on Windows when codex#24453 is resolved.
