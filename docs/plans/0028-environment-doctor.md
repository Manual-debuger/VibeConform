# Plan 0028: Environment context and `vibe doctor`

Implements `docs/specs/0028-environment-doctor.md`. Tracks issue #44.
Spec accepted at review (2026-09-30).

Branch: `feat/environment-doctor`, off `main`, one pull request, merged
with a merge commit. It is the first of three sequential PRs: #44, then
#45 (spec 0029), then #42 (spec 0030).

## Resolved design questions

**Where do the doctor checks live?** In a new package, `internal/doctor`.
It defines `Status` (`Pass`, `Warn`, `Fail`, `Unverified`), `Result`
(status, name, detail) and `Report`, and it formats the report. Each
check is a function that takes an `Env`, which holds the seams:
`LookPath`, `Run(ctx, dir, name, args...)`, `Exists`, `GOOS` and `GOARCH`.
The package imports nothing from `internal/`. `internal/cli/doctor.go`
converts the plan and the module data into check inputs, so the
dependency direction is `cli → doctor`, and `doctor` depends on no module.

**How does doctor reuse `missingTools`?** `internal/cli/tools.go` is
split. A new `requiredTools(s, mctx)` walks the modules in order and
dedupes the tools, returning `(module, Tool)` pairs. `missingTools`
becomes a `lookPath` filter over that list, and doctor consumes the same
list, so the two cannot disagree about which tools the standard needs.

**How does a tool declare its version probe?** `module.Tool` gains
`Version []string`, the arguments to pass. If it is nil, doctor shows the
tool's path. The probes filled in are:

- `task --version`
- `go env GOVERSION`
- `lefthook version`
- `govulncheck -version`
- `actionlint -version`
- `golangci-lint version`
- `node --version`
- `pnpm --version`
- `uv --version`

`goimports` has no version flag, so it stays nil.

**How does the agent-hooks check learn the guard's runtime?** Through a
new optional interface, `module.HookRuntime`:

```go
type HookRuntime interface {
    HookBinaries(mctx *Context) []string
}
```

The four repo-tooling modules implement it:

| Standard | Hook binaries |
|---|---|
| `prod-go` | `task`, `go` |
| `prod-ts` | `task`, `node` |
| `prod-py` | `task`, `uv` (its guard runs `uv run --no-project python`) |
| `prod-mono` | `task` and the runtime of the first profile its guard uses, in the order Go, Node, Python |

The spec's "go, node, or python" means `uv` for `prod-py`. That is where
the guard command actually starts, and this plan records it.

**Where do the agent hook config paths come from?** From a small table
in `internal/cli/doctor.go`:

- `claude`: `.claude/settings.json`. The claude module's resource path
  is reused, not copied.
- `codex`: marked as suspended (spec 0024).

A catalog entry for an agent with no row in the table is a test failure,
so a new agent cannot ship without a doctor line.

**What happens when there is no `vibe.yaml`?** `buildPlan` fails, and
doctor reports `FAIL manifest` with the error text. The checks that do
not need the manifest still run: git, runtime and worktree. Everything
else is `UNVERIFIED` ("needs a valid vibe.yaml"). There is no `standard:`
header line.

**How are `Worktree` and `Runtime` detected, in the shell and in Go?**

- **Worktree:** both implementations compare `git rev-parse
  --path-format=absolute --git-dir` with `--git-common-dir` (Git 2.31 or
  newer). The main checkout is the common dir with its trailing `/.git`
  removed.
- **Runtime:** both use the same marker paths. Go exports them as
  `doctor.RuntimeMarkers`. A test in `internal/doctor` reads the four
  Taskfile templates from disk and asserts that each one names every
  marker. The test does not import the templates, so the dependency
  direction is unaffected.

**Timeouts:** version probes 5 s, and `git` and `task --list-all` 10 s.
A timeout is reported as `WARN` for a version probe, and as `FAIL` for
the git and Taskfile checks, since the workflow cannot rely on either.

## Repository impact

