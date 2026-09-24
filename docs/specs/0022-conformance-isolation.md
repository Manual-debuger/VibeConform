# Spec 0022: Isolate, Pin, and Fall Back for the Conformance Check

Status: accepted and implemented.

Closes the remaining items of issue #26 (1, 2, 3, 8). Items 11 and 16 were
closed by #29 and #30.

## Problem

Principle 3 (`docs/architecture/principles.md`) says where `vibe` is
genuinely necessary it must be confined to as few files as possible, each
removable on its own, with a fallback when `vibe` is not installed and a
reproducible result. Today:

1. **Removal means editing, not deleting.** The `conformance` job lives
   inside the generated `.github/workflows/ci.yml`. Removing it takes three
   edits (the job, its entry in `gate`'s `needs`, its
   `needs.conformance.result` line); miss the third and `gate` fails on
   every run. The `audit` task likewise lives inside the generated
   `Taskfile.yml`.
2. **No fallback.** `task audit` runs `vibe audit --repo-root .` and fails
   with `"vibe": executable file not found in $PATH` when nothing is
   installed. That failure is loud, which is correct, but there is no way
   to obtain the right `vibe` without a prior install.
3. **Not reproducible.** Outside this repository, the generated
   `conformance` job runs `go install …/cmd/vibe@latest`. A new release can
   change CI's verdict with no change to the repository.
8. **Incomplete missing-tool warnings.** `vibe sync` warns only about
   binaries a module declares through `ToolRequirer`, and several binaries
   the generated files call are undeclared.

## Design

### 1. A new language-neutral module: `vibe-conformance`

Package `internal/module/conformance`, module name `vibe-conformance`,
composed into all three standards (`prod-go/v1`, `prod-ts/v1`,
`prod-py/v1`) directly after the CI module. It owns exactly the
VibeConform-specific pieces, as two `Generated` resources:

| Path | Contents |
|---|---|
| `.github/workflows/conformance.yml` | Workflow `Conformance`, single job `audit` |
| `Taskfile.vibe.yml` | Single task `audit` |

Both are written in place into the existing `v1` standards, following the
precedent of specs 0016, 0018 and 0020 (pre-alpha, only known adopters are
this repository and `examples/`).

**`conformance.yml`** checks out the repository, sets up Go and Task at the
same pinned `GO_VERSION` / `TASK_VERSION` the CI templates use, runs the
self-hosting probe from ADR 0008 (build `./cmd/vibe` onto `PATH` if that
directory exists), and runs `task audit`. It has **no `vibe` install
step**: pinning is `task audit`'s job (section 3), so CI and a developer
machine obtain `vibe` the same way. The self-hosting probe was previously
only in the Go template; being shared now, it is present for all three
languages, and is inert in any repository without `cmd/vibe`.

The job's check name is `Conformance / audit`. It is a separate required
check from `CI / gate`; adding it to branch protection is a repository
setting and outside what `vibe` writes.

**The three CI templates** (`github-ci`, `github-ci-ts`, `github-ci-py`)
drop the `conformance` job, its `needs` entry, and its result line. `gate`
covers the language jobs only.

**The three repo-tooling Taskfiles** drop the `audit` task and gain a
second optional include next to the existing `local` one:

```yaml
includes:
  vibe:
    taskfile: ./Taskfile.vibe.yml
    optional: true
    flatten: true
```

`task audit` keeps working unprefixed. With `Taskfile.vibe.yml` deleted,
the include does nothing, exactly as `Taskfile.local.yml`'s does today.
Flattening means a collision with a task in `Taskfile.yml` is a Task error
(exit 203), not a silent override — the same property ADR 0009 relies on.

Removing VibeConform becomes deletions only:

- `vibe.yaml`
- `.vibe/`
- `.github/workflows/conformance.yml`
- `Taskfile.vibe.yml`

The leftover `vibe` include is inert and may be deleted at leisure.

### 2 and 3. `task audit`: pinned, with a fallback, never silent

`Taskfile.vibe.yml`'s `audit` task, written for Task's built-in shell only
(no `sed`/`grep`/`awk`, so it behaves identically on Windows and Linux —
principle 2):

1. If `vibe` is on `PATH`, run `vibe audit --repo-root .`. A developer's
   installed `vibe` and the self-hosting CI build both take this path. A
   `vibe` older than the recorded writer is already refused by spec 0019's
   guard, so this path cannot silently regress.
