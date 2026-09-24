# Spec 0025: `prod-mono/v1`, a Polyglot Monorepo Standard

Status: accepted and implemented.

## Problem

Every standard assumes one language at the repository root. A repository
with Go services, a TypeScript app, and a Python worker side by side has
no standard to declare:

- `prod-go/v1` would run `gofmt -l .` and `go vet ./...` from the root,
  where there is no `go.mod`, and ignore the TypeScript and Python code.
- There is no way to put `prod-ts`'s `eslint.config.js` in `apps/web` or
  `prod-py`'s `ruff.toml` in `services/worker`: resource paths are
  constants.
- `vibe.yaml` names one standard and nothing else, so nothing could tell
  a standard where the languages live even if it wanted to know
  (spec 0014 deferred this on purpose).

## Scope

### 1. `components:` in `vibe.yaml`

Per ADR 0012: an optional list of `{id, path, profile}` with `profile` in
`go`, `ts`, `py`; strict decoding; overlapping paths rejected. A standard
declares whether it takes components, and a mismatch in either direction
is an error before anything resolves.

```yaml
standard: prod-mono
version: v1
components:
  - id: api
    path: services/api
    profile: go
  - id: web
    path: apps/web
    profile: ts
  - id: worker
    path: services/worker
    profile: py
```

`vibe init prod-mono v1` writes the two-field `vibe.yaml` as for every
other standard; `vibe diff`/`sync` then say that `prod-mono/v1` needs at
least one component, and name the file to add them to.

### 2. Each component is a single-language project

A component is laid out exactly as the matching single-language standard
expects a repository to be, only at `path`: its own `go.mod`; its own
`package.json` (with `packageManager`) and `pnpm-lock.yaml`; its own
`pyproject.toml` and `uv.lock`. The standard ships the same language
configuration into it, byte for byte, that `prod-go`/`prod-ts`/`prod-py`
ship to a root:

| Profile | Resources at `<path>/` |
|---|---|
| `go` | `.golangci.yml` |
| `ts` | `eslint.config.js`, `.prettierrc.json`, `tsconfig.base.json` |
| `py` | `ruff.toml`, `pyrightconfig.json` |

They come from the existing `go-tooling`, `ts-tooling`, and
`python-tooling` modules, re-rooted, so there is one template per file
and no second copy to drift.

### 3. A Taskfile per component, one at the root

Each component gets a generated `<path>/Taskfile.yml` with its language's
verification tasks — `fmt`, `fmt:check`, `fmt:changed`, `lint`,
`typecheck`, `test`, `verify`, `verify:fast`, plus Go's `test:race`,
`mod:verify`, and `security`. Their commands are the single-language
templates' commands verbatim, and a test holds them to that. `task verify`
works from inside a component directory, as in a single-language
repository.

The root `Taskfile.yml` includes each component's Taskfile under its `id`
with `dir: <path>`, so `task api:test` runs `go test ./...` in
`services/api`. It keeps the interface every standard offers — `fmt`,
`fmt:check`, `lint`, `typecheck`, `test`, `verify`, `verify:fast`,
`verify-ci`, `workflows:lint`, and the `hook:*` tasks — each fanning out
to every component in manifest order. `Taskfile.local.yml` and
`Taskfile.vibe.yml` are included exactly as in the other standards.

The agent hooks (spec 0023) work per component:

- `hook:guard` runs the guard in the first runtime the repository has, in
  the order Go, Node, Python, so it needs no toolchain the repository
  does not already use.
- `hook:format` runs every component's `fmt:changed`, which formats only
  that component's changed files.
- `hook:check` runs every component's `typecheck`, `lint`, and `test`.
- `hook:context` prints the toolchain versions of the profiles declared
  and the component list.
- `hook:done` gates on `verify:fast`, as everywhere else.

### 4. `lefthook.yml`

Pre-commit: per component, `task fmt:check` and `task lint` run with
lefthook's `root:` set to the component, filtered by that language's file
globs, so a commit touching only `apps/web` runs only `web`'s checks.
Pre-push: `task verify:fast` at the root.

### 5. `github-ci-mono`

- `ci.yml`: one job per component, named by its `id`, which sets up only
  that component's toolchain, installs its dependencies in its directory,
  and runs `task <id>:verify`. Go components also run `task <id>:test:race`
  and, like `prod-go`, test on `ubuntu-latest` and `windows-latest`.
  A `workflows` job runs `task workflows:lint`, and `CI / gate` requires
  every job, so branch protection needs one check name whatever the
  component list.
- `dependabot.yml`: one entry per component in its ecosystem (`gomod`,
  `npm`, `pip`) and directory, plus `github-actions` at `/`.
- `pull_request_template.md`: the existing one.

### 6. `examples/monorepo`

A real Go + TypeScript + Python repository declaring `prod-mono/v1`,
synced and committed, covered by `TestExamplesAreConformant` and by a job
in this repository's `examples.yml` that runs `task verify`,
`task verify:fast`, `task hook:context`, and `task audit` in it.

## Behavior

- `vibe audit`/`diff`/`sync` report resources in module order, then
  manifest order within a module.
- Missing-tool warnings cover only the profiles a repository declares.
- Removing VibeConform is unchanged (`docs/usage.md`): the component
  Taskfiles are plain Task files and keep working.

## Explicit non-goals

- **pnpm or uv workspaces, and `go.work`.** Each component is its own
  project with its own lockfile. A root workspace changes where
  lockfiles and `packageManager` live, and so what CI caches and
  installs; it gets its own spec. `go.work` is not generated, and nothing
  here stops a repository adding one.
- **`depends_on` and affected-only verification.** Every task runs every
  component. The affected-component graph remains unbuilt (overview).
- **A component at the root, or nested components.** Rejected by the
  manifest (ADR 0012).
- **Per-component overrides** (a different Node version, extra CI steps).
  Repository tasks go in `Taskfile.local.yml`, as elsewhere.
- **Changes to `prod-go`, `prod-ts`, or `prod-py` output.** No generated
  byte changes for existing adopters.

## Design notes

- Why a Taskfile per component rather than one root file with `dir:` on
  every command: it keeps each component's commands identical to its
  single-language standard's, lets a contributor work inside one
  component with the commands they already know, and makes lefthook's
  per-component `root:` a one-liner.
- Why the root Taskfile is templated rather than embedded: its includes
  and fan-out are the component list. It is still deterministic — the
  same `vibe.yaml` always resolves the same bytes.
- Why one CI job per component rather than per check: a component's
  toolchain setup is the expensive part, and one job per component sets
  each up once. The single gate keeps the required-check list fixed.
