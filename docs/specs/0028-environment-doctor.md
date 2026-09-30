# Spec 0028: Environment Context and `vibe doctor`

Status: accepted. Tracks issue #44. Builds on spec 0023 (agent lifecycle
hooks) and spec 0014 (`ToolRequirer`).

## Problem

- Agents and developers hit failures whose cause is the machine, not the
  repository: a Bash idiom run under PowerShell, a tool missing from PATH,
  a Windows checkout that passes locally and then fails in Linux CI, or a
  worktree mistaken for the main checkout. Each one surfaces late, as a
  failed command or a failing CI check.
- `hook:context` (spec 0023 §4.1) reports branch, git state, toolchain
  versions and dirty files. It does not report the platform, the kind of
  checkout, or whether the session runs inside WSL or a container. So an
  agent guesses, and a guess based only on the OS (Windows, so PowerShell)
  is often wrong.
- `vibe doctor` is a stub that returns "not implemented". Spec 0014
  already named it as the second consumer of `ToolRequirer`, and noted that
  it would want a richer answer than `LookPath`.
- Growing AGENTS.md is not the answer. Facts that change from session to
  session belong in session context. Machine state that can be diagnosed
  belongs in a deterministic command (principle 1).

## Scope

### 1. `hook:context` gains environment facts

All four repo-tooling Taskfiles (`prod-go`, `prod-ts`, `prod-py`,
`prod-mono`) print these lines, next to what they print today:

```text
Repo: VibeConform
Platform: windows/amd64
Runtime: native
Shell: unknown (not reported by the harness)
Worktree: linked (main checkout at D:/VibeConform)
Branch: feat/environment-doctor
…
```

- **Platform**: `{{OS}}/{{ARCH}}`, Task's own template functions. The
  values are Go's `GOOS`/`GOARCH` for the Task binary, which runs its
  commands in its built-in POSIX shell on every OS. No probe runs, so the
  line is deterministic and cannot fail. It is printed even outside a git
  repository.
- **Runtime**: `wsl` if `/proc/sys/fs/binfmt_misc/WSLInterop` exists,
  `container` if `/.dockerenv` or `/run/.containerenv` exists, otherwise
  `native`. Only the shell's built-in `test -e` is used. On Windows and
  macOS these paths never exist, so the line reads `native`. The result
  is a positive marker or its absence. `native` means that no marker was
  found; it does not claim more.
- **Shell**: printed as `unknown (not reported by the harness)`. Neither
  Claude Code's `SessionStart` input nor its environment reports which
  shell the agent's command tool uses, and the OS does not settle it: on
  Windows the agent may have Git Bash, PowerShell, or both. The line
  exists so the agent sees that the value is unknown, rather than
  assuming. When a harness documents a reliable value, the adapter for
  that harness may fill it in (follow-on). Until then, no generated
  Taskfile infers it.
- **Worktree**: printed only when `git rev-parse --git-dir` and
  `--git-common-dir` differ, meaning this checkout is a linked worktree.
  It names the main checkout's directory. It is absent in an ordinary
  clone, so the common case costs no line.
- Everything spec 0023 §4.1 requires stays intact: Repo, Branch, State,
  toolchain versions, Task, Dirty files, and Affected components. It still
  runs no heavy commands, never calls `vibe` or reads `vibe.yaml`, ends
  every probe in `|| echo missing` or `|| true`, always exits 0, and
  stays under 10,000 characters.

### 2. `vibe doctor`

```text
$ vibe doctor
standard: prod-go/v1
PASS        git            git version 2.47.1.windows.1; repository readable
PASS        manifest       vibe.yaml resolves prod-go/v1 (integrations: claude, codex)
PASS        task           3.53.1; Taskfile.yml loads
PASS        go             go1.27.0
FAIL        golangci-lint  not found on PATH (required by go-tooling: task lint, and CI's lint job)
PASS        lefthook       1.11.3
PASS        claude hooks   .claude/settings.json present; task, go on PATH
WARN        line endings   core.autocrlf=true and Taskfile.yml has no eol attribute: managed files check out as CRLF
PASS        runtime        windows/amd64, native
UNVERIFIED  worktree       not a linked worktree; cross-worktree collisions not checked
summary: 8 pass, 1 warn, 1 fail, 1 unverified
Error: doctor: 1 required check failed
```

#### 2.1 Statuses

