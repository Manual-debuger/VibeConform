# Spec 0001: V0 Control Plane (M0 Bootstrap)

Status: accepted for M0. Supersedes nothing.

## Problem

VibeConform should eventually be a desired-state repository control plane,
but it does not exist yet, and this very repository needs production
guardrails today. See `docs/plans/0001-bootstrap.md` for the bootstrap
sequencing that resolves this paradox.

## Scope of M0

M0 delivers:

1. A compiling Go CLI skeleton (`vibe`) with `init`, `audit`, `diff`,
   `sync`, `check`, `doctor` registered as commands that return an explicit
   "not implemented yet" error rather than silently succeeding.
2. Minimal, justified extension-point interfaces: `internal/manifest`
   (parses `vibe.yaml`'s `standard`/`version` fields only),
   `internal/module` (`Module.Resolve`), `internal/resource` (`Resource`,
   `Ownership`).
3. Manually authored production guardrails for *this* repository: CI,
   lint/format/test/security tooling, Git hooks, Dependabot, release
   preparation, and Codex/Claude Code project configuration.
4. SDD documentation structure (`docs/specs`, `docs/plans`,
   `docs/architecture`, `docs/decisions`).

## Explicit non-goals for M0

- No manifest resolver beyond field parsing.
- No reconciliation engine, lock file, or state file.
- No affected-component graph.
- No GitNexus or Skills Manager provider integration.
- No migration engine (`vibe eject`/`vibe new` do not exist yet).
- No `.vibe/lock.yaml` / `.vibe/state.yaml` — nothing writes or reads them
  yet, so they are not created.

## Definition of done

See `docs/plans/0001-bootstrap.md` for the full M0 checklist. In short: the
CLI builds and runs, local verification (`task verify`) passes, CI
reproduces the same verification independently, and this repository's own
guardrails are the reference example for what VibeConform will later
automate.

## Follow-on work (M1+)

Once `init`, `audit`, `diff`, `sync` have real implementations against a
manifest + standard + resolver, migrate this repository's manually managed
`.github/`, `.claude/`, `.codex/`, `Taskfile.yml`, `lefthook.yml` under
VibeConform itself, making VibeConform its own first dogfood repository.
