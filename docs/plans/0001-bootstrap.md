# Plan 0001: Repository Bootstrap (M0)

## Sequencing

```text
1. Manually bootstrap deterministic production guardrails (this plan).
2. Build VibeConform (init, audit, diff, sync) against a manifest + standard.
3. Migrate this repository's manually managed infra under VibeConform.
4. VibeConform becomes its own first dogfood repository.
```

## M0 checklist

- [x] Go module (`github.com/Manual-debuger/VibeConform`) and CLI skeleton
      (`cmd/vibe`, `internal/cli`) with placeholder subcommands.
- [x] Minimal `internal/manifest`, `internal/module`, `internal/resource`
      interfaces, each with tests.
- [x] `Taskfile.yml` as the single local/CI verification entry point
      (`fmt`, `lint`, `test`, `security`, `verify`, `verify-ci`).
- [x] `.golangci.yml` curated lint configuration.
- [x] `lefthook.yml` for fast pre-commit / stronger pre-push hooks.
- [x] `.github/workflows/ci.yml` with a stable `gate` job, third-party
      Actions pinned to commit SHAs.
- [x] `.github/workflows/release.yml` driving GoReleaser on tags.
- [x] `.github/dependabot.yml` for `gomod` and `github-actions`.
- [x] `AGENTS.md` / `CLAUDE.md` as concise routing surfaces.
- [x] `.claude/` and `.codex/` project configuration using currently
      supported mechanisms (see `docs/decisions` for what was verified).
- [x] Apache-2.0 `LICENSE`.
- [x] `PUBLIC_REPO_CHECKLIST.md` documenting GitHub Free/private-repo
      limitations (CodeQL, secret scanning, rulesets) to enable later.
- [x] `README.md` describing current (pre-alpha) status honestly.
- [x] Architecture/spec/decision docs (this directory tree).

## Explicitly deferred to M1+

- Manifest resolver beyond field parsing.
- Reconciliation engine and `.vibe/lock.yaml` / `.vibe/state.yaml`.
- Affected-component graph and `vibe check` real implementation.
- GitNexus and Skills Manager provider adapters.
- `vibe new` / `vibe eject`.
- Branch protection rulesets (blocked by GitHub Free + private repo; see
  `PUBLIC_REPO_CHECKLIST.md`).

## Verification run for this bootstrap

Commands actually executed locally before opening the PR (see PR
description for pass/fail results):

```text
go build ./...
go test ./...
go vet ./...
golangci-lint run
govulncheck ./...
gofmt -l .
goimports -l .
go mod tidy && git diff --exit-code -- go.mod go.sum
actionlint
task verify
```
