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
intends to conform to — see `docs/specs/0002-vibe-init.md`.

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

- `init` writes `standard` and `version` as given, without checking them
  against the standard registry (`internal/standard`, see
  `docs/specs/0003-standard-registry.md`). `audit`, `diff` and `sync` do
  check: against an unregistered pair they fail with
  `standard: no such standard <name>/<version>`. The registered standards
  are `prod-go`, `prod-ts` and `prod-py`, each at `v1`.
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
1 resource checked, 0 drifted, 0 out of date, 0 conflicts
conformant
```

on one where a managed file was edited:

```
standard: prod-go/v1
.golangci.yml: drifted (edited since last sync; run vibe sync to restore)
1 resource checked, 1 drifted, 0 out of date, 0 conflicts
not conformant
```

and on one nobody touched, where the standard moved on instead:

```
standard: prod-go/v1
.golangci.yml: out of date (standard moved; run vibe sync to update)
1 resource checked, 0 drifted, 1 out of date, 0 conflicts
not conformant
last synced by vibe v0.2.0; this vibe is v0.3.0
```

Those last two are different situations and, since spec 0019, say so.
Before it, both printed `drifted`, so upgrading `vibe` told everyone who
had changed nothing that files they never opened had drifted. `sync` fixes
either one; only the first is anyone's mistake.

**Flags:**

| Flag          | Default | Meaning                          |
|---------------|---------|-----------------------------------|
| `--repo-root` | `.`     | Directory to read `vibe.yaml` and check files under |

**What each line means:**

| Line | Meaning | Conformant? |
|---|---|---|
| `ok` | file matches the standard | yes |
| `missing (run vibe sync)` | the standard resolves it, the file isn't there | no |
| `drifted (edited since last sync; run vibe sync to restore)` | the file was changed after VibeConform wrote it | no |
| `out of date (standard moved; run vibe sync to update)` | the file is exactly as VibeConform last wrote it; the standard has since changed | no |
| `conflict: manual changes detected` | file and standard both moved; `sync` won't touch it | no |
| `not yet checked (unsupported ownership)` | no command handles this ownership mode yet | not counted |

**Exit codes:**

| Code | Meaning |
|---|---|
| `0` | conformant |
| `3` | conformant except **out of date**: nothing was edited, the standard moved |
| `2` | audited successfully, repository is **not** conformant — something was edited, is missing, or conflicts |
| `1` | could not answer: `vibe.yaml` missing/unreadable, unregistered `(standard, version)`, malformed `.vibe/state.yaml`, an I/O failure, or a `vibe` older than the one that last synced this repository |

The split is the point of the command in CI. A `2` or a `3` is fixed by
running `vibe sync`; a `1` means the check itself could not run.

`3` is separate from `2` so a `conformance` job can decide for itself
whether being behind the standard should fail the build. It is non-zero by
default, because the repository *is* behind — the generated workflow treats
any non-zero exit as a failure, and a repository that wants to tolerate
being out of date has to say so deliberately. Mixed findings report `2`:
anything worse than out-of-date outranks it.

```
Error: audit: open vibe.yaml: no such file or directory
Error: audit: standard: no such standard prod-go/v99
```

**When `vibe` itself is out of date**, `audit` declines to judge rather
than reporting drift:

```
11 resources checked, 0 drifted, 0 out of date, 0 conflicts
no verdict: this vibe is v0.1.0, older than the v0.3.0 that last synced this
repository. Its templates predate this repository's state, so the
findings above cannot be trusted and vibe sync would revert managed
files. Upgrade vibe and audit again.
```

This is exit `1`, not `2` or `3`: the repository may be in perfect shape,
and it is the install that is behind. Following a `drifted` message here
would have been actively destructive — `vibe sync` with an older binary
rewrites managed files from its own older templates and records the result
as correct. `sync` refuses outright for the same reason; see below.

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
.golangci.yml: would update (file edited since last applied state)
```

