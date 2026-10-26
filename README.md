# VibeConform

> **Status: pre-alpha.** `vibe audit` checks a repository against
> contract 1.1.0 (spec 0051), and this repository is gated by its own
> audit in CI (`Contract / audit`). The template-sync system (`vibe
> sync`, standards, `.vibe/state.yaml`) was removed in spec 0059;
> v0.13.x is the last release that has it. To move a repository off it,
> see [Leaving the template-sync system](docs/usage.md#leaving-the-template-sync-system).

## What VibeConform is

VibeConform checks that a repository's tooling (tasks, Git hooks, CI and
AI-agent guardrails) has the properties a versioned **contract**
requires. The contract is a list of rules, each with a stable ID such as
`C-02` or `A-02`, a level (MUST or SHOULD) and a check. It names
properties, never files or tools: a repository may use Task, make, just
or package scripts, lefthook or pre-commit, GitHub Actions or GitLab CI.
See [ADR 0025](docs/decisions/0025-contract-model.md) and
[spec 0051](docs/specs/0051-contract-v1.md).

Three parts, three roles:

- **`vibe audit`** verifies the rules, deterministically, and never
  writes a file. Where a rule is about behavior, it runs a *probe*: it
  plants a known defect, or feeds the agent guard a known command, in a
  throwaway copy of the repository, and checks the reaction.
- **The setup skill** (`vibeconform-setup`, written by `vibe init
  --skill`) is an agent skill that gives a repository those properties
  in whatever way fits it, then runs the audit until it passes. The
  repository owns the skill and every file it writes.
- **`vibe-guard`** is the reference agent guard: a separate binary that
  Claude Code runs before each shell command and file edit. Any guard
  that passes the guard probes will do.

## Core philosophy

1. **If a rule can be expressed mechanically, do not leave it only in an
   AI prompt.** Procedural guidance belongs in agent instructions
   (`AGENTS.md`, `CLAUDE.md`); mechanical rules belong in deterministic
   tooling (`Taskfile.yml`, CI, linters, hooks, `vibe audit`).
2. **Easy to use, cross-platform.** Windows and Linux are both
   first-class; a repository that conforms on one conforms on the other.
3. **Easy to install and easy to remove.** Nothing a conformant
   repository runs calls `vibe` (rule R-01). Its tooling stands on
   third-party tools (`task`, `lefthook`, `go`, `uv`, `ruff`, `pnpm`, …),
   so removing VibeConform leaves it working.

See [`docs/architecture/principles.md`](docs/architecture/principles.md)
for what each one means in practice.

## CLI

```text
vibe init            # write a vibe.yaml version 2 with the components it finds
vibe init --skill    # write the setup skill into the repository
vibe audit           # check the repository against the contract (exit 0, 2, or 1)
vibe doctor          # can this machine run the repository's workflow?
vibe unlock          # let an agent change the guardrail settings for a while
vibe-guard hook      # the agent guard, run by Claude Code's PreToolUse hook
```

## Using `vibe`

To adopt VibeConform in a repository, follow
[`docs/adopting.md`](docs/adopting.md), a start-to-finish walkthrough.
[`docs/usage.md`](docs/usage.md) is the reference manual: installing,
every command, `vibe.yaml`, the report and its exit codes, the setup
skill, the agent hooks and the guard.

The `examples/` directory holds three repositories assembled from the
setup skill's reference files (TypeScript, Python, and a Go, TypeScript
and Python monorepo). The `Examples` workflow audits each one in a fresh
repository on Linux and Windows.

## Build and test

Requires a current stable Go toolchain (this repository was bootstrapped
against Go 1.27.0).

```bash
go run ./cmd/vibe --help
go build ./...
go test ./...
```

[go-task](https://taskfile.dev/) is the canonical cross-platform
verification interface; CI runs the same tasks:

```bash
task verify     # fmt check, build, lint, tests, go mod tidy/verify, govulncheck, actionlint
task audit      # the full contract audit, with vibe and vibe-guard built from the checkout
```

## Repository layout

```text
cmd/vibe/            CLI entrypoint
cmd/vibe-guard/      The reference agent guard
internal/cli/        Command tree (init, audit, doctor, unlock)
internal/contract/   The contract audit: vibe.yaml version 2, the rules, static checks, probes
internal/guardrule/  Guard rule IDs, policy.json, command parsing shared by the audit and the guard
internal/guard/      vibe-guard: rules, hook, explain, relock, the reference hook commands
internal/setupskill/ The embedded setup skill and its reference files
internal/doctor/     vibe doctor's checks
examples/            Three repositories built from the setup skill's references
docs/                Specs, architecture, and decision records (source of truth)
```

## License

Apache License 2.0 — see [`LICENSE`](LICENSE).
