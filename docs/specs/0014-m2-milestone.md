# Spec 0014: M2 — hook registration, tool warnings, TypeScript/Python

Status: proposed.

One spec for the whole milestone rather than a spec/plan pair per
increment, as M1 used. The three increments below are small and share one
question — *what does a repository need beyond a correct file on disk?* —
so splitting them into three specs would repeat the same framing three
times. The implementation plan is `docs/plans/0014-m2-milestone.md`.

## Problem

M1 closed the reconciliation loop: `vibe sync` writes the files a standard
resolves, `vibe audit` gates on them, and this repository manages its own
guardrails. The loop is correct and, in two ways, inert.

**A written file is not an active file.** `production/v1` resolves
`lefthook.yml`, and a repository that syncs it gets the configuration
without the git hooks: lefthook registers hooks by writing into `.git/hooks`
when `lefthook install` runs, and nothing runs it. The pre-commit gate that
the standard exists to provide is off until a human reads `docs/usage.md`
closely enough to notice the sentence telling them to run it themselves.
That is a guardrail that fails silently and fails open — the same failure
mode spec 0012 called the worst in M1, arrived at from the other direction.

**A conformant repository can still be unusable.** `vibe sync` will happily
write `.golangci.yml`, `Taskfile.yml`, and `lefthook.yml` into a machine
with none of `golangci-lint`, `task`, or `lefthook` installed, report `0
conflicts`, and exit 0. `vibe audit` will then call it conformant. Both are
telling the truth — the files match the standard — and both are unhelpful,
because the next command the user runs fails with a shell error that says
nothing about what is missing.

**The standard speaks one language.** `production/v1` is Go, end to end:
`.golangci.yml`, a Taskfile of `go` invocations, a CI workflow on
`actions/setup-go`, a dependabot config on `gomod`. Everything VibeConform
claims about codifying a baseline is, so far, a claim demonstrated once in
one ecosystem. Until a second language exists, the module/standard/resource
model is unfalsified: nothing has tested whether the abstraction survives
contact with a repository that is not this one.

## Scope

Three increments, in order. Each is one commit on one milestone branch.

### 1. Hook registration in `vibe sync`

`vibe sync` runs `lefthook install` after a successful sync of a standard
that manages `lefthook.yml`. Implemented as a special case inside
`internal/cli/sync.go`, keyed on the resource path — **not** as a new method
on `module.Module`. One consumer does not justify widening the interface
every module must implement, and the alternative generalization (modules
declaring post-apply commands) is a plugin system nobody has asked for.

### 2. Tool-availability warnings in `vibe sync`

`vibe sync` checks `exec.LookPath` for the external binaries the resolved
standard's modules require, and writes one non-blocking warning line to
**stderr** per missing binary. It never changes sync's exit code.

Modules declare their requirements through an *optional* interface in
`internal/module`:

```go
// ToolRequirer is implemented by modules whose resources are inert without
// an external binary on PATH.
type ToolRequirer interface {
	RequiredTools() []Tool
}

type Tool struct {
	Name string // binary name as passed to exec.LookPath
	Why  string // what stops working without it
}
```

Type-asserted at the call site; a module that does not implement it requires
nothing. This is deliberately not a `Module` method: forcing four existing
modules to return `nil` to satisfy one caller is the cost the optional
interface avoids, and `vibe doctor` is a second consumer later.

Declarations in M2:

| Module | Required tools |
|---|---|
| `go-tooling` | `golangci-lint` |
| `repo-tooling` | `task`, `lefthook` |
| `github-ci` | none — its resources run on GitHub's runners |
| `agent-config` | none |
| `ts-tooling` | `node` |
| `python-tooling` | none |

### 3. TypeScript and Python support (lint/format/typecheck only)

Two new modules, each mirroring `go-tooling`'s shape — configuration files,
nothing else:

| Module | Package | Resources |
|---|---|---|
| `ts-tooling` | `internal/module/tstooling` | `eslint.config.js`, `.prettierrc.json`, `tsconfig.base.json` |
| `python-tooling` | `internal/module/pythontooling` | `ruff.toml`, `pyrightconfig.json` |

All `resource.Generated`, default mode, templates embedded under
`templates/` per the `repo-tooling`/`agent-config` convention.

Two new standards, registered in `internal/standard`:

| Standard | Modules |
|---|---|
| `production-typescript/v1` | `ts-tooling`, `agent-config` |
| `production-python/v1` | `python-tooling`, `agent-config` |

And two example repositories committed in-tree, one per new standard:

```text
examples/typescript/    vibe.yaml + the synced resources + .vibe/state.yaml
examples/python/        vibe.yaml + the synced resources + .vibe/state.yaml
```

These are the milestone's drift alarm, not documentation samples. See
"Behavior" for what enforces them and "Design notes" for why they are
necessary rather than nice.

