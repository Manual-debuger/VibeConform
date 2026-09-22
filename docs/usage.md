# VibeConform User Manual

Status: living document, tracks `vibe`'s actual implemented behavior. If
something below and the CLI's own `--help` output disagree, trust
`--help` and file that as a doc bug.

## Installing

### `go install` or `go run`, by module path (no clone needed)

The repository is public, so Go can fetch it unauthenticated. `go install`
puts a `vibe` binary on `$GOPATH/bin` (usually `~/go/bin`):

```bash
go install github.com/Manual-debuger/VibeConform/cmd/vibe@latest
```

`go run` does the same fetch without leaving a binary behind:

```bash
go run github.com/Manual-debuger/VibeConform/cmd/vibe@latest --help
```

Pin an exact tag instead of tracking `@latest` — see
`docs/decisions/0007-release-tag-naming.md` for what a tag looks like and
why:

```bash
go run github.com/Manual-debuger/VibeConform/cmd/vibe@v0.1.0-alpha.1 --help
```

This previously required git credentials even with a version suffix,
because the repository was private; it works unauthenticated now.

### Prebuilt binaries

`.github/workflows/release.yml` runs GoReleaser on every `v*` tag push and
attaches per-OS/arch archives (`.tar.gz` for Linux/macOS, `.zip` for
Windows) plus a `checksums.txt` to a GitHub Release. `.goreleaser.yaml` sets
`release.draft: true`, so each run creates that release as a **draft**
first — nothing is downloadable from it until a maintainer opens it on the
Releases page and publishes it by hand.

### From source

Still works, and is the only path if you want to build from a commit that
hasn't been tagged:

```bash
git clone <this repo>
cd VibeConform
go build -o vibe ./cmd/vibe
```

Or without building:

```bash
go run ./cmd/vibe --help
```

## Commands

### `vibe init <standard> <version>`

Writes a `vibe.yaml` file declaring the standard and version a repository
intends to conform to. This is the only command with a real
implementation right now — see `docs/specs/0002-vibe-init.md`.

```bash
vibe init prod-go v1
```

produces:

```yaml
standard: prod-go
version: v1
```

**Flags:**

| Flag          | Default | Meaning                                   |
|---------------|---------|--------------------------------------------|
| `--repo-root` | `.`     | Directory to write `vibe.yaml` into        |

```bash
vibe init prod-go v1 --repo-root ./some/other/repo
```

**Behavior to know:**

- `standard` and `version` are free-form strings today. Nothing checks
  that `standard` refers to a real, known standard — that validation
  doesn't exist yet (see `docs/specs/0003-standard-registry.md`).
- `init` **never overwrites** an existing `vibe.yaml`. There is no
  `--force` flag; if you need to change it, edit or delete the file
  yourself. Running `init` again against an existing `vibe.yaml` fails
  with:
  ```
  Error: init: vibe.yaml already exists
  ```
- `standard` cannot be empty:
  ```
  Error: init: new manifest: "standard" is required
  ```

### `vibe audit`

The conformance gate: checks every resource the declared standard resolves
against the repository's actual files and exits non-zero if any of them is
not what the standard says it should be. Safe to run in CI — see
`docs/specs/0009-vibe-audit-v2.md`.

```bash
vibe audit
```

on a conformant repository:

```
standard: prod-go/v1
.golangci.yml: ok
1 resource checked, 0 drifted, 0 conflicts
conformant
```

and on one that has drifted:

```
standard: prod-go/v1
.golangci.yml: drifted (run vibe sync)
1 resource checked, 1 drifted, 0 conflicts
not conformant
```

**Flags:**

| Flag          | Default | Meaning                          |
|---------------|---------|-----------------------------------|
| `--repo-root` | `.`     | Directory to read `vibe.yaml` and check files under |

**What each line means:**

| Line | Meaning | Conformant? |
|---|---|---|
| `ok` | file matches the standard | yes |
| `missing (run vibe sync)` | the standard resolves it, the file isn't there | no |
| `drifted (run vibe sync)` | file differs from the standard, and `sync` can fix it | no |
| `conflict: manual changes detected` | file and standard both moved; `sync` won't touch it | no |
| `not yet checked (unsupported ownership)` | no command handles this ownership mode yet | not counted |

**Exit codes:**

