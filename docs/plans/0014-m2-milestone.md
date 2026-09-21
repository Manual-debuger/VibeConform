# Plan 0014: M2 — hook registration, tool warnings, TypeScript/Python

Implementation plan for `docs/specs/0014-m2-milestone.md`. Unlike M1, which
sequenced six spec/plan pairs, M2 has one spec and this one plan: the
increments are small, so the per-increment detail that M1 put in six plans
lives in the "Sequence" section below.

One branch, `feature/m2-milestone`, merged with a merge commit. One commit
per increment, plus an opening docs commit for the spec and this plan.
Documentation updates land in the commit that changes the behavior they
describe, not in a trailing docs sweep.

## What M2 is

M1 made the loop correct. M2 makes it *effective* in two directions:
outward, so a written file actually does something (`lefthook install`) and
a missing prerequisite is visible (tool warnings); and sideways, so the
standard is not a single-language artifact. Nothing in M2 changes the
resolver, the reconciler, the state format, or `vibe.yaml`'s schema — it is
deliberately the cheapest milestone that answers whether the M1 model
generalizes.

## Where M1 left off

| Piece | State |
|---|---|
| `vibe init` / `audit` / `diff` / `sync` | all implemented; `check` / `doctor` still `errNotImplemented` |
| `production/v1` | four modules, 11 resources, all Go or language-neutral |
| Other standards | none registered; `vibe init foo v1` writes a manifest that resolves to nothing |
| External processes | `vibe` never executes a subprocess |
| Prerequisite checking | none; `sync` writes config for tools that may not exist |
| Git hooks | `lefthook.yml` is written; `.git/hooks` is never touched |
| This repository | conformant, self-auditing in CI |

The two gaps are the same gap seen twice: VibeConform knows what a
repository's files should say, and nothing about whether the machine can act
on them.

## Exit criteria

M2 is done when all four hold:

1. `vibe sync` runs `lefthook install` after a clean sync of a standard that
   manages `lefthook.yml`, and no failure mode of that step — missing
   binary, non-zero exit, not a git work tree — changes sync's exit code.
2. `vibe sync` writes one stderr warning per missing required binary, and
   neither `sync`'s nor `audit`'s exit code depends on tool availability.
3. `production-typescript/v1` and `production-python/v1` are registered and
   resolve lint/format/typecheck configuration, with no change to
   `manifest.Manifest` or `vibe.yaml`'s schema.
4. `examples/typescript` and `examples/python` are committed, hold the exact
   output of syncing their standard, and a Go test in the existing CI matrix
   fails if a template changes without them being re-synced.

## Sequence

### Commit 0 — `docs: add spec and plan for M2`

`docs/specs/0014-m2-milestone.md` and this file. No code.

### Commit 1 — hook registration

`internal/cli/sync.go`:

- `const lefthookResourcePath = "lefthook.yml"` and
  `planManagesLefthook(p *repoPlan) bool`, scanning `p.Resources` for a
  supported resource at that path. A plain function over the already-built
  plan — no new module surface, per the spec.
- `runSync` calls the registration step after `state.Save` succeeds, only
  when `applyErr == nil && saveErr == nil && counts.conflicts == 0`. It runs
  after the summary line so the resource report is never interleaved with
  subprocess output.
- Test seam: `var installGitHooks = runLefthookInstall`, unexported,
  documented as a seam. `runLefthookInstall(ctx context.Context, repoRoot
  string) error` does `exec.LookPath("lefthook")`, returning a sentinel
  `errLefthookNotFound` on a miss, then
  `exec.CommandContext(ctx, path, "install")` with `Dir: repoRoot` and
  combined output captured. Both the binary name and the argument are
  literals, so gosec's G204 has nothing to flag.
- Reporting: success → `lefthook: git hooks registered` on stdout;
  `errLefthookNotFound` → one stderr line naming what was not done (removed
  in commit 2, see the spec); any other error → one stderr line carrying
  lefthook's first line of output. `cmd.ErrOrStderr()` for stderr.
  The step returns nothing that `runSync` propagates.

`internal/cli/sync_test.go`:

- A `stubHookInstall(t)` helper that swaps `installGitHooks` for a recorder
  and restores it with `t.Cleanup`, called from the existing `runSyncIn`
  helper. Mandatory, not cosmetic: every current sync test runs against a
  `t.TempDir()` that is not a git work tree, so without the seam the whole
  suite starts shelling out to `lefthook install` and failing.