```
standard: prod-go/v1
.golangci.yml: would update (standard moved since last applied state)
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

One more stderr warning, about the binary rather than the machine. A `vibe`
built locally reports `dev` or a `+dirty` version, and that is what `sync`
records as `vibe_version`. `task audit` cannot fetch such a version when
`vibe` is not installed (see "`task audit`, with or without `vibe`" below),
so CI's conformance check would fail:

```
warning: vibe dev is not a released or pseudo-version, so task audit
cannot pin it; CI's conformance check will fail unless vibe is on PATH.
Sync with a released vibe (go install github.com/Manual-debuger/VibeConform/cmd/vibe@<tag>) before pushing.
```

It is skipped in a repository that contains `cmd/vibe`. That means
VibeConform itself, whose conformance job builds `vibe` from source instead
of pinning it (`docs/decisions/0008-self-hosting-probe-in-shipped-templates.md`).

**Flags:**

| Flag          | Default | Meaning                          |
|---------------|---------|-----------------------------------|
| `--repo-root` | `.`     | Directory to read `vibe.yaml` and write resources under |
| `--allow-downgrade` | `false` | Write even though this `vibe` is older than the one that last synced this repository |

**`sync` refuses to run backwards.** If `.vibe/state.yaml` records that a
newer `vibe` last synced this repository, an older one stops before writing
anything:

```
Error: sync: this vibe (v0.1.0) is older than the one that last synced this
repository (v0.3.0); upgrade vibe and run again: syncing would revert managed
files to older templates; pass --allow-downgrade if that is what you mean
```

Without this, an older binary rewrites every managed file from its own
older templates and then records the result as correct — so the repository
ends up *certified* conformant while carrying reverted content, and the
next `audit` with a current binary disagrees. It is easy to reach by
accident: a `vibe` left in `~/go/bin` by an earlier `go install` wins the
`PATH` lookup that `task audit` uses.

Reverting on purpose is legitimate, which is what `--allow-downgrade` is
for. It has to be typed, not stumbled into.

The check only fires when both versions are comparable. An unrecorded
writer (any `.vibe/state.yaml` written before schema 2) or an unstamped
build never blocks a sync — see "`.vibe/state.yaml`" below.

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
Error: check: not implemented yet (see docs/architecture/overview.md)
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
| `vibe-conformance` | `.github/workflows/conformance.yml`, `Taskfile.vibe.yml` |
| `repo-tooling` | `Taskfile.yml`, `lefthook.yml`, `.claude/hooks/guard.go` |
| `agent-config` | `.claude/settings.json`, `.claude/hooks/policy.json`, `.codex/config.toml`, `.codex/hooks.json` |

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
- `Taskfile.local.yml` — your own tasks, deliberately outside the managed
  set. See "Adding your own tasks" below.

**`task audit`, with or without `vibe`.** All three standards get the same
`audit` task, in `Taskfile.vibe.yml`, which `Taskfile.yml` includes
optionally. It takes the first of these that applies (spec 0022):

1. If `vibe` is on `PATH`, it runs `vibe audit --repo-root .`.
2. Otherwise it reads `vibe_version` from `.vibe/state.yaml`. It installs
   exactly that version with
   `go install github.com/Manual-debuger/VibeConform/cmd/vibe@<version>` into
   `$(go env GOCACHE)/vibeconform/<version>`, and runs it. Later runs reuse
   the binary.
3. Otherwise it says why it could not run and exits 1: `go` is missing,
   there is no state file, or the recorded version is `dev` or `+dirty`,
   which no module proxy serves.

It never exits 0 without `vibe audit` having run. Step 2 pins the result:
CI runs the exact `vibe` that last synced the repository, so a new
VibeConform release cannot change CI's verdict. Moving to a new `vibe` is
an ordinary `vibe sync` with the newer binary, and it shows up as a
reviewable change to `vibe_version`. It uses `go install` rather than
`go run` because `go run` reports every non-zero exit as 1, which would
blur "not conformant" (2) and "could not answer" (1).

The generated `.github/workflows/conformance.yml` runs `task audit` in its
own job, `Conformance / audit`, separate from `ci.yml`'s `CI / gate`. The
job has no step that installs `vibe`; step 2 obtains it. Make both checks
required in branch protection. Nothing else in `Taskfile.yml` depends on
`vibe`: since spec 0017, `task verify` and `task verify-ci` use native
language tooling only, so a contributor with no `vibe` installed can still
run the full verification gate.

**File modes.** Every resource is written `0644`. Mode is applied on write but
is **not** audited (`docs/decisions/0006-resource-file-mode.md`). Since spec
0021 nothing depends on it: the agent guards are run through an interpreter,
never executed directly, so no `chmod` can switch them off. Until then, the
bash hook scripts were written `0755`, and a `chmod -x` silently disabled
them.

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
| `prod-ts/v1` | `ts-repo-tooling` | `Taskfile.yml`, `lefthook.yml`, `.claude/hooks/guard.mjs` |
| `prod-py/v1` | `python-tooling` | `ruff.toml`, `pyrightconfig.json` |
| `prod-py/v1` | `github-ci-py` | `.github/workflows/ci.yml`, `.github/dependabot.yml`, `.github/pull_request_template.md` |
| `prod-py/v1` | `py-repo-tooling` | `Taskfile.yml`, `lefthook.yml`, `.claude/hooks/guard.py` |

Both also compose two language-neutral modules. `vibe-conformance`
provides `.github/workflows/conformance.yml` and `Taskfile.vibe.yml`,
exactly as it does for `prod-go`. `agent-config` gives a TypeScript or
Python repository the same `.claude/` and `.codex/` guardrails a Go
repository gets; only the guard's language differs (see "The agent guard"
below).

**As of M3 these are complete standards, not lint/format/typecheck only.**
Through M2 they composed neither `repo-tooling` nor `github-ci` — a
repository declaring `prod-ts` got correct eslint/prettier/tsc configuration
but no `Taskfile.yml`, no `lefthook.yml`, and no CI workflow to run any of it
through. `ts-repo-tooling`/`py-repo-tooling` and `github-ci-ts`/`github-ci-py`
close that gap:

- `ts-repo-tooling`'s `Taskfile.yml` shells out to `eslint`/`prettier`/`tsc`
  via `pnpm exec` (project-local, not assumed on `PATH`; see "`prod-ts` uses
  pnpm" below); `py-repo-tooling`'s shells
  out to `ruff`/`pyright`/`pytest` via `uv run`. All three expose the same
  target names (`fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `audit`,
  `verify`, `verify-ci`; `audit` is defined in the included
  `Taskfile.vibe.yml`), plus Go's language-specific `test:race`,
  `mod:verify`, `security`, and `workflows:lint`.

  That set is the whole of what a standard asserts. Until spec 0018,
  `prod-go/v1` also shipped `build` and `run`, which built and ran *this*
  repository's CLI — commands no adopter could use and that don't
  generalize in any language, since two Go repositories on the same
  standard may build a CLI, a library, several binaries, or nothing.
  Repository-specific tasks now live in `Taskfile.local.yml` instead (see
  below).