| Code | Meaning |
|---|---|
| `0` | conformant |
| `2` | audited successfully, repository is **not** conformant |
| `1` | could not answer: `vibe.yaml` missing/unreadable, unregistered `(standard, version)`, malformed `.vibe/state.yaml`, or an I/O failure |

The `1` / `2` split is the point of the command in CI: only a `2` is fixed by
running `vibe sync`. A `1` means the check itself is broken.

```
Error: audit: open vibe.yaml: no such file or directory
Error: audit: standard: no such standard prod-go/v99
```

**Behavior to know:**

- Read-only: never writes `vibe.yaml`, `.vibe/state.yaml`, or any managed
  file. `vibe sync` is the only writer.
- Checks files, not machines. `audit` does not look at `PATH`, so tool
  availability never affects its verdict or its exit code — that is `sync`'s
  warning and, eventually, `doctor`'s job.
- There is no `--fix` and no `--strict`. Strict *is* the behavior; fixing is
  a different command on purpose.
- `audit`, `diff`, and `sync` share one decision engine, so they never
  disagree about a resource — they only differ in whether they judge,
  describe, or apply.

### `vibe diff`

Reads `vibe.yaml`, resolves the declared standard/version, and previews the
reconciliation decision for each `Generated`-ownership resource against
`.vibe/state.yaml` and the repository's actual files — see
`docs/specs/0006-reconcile-diff-v1.md`.

```bash
vibe diff
```

produces one of, depending on repository state:

```
standard: prod-go/v1
.golangci.yml: create (no file on disk)
```

```
standard: prod-go/v1
.golangci.yml: no change
```

```
standard: prod-go/v1
.golangci.yml: would update (drift from last applied state)
```

```
standard: prod-go/v1
.golangci.yml: conflict: manual changes detected, review before sync
```

**Flags:**

| Flag          | Default | Meaning                          |
|---------------|---------|-----------------------------------|
| `--repo-root` | `.`     | Directory to read `vibe.yaml` and target files from |

**Behavior to know:**

- Read-only: never writes `vibe.yaml`, files on disk, or `.vibe/state.yaml`.
  `would update` appears only on a repository `vibe sync` has already
  applied to, since it needs a recorded prior state to compare against.
- `diff` and `sync` share one decision walk (`buildPlan`), so `diff` is
  exactly the preview of what `sync` would do.
