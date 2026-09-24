# Plan 0022: Isolate, pin, and fall back for the conformance check

See `docs/specs/0022-conformance-isolation.md` for the accepted scope.
Approved at spec review (2026-09-24), including section 3a (sync warning
for unpinnable versions). Branch protection is left to the maintainer.

Branch: `feature/issue-26-conformance-isolation`, off `main`, one pull
request that closes #26.

## Resolved questions

- **Task's shell can do the fallback.** Prototyped under Task v3.53.1 on
  Windows: `command -v` resolves `.exe` via PATHEXT, and `while read` /
  `case` / `${v#…}` / `${v%…}` all work. A CRLF state file parses once the
  trailing CR is stripped. No external `sed`/`grep` needed.
- **`go install pkg@version` (and `go run`) inside an adopting Go repository** ignores that
  repository's `go.mod` (module-aware `@version` form), and VibeConform's
  `go.mod` has no `replace` directives, so the form is valid.
- **Spec 0018's `TestAuditInvokesVibeFromPath`** forbids building `vibe`
  out of the audited repository. `go install github.com/…/cmd/vibe@<v>` is a
  remote module, not a repository-local path, so the invariant still
  holds. The test is restated for `Taskfile.vibe.yml`: every `vibe`
  invocation is either a bare `vibe …` or
  `go install github.com/Manual-debuger/VibeConform/cmd/vibe@…`, and never
  `./cmd/vibe`.
- **`go install`, not `go run`.** Found in C1: `go run` reports a
  program's exit 2 as 1 (checked with go1.27.0), which would hide "not
  conformant" behind "could not answer". The fallback therefore installs
  into `$(go env GOCACHE)/vibeconform/<v>` and runs that binary. The spec
  was amended to match.
- **Check name.** The job's `name:` is literally `Conformance / audit`,
  mirroring `CI / gate`, so the branch-protection context is predictable.

## Repository impact

| Area | Change |
|---|---|
| `internal/module/conformance/` (new) | module, 2 templates, tests |
| `internal/standard/standard.go` | add `conformance.New()` to all 3 standards |
| `internal/module/ci/{github,githubts,githubpy}/templates/ci.yml` | drop `conformance` job and `gate` references |
| `internal/module/{repotooling,tsrepotooling,pyrepotooling}/templates/Taskfile.yml` | drop `audit`, add `vibe` include, reword `verify` desc |
| `internal/module/repotooling`, `pyrepotooling` `.go` | `RequiredTools` additions |
| `internal/module/verify_independence_test.go` | invariants move: repo-tooling Taskfiles never mention vibe; audit tests target `Taskfile.vibe.yml` |
| `internal/state` | `Pinnable(version string) bool` |
| `internal/cli/sync.go` | 3a warning |
| `internal/cli/tools_test.go` | expected declarations |
| Generated files (root, `examples/typescript`, `examples/python`) | re-synced, plus `.vibe/state.yaml` |
| Docs | `usage.md`, `architecture/overview.md`, `architecture/principles.md`, `README.md`, `AGENTS.md`, ADR 0008 note |

Dependency direction is unchanged. `internal/cli` depends on
`internal/state`, and the new module depends only on `internal/module`
and `internal/resource`.

## Order of work

Four commits, each leaving the tree green.

1. **C1: module, templates, tests.** The new module, CI and Taskfile
   template edits, and `RequiredTools`. Then rebuild, sync the root and
   both examples, and commit the regenerated files and state.
2. **C2: `Pinnable` and the sync warning (3a).**
3. **C3: docs.**
4. **C4: plan checklist ticked, verification recorded.**

## Checklist

### C1: module, templates, tests

Regression tests first, watched failing:

- [x] `conformance_test.go`: name `vibe-conformance`; resolves
      `.github/workflows/conformance.yml` then `Taskfile.vibe.yml`,
      `Generated`, slash paths, deterministic, LF, and templates match the
      live root files.
- [x] `conformance.yml`: parses; single job whose name is
      `Conformance / audit`; its last step runs `task audit`; no `@latest`
      anywhere; no `go install` of `vibe`; self-hosting probe present.
- [x] `Taskfile.vibe.yml`: only task `audit`. Every `vibe` invocation is
      bare `vibe …` or `go install github.com/Manual-debuger/VibeConform/cmd/vibe@…`
      (replaces `TestAuditInvokesVibeFromPath` / `TestAuditStillInvokesVibe`
      for this file).
- [x] Behaviour test (skipped unless `task` is on PATH): copy
      `Taskfile.vibe.yml` into a temp dir and set a PATH with neither
      `vibe` nor `go`, only `task`'s own directory. Table: no state file,
      `dev`, `v0.3.0+dirty`, missing key: each exits non-zero and prints
      the reason. `v0.2.0-alpha.1` with no `go`: non-zero, and the message
      names `go`. The same table drives the `state.Pinnable` test in C2.
