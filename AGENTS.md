# Agent Instructions

This file is a routing surface, not a handbook. Durable architecture lives
in `docs/` (specs, plans, architecture, decisions) — read the relevant doc
before non-trivial changes rather than relying on this file to explain the
system.

## Before non-trivial work

1. Read `docs/architecture/overview.md` and any relevant `docs/decisions/*`
   before changing package boundaries or the module/resource model, and
   `docs/architecture/principles.md` before changing what a module
   generates.
2. For anything beyond a small fix, produce or update a plan under
   `docs/plans/` following: requirement → spec → implementation plan →
   repository impact analysis → implementation → tests → verification →
   documentation sync.
3. Preserve the dependency direction documented in
   `docs/architecture/overview.md` (e.g. `internal/module` depends on
   `internal/resource`, not the reverse).

## Rules

- **This repository is managed by VibeConform.** Files that `vibe audit`
  lists are generated from module templates under `internal/module/`. Do not
  edit them directly: change the template, rebuild, run `vibe sync`, and
  commit both the file and the updated `.vibe/state.yaml`. A direct edit is
  non-conformant the moment it lands, and CI will say so. Run `vibe audit`
  (or `task audit`) to see the current list. `go:embed` resolves at build
  time, so a stale binary syncs stale templates — rebuild after every
  template change.
- Bug fixes require a regression test. Do not weaken or delete a test to
  make an implementation pass.
- Do not introduce a production dependency without recording why (an ADR
  under `docs/decisions/` for anything non-trivial).
- Run `task verify` **and** `task audit` before declaring work done. Since
  spec 0017, `verify` covers native language tooling only and no longer
  depends on `audit`, so a hand-edited managed file passes `verify` locally
  and fails only later, in CI's `conformance` job. Do not claim success
  with failing checks.
- Use repository intelligence (GitNexus, if configured) when a change has
  cross-file impact — it augments the compiler/linter/tests, it does not
  replace them.
- Mechanical rules belong in tooling (`Taskfile.yml`, `.golangci.yml`,
  `lefthook.yml`, CI), not just in this file — see
  `docs/architecture/overview.md`'s "canonical verification interface"
  section.

## Guardrails

- `.claude/settings.json` and `.codex/hooks.json` run `task -x hook:guard`
  before tool calls. It runs `.claude/hooks/guard.go` against
  `.claude/hooks/policy.json`, which blocks destructive commands (`rm -rf`,
  `git reset --hard`, `git push --force`, PowerShell's recursive forced
  `Remove-Item`, …) in Bash and PowerShell calls, and, for Claude Code,
  edits to secret-looking files. The rules live in
  `internal/module/agents/policy.go`; change them there, not in the
  generated `policy.json`.
- Known gaps (`docs/usage.md`, "The agent guard"): Codex on Windows fires
  no hook for shell commands, and Codex has no file-edit hook at all. A
  Taskfile that fails to load turns the guard off. A dangerous pattern
  quoted inside a command still matches.
- The guard is live in this repository: a Bash command that merely
  *contains* a denied pattern, even inside a heredoc, is refused. Write such
  text with a file-editing tool rather than through the shell. After
  changing the guard, check it with the canary in `docs/usage.md` rather
  than assuming when the running agent picked the change up.

These are guardrails, not a complete enforcement boundary — apply the same
judgment regardless of which agent runtime is in use.
