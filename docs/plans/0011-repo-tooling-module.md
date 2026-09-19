# Plan 0011: Repo-tooling module (`internal/module/repotooling`)

See `docs/specs/0011-repo-tooling-module.md` for the accepted scope, and
`docs/plans/0007-m1-milestone.md` for where this sits in M1.

## Checklist

- [x] `internal/module/repotooling/templates/Taskfile.yml` and
      `templates/lefthook.yml` — byte-for-byte copies of this repository's
      current files, copied with a command rather than transcribed.
- [x] `internal/module/repotooling/repotooling.go`: unexported struct
      satisfying `module.Module`, `New() module.Module`, `Name()` →
      `"repo-tooling"`, `//go:embed templates/*`, `Resolve` returning the
      two `resource.Generated` resources in documented order.
- [x] `internal/module/repotooling/repotooling_test.go`: `Name()`; `Resolve`
      returns exactly two resources with expected paths/ownership/non-empty
      content in the documented order; determinism across two calls.
- [x] `internal/module/repotooling/templates_test.go`: each embedded
      template equals the live file it was seeded from — the same drift
      alarm as spec 0010's, and more load-bearing here, since a broken
      `Taskfile.yml` breaks every CI job and every local `task verify`.
- [x] `internal/standard/standard.go`: register `repotooling.New()` after
      `github.New()`.
- [x] `internal/standard/standard_test.go`: extend the ordered module-name
      assertion to `go-tooling`, `github-ci`, `repo-tooling`.
- [x] `internal/cli/*_test.go`: update any assertion that depends on the
      full report or the resource count (now six resources).
- [x] `docs/usage.md`: `production/v1` now manages `Taskfile.yml` and
      `lefthook.yml`; document that syncing `lefthook.yml` does **not**
      install git hooks — `lefthook install` stays a manual step — and that
      VibeConform writes configuration but never provisions toolchains.
- [x] `README.md`: status banner — three modules.
- [x] `task verify` clean.

## Notes

- Sequencing within M1: this increment can be implemented in parallel with
  spec 0012's, since they touch disjoint files apart from
  `internal/standard/standard.go` and the shared CLI test assertions. If
  both are in flight, expect a conflict in exactly those two places.
- Reviewing the diff of this increment is mostly reviewing that two files
  were copied unmodified. `git diff --no-index Taskfile.yml
  internal/module/repotooling/templates/Taskfile.yml` should print nothing.

## Explicitly still deferred

Per `docs/specs/0011-repo-tooling-module.md`: no `.gitattributes`, no
`.gitignore`, no toolchain installation or version management, no
`lefthook install`, no parameterization, no conditionality, no ownership
mode beyond `Generated`. Unchanged from
`docs/plans/0010-github-ci-module.md`: no `release.yml`/`.goreleaser.yaml`,
no `--force`, no `--dry-run`, no `--json`, no orphan pruning or detection,
no `.vibe/lock.yaml`, no affected-component graph, no `check`/`doctor`, no
dynamic standard loading, no `vibe.yaml` overrides.
