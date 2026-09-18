# Claude Code Instructions

This file is a routing surface, not a handbook. Durable architecture lives
in `docs/` (specs, plans, architecture, decisions) — read the relevant doc
before non-trivial changes. Shared agent expectations (not Claude-specific)
live in `AGENTS.md`; read that too.

## Claude-specific notes

- Project hooks live in `.claude/settings.json` and block destructive Bash
  patterns (`git reset --hard`, `git push --force`, `rm -rf`) before they
  execute — see `.claude/hooks/block-dangerous.sh`. This is a guardrail,
  not a substitute for judgment: it cannot see every dangerous action, and
  it does not cover PowerShell equivalents unless a pattern is added.
- Use a plan (`ExitPlanMode`/plan mode) for non-trivial implementation work
  per `AGENTS.md`'s SDD workflow, rather than jumping straight to edits.
- When cross-file impact matters, prefer repository intelligence tools
  (GitNexus, if connected in this session) over guessing from a partial
  read of the codebase.

Everything else — verification commands, testing rules, dependency
policy — is in `AGENTS.md` and `docs/`, not duplicated here.
