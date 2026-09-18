# Spec 0002: `vibe init` (v1)

Status: accepted and implemented.

## Problem

M0 shipped `vibe init` as a stub. `internal/manifest` can parse `vibe.yaml`
but nothing produces one, so there is no way to actually declare a
repository's desired standard yet.

## Scope

`vibe init <standard> <version>` writes a `vibe.yaml` to the repository
root (`--repo-root`, default `.`) declaring the standard and version a
repository conforms to:

```yaml
standard: production
version: v1
```

Behavior:

- Refuses to run if `vibe.yaml` already exists at the target path — `init`
  never overwrites existing desired state. There is no `--force` flag;
  editing or removing `vibe.yaml` is an explicit, human action.
- `standard` and `version` are required positional arguments; `standard`
  must be non-empty (enforced by `manifest.New`).
- Prints the path written and the values on success.

## Explicit non-goals (unchanged from `docs/specs/0001-v0-control-plane.md`)

- No standard/module resolution — `init` only writes the manifest fields a
  human provides on the command line, nothing is looked up or resolved.
- No `.vibe/lock.yaml` / `.vibe/state.yaml` — still nothing to read or
  write them.
- No validation that `<standard>` refers to a real, known standard —
  standards are not defined anywhere yet.

## Design notes

- `internal/manifest` gained `New` (constructor with the same validation
  as `Parse`) and `Marshal` (the inverse of `Parse`), keeping all
  `vibe.yaml` (de)serialization inside one package rather than having
  `internal/cli` construct YAML directly.
- `--repo-root` is introduced now (rather than assuming the working
  directory) because `audit`/`diff`/`sync` will need the same flag, and
  `internal/module.Context` already anticipates a `RepoRoot` field.

## Follow-on work

Once `internal/standard` (named, versioned standards) exists, `init` can
optionally resolve and write a standard's generated files immediately
after writing `vibe.yaml`, rather than only recording intent.