| Status | Meaning |
|---|---|
| `PASS` | Checked, and healthy. |
| `WARN` | The workflow runs, but something diverges from what CI sees, or an optional capability is missing. |
| `FAIL` | A required local workflow cannot run: `task verify`, the Git hooks, or the selected agent's hooks. |
| `UNVERIFIED` | VibeConform cannot probe this reliably, and says so rather than guessing. |

#### 2.2 Exit code

The CLI's existing contract (`internal/cli/exit.go`) applies. It gains no
new code:

- `0`: every check is `PASS`, `WARN`, or `UNVERIFIED`.
- `1`: at least one `FAIL`, or doctor could not run at all. Code 1
  already means "no usable verdict", and a local prerequisite that is
  broken fits that. Doctor never returns 2 or 3, which stay reserved for
  conformance verdicts.

Richer machine-readable detail belongs in a later `--json` (follow-on),
not in extra exit codes.

#### 2.3 Checks

Each check prints one line: status, name, and a one-line reason. Checks
run in a fixed order, so the output is deterministic for a given machine
and repository.

1. **git**: `git --version` runs, and `git rev-parse --git-dir` succeeds
   at the repository root. If either fails, the result is `FAIL`, and
   every later check that needs git is reported `UNVERIFIED` with the
   reason "needs git".
2. **manifest**: `vibe.yaml` parses, the standard and version resolve, and
   the integration selection is valid. This is the same resolution
   `audit` uses. If it fails, the result is `FAIL`, and every
   standard-specific check that follows is `UNVERIFIED` with the reason
   "needs a valid vibe.yaml".
