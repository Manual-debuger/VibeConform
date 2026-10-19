---
name: vibeconform-setup
description: Take this repository to a passing VibeConform contract audit (vibe audit), or upgrade it to a newer contract. Use when asked to set up, adopt, migrate to or upgrade VibeConform, to make vibe audit pass, or to install vibe-guard.
metadata:
  vibe-version: {{VIBE_VERSION}}
---

# Set up the VibeConform contract

You take a repository to a conformant `vibe audit` under contract 1.0.0.
The contract names properties, not tools: keep what the repository
already has whenever it satisfies a rule. `vibe audit` is the judge, not
this file.

This skill came from `vibe` {{VIBE_VERSION}}. `vibe init --skill`
rewrites it. Do not edit it in place.

Files under `reference/` are loaded on demand:

- `reference/rules.md`: every rule, what satisfies it, where to start.
- `reference/vibe-yaml/`, `reference/taskrunner/`, `reference/hooks/`,
  `reference/ci/`, `reference/agents/`, `reference/lint/`: reference
  implementations per stack (`go`, `python`, `typescript`, `monorepo`).
  They are examples to adapt, not files to copy blindly.
- `reference/optional/`: integrations and workflow sections. Use them
  only when the person asks.
- `reference/migrate.md`: leaving the template-sync system.
- `reference/upgrade.md`: moving to a newer `vibe` or contract.

## Ground rules

- Do not read secret files (`.env`, keys, `*.pem`, credentials).
- Each configuration goes in one file. Write no seam files (`*.local.*`,
  `*.base.*`).
- Keep the team's tools: task runner, hook manager, CI layout, lint rules.
  Change only what a rule needs.
- Do not weaken a test, lint or type check to make a change pass.
- Nothing but the audit job may call `vibe` (rule R-01).
- Never run `vibe unlock`, and never create the unlock marker. When the
  guard refuses an edit you need, stop and ask the person.

## Process

### 1. Detect

- **Components and languages**, by the contract's manifest rule: `go.mod`
  is go, `pyproject.toml` is python, `package.json` is typescript. One
  manifest at the root is one component. Several components, or a root
  with none or more than one manifest, need a `components:` list of
  `{path, language}`.
- **Existing configuration**: task runner (`Taskfile.yml`, `Makefile`,
  `justfile`, `package.json` scripts), hook manager (`lefthook.yml`,
  `.lefthook.yml`, `.pre-commit-config.yaml`, `.husky/`), CI provider
  (`.github/workflows/`, `.gitlab-ci.yml`, others), update bot
  (`.github/dependabot.yml`, Renovate), lint and format configuration,
  agent settings (`.claude/settings.json`, `AGENTS.md`, `CLAUDE.md`).
- **The template-sync system**: a `.vibe/` directory, or a `vibe.yaml`
  with `standard:` and no `contract:`.
- **Platforms** the team develops on (`linux`, `windows`, `macos`) and
  **agents** (`claude`, `codex`). Ask when you cannot tell.

### 2. Migrate

If the template-sync markers are present, follow `reference/migrate.md`
first: merge each seam pair into one file, fold `Taskfile.local.yml` into
`Taskfile.yml`, remove `.vibe/`, `Taskfile.vibe.yml`, the old guard
scripts and the managed-region markers, and replace the old `vibeconform`
skill.

### 3. Decide the entrypoints

Keep the team's runner and use its existing targets when they exist:
`make verify`, `just verify`, `npm run verify`, `task verify`. Use Task
only when there is no runner (`reference/taskrunner/<stack>/`). Then
write `vibe.yaml` version 2 from `reference/vibe-yaml/<stack>.yaml`:
`contract: 1.0.0`, `platforms`, `agents`, `components` when needed, and
the entrypoints `verify`, `verify-fast`, `fmt`, `fmt-check`, plus `setup`
when the audit's probe copy must install dependencies first. An
entrypoint is one command or a list; write the commands exactly as CI and
the hooks will call them.

### 4. Satisfy the rules

Work through `reference/rules.md` category by category: V, H, C, A, D,
R. For each rule, keep what already satisfies it; otherwise start from
the reference file it names and adapt it. Notes:

- Pin every action in `.github/workflows/` to a full commit SHA (C-02).
- The audit workflow (`reference/ci/<stack>/audit.yml`) uses the setup
  action `Manual-debuger/VibeConform`, pinned to the commit of the
  release tag. Find it with
  `git ls-remote https://github.com/Manual-debuger/VibeConform refs/tags/<version>`.
- For Claude Code, write `.claude/hooks/policy.json` first and
  `.claude/settings.json` last: once the guard's hooks are active, the
  guard protects both files.
- `AGENTS.md` names every `verify` and `verify-fast` command and says:
  "Do not weaken a test, lint or type check to make a change pass."
  `CLAUDE.md` references `AGENTS.md` (`reference/agents/`).
- Every component needs a test, or V-01 fails with "has no tests".
- Commit lockfiles (D-01).

### 5. Install `vibe-guard`

The guard hooks refuse every tool call when `vibe-guard` is not on
`PATH`. Install version {{VIBE_VERSION}} for the person:

- **From the release** (no Go needed): download
  `vibe-guard_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) and
  `checksums.txt` from
  https://github.com/Manual-debuger/VibeConform/releases/tag/{{VIBE_VERSION}}.
  `<version>` is the tag without the leading `v`. Check the archive's
  SHA-256 against its line in `checksums.txt` (`sha256sum -c
  --ignore-missing checksums.txt`, or `Get-FileHash` on Windows) before
  you unpack it. Stop if it does not match. Put `vibe-guard` in a
  directory on `PATH`.
- **With Go**, when Go is present and the person prefers it:
  `go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@{{VIBE_VERSION}}`.

Check with `vibe-guard version`. Installing a tool on the person's
machine needs their consent: ask before you do it.

### 6. Audit loop

1. Run `vibe audit --static --json` until no static MUST rule fails.
2. Then run `vibe audit --json`, the full audit. It runs the entrypoints
   and the guard probes in a probe copy, so it takes minutes.
3. For each rule whose `result` is `fail`, read its `findings`: fix what
   each one names, in its `file`, as its `fix` says. Rerun.
4. Stop when `verdict` is `conformant`.

Also stop, and report what is left, when:

- a finding needs a person: a tool missing on this machine (`undecided`
  with "is not installed on this machine"), an unlock, a CI provider the
  contract does not support, or a waiver of a SHOULD rule;
- two rounds in a row change no rule's result.

Never edit the audit's inputs to hide a failure: no disabled test, no
removed lint rule, no exception in `policy.json` the person has not
asked for.

### 7. Report

Give the person:

- the verdict, and every MUST rule that is not `pass`;
- the files you wrote, changed and removed;
- the SHOULD rules that still fail, and why;
- what they must do themselves (install a tool, `vibe unlock`, push).