| Area | Change |
|---|---|
| `internal/module/{repotooling,tsrepotooling,pyrepotooling}/templates/Taskfile.yml`, `monorepotooling/templates/Taskfile.yml.tmpl` | `hook:context` gains the `Platform`, `Runtime`, `Shell` and `Worktree` lines. `Platform` is also printed outside git. |
| `internal/module/hook_tasks_test.go` | assert the new lines; add a linked-worktree case and an outside-git `Platform` case; the allowlist is unchanged |
| `internal/module/monorepotooling/monorepotooling_test.go` | the new lines appear in every profile combination |
| `internal/module/module.go` | `Tool.Version`, `HookRuntime` |
| `internal/module/*tooling*/…go` | fill `Version`; implement `HookBinaries` |
| `internal/doctor/` | new package: statuses, report, checks, marker constants, tests |
| `internal/cli/tools.go` | `requiredTools` split out; `missingTools` unchanged in behaviour |
| `internal/cli/doctor.go`, `commands.go`, `root.go` | real `doctor` command with `--repo-root`; the stub is removed |
| `internal/cli/{commands,root}_test.go` | stub assertions keep `check` only |
| `internal/cli/doctor_test.go` | exit codes, no-write, missing manifest, agent table covers the catalog |
| Root `Taskfile.yml`, `.vibe/state.yaml` | re-synced (C3) |
| `examples/{typescript,python,monorepo}` | `Taskfile.yml` and `.vibe/state.yaml` re-synced (C3) |
| Docs | `docs/usage.md`, `docs/architecture/overview.md`, `docs/architecture/principles.md`, `README.md`, spec 0028 status |

## Order of work

C1 (the spec) is committed. Every later commit leaves `task verify` and
`task audit` green.

1. **C2 `docs: plan 0028`**: this file. Gate: user approval.
2. **C3 `feat: hook:context reports platform, runtime, shell and
   worktree (spec 0028 C3)`**:
   - Update the four templates and their tests first, and watch the tests
     fail against the old templates.
   - Rebuild `vibe`, then `vibe sync` the root and the three examples.
     Commit each `Taskfile.yml` together with its `.vibe/state.yaml`.
3. **C4 `feat: vibe doctor with PASS/WARN/FAIL/UNVERIFIED (spec 0028
   C4)`**:
   - The `internal/doctor` framework.
   - The git, manifest, required-tools and Taskfile-loads checks.
   - `Tool.Version`, the `requiredTools` split, the command wiring, and the
     replaced stub tests.
4. **C5 `feat: doctor checks agent hooks, line endings, runtime and
   worktree (spec 0028 C5)`**:
   - `HookRuntime`, the agent table, and the remaining checks.
   - The runtime-marker drift test, and the no-write test.
5. **C6 `docs: doctor and environment context (spec 0028 C6)`**:
   - `usage.md`: the "vibe check, vibe doctor" section (now only `check`
     is a stub) and the `hook:context` section.
   - `principles.md`: the cross-platform contract under principle 2.
   - `overview.md:322` and the README's "not implemented" note.
   - Spec status set to implemented, this plan's checklist, and the
     verification record.

**Dogfooding hazard.** `go:embed` bakes templates into the binary. In C3,
rebuild before every `vibe sync`. `Stop` runs `verify:fast`, not `audit`,
so a stale `Taskfile.yml` would only show up in CI. Run `task audit`
before each commit.

## Checklist

