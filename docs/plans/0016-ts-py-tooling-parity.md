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

- [ ] `internal/module/ci/githubts/githubts.go` + `templates/ci.yml`,
      mirroring `internal/module/ci/github`'s structure. `Name()` returns
      `"github-ci-ts"`. Uses `actions/setup-node`, calls increment 1's
      Taskfile targets. Reuses `github` package's `dependabot.yml` /
      `pull_request_template.md` content (embed directly, or factor into a
      shared template if duplication becomes a maintenance problem —
      decide at implementation time).
- [ ] `internal/module/ci/githubts/githubts_test.go`.
- [ ] `internal/module/ci/githubpy/githubpy.go` + `templates/ci.yml`.
      `Name()` returns `"github-ci-py"`. Uses `actions/setup-python`.
- [ ] `internal/module/ci/githubpy/githubpy_test.go`.
- [ ] `internal/standard/standard.go`: `prod-ts` and `prod-py` compose
      `github-ci-ts`/`github-ci-py` in place of generic `github`.
- [ ] Confirm module resolution order is documented and stable per
      standard (audit/diff/sync report in module order — existing
      contract from `docs/specs/0010-github-ci-module.md`).

### Increment 3 — real example fixtures + CI verification

- [ ] `examples/typescript`: add minimal real source + `package.json`
      with pinned eslint/prettier/typescript dev dependencies + one
      deliberate lint violation.
- [ ] `examples/python`: add minimal real source + `pyproject.toml` with
      pinned ruff/pyright dev dependencies + one deliberate lint
      violation.
- [ ] Re-sync both examples (`vibe sync --repo-root examples/typescript`,
      `--repo-root examples/python`) so `.vibe/state.yaml` and the
      generated files reflect increments 1–2's new module output.
- [ ] Add a hand-authored, unmanaged CI job (not part of any module's
      resolved resources — see spec 0016's design notes on why) running
      `setup-node`/`setup-python` and executing eslint/ruff/pyright
      against the two example fixtures. Confirm it fails on the
      deliberate violations before they're fixed, then fix them and
      confirm it passes (proves the job actually checks something).
- [ ] `internal/cli/examples_test.go`: confirm `TestExamplesAreConformant`
      and `TestExamplesCoverEveryLanguageStandard` still pass against the
      new module set.

### Cross-cutting

- [ ] `docs/usage.md` / `README.md`: document the per-language Taskfile/CI
      differences if the current docs describe `Taskfile.yml`/`ci.yml` as
      uniform across standards.
- [ ] `task verify` clean.

## Explicitly still deferred

Per `docs/specs/0016-ts-py-tooling-parity.md`: `vibe doctor` (spec 0017),
`StructuredPatch` ownership, any new standard beyond the existing three,
renaming `repotooling`/`github` for symmetry.
