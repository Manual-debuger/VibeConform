# VibeConform User Manual

Status: living document, tracks `vibe`'s actual implemented behavior. If
something below and the CLI's own `--help` output disagree, trust
`--help` and file that as a doc bug.

New to VibeConform? [`adopting.md`](adopting.md) walks through adopting it
in a new repository, start to finish. This manual is the reference.

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
  are `prod-go`, `prod-ts`, `prod-py`, and `prod-mono`, each at `v1`.
  `prod-mono` also needs a `components:` list, which `init` does not
  write; see "What `prod-mono/v1` manages" below.
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
  warning and `doctor`'s job.
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
that matter. Environment-aware checking, with versions and a verdict, is
[`vibe doctor`](#vibe-doctor)'s job.

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
| `removed` | deleted (a deselected integration's, unmodified) | entry dropped | 0 |
| `forgotten` | already gone | entry dropped | 0 |

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
  translation. If you work across Windows and Unix, select the
  [line-ending policy](#line-ending-policy) or pin `* text=auto eol=lf` in
  `.gitattributes` yourself. Otherwise checkouts re-hash differently and
  report drift forever.
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
- `sync` deletes only what a deselected integration left behind (spec
  0026; see "Selecting integrations" below), and only files it recorded
  and nobody changed since. A modified file is kept and reported as a
  conflict; a file already gone is just dropped from state. Resources an
  older standard stopped producing are still not deleted.
- A managed file that Git ignores (and does not track) is an error: `sync`
  does not write it and exits non-zero, `audit` exits 2, and `diff` names
  the `.gitignore` exception to add. Written but never committed, it would
  be missing from every fresh checkout, CI's included. Outside a git work
  tree, or without `git` on `PATH`, the check is skipped with one warning.
- Fails (non-zero exit) if `vibe.yaml` is missing/unreadable, the declared
  `(standard, version)` isn't registered, `.vibe/state.yaml` is malformed, a
  write fails, or any resource conflicts. A write failure stops the run but
  still records the resources that already landed, so re-running is safe.

### `vibe doctor`

Diagnoses whether this machine can run the repository's workflow. Where
`audit` asks "are the files right?", `doctor` asks "can I work here?":

```console
$ vibe doctor
standard: prod-go/v1
PASS        git            git version 2.53.0.windows.1 (C:\Program Files\Git\mingw64\bin\git.exe); repository readable
PASS        manifest       vibe.yaml resolves prod-go/v1 (integrations: claude, codex)
FAIL        golangci-lint  not found on PATH (required by go-tooling: task lint, and CI's lint job)
PASS        task           3.53.1 (C:\Users\me\AppData\Local\Microsoft\WinGet\Links\task.exe); Taskfile.yml loads
PASS        lefthook       2.1.14 (C:\Users\me\AppData\Local\Microsoft\WinGet\Links\lefthook.exe)
PASS        go             go1.27.0 (C:\Program Files\Go\bin\go.exe)
PASS        goimports      C:\Users\me\go\bin\goimports.exe
PASS        govulncheck    C:\Users\me\go\bin\govulncheck.exe
PASS        actionlint     v1.7.12 (C:\Users\me\go\bin\actionlint.exe)
PASS        claude hooks   .claude/settings.json present; on PATH: task (C:\Users\me\AppData\Local\Microsoft\WinGet\Links\task.exe), go (C:\Program Files\Go\bin\go.exe)
PASS        codex hooks    suspended (spec 0024); nothing to check
WARN        line endings   core.autocrlf=true and Taskfile.yml has no eol attribute: managed files check out as CRLF
PASS        runtime        windows/amd64, native
PASS        worktree       main checkout, not a linked worktree
summary: 12 pass, 1 warn, 1 fail, 0 unverified
Error: doctor: 1 required check failed
```

| Status | Meaning |
|---|---|
| `PASS` | Checked, and healthy. |
| `WARN` | The workflow runs, but something diverges from what CI sees, or an optional capability is missing. |
| `FAIL` | A required local workflow cannot run: `task verify`, the Git hooks, or the selected agent's hooks. |
| `UNVERIFIED` | `vibe` can't probe this reliably, and says so rather than guessing. |

The checks, in order:

- **git:** it runs, and `--repo-root` is a repository.
- **manifest:** `vibe.yaml` and its standard resolve.
- **One line per required tool:** the same list `sync` warns about, with
  each tool's version where it has a version flag, and the path it was
  found at. In WSL the path tells a Linux binary (`/usr/bin/task`) from a
  Windows one reached through interop (`/mnt/c/...`). `task`'s line also
  says whether `Taskfile.yml` loads. A Taskfile that doesn't load turns
  off every task, [the agent guard](#the-agent-guard) included.
- **Agent hooks:** for each selected agent, the file that registers its
  hooks is present, and the binaries the hooks start are on `PATH`
  (`task`, and `go`, `node`, or `uv` for the guard), with where each was
  found.
- **Graphify**, only when selected: whether the graph exists and was
  built from HEAD, and whether git ignores it (see [Graphify](#graphify)).
  These lines, like the `graphify` tool line, are at most `WARN`: the
  graph is optional.
- **Line endings:** how git will check out a managed file, from
  `core.autocrlf` and the `eol` attribute. With the
  [line-ending policy](#line-ending-policy) selected, the line reports
  its health instead:
  - `FAIL` when a rule outside the root `.gitattributes` overrides it;
  - `WARN` when tracked files are still stored with CRLF;
  - `PASS` otherwise.

  This is a report only, and doctor never edits `.gitattributes` or git
  config.
- **Runtime:** the platform, and `wsl` or `container` when a marker file
  says so.
- **Worktree:** a linked worktree is `UNVERIFIED`. It shares Git hooks
  with the main checkout, and `vibe` doesn't probe for collisions.

When a check can't run because an earlier one failed (no git, no valid
`vibe.yaml`), it is `UNVERIFIED`, with the reason.

**Exit codes:** `1` if any check is `FAIL`, otherwise `0`, whatever the
`WARN` and `UNVERIFIED` lines say. Doctor never returns `2` or `3`, which
stay the conformance verdicts of `audit` and `sync`.

**Behavior to know:**

- Read-only: installs nothing, writes no file, changes no configuration.
  There is no `--fix`.
- No network access, no builds or tests. It runs `git`,
  `task --list-all`, and each tool's version flag, each with a timeout.
- It doesn't check conformance. A doctor that failed on drift would give
  a second answer to `audit`'s question.
- It doesn't report your shell. Neither the OS nor environment variables
  say reliably which shell an agent's command tool uses.
- CI is still the authority. A clean `doctor` on Windows says the
  workflow runs here, not that Linux CI will pass.

### `vibe check`

Not implemented. It returns an explicit error rather than silently doing
nothing or exiting 0:

```
Error: check: not implemented yet (see docs/architecture/overview.md)
```

Don't script against it expecting real output. It is scaffolding for
affected-component validation (see `docs/architecture/overview.md`).

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
| `claude-config` | `.claude/settings.json`, `.claude/hooks/policy.json` |
| `codex-config` | `.codex/config.toml`, `.codex/hooks.json` (no hooks: suspended, see "Codex: hooks suspended") |

The last two are default-on integrations, and `repo-tooling`'s guard and
`hook:*` tasks follow `claude`; the `vscode` and `zed` integrations add
editor files when selected. See "Selecting integrations".

Deliberately **not** managed, and left for you to maintain by hand:

- `.github/workflows/release.yml` and `.goreleaser.yaml` — releasing
  binaries is a repository policy choice, not a baseline guardrail, and a
  `v1` standard is all-or-nothing (no optional resources yet).
- `AGENTS.md` and `CLAUDE.md` — prose written by a human for a specific
  repository. Generating them whole would produce exactly the fabricated,
  ignored-by-everyone instruction file this project argues against. Both
  stay yours, except for the one short section each that a selected
  [development workflow](#development-workflow) adds (ADR 0015).
- `.claude/settings.local.json` — user-local, possibly
  machine-specific. Never written. With `claude` selected, a managed
  `claude` section of `.gitignore` ignores it (spec 0039).
- `.gitignore` — genuinely project-specific.
- `.gitattributes` — yours, by default. It governs how git materializes
  every file, the ones VibeConform writes and hashes included. Opting in
  to the [line-ending policy](#line-ending-policy) adds one managed
  section to it, and the rest of the file stays yours.
- `Taskfile.local.yml` — your own tasks, deliberately outside the managed
  set. See "Adding your own tasks" below.
- `lefthook.local.yml` — your own Git hooks, outside the managed set in
  the same way. See "Adding your own Git hooks" below.

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
exactly as it does for `prod-go`. `claude-config` gives a TypeScript or
Python repository the same `.claude/` guardrails a Go repository gets;
only the guard's language differs (see "The agent guard" below).
`codex-config` writes the same `.codex/` files as for `prod-go`, with
hooks suspended.

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
  `verify`, `verify:fast`, `verify-ci`; `audit` is defined in the included
  `Taskfile.vibe.yml`), plus Go's language-specific `test:race`,
  `mod:verify`, `security`, and `workflows:lint`. They also define the
  hidden `hook:*` tasks the agents call (see "Agent hooks" below).

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

`examples/typescript/` and `examples/python/` (and `examples/monorepo/`,
for `prod-mono/v1`) in this repository are real repositories declaring
these standards, holding the exact output of syncing
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

## What `prod-mono/v1` manages

A standard for a polyglot monorepo: Go, TypeScript, and Python projects
side by side in one repository (spec 0025). `vibe.yaml` names each
project, a *component*, with an `id`, a `path`, and a `profile`
(`go`, `ts`, or `py`):

```yaml
standard: prod-mono
version: v1
components:
  - id: api
    path: services/api
    profile: go
  - id: web
    path: apps/web
    profile: ts
  - id: worker
    path: services/worker
    profile: py
```

`vibe init prod-mono v1` writes the first two lines; add `components:`
yourself. Until you do, `diff`/`sync`/`audit` fail with
`prod-mono/v1 needs at least one component`. The rules
(`docs/decisions/0012-manifest-components.md`):

- `id` is lowercase letters, digits, and `-`, starting with a letter,
  and unique. It is the component's Task namespace and CI job name, so
  names the root files already use are reserved: `audit`, `fmt`, `gate`,
  `hook`, `lint`, `local`, `test`, `typecheck`, `verify`, `verify-ci`,
  `vibe`, `workflows`.
- `path` is relative, slash-separated, and clean (`apps/web`, not
  `./apps/web/`). It cannot be the root, and no component can be inside
  another.
- `vibe.yaml` is decoded strictly in every standard: an unknown key is an
  error. `components:` on `prod-go`, `prod-ts`, or `prod-py` is an error
  too.

Each component is a complete single-language project at its path — its
own `go.mod`; its own `package.json` (with `packageManager`) and
`pnpm-lock.yaml`; its own `pyproject.toml` and `uv.lock` — laid out
exactly as the matching single-language standard expects a repository
root to be. pnpm and uv workspaces are not supported yet, and neither is
generating `go.work`.

| Module | Resources |
|---|---|
| `mono-tooling` | per component, its profile's config at `<path>/`: `.golangci.yml`; `eslint.config.js`, `.prettierrc.json`, `tsconfig.base.json`; or `ruff.toml`, `pyrightconfig.json` — byte for byte what `prod-go`/`prod-ts`/`prod-py` write at a root, plus the component's [`generated:`](#generated-code-generated) exclusions and its `.prettierignore` section |
| `github-ci-mono` | `.github/workflows/ci.yml`, `.github/dependabot.yml`, `.github/pull_request_template.md` (under `ci.provider: github`, the default) |
| `gitlab-ci-mono` | `.gitlab-ci.yml`, `.gitlab/merge_request_templates/Default.md` (under `ci.provider: gitlab` only) |
| `vibe-conformance` | `.github/workflows/conformance.yml` (or `.gitlab-ci.vibe.yml`, or nothing, per `ci.provider`), `Taskfile.vibe.yml`, as in every standard |
| `mono-repo-tooling` | `Taskfile.yml`, `lefthook.yml`, `.claude/hooks/guard.*`, and `<path>/Taskfile.yml` per component |
| `claude-config`, `codex-config` | as in every standard |

**Tasks.** Every component's `Taskfile.yml` has its language's tasks
with the single-language standard's exact commands (`fmt`, `fmt:check`,
`lint`, `typecheck`, `test`, `verify`, `verify:fast`, and for Go
`test:race`, `mod:verify`, `security`), plus `fmt:changed`, which formats
only the files git reports as changed. Inside a component directory
`task verify` works as in a single-language repository. The root
`Taskfile.yml` includes each one under its `id`:

```bash
task api:test          # go test ./... in services/api
task web:lint          # eslint in apps/web
task verify            # every component's verify, then workflows:lint
task verify:fast       # every component's verify:fast
task fmt               # every component's formatter
```

`Taskfile.local.yml` works exactly as in the other standards. Run
`fmt:changed` from inside the component (`cd apps/web && task
fmt:changed`), as `hook:format` does: through the root include, Task's
built-in `xargs`, which Windows uses, ignores the include's directory.

**Git hooks.** Pre-commit runs `task fmt:check` and `task lint` inside
each component with a staged file of its language, so a commit touching
only `apps/web` checks only `web`. Pre-push runs `task verify:fast` at
the root.

**CI.** One job per component, named by its `id`, sets up only its own
toolchain, installs its dependencies in its directory
(`pnpm install --frozen-lockfile` or `uv sync`), and runs
`task <id>:verify`. Go components also run `task <id>:test:race`, and
build and test on Windows as `prod-go` does. A `workflows` job runs
actionlint. `CI / gate` requires every job, so branch protection needs
that one check however many components there are. Dependabot gets one
entry per component, in its ecosystem and directory, plus
`github-actions`. That is the default; see [CI provider](#ci-provider-ciprovider)
for GitLab, or for no generated CI at all.

**Agent hooks.** Work as described in "Agent hooks" below, per component:
`hook:format` runs each component's `fmt:changed` from inside that
component, `hook:check` runs each component's `typecheck`, `lint`, and
`test`, and `hook:context` lists the components and only the toolchains
they use. The guard runs in the first runtime the repository has, in the
order Go, Node, Python (`guard.go`, `guard.mjs`, or `guard.py`).

**Agent instructions.** With a development workflow selected (see
"Development workflow" below), each component's `AGENTS.md` gets a short
`component` section at the bottom (spec 0032). It names the component
and its profile, links to the root `AGENTS.md` (whose workflow and rules
apply unchanged), and names the component's verification tasks. With
`claude` selected, each component's `CLAUDE.md` also gets the `@AGENTS.md`
import, so Claude Code reads that section when it works there. The rest
of both files is yours. Deselecting the workflow removes the sections.
Removing a component leaves them, like its other generated files. (The
one exception is a [`generated:`](#generated-code-generated) section in
the component's `.prettierignore`, which goes with its declaration.)

`sync`'s missing-tool warnings cover only the profiles declared: a
repository with no Python component is not warned about `uv`.

`examples/monorepo/` is a worked example with one component of each
profile, checked the same way as the other examples.

### CI provider: `ci.provider`

`prod-mono` generates GitHub Actions by default. A repository hosted
elsewhere says so in `vibe.yaml` (spec 0038):

```yaml
ci:
  provider: gitlab   # github (the default) | gitlab | none
```

CI stays part of the standard; only the system that carries it varies.
`prod-go`, `prod-ts` and `prod-py` do not offer the setting yet, and a
`ci:` key there is an error.

| `provider` | CI files | Root `Taskfile.yml` | `actionlint` |
|---|---|---|---|
| `github` (or absent) | `.github/workflows/ci.yml`, `.github/dependabot.yml`, `.github/pull_request_template.md`, `.github/workflows/conformance.yml` | `verify` ends with `workflows:lint` | required |
| `gitlab` | `.gitlab-ci.yml`, `.gitlab/merge_request_templates/Default.md`, `.gitlab-ci.vibe.yml` | no `workflows:lint` | not required |
| `none` | none | no `workflows:lint` | not required |

`Taskfile.vibe.yml`, `task verify`, `task verify-ci` and `task audit`
are generated under every provider. They are what any CI system calls.
Under `none`, your own CI must run `task verify-ci` and `task audit`.

**What `.gitlab-ci.yml` contains.**

- One job per component, named by its `id`, in its toolchain's image:
  `golang:${GO_VERSION}`, `node:${NODE_VERSION}` (pnpm through
  corepack, from `packageManager`), or
  `ghcr.io/astral-sh/uv:python${PYTHON_VERSION}-bookworm`.
- Each job installs the pinned Task, installs its dependencies in its
  directory, and runs `task <id>:verify`. Go jobs also run
  `task <id>:test:race`.
- In the Node and uv images, Task comes from its release archive,
  checked against `TASK_SHA256`. Bumping `TASK_VERSION` means bumping
  both.
- Caches live under `.ci-cache/<id>/` at the repository root, which is
  never a component.
- Go jobs set `GOFLAGS: -count=1`, so a restored `GOCACHE` cannot make
  `go test` report a cached pass instead of running. The GitHub workflow
  sets the same in its `env:`, because `setup-go` caches `GOCACHE` as
  well. Local `task verify:fast` keeps the test cache (issue #64).
- `workflow:rules` runs merge request pipelines, and branch pipelines
  for branches without an open merge request.
- Every generated job declares `stage`, `image`, `needs`, `rules`,
  `allow_failure: false`, `before_script`, `script` and
  `interruptible`, so an included file cannot change them by merging
  into it. These, the declared variables and the cache key and paths
  are the floor: what a required job runs, and whether its result
  counts (spec 0040).
- Each component job's cache policy is `$VIBE_CACHE_POLICY`, which is
  `pull-push` unless you set it.
- There is no gate job: GitLab's "Pipelines must succeed" covers the
  whole pipeline. Turn that setting on yourself; VibeConform does not
  configure GitLab projects.
- `.gitlab-ci.vibe.yml` holds the one `conformance:audit` job, the only
  GitLab file that runs `vibe` (through `task audit`, which pins the
  recorded version). `.gitlab-ci.yml` includes it only when it exists.

**Your own jobs: `.gitlab-ci.local.yml`.** A generated job named `local`
runs `.gitlab-ci.local.yml` as a child pipeline when the file exists. It
uses `strategy: mirror`, so the `local` job's status is the child
pipeline's, and a failing job of yours fails the pipeline. VibeConform
never creates, reads or audits the file. A sketch of a migration check
against Postgres:

```yaml
# .gitlab-ci.local.yml: a separate pipeline, so declare what you need.
migrations:
  image: golang:1.27.0
  services:
    - postgres:17
  variables:
    POSTGRES_PASSWORD: test
    DATABASE_URL: postgres://postgres:test@postgres:5432/postgres?sslmode=disable
  script:
    - go run ./services/api/cmd/migrate up
```

A child pipeline can add jobs but cannot redefine the generated ones,
which is the same property `Taskfile.local.yml` has. The cost is that
its jobs cannot `needs:` a generated job or share its artifacts, and
they appear as a downstream pipeline. A child pipeline sees
`CI_PIPELINE_SOURCE=parent_pipeline` and the parent's
`CI_MERGE_REQUEST_*` variables.

**Where and how the generated jobs run: `.gitlab-ci.defaults.yml`.**
Runner tags, retries, services and the like are yours to set (spec
0040, ADR 0021). Put them under `default:` in `.gitlab-ci.defaults.yml`,
which `.gitlab-ci.yml` includes when it exists:

```yaml
# .gitlab-ci.defaults.yml: default: only.
default:
  tags: [linux, docker]
  retry: 1
```

- GitLab applies `default:` only to keywords a job leaves out. Every
  generated job declares its floor, so a default can set `tags`,
  `retry`, `services`, `artifacts`, `after_script`, `hooks` or
  `id_tokens`, but not what the job runs or whether it may fail.
- The defaults reach every component job and `conformance:audit`. They
  do not reach the `local` job (it declares `inherit: default: false`),
  or the child pipeline, which sets its own in `.gitlab-ci.local.yml`.
- VibeConform never creates or writes the file. `vibe audit`, `diff`
  and `sync` check its shape: `default:` must be its only top-level key.
  A job, `variables`, `include` or anything else there could merge into
  a generated job, so each one is reported as a conflict and audit
  exits 2. Nothing under `default:` is checked.

**Cache policy on merge requests.** Every job uploads its cache when it
ends, even when nothing changed. To have merge request pipelines only
read caches, set the project CI/CD variable `VIBE_MR_CACHE_POLICY` to
`pull` (Settings > CI/CD > Variables). Other pipelines keep
`pull-push`. To change the policy for every pipeline, set
`VIBE_CACHE_POLICY` instead.

It is off by default because of GitLab's "Use separate caches for
protected branches", which is on by default. A pipeline started by a
Developer on an unprotected branch uses `-non_protected` cache keys,
and only merge request pipelines write those. With `pull`, such
pipelines never get a cache. A pipeline started by a Maintainer or
Owner uses `-protected` keys and reads the default branch's cache. Turn
`pull` on when your merge request pipelines are mostly started by
Maintainers, or when that setting is off.

**What this does not guard.** Project CI/CD variables override anything
in these files and can change how tools behave (for example
`PYTEST_ADDOPTS`). `default: retry` can turn a flaky failure into a
pass. The floor stops accidental drift, not a determined maintainer.

**Moving an existing GitLab repository over.**

1. Move your own jobs into `.gitlab-ci.local.yml`.
2. Delete your hand-written `.gitlab-ci.yml`. If it is still there,
   `sync` reports it as a conflict and leaves it alone.
3. Set `ci: {provider: gitlab}` in `vibe.yaml`.
4. Run `vibe diff`, then `vibe sync`, then `task audit`.

**Switching provider.** The old provider's files are removed if you left
them unmodified, forgotten if you already deleted them, and kept as a
conflict if you edited them, as for any deselected option
([Deselecting](#deselecting)). Switching back is the same in reverse.

**Known gaps under `gitlab`.**

- No Windows job. `github` tests Go on Windows; GitLab's hosted Windows
  runners are beta, and self-managed instances may have none.
- No dependency-update bot (no Dependabot equivalent).
- No offline lint of the GitLab configuration in `task verify`; GitLab
  rejects invalid configuration when it creates the pipeline.
- Images are pinned by version tag, not digest.
- Requires GitLab 18.2 or newer, for `trigger:strategy: mirror`.

## Generated code: `generated:`

Code a generator writes from a schema (API types, database models) is
committed, but its style is the generator's, not yours. Declare it, and
the managed formatter and linter configuration skip it (spec 0037):

```yaml
# prod-mono: per component, relative to the component's path
components:
  - id: worker
    path: services/worker
    profile: py
    generated:
      - src/worker/contracts/**
  - id: web
    path: apps/web
    profile: ts
    generated:
      - src/contracts/**
      - src/api/client.gen.ts
```

```yaml
# prod-ts and prod-py: top level, relative to the repository root
standard: prod-py
version: v1
generated:
  - src/example/contracts/**
```

**What is skipped.** Checks whose findings you would fix by editing the
file are skipped. Checks that judge whether the program is correct are
not, because a generated file is fixed by changing the generator or the
schema, and a type error in it is a real defect in a contract everything
imports.

| Profile | Format | Lint | Typecheck |
|---|---|---|---|
| py | skipped: `ruff.toml` `extend-exclude`, with `force-exclude = true` | skipped: the same keys | still checked (`pyright`) |
| ts | skipped: a managed `generated` section in `.prettierignore` | skipped: `eslint.config.js` global `ignores` | still checked (`tsc`) |
| go | not offered: `generated:` is an error | skipped by the `// Code generated ... DO NOT EDIT.` header (golangci-lint's default) | still checked (`go build`, `go vet`) |

The exclusion is ordinary tool configuration, so it holds wherever the
tool runs: `task fmt`, `fmt:check`, `lint` and `verify`, the agents'
`hook:format` and `hook:check`, lefthook (where files are passed
explicitly, hence `force-exclude`, and `--no-warn-ignored` on `prod-ts`'s
ESLint pre-commit command), and your editor. When every file ruff is
given is excluded, it prints `warning: No Python files found under the
given path(s)` and exits 0.

**Go uses its own convention.** Go marks a generated file with a first
comment line matching `^// Code generated .* DO NOT EDIT\.$`, and
golangci-lint skips such files by default. `gofmt` still checks them:
it has one style and no configuration, and Go generators emit it
through `go/format`. So `generated:` on `prod-go` or on a `profile: go`
component is an error that points to the header. Verified with
golangci-lint v2.13.2.

**The pattern grammar** is narrow on purpose, so that a typo cannot
exclude a whole component, and so that a pattern means the same thing in
ruff, ESLint and Prettier:

- relative and clean, with forward slashes: no leading `/`, `./`, `..`,
  `//`, trailing `/`, `:` or whitespace;
- at least two segments, and the first is a literal directory name:
  `src/gen/**`, not `**/gen` or `gen.py`;
- each segment is exactly `**`, or uses only `A-Z a-z 0-9 . _ -`. A
  single `*` is not accepted: ruff lets it match across `/` and the
  other tools do not. Use `dir/**` for a directory's contents, and name
  a single file in full (`src/api/client.gen.ts`);
- no duplicates in a list.

`**` matches zero or more directories in the middle (`src/**/gen/x.ts`
matches `src/gen/x.ts` and `src/a/b/gen/x.ts`) and everything below at
the end. A pattern that matches nothing excludes nothing; `vibe` never
looks at the files.

**Removing a declaration.** Edit the list and `vibe sync` re-renders the
files. When a list becomes empty or is removed, or its component is
removed, the `.prettierignore` section is removed if you left it
unmodified (and the file with it, if VibeConform created it and nothing
else is in it), and kept as a conflict if you edited it. A stale ignore
list never outlives its declaration.

**Keeping generated code current** is the repository's job, not the
standard's: VibeConform never runs generators. A `Taskfile.local.yml`
task that regenerates and fails on a diff does it:

```yaml
version: "3"

tasks:
  contracts:check:
    desc: Fail if committed generated contracts are out of date.
    cmds:
      - task: contracts:generate
      - git diff --exit-code -- services/worker/src/worker/contracts apps/web/src/contracts
```

`examples/monorepo/` declares a generated file in `web` and in `worker`,
each deliberately unformatted and with a lint finding. CI checks that
`task verify` passes there, and that the same content at a path that is
not declared still fails.

## Selecting integrations

A standard is two things: a **core** that every repository gets —
language tooling, `Taskfile.yml`, lefthook, CI, the conformance check —
and a catalog of optional **integrations** around it: editors, coding
agents, and code-intelligence providers. `vibe.yaml`'s `integrations:`
map picks from the catalog, one list per category (spec 0026,
`docs/decisions/0013-optional-integrations.md`). The core cannot be
deselected.

| Category | Name | Default | What it manages |
|---|---|---|---|
| `editors` | `vscode` | off | four tasks in `.vscode/tasks.json`; per-language entries in `.vscode/extensions.json` |
| `editors` | `zed` | off | four tasks in `.zed/tasks.json` |
| `agents` | `claude` | on | `.claude/settings.json`, `.claude/hooks/policy.json`, the guard program, and the `hook:*` tasks in `Taskfile.yml` |
| `agents` | `codex` | on | `.codex/config.toml`, `.codex/hooks.json` |
| `intelligence` | `graphify` | off | a `graphify` section in `.gitignore`; `graph:update` in `Taskfile.yml` and the `post-commit`/`post-checkout` jobs in `lefthook.yml`; with `claude`, `.claude/skills/graphify/SKILL.md` and a `hook:context` line; with a workflow, a paragraph in AGENTS.md (see [Graphify](#graphify)) |

A category you leave out takes its defaults; an empty list means none.
The categories are independent: choosing editors never changes agents.

**1. Defaults.** No `integrations:` key at all — exactly what every
standard generated before integrations existed:

```yaml
standard: prod-go
version: v1
```

**2. Claude only.** `codex` is deselected; `vibe diff` shows
`.codex/config.toml: would remove (codex deselected)` and the same for
`.codex/hooks.json`, and `vibe sync` deletes both if unmodified:

```yaml
standard: prod-go
version: v1
integrations:
  agents: [claude]
```

**3. Terminal only, no agent configuration.** No `.claude/`, no
`.codex/`, and no `hook:*` tasks. `task verify`, lefthook, and CI are the
same as ever. (`examples/python` is terminal-only too, with `editors: []`,
but keeps the default agents, since this repository's CI runs the agent
hooks there.)

```yaml
standard: prod-py
version: v1
integrations:
  editors: []
  agents: []
```

**4. One editor.** Agents stay at their defaults (`examples/typescript`):

```yaml
standard: prod-ts
version: v1
integrations:
  editors: [vscode]
```

**5. Two editors in a monorepo.** Listing order does not matter: output
always follows the catalog (vscode, then zed). Recommendations cover
every declared language (`examples/monorepo`):

```yaml
standard: prod-mono
version: v1
components:
  - { id: api, path: services/api, profile: go }
  - { id: web, path: apps/web, profile: ts }
integrations:
  editors: [zed, vscode]
  agents: [claude]
```

**6. Invalid.** Each of these is an error before anything resolves, exit
1:

```yaml
integrations:
  editors: [vscode, vscode]   # duplicate within a category
  agents: [cursor]            # unknown: the error lists claude, codex
  intelligence: [gitnexus]    # unknown: the error lists graphify
  editor: [zed]               # not a category: vibe.yaml is decoded strictly
```

### Editor files are shared

`.vscode/tasks.json`, `.vscode/extensions.json`, and `.zed/tasks.json`
belong to your team as much as to VibeConform, so VibeConform owns only
its own *entries* in them: the tasks labelled `task fmt`, `task lint`,
`task test`, and `task verify`, and the recommended extensions for your
languages. Everything else — your own tasks and recommendations, other
keys, comments, indentation, trailing commas — is left exactly as you
wrote it, and adding more of your own is not drift.

- Each owned entry is reconciled like a generated file: a missing one is
  appended, an edited one is drift that `sync` restores in place, and one
  the standard changed is updated.
- A task of yours that already uses one of those labels is a conflict:
  `sync` leaves the whole file alone until you rename or remove it.
- Deselecting an editor removes only its unmodified entries. The file is
  deleted only if VibeConform created it and nothing else is in it.
- Commit these files. A `.gitignore` that ignores `.vscode/` wholesale
  makes `sync` refuse them; the common convention is to ignore
  `.vscode/*` and add exceptions for `tasks.json`, `extensions.json`,
  `settings.json`, and `launch.json`.
- In `prod-ts`, Prettier leaves the selected editors' owned files alone:
  `fmt`, `fmt:check`, the pre-commit hook, and `hook:format` all exclude
  exactly those paths. Prettier would otherwise re-lay out the owned
  entries, and no fixed layout suits every Prettier configuration. Only
  layout goes unchecked: `vibe audit` still compares every owned entry
  by content, ignoring whitespace, so an editor reformatting the file on
  save is not drift. Your own `settings.json` and `launch.json` are
  still formatted.

Selecting an editor configures it for people. It does not give a coding
agent access to that editor's language server; that is the LSP
integration's job, not yet built.

### Graphify

[Graphify](https://pypi.org/project/graphifyy/) (PyPI package
`graphifyy`, MIT license) builds a knowledge graph of the repository in
`graphify-out/`: communities, central nodes, and cross-file relationships
that an agent can query with `graphify query`, `graphify path`, and
`graphify explain`. It is off by default (spec 0035):

```yaml
integrations:
  intelligence: [graphify]
```

**Install it**, on Windows or Linux, with either of:

```sh
uv tool install graphifyy
pip install graphifyy        # or: pip install --user graphifyy
graphify --version
```

VibeConform was verified with graphify 0.8.18. It relies only on
`graphify update <path>` and on the top-level `built_at_commit` key of
`graphify-out/graph.json`. `vibe sync` never installs graphify, never
runs it, and never runs upstream's `graphify install`. That installer
merges hooks into `.claude/settings.json` and `.codex/hooks.json`, which
VibeConform owns, so `vibe audit` would report them as drift.

**What selecting it generates:**

- A `graphify` section at the bottom of `.gitignore` that ignores all
  of `graphify-out/` (graph, report, cache, manifest). The graph is
  derived and machine-local, and a committed copy would go stale while
  looking current. The rest of `.gitignore` stays yours. If the file
  does not exist, sync creates it.
- A `graph:update` task that runs `graphify update .`. That is the
  code-only rebuild: no LLM, no network.
- `post-commit` and `post-checkout` jobs in `lefthook.yml` that run
  `task graph:update`, so the graph follows every commit and branch
  switch. The logic is in the task, not in `lefthook.yml`, because
  lefthook on Windows does not keep shell quoting in a `run:` line.
  - Without graphify on PATH, the task prints
    `graphify: not on PATH; graph not updated …` and exits 0.
  - If the update fails, it prints `graphify: update failed;
    graphify-out/ may be stale` and exits 0.
  - Neither job is in `pre-commit` or `pre-push`. `task verify`,
    `verify:fast`, `verify-ci`, and CI never touch graphify.
- With `claude`:
  - `.claude/skills/graphify/SKILL.md` tells the agent to check
    freshness first (`built_at_commit` against `git rev-parse HEAD`), to
    fall back to search, the compiler, and the tests when the graph is
    missing or stale, and never to treat an empty result as proof of
    absence;
  - `hook:context` prints `Graphify: graph present …` or
    `Graphify: no graph yet …` at session start.
- With a development workflow, the AGENTS.md section gains a "Repository
  intelligence" paragraph that every agent, Codex included, reads.

**Diagnostics.**

- `vibe sync` warns when `graphify` is not on PATH, and still succeeds.
- `vibe doctor` adds three lines, all at most `WARN`, so graphify never
  makes doctor exit 1:
  - `graphify`: the tool and its version;
  - `graphify graph`: absent, unreadable, stale (`built at <commit>,
    HEAD <commit>`), or current (with a note when the working tree has
    uncommitted changes, which no graph includes);
  - `graphify ignore`: whether git ignores `graphify-out/graph.json`.

**Troubleshooting.**

- *The graph is not updating.*
  - Run `lefthook install`; `vibe sync` does this when lefthook is on
    PATH.
  - Check that `graphify --version` works in the shell Git hooks use.
  - On Windows, `pip install --user` puts `graphify.exe` in
    `%APPDATA%\Python\Python3XX\Scripts`, which is often not on PATH.
    `uv tool install` puts it in `~/.local/bin`.
- *Doctor says stale.*
  - A commit was made with hooks skipped (`LEFTHOOK=0`, `--no-verify`),
    or graphify refused to overwrite a graph that would shrink. That
    happens after a large deletion, or when a graph was built with LLM
    extraction.
  - Run `task graph:update`. To accept a smaller graph, run
    `graphify update . --force`, or set `GRAPHIFY_FORCE=1`.
- *Doctor says not ignored.* A user rule below the section, such as
  `!graphify-out/…`, re-includes it, or the graph was committed before.
  Run `git rm -r --cached graphify-out`.

**Using graphify's own skill beside VibeConform's** (spec 0039, issue
#56). VibeConform's skill and graphify's do different jobs. VibeConform's
`graphify` skill is a query skill: it checks that the graph is present
and current, and says never to let the graph replace verification.
graphify's own skill builds graphs: LLM extraction of documents and
media, exports, merges. To have both, use this layout instead of
`graphify install` (verified with graphify 0.9.73 in a `prod-mono`
repository):

| What | Where | Owner |
|---|---|---|
| VibeConform's query skill | `.claude/skills/graphify/SKILL.md` | VibeConform (managed) |
| graphify's builder skill, with its `references/` | `.claude/skills/graphify-builder/`, with `name: graphify-builder` in its frontmatter | you |
| Routing `/graphify` to the builder | a line in `.claude/CLAUDE.md`, e.g. "`/graphify` means the graphify-builder skill" | you |
| graphify's `PreToolUse` hook-guard hooks (`graphify hook-guard search` / `read`) | `.claude/settings.local.json`, which Claude Code merges over the managed `settings.json` | each developer |
| Git hooks | lefthook → `task graph:update` (generated) | VibeConform |

- With `claude` selected, `.gitignore` gets a managed `claude` section
  that ignores `.claude/settings.local.json`, since that file is
  per-developer.
- Don't run `graphify hook install`: it writes into `.git/hooks/`, which
  lefthook owns.
- **Don't re-run `graphify install`.** If you did, `vibe audit` reports
  `.claude/skills/graphify/SKILL.md` and `.claude/settings.json` as
  drifted. To recover:
  1. Move graphify's new skill into `.claude/skills/graphify-builder/`
     and rename it in its frontmatter.
  2. Move the hooks it added to `settings.json` into
     `.claude/settings.local.json`.
  3. Run `vibe sync` to restore the two managed files, then
     `vibe audit`.

**Limitations.**

- `graphify update` re-extracts code only. Documents, papers, and images
  need graphify's own LLM pipeline (`/graphify` in an agent), which
  VibeConform does not run.
- `post-checkout` adds the rebuild's time to every branch switch.
- The graph is static analysis. It never replaces the compiler, a
  language server, the linter, the tests, or CI.

**Removal.**

- Delete `graphify` from `intelligence:` and run `vibe sync`. That
  removes the `.gitignore` section (and `.gitignore` itself, if
  VibeConform created it and nothing else is in it) and the skill, and
  takes `graph:update`, the Git hook jobs, and the context lines out of
  their files. A skill you edited is kept and reported as a conflict.
- `graphify-out/` is never deleted, because VibeConform never recorded
  it. Delete it yourself, or run `graphify uninstall --purge`, which also
  removes graphify's skill from every platform it was installed for.
- Anything graphify's own installer put in your home directory is not
  VibeConform's to touch.

### Deselecting

Removing a name from `vibe.yaml` is reviewed like any change: `vibe diff`
lists what would go, and `vibe audit` reports it as out of date (exit 3)
until `vibe sync` runs. `sync` removes a deselected integration's files
and entries only when `.vibe/state.yaml` records them and they are
unchanged; directories they leave empty go too. Anything modified since
is kept and reported as a conflict (exit non-zero): delete it by hand, or
select the integration again. Files VibeConform never recorded are never
touched.

## Line-ending policy

An opt-in repository policy that pins every text file to LF on every
checkout, so a Windows clone and Linux CI hash managed files the same
(spec 0029):

```yaml
standard: prod-go
version: v1
policy:
  line_endings: lf
```

`lf` is the only value, and leaving the key out selects no policy. It is
the same for every standard.

**What it writes.** One managed section, at the top of the root
`.gitattributes`:

```gitattributes
# vibeconform:begin line-endings
# Managed by VibeConform: policy.line_endings in vibe.yaml.
* text=auto eol=lf
# vibeconform:end line-endings

*.png binary
*.bat eol=crlf
```

VibeConform owns only the lines between the markers. Everything below
them is yours, and `sync` never reads it for a decision or rewrites it:

- If there is no `.gitattributes`, the file is created with the section
  alone.
- If the file exists, the section is inserted above your rules, followed
  by one empty line.
- Once the section is there, you may move it, and that is not drift.
- Editing inside the markers is drift, exactly as for a generated file.
- Deleting one marker, or duplicating a section, is a conflict.

**Your exceptions win.** For each attribute, git applies the last line
that matches a path, so a rule below the section overrides the policy
for the paths it names. `*.bat eol=crlf` and `*.png binary` are fine.

What isn't fine is a later rule on **every** path that changes `text`,
`eol` or `crlf`, such as `* eol=crlf`, `* -text`, `* binary` or `* text`.
That disables the policy everywhere, so it is reported as a conflict:

```console
$ vibe audit
.gitattributes (section line-endings): conflict: line 6: "* eol=crlf" overrides policy.line_endings (eol=crlf) for every path; narrow it to the paths that need it, or deselect the policy
```

`audit` exits `2`. `sync` writes nothing and exits non-zero, and neither
one changes your rule. A rule that changes those attributes but sits
*above* the section is overridden by the policy, and is reported as a
warning.

The check reads only the root `.gitattributes`. A nested
`.gitattributes`, `.git/info/attributes` or `core.attributesFile` can
override the policy too. [`vibe doctor`](#vibe-doctor) reports the
effective `eol` git resolves, so it catches those, and `audit` does not.
Macro attributes (`[attr]`) that you apply to `*` are not expanded.

**`core.autocrlf`.** The `eol` attribute takes precedence over
`core.autocrlf` and `core.eol` for every path it covers. A Windows
machine with `core.autocrlf=true` still checks these files out as LF, so
nobody has to change their git configuration, and VibeConform never
does.

**Existing files.** The policy changes how files are checked out and
committed from now on. It doesn't convert what is already committed. If
the repository has CRLF files in its history, run this once after the
first `vibe sync`, and commit the result:

```console
git add --renormalize .
```

`vibe doctor` reports `WARN` until no tracked text file is stored with
CRLF.

**Formatters and editors.** Nothing else needs a newline setting:

- `prod-ts`'s Prettier already sets `endOfLine: lf`.
- gofmt writes LF.
- ruff keeps a file's line endings.
- An editor that saves CRLF is normalized by git when you commit.

`.editorconfig` stays yours, and so do editor settings. Agents don't
need an "always write LF" instruction in `AGENTS.md`, because git
enforces it.

**Deselecting.** Removing `line_endings` from `vibe.yaml` removes the
section, if it is unchanged, together with the empty line inserted with
it. Your rules stay. If VibeConform created `.gitattributes` and nothing
else is in it, the file is deleted. A modified section is kept and
reported as a conflict, as in "Deselecting" above.

## Development workflow

Two opt-in settings, the same for every standard, that give people and
agents one agreed way to work here (spec 0030, ADR 0015):

```yaml
standard: prod-go
version: v1
development:
  workflow: plan-triggered-sdd   # direct | plan-triggered-sdd | always-sdd
  docs_layout: standard
```

Leaving `development:` out selects neither. `workflow` takes one value,
so the three modes can't be combined:

| `workflow` | Normal work | In a planning context |
|---|---|---|
| `direct` | implement, then verify | write a spec when asked |
| `plan-triggered-sdd` | implement, then verify | constraints and assumptions, a lightweight spec with acceptance criteria, then a plan; nothing is implemented before you approve |
| `always-sdd` | a non-trivial behavioural change needs an approved spec first, in any mode | as for `plan-triggered-sdd` |

A *planning context* is the harness's own plan mode (Claude Code's plan
mode, for example), or an explicit request for a spec, such as `/spec`
below. Nothing detects plan mode with a hook: the agent knows when it is
in it.

**What `workflow` writes.** One managed section at the end of
`AGENTS.md`, the `workflow` section between HTML-comment markers. It
routes rather than explains:

- where specs, architecture docs and ADRs live;
- the workflow for each context, and that the spec (WHAT) stays apart
  from the plan (HOW);
- `task verify:fast` while working and `task verify` before declaring
  done;
- a closing ledger, one line per check (PASS, FAIL or UNVERIFIED), so
  "unit-tested", "CI passed" and "checked against a real integration"
  stay separate claims.

Every line follows from what `vibe.yaml` selects. It names the docs
directories only with `docs_layout`, and `/spec` only with the `claude`
integration. The section is held to at most 300 words, so there is room
above it for your own rules. As with `.gitattributes`:

- Everything outside the markers is yours and is never rewritten.
- If there is no `AGENTS.md`, it is created with the section alone.
- Editing inside the markers is drift.

**Claude Code.** Claude Code reads `CLAUDE.md`, not `AGENTS.md`. With
`claude` selected, a workflow also adds:

- the `agents` section at the top of `CLAUDE.md`, holding `@AGENTS.md`,
  Claude Code's documented import. If your `CLAUDE.md` already imports
  `AGENTS.md` outside the section, `sync` warns and you can delete your
  line.
- `.claude/skills/spec/SKILL.md`, a project skill named `spec`, which is
  also the slash command `/spec <feature>` (spec 0031). It reads the
  relevant docs, lists constraints and unverified assumptions, writes or
  reuses a spec from the template, proposes a plan, and stops before
  implementing. It never changes code. Under the two SDD workflows,
  Claude may use it by itself in plan mode, and the `AGENTS.md` section
  tells it to. Under `direct` it is yours alone to invoke
  (`disable-model-invocation: true`).

Codex reads `AGENTS.md` directly and gets no skill.

Before spec 0031 this was a command, `.claude/commands/spec.md`. Once
the skill replaces it, `sync` removes a recorded copy that you have not
changed ("replaced by .claude/skills/spec/SKILL.md"), keeps a changed
one as a conflict for you to delete, and never touches one it did not
write.

**What `docs_layout` writes.** It names three canonical directories,
`docs/specs/`, `docs/architecture/` and `docs/decisions/`, and seeds
only two files, each with one managed section:

- `docs/README.md`: a short "Layout" list, below your own introduction.
- `docs/specs/README.md`: how to name a spec, its Status line (draft,
  accepted, implemented, superseded), and the spec template. It is at
  the top, and your own conventions can follow it.

`architecture/` and `decisions/` get no file; they appear with your
first document there. There are no `active/` or `completed/`
directories: a spec's Status line says where it stands.

**Optional docs directories.** Two more keys, each set to `on`, add a
directory to the layout (spec 0033):

```yaml
development:
  docs_layout: standard
  docs_development: on   # docs/development/: build, test, contribute locally
  docs_operations: on    # docs/operations/: deploy, run, handle incidents
```

Each adds its directory to `docs/README.md`'s list and to the knowledge
rule of the `AGENTS.md` section, and creates no file. They are
independent, so a library can take `docs_development` alone. Each needs
`docs_layout`; without it, `sync` stops with
`development.docs_operations: on requires development.docs_layout: standard, which is not selected`.
Turning one off updates both sections back.

**Adopting an existing layout.** If your documents already live
somewhere else, name that directory instead of moving them (spec 0034):

```yaml
development:
  docs_layout: standard
  decisions_dir: docs/adr   # instead of docs/decisions
  specs_dir: rfcs           # instead of docs/specs
```

The keys are `specs_dir`, `architecture_dir`, `decisions_dir`,
`development_dir` and `operations_dir`. The last two need their
`docs_development` or `docs_operations` key. Nothing is detected or
moved:
- each path must be a clean path relative to the repository root, and
  no two directories may overlap;
- an adopted directory must exist, and `diff`, `audit` and `sync` stop
  with an error naming the key until it does.

The `AGENTS.md` section and `docs/README.md` then name your directories,
and the specs section goes in `<specs_dir>/README.md`. Changing
`specs_dir` later moves that section: the copy at the old path is
removed if you have not changed it, and kept as a conflict if you have.

**Deselecting.** Removing `workflow` removes the `AGENTS.md` section,
`/spec` and the `CLAUDE.md` section. Deselecting `claude` removes the
last two. Removing `docs_layout` removes its two sections. Your text
stays. A file VibeConform created is deleted once nothing else is in it,
and a modified section is kept and reported as a conflict.

## Agent hooks

When the `claude` integration is selected — the default — the standard
configures Claude Code (`.claude/settings.json`) to run five hooks. Each one is a hidden task in the generated `Taskfile.yml`,
called as `task -x hook:<name>`, the same command in every standard and on
every OS. See `docs/specs/0021-agent-hooks-task-interface.md` and
`docs/specs/0023-agent-lifecycle-hooks.md`. Codex hooks are suspended; see
"Codex: hooks suspended" below.

| When | Task | What it does | Blocks? |
|---|---|---|---|
| Session start | `hook:context` | prints a few lines of facts into the agent's context | no |
| Before a shell command or file edit | `hook:guard` | denies destructive commands and secret-file edits | yes |
| After a file edit | `hook:format` | formats the changed files, with cheap fixes | no (tool already ran) |
| After a file edit, in the background | `hook:check` | runs the incremental checks | no |
| When the agent ends its turn | `hook:done` | runs `task verify:fast` as a gate | yes, once per stop |

Nothing on these paths calls `vibe`, so all five keep working after you
remove VibeConform. Every `hook:*` name is reserved like `verify`:
`Taskfile.local.yml` can't redefine it. They have no `desc`, so
`task --list` doesn't show them.

**Exit 2 is the only code that reaches the agent.** On exit 2 Claude Code
shows the model the hook's stderr; any other exit is treated as a
non-blocking error. That's why every command is `task -x …` (without `-x`,
Task reports a failure as 201) and why each hook ends its failing paths in
`exit 2`.

### `verify:fast` and the tool caches

`hook:check` and `hook:done` run the fast, incremental part of `verify`,
which you can also run yourself:

| Standard | `task verify:fast` runs | Only in `task verify` |
|---|---|---|
| `prod-go` | `fmt:check`, `typecheck`, `lint`, `test` | `security`, `mod:verify`, `workflows:lint` |
| `prod-ts` | same as `verify` | none |
| `prod-py` | same as `verify` | none |

"Affected" comes from each tool's own cache, not from anything
VibeConform computes:

- **Go:** `go test ./...` reuses a package's cached result unless the
  package, or anything it imports, changed. So only affected tests rerun,
  including packages that depend on your change. golangci-lint caches its
  analysis. Don't add `-count=1` to `test`: it turns the cache off.
- **TypeScript:** ESLint `--cache`, Prettier `--cache`, and
  `tsc --incremental` keep their caches under `node_modules/.cache/`,
  which a Node repository already ignores. ESLint's cache doesn't track
  links between files, so a type-aware rule can report a stale result
  locally until the file itself changes. CI starts cold and is unaffected.
- **Python:** Ruff caches by default (in `.ruff_cache/`, which ignores
  itself). `pyright` and `pytest` have no changed-only mode and run whole.
- **`pnpm test` is yours.** `prod-ts` doesn't choose a test runner. If you
  use Vitest or Jest, put its changed-only mode in your own `test` script
  (`vitest related`/`--changed`, `jest --findRelatedTests`/`--changedSince`).

`task verify` remains the full gate, and CI runs it.

### `hook:context` (session start)

Plain text on stdout, which Claude Code adds to the model's context:

```console
$ task hook:context
Repo: VibeConform
Platform: windows/amd64
Runtime: native
Shell: unknown (not reported by the harness)
Branch: feature/agent-lifecycle-hooks
State: dirty
Go: go1.27.0
Task: 3.53.1
Dirty files: 2
 M internal/module/agents/claude/claude.go
?? docs/notes.md
Affected components:
 100.0% internal/module/agents/claude/
```

`Platform` comes from Task itself (`{{OS}}/{{ARCH}}`), so it can't fail,
and it is printed even outside git. `Runtime` is `wsl` or `container`
only when a marker file exists (`/proc/sys/fs/binfmt_misc/WSLInterop`,
`/.dockerenv`, `/run/.containerenv`). Otherwise it is `native`, meaning no
marker was found. `Shell` is always `unknown`. The harness doesn't report
which shell the agent's command tool uses, and the OS doesn't settle it,
so the line tells the agent not to assume. A linked worktree adds
`Worktree: linked (main checkout at <path>)`.

`State` also names a rebase, merge, cherry-pick, revert, or bisect in
progress. The runtime lines depend on the standard: `Go` for `prod-go`,
`Node` and `pnpm` for `prod-ts`, `uv` and `Python` for `prod-py`. A missing
tool shows as `missing`. The list stops after 20 files with `… and N more`.

It only looks. It never installs dependencies, builds, runs tests, starts
services, or runs migrations; those stay explicit tasks. It doesn't repeat
`AGENTS.md`, which the agent loads itself, and it always exits 0.

`git status` is fast even on large repositories (about 85 ms on a
100,000-file checkout with Git for Windows' default settings). If yours is
slower, enable git's own speed-ups, `core.fsmonitor` and
`core.untrackedCache`. VibeConform doesn't set them for you.

### `hook:format` (after each edit)

Formats the files git reports as changed: tracked changes plus untracked
files git doesn't ignore, filtered to the extensions `task fmt` covers.
Not the whole repository, and not only the file just edited: your own
uncommitted edits get formatted too.

| Standard | Runs on the changed files |
|---|---|
| `prod-go` | `goimports -w` (also adds and drops imports), then `gofmt -w` |
| `prod-ts` | `prettier --write --cache` |
| `prod-py` | `ruff format`, then `ruff check --fix` (safe fixes only; remaining findings are `hook:check`'s to report) |

`prod-ts` doesn't run `eslint --fix` here: through `pnpm exec` it takes
about two seconds per edit, over the one-second budget spec 0023 sets.
ESLint's findings still arrive through `hook:check`.

It prints nothing when it succeeds. A formatter that fails, on a syntax
error for instance, exits 2 and the agent sees the error. After it runs,
the agent's copy of the file is out of date until it reads the file again.

### `hook:check` (after each edit, in the background)

Runs `typecheck`, `lint`, and `test` (in the standard's `verify` order),
without blocking the agent. It leaves out `fmt:check` because Claude Code
runs an event's hooks in parallel, so it starts while `hook:format` may
still be writing the file.

Claude Code runs it with `asyncRewake`: on failure (exit 2) it wakes the
session straight away and shows the output as a system reminder. A plain
`async` hook would only deliver JSON `additionalContext` or
`systemMessage` output, and only on the next turn.

Quick successive edits start overlapping runs.

### `hook:done` (end of turn)

Runs `task verify:fast`. If it fails, the hook exits 2 and the agent
can't stop: Claude Code keeps working with the failure as its reason.

It blocks **once per stop**. If the agent stops again with the checks
still failing, the payload's `stop_hook_active` is `true`, and the stop
goes through with the failure printed. That keeps a failure the agent
can't fix (a flaky test, a missing tool) from looping forever. Claude Code
also has its own cap of eight blocks. It's a deliberate
soft spot: CI is still the authoritative gate, and `task audit` isn't part
of it at all.

To try it by hand, give it a payload on stdin. It reads stdin, so with
nothing piped it waits:

```console
$ echo '{"stop_hook_active": false}' | task -x hook:done; echo $?
0
```

### Checking the hooks fire

As for the guard, no automated check can see an agent's own hook dispatch.
In the session you mean to rely on:

- **Session start:** ask the agent what branch it's on without letting it
  run anything. It should know from the context.
- **Format:** ask it to write a badly formatted file; it should come back
  formatted.
- **Check:** break a test in an edit; the failure should reach the agent
  without it running the tests.
- **Stop:** leave the test broken and let it finish; the first stop should
  be refused, and the second let through.

**Known limitations** of these four, beyond the guard's (below), which
mostly apply to them too:

- **A Taskfile that won't load turns them off**, and **the nearest
  Taskfile wins**, exactly as for the guard. A subdirectory Taskfile
  without the hook tasks makes each hook a non-blocking error.
- **The Stop gate is only as fast as your cached tests.** Go re-checks
  every file and environment lookup a cached test made before it reuses
  the result, so a suite that looks up many paths is slow even when fully
  cached.
- **Pre-existing failures block the first stop too.** `verify:fast` checks
  the whole repository, not only what the agent changed. The second stop
  goes through.

### The agent guard

With the `claude` integration selected (the default), every standard
configures Claude Code to run a guard before each shell command and each
file edit. It blocks a short list
of destructive commands (`rm -rf`, `git reset --hard`, `git push --force`,
and a few more) and edits to files that conventionally hold secrets
(`.env`, `*.pem`, `id_rsa`, …). See
`docs/specs/0021-agent-hooks-task-interface.md`.

How it fits together:

- `.claude/settings.json` runs the same command in every standard:
  `task -x hook:guard`.
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
  the guard runs. Claude Code treats any exit other than `2` as allow.
  `task verify` breaks at the same moment, so it rarely goes unnoticed for
  long.
- **The nearest Taskfile wins.** Task searches upward from the agent's
  current directory. In a subdirectory with its own `Taskfile.yml`, that
  file's `hook:guard` runs. If it has none, Task exits `200` and the
  command is allowed.
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

### Codex: hooks suspended

Since spec 0024, VibeConform runs no hooks in Codex. `codex-config` still
writes `.codex/config.toml` (approval policy and sandbox mode) and writes
`.codex/hooks.json` with no hooks:

```json
{
  "hooks": {}
}
```

A Codex session in a conformant repository therefore gets no guard, no
session context, no formatting, and no `Stop` gate from VibeConform.
`AGENTS.md` still applies, and CI is still the gate.

Why: a live test of Codex 0.156.1 found that on Windows it ignores exit
code 2, so neither the guard's deny nor the `Stop` gate's block takes
effect, and that on every platform a background hook's stderr never
reaches the model. Hooks written for Claude Code's contract don't carry
over. See `docs/decisions/0011-one-module-per-agent-runtime.md` for the
results and what resuming would take.

After upgrading `vibe`, `vibe audit` reports both `.codex/` files out of
date, and `vibe sync` writes them; the old Codex hooks stop running with
nothing to delete. `.codex/config.toml` no longer sets `hooks = true`, and
deliberately doesn't set `hooks = false` either, so hooks you configure in
your own `~/.codex/` keep running.

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

## Adding your own Git hooks: `lefthook.local.yml`

`lefthook.yml` is fully generated too. Put repository-specific Git hooks
(a `post-merge` dependency install, a `commit-msg` linter, an extra
`pre-commit` check) in a committed `lefthook.local.yml` beside it:

```yaml
post-merge:
  commands:
    deps:
      run: task deps   # a task from your Taskfile.local.yml

pre-commit:
  commands:
    migrations-named:
      glob: "migrations/*.sql"
      run: task migrations:check-names
```

The generated `lefthook.yml` names it under lefthook's own `extends:`
key, so `lefthook install` (which `vibe sync` runs) registers its hooks
and every hook run merges it in. Things to know:

- **It is optional.** A missing `extends` file is skipped silently. With
  no `lefthook.local.yml`, the hooks are exactly the managed ones.
- **It is yours.** `vibe` never creates, writes, reads, validates, or
  audits it, as with `Taskfile.local.yml`.
- **It is merged, per hook, then per command, then per field.** A new
  hook (`post-merge`) is added. A new command in a managed hook
  (`pre-commit.commands.migrations-named`) runs beside the managed ones.
  A command with a managed command's name is merged into it field by
  field, so `run:` can be replaced, and `skip: true` disables it.
- **That override is allowed on purpose.** Unlike Task, lefthook has no
  name-collision error. Git hooks are a fast local first line of
  defence, not the gate: CI runs the full verification, and any
  developer can already skip hooks with `LEFTHOOK=0` or `--no-verify`.
  So a skipped managed command costs speed of feedback, not conformance.
- **Per-developer overrides go in `lefthook-local.yml`** (with a hyphen).
  That is lefthook's own per-machine file. It is loaded last, over
  everything else, and upstream recommends gitignoring it. VibeConform
  does not add it to `.gitignore`: whether to commit it is your call.

Merge order is `lefthook.yml` → `lefthook.local.yml` → `lefthook-local.yml`.
This was verified with lefthook 1.13.6 (loader source), 2.1.14 on
Windows and 2.1.16 on Linux.

**Already edited `lefthook.yml` by hand?** `vibe audit` reports it as
`conflict` once your `vibe` carries the `extends` entry. Move your
additions into `lefthook.local.yml`, restore the managed file with
`git checkout lefthook.yml`, then run `vibe sync`.

**Graphify's hook jobs are not yours to copy.** When the
[Graphify](#graphify) integration is selected, the `post-commit` and
`post-checkout` `graph:update` jobs are generated into `lefthook.yml`.
Don't repeat them in `lefthook.local.yml`.

See `docs/decisions/0017-lefthook-local-extension.md`.

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
./bin/vibe sync --repo-root examples/monorepo
./bin/vibe audit --repo-root .              # expect: conformant
./bin/vibe audit --repo-root examples/typescript
./bin/vibe audit --repo-root examples/python
./bin/vibe audit --repo-root examples/monorepo
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
`Taskfile.local.yml`. This repository selects the line-ending policy, so
its `.gitattributes` is the managed section alone. It also selects
`workflow: always-sdd` and the docs layout: `AGENTS.md` and
`docs/README.md` are its own prose plus one managed section each, and
`CLAUDE.md` is the managed import alone.

## What `vibe.yaml` means today

In `prod-go`, `prod-ts`, and `prod-py` it's exactly two fields, nothing
more:

```yaml
standard: prod-go
version: v1
```

`prod-mono` adds a third, `components:`, a list of `id`, `path`, and
`profile` entries; see "What `prod-mono/v1` manages" above. Every standard
also accepts `integrations:`, which selects editors, agents, and
code-intelligence providers; see "Selecting integrations" above. Every
standard also accepts `policy:`, which opts in to repository policies;
see "Line-ending policy" above. And every standard accepts
`development:`, which opts in to a development workflow and the docs
layout; see "Development workflow" above. `prod-ts` and `prod-py` accept a
top-level `generated:` list, and `prod-mono` components accept one each;
see "Generated code" above. `prod-mono` accepts `ci:`, which selects the
CI system; see "CI provider" above. No other key is accepted: `vibe.yaml` is
decoded strictly, and `components:` on any other standard is an error.

There is no `.vibe/lock.yaml`, no `depends_on` between components (so no
affected-component graph yet), and no overrides (`generated:` describes
the code; it does not relax a check for hand-written files): a repository either
conforms to its standard as written or it does not.
Editing `vibe.yaml` by hand is safe and expected — `init` only exists to
create the first one.

## `.vibe/state.yaml`

Machine-owned bookkeeping, written only by `vibe sync`. Commit it; don't
hand-edit it.

```yaml
schema: 4
vibe_version: v0.3.0
standard: prod-go/v1
resources:
    - path: .gitattributes
      section_id: line-endings
      ownership: managed-section
      sha256: eac2…
    - path: .golangci.yml
      ownership: generated
      sha256: 6bb1…
    - path: .vscode/tasks.json
      ownership: structured-patch
      created: true
      elements:
        tasks/task verify:
            sha256: 0e7d…
```

Since schema 4 (spec 0029), `resources` is a list of records, sorted by
`path` and then `section_id`. Each record names its ownership:

- **`generated`:** a whole file, with one hash.
- **`structured-patch`:** a shared file (see "Editor files are shared").
  It has a hash for each entry VibeConform owns, keyed `<array>/<label>`,
  and records whether VibeConform created the file.
- **`managed-section`:** one marked section of a file you own, identified
  by `path` together with `section_id`. Its hash covers only the lines
  between the markers.

Schema 1, 2 and 3 files, where `resources` is a map keyed by path, still
load. The next `vibe sync` rewrites them as schema 4, and the hashes stay
the same. A `vibe` older than spec 0029 can't read schema 4, and it fails
with `load state` (exit 1) instead of misreading it. `task audit`
installs the recorded `vibe_version` (spec 0022), so CI is unaffected.

The `resources` list is what reconciliation compares against: it records the
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
- **Older binaries read the provenance fields fine.** They ignore fields
  they don't know, so upgrading some machines and not others is not a
  migration. The exception is schema 4's list layout, described above.
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
`lefthook.yml`'s `extends: [lefthook.local.yml]` can stay for the same
reason: lefthook skips a missing `extends` file.

Everything else `vibe sync` wrote — `.golangci.yml`, `eslint`/`prettier`/
`tsconfig`, `ruff`/`pyright` config, the rest of `Taskfile.yml`, `ci.yml`
and its `CI / gate`, and any editor or agent configuration you selected —
is ordinary project configuration at that point, no different from having
written it by hand. That includes the line-ending policy's section of
`.gitattributes`, and the development workflow's sections of `AGENTS.md`,
`CLAUDE.md`, `docs/README.md` and `docs/specs/README.md`. Git and the
agents read them with or without VibeConform, and the markers are plain
comments (HTML comments in Markdown, invisible when rendered) that you
can delete or keep. The `spec` skill stays an ordinary Claude Code
skill. A [`generated:`](#generated-code-generated) section of
`.prettierignore` is an ordinary ignore list. With [Graphify](#graphify) selected, its `.gitignore` section,
skill, `graph:update` task, and Git hook jobs keep working the same
way: they call `graphify` and `task`, never `vibe`.
On GitLab (`ci.provider: gitlab`) the VibeConform-specific files are
`vibe.yaml`, `.vibe/`, `.gitlab-ci.vibe.yml` and `Taskfile.vibe.yml`;
`.gitlab-ci.yml`'s include of `.gitlab-ci.vibe.yml` then matches nothing
and does nothing.
`.github/workflows/examples.yml` in this repository demonstrates the split
for its own TS/PY fixtures: `task verify` runs first, with no `vibe` on
`PATH`; building `vibe` and running `task audit` is a separate, later step.

## Getting help

```bash
vibe --help
vibe init --help
```