- [x] C2 plan approved and committed
- [x] C3 `TestHookContextOutput` asserts `Platform: <goos>/<goarch>`, `Runtime: `, `Shell: unknown (not reported by the harness)`
- [x] C3 `TestHookContextOutsideGit` asserts `Platform:`
- [x] C3 `TestHookContextLinkedWorktree`: a `git worktree add` checkout prints `Worktree: linked`; the main checkout does not
- [x] C3 `TestHookContextRunsNothingHeavy` still passes with an unchanged allowlist
- [x] C3 monorepo: the new lines are present for every profile set
- [x] C3 root and examples re-synced; `vibe audit` is clean in all four
- [x] C4 `doctor`: one table test per check and status, through `Env` fakes
- [x] C4 exit codes: PASS/WARN/UNVERIFIED only exits 0; any FAIL exits 1; no manifest exits 1 and still runs git
- [x] C4 `missingTools` behaviour unchanged (existing `tools_test.go` green)
- [x] C4 stub tests cover `check` only
- [x] C5 agent hooks: config missing is FAIL; `task` missing is FAIL; guard runtime missing is FAIL; codex is PASS (suspended)
- [x] C5 every agent in the catalog has a doctor table row
- [x] C5 line endings: `eol=lf` is PASS; unset with autocrlf=true is WARN; unset otherwise is PASS "not pinned"; no git is UNVERIFIED
- [x] C5 runtime markers: Go constants and all four templates agree
- [x] C5 worktree: the main checkout is PASS; a linked worktree is UNVERIFIED, naming the main checkout and the unchecked collisions
- [x] C5 no-write: sync, doctor, then the tree and state are byte-identical
- [x] C6 docs updated; spec status set to implemented

## Found during implementation

- **Two probes changed from the plan (C4).** `govulncheck -version`
  prints `Go: go1.27.0` on its first line, not its own version, so it
  gets no probe (like `goimports`) and doctor shows its path.
  `golangci-lint version` prints a long build line; the probe is
  `golangci-lint version --short`, which prints `2.13.2`.
- **A worktree is one line, not two (C5).** Spec §2.3.8 describes a PASS
  line for the fact and an UNVERIFIED line for collisions. Doctor prints
  one line instead: `PASS` for the main checkout, and `UNVERIFIED` for a
  linked worktree, whose detail names the main checkout and the collisions
  that go unchecked. The collision risk exists only in a linked worktree,
  so an UNVERIFIED line on every ordinary clone would be noise.
- **The runtime markers are probed on Linux only (C5).** On Windows, Go
  would resolve `/.dockerenv` against the current drive, and a stray file
  there proves nothing. `hook:context` still runs `test -e` on every OS;
  there the paths never exist, so the two agree.
- **`task audit` uses the `vibe` on PATH (C3).** After a template change,
  `go install ./cmd/vibe` before syncing and auditing. A build elsewhere
  leaves `task audit` running an older binary, which refuses a verdict
  (spec 0019).

## Verification

Observed locally on 2026-09-30, on Windows 11 (windows/amd64) with Go
1.27.0, Task 3.53.1, Git 2.53.0.windows.1, and `core.autocrlf=true`:

- `task verify` passed at C3, C4, C5 and C6. `task audit` was conformant
  at each (13 resources), and `vibe audit` was conformant in all three
  examples after the C3 re-sync.
- `go test -count=1` passed for `internal/cli`, `internal/doctor`, and
  `internal/module/...`. The hook:context tests ran against the real Task
  binary, including `TestHookContextLinkedWorktree`.
- In the root, `task hook:context` printed `Platform: windows/amd64`,
  `Runtime: native`, `Shell: unknown (not reported by the harness)`, and
  no `Worktree:` line. In a `git worktree add` checkout it printed
  `Worktree: linked (main checkout at D:/VibeConform)`.
- On this machine, `vibe doctor` printed 14 `PASS` lines and exited 0.
  In the linked worktree it printed 13 `PASS` and 1 `UNVERIFIED`
  (worktree), and still exited 0.
- With `~/go/bin` stripped from PATH, `vibe doctor` exited 1. It printed
  `FAIL` for golangci-lint, goimports, govulncheck and actionlint, each
  naming the module that needs it, and `Error: doctor: 4 required checks
  failed`.

Not observed locally: Linux, and the WSL and container runtime markers.
The Ubuntu leg of CI covers Linux. The markers are covered only by
unit tests through the `Env` seam.

CI on the PR: `ci.yml` (Ubuntu and Windows test matrix), `examples.yml`,
`hook-guard.yml`, and `Conformance / audit` (recorded after the run).

## Explicitly still deferred

- `vibe doctor --json`.
- Filling in `Shell:` from a harness adapter.
- Doctor checks contributed by integrations (LSP, GitNexus).
- Probes for a local Linux path and for worktree collisions.
