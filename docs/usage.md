# VibeConform User Manual

Status: living document, tracks `vibe`'s actual implemented behavior. If
something below and the CLI's own `--help` output disagree, trust
`--help` and file that as a doc bug.

## Installing

No releases exist yet. Build from source:

```bash
git clone <this repo>
cd VibeConform
go build -o vibe ./cmd/vibe
```

Or run it without building, from inside the repo:

```bash
go run ./cmd/vibe --help
```

`go run github.com/Manual-debuger/VibeConform/cmd/vibe ...` (by module
path, from *outside* the repo) only works with a versioned `@vX.Y.Z`
suffix, since it requires Go to fetch the module — and this is currently a
private repo, so that fetch will fail without git credentials configured
for it. Building or running from a local clone is the supported path today.

## Commands

### `vibe init <standard> <version>`

Writes a `vibe.yaml` file declaring the standard and version a repository
intends to conform to. This is the only command with a real
implementation right now — see `docs/specs/0002-vibe-init.md`.

```bash
vibe init production v1
```

produces:

```yaml
standard: production
version: v1
```

**Flags:**

| Flag          | Default | Meaning                                   |
|---------------|---------|--------------------------------------------|
| `--repo-root` | `.`     | Directory to write `vibe.yaml` into        |

```bash
vibe init production v1 --repo-root ./some/other/repo
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

Reads `vibe.yaml`, resolves the declared standard/version via
`internal/standard.Lookup`, and reports the resolved standard's module
count — see `docs/specs/0004-vibe-audit-v1.md`.

```bash
vibe audit
```

produces:

```
standard: production/v1
1 modules configured, nothing to check
```

**Flags:**

| Flag          | Default | Meaning                          |
|---------------|---------|-----------------------------------|
| `--repo-root` | `.`     | Directory to read `vibe.yaml` from |

**Behavior to know:**

- Read-only: never writes `vibe.yaml`, `.vibe/lock.yaml`, or
  `.vibe/state.yaml` (none of the latter exist yet).
- A module count of `0` (a standard with no registered modules) is
  **not** a failure — exit code is 0 whenever the manifest and standard
  both resolve, regardless of module count. `production`/`v1` currently
  composes one module, `go-tooling` (see
  `docs/specs/0005-gotooling-module.md`).
- Fails (non-zero exit) only if `vibe.yaml` is missing/unreadable, or the
  declared `(standard, version)` isn't registered:
  ```
  Error: audit: open vibe.yaml: no such file or directory
  Error: audit: standard: no such standard production/v99
  ```

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
standard: production/v1
.golangci.yml: create (no file on disk)
```

```
standard: production/v1
.golangci.yml: no change
```

```
standard: production/v1
.golangci.yml: would update (drift from last applied state)
```

```
standard: production/v1
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
standard: production/v1
.golangci.yml: created
1 created, 0 updated, 0 unchanged, 0 conflicts
```

Running it again changes nothing:

```
standard: production/v1
.golangci.yml: unchanged
0 created, 0 updated, 1 unchanged, 0 conflicts
```

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
- There is no `--dry-run`: `vibe diff` is the dry run.
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
Error: check: not implemented yet (see docs/plans/0001-bootstrap.md)
```

Don't script against these expecting real output — they exist as
scaffolding for commands that will eventually read/reconcile against
`vibe.yaml` (see `docs/architecture/overview.md`).

## What `vibe.yaml` means today

Right now it's exactly two fields, nothing more:

```yaml
standard: production
version: v1
```

There is no `.vibe/lock.yaml` and no component graph yet. `.vibe/state.yaml`
exists once you run `vibe sync`; it is machine-owned bookkeeping — commit it,
but don't hand-edit it. Editing `vibe.yaml` by hand is safe and expected —
`init` only exists to create the first one.

## Getting help

```bash
vibe --help
vibe init --help
```