3. **Required tools**: one line per tool that the standard's modules
   declare through `ToolRequirer`, in module order and then declaration
   order, deduplicated as `missingTools` already does. A missing tool is
   `FAIL`, since the task that names it cannot run. A tool that is present
   shows its version when it declares a version probe. A probe that fails
   or takes longer than 5 seconds is `WARN` ("found on PATH but did not
   report a version"). A tool without a version probe is `PASS` with its
   path.
4. **Taskfile loads**: `task --list-all` exits 0 at the repository root.
   A Taskfile that does not load is `FAIL`. It turns off every `task`
   entry point, and with it the agent guard (`docs/usage.md`, known gap).
   This check is merged into the `task` tool line.
5. **Agent hooks**: one line per selected agent integration that has
   hooks, which today is only `claude`. The line is `FAIL` if the hook
   config file is missing, or if a binary the hook commands need is not on
   PATH (`task`, plus the guard's runtime: `go`, `node`, or `python`
   according to the standard). The content of the file is `vibe audit`'s
   concern, and doctor does not repeat it. Codex hooks are suspended (spec
   0024), so doctor says so for `codex`: `PASS codex hooks  suspended
   (spec 0024); nothing to check`.
6. **Line endings**: reports `core.autocrlf` and the `text` and `eol`
   attributes that `git check-attr` resolves for one managed file (the
   first `Generated` resource in plan order).
   - `eol=lf` is `PASS`.
   - `eol` unset with `core.autocrlf=true` is `WARN`: the managed files
     check out as CRLF, which is spec 0008's hazard.
   - `eol` unset otherwise is `PASS`, with the note "not pinned".
   - This is a report only. Enforcing line endings is spec 0029's
     concern, and doctor never edits `.gitattributes` or git config.
7. **Runtime**: `PASS` with the platform and the result of the same marker
   detection that §1 uses. It is informational.
8. **Worktree**: whether this checkout is a linked worktree, with the path
   of its main checkout. `PASS` when it is linked, so the fact is shown.
   Whether the worktree collides with others (for example Git hooks
   installed by lefthook into the shared git dir, or tool caches) is
   `UNVERIFIED`. No probe for that is reliable yet.

#### 2.4 What doctor never does

- It installs nothing, writes no file, and changes no git or user
  configuration. It is read-only by default, and this spec adds no flag
  that changes that.
- It makes no network access and starts no container or service.
- It runs no tests or builds. The only external commands it runs are
  `git`, `task --list-all`, and each declared tool's version probe, each
  with a timeout.
- It does not check conformance. `vibe audit` does, and a doctor that
  failed on drift would give two answers to one question.

### 3. Version probes for required tools

`module.Tool` gains an optional version probe: the arguments that make
the binary print its version (for example `go env GOVERSION`, or
`golangci-lint version`). Doctor uses the first non-empty line of output.
`sync`'s missing-tool warning is unchanged. A module that declares no
probe still works, and doctor reports the tool with its path.

### 4. Cross-platform contract

`docs/architecture/principles.md`, principle 2, gains an explicit
statement:

- Task is the canonical operational interface on every platform.
- Supported developer platforms are Windows and Linux. CI runs Ubuntu,
  and Windows where a workflow's matrix says so. Both lists are stated
  explicitly in `docs/usage.md`.
- Differences between platforms are surfaced, by `hook:context` and
  `vibe doctor`, not guessed away.
- CI is the independent, clean-environment authority. A `PASS` from
  doctor does not replace it.

## Behavior

- After upgrading `vibe` and running `vibe sync`, `Taskfile.yml` is out of
  date in every repository on every standard: the `hook:context` task
  changes. Nothing else changes.
- `vibe doctor` works in any repository that has a `vibe.yaml`. Without
  one, the git check still runs and the rest are `UNVERIFIED`, with exit
  code 1 from the manifest `FAIL`.
- Removing VibeConform leaves `hook:context` working exactly as before.
  Doctor is a `vibe` command, and no generated file refers to it.

## Tests

- `hook:context` in all four templates prints `Platform:`, `Runtime:` and
  `Shell:`. The existing tests stay green: the allowlist, nothing heavy,
  no runtimes on PATH, outside git, and the output cap. Outside git,
  `Platform:` is still printed.
- A linked worktree created in a test prints `Worktree:`. An ordinary
  clone does not.
- The command allowlist for `hook:context` does not grow beyond the
  commands it already allows.
- Doctor: each check is tested for each status it can return, through
  seams for PATH lookup and command execution. The machine running the
  tests is never asserted on.
- Exit codes: all-`PASS`, `WARN`-only and `UNVERIFIED`-only runs exit 0.
  Any `FAIL` exits 1.
- Doctor writes nothing: a test syncs a repository, runs doctor, and
  asserts the tree and `.vibe/state.yaml` are byte-identical afterwards.
- The existing Ubuntu and Windows CI test matrix runs all of the above.
  Tests that depend on the platform branch on `runtime.GOOS` and are named
  after it.
- The tests that pinned the stub (`commands_test.go`, `root_test.go`)
  stop covering `doctor`, and keep covering `check`.

## Explicit non-goals

- **Replacing CI** with local checks.
- **Guessing the shell** from the OS or from `$SHELL`, `$PSModulePath` or
  similar variables. They describe the process that launched `task`,
  which may not be the agent's shell.
- **Installing toolchains** or offering a `--fix`.
- **Editing user-global** shell, git or editor configuration.
- **Enforcing line endings.** Spec 0029 does that; doctor only reports.
- **A local Linux execution path** (WSL or a container) for a Windows
  developer. `wsl.exe` is present on every Windows 11 machine whether or
  not a distribution is installed, so its presence proves nothing, and
  running it may start a VM. This is deferred until the repository
  model can declare that it wants a local Linux path.
- **Probing for worktree collisions.** They are reported `UNVERIFIED`.
- **`--json` output.** Follow-on.

## Design notes

- **Why print `Shell: unknown` instead of omitting the line.** An absent
  line reads as "nothing to know". An explicit unknown tells the agent to
  use the harness's own tool description, rather than an OS-based default.
- **Why `{{OS}}/{{ARCH}}` and not `uname`.** `uname` does not exist on
  native Windows. Task's template functions are evaluated by the Task
  binary itself, and need no probe.
- **Why a missing required tool is `FAIL` in doctor but only a warning in
  `sync`.** `sync` answers "are the files right", and a missing tool does
  not make them wrong. Doctor answers "can this machine run the
  workflow", and without the tool it cannot.
- **Why exit code 1, not a new code.** The exit codes are a public API.
  "The workflow cannot run here" is a failure to reach a usable state,
  which is what 1 already means. A dedicated code can be added later if a
  caller needs one.
- **Why doctor does not check conformance.** It keeps one question per
  command: `audit` for the repository, `doctor` for the machine.

## Follow-on work

- `vibe doctor --json`.
- Filling `Shell:` from a harness adapter once a harness documents the
  value.
- Doctor checks contributed by integrations (LSP, GitNexus; #39, #40)
  through the same per-module seam as `ToolRequirer`.
- Spec 0029: doctor reports the health of the line-ending policy.
