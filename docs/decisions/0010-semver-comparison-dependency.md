# ADR 0010: Depend on `golang.org/x/mod/semver` for Version Ordering

## Status

Accepted. Implemented by `internal/state.CompareWriters`, per
`docs/specs/0019-drift-classification.md`. First production dependency
added since the repository was bootstrapped with `cobra` and `yaml.v3`.

## Context

Spec 0019 needs to answer one question: is the running `vibe` binary older
than the one that last wrote `.vibe/state.yaml`? If it is, `vibe sync`
would revert managed files to older templates — verified to silently undo
three specs' worth of work in a single command — and `vibe audit` cannot
judge conformance against a standard definition older than the
repository's.

Answering it means ordering two version strings. This repository had no
semver comparator, and `go.mod` required only `github.com/spf13/cobra` and
`gopkg.in/yaml.v3`.

## Decision

Add `golang.org/x/mod` and use its `semver` package.

The alternative — roughly sixty lines implementing semver precedence by
hand — was rejected on the evidence gathered while designing spec 0019.
Semver precedence is subtle in exactly the places this feature depends on,
and two measured results made the point:

**Prerelease identifiers compare lexically, not numerically.**

```text
semver.Compare("v0.2.0-alpha.1-42-gAAAAAAA", "v0.2.0-alpha.1-9-gBBBBBBB") == -1
```

A build 42 commits past a tag orders *below* one 9 commits past it. This
killed the original plan to stamp local builds with `git describe` output;
spec 0019 stamps a Go-style pseudo-version instead, precisely because that
format is defined to order correctly. A hand-rolled comparator would have
been written against the same wrong intuition that produced that plan.

**`Compare` returns `-1` for invalid input, not an error.**

```text
semver.IsValid("dev") == false
semver.Compare("dev", "v0.2.0-alpha.1") == -1
```

`cmd/vibe` sets `version = "dev"` for every build GoReleaser does not
stamp, so a comparator that trusted `Compare`'s result would report every
locally built binary as older than the recorded writer and refuse to sync
on the most common developer path there is. Measured against a deliberately
naive implementation, the same flaw also made an *absent* recorded version
(`""`, i.e. every existing schema-1 state file) compare as `RunningNewer` —
a confident "your repository is out of date" for every adopter's first run
after upgrading, and `("", "")` compare as `Same`.

Writing that comparator by hand, to guard a feature whose entire purpose is
preventing a destructive revert, is the wrong place to avoid a dependency.

## Consequences

- **Build footprint is standard library only.**
  `go list -deps golang.org/x/mod/semver` resolves to `slices`, `io`, and
  `strings`. `go list -m all` also shows `golang.org/x/tools`: that is a
  module-graph entry of `x/mod`, not a build dependency of the `semver`
  package, and nothing links it. `go mod tidy` added no new indirect
  requirements.
- `x/mod` is maintained by the Go team as part of the toolchain's own
  module handling — the same code `go` uses to order versions — so its
  behaviour and the behaviour of `go install …@latest` agree by
  construction. That matters here: the versions being compared are exactly
  the ones `go install` resolves.
- **Callers get no access to `Compare`.** `internal/state` exports
  `CompareWriters`, returning a `WriterOrder` enum whose zero value is
  `WriterUnknown`. A bare `-1/0/+1` would invite sign errors at call sites,
  and a forgotten assignment would yield a confident wrong direction
  instead of no claim. The `IsValid` gate lives in one function, with a
  regression test written against the naive implementation to confirm it
  catches the trap.
- `internal/state` was chosen over a new `internal/version` package because
  the recorded writer version is state provenance, lives in the same file
  `state` already reads and writes, and a package containing one comparator
  would not earn its boundary. It keeps `x/mod`'s blast radius to one leaf.
- Adding any further `golang.org/x/*` module is not implied by this ADR.
  Each is its own decision with its own footprint.