- `github-ci-ts`/`github-ci-py` run on `actions/setup-node` /
  `astral-sh/setup-uv` instead of `actions/setup-go` (Go is still installed
  in-workflow to install `task`), calling those
  Taskfile targets. Each also gets its own `dependabot.yml`: the Go one
  hardcodes `package-ecosystem: gomod`, so it isn't reusable as-is —
  `github-ci-ts` declares `npm` (Dependabot's ecosystem for pnpm too),
  `github-ci-py` declares `pip`.

### `prod-ts` uses pnpm

Since spec 0020, every command `prod-ts/v1` generates runs through
[pnpm](https://pnpm.io/): `pnpm exec prettier`/`eslint`/`tsc` and `pnpm test`
in `Taskfile.yml` and `lefthook.yml`, and `pnpm install --frozen-lockfile` in
CI. `pnpm exec` only runs what the repository installed, so a missing
devDependency fails instead of being fetched, and pnpm's strict
`node_modules` fails an import of anything `package.json` doesn't declare.

What the standard needs from your repository:

- **`pnpm` on `PATH`.** Install it globally: the standalone installer,
  `npm install -g pnpm`, or Corepack on Node versions that still ship it.
  `vibe sync` warns when it is missing.
- **A `packageManager` field in `package.json`**, e.g.
  `"packageManager": "pnpm@10.34.5"`. CI's `pnpm/action-setup` step reads
  the version from it; the standard pins no pnpm version of its own, the
  same way it pins no eslint version. Without the field that step fails.
- **A committed `pnpm-lock.yaml`.** CI installs with `--frozen-lockfile`, so
  a lockfile that is missing or out of date fails the job.

**Keep `packageManager` at pnpm 10 or below.** Dependabot's `npm` ecosystem
supports pnpm v7 to v10. A newer `packageManager` leaves Dependabot unable to
update `pnpm-lock.yaml`, even though everything else works.

**Moving an existing `prod-ts` repository from npm.** After upgrading
`vibe`, `vibe audit` reports `Taskfile.yml`, `lefthook.yml`, and
`.github/workflows/ci.yml` as out of date (exit 3), and `vibe sync` rewrites
them. The rest is yours, because `package.json` and lockfiles are
project-owned:

1. Run `pnpm import` to turn `package-lock.json` into `pnpm-lock.yaml`, then
   delete `package-lock.json`. The resolved versions carry over.
2. Add the `packageManager` field.
3. Run `task verify`. A `Cannot find module` error means code was importing a
   package it never declared, which npm's hoisting allowed; add it to
   `package.json`.

`vibe sync` does not print these steps: it cannot tell whether you have
already done them, and a warning that fires on a repository that already
migrated would teach you to ignore it.

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
(`package.json`/`pnpm-lock.yaml`, `pyproject.toml`/`uv.lock`) that
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
adopting repository): it runs `task verify` inside each example with no
`vibe` on `PATH`, then builds `vibe` from source and runs `task audit` as a
separate step, so a template change that breaks linting fails CI, not just
a byte-comparison test.

## The agent guard

Every standard configures Claude Code and Codex to run a guard before each
shell command and, for Claude Code, each file edit. It blocks a short list
of destructive commands (`rm -rf`, `git reset --hard`, `git push --force`,
and a few more) and edits to files that conventionally hold secrets
(`.env`, `*.pem`, `id_rsa`, …). See
`docs/specs/0021-agent-hooks-task-interface.md`.

How it fits together:

- `.claude/settings.json` and `.codex/hooks.json` run the same command in
  every standard: `task -x hook:guard`.
- `Taskfile.yml` defines `hook:guard`, which runs the guard on the
  standard's own runtime: `go run .claude/hooks/guard.go` for `prod-go`,
  `node .claude/hooks/guard.mjs` for `prod-ts`, and
  `uv run --no-project python .claude/hooks/guard.py` for `prod-py`.
- All three guards read the same rules from `.claude/hooks/policy.json`.

Nothing on that path calls `vibe`, so the guard keeps working after you
remove VibeConform.

**`hook:guard` is reserved.** Like `verify` and `audit`, it is a managed
task, and `Taskfile.local.yml` cannot redefine it. It has no `desc`, so
`task --list` doesn't show it: agents call it, people don't. To try it by
hand:

```console
$ echo '{"tool_name":"Bash","tool_input":{"command":"ls"}}' | task -x hook:guard; echo $?
0
```

**It fails closed once it starts.** A denied command exits `2` with the
reason on stderr. Every `hook:guard` command ends in `|| exit 2`, so any
other failure after Task starts the task is a deny too: an unreadable
`policy.json`, a Go compile error, a missing `node` or `uv`. (`go run`
reports its program's exit `2` as `1`, which is how this was found.) When
every tool call is suddenly blocked with a `guard:` message, fix what it
names; the agent can't do it for you until you do.

**Known limitations**, where the guard does *not* run, or allows what it
shouldn't:

- **A Taskfile that won't load turns the guard off.** A YAML error in
  `Taskfile.yml` or `Taskfile.local.yml`, or a task name in
  `Taskfile.local.yml` that collides with a managed one, stops Task before
  the guard runs. Both agents treat any exit other than `2` as allow.
  `task verify` breaks at the same moment, so it rarely goes unnoticed for
  long.
- **The nearest Taskfile wins.** Task searches upward from the agent's
  current directory. In a subdirectory with its own `Taskfile.yml`, that
  file's `hook:guard` runs. If it has none, Task exits `200` and the
  command is allowed.
- **Codex on Windows** fires no `PreToolUse` hook for shell commands
  ([openai/codex#24453](https://github.com/openai/codex/issues/24453)), so
  Codex has no command guard there.
- **Codex has no file-edit hook**, so there is no Codex secret-file guard on
  any platform.
- **Claude Code on Windows without Git Bash** runs hooks through
  PowerShell. If that shell cannot start, the hook never runs and the
  command is allowed
  ([anthropics/claude-code#90077](https://github.com/anthropics/claude-code/issues/90077)).
- **Matching is textual.** The guard matches the parsed command, not the
  whole tool call, so a dangerous pattern in a Bash call's `description`
  no longer blocks it. But a pattern *inside* the command still matches,
  even inside a quoted argument or a heredoc: `gh issue create --body "…git
  reset --hard…"` is denied.

**Checking it fires.** No automated check can see an agent's own hook
dispatch. The manual canary: ask the agent to run
`git branch -D some-branch-that-does-not-exist`. It should be refused with
the guard's message, prefixed `[task -x hook:guard]`. Don't assume when an
agent picks up a changed configuration: Claude Code switched to the new
guard mid-session while this was being built, while other agents or
versions may only read it at startup. Run the canary in the session you
actually mean to rely on.

**Moving from the bash hooks.** Repositories synced before spec 0021 have
`.claude/hooks/block-dangerous.sh` and `block-secret-files.sh`. After
upgrading `vibe`, `sync` writes the new files, but, as for any resource a
standard stops resolving, it does **not** delete the old ones. Nothing
references them any more, so they do nothing; delete them yourself, and
restart any running agent session.

## Adding your own tasks: `Taskfile.local.yml`

`Taskfile.yml` is fully generated, so adding a task to it directly makes the
repository non-conformant. Put repository-specific tasks — build, run,
deploy, migrations, whatever you actually need — in a `Taskfile.local.yml`
beside it:

```yaml
version: "3"

tasks:
  build:
    desc: Build the service binary.
    cmds:
      - go build -o bin/server ./cmd/server
```

```console
$ task build
task: [build] go build -o bin/server ./cmd/server
```

The generated `Taskfile.yml` includes it automatically. Three things to
know:

- **It is optional.** With no such file the include does nothing — no
  warning, no error. Most repositories never need one.
- **It is yours.** `vibe` never creates, writes, reads, validates, or
  audits it. It will not appear in `vibe audit` output, and `vibe sync`
  will not touch it. Commit it like any other project configuration.
- **It extends the standard; it cannot override it.** Defining a task the
  generated file already defines is a hard error, not a silent override:

  ```console
  $ task verify
  task: Found multiple tasks (verify) included by "local"
  $ echo $?
  203
  ```

  So you can add `build`, but you cannot redefine `verify`, `audit`,
  `lint`, or `test` — which is what stops the seam from being a way around
  the conformance gate.

Requires Task v3.39.0 or newer. Older versions ignore `flatten` and namespace
the tasks instead, so `task build` fails with `Task "build" does not exist`
while `task --list` shows `local:build` — a confusing failure that names the
wrong problem. The `TASK_VERSION` pinned in the generated CI workflow is well
above that floor; only a local install can be too old.

See `docs/decisions/0009-managed-file-local-extension.md` for why the
standard stops at the verification interface.

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

Concretely, for every template change, in one commit:

```bash
# 1. edit the template under internal/module/**/templates/
task build                                  # go build -o bin/vibe ./cmd/vibe
./bin/vibe sync --repo-root .
./bin/vibe sync --repo-root examples/typescript
./bin/vibe sync --repo-root examples/python
./bin/vibe audit --repo-root .              # expect: conformant
./bin/vibe audit --repo-root examples/typescript
./bin/vibe audit --repo-root examples/python
# 2. commit the template, the regenerated files, and .vibe/state.yaml
```

The rebuild is not optional and is the classic way to waste an afternoon:
`go:embed` resolves at build time, so a stale binary syncs the *old*
template and every root still reports conformant.

A related trap, now that `task audit` resolves `vibe` from `PATH` rather
than compiling one: if an older `vibe` is installed in `~/go/bin` from a
previous `go install`, `task audit` picks *that* up rather than the one you
just built. Since spec 0019 this is caught rather than silently believed —
`audit` declines to give a verdict and `sync` refuses to write, both naming
the two versions. Upgrade or remove the stale install; `./bin/vibe audit`
run directly is the quickest way to confirm which binary is answering.

`task build` stamps the binary it produces with a version derived from git,
which is what makes that comparison possible at all. An unstamped build
reports `dev`, and two `dev` binaries are indistinguishable.

Note also that on Windows `go build -o bin/vibe` produces an extensionless
file that `PATH` lookup will not find as `vibe` — hence `./bin/vibe` above
rather than putting `bin/` on `PATH`.

Still hand-maintained here, by the non-goals above:
`.github/workflows/release.yml`, `.goreleaser.yaml`, `.gitignore`,
`.gitattributes`, `AGENTS.md`, `CLAUDE.md`, `Taskfile.local.yml`.

## What `vibe.yaml` means today

Right now it's exactly two fields, nothing more:

```yaml
standard: prod-go
version: v1
```

There is no `.vibe/lock.yaml` and no component graph yet — and no overrides:
a repository either conforms to `prod-go/v1` as written or it does not.
Editing `vibe.yaml` by hand is safe and expected — `init` only exists to
create the first one.

## `.vibe/state.yaml`

Machine-owned bookkeeping, written only by `vibe sync`. Commit it; don't
hand-edit it.

```yaml
schema: 2
vibe_version: v0.3.0
standard: prod-go/v1
resources:
    .golangci.yml:
        sha256: 6bb1…
```

The `resources` map is what reconciliation compares against: it records the
content VibeConform last wrote, so `audit` can tell a file you changed from
a file the standard changed.

The three fields above it are provenance, added in schema 2 (spec 0019).
They record which `vibe` wrote the file, which is the one thing the hashes
cannot express — a hash says the target moved, not whether it moved forward
or backward. That is what lets `sync` refuse to run backwards.

- **A file with none of them is a schema-1 file** and keeps working
  unchanged. Every `.vibe/state.yaml` written before spec 0019 is one.
  Nothing is inferred from their absence: an unrecorded writer is unknown,
  not old, so no direction is claimed and no sync is blocked. The next
  `vibe sync` fills them in.
- **Older binaries read schema 2 fine.** The fields are ignored by anything
  that doesn't know them, so upgrading some machines and not others is not
  a migration.
- **`vibe_version` is only compared when both sides are real versions.**
  An unstamped build reports `dev`, which is not a version and is never
  treated as one. `go install …@latest` and released binaries both report
  a real version.
- **`standard` is recorded but not yet checked.** Nothing currently
  compares it to `vibe.yaml`.
- **There is no timestamp**, deliberately: it would rewrite the file on
  every sync and churn its diff for nothing. `vibe_version` does change
  when the binary does, which is the intended cost of recording provenance
  at all.

## Removing VibeConform

Nothing generated depends on VibeConform staying installed to keep
working: `task verify`/`task verify-ci` depend only on native language
tooling (spec 0017), never on a `vibe` binary. Since spec 0022, every
VibeConform-specific piece is a file of its own, so removing it means
deleting four things and editing none:

- `vibe.yaml`
- `.vibe/`
- `.github/workflows/conformance.yml`. Also drop `Conformance / audit`
  from your branch protection's required checks, or pull requests will
  wait forever for a check that no longer runs.
- `Taskfile.vibe.yml`

`Taskfile.yml`'s `includes:` block then names two optional files that
don't have to exist: `Taskfile.local.yml` (yours, never `vibe`'s) and the
deleted `Taskfile.vibe.yml`. An optional include of a missing file does
nothing, so both can stay. Delete the `vibe` entry whenever you like.

Everything else `vibe sync` wrote — `.golangci.yml`, `eslint`/`prettier`/
`tsconfig`, `ruff`/`pyright` config, the rest of `Taskfile.yml`, `ci.yml`
and its `CI / gate` — is ordinary project configuration at that point, no
different from having written it by hand.
`.github/workflows/examples.yml` in this repository demonstrates the split
for its own TS/PY fixtures: `task verify` runs first, with no `vibe` on
`PATH`; building `vibe` and running `task audit` is a separate, later step.

## Getting help

```bash
vibe --help
vibe init --help
```
