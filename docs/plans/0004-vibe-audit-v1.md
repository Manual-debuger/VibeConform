# Plan 0004: `vibe audit` (v1)

See `docs/specs/0004-vibe-audit-v1.md` for the accepted scope.

## Checklist

- [ ] `internal/cli/audit.go`: `newAuditCmd` + `runAudit(cmd, repoRoot)`,
      mirroring `internal/cli/init.go`'s structure and `--repo-root` flag.
- [ ] `runAudit` reads `vibe.yaml` via `manifest.Parse`, looks up the
      standard via `standard.Lookup`, and prints
      `standard: <name>/<version>` and `<N> modules configured, nothing to
      check` to `cmd.OutOrStdout()`.
- [ ] Wire `newAuditCmd()` into `internal/cli/root.go` in place of the
      stub from `internal/cli/commands.go`; remove `newAuditCmd` from
      `commands.go`.
- [ ] `internal/cli/root_test.go`: drop `"audit"` from
      `TestSubcommandsNotYetImplemented`.
- [ ] `internal/cli/audit_test.go`: happy path (manifest + known standard
      resolve, exit 0, output contains standard/version and module count),
      missing `vibe.yaml` (non-zero exit), unknown standard/version
      (non-zero exit).
- [ ] `README.md` status banner and command list updated to reflect `audit`
      no longer being a stub.
- [ ] `docs/usage.md` updated: `audit` moves out of the "not implemented"
      section into its own documented command (behavior, example output,
      `--repo-root` flag, error text for both failure cases).
- [ ] `task verify` clean.

## Explicitly still deferred

Unchanged from `docs/plans/0003-standard-registry.md`: standard/module
resolution beyond `Lookup`, `.vibe/lock.yaml` / `.vibe/state.yaml`,
reconciliation, affected-component graph, `diff`/`sync`/`check`/`doctor` real
implementations, concrete `module.Module` implementations, dynamic/config-
driven standard loading. Additionally, per
`docs/specs/0004-vibe-audit-v1.md`: no `--strict`/severity flags, no
non-zero exit for "0 modules configured."