2. Otherwise read `vibe_version` from `.vibe/state.yaml` (line-by-line,
   trailing CR stripped). If it is a **pinnable** version and `go` is on
   `PATH`, `go install github.com/Manual-debuger/VibeConform/cmd/vibe@<vibe_version>`
   into `$(go env GOCACHE)/vibeconform/<vibe_version>` and run that
   binary's `audit --repo-root .`. Not `go run`, which the principle
   names as an example: `go run` reports every non-zero exit as 1, which
   would erase the difference between "not conformant" (2) and "could not
   answer" (1). The per-version directory doubles as a cache, so later
   runs skip the build.
   Pinnable means it starts with `v<digit>` and carries no `+build`
   suffix: a release tag (`v0.2.0-alpha.1`) or a clean pseudo-version.
   `dev`, `+dirty`, and missing values are not.
3. Otherwise print which condition failed (no `vibe`, no `go`, no state
   file, or an unpinnable recorded version) and how to proceed, and exit 1.

Pinning to the recorded writer is what makes the result reproducible: CI
runs the exact binary that last synced the repository, so a new release
cannot change the verdict, and the spec 0019 comparison sees equal
versions. Upgrading `vibe` in a repository is an explicit `vibe sync` with
the newer binary, which rewrites `vibe_version` in a reviewable diff.

Exit codes are `vibe audit`'s own on paths 1 and 2 (0 conformant, 2
non-conformant, 1 could not answer); path 3 exits 1, "could not answer".
There is no path that exits 0 without `vibe audit` having run.

### 3a. `vibe sync` warns when it records an unpinnable version

Path 3 is correct for a repository synced by a `dev` or `+dirty` binary:
that state names no reproducible writer. But finding out only when CI goes
red is late. So `vibe sync` warns on stderr, at the moment it writes such
a version into `.vibe/state.yaml`:

```
warning: vibe <version> is not a released or pseudo-version, so task audit
cannot pin it; CI's conformance check will fail unless vibe is on PATH.
Sync with a released vibe (go install …/cmd/vibe@<tag>) before pushing.
```

A warning only, like the missing-tool warnings: the files are correct and
the exit code does not change. It is skipped when the repository contains
`cmd/vibe`, the same self-hosting test the workflow probe uses (ADR 0008).
There the conformance job builds `vibe` from source and never pins, and a
warning on every maintainer sync would teach people to ignore it.

"Pinnable" is one rule in two places, Go (`vibe sync`) and the Taskfile
shell (`task audit`). A test holds them to the same table of cases.

### 8. Tool declarations

Following `ToolRequirer`'s rule — declare only binaries a correctly
configured repository genuinely has on `PATH`:

| Module | Adds | Used by |
|---|---|---|
| `repo-tooling` | `go` | every build, test, vet, mod task; `hook:guard` |
| `repo-tooling` | `goimports` | `task fmt`, `task fmt:check` |
| `repo-tooling` | `govulncheck` | `task security` |
| `repo-tooling` | `actionlint` | `task workflows:lint` |
| `py-repo-tooling` | `uv` | every Taskfile and lefthook command |

`ts-repo-tooling` gains nothing. The issue's `npm`/`npx` entry predates
spec 0020, which moved every command to `pnpm` and declared it.
`vibe-conformance` declares nothing: `go` matters only on the fallback
path, which explains its own failure.

## Non-goals

- Downloading released binaries. `go install …@<version>` is the one fallback;
  a release-archive fallback can follow once releases publish archives.
- Pinning without Go. A TS or Python repository with no Go toolchain and no
  `vibe` gets the loud failure of path 3.
- Changing branch protection. The new required check is documented, not
  configured.

## Acceptance

- No generated `ci.yml` mentions `conformance` or `vibe`; no generated
  Taskfile other than `Taskfile.vibe.yml` defines `audit` or calls `vibe`.
- No generated file contains `vibe@latest`.
- With no `vibe` on `PATH`, `task audit` exits non-zero with an
  explanation when the state file is missing or records `dev` / `+dirty`,
  and installs and runs that exact version when it records a pinnable version.
- `vibe sync` warns about the five newly declared binaries when absent.
- `vibe sync` by a `dev` / `+dirty` binary warns about the unpinnable
  version, except in a repository containing `cmd/vibe`.
- This repository and both `examples/` are re-synced and pass `task verify`
  and `task audit`; CI reports `CI / gate` and `Conformance / audit` green.
