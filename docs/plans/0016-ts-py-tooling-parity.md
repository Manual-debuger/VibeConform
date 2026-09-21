# Plan 0016: TS/PY Tooling Parity

See `docs/specs/0016-ts-py-tooling-parity.md` for the accepted scope.
Sequenced after plan 0015 (uses `prod-ts`/`prod-py` names).

## Checklist

### Increment 1 — `ts-repo-tooling` / `py-repo-tooling`

- [x] `internal/module/tsrepotooling/tsrepotooling.go` + `templates/
      Taskfile.yml` + `templates/lefthook.yml`, mirroring `internal/module/
      repotooling`'s structure. `Name()` returns `"ts-repo-tooling"`.
      Targets: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `audit`,
      `verify`, `verify-ci` (mirroring `repotooling`'s target names so the
      interface stays uniform across standards; `build`/`run` are dropped —
      those are specific to building the `vibe` binary this repository
      ships, not generalizable to an adopting repository), shelling out to
      eslint/prettier/tsc via `npx` (not assumed on PATH). `audit` runs
      plain `vibe audit --repo-root .`, matching the documented external
      install path (`go install .../cmd/vibe@latest`), not the self-hosted
      `go run ./cmd/vibe` form `repotooling`'s own template uses.
- [x] `internal/module/tsrepotooling/tsrepotooling_test.go`: no live-file
      equality test (there is no live TS counterpart in this Go
      repository) — mirrors `tstooling_test.go`'s pattern instead
      (Name/Resolve/Deterministic/RequiredTools/LF-only).
- [x] `internal/module/pyrepotooling/pyrepotooling.go` + `templates/
      Taskfile.yml` + `templates/lefthook.yml`. `Name()` returns
      `"py-repo-tooling"`. Same target set, shelling out to ruff/pyright/
      pytest via `uv run`.
- [x] `internal/module/pyrepotooling/pyrepotooling_test.go`: same pattern
      as `tsrepotooling_test.go`.
- [x] `internal/standard/standard.go`: `prod-ts` now composes `ts-tooling`,
      `ts-repo-tooling`, `agent-config` (previously composed no repo-tooling
      module at all). `prod-py` now composes `python-tooling`,
      `py-repo-tooling`, `agent-config` (same gap, now closed).
- [x] Re-synced both examples (`Taskfile.yml`/`lefthook.yml` created);
      `vibe audit` clean on both afterward.
- [x] Fixed `TestSyncCmdSkipsHookRegistrationForStandardsWithoutLefthook`
      (`internal/cli/sync_test.go`): it asserted on `prod-ts` as "a standard
      with no lefthook.yml," which is no longer true once increment 1
      lands. Replaced with a package-local fixture standard
      (`test-fixture-no-hooks/v1`, registered via `init()` in
      `sync_test.go`) so the negative-case integration coverage survives.
      `TestPlanManagesLefthook`'s unit-level negative case was already
      standard-agnostic and needed no change.
- [x] `go build ./...`, `go test ./...` (141 passed), `task verify` all
      clean.

### Increment 2 — `github-ci-ts` / `github-ci-py`

- [x] **Correction to the spec during implementation:** `dependabot.yml` is
      *not* language-neutral — it hardcodes `package-ecosystem: gomod`.
      Only `pull_request_template.md` is genuinely shared content. Spec
      0016 updated accordingly before implementing.
- [x] `internal/module/ci/githubts/githubts.go` + `templates/ci.yml` +
      `templates/dependabot.yml` (npm ecosystem) + `templates/
      pull_request_template.md` (duplicated verbatim from `github`'s —
      three copies of a 16-line file is cheaper than a shared-template
      abstraction). `Name()` returns `"github-ci-ts"`. Workflow: `lint`,
      `typecheck_test`, `conformance`, `gate` jobs on `actions/setup-node`
      + `actions/setup-go` (Go is still needed to install `task` and
      `vibe` itself), calling increment 1's Taskfile targets. Action pins
      (`actions/setup-node@820762786026740c76f36085b0efc47a31fe5020` #
      v7.0.0) looked up live from GitHub's API rather than guessed, to
      match this repo's SHA-pinning convention honestly.
- [x] `internal/module/ci/githubts/githubts_test.go`: same no-live-file
      pattern as `tsrepotooling_test.go`, plus a
      `TestDependabotIsNPMNotGoMod` regression guard.
- [x] `internal/module/ci/githubpy/githubpy.go` + `templates/ci.yml` +
      `templates/dependabot.yml` (pip ecosystem — a dedicated `uv`
      ecosystem is a future refinement) + `templates/
      pull_request_template.md`. `Name()` returns `"github-ci-py"`. Uses
      `astral-sh/setup-uv` (`bec219d24cd3e171d82865faccec33120bb574f4` #
      v10.1.0) instead of `actions/setup-python` directly, since the
      Taskfile shells out via `uv run`.