`agent-config` is in both because it is language-neutral — hook scripts and
Claude/Codex settings that block destructive shell patterns — and because a
standard composing exactly one module would be a standard in name only.
`go-tooling`, `repo-tooling`, and `github-ci` are in neither: the first is
Go by definition, and the other two are Go by content (a Taskfile of `go`
commands, a workflow on `setup-go`, a `gomod` dependabot config).

No manifest change. `manifest.Manifest.Standard` is already a free-form
string, `standard.Lookup` already keys on `(name, version)`, and
`vibe init production-typescript v1` already works today — it just resolved
to nothing. M2 makes those names resolve.

## Behavior

### `vibe sync` — hook registration

Ordering within a run: tool warnings (increment 2) print first, then the
per-resource apply lines, then the summary, then hook registration.

Hook registration runs only when **all** of the following hold:

1. the resolved plan contains a `Generated` resource at path `lefthook.yml`;
2. `applyPlan` returned no error and `state.Save` succeeded;
3. the run recorded zero conflicts.

Condition 3 is the conservative choice: a run that refused to write part of
the standard has not finished configuring the repository, and registering
hooks against a half-applied repository is worse than leaving it to the
human who now has a conflict to resolve anyway.

Given those, sync runs `lefthook install` with the working directory set to
`--repo-root` and the command's context, and reports:

```
lefthook: git hooks registered
```

on stdout. Failure never fails sync:

- **`lefthook` not on PATH** — skipped. In increment 1, this prints a
  warning to stderr naming what was not done. Increment 2 subsumes that
  message (`repo-tooling` requires `lefthook`, so the missing binary is
  already reported at the top of the run) and demotes this path to a silent
  skip. The transition is deliberate, not drift: one missing binary should
  produce one line of output.
- **non-zero exit** — one stderr warning carrying lefthook's own first line
  of output, then sync exits 0. The most common cause is `--repo-root`
  pointing at a directory that is not a git work tree, which is a normal
  state for a scratch directory and not a sync failure.

`vibe audit` and `vibe diff` do not register hooks: both are read-only, and
`lefthook install` writes to `.git/hooks`.

### `vibe sync` — tool warnings

Before applying anything, sync walks the resolved standard's modules,
type-asserts each to `module.ToolRequirer`, and for each declared tool calls
`exec.LookPath`. Each miss produces one line on stderr:

```
warning: golangci-lint not found on PATH (required by go-tooling: task lint and CI's lint job)
```

Rules:

- **stderr, never stdout.** stdout is sync's report of what it did to the
  repository; a warning about the machine is not that.
- **Never affects the exit code.** A missing tool is not non-conformance:
  the repository's files are correct, and `vibe audit` must keep agreeing.
- Deduplicated by binary name across modules, in module then declaration
  order, so output is deterministic (the same constraint spec 0010 put on
  resource ordering).
- `vibe init`, `vibe audit`, and `vibe diff` do not check tools. `init`
  writes a manifest before any standard is resolved; `audit` is the CI gate
  and must answer exactly one question.

### `ts-tooling` and `python-tooling`

`Resolve` returns the resources above in the fixed order listed, with no
dependency on `module.Context` — the same pure, deterministic shape as every
M1 module. Both packages assert their templates contain no CR bytes, per the
CRLF/`go:embed` defect M1 found.

Template contents are seeded from `D:\fai-am-bot` (a working repository on
modern toolchains), verbatim except where a path glob encodes that
repository's layout rather than a standard's opinion:

**`.prettierrc.json`, `tsconfig.base.json`** — verbatim, no edits.

**`eslint.config.js`** — verbatim except:

- `typedFiles` becomes `['**/*.ts']`. The source is
  `['packages/*/{src,bin,test}/**/*.ts']`, which is a pnpm-workspace layout.
- The `ignores` list drops `packages/inferclient/src/generated/**` and
  `packages/db/migrations/**`, keeping `**/dist/**` and `**/coverage/**`.
  Those two entries name directories that exist in exactly one repository;
  shipping them in a standard would be shipping someone else's file tree.

**`ruff.toml`** — the content of `pyproject.toml`'s `[tool.ruff]`,
`[tool.ruff.lint]`, and `[tool.ruff.lint.per-file-ignores]` sections,
re-rooted for a standalone `ruff.toml` (`[tool.ruff.lint]` → `[lint]`), with
the `"app/routes/*.py" = ["B008"]` entry dropped — it exists because that
repository is FastAPI, and `B008` is only worth ignoring where a framework
evaluates call expressions in parameter defaults. `"tests/**/*.py" =
["S101"]` stays: assertions being the test contract is true everywhere.

**`pyrightconfig.json`** — the content of `[tool.pyright]`, with `include`
dropped. `["app"]` is that repository's package directory; with no
`include`, pyright checks the project directory, which is the right default
for a standard that does not know the layout.