- Always exits 0 on a successful run, regardless of the decisions found —
  `diff` is a preview, not a compliance gate (that's `audit`'s and, later,
  a strict `audit`'s job).
- Fails (non-zero exit) only if `vibe.yaml` is missing/unreadable, the
  declared `(standard, version)` isn't registered, or `.vibe/state.yaml`
  exists but is malformed.
- Only `Generated`-ownership resources are diffed; any other ownership
  mode prints `<path>: not yet supported by diff` (none exist in the
  registry today).

### `vibe sync`

Applies what `vibe diff` previews: writes each `Generated`-ownership
resource the declared standard resolves, and records what it wrote in
`.vibe/state.yaml` — see `docs/specs/0008-vibe-sync-v1.md`.

```bash
vibe sync
```

On a repository that has never been synced:

```
standard: prod-go/v1
.golangci.yml: created
1 created, 0 updated, 0 unchanged, 0 conflicts
lefthook: git hooks registered
```

Running it again changes nothing:

```
standard: prod-go/v1
.golangci.yml: unchanged
0 created, 0 updated, 1 unchanged, 0 conflicts
lefthook: git hooks registered
```

The last line appears for any standard that manages `lefthook.yml`, and
`lefthook install` is idempotent — re-registering the same hooks on every
sync is the intended behavior, not a sign the previous run failed.

Before writing anything, `sync` checks that the external tools the
standard's modules need are on `PATH`, and warns on **stderr** about the
ones that are not:

```
warning: golangci-lint not found on PATH (required by go-tooling: task lint, and CI's lint job)
warning: task not found on PATH (required by repo-tooling: every verification entry point Taskfile.yml defines)
```

These never change the exit code, and they are not a conformance finding: a
repository whose files match the standard is conformant on a machine with
nothing installed, and `vibe audit` will agree. What the warnings buy is
that the next command you run fails with a shell error you can already
explain.

Only binaries a correctly configured repository would genuinely have on
`PATH` are checked. Project-local tools — anything run through
`node_modules`, a virtualenv, or `uv run` — are deliberately not, because a
warning that fires on a healthy repository teaches you to ignore the ones
that matter. Environment-aware checking is what `vibe doctor` is for, and it
does not exist yet.

**Flags:**

| Flag          | Default | Meaning                          |
|---------------|---------|-----------------------------------|
| `--repo-root` | `.`     | Directory to read `vibe.yaml` and write resources under |

**What each decision does:**

| Decision | File on disk | `.vibe/state.yaml` | Exit |
|---|---|---|---|
| `created` | written | hash recorded | 0 |
| `updated` | replaced | hash recorded | 0 |
| `unchanged` | untouched | hash recorded | 0 |
| `conflict` | **untouched** | prior entry kept | non-zero |

**Behavior to know:**

- A conflict is a hard stop. `sync` never overwrites a file that has been
  changed by hand in a way it can't account for; there is no `--force` yet.
  Resolve it by reconciling the file yourself, or delete it and re-run.
  Other resources in the same run are still applied — the non-zero exit
  comes at the end.
- `unchanged` still records a hash. That is what makes a repository tracked:
  until a resource is in `.vibe/state.yaml`, reconciliation can only compare
  two ways, and genuine drift is indistinguishable from a conflict.
- Writes are atomic (temp file + rename), so an interrupted run leaves
  either the old file or the new one, never a half-written one.
- Content is written exactly as the module resolved it, with no line-ending
  translation. Pin `* text=auto eol=lf` in `.gitattributes` if you work
  across Windows and Unix, or checkouts will re-hash differently and report
  drift forever.
- There is no `--dry-run`: `vibe diff` is the dry run. Note that `diff`
  previews file changes only — it does not register git hooks, because it
  writes nothing.
- If the standard manages `lefthook.yml`, a clean sync ends by running
  `lefthook install` in `--repo-root`, so the hooks the config describes are
  actually registered. It runs **only** after a run with no conflicts and no
  failures, and it can never cause one: a missing `lefthook` binary, or a
  directory that is not a git work tree, prints a warning to stderr and
  leaves the exit code at 0.

  ```
  warning: lefthook not found on PATH; git hooks were not registered
  ```

  ```
  warning: git hooks were not registered: exit status 128
  │  > git rev-parse --show-toplevel
  │    fatal: not a git repository (or any of the parent directories): .git
  ```

  When `lefthook` runs and fails, its own output is repeated under the
  warning (up to 10 lines) rather than reduced to an exit status, since the
  exit status alone never says what went wrong.
- Resources dropped from a standard are not deleted. `sync` only writes what
  the standard currently resolves; it never removes files.
- Fails (non-zero exit) if `vibe.yaml` is missing/unreadable, the declared
  `(standard, version)` isn't registered, `.vibe/state.yaml` is malformed, a
  write fails, or any resource conflicts. A write failure stops the run but
  still records the resources that already landed, so re-running is safe.

### `vibe check`, `vibe doctor`

Not implemented. Each returns an explicit error rather than silently doing
nothing or exiting 0:

```
Error: check: not implemented yet (see docs/plans/0014-m2-milestone.md)
```

Don't script against these expecting real output — they exist as
scaffolding for commands that will eventually read/reconcile against
`vibe.yaml` (see `docs/architecture/overview.md`).

## What `prod-go/v1` manages

The command examples above are abbreviated — they show one resource so the
output format is readable. `prod-go/v1` actually resolves every resource
its modules compose:

| Module | Resources |
|---|---|
| `go-tooling` | `.golangci.yml` |
| `github-ci` | `.github/workflows/ci.yml`, `.github/dependabot.yml`, `.github/pull_request_template.md` |
| `repo-tooling` | `Taskfile.yml`, `lefthook.yml` |
| `agent-config` | `.claude/settings.json`, `.claude/hooks/block-dangerous.sh`, `.claude/hooks/block-secret-files.sh`, `.codex/config.toml`, `.codex/hooks.json` |

Deliberately **not** managed, and left for you to maintain by hand:

- `.github/workflows/release.yml` and `.goreleaser.yaml` — releasing
  binaries is a repository policy choice, not a baseline guardrail, and a
  `v1` standard is all-or-nothing (no optional resources yet).
- `AGENTS.md` and `CLAUDE.md` — prose written by a human for a specific
  repository. Generating them would produce exactly the fabricated,
  ignored-by-everyone instruction file this project argues against.
- `.claude/settings.local.json` — gitignored, user-local, possibly
  machine-specific. Never written.
- `.gitignore` — genuinely project-specific.
- `.gitattributes` — it governs how git materializes every file, including
  the ones VibeConform writes and hashes. Managing the file that determines
  how your own outputs are compared is a loop worth entering deliberately,
  with its own spec.

**File modes.** Resources are written `0644`, except the agent hook scripts,
which are written `0755` — a hook script that is not executable does not run,
and it fails open. Mode is applied on write but is **not** audited: a
`chmod -x` on a hook script disables a guardrail and `vibe audit` will still
report the repository conformant. See
`docs/decisions/0006-resource-file-mode.md`. On Windows, Unix permission bits
are not modeled at all, so the executable bit comes from your git checkout
rather than from `sync`.

Syncing `lefthook.yml` writes the configuration **and** registers the hooks,
by running `lefthook install` at the end of a clean sync. Writing the config
without registering it would leave a pre-commit gate that is configured and
off — a guardrail that fails silently and fails open.

That is the only command `vibe` runs on your behalf, and the boundary it
sits on is narrower than it looks: VibeConform writes configuration and does
not provision toolchains. A repository that syncs `Taskfile.yml` without
`task` installed gets a file it cannot run, and that is the correct division
of responsibility. `lefthook install` activates a file VibeConform just
wrote; it does not install a tool VibeConform did not. If `lefthook` itself
is missing, `sync` warns and moves on — see `vibe sync` above.

Content is fixed in `v1`: Go version, action pins, and job names come from
the standard, not from your repository. A repository that needs different
values cannot conform to `prod-go/v1` yet.

## What `prod-ts/v1` and `prod-py/v1` manage

Two language standards, added in M2. Declare one the same way:

```bash
vibe init prod-ts v1
vibe sync
```

| Standard | Module | Resources |
|---|---|---|
| `prod-ts/v1` | `ts-tooling` | `eslint.config.js`, `.prettierrc.json`, `tsconfig.base.json` |
| `prod-ts/v1` | `github-ci-ts` | `.github/workflows/ci.yml`, `.github/dependabot.yml`, `.github/pull_request_template.md` |
| `prod-ts/v1` | `ts-repo-tooling` | `Taskfile.yml`, `lefthook.yml` |
| `prod-py/v1` | `python-tooling` | `ruff.toml`, `pyrightconfig.json` |
| `prod-py/v1` | `github-ci-py` | `.github/workflows/ci.yml`, `.github/dependabot.yml`, `.github/pull_request_template.md` |
| `prod-py/v1` | `py-repo-tooling` | `Taskfile.yml`, `lefthook.yml` |

Both also compose `agent-config`, which is language-neutral, so a TypeScript
or Python repository gets the same `.claude/` and `.codex/` guardrails a Go
one does.

**As of M3 these are complete standards, not lint/format/typecheck only.**
Through M2 they composed neither `repo-tooling` nor `github-ci` — a
repository declaring `prod-ts` got correct eslint/prettier/tsc configuration
but no `Taskfile.yml`, no `lefthook.yml`, and no CI workflow to run any of it
through. `ts-repo-tooling`/`py-repo-tooling` and `github-ci-ts`/`github-ci-py`
close that gap:

- `ts-repo-tooling`'s `Taskfile.yml` shells out to `eslint`/`prettier`/`tsc`
  via `npx` (project-local, not assumed on `PATH`); `py-repo-tooling`'s shells
  out to `ruff`/`pyright`/`pytest` via `uv run`. Both expose the same target
  names `prod-go/v1`'s Taskfile does (`fmt`, `fmt:check`, `lint`, `typecheck`,
  `test`, `audit`, `verify`, `verify-ci`), minus `build`/`run` — those build
  the `vibe` binary this repository ships, which doesn't generalize to an
  adopting repository.
