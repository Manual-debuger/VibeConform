# Plan 0024: Claude first, one module per agent, Codex hooks suspended

See `docs/specs/0024-claude-first-agent-modules.md` for the accepted scope.
Accepted at spec review (2026-09-24), with the proposed answer to both open
questions: `codex-config` stays composed, and the packages nest under
`internal/module/agents/`.

Branch: `feature/claude-first-agent-modules`, off `main`, one pull request.

## Repository impact

| Area | Change |
|---|---|
| `internal/module/agents/` | split into `agents/claude` (package `claude`) and `agents/codex` (package `codex`); nothing Go-level is left in `agents/` itself |
| `internal/module/agents/claude/` | `claude.go` (module `claude-config`, command constants), `policy.go`, `templates/settings.json`, `testdata/guard_corpus.json`, and the tests that cover them: Claude parts of `agents_test.go`, `policy_test.go`, `guard_corpus_test.go` |
| `internal/module/agents/codex/` | `codex.go` (module `codex-config`), `templates/config.toml`, `templates/hooks.json`, `codex_test.go` |
| `internal/module/agents/codex/templates/*` | `hooks.json` → `{"hooks": {}}`; `config.toml` loses `[features] hooks = true`, gains a comment naming spec 0024 |
| `internal/standard/standard.go` | each standard composes `claude.New(), codex.New()` where it composed `agents.New()` |
| `internal/standard/wiring_test.go` | `TestAgentConfigWiring` → per-module: each agent module's own config files, not a fixed path pair |
| `.github/workflows/hook-guard.yml` | `./internal/module/agents` → `./internal/module/agents/claude`; comment path |
| Generated files (root, `examples/typescript`, `examples/python`) | `.codex/config.toml`, `.codex/hooks.json` re-synced, plus `.vibe/state.yaml` |
| New ADR | `docs/decisions/0011-one-module-per-agent-runtime.md` |
| Docs | `docs/usage.md`, `.codex/README.md`, `AGENTS.md`, `README.md`, `docs/architecture/overview.md`, `docs/architecture/principles.md`, spec 0023 correction note, spec 0024 status |

Dependency direction is unchanged: both new packages import
`internal/module` and `internal/resource`, and `internal/standard` imports
them. No new dependency.

Two relative paths move one level deeper and must be fixed, not
discovered by a failing test in CI:

- `agents_test.go:263` — `repoRoot := filepath.Join("..", "..", "..")`
  becomes four levels. It exists in both new packages' template-vs-live
  tests.
- `guard_corpus_test.go:131` and `:149` — `filepath.Join("..", module, …)`
  and `"..", "repotooling"` become `"..", "..", …`.

## Order of work

Four commits, each leaving `task verify` and `task audit` green.

1. **C1: the split, no behaviour change.** `git mv` the files so history
   follows them, then create the two packages. `claude-config` produces
   exactly today's `.claude/settings.json` and `policy.json`;
   `codex-config` produces exactly today's `.codex/` files. `vibe audit`
   stays clean with no re-sync, because resource paths and bytes are
   unchanged, and state is keyed by path. This isolates the refactor from
   the suspension.
2. **C2: suspend Codex hooks.** Change the two Codex templates, add the
   suspension test, rebuild `vibe`, re-sync the root and both examples,
   commit the files with `.vibe/state.yaml`.
3. **C3: docs.** The ADR, then every doc in the impact table.
4. **C4: verification record, spec marked implemented.**

**Dogfooding hazard.** C2 changes this repository's own `.codex/hooks.json`.
That affects Codex sessions only; the Claude Code session doing the work is
unaffected, since `.claude/settings.json` doesn't change in any commit.

## Checklist

### C1: the split

- [ ] `git mv` into `agents/claude/`: `policy.go`, `policy_test.go`,
      `guard_corpus_test.go`, `testdata/guard_corpus.json`,
      `templates/claude/settings.json` → `templates/settings.json`.
- [ ] `git mv` into `agents/codex/`: `templates/codex/config.toml`,
      `templates/codex/hooks.json` → `templates/`.
- [ ] `agents.go` → `claude/claude.go`: package `claude`, `Name()` returns
      `claude-config`, resolves `.claude/settings.json` then `policy.json`.
      Keeps `GuardCommand` and the four lifecycle constants. Package comment
      states Claude Code's hook contract (exit 2 blocks or reaches the
      model; `asyncRewake` wakes on exit 2) and drops every "both agents"
      sentence.
