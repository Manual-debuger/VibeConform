# ADR 0007: Release Tag Naming

## Status

Accepted. Not implemented by `vibe` — `.github/workflows/release.yml` and
`.goreleaser.yaml` are hand-maintained, by the non-goal recorded in
`docs/specs/0013-dogfood-self-management.md`. This document is the policy
that governs what a human types when cutting a tag; nothing enforces it in
code.

## Context

`release.yml` triggers GoReleaser on any tag matching `v*`, and
`.goreleaser.yaml` templates `{{ .Version }}` from that tag with no
`release.prerelease` override, so GoReleaser falls back to its default:
auto-detect a prerelease from the semver string. No versioning scheme had
been decided before the first tag — nothing tied a tag to the project's
milestone numbering (M1, M2, …) or to the "pre-alpha" status
`README.md`'s banner already declares.

## Decision

Tags are plain semver with a `v` prefix: `vMAJOR.MINOR.PATCH[-PRERELEASE]`.

- **Major stays `0`** for as long as `README.md`'s status banner says
  pre-alpha/pre-beta — semver's "anything may change" phase, matching
  reality: `production/v1`'s content (Go version, action pins, rule sets)
  is still fixed-and-young, not a compatibility promise.
- **A preview/snapshot cut** — one that exists to be tried, not to be
  depended on — gets a prerelease suffix: `-alpha.N` today, moving to
  `-beta.N` or `-rc.N` only once there's a real stability signal to make.
  `N` increments per preview; it does not reset or reuse.
- **Milestone numbers and version numbers are deliberately not coupled.**
  `M1`, `M2`, … track scope (what `docs/plans/000N-mN-milestone.md`
  covers); the tag tracks *when a preview was cut* and *how stable it is*.
  Cutting `v0.1.0-alpha.2` after M3 lands is fine — it does not imply
  `v0.1.0-alpha.2` == "M3 complete".
- The first tag is `v0.1.0-alpha.1`.

The practical effect of the `-alpha.N`/`-beta.N`/`-rc.N` suffix: GoReleaser
reads it as a semver prerelease and marks the resulting GitHub Release
"Pre-release", which keeps it out of "Latest release" on the repository's
Releases page. That distinction is what a preview tag is for — the
combination with `release.draft: true` (a human publishes explicitly) means
a snapshot never gets mistaken for a real, supported cut.

## Consequences

- Nothing validates a pushed tag against this scheme. A typo'd tag
  (`v1.O.0`, `V0.1.0`) still matches `release.yml`'s `v*` trigger and still
  runs GoReleaser; whether it *builds a sensible version string* is on the
  human pushing it, not on CI.
- The first stable `v1.0.0` is a real decision to make later — this ADR
  only commits to how previews are named until then, not to when major `0`
  ends.
- Tag protection (a ruleset on `v*`, see the repository's Settings → Rules)
  guards these tags from force-push/deletion once pushed, but does nothing
  to enforce the naming convention itself — that stays a documented
  agreement, not a checked one.
