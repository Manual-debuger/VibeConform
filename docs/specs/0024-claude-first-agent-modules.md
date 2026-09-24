# Spec 0024: Claude First, One Module per Agent, Codex Hooks Suspended

Status: implemented (2026-09-24), with the proposed answer to both open
questions: keep composing `codex-config`, and nest the packages. See
`docs/plans/0024-claude-first-agent-modules.md` for the verification
record.

## Problem

Specs 0021 and 0023 gave every standard one `agent-config` module that
writes Claude Code and Codex settings together, both calling the same
`task -x hook:<name>` commands. The premise was that one hook contract could
serve every agent runtime. A live test on 2026-09-24 (below) shows it
doesn't, and the cost of pretending it does keeps growing:

1. **Codex on Windows ignores exit code 2.** Its hooks fire, but a
   `PreToolUse` that exits 2 doesn't block the command, and a `Stop` that
   exits 2 doesn't continue the turn. The guard and the `Stop` gate, the
   two hooks that exist to enforce something, enforce nothing there.
2. **Codex on Linux needs its own output format.** A background hook's
   stderr and exit 2 never reach the model; only JSON `additionalContext`
   does. `hook:check` as written reports nothing under Codex anywhere.
3. **Codex file edits skip the guard.** Covering `apply_patch` means parsing
   file paths out of a patch body (relative on Windows, absolute on Linux
   in the test), in all three guards.
4. **Every fix is shared.** Because one module and one set of commands
   serve both agents, each Codex difference becomes a change to what Claude
   Code runs too, or a special case inside a shared file.
5. **The docs were wrong in both directions.** `docs/usage.md` says Codex on
   Windows fires no `PreToolUse` for shell commands (openai/codex#24453); in
   Codex 0.156.1 it does. `.codex/README.md` says Codex has no file-edit
   hook; it has one. Several rounds of work went into Codex hooks without
   anyone watching them run.

Claude Code, by contrast, has been observed working on Windows in this
repository's own sessions (session context, the guard, the `Stop` gate), and
its hook contract matches what the tasks were written for.

## Evidence: the 2026-09-24 canary

A scratch repository registered a logging hook on every event, then asked
Codex 0.156.1 to run shell commands, make an `apply_patch` edit, run a
command the hook denies, and stop. Windows ran natively; Linux ran in WSL
(Ubuntu 24.04) with the matching release binary.

| Behaviour | Linux | Windows |
|---|---|---|
| `SessionStart`, `UserPromptSubmit`, `PreToolUse` (`Bash`, `apply_patch`), `Stop` fire | yes | yes |
| `SessionStart` stdout reaches the model | yes | yes |
| `PreToolUse` exit 2 blocks the command | yes | **no** |
| `Stop` exit 2 continues the turn; second stop has `stop_hook_active: true` | yes | **no** |
| Sync `PostToolUse` exit 2 reaches the model | yes | untested (see below) |
| Background hook, stderr + exit 2, reaches the model | **no** | untested |
| Background hook, JSON `additionalContext`, reaches the model | yes | untested |

On Windows every tool call failed inside Codex's own sandbox
(`helper_sandbox_lock_failed … SetNamedSecurityInfoW … failed: 5`), inside
and outside Claude Code, so `PostToolUse` never had a completed call to
report. The `Stop` result doesn't involve the sandbox and is unambiguous.
The cause of the lost exit code (for example, a PowerShell wrapper turning
2 into 1) was not established.

## Design in one paragraph

Split `agent-config` into **one module per agent runtime**, each owning its
own files and its own choice of events and commands, with no requirement
that two runtimes share a contract. Standards compose `claude-config` as
the supported agent integration. `codex-config` stays composed but
**suspended**: it still writes `.codex/config.toml`, and writes
`.codex/hooks.json` with no hooks, so that `vibe sync` actively removes the
hooks it wrote earlier. The `hook:*` tasks in each standard's `Taskfile.yml`
are unchanged; they are now the interface `claude-config` calls, and any
future agent module may call them, wrap them, or ignore them.

## Scope

### 1. `agent-config` becomes two modules

| Module | Package | Resources |
|---|---|---|
| `claude-config` | `internal/module/agents/claude` | `.claude/settings.json`, `.claude/hooks/policy.json` |
| `codex-config` | `internal/module/agents/codex` | `.codex/config.toml`, `.codex/hooks.json` |

- `.claude/settings.json` is **byte-identical** to today's. Nothing Claude
  Code runs changes in this spec.
- `policy.json` and its source (`policy.go`, the guard corpus) move with
  `claude-config`, because only Claude Code runs the guard now. The guard
  programs themselves stay in the repo-tooling modules, as today.
- Each module documents its own hook contract in its package comment:
  which events, which commands, and what the runtime does with exit codes
  and output. There is no shared "both agents" description any more.
- The command constants (`GuardCommand`, `ContextCommand`, …) move to
  `claude-config`. Nothing requires another module to use them.

Every standard's module list changes from `…, agents.New()` to
`…, claude.New(), codex.New()`, in that order.

### 2. Suspending Codex hooks

`.codex/hooks.json` becomes:

```json
{
  "hooks": {}
}
```

`.codex/config.toml` keeps `approval_policy` and `sandbox_mode`, drops the
`[features] hooks = true` block, and gains a comment pointing to this spec.
It does **not** set `hooks = false`: that could turn off hooks a user
configured in their own `~/.codex/`, which isn't VibeConform's to disable.

**Why not remove the files.** `vibe sync` never deletes a file a standard
stops producing (spec 0008, "No orphan pruning"). Dropping `.codex/hooks.json`
from the standards would leave every existing repository with the old hooks
on disk, still running, and no longer audited. An empty managed file removes
them on the next sync, and `vibe audit` flags anyone re-adding hooks by hand.
This repository's state file already carries the precedent for the other
path (`.claude/hooks/block-*.sh`, removed by hand after spec 0021); the empty
file avoids asking every adopter to do that.

**Resuming** is a later spec that fills `codex-config`'s `hooks.json` again,
designed against Codex's own contract (JSON output, its Windows exit-code
behaviour, `apply_patch` paths) rather than Claude Code's.

