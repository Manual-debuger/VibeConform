# ADR 0011: One Module per Agent Runtime; Codex Hooks Suspended

## Status

Accepted. Implemented by `internal/module/agents/claude` (`claude-config`)
and `internal/module/agents/codex` (`codex-config`), per
`docs/specs/0024-claude-first-agent-modules.md`.

## Context

Spec 0012 created one `agent-config` module for Claude Code and Codex,
because they shared a guard. Spec 0021 made both call the same
`task -x hook:guard` command, and spec 0023 extended that to five lifecycle
events: one command per event, the same for both agents, in every
standard. The premise was that exit code 2 and plain stderr mean the same
thing to every agent runtime.

A live canary on 2026-09-24 (Codex 0.156.1, natively on Windows and in WSL
on Linux) tested that premise:

| Behaviour | Linux | Windows |
|---|---|---|
| Hooks fire (`SessionStart`, `UserPromptSubmit`, `PreToolUse` for `Bash` and `apply_patch`, `Stop`) | yes | yes |
| `SessionStart` stdout reaches the model | yes | yes |
| `PreToolUse` exit 2 blocks the command | yes | no |
| `Stop` exit 2 continues the turn | yes | no |
| Background hook, stderr + exit 2, reaches the model | no | untested |
| Background hook, JSON `additionalContext`, reaches the model | yes | untested |

So under Codex on Windows the guard and the `Stop` gate enforce nothing,
and on every platform the background check's failures never reach the
model. Covering Codex file edits would also mean parsing paths out of
`apply_patch` bodies in all three guards. Every one of these is a
difference in Codex's contract, and with one shared module each fix
becomes a change to what Claude Code runs too. Meanwhile Claude Code's
hooks have been observed working in this repository's own sessions, on
Windows.

## Decision

1. **Each agent runtime is its own module**, owning its own files and its
   own choice of events, commands, and output format. No module is
   required to share another's hook contract.
2. **Claude Code is the supported agent runtime.** `claude-config` owns
   `.claude/settings.json` and the guard's `policy.json`; the `hook:*`
   tasks in each standard's `Taskfile.yml` are written for its contract.
3. **Codex hooks are suspended.** `codex-config` still writes
   `.codex/config.toml`, without the hooks feature flag, and writes
   `.codex/hooks.json` with no hooks. The file stays managed rather than
   removed, because `vibe sync` never deletes a file a standard stops
   producing (spec 0008): an empty managed file is what removes the old
   hooks from existing repositories, and `vibe audit` flags hooks added
   back by hand. The flag is not set to `false`, since that would also
   turn off hooks a user configured in `~/.codex/`.

## Consequences

- Claude Code hook changes no longer need a Codex counterpart.
- Codex sessions in a conformant repository get no guard, no session
  context, no formatting, and no `Stop` gate from VibeConform. The rules
  in `AGENTS.md` still apply, and CI remains the authoritative gate.
- `TestCodexHooksSuspended` fails if hooks reappear in `codex-config`'s
  templates, so lifting the suspension is a deliberate act.
- **Resuming Codex hooks** takes its own spec, designed against Codex's
  contract rather than Claude Code's: JSON output for anything reported
  from a background hook, `apply_patch` path parsing (relative and
  absolute) for the guard, and a live Windows canary showing either exit
  code 2 or a JSON block/deny decision being honoured.
- A later spec may drop `codex-config` from the standards altogether, once
  adopters have synced past spec 0024.