Everything else is fixed in `v1`, as `production/v1` already is: Python
3.12, ES2023, the rule sets as written. A repository needing different
values cannot conform yet.

### The example repositories

`examples/typescript` and `examples/python` each hold a `vibe.yaml`
declaring the new standard, the exact files `vibe sync` writes for it, and
the resulting `.vibe/state.yaml` — committed, as a real adopting repository
would.

They are enforced by a test in `internal/cli` that runs the existing plan
walk against each example directory and requires every resource to decide
`NoChange`. That is `vibe audit`'s conformance rule, reached through Go
rather than through a subprocess, so it runs under `task test` in the
existing CI matrix on both Linux and Windows.

The enforcement deliberately does **not** go into `Taskfile.yml` or
`.github/workflows/ci.yml`. Both are resources of `production/v1`, shipped
to every repository that adopts it; an `--repo-root examples/typescript`
line in either would push a path that exists only here into every consumer's
verification interface. A Go test is the only place this check can live
without leaking VibeConform's own layout into the standard.

Consequence, and it is the same one `AGENTS.md` already states for managed
files: changing a `ts-tooling` or `python-tooling` template means rebuilding,
running `vibe sync --repo-root examples/<lang>`, and committing the result.
Skip it and the test fails, which is the entire point.

## Explicit non-goals

- **Toolchain provisioning.** M2 warns; it does not install. `docs/usage.md`
  states that VibeConform writes configuration and never provisions
  toolchains, and that stays true — increment 2 exists precisely so the
  boundary can be *observable* without being crossed. Reversing this needs
  its own ADR, not a milestone increment.
- **`vibe doctor`.** Still deferred. The tool check in increment 2 is
  incidental to a command that is doing something else, reports only what
  the resolved standard's modules declare, and answers "is this binary on
  PATH" — not versions, not virtualenvs, not whether the toolchain actually
  works. `doctor` is a command whose whole purpose is that question, and it
  will want a richer answer than `LookPath`.
- **Affected-component graph and `vibe check`.** Unchanged from M1.
- **Multiple profiles or components per repository.** A repository declares
  one standard. A polyglot monorepo that wants `production-typescript` for
  `ts/` and `production-python` for `py/` cannot express that, and M2 does
  not add a `profile:`/`components:` field to `vibe.yaml` to let it. Reusing
  the existing free-form `standard` field is the whole point: two new
  standards cost a registry entry, where a manifest schema change costs a
  schema, a migration, and a resolver.
- **Per-language `repo-tooling` and `github-ci` variants.** No Taskfile, CI
  workflow, or dependabot config for TypeScript or Python in M2. This is the
  largest deliberate gap: `production-typescript/v1` configures how a TS
  repository is linted without saying how it is verified or built. Deferred
  to M3 — and the seed repository is the reason it is honest to defer, since
  its CI is GitLab, so there is no vetted GitHub Actions workflow to seed
  from and inventing one would be guessing.
- **`package.json`, `pyproject.toml`, `uv.lock`, `pnpm-workspace.yaml`.**
  VibeConform does not take ownership of a file it can only partly own.
  `pyproject.toml` holds project-owned `[project]` metadata alongside tool
  configuration, and `Generated` is all-or-nothing: there is no
  `StructuredPatch` or `ManagedSection` apply logic, which is exactly why
  increment 3 ships a standalone `ruff.toml` instead. The same reasoning
  keeps `package.json` (and therefore dependency pinning for eslint,
  prettier, and typescript) out of scope.
- **Validating the generated configuration by running the tools it
  configures.** The example repositories prove the bytes are what the module
  resolved; they do not prove that eslint can parse `eslint.config.js`, that
  ruff accepts `ruff.toml`, or — the failure that matters most — that
  type-aware linting still fires after the `typedFiles` generalization. A
  lint run that checks nothing also exits 0. Catching that needs the real
  tools run against real source in CI, which means `node` and `python`
  toolchains, pinned dev dependencies in the examples, and a new CI job.
  Deferred to M3 as its own increment. Until then this is the milestone's
  known blind spot, and the one-time check against `D:\fai-am-bot` is what
  substitutes for it.
- **Installing or running the tools the modules configure.** `sync` runs
  `lefthook install` and nothing else. It does not run `eslint`, `ruff`,
  `task`, or `golangci-lint`, and increment 1 is not a precedent for a
  general post-apply command hook.
- **Auditing hook registration.** `vibe audit` checks file content. It does
  not check whether `.git/hooks` is wired up, for the same reason it does
  not check file modes (ADR 0006): it compares content hashes, and
  `.git/hooks` is outside the resource model entirely.

## Design notes

