# Spec 0012: Agent-config module (`internal/module/agents`)

Status: proposed.

## Problem

VibeConform's stated philosophy is that mechanical rules belong in
deterministic tooling rather than in AI prompts. This repository already
practices that: `.claude/settings.json` and `.codex/hooks.json` register
pre-tool-use hooks that block destructive shell patterns before they run,
and `AGENTS.md`/`CLAUDE.md` are deliberately thin routing surfaces rather
than handbooks.

None of it is expressible as desired state. A repository declaring
`production/v1` gets lint config, CI, and task definitions, but its AI
agents run unguarded — which is the one part of the standard whose absence
is actively dangerous rather than merely untidy.

This is also the increment that forces the file-mode question: the hooks are
shell scripts, and a hook script that is not executable fails open.

## Scope

A new package, `internal/module/agents`, providing one module:

- `Module.Name()` returns `"agent-config"`.
- `Module.Resolve` returns five `resource.Generated` resources, embedded via
  `go:embed`, seeded verbatim from this repository:

  | Path | Mode |
  |---|---|
  | `.claude/settings.json` | default |
  | `.claude/hooks/block-dangerous.sh` | `0o755` |
  | `.claude/hooks/block-secret-files.sh` | `0o755` |
  | `.codex/config.toml` | default |
  | `.codex/hooks.json` | default |

Registered into `production/v1` after `repo-tooling`.

Plus the `resource.Resource.Mode` field and the default change from `0o600`
to `0o644`, per `docs/decisions/0006-resource-file-mode.md`.

## Behavior

- `resource.Resource` gains `Mode os.FileMode`; zero means the
  ownership-appropriate default (`0o644` for `Generated`). `vibe sync`
  passes the resolved mode to `atomicfile.Write` instead of the hardcoded
  `0o600`.
- Mode does not participate in reconciliation — content hashes decide, as
  before. See ADR 0006's consequences.
- The two hook scripts and the configs that reference them are one module,
  not two, because they are only correct together: `.claude/settings.json`
  naming a hook script that was not written is worse than neither.

## Explicit non-goals

- **No `AGENTS.md` or `CLAUDE.md`.** They are prose — project-specific
  routing surfaces whose whole value is that a human wrote them for this
  repository. Generating them would produce exactly the fabricated,
  ignored-by-everyone instruction file the project's philosophy argues
  against. They stay project-owned and unmanaged.
- **No `.claude/settings.local.json`.** It is gitignored, user-local, and
  may hold machine-specific settings. VibeConform must never write it.
- No `.codex/README.md` — documentation about the configuration, not
  configuration.
- No mode participation in reconciliation or state (ADR 0006).
- No hook *content* validation: the module writes the scripts, it does not
  execute them or verify they block what they claim to. Testing a guardrail
  requires running it, and `Resolve` must stay pure.
- No agent installation, no `claude`/`codex` CLI invocation, no settings
  merged into a user's global configuration — repository scope only.
- No `StructuredPatch` ownership for `.claude/settings.json`, even though
  merging VibeConform's hooks into a repository's existing settings is the
  obviously better long-term behavior. `Generated` means a repository with
  its own `settings.json` gets a conflict it must resolve by hand. That is
  the honest v1 answer; see follow-on work.

## Design notes

- The hook scripts are POSIX `sh`, invoked on Windows through `bash` (see
  `.codex/hooks.json`'s `commandWindows`). They are embedded and written
  byte-for-byte with LF endings; CRLF would break them under `bash` on
  Windows. Spec 0008's no-translation rule is what makes this safe.
- **Review this increment's generated output against the live files by
  hand before merging.** These five files are the guardrails constraining
  the agents that work in this repository, including the one implementing
  this spec. A template that quietly drops a blocked pattern would disarm a
  protection that everything else assumes is on, and no test in this
  increment would fail.
- Package `internal/module/agents` (not `agents/claude` + `agents/codex`):
  the two agents share hook scripts, so splitting them would put the same
  files in two modules or invent a shared third. Revisit if a repository
  ever wants one agent configured and not the other — which needs
  conditionality that does not exist until M2.

## Follow-on work

- `StructuredPatch` ownership and a real merge for `.claude/settings.json`,
  so a repository can keep its own hooks alongside the standard's. This is
  the first concrete demand for a non-`Generated` ownership mode, and it
  needs the decision-engine work spec 0006 deferred.
- Mode in `.vibe/state.yaml` and in `reconcile.Decide`, closing ADR 0006's
  known hole.
- Managing the blocked-pattern list as data rather than as a shell script,
  once more than one agent runtime needs it.