- [x] `internal/module/ci/githubpy/githubpy_test.go`: same pattern, plus
      `TestDependabotIsPipNotGoMod`.
- [x] `internal/standard/standard.go`: `prod-ts` now composes `ts-tooling`,
      `github-ci-ts`, `ts-repo-tooling`, `agent-config` (previously
      composed no CI module at all). `prod-py` composes `python-tooling`,
      `github-ci-py`, `py-repo-tooling`, `agent-config` — same order
      `prod-go` uses (language tooling, CI, repo-tooling, agent-config).
- [x] Re-synced both examples; `vibe audit` clean on both (13 and 12
      resources respectively). `actionlint` run directly against both new
      `ci.yml` files (outside `task workflows:lint`'s repo-root-only
      scope) — clean.
- [x] `go test ./...` (151 passed), `task verify` clean.

### Increment 3 — real example fixtures + CI verification

- [x] `examples/typescript`: `package.json` (pinned eslint/prettier/
      typescript/typescript-eslint/@types/node dev deps — `typescript`
      pinned to `5.9.3`, not the newly-released `7.0.2`, since
      `typescript-eslint@8.70.0`'s peer range is `<6.1.0`, discovered by
      actually running `npm install`) + `package-lock.json`, `tsconfig.json`
      extending `tsconfig.base.json`, `src/index.ts`.
- [x] `examples/python`: `pyproject.toml` (`[dependency-groups] dev`, added
      via `uv add --dev ruff pyright pytest` rather than hand-pinned, so
      `uv.lock` carries the exact resolution) + `uv.lock`,
      `src/example/greet.py`, `tests/test_greet.py`. `[tool.uv] package =
      false` (no build backend needed for a fixture) plus `[tool.pytest.
      ini_options] pythonpath = ["src"]` (discovered by running `pytest`
      and hitting `ModuleNotFoundError` — package=false means nothing
      installs the src layout onto the path automatically).
- [x] Deliberate violation introduced and confirmed to fail in both
      (`eslint`: unused var → `@typescript-eslint/no-unused-vars`; `ruff`:
      unused import → `F401`), then fixed; confirmed clean afterward. The
      committed fixtures are clean, not permanently broken — the violation
      was a one-time proof the checks have teeth, per this checklist's own
      original wording.
- [x] Re-synced both examples after the increment-1/2 template fixes below.
- [x] Hand-authored, unmanaged `.github/workflows/examples.yml` (this
      repo's own CI, not a module resource) builds `vibe` from source,
      installs `task`, and runs `task verify` inside each example
      directory — reuses each fixture's own Taskfile rather than
      duplicating eslint/ruff/pyright commands, and checks against the
      templates actually in this change rather than the last release.
      `actionlint` clean.
- [x] Found and fixed a real bug: `ts-repo-tooling`'s `fmt`/`fmt:check` ran
      unscoped `prettier --write/--check .`, which also tried to reformat
      this standard's own generated YAML (`Taskfile.yml`, `lefthook.yml`,
      `.github/*`, `.vibe/*.yaml`) — caught by actually running it against
      `examples/typescript`, not just reading the template. Scoped to
      `**/*.{js,jsx,ts,tsx,json,css,md}`, matching what `lefthook.yml`'s
      pre-commit hook already correctly did. Template fixed, both examples
      re-synced.
- [x] `internal/cli/examples_test.go`: `TestExamplesAreConformant` and
      `TestExamplesCoverEveryLanguageStandard` pass against the new module
      set (source files aren't vibe resources, so they don't affect these
      tests either way).
- [x] Verified `task verify` end-to-end in both example directories
      locally, with a from-source `vibe` on `PATH` — matching exactly what
      `examples.yml` runs in CI — before committing.

### Cross-cutting

- [x] `docs/usage.md`: rewrote "What `prod-ts/v1` and `prod-py/v1` manage"
      — the old text described them as lint/format/typecheck-only with "no
      Taskfile, no CI workflow" as a documented limitation; replaced with
      the full per-language module table and what `ts-repo-tooling`/
      `py-repo-tooling`/`github-ci-ts`/`github-ci-py` add. Also updated the
      worked-examples section to mention real fixture source and
      `examples.yml`.
- [x] `README.md`: status banner updated from "M2 complete" to "M3 in
      progress," linking specs 0015/0016 alongside 0014.
- [x] `task verify` clean (repo root, with `examples/typescript/
      node_modules` present locally — harmless: `go test ./...` picks up
      a stray vendored `.go` file inside one npm dependency as a no-op `?`
      package; `node_modules/` is gitignored so this never reaches CI).

## Explicitly still deferred

Per `docs/specs/0016-ts-py-tooling-parity.md`: `vibe doctor` (spec 0017),
`StructuredPatch` ownership, any new standard beyond the existing three,
renaming `repotooling`/`github` for symmetry.