- `github-ci-ts`/`github-ci-py` run on `actions/setup-node` /
  `astral-sh/setup-uv` instead of `actions/setup-go` (Go is still installed
  in-workflow to install `task` and `vibe` themselves), calling those
  Taskfile targets. Each also gets its own `dependabot.yml`: the Go one
  hardcodes `package-ecosystem: gomod`, so it isn't reusable as-is —
  `github-ci-ts` declares `npm`, `github-ci-py` declares `pip`.

What's still true from M2:

- **No `package.json` or `pyproject.toml` content beyond dev tooling.**
  VibeConform owns whole files, and both of those also hold project-owned
  metadata (dependencies, project name, build config). Nothing pins eslint,
  prettier, typescript, ruff, or pyright to a version on your behalf — you
  install and pin them yourself, the way `examples/typescript/package.json`
  and `examples/python/pyproject.toml` do. That is also why `ruff.toml` and
  `pyrightconfig.json` are standalone files rather than `[tool.*]` sections.
- **`tsconfig.base.json`, not `tsconfig.json`.** The standard owns the
  compiler options; your repository owns a `tsconfig.json` that extends them:

  ```json
  { "extends": "./tsconfig.base.json", "include": ["src"] }
  ```

Content is fixed in `v1` exactly as it is for `prod-go/v1`: ES2023,
Python 3.12, and the rule sets as written.

