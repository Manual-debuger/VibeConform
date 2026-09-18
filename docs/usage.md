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
0 modules configured, nothing to check
```

**Flags:**

| Flag          | Default | Meaning                          |
|---------------|---------|-----------------------------------|
| `--repo-root` | `.`     | Directory to read `vibe.yaml` from |

**Behavior to know:**

- Read-only: never writes `vibe.yaml`, `.vibe/lock.yaml`, or
  `.vibe/state.yaml` (none of the latter exist yet).
- "0 modules configured" is **not** a failure — every registered standard
  has zero modules today (see `docs/specs/0003-standard-registry.md`), so
  exit code is 0 whenever the manifest and standard both resolve.
- Fails (non-zero exit) only if `vibe.yaml` is missing/unreadable, or the
  declared `(standard, version)` isn't registered:
  ```
  Error: audit: open vibe.yaml: no such file or directory
  Error: audit: standard: no such standard production/v99
  ```

### `vibe diff`, `vibe sync`, `vibe check`, `vibe doctor`

Not implemented. Each returns an explicit error rather than silently doing
nothing or exiting 0:

```
Error: diff: not implemented yet (see docs/plans/0001-bootstrap.md)
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

There is no resolver, no `.vibe/lock.yaml` / `.vibe/state.yaml`, and no
component graph yet. Editing `vibe.yaml` by hand is safe and expected —
`init` only exists to create the first one.

## Getting help

```bash
vibe --help
vibe init --help
```