- New tests: registration happens exactly once on a clean sync, with the
  right repo root; does not happen when the run recorded a conflict
  (extend `TestSyncCmdRefusesToOverwriteConflict`); a stubbed install error
  produces a stderr line and still returns `nil`.
- `planManagesLefthook` gets a direct unit test over a synthesized
  `repoPlan` — including the negative case, which otherwise has no test
  until commit 3 registers a standard without `lefthook.yml`.

`docs/usage.md`: the paragraph at "Syncing `lefthook.yml` writes the
configuration; it does **not** register git hooks. Run `lefthook install`
yourself." is now false. Rewrite it, keeping the sentence after it — the
general rule that VibeConform never provisions toolchains — intact and
explicitly reconciled with the new behavior.

**Side effect on this repository.** From this commit on, `vibe sync` here
writes `.git/hooks`. It is idempotent, untracked, and exactly what lefthook
is for, but it is the first time `vibe` reaches outside the resource model.

### Commit 2 — tool-availability warnings

`internal/module/module.go`: add `Tool` and the optional `ToolRequirer`
interface, as specified. Nothing else in the package changes; `Module` keeps
its two methods.

Module declarations: `RequiredTools()` on `gotooling` (`golangci-lint`) and
`repotooling` (`task`, `lefthook`), each with a `Why` string naming what
stops working. `github` and `agents` implement nothing.

`internal/cli`: a `warnMissingTools(w io.Writer, s *standard.Standard)`
helper, called from `runSync` before `applyPlan`, deduplicating by binary
name in module then declaration order. `var lookPath = exec.LookPath` as the
second test seam.

Then delete commit 1's `errLefthookNotFound` warning, leaving a silent skip:
`repo-tooling` declares `lefthook`, so the missing binary is already
reported at the top of the run, and one missing binary should produce one
line of output.

Tests: warnings go to stderr and not stdout; a missing tool leaves the exit
code at 0 and leaves `vibe audit` reporting conformant on the same
repository (the exit-code invariant is the point of the increment, so it
gets its own test rather than being implied); deduplication across two
modules requiring the same binary; ordering is stable.

`docs/usage.md`: document the warning format under `vibe sync`, and state in
the exit-code section that tool availability never affects it.

### Commit 3 — `ts-tooling`, `python-tooling`, and two standards

`internal/module/tstooling/` and `internal/module/pythontooling/`, each
`gotooling`-shaped: embedded templates under `templates/`, a `New()`
returning `module.Module`, a `Resolve` returning `resource.Generated` in
fixed order, and tests covering name, resource order, determinism across two
`Resolve` calls, and the no-CR assertion every module has carried since the
`go:embed` defect.

Templates seeded from `D:\fai-am-bot` per the spec's table — verbatim for
`.prettierrc.json`, `tsconfig.base.json`, and everything in `ruff.toml` and
`pyrightconfig.json` that is not a layout glob; edited only where the spec
lists an edit. `ruff.toml` needs the section re-rooting (`[tool.ruff.lint]`
→ `[lint]`), which means it is not a byte copy and must be read against the
source by hand.

`ts-tooling` declares `node` under commit 2's interface; `python-tooling`
declares nothing.

`internal/standard/standard.go`: register `production-typescript/v1` as
`[tstooling, agents]` and `production-python/v1` as `[pythontooling,
agents]`. The registry, `Lookup`, and `manifest` are untouched.

`internal/cli/diff_test.go`: generalize `writeManifest` to take a standard
and version so the CLI tests can exercise a standard other than
`production/v1` — in particular the negative `planManagesLefthook` path from
commit 1, which now has a real standard to test against.

**The example repositories.** Built by running the tool, not by hand:
`vibe init production-typescript v1 --repo-root examples/typescript` then
`vibe sync` against it, same for `examples/python`, with `vibe.yaml`, the
resolved resources, and `.vibe/state.yaml` all committed.

`internal/cli/examples_test.go`, one test per example: call `buildPlan` on
`filepath.Join("..", "..", "examples", "<lang>")` and require every
`resourcePlan` to be `Supported` with `Decision == reconcile.NoChange`. The
failure message must name the fix — `vibe sync --repo-root examples/<lang>`
— because the failure will usually mean someone edited a template and
rebuilt without re-syncing, which is the whole behavior being enforced.

Two things this must not do:

- **Not a Taskfile or CI-workflow line.** Both files are resources of
  `production/v1` and ship to every adopting repository; an
  `--repo-root examples/typescript` line in either leaks this repository's
  layout into the standard. A Go test is the only place the check can live.
  It also gets the Windows leg of the existing `test` matrix for free, which
  matters more than usual here — see the CRLF risk below.
- **Not a subprocess.** It reuses `buildPlan` directly, the same walk
  `audit`, `diff`, and `sync` share, so the test cannot drift from what the
  commands do.

Docs, in this commit:

- `docs/usage.md`: a "What `production-typescript/v1` and
  `production-python/v1` manage" section with the resource tables, the
  deliberate gap (no Taskfile, no CI workflow, no dependency pinning) stated
  where a reader will hit it rather than only in the spec, and the seed
  provenance.
  Point at `examples/` as the worked example, the way the existing
  "VibeConform manages itself" section points at this repository for Go.
- `docs/architecture/overview.md`: add `tstooling/` and `pythontooling/` to
  the internal package layout, which currently reads "Present after M1", and
  note `examples/` as the drift alarm for templates this repository does not
  itself resolve.

### Smoke check — the seed repository, once

The milestone's gate is `examples/`, in CI, on every commit. This step is
not that. It is a single pass against `D:\fai-am-bot` before the merge,
buying the one thing five fixture files cannot: evidence that the rule sets
are survivable on a large real codebase, and that the generalized
`eslint.config.js` still lints something. It is Tier 2's stand-in until M3
automates the equivalent, and no exit criterion depends on it.

`D:\fai-am-bot` is a clean git work tree on `refactor/1347-config-deploy`,
so `git diff` is the instrument and `git checkout` is the undo.

1. `go build -o vibe.exe ./cmd/vibe` — `go:embed` resolves at build time, so
   a stale binary proves nothing.
2. `vibe init production-typescript v1 --repo-root D:\fai-am-bot\ts`, then
   `vibe sync` against the same root. Same for `production-python v1`
   against `D:\fai-am-bot\py`.
3. **Byte fidelity against the seed.** `.prettierrc.json` and
   `tsconfig.base.json` must report `unchanged` — they were seeded verbatim
   from those exact files, so anything else means the template drifted in
   transit, with CRLF the first suspect. `eslint.config.js` is expected to
   report `conflict` (it was edited, and a first sync with no recorded state
   cannot safely overwrite); confirm the difference is exactly the three
   documented edits and nothing else. `ruff.toml` and `pyrightconfig.json`
   are new files in `py/` and will report `created`; check them against
   `pyproject.toml`'s sections by hand.