### Worked examples

`examples/typescript/` and `examples/python/` in this repository are real
repositories declaring these standards, holding the exact output of syncing
them, plus real source (`src/`, `tests/`) and pinned dev dependencies
(`package.json`/`package-lock.json`, `pyproject.toml`/`uv.lock`) that
VibeConform itself does not manage. They are the same worked example that
VibeConform itself is for `prod-go/v1` — and, since this is a Go repository
that never resolves either language module, they are also what keeps those
templates honest: a test fails if a template changes and the examples are
not re-synced.

`TestExamplesAreConformant` (`internal/cli/examples_test.go`) only checks
that the generated files are byte-for-byte what the module resolved — it
does not run eslint, ruff, or pyright. That gap is closed by
`.github/workflows/examples.yml`, a hand-authored workflow (not a module
resource, since encoding an `examples/`-specific job into `github-ci-ts`/
`github-ci-py`'s template would leak this repository's layout into every
adopting repository): it builds `vibe` from source and runs `task verify`
inside each example, so a template change that breaks linting fails CI, not
just a byte-comparison test.

## VibeConform manages itself

This repository is the worked example: it has a `vibe.yaml` declaring
`prod-go/v1`, a committed `.vibe/state.yaml`, and a CI job that runs
`vibe audit` against itself. Every file in the table above is generated from
a module template rather than hand-maintained.

The practical consequence, and the main cost of the arrangement: changing a
managed file means changing its template under `internal/module/`, rebuilding
(`go:embed` resolves at build time), running `vibe sync`, and committing both
the file and the updated state. Editing the file directly makes the
repository non-conformant, and `task audit` fails.

Still hand-maintained here, by the non-goals above:
`.github/workflows/release.yml`, `.goreleaser.yaml`, `.gitignore`,
`.gitattributes`, `AGENTS.md`, `CLAUDE.md`.

## What `vibe.yaml` means today

Right now it's exactly two fields, nothing more:

```yaml
standard: prod-go
version: v1
```

There is no `.vibe/lock.yaml` and no component graph yet — and no overrides:
a repository either conforms to `prod-go/v1` as written or it does not.
`.vibe/state.yaml` exists once you run `vibe sync`; it is machine-owned
bookkeeping — commit it, but don't hand-edit it. Editing `vibe.yaml` by hand is safe and expected —
`init` only exists to create the first one.

## Removing VibeConform

Nothing generated depends on VibeConform staying installed to keep
working: `task verify`/`task verify-ci` depend only on native language
tooling (spec 0017), never on a `vibe` binary. The VibeConform-specific
pieces are concentrated and removable on their own:

- Delete `.vibe/` and `vibe.yaml`.
- Remove the `conformance` job from `.github/workflows/ci.yml`, and drop
  `conformance` from `gate`'s `needs` list.
- `task audit` in `Taskfile.yml` becomes inert once `vibe.yaml` is gone —
  it has nothing left to check against. Delete it, or leave it as dead
  code; either is safe, since nothing else in `Taskfile.yml` depends on it.

Everything else `vibe sync` wrote — `.golangci.yml`, `eslint`/`prettier`/
`tsconfig`, `ruff`/`pyright` config, the rest of `Taskfile.yml`, the
language CI jobs — is ordinary project configuration at that point, no
different from having written it by hand.
`.github/workflows/examples.yml` in this repository demonstrates the split
for its own TS/PY fixtures: `task verify` runs first, with no `vibe` on
`PATH`; building `vibe` and running `task audit` is a separate, later step.

## Getting help

```bash
vibe --help
vibe init --help
```