- [ ] New `codex/codex.go`: package `codex`, `Name()` returns
      `codex-config`, resolves `.codex/config.toml` then `.codex/hooks.json`.
- [ ] Split `agents_test.go`: Claude cases (`TestName`, resource order,
      mode bit, determinism, `task -x` commands, events, matcher, templates
      match live files, LF) to `claude/claude_test.go`; the Codex cases to
      `codex/codex_test.go`. `TestCodexHooksMatchDocumentedSchema` and the
      Codex rows of `TestAgentHookEvents` stay as they are in this commit.
- [ ] Fix the relative paths listed under "Repository impact".
- [ ] `standard.go`: three module lists; update the comment that says
      "agent-config".
- [ ] `wiring_test.go`: detect an agent module by the config files it
      resolves (`.claude/settings.json`, `.codex/hooks.json`) rather than by
      the name `agent-config`, and check only the files that module
      produces. Keep the `hook:guard` checks, gated on the Claude module.
- [ ] `hook-guard.yml`: package path in the `go test` step and the header
      comment.
- [ ] `go build ./... && task verify && task audit` (rebuilt binary):
      audit clean with no sync.

### C2: suspend Codex hooks

- [ ] `codex/templates/hooks.json` → `{"hooks": {}}` with a trailing
      newline, in Prettier's style: `examples/typescript` runs Prettier's
      `fmt:check` over it, locally and in `examples.yml`.
- [ ] `codex/templates/config.toml`: drop `[features]` / `hooks = true`;
      add a comment: hooks suspended by spec 0024, `hooks = false` not set on
      purpose, see ADR 0011.
- [ ] `codex_test.go`: replace the hook-shape tests with
      `TestCodexHooksSuspended` — `hooks.json` decodes strictly to an empty
      `hooks` object, and `config.toml` has no `hooks` key under
      `[features]`. Its failure message names spec 0024, so lifting the
      suspension is deliberate.
- [ ] Remove the Codex rows from any remaining table test in `claude`.
- [ ] Rebuild `vibe` (`go:embed` resolves at build time), then `vibe sync`
      at the root, `examples/typescript`, `examples/python`. Expect exactly
      `.codex/config.toml`, `.codex/hooks.json`, and `.vibe/state.yaml` to
      change in each.
- [ ] `task verify && task audit`; `TestExamplesAreConformant` passes.

### C3: docs

- [ ] ADR `0011-one-module-per-agent-runtime.md`: context (specs 0021,
      0023, the canary table from spec 0024), decision (one module per
      runtime, Claude Code supported, Codex hooks suspended by an empty
      managed file), consequences (no shared hook contract; resuming needs
      its own spec and a Windows canary showing exit codes or JSON decisions
      honoured).
- [ ] `docs/usage.md`: module table row (line ~466) becomes two rows;
      "Agent hooks" and "The agent guard" describe Claude Code only; the
      two Codex gap bullets and the #24453 claim become one "Codex hooks are
      suspended" note; the `internal/module/agents/agents.go` sample output
      (lines ~752–755) uses a path that still exists.
- [ ] `.codex/README.md`: rewritten around the suspension, the canary
      results, and the templates' new location.
- [ ] `AGENTS.md`: guardrails name `.claude/settings.json` only; drop the
      Codex gap sentence; policy path becomes
      `internal/module/agents/claude/policy.go`.
- [ ] `README.md`: the `agent-config` paragraph names both modules.
- [ ] `docs/architecture/overview.md`: package list shows `agents/claude`
      and `agents/codex`.
- [ ] `docs/architecture/principles.md`: the `PreToolUse` table row is
      Claude Code's.
- [ ] Spec 0023: a dated correction note under 4.3 about `async` output.
- [ ] `grep -rn "agent-config\|module/agents\b\|\.codex/hooks.json"` over
      `docs/usage.md`, `README.md`, `AGENTS.md`, `docs/architecture/` finds
      nothing stale. Historical specs and plans stay as written.

### C4: verification

- [ ] `task verify` and `task audit` with a freshly built binary, at the
      root; `task verify` in both examples.
- [ ] `vibe diff` on a checkout of `main`'s state (the pre-0024 files)
      reports exactly the two `.codex/` files out of date, and nothing for
      `.claude/`.
- [ ] Record the results below; spec 0024 status → implemented.

## Verification record

To be filled in at C4.
