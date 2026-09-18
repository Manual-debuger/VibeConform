# .codex/ configuration

Verified against current OpenAI Codex documentation at bootstrap time
(`learn.chatgpt.com/docs/config-file`, `.../docs/hooks`), 2026-09-18.

- `config.toml` — project-level Codex settings. Loaded only when this
  project directory is trusted.
- `hooks.json` — a `PreToolUse` hook that reuses
  `../.claude/hooks/block-dangerous.sh` (see that script for the exact
  patterns blocked) rather than duplicating the destructive-command list.

## Known limitation

As of this bootstrap, Codex CLI's `PreToolUse` hook only fires for the
`Bash` tool call, not for file-edit tools (`apply_patch`, etc.). That means
the secret-file guard implemented for Claude Code
(`.claude/hooks/block-secret-files.sh`, wired via `Write|Edit` matchers in
`.claude/settings.json`) has **no Codex equivalent today** — Codex can
still directly edit a file like `.env` without a hook intercepting it. This
is documented here rather than silently assumed to be covered. Revisit if
Codex adds a file-edit hook event.
