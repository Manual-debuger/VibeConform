# Leaving the template-sync system

A repository set up by the template-sync system has a `.vibe/`
directory, and a `vibe.yaml` with `standard:` and `version:` but no
`contract:`. `vibe sync` wrote its files from templates. After this
migration the repository owns every file, and `vibe` only audits it.

Do the steps in order. Commit nothing until the audit loop in `SKILL.md`
has run.

## 1. Read what the old configuration declares

From the old `vibe.yaml`, note:

- `standard` (`prod-go`, `prod-py`, `prod-ts` or `prod-mono`) and
  `components` (`id`, `path`, `profile`: `go`, `py`, `ts`). These become
  the new `components` (`path`, `language`: `go`, `python`,
  `typescript`). `id` and `generated` have no counterpart: keep
  generated-code excludes in the lint and format configuration.
- `integrations`: `editors`, `agents` and `intelligence`. `agents`
  carries over; editors and intelligence are optional content
  (`reference/optional/`).
- `development`: the workflow section of `AGENTS.md` and the docs
  layout. Keep the section's text as the repository's own
  (`reference/optional/workflow.md`).
- `policy`: `agent_guard` and `line_endings`. Line endings stay as a
  plain `.gitattributes`.
- `ci`: `provider`. Contract 1.0.0 decides the C rules for GitHub
  Actions only; keep GitLab CI and report that the C rules cannot be
  decided.

## 2. Merge each seam pair into one file

The old system split a configuration into a generated base and a local
extension. Merge each pair into the file the tool reads, keeping the
repository's own additions:

- `ruff.base.toml` + `ruff.toml` → `ruff.toml` (drop `extend`).
- `eslint.base.config.js` + `eslint.config.js` → `eslint.config.js`.
- `tsconfig.base.json` + `tsconfig.json` → `tsconfig.json` (drop
  `extends`).
- `lefthook.local.yml` → `lefthook.yml`, when the team commits it.
- `.gitlab-ci.local.yml` → `.gitlab-ci.yml`.
- `.claude/hooks/policy.local.json` → the `rules` and `exceptions` of
  `.claude/hooks/policy.json` version 2. Each exception needs a guard
  rule ID, a `match` and a `reason`.

## 3. Fold the Taskfiles

- Move every task of `Taskfile.local.yml` into `Taskfile.yml`. A name
  collision means the local task was never reachable: ask the person.
- Remove the `includes:` of `Taskfile.local.yml` and `Taskfile.vibe.yml`.
- Remove `hook:guard` and the old guard script it ran
  (`.claude/hooks/guard.go`, `guard.py` or `guard.mjs`). `vibe-guard`
  replaces it.
- Remove `audit` and any other task that calls `vibe` (rule R-01).
- Compare the rest with `reference/taskrunner/<stack>/Taskfile.yml`.

## 4. Remove what belonged to the old system

- `.vibe/` (state and backups).
- `Taskfile.vibe.yml`.
- The managed-region markers: lines such as
  `# vibeconform:begin <name>` and `# vibeconform:end <name>` (and their
  `<!-- -->` forms in Markdown). Keep the text between them.
- Comments that say a file is managed by VibeConform or must not be
  edited.
- The old `vibeconform` skill: `.claude/skills/vibeconform/` and
  `.agents/skills/vibeconform/`. This skill, `vibeconform-setup`,
  replaces it.
- `.github/workflows/conformance.yml`. The audit workflow from
  `reference/ci/<stack>/audit.yml` replaces it. On GitLab, the old
  system's `.gitlab-ci.vibe.yml` ran `task audit`: keep a job that runs
  `vibe audit` instead, and merge `.gitlab-ci.defaults.yml` into
  `.gitlab-ci.yml`.

## 5. Move the guardrails to `vibe-guard`

`.claude/settings.json` and `.claude/hooks/` are guardrail paths. If the
old guard is active in this session, it refuses your edits there. Ask the
person to run `vibe unlock` in their own terminal, wait until they say it
is done, then:

1. Write `.claude/hooks/policy.json` version 2
   (`reference/agents/policy.json`, plus the merged exceptions).
2. Install `vibe-guard` (`SKILL.md`, step 5).
3. Write `.claude/settings.json` from `reference/agents/settings.json`,
   keeping any other hooks the team added.

The next Stop locks the guardrails again.

## 6. Write `vibe.yaml` version 2

Replace the old `vibe.yaml` with version 2 (`SKILL.md`, step 3). Then
continue with step 4 of `SKILL.md`.
