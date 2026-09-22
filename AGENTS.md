# Agent Instructions

This file is a routing surface, not a handbook. Durable architecture lives
in `docs/` (specs, plans, architecture, decisions) — read the relevant doc
before non-trivial changes rather than relying on this file to explain the
system.

## Before non-trivial work

1. Read `docs/architecture/overview.md` and any relevant `docs/decisions/*`
   before changing package boundaries or the module/resource model.
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
  spec 0017 `verify` covers native language tooling only and no longer
  depends on `audit`, so a hand-edited managed file passes `verify` locally
  and fails the `conformance` job in CI. Do not claim success with failing
  checks.
- Use repository intelligence (GitNexus, if configured) when a change has
  cross-file impact — it augments the compiler/linter/tests, it does not
  replace them.
- Mechanical rules belong in tooling (`Taskfile.yml`, `.golangci.yml`,
  `lefthook.yml`, CI), not just in this file — see
  `docs/architecture/overview.md`'s "canonical verification interface"
  section.

## Guardrails

- `.codex/config.toml` and `.codex/hooks.json` block destructive shell
  patterns (`git reset --hard`, `git push --force`, `rm -rf`, etc.) at the
  tool-call level where the current Codex hooks mechanism supports it.
- `.claude/settings.json` and `.claude/hooks/block-dangerous.sh` block the
  same class of destructive Bash patterns for Claude Code, but do not cover
  PowerShell equivalents unless a pattern is added there too.

These are guardrails, not a complete enforcement boundary — apply the same
judgment regardless of which agent runtime is in use.