- [x] `verify_independence_test.go`: no command anywhere in any
      repo-tooling Taskfile mentions `vibe`; each Taskfile has no `audit`
      task and includes `./Taskfile.vibe.yml` with `optional: true` and
      `flatten: true`.
- [x] Each CI template test: no `conformance` job, `gate.needs` excludes
      it, and the file does not contain `vibe`.
- [x] `tools_test.go` `TestRegisteredModulesDeclareTheirTools`: expect
      `go`, `goimports`, `govulncheck` and `actionlint` from
      `repo-tooling`. Add the same for `prod-py/v1` `uv` from
      `py-repo-tooling`.
- [x] `repotooling_test.go` and `pyrepotooling_test.go` `RequiredTools`
      expectations.
- [x] `wiring_test.go`: every registered standard includes
      `vibe-conformance`.

Then implementation:

- [x] `internal/module/conformance/{conformance.go,templates/conformance.yml,templates/Taskfile.vibe.yml}`
- [x] `standard.go` wiring.
- [x] CI templates: remove `conformance`; replace the "remove all three"
      comment with a pointer to `conformance.yml`.
- [x] Taskfile templates: remove `audit`, add include, reword `verify` desc.
- [x] `RequiredTools` additions, each with its `Why`.
- [x] Rebuild `vibe` (fresh binary, per the CRLF / stale-embed hazard).
      Run `vibe sync` in the root, `examples/typescript` and
      `examples/python`. `task verify` and `task audit` pass in each.
- [x] `examples.yml` still calls `task audit` after building `vibe`. No
      change is expected; confirm.

### C2: `Pinnable` and sync warning

- [x] `state.Pinnable` test from the shared table.
- [x] `sync` tests: warns for `dev` without `cmd/vibe`; silent when
      `cmd/vibe` exists; silent for a pinnable version; exit code
      unchanged.
- [x] Implement `state.Pinnable` and the warning in `runSync`, after the
      missing-tool warnings, on stderr.

### C3: docs

- [x] `docs/usage.md`:
  - "Removing VibeConform" becomes four deletions, plus the inert include.
  - A new "`task audit` without vibe installed" subsection covering
    pinning, the fallback, the failure messages and the 3a warning.
  - The per-standard "manages" lists gain the two files.
  - Required checks: `CI / gate` and `Conformance / audit`.
- [x] `docs/architecture/overview.md`: package layout gains
      `internal/module/conformance`.
- [x] `docs/architecture/principles.md`: "every CI job except
      `conformance`" becomes "every workflow except `conformance.yml`";
      update the mechanism table if it cites the old location.
- [x] `AGENTS.md`, `README.md`: wording that places `audit` in
      `Taskfile.yml` or conformance in `ci.yml`.
- [x] ADR 0008: a one-line note that the probe moved to `conformance.yml`
      and is now shared.
- [x] `grep -rn "conformance job\|needs.conformance\|vibe@latest"`
      returns only history (specs and plans up to 0021).
- [x] Spec 0022 status: accepted and implemented.

### C4: verification

- [x] `task verify` and `task audit` at the root and in both examples.
- [x] Manual fallback on Windows with `vibe` off PATH: state `dev` fails
      loudly. A temp copy with state `v0.2.0-alpha.1` resolves
      `go install …@v0.2.0-alpha.1`; a non-zero verdict is expected, because
      that release's templates differ.
- [x] Push. CI reports `CI / gate`, `Conformance / audit` and
      `Examples / gate` green.
- [x] The PR body gives the branch-protection step for the maintainer.

## Verification record (2026-09-24, Windows 11, go1.27.0, Task v3.53.1)

- `go test ./internal/... ./cmd/...`: all pass. `TestAuditFallbackNeverPassesSilently`
  ran 14 cases through Task's real shell, including CRLF state files.
- Root: `task verify` green (golangci-lint 0 issues, govulncheck clean,
  actionlint accepts `conformance.yml`); `task audit` conformant.
- `examples/typescript`, `examples/python`: `task verify` and `task audit`
  green after re-sync.
- Fallback, live: in a temp copy with `vibe_version: v0.2.0-alpha.1` and
  no `vibe` on PATH, `task -x audit` installed that release into
  `$(go env GOCACHE)/vibeconform/v0.2.0-alpha.1`, ran it, and exited 2
  (drift, expected against that release's older templates), which
  confirms exit codes pass through.
- The sync warning is silent at the root (which has `cmd/vibe`) and fires in
  `examples/python` for a `+dirty` build, as specified.
