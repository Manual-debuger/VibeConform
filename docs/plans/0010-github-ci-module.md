# Plan 0010: GitHub CI module (`internal/module/ci/github`)

See `docs/specs/0010-github-ci-module.md` for the accepted scope, and
`docs/plans/0007-m1-milestone.md` for where this sits in M1.

## Checklist

- [x] `internal/module/ci/github/templates/ci.yml`,
      `templates/dependabot.yml`, `templates/pull_request_template.md` —
      byte-for-byte copies of this repository's current `.github/workflows/
      ci.yml`, `.github/dependabot.yml`, `.github/pull_request_template.md`.
      Copy them with a command, not by hand: a transcription typo here
      becomes a spurious conflict for every repository that syncs.
- [x] `internal/module/ci/github/github.go`: unexported struct satisfying
      `module.Module`, `New() module.Module`, `Name()` → `"github-ci"`,
      `//go:embed templates/*` wiring, `Resolve` returning the three
      `resource.Generated` resources in the documented order with
      slash-separated paths.
- [x] `internal/module/ci/github/github_test.go`: `Name()`; `Resolve`
      returns exactly three resources with the expected paths, ownership,
      and non-empty content; resolution order is the documented one; two
      calls are byte-identical (determinism); every resource path is
      slash-separated (`!strings.Contains(path, "\\")`), which is what keeps
      state keys identical across platforms.
- [x] `internal/module/ci/github/templates_test.go`: each embedded template
      matches the live file it was seeded from
      (`../../../../.github/...`). This is the drift alarm for the window
      between this increment and the dogfood one — without it, the two
      copies can diverge silently and nobody finds out until 0013.
- [x] `internal/standard/standard.go`: register `github.New()` after
      `gotooling.New()` in `production`/`v1`.
- [x] `internal/standard/standard_test.go`: assert `production`/`v1`'s
      module names in order (`go-tooling`, `github-ci`), not just a count.
- [x] `internal/cli/sync_test.go`: add a nested-path case — sync into an
      empty temp repo creates `.github/workflows/ci.yml` including its
      parent directories, and records the slash-separated key
      `.github/workflows/ci.yml` in state. This is the test spec 0008
      deferred to this increment.
- [x] `internal/cli/diff_test.go` / `audit_test.go`: existing single-resource
      assertions still hold, but the reports now list four resources —
      update any assertion that depends on the full output or the resource
      count.
- [x] `docs/usage.md`: example outputs now list four resources; note that
      `production/v1` manages `.github/` and that `release.yml` /
      `.goreleaser.yaml` deliberately stay project-owned.
- [x] `README.md`: status banner — `production`/`v1` composes two modules.
- [x] `task verify` clean.

## Notes

- The template-vs-live equality test is the single most valuable test in
  this increment. Everything else is mechanical; that one catches the
  failure mode this increment actually introduces.
- Nothing in this increment changes this repository's real `.github/` files.
  They are still hand-maintained until 0013 — the module only knows how to
  produce them.

## Explicitly still deferred

Per `docs/specs/0010-github-ci-module.md`: no `release.yml`, no
`.goreleaser.yaml`, no conditional/optional resources, no parameterization
(Go version, action pins, job names are fixed), no runtime validation of the
embedded workflow, no ownership mode beyond `Generated`, no interface
changes. Unchanged from `docs/plans/0009-vibe-audit-v2.md`: no `--force`,
no `--dry-run`, no `--json`, no orphan pruning or detection, no
`.vibe/lock.yaml`, no affected-component graph, no `check`/`doctor`, no
dynamic standard loading, no `vibe.yaml` overrides.
