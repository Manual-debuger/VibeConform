# Plan 0025: `prod-mono/v1`

Implements `docs/specs/0025-prod-mono.md` and
`docs/decisions/0012-manifest-components.md`.

## Resolved design questions

- **Where component validation lives.** In `internal/manifest`: shape
  (id pattern, reserved ids, clean relative paths, overlap) and the closed
  profile set. Whether a standard takes components is the standard's
  (`Standard.TakesComponents`), checked by `internal/cli`'s `buildPlan`
  before anything resolves.
- **How modules see components.** `module.Context.Components`, read through
  `module.ComponentsOf`, which accepts the nil context tests resolve with.
- **Tool requirements.** `ToolRequirer.RequiredTools` takes the context:
  a static list would warn about every profile's tools in every monorepo.
- **One template per file.** `mono-tooling` re-roots `go-tooling`,
  `ts-tooling`, and `python-tooling`'s own resources; `mono-repo-tooling`
  takes the guard from the single-language repo-tooling module that owns
  it; `github-ci-mono` takes the pull request template from `github-ci`.
  The component Taskfiles are new files, held to the single-language
  commands by `TestComponentTaskfilesMatchSingleLanguage`.
- **Templating.** Root `Taskfile.yml`, `lefthook.yml`, `ci.yml`, and
  `dependabot.yml` are `text/template` with `[[ ]]` delimiters, leaving
  Task's and GitHub Actions' own `{{ }}`/`${{ }}` untouched.

## Found during implementation

- **Task's built-in `xargs` ignores an include's `dir`.** On Windows, Task
  runs `xargs` in-process (its coreutils), and the command it launches
  gets the process's directory, not the included Taskfile's `dir`. So
  `task api:fmt:changed` from the root cannot find the files git listed
  relative to `services/api`. Reproduced in isolation (`printf 'f.txt\0' |
  xargs -0 cat` under an include with `dir:` fails; the same task run as
  `(cd svc/a && task -s x)` works). The root's `hook:format` therefore
  starts each component's `fmt:changed` as a child `task` inside the
  component directory. `examples.yml` exercises it on Windows and Linux.
- **setup-go's cache key needs a file that exists.** A Go component with no
  dependencies has no `go.sum`, so the generated job keys the cache on
  `go.mod`.

## Checklist

- [x] ADR 0012, spec 0025.
- [x] `manifest`: `components:`, strict decoding, validation, tests.
- [x] `module.Context.Components`, `ComponentsOf`; `RequiredTools(mctx)`
      across all implementers; `warnMissingTools` gets the plan's context.
- [x] `standard.TakesComponents`; `buildPlan` refuses mismatches.
- [x] `mono-tooling`, `github-ci-mono`, `mono-repo-tooling`, each with
      tests; `prod-mono/v1` registered.
- [x] `TestAgentConfigWiring` resolves component standards with a sample
      component of every profile.
- [x] `examples/monorepo` synced and committed; `TestExamplesAreConformant`
      and `examples.yml` cover it.
- [x] `docs/usage.md`, `README.md`, `docs/architecture/overview.md`.

## Verification

Observed locally (Windows 11, 2026-09-24; Go 1.27.0, Task 3.53.1,
lefthook 2.1.14, pnpm 10.34.5, uv 0.10.8):

- `task verify` and `task audit` at the repository root: pass, conformant.
- In `examples/monorepo`: `task verify`, `task hook:context`,
  `task -x hook:check`, `task -x hook:done`: exit 0. `task -x hook:format`
  formatted an unformatted untracked file in each of the three
  components. `task -x hook:guard` denied a `git reset --hard` payload
  with exit 2. `task audit` (vibe built from this tree on `PATH`):
  21 resources, conformant. `actionlint` on the generated workflows:
  clean.
- lefthook, in a scratch git copy of the example: with only a Python file
  staged, only `worker-fmt` and `worker-lint` ran; an unformatted staged Go
  file failed `api-fmt`.

Not observed locally: the generated `ci.yml` and the new `examples.yml`
job on GitHub-hosted runners, until the pull request runs them.

## Explicitly still deferred

As in the spec: pnpm/uv workspaces and `go.work`, `depends_on` and
affected-only runs, root or nested components, per-component overrides.
