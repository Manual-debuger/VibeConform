# ADR 0001: Go + Cobra for the `vibe` CLI

## Status

Accepted.

## Context

VibeConform must be a cross-platform (Windows/Linux/macOS) CLI that
manipulates repository files, shells out to language tooling, and is easy
to distribute as a single static binary. It will eventually orchestrate
adjacent Go-based tooling (linters, `govulncheck`, GitHub Actions
validation).

## Decision

- Implementation language: Go, using a currently supported stable release
  (`go.mod` pins `go 1.27.0`, matching the toolchain and `golangci-lint`
  build available in the bootstrap environment; `govulncheck ./...` is
  clean against this version — 0 reachable vulnerabilities, including
  standard library. Go 1.27.1 is a safe upgrade target once a
  `golangci-lint` release is built against it).
- CLI framework: [Cobra](https://github.com/spf13/cobra) (`v1.10.2` at
  bootstrap time). Cobra is the de facto standard for Go CLIs, gives us
  subcommand structure, help/usage generation, and shell completion for
  free, and is already load-bearing in tools VibeConform will interoperate
  with (`gh`, `kubectl`, etc.). A hand-rolled `flag`-based CLI would save a
  dependency but would require reimplementing subcommand routing and help
  text as the command surface (`init`, `audit`, `diff`, `sync`, `check`,
  `doctor`, later `new`/`eject`) grows.
- Module path: `github.com/Manual-debuger/VibeConform` (see ADR-adjacent
  note below on casing).

## Module path casing

Go module paths are case-sensitive at the source level, but the module
proxy and module cache case-fold them, and GitHub itself treats
`Manual-debuger/VibeConform` and any lowercase variant as the same
repository for cloning purposes. We keep the module path matching the
actual GitHub repository casing (`Manual-debuger/VibeConform`) rather than
introducing a lowercase alias, since this repository is not yet published
as a dependency for other modules and matching the canonical repository
name avoids a second source of truth.

## Consequences

- `go install github.com/Manual-debuger/VibeConform/cmd/vibe@latest` will
  work once released.
- Command implementations live under `internal/cli`, one file per command
  group, so `vibe init`/`audit`/`diff`/`sync`/`check`/`doctor` can each grow
  independently without touching `root.go`.
