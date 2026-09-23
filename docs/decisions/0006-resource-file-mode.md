# ADR 0006: Resource File Mode

## Status

Accepted (implemented by `docs/specs/0012-agent-config-module.md`).

## Context

`resource.Resource` describes a managed file as a path, an ownership mode,
and content. It says nothing about permissions, so `vibe sync` writes every
resource with a hardcoded `0o600` (spec 0008), chosen to match
`internal/cli/init.go`'s manifest write.

Two problems surface as soon as a module manages anything but a config file:

1. `.claude/hooks/block-dangerous.sh` and `block-secret-files.sh` are
   referenced as commands by `.claude/settings.json` and `.codex/hooks.json`.
   On Unix they must be executable, or the guardrails they implement simply
   do not run — and they fail open, silently, which is the worst possible
   failure mode for a file whose job is blocking destructive commands.
2. `0o600` is wrong as a general default anyway. A synced `.golangci.yml` or
   `Taskfile.yml` is owner-read-only, which is surprising for files meant to
   be read by other tools and other users on the machine.

Git tracks only the executable bit, so a repository that commits a synced
script is not protected by git alone: a fresh `vibe sync` on a Unix machine
must produce an executable file directly.

## Decision

`resource.Resource` gains a `Mode os.FileMode` field.

- The zero value means "use the default for this resource's ownership":
  `0o644` for `Generated`. Existing modules therefore need no change, and
  the default moves from `0o600` to `0o644`.
- A module that needs something else declares it explicitly — `0o755` for
  hook scripts.
- `vibe sync` applies `Mode` when writing. `internal/atomicfile.Write`
  already takes a mode, so the change is confined to how the mode is chosen,
  not how it is applied.
- **Mode is not part of reconciliation.** Decisions compare content hashes
  only; a file whose content matches but whose mode has been changed decides
  `NoChange`, and `vibe audit` calls the repository conformant.

## Consequences

- Mode drift is undetectable. `chmod -x .claude/hooks/block-dangerous.sh`
  disables a guardrail, and `vibe audit` will not notice. This is a real
  hole, accepted on the grounds that fixing it properly means putting mode
  into `.vibe/state.yaml` and into the decision engine — a change to the
  reconciliation core that should not ride along with the module that first
  needs executable files. It is recorded here so the gap is a known one, and
  named as follow-on work in `docs/specs/0012-agent-config-module.md`.
- Windows ignores Unix permission bits beyond read-only, so a sync on
  Windows produces a non-executable script. The repository's committed
  executable bit is what carries it; a Windows-only contributor cannot
  produce the correct mode from `sync` alone.
- The default changing from `0o600` to `0o644` alters spec 0008's behavior
  for already-synced repositories. Since mode does not participate in
  reconciliation, a re-sync will *not* correct the mode of a file that
  already exists and matches — the file must change content, or be deleted
  and recreated, for the new default to apply.
- `Ownership` and `Mode` stay orthogonal: ownership says whether VibeConform
  may write the file, mode says what it writes it as.

## Note, 2026-09-23: no shipped resource depends on mode any more

Spec 0021 replaced the bash hook scripts, the only resources that needed
`0o755`, with guards launched through an interpreter (`go run`, `node`,
`uv run`) by `task -x hook:guard`. A guard run that way ignores its mode
bit, so a `chmod -x` can no longer switch it off. The unaudited-mode gap
above therefore no longer weakens any guardrail VibeConform ships.

The decision itself stands: `Resource.Mode` still exists, is still applied
on write, and still does not take part in reconciliation. Auditing mode
becomes worth its cost again only if a future resource must be executable.