- **Two testability seams.** Both increments 1 and 2 need `internal/cli`
  tests to run without shelling out — the test suite works in temp
  directories that are not git work trees, and CI machines have no
  `golangci-lint`. Package-level function variables (`var lookPath =
  exec.LookPath`, and one seam for the lefthook invocation) overridden in
  tests are the pragmatic Go idiom here and the smallest change to a
  codebase that otherwise uses plain functions. They are unexported and
  documented as test seams so they do not read as configuration.
- **Why the examples exist at all.** VibeConform is a Go repository and will
  never resolve `ts-tooling` or `python-tooling`, so the five new templates
  have no live counterpart here — and M1's actual safety property was not
  "seeded verbatim" but *the drift alarm lives next to the code and fires on
  every commit*. A one-time manual check against an external repository does
  not have that property: it cannot run in CI, it runs once, and it is
  unreproducible on any machine but one. The examples restore the property
  exactly, and improve on `TestTemplateMatchesLiveFile` while doing it,
  because the committed file *is* the tool's output rather than an assertion
  about a copy of it.
- **The examples are the first non-self `--repo-root` in the test suite.**
  Every existing test either uses a `t.TempDir()` or this repository itself.
  Syncing a standard into a committed, tracked directory that is not the
  tool's own root is closer to what an adopting repository does than
  anything M1 exercised.
- **`D:\fai-am-bot` is a seed, not a gate.** It is where the templates came
  from and a useful one-time smoke test — a large real codebase says more
  about whether a rule set is survivable than five fixture files can. It has
  no standing role after that, and nothing in the milestone's exit criteria
  depends on a path that exists on one machine.
- **Hand-review `eslint.config.js` before merging**, the way spec 0012
  required for the agent hook scripts. It is the one new template that is
  executable code rather than data, and the one being edited rather than
  copied. A wrong `typedFiles` glob does not fail any test in this
  milestone: it silently turns type-aware linting off, and an eslint run
  that checks less than it claims is the lint equivalent of a hook that
  fails open.
- **`tsconfig.base.json`, not `tsconfig.json`.** Keeping the seed's name
  preserves a useful ownership split: the standard owns the base, and the
  repository owns a `tsconfig.json` that extends it with its own `include`,
  `outDir`, and paths. That is the closest thing to `ManagedSection` that
  works without any apply-logic change — the extension point is in the file
  format rather than in VibeConform.
- **`customConditions: ["development"]` stays** in `tsconfig.base.json`
  despite being workspace-flavored. It is inert unless a package publishes
  that export condition, so it costs nothing in a single-package repository
  and is correct in a monorepo. Noted rather than edited, so the next reader
  does not have to rediscover the question.
- **Why `python-tooling` declares no required tools.** `ruff` and `pyright`
  are normally project-local — pinned in `[dependency-groups]` and run
  through `uv run` or a virtualenv — so `exec.LookPath("ruff")` fails on a
  perfectly healthy Python repository. A warning that fires on correct
  setups teaches people to ignore warnings, which is worse than no warning.
  `ts-tooling` declares only `node` for the same reason: eslint, prettier,
  and tsc live in `node_modules`, but `node` itself is genuinely a PATH
  binary. Environment-aware checks (is there a venv? what is in it?) are
  what `vibe doctor` is for.
- **The standard naming asymmetry is known.** `production/v1` is the Go
  standard but is not called `production-go/v1`, because renaming it would
  break this repository's own `vibe.yaml` and `.vibe/state.yaml` for
  cosmetic gain. M3's profile/component work is where naming gets settled;
  until then, `production` means Go and the other two say so in their names.

## Follow-on work

- Per-language `repo-tooling` and `github-ci` variants (M3), which is what
  makes `production-typescript/v1` a complete standard rather than a lint
  configuration.
- Running eslint, ruff, and pyright against the example repositories in CI
  (M3): dev dependencies pinned in each example, fixture sources carrying a
  deliberate violation each, and a job on `setup-node`/`setup-python`. This
  is the increment that closes the blind spot above, and it pairs naturally
  with the per-language CI variants since both put a non-Go toolchain into
  this repository's pipeline for the first time.
- `vibe doctor`: versions, virtualenvs, and whether the toolchain runs —
  the question increment 2 deliberately does not answer.
- `StructuredPatch` ownership, which would let `python-tooling` own
  `[tool.ruff]` inside a repository's existing `pyproject.toml` instead of
  adding a second file, and `ts-tooling` own a `package.json` scripts block.
  This is now the second concrete demand for it, after spec 0012's
  `.claude/settings.json`.
- Renaming `production/v1` to `production-go/v1`, together with whatever
  M3's manifest work does about repositories declaring more than one
  standard.
- A general post-apply mechanism, if a third module ever needs one.
  Increment 1 is a special case on purpose; two special cases would still be
  cheaper than the interface, three would not.