### 3. The wiring test follows the modules

`TestAgentConfigWiring` (spec 0021, extended by 0023) checks that every
command an agent config runs names a task the standard's `Taskfile.yml`
defines. It keeps that check, but per module: it applies to whichever agent
modules a standard composes and to their own config files, instead of a
fixed pair of paths. With `hooks.json` empty, `codex-config` contributes no
commands and passes trivially.

A new test pins the suspension: `codex-config`'s `hooks.json` decodes to an
empty `hooks` object, and `config.toml` contains no `[features]` hooks key.
Lifting the suspension means changing that test on purpose.

### 4. Documentation

- **New ADR, `docs/decisions/0011-one-module-per-agent-runtime.md`:** one
  module per agent runtime; Claude Code is the supported runtime; Codex
  hooks suspended, with the canary evidence and the conditions for
  resuming.
- **`docs/usage.md`:** the "Agent hooks" and "The agent guard" sections
  describe Claude Code only. The Codex gap bullets are replaced by one note:
  Codex hooks are suspended, what `.codex/` still contains, and a pointer to
  the ADR. The wrong #24453 claim goes.
- **`.codex/README.md`** (hand-maintained, not a resource): rewritten
  around the suspension and the canary results.
- **`AGENTS.md`** (hand-maintained here): the guardrails section names
  `.claude/settings.json` only and drops the Codex gap sentence.
- **`docs/architecture/overview.md`:** the package list shows
  `agents/claude` and `agents/codex`.
- **`docs/architecture/principles.md`:** the table row "Claude/Codex
  `PreToolUse` hooks" becomes Claude Code's.
- **`README.md`:** the `agent-config` description becomes the two modules.
- **Spec 0023, correction note:** section 4.3 says Claude Code discards an
  `async` hook's output. The current reference says it delivers
  `additionalContext` and `systemMessage` on the next turn. `asyncRewake`
  remains the right choice, because only it wakes Claude immediately on
  exit 2, so the correction is to the reasoning, not the configuration.

## Behavior

After upgrading `vibe`, in a repository on any standard:

- `vibe audit` reports `.codex/config.toml` and `.codex/hooks.json` out of
  date. `vibe sync` writes them, and Codex stops running VibeConform's
  hooks in that repository.
- `.claude/settings.json`, `policy.json`, and `Taskfile.yml` are unchanged:
  no audit finding, no difference in a Claude Code session.
- `audit`, `diff`, and `sync` output names `claude-config` and
  `codex-config` where it named `agent-config`. `.vibe/state.yaml` is keyed
  by path, so it needs no migration.
- **This repository dogfoods `prod-go`**, so its own `.codex/` files change
  in the same commit as the templates.

## Explicit non-goals

- **No change to anything Claude Code runs**: same events, matchers,
  commands, options, and tasks.
- **No new Claude hook work in this spec**, including the known matcher
  inconsistency (`MultiEdit`/`NotebookEdit` in `PostToolUse` but not
  `PreToolUse`). That is follow-on work, now free to change without
  touching Codex.
- **No Codex JSON output, `apply_patch` guard, or Windows workaround.** Those
  belong to the spec that resumes Codex hooks.
- **No orphan pruning** (spec 0008's non-goal stands).
- **No removal of Codex support altogether.** `.codex/config.toml` stays
  managed; only the hooks are suspended.
- **No change to the `hook:*` task names or behaviour.**

## Open questions

1. **Keep composing `codex-config`, or drop it from the standards?** This
   spec keeps it, so the empty `hooks.json` reaches existing repositories.
   Dropping it would leave today's hooks running in every repository that
   has them (section 2). A later spec can drop the module once adopters
   have synced past this one.
2. **Package layout.** `internal/module/agents/claude` and `…/codex` keep
   the two runtimes side by side. Top-level `internal/module/claude` and
   `…/codex` would match `gotooling`, `tstooling`, and the rest. Either works;
   this spec proposes the nested form so the runtimes read as siblings.

## Follow-on work

- Claude Code matcher alignment: `MultiEdit` and `NotebookEdit` in
  `PreToolUse` (the latter has `notebook_path`, not `file_path`, so the
  guard's policy needs it too).
- Resuming Codex hooks under their own contract, once a live canary on
  Windows shows exit codes or JSON decisions being honoured.
- Dropping `codex-config` from the standards, if Codex support is later
  retired rather than resumed.
- `hook:format` from a subdirectory: on Windows it fails because the
  root-relative paths git prints are resolved against the agent's working
  directory (found during implementation; see the plan).
- The repo-tooling `Taskfile.yml` templates' comments still describe the
  hook tasks as called by Claude Code and Codex.
