# VibeConform

> **Status: pre-alpha (M0 + `vibe init` v1 + `vibe audit` v1 + `vibe diff`
> v1 + `vibe sync` v1).** `vibe init` writes a `vibe.yaml` desired-state file (see
> [`docs/specs/0002-vibe-init.md`](docs/specs/0002-vibe-init.md)); `vibe
> audit` is the conformance gate: it checks every resolved resource against
> the repository and exits `2` when the repository is not conformant, `1`
> when it cannot answer at all
> (see [`docs/specs/0009-vibe-audit-v2.md`](docs/specs/0009-vibe-audit-v2.md)).
> The `production`/`v1` standard composes one real module, `go-tooling`,
> resolving a fixed `.golangci.yml` (see
> [`docs/specs/0005-gotooling-module.md`](docs/specs/0005-gotooling-module.md)).
> `vibe diff` previews the reconciliation VibeConform would perform for that
> resource against `.vibe/state.yaml` — see
> [`docs/specs/0006-reconcile-diff-v1.md`](docs/specs/0006-reconcile-diff-v1.md).
> `vibe sync` applies it: it writes `Generated` resources, records their
> hashes in `.vibe/state.yaml`, and refuses to overwrite a conflict (see
> [`docs/specs/0008-vibe-sync-v1.md`](docs/specs/0008-vibe-sync-v1.md)).
> `check` and `doctor` still return "not implemented yet."
> Nothing described below as "eventually" or "will" exists yet. See
> [`docs/plans/0001-bootstrap.md`](docs/plans/0001-bootstrap.md) for what M0
> delivered.

## What VibeConform is

VibeConform is a cross-platform repository control-plane CLI. The goal is a
tool that treats a repository's tooling, CI, hooks, linters, and AI-agent
configuration as **desired state** to reconcile against, not a one-shot
template to generate and then drift away from.

Once built, `vibe` will:

- initialize new repositories against a versioned production standard,
- migrate existing repositories onto that standard,
- audit repositories for compliance/drift (`vibe audit`),
- preview reconciliation (`vibe diff`) and apply it (`vibe sync`),
- run affected-component validation for a change set (`vibe check`),
- configure AI coding agents (Codex, Claude Code) and CI/hooks/linters
  using currently supported, non-fabricated mechanisms.

See [`docs/architecture/overview.md`](docs/architecture/overview.md) for the
full design, including how it adapts ideas from
[projen](https://github.com/projen/projen) (module composition),
[Copier](https://github.com/copier-org/copier) (three-way reconciliation),
[Cruft](https://github.com/cruft/cruft) (audit/diff/sync UX), and
[Nx](https://nx.dev/) (affected-component graphs).

## Core philosophy

> If a rule can be expressed mechanically, do not leave it only in an AI
> prompt.

Procedural guidance belongs in agent instructions (`AGENTS.md`,
`CLAUDE.md`); mechanical rules belong in deterministic tooling (`Taskfile.yml`,
CI, linters). This repository's own guardrails are meant to be the
reference example for the production standard VibeConform will eventually
enforce on other repositories — see the bootstrap rationale in
[`docs/plans/0001-bootstrap.md`](docs/plans/0001-bootstrap.md).

## Intended CLI

```text
vibe init <standard> <version>  # write vibe.yaml declaring the desired standard
vibe audit                      # read-only compliance/drift check (non-zero exit on failure)
vibe diff                       # human-readable reconciliation preview
vibe sync                       # perform reconciliation
vibe check                      # affected-component validation for the current change set
vibe doctor                     # diagnose local environment/tooling issues
```

Later: `vibe new`, `vibe eject`.

## Using `vibe`

See [`docs/usage.md`](docs/usage.md) for the user manual: install/build
instructions, the `vibe init` command reference, and exact error text for
each failure case.

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
task verify     # fmt check, build, lint, race tests, mod tidy, govulncheck, actionlint
```

## Repository layout

```text
cmd/vibe/          CLI entrypoint
internal/cli/      Command tree (root + init/audit/diff/sync/check/doctor)
internal/manifest/ vibe.yaml parsing
internal/standard/ Named, versioned standard registry
internal/module/   Module composition interface
internal/resource/ Resource + ownership model
docs/              Specs, plans, architecture, and decision records (source of truth)
```

## License

Apache License 2.0 — see [`LICENSE`](LICENSE).
