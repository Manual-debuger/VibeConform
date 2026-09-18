# Spec 0003: Standard Registry (`internal/standard`)

Status: accepted and implemented.

## Problem

`vibe init <standard> <version>` accepts any strings for `standard` and
`version` (spec 0002 explicitly disclaims validation), and nothing in the
codebase defines what a "standard" actually is yet. Per
`docs/architecture/overview.md`, a standard is a named, versioned
composition of modules — but `internal/standard` doesn't exist, so
`audit`/`diff`/`sync` have nothing to resolve a manifest against.

## Scope

`internal/standard` defines:

- `Standard`: a named, versioned bundle of `module.Module`s.
- A registry: in-process, compiled-in registration (`Register(Standard)`),
  not a plugin or config-driven system — no dynamic standard loading exists
  yet.
- `Lookup(name, version string) (*Standard, error)`: returns the registered
  standard, or an explicit not-found error.

One seed standard, `production`/`v1`, is registered with zero modules, to
prove the registry plumbing end-to-end without inventing module
implementations prematurely — none exist yet; `internal/module.Module` has
no concrete implementations.

## Behavior

- `Register` panics on a duplicate `(name, version)` pair. This is a
  programmer error caught at package `init()` time, not a runtime
  condition.
- `Lookup` returns an error, not a panic, for an unknown `(name, version)`.
  This IS a runtime condition — e.g. a typo'd `vibe.yaml`.
- No CLI command is wired to this package in this change; that is
  deliberately deferred (see Follow-on work).

## Explicit non-goals

- No concrete `module.Module` implementations — this spec only wires the
  registry, not real modules.
- No CLI wiring: `vibe init` still does not validate its `<standard>`
  argument against the registry (spec 0002 already disclaims this; revisit
  once real standards exist and the cost of false-positive rejection is
  worth paying).
- No dynamic/config-driven standard loading (e.g. reading standards from
  YAML) — standards are compiled Go values registered at `init()` time.
- No resolution engine — `Standard.Modules()` is exposed but nothing calls
  `Resolve` on the returned modules yet.

## Design notes

- `internal/standard` depends on `internal/module`, preserving the
  dependency direction already documented in
  `docs/architecture/overview.md` — not the reverse.
- Compiled-in registration mirrors patterns like `database/sql` driver
  registration: predictable, no I/O, and defers the "should standards be
  user-authored/pluggable" design question until a concrete use case forces
  the answer.

## Follow-on work

- Wire `vibe audit`/`diff`/`sync` to `standard.Lookup` once a resolver
  exists to hand the returned `Standard.Modules()` to.
- Once a first real module exists (e.g. a Go-tooling module producing
  `.golangci.yml`), register it into `production`/`v1` and drop the
  zero-modules placeholder.
- Revisit whether `vibe init` should validate `<standard>` against the
  registry.
