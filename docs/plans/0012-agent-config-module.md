# Plan 0012: Agent-config module (`internal/module/agents`)

See `docs/specs/0012-agent-config-module.md` for the accepted scope and
`docs/decisions/0006-resource-file-mode.md` for the file-mode decision it
depends on.

## Checklist

### File mode (ADR 0006)

- [x] `internal/resource/resource.go`: add `Mode os.FileMode` to `Resource`
      with a doc comment stating that the zero value means the
      ownership-appropriate default, and `DefaultMode(Ownership) os.FileMode`
      returning `0o644` for `Generated`. Package doc cites ADR 0006.
- [x] `internal/resource/resource_test.go` (new — the package has no test
      file yet): zero-value `Mode` resolves to `0o644` for `Generated`; an
      explicit mode is returned unchanged.
- [x] `internal/cli/sync.go`: `writeResource` passes the resource's resolved
      mode to `atomicfile.Write` instead of the hardcoded `0o600`.
- [x] `internal/cli/sync_test.go`: on non-Windows, a synced script resource
      is executable and a synced config resource is `0o644`; skip the
      assertion on Windows, which does not model the bits (mirroring
      `atomicfile_test.go`).

### Module

- [x] `internal/module/agents/templates/`: byte-for-byte copies of
      `.claude/settings.json`, `.claude/hooks/block-dangerous.sh`,
      `.claude/hooks/block-secret-files.sh`, `.codex/config.toml`,
      `.codex/hooks.json`. Copy with a command; verify with
      `git diff --no-index` per file.
- [x] `internal/module/agents/agents.go`: `New()`, `Name()` →
      `"agent-config"`, `//go:embed templates/*`, `Resolve` returning the
      five resources in the spec's documented order, with `Mode: 0o755` on
      the two `.sh` resources and the zero value elsewhere.
- [x] `internal/module/agents/agents_test.go`: `Name()`; five resources with
      expected paths/ownership/order; the two hook scripts declare `0o755`
      and the three configs declare the zero value; content is non-empty;
      determinism across two calls; every path slash-separated.
- [x] `internal/module/agents/templates_test.go`: each embedded template
      equals the live file it was seeded from.
- [x] `internal/module/agents/templates_lf_test.go` (or a case in the above):
      neither hook script contains a CR byte. CRLF would break them under
      `bash`, and nothing else in the pipeline would notice.
- [x] `internal/standard/standard.go`: register `agents.New()` after
      `repotooling.New()`.
- [x] `internal/standard/standard_test.go`: ordered module-name assertion
      extended to four modules.
- [x] `internal/cli/*_test.go`: update assertions that depend on the full
      report or resource count (now eleven resources).
- [x] `docs/usage.md`: `production/v1` now manages agent configuration;
      state plainly that `AGENTS.md`, `CLAUDE.md`, `.claude/settings.local.json`,
      and `.codex/README.md` are never written, and that mode is applied on
      write but not audited.
- [x] `README.md`: status banner — four modules.
- [x] `task verify` clean.

## Manual review step

Before merging, diff each generated resource against the live file it
replaces and read the two hook scripts line by line. These files block
destructive commands for the agents working in this repository; a template
that silently drops a pattern disarms a guardrail, and no test here would
catch it. This step is not optional and not automatable.

## Notes

- This increment changes a core type (`resource.Resource`), which is why it
  carries an ADR. Every existing module keeps working untouched because the
  zero value is meaningful.
- ADR 0006's known hole — mode drift is invisible to `audit` — is accepted
  here, not fixed. Do not quietly add mode to the hash to "fix" it; that
  changes the reconciliation core and belongs in its own spec.

## Explicitly still deferred

Per `docs/specs/0012-agent-config-module.md`: no `AGENTS.md`/`CLAUDE.md`, no
`.claude/settings.local.json`, no `.codex/README.md`, no mode in
reconciliation or state, no hook content validation, no agent installation,
no `StructuredPatch` merge for `.claude/settings.json`. Unchanged from
`docs/plans/0011-repo-tooling-module.md`: no `.gitattributes`/`.gitignore`,
no toolchain provisioning, no `lefthook install`, no `release.yml`, no
parameterization or conditionality, no `--force`/`--dry-run`/`--json`, no
orphan pruning or detection, no `.vibe/lock.yaml`, no affected-component
graph, no `check`/`doctor`, no dynamic standard loading, no `vibe.yaml`
overrides.
