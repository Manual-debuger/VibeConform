# Spec 0039: Tool Paths in `vibe doctor`, and Graphify's Own Skill Beside VibeConform's

Status: accepted and implemented (ADR 0020). Issue #56. Implementation
plan: `docs/plans/0039-doctor-paths-graphify-coexistence.md`. Builds on
spec 0028 (environment doctor), spec 0035 (Graphify) and ADR 0016
(integration-owned `.gitignore` sections).

## Problem

1. **WSL.** In WSL, a tool on `PATH` may be the Linux binary or a Windows
   `.exe` reached through interop. Which one runs matters, for example a
   Windows `task` running a Linux repository's commands. `vibe doctor`
   prints a tool's path only when the tool has no version flag
   (`internal/doctor/doctor.go`, `tool`). The `git` line and the agent
   hooks line never print one.
2. **Graphify.** `graphify install` overwrites two files that
   VibeConform manages, so `vibe audit` reports both as drifted:
   - `.claude/skills/graphify/SKILL.md` is replaced by graphify's builder
     skill;
   - `.claude/settings.json` gains graphify's `PreToolUse` hook-guard
     hooks.

   Spec 0035 says not to run `graphify install`, but adopters want its
   builder skill (`/graphify`). The two skills do different jobs:
   VibeConform's is a query skill with the freshness and verification
   rules, and graphify's builds graphs. Removing VibeConform's loses
   those rules, and the `settings.json` drift remains either way.

## Behaviour

- **D1.** Every tool line (`tool`) and the `git` line show the resolved
  path. The detail is `<version> (<path>)`, or `<path>` for a tool with
  no version flag. A missing tool reads as before.
- **D2.** The agent hooks line names where each binary was found:
  `.claude/settings.json present; on PATH: task (/usr/bin/task), go (/usr/local/go/bin/go)`.
- **D3.** `doctor` stays read-only and still does not check conformance.
- **G1.** With `agents: [claude]` selected, `claude-config` owns a
  managed `.gitignore` section, `claude`. It uses `#` markers, sits at
  the bottom, and holds `.claude/settings.local.json`, Claude Code's
  per-developer settings file. That is where the coexistence layout puts
  graphify's hook-guard hooks.
  - Deselecting `claude` removes the section under spec 0026's rules.
  - ADR 0020 amends ADR 0016 to allow it.
- **G2.** `docs/usage.md`, Graphify section, documents the coexistence
  layout, which was verified in an adopter repository with graphify
  0.9.73:
  - VibeConform's query skill stays at `.claude/skills/graphify/`
    (managed).
  - graphify's builder skill moves to `.claude/skills/graphify-builder/`,
    with its `references/` and `name: graphify-builder` in its
    frontmatter.
  - graphify's hook-guard hooks go in `.claude/settings.local.json`,
    which Claude Code merges over the managed `settings.json`.
  - Git hooks stay with lefthook → `task graph:update` (spec 0035);
    `graphify hook install` would write into `.git/hooks/`, which
    lefthook owns.
  - Don't re-run `graphify install`. If you did, there are recovery
    steps.

## Non-goals

- A `vibe graphify-setup` command, or a `doctor` hint that moves files.
  `doctor` is read-only.
- Changing the graphify module's own resources.
- Detecting WSL interop specifically. The path is enough for a person to
  tell `/mnt/c/...` from `/usr/bin/...`.

## Acceptance criteria

1. `internal/doctor` tests assert the path in the detail of each found
   tool, the `git` line, and the agent hooks line.
2. The CLI doctor tests pass with the new format, and none is weakened.
3. With `claude` selected, the resolution includes the `.gitignore`
   section `claude` holding `.claude/settings.local.json`. Without
   `claude`, it does not.
   - A user's `.gitignore` lines are kept.
   - An absent `.gitignore` is created.
   - Deselecting `claude` removes the unmodified section.
4. This repository and every example that selects `claude` are synced
   and conformant. This repository's hand-written
   `.claude/settings.local.json` line is replaced by the section.
5. The docs in G2 exist, together with the updated doctor sample and
   wording in `docs/usage.md` and spec 0028, and ADR 0020.