4. **Does it work — the part `examples/` cannot answer.** With the generated
   configs, against that repository's real source: `ruff check --config
   <generated ruff.toml>` and `pyright -p <generated pyrightconfig.json>`
   need no file substitution. For eslint, copy the generated config over
   `ts/eslint.config.js` (the repo is clean; `git checkout` restores it) and
   run the repository's own lint script. The specific thing to confirm is
   that type-aware rules still fire after the `typedFiles` generalization —
   a config that lints *nothing* also exits 0.
5. **Confirm hook registration stayed out of it.** Neither new standard
   manages `lefthook.yml`, and `ts/`/`py/` are not git roots, so
   `D:\fai-am-bot\.git\hooks` must be untouched. Check it explicitly; it is
   the one action in M2 that reaches outside the resource model.
6. **Restore.** `git -C D:\fai-am-bot checkout -- .`, then delete the
   untracked files the run created, **by name**: `ts/vibe.yaml`, `ts/.vibe/`,
   `ts/.claude/`, `ts/.codex/`, and the same five under `py/` plus
   `py/ruff.toml` and `py/pyrightconfig.json`. Not `git clean` — that
   repository has gitignored `.env.local`, `node_modules/`, and `.venv/`,
   and the blast radius of a wrong flag there is somebody's local
   environment. Finish with `git -C D:\fai-am-bot status --short` clean and
   the branch unchanged.

Record the outcome in the PR description, including what step 4 actually
reported — "eslint ran and flagged N problems" is the evidence; "eslint
exited 0" is not. If step 3 or 4 fails, the fix is a template change, a
re-sync of `examples/`, and a re-run.

### Housekeeping (fold into whichever commit touches the file)

- `docs/plans/0007-m1-milestone.md`'s "Explicitly deferred past M1" list
  should point here for what M2 absorbed, mirroring what 0007 did to plan
  0001.
- `docs/specs/0014-m2-milestone.md` status → `accepted and implemented` in
  the last commit.
- `internal/cli/commands.go`'s `errNotImplemented` still cites
  `docs/plans/0001-bootstrap.md` for `check`/`doctor`; re-point it here,
  since this milestone is where both are explicitly deferred again.

## Explicitly deferred past M2

Unchanged from the spec's non-goals, restated as the M3+ queue:

- Per-language `repo-tooling` and `github-ci` variants — Taskfile, CI
  workflow, dependabot config for TypeScript and Python. The largest gap M2
  leaves open.
- Running the configured tools against `examples/` in CI (Tier 2): pinned
  dev dependencies per example, fixture sources with a deliberate violation
  each, a job on `setup-node`/`setup-python`. Pairs with the item above,
  since both introduce a non-Go toolchain to this repository's pipeline.
- Toolchain provisioning. Needs an ADR before it needs a plan.
- `vibe doctor`, and with it version and virtualenv awareness.
- Affected-component graph and `vibe check`.
- Multi-profile / multi-component repositories, and therefore the
  `profile:`/`components:` manifest fields sketched in
  `docs/architecture/overview.md`.
- `StructuredPatch` / `ManagedSection` apply logic — now wanted by three
  files (`.claude/settings.json`, `pyproject.toml`, `package.json`).
- Renaming `production/v1` to `production-go/v1`.
- `.vibe/lock.yaml`, `vibe new` / `vibe eject`, GitNexus and Skills Manager
  adapters.

## Cross-cutting risks

- **The examples prove fidelity, not validity.** They guarantee the bytes on
  disk are what the module resolved. They say nothing about whether eslint
  can parse the config or whether type-aware rules still fire — a lint run
  that checks nothing also exits 0. That gap is M2's known blind spot,
  deferred to M3 with the seed-repository smoke check standing in once. The
  risk is that a green CI reads as more assurance than it is; the mitigation
  is that both the spec and `docs/usage.md` say plainly what the examples do
  and do not test.
- **The examples add a maintenance obligation.** Every template change now
  requires a rebuild, a re-sync of two directories, and a commit of the
  result. That is the same discipline `AGENTS.md` already imposes for this
  repository's own managed files, and it is the cost of the drift alarm
  rather than an accident of it — but it is a third place to forget, and the
  failure mode is a red test rather than a wrong file, which is the right
  way round.
- **The first subprocess.** Commit 1 gives `vibe` the ability to change
  something outside the files it resolves. The containment is that there is
  exactly one such call, with literal arguments, gated on three conditions,
  and unable to affect an exit code. Every later request for "sync should
  also run X" should be made to argue against this paragraph.
- **Warning fatigue.** A warning that fires on a correctly configured
  repository is worse than no warning, because it trains people to skip the
  stderr block that also carries the real ones. This is the whole reason
  `python-tooling` declares no tools and `ts-tooling` declares only `node`.
  If M3 adds a tool to a module, the test is not "is this tool used" but
  "would a healthy repository ever lack it on PATH".
- **Seed provenance.** Five templates now come from a repository outside
  this one, and there is no `TestTemplateMatchesLiveFile` equivalent to
  catch drift — `D:\fai-am-bot` is a local path, not a dependency. Once the
  templates are committed, this repository is the source of truth and the
  seed is history; the spec records what was changed on the way in, and
  future edits are edits to the standard, not re-seeds.
- **CRLF, again — now in two places.** New embedded templates on a Windows
  working copy with `core.autocrlf=true` is the exact configuration that
  produced M1's `go:embed` defect. Each new package gets the no-CR test in
  the same commit that adds its templates. The examples double the exposure:
  their committed files are hashed byte-for-byte against the embedded
  templates, so a CRLF working copy makes the conformance test fail on
  Windows and pass on Linux. `.gitattributes` pins `* text=auto eol=lf`,
  which is what makes this safe — and the Windows leg of the existing `test`
  matrix is what makes it visible if it ever stops being true.

## Verification

`task verify` clean at every commit, CI reproducing it independently, and
`vibe audit` against this repository still passing — `Taskfile.yml` and
`lefthook.yml` are managed resources, so a commit that changes how they are
applied must leave this repository conformant.

From commit 3, `task test` also covers `examples/typescript` and
`examples/python`, on both legs of the existing matrix. No new CI job, no
new toolchain, no change to any shared template: that is what buying Tier 1
costs, and it is why it belongs in this milestone rather than the next one.

The seed-repository smoke check is additional, manual, and run once before
the merge. It gates nothing.
