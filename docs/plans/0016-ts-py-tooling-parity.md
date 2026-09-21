# Plan 0016: TS/PY Tooling Parity

See `docs/specs/0016-ts-py-tooling-parity.md` for the accepted scope.
Sequenced after plan 0015 (uses `prod-ts`/`prod-py` names).

## Checklist

### Increment 1 — `ts-repo-tooling` / `py-repo-tooling`

- [ ] `internal/module/tsrepotooling/tsrepotooling.go` + `templates/
      Taskfile.yml` + `templates/lefthook.yml`, mirroring `internal/module/
      repotooling`'s structure. `Name()` returns `"ts-repo-tooling"`.
      Targets: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `audit`,
      `verify` (mirroring `repotooling`'s target names so the interface
      stays uniform across standards), shelling out to eslint/prettier/tsc
      via local-binary invocation (not assumed on PATH).
- [ ] `internal/module/tsrepotooling/tsrepotooling_test.go` (template ==
      live-file equality test, mirroring `repotooling_test.go`).
- [ ] `internal/module/pyrepotooling/pyrepotooling.go` + `templates/
      Taskfile.yml` + `templates/lefthook.yml`. `Name()` returns
      `"py-repo-tooling"`. Same target set, shelling out to ruff/pyright/
      pytest via `uv run` or equivalent.
- [ ] `internal/module/pyrepotooling/pyrepotooling_test.go`.
- [ ] `internal/standard/standard.go`: `prod-ts` composes `ts-tooling`,
      `ts-repo-tooling`, `agents` (drop generic `repotooling`). `prod-py`
      composes `python-tooling`, `py-repo-tooling`, `agents` (drop generic
      `repotooling`).

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
