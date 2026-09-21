# Spec 0016: TS/PY Tooling Parity

Status: accepted and implemented.

## Problem

`prod-ts` and `prod-py` (spec 0015 names; `production-typescript` and
`production-python` before it) compose only their language-tooling module
(`ts-tooling`/`python-tooling`) plus `agent-config` — neither `repo-
tooling` nor `github-ci`, unlike `prod-go`. A repository declaring
`prod-ts` gets correct eslint/prettier/tsc configuration, but no
`Taskfile.yml`, no `lefthook.yml`, and no CI workflow at all: nothing to
actually run those linters through. `docs/specs/0014-m2-milestone.md`'s
follow-on work names this directly:

> Per-language `repo-tooling` and `github-ci` variants (M3), which is what
> makes `production-typescript/v1` a complete standard rather than a lint
> configuration.

Separately, nothing in this repository's CI ever runs eslint, ruff, or
pyright against real source — `examples/typescript` and `examples/python`
are config-only fixtures, so `TestExamplesAreConformant` (`internal/cli/
examples_test.go`) proves the templates round-trip through `vibe sync` but
not that they catch anything.

## Scope

Three increments.

### 1. `ts-repo-tooling` / `py-repo-tooling` modules

New packages, `internal/module/tsrepotooling` and `internal/module/
pyrepotooling`, mirroring `internal/module/repotooling`'s shape (fixed
content, `go:embed`, `resource.Generated`, no parameterization):

- `ts-repo-tooling` emits `Taskfile.yml` + `lefthook.yml` with `fmt`,
  `fmt:check`, `lint`, `typecheck`, `test`, `audit`, `verify` targets that
  shell out to eslint/prettier/tsc the way a Node project normally invokes
  them (`npx` or an equivalent local-binary path — not assumed to be on
  PATH, consistent with `ts-tooling`'s existing `RequiredTools` reasoning).
- `py-repo-tooling` emits the same target set shelling out to ruff/pyright/
  pytest the way a Python project normally invokes them (`uv run` or
  equivalent — not assumed to be on PATH, consistent with `python-
  tooling`'s existing no-`RequiredTools` reasoning).

Added to `prod-ts`/`prod-py`, which compose no repo-tooling module today.

### 2. `github-ci-ts` / `github-ci-py` modules

New packages, `internal/module/ci/githubts` and `internal/module/ci/
githubpy`, mirroring `internal/module/ci/github`'s shape:

- Each emits its own `.github/workflows/ci.yml` on `actions/setup-node` /
  `actions/setup-python` instead of `actions/setup-go`, calling the
  Taskfile targets from increment 1.
- Each also emits its own `dependabot.yml`: the shared one hardcodes
  `package-ecosystem: gomod`, so it is Go-specific despite its name — `npm`
  for TS, `pip` for Python (the well-established ecosystem that also reads
  `pyproject.toml`; a dedicated `uv` ecosystem is left as a future
  refinement, not a blocker here).
- `pull_request_template.md` is genuinely language-neutral (it only
  mentions `task verify`, uniform across all three standards) — reused as
  a shared embed rather than duplicated per language.

Added to `prod-ts`/`prod-py`, which compose no CI module today.

### 3. Example fixtures become real, and CI proves it

- `examples/typescript` and `examples/python` gain minimal real source,
  pinned dev dependencies (`package.json` / `pyproject.toml`), and one
  deliberate lint violation each — enough for eslint/ruff to have
  something to catch and pyright/tsc to have something to type-check.
- A CI job — hand-authored, **not** a `vibe`-managed resource (see Design
  notes) — runs `setup-node`/`setup-python` and executes eslint/ruff/
  pyright against the fixtures. A broken template now fails this job
  instead of only failing to round-trip through `vibe sync`.

## Behavior

`vibe init prod-ts v1` produces a TypeScript-flavored `Taskfile.yml`,
`lefthook.yml`, and `.github/workflows/ci.yml` instead of Go's. Same for
`prod-py`. `prod-go` is unchanged — it keeps `repotooling` and `github`
as-is.

## Explicit non-goals

- **`vibe doctor`** (environment/toolchain health: node version, venv
  presence, whether ruff/pyright actually run) — separate spec (0017).
  It's a general-purpose CLI feature, not TS/PY-specific, and `prod-go`
  doesn't have it either, so it is not required to reach parity.
- **`StructuredPatch` ownership** (owning a section of `package.json` or
  `pyproject.toml` instead of a standalone config file) — a general
  mechanism, not required for parity; the file-based approach already
  matches how `go-tooling` behaves relative to `go.mod`.
- No package-manager opinion beyond what's needed to run the tools: no
  npm vs. pnpm vs. yarn decision, no uv vs. plain-venv decision. Left to
  the implementation plan.
- No new standard beyond the existing three (`prod-go`, `prod-ts`,
  `prod-py`) — this is depth on two standards, not breadth.
- No renaming of `repotooling` or `github` (Go's variants) for symmetry
  with the new per-language package names — churn with no behavior
  change; noted as optional follow-on instead.

## Design notes

- **Why increment 3's CI job can't just live in the `github-ci` module's
  template.** `.github/workflows/ci.yml` in `examples/typescript` and
  `examples/python` is itself a *generated resource* — the exact output
  of `github-ci-ts`/`github-ci-py`'s `Resolve`, same as this repository's
  own `.github/workflows/ci.yml` is generated by `github-ci`. Encoding an
  `examples/typescript`-specific job into that template would leak this
  repository's own layout into a standard that ships to every adopting
  repository — exactly the constraint `internal/cli/examples_test.go`
  already documents for why example verification lives in a Go test
  rather than in `Taskfile.yml`/`ci.yml`. Increment 3's job must instead
  be a plain, unmanaged workflow file (or a job added to this
  repository's own hand-maintained CI) that is not part of any module's
  resolved resources.
- New package names: `internal/module/tsrepotooling`, `internal/module/
  pyrepotooling`, `internal/module/ci/githubts`, `internal/module/ci/
  githubpy`. `repotooling` and `github` keep their existing names — they
  are now implicitly "the Go variants" the same way `prod-go` is now
  explicitly the Go standard (spec 0015).

## Follow-on work

- `vibe doctor` (spec 0017).
- Renaming `repotooling`/`github` to `gorepotooling`/`githubgo` for full
  symmetry, if ever desired — cosmetic, not urgent.
