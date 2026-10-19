# Leaving the template-sync system

A repository set up by the template-sync system has a `.vibe/`
directory, and a `vibe.yaml` with `standard:` and `version:` but no
`contract:`. `vibe sync` wrote its files from templates. After this
migration the repository owns every file, and `vibe` only audits it.

Do the steps in order. Stage what you write, change and remove as you
go: the audit reads tracked files only (`SKILL.md`, step 6). Commit
nothing until the audit loop in `SKILL.md` has run.

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
- `ci`: `provider`. Contract 1.0.1 decides the C rules, and D-03, for
  GitHub Actions only. Keep GitLab CI (step 4) and report that C-01 to
  C-04 cannot be decided until contract 1.1.0 (#74): the verdict stays
  "cannot decide".

## 2. Merge each seam pair into one file

The old system split a configuration into a generated base and a local
extension. Merge each pair into the file the tool reads, keeping the
repository's own additions:

- `ruff.base.toml` + `ruff.toml` → `ruff.toml` (drop `extend`).
- `eslint.base.config.js` + `eslint.config.js` → `eslint.config.js`.
- `tsconfig.base.json` + `tsconfig.json` → `tsconfig.json` (drop
  `extends`).
- `lefthook.local.yml` → `lefthook.yml`, when the team commits it.
  Remove the `extends:` entry that names it from `lefthook.yml` in any
  case, also when the file does not exist.
- `.gitlab-ci.local.yml` → `.gitlab-ci.yml`. When it ran as a child
  pipeline (a job whose `trigger:` includes it), move its `stages`,
  `default` and jobs into `.gitlab-ci.yml`, drop its `workflow:`, and
  remove the trigger job. Check that the jobs still run in the order the
  team wants.
- `.claude/hooks/policy.local.json` → the `rules` and `exceptions` of
  `.claude/hooks/policy.json` version 2, which step 5 writes. Each
  exception needs a guard rule ID, a `match` and a `reason`.

## 3. Fold the Taskfiles

- Move every task of `Taskfile.local.yml` into `Taskfile.yml`. A name
  collision means the local task was never reachable: ask the person.
- Remove the `includes:` of `Taskfile.local.yml` and `Taskfile.vibe.yml`.
- Keep `hook:guard` and the old guard script it runs
  (`.claude/hooks/guard.go`, `guard.py` or `guard.mjs`) for now. The old
  PreToolUse hook calls them, and it fails closed: without them it
  refuses every call, the settings write included. Step 5 removes them.
- Remove `audit` and any other task that calls `vibe` (rule R-01).
- In a monorepo, give each component's Taskfile the `setup` task the
  root `setup` calls (`reference/taskrunner/monorepo/`).
- Compare the rest with `reference/taskrunner/<stack>/Taskfile.yml`.

## 4. Remove what belonged to the old system

- `.vibe/` (state and backups).
- `Taskfile.vibe.yml`.
- The managed-region markers: lines such as
  `# vibeconform:begin <name>` and `# vibeconform:end <name>` (and their
  `<!-- -->` forms in Markdown). Keep the text between them.
- Comments that say a file is managed by VibeConform or must not be
  edited.
- Prose about the old system in the team's own docs, README and
  comments. Search for `vibe sync`, `Taskfile.local`, `.gitlab-ci.local`,
  `task audit`, "managed" and "VibeConform", and rewrite each passage
  for the repository as it now is.
- The old `vibeconform` skill: `.claude/skills/vibeconform/` and
  `.agents/skills/vibeconform/`. This skill, `vibeconform-setup`,
  replaces it.
- `.github/workflows/conformance.yml`. The audit workflow from
  `reference/ci/<stack>/audit.yml` replaces it.
- On GitLab, `.gitlab-ci.vibe.yml`, which ran `task audit`. Merge
  `.gitlab-ci.defaults.yml` into `.gitlab-ci.yml`. Keep the team's jobs,
  and make them call the entrypoints: `task verify`, or each command of
  a list `verify`. Add an audit job of its own, which needs every tool
  the entrypoints call. A starting point (not checked by the contract
  until 1.1.0):

  ```yaml
  conformance:audit:
    stage: test
    image: <an image with every toolchain the entrypoints call>
    rules:
      - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
    variables:
      VIBE_VERSION: <an exact release tag, never latest>
    before_script:
      - <install the tools the entrypoints call, at the versions CI pins>
      - |
        base="https://github.com/Manual-debuger/VibeConform/releases/download/$VIBE_VERSION"
        dl=$(mktemp -d) && mkdir -p "$HOME/.local/bin"
        curl -fsSL -o "$dl/checksums.txt" "$base/checksums.txt"
        for tool in vibe vibe-guard; do
          archive="${tool}_${VIBE_VERSION#v}_linux_amd64.tar.gz"
          curl -fsSL -o "$dl/$archive" "$base/$archive"
          (cd "$dl" && grep " $archive\$" checksums.txt | sha256sum -c -)
          tar -xzf "$dl/$archive" -C "$HOME/.local/bin" "$tool"
        done
      - export PATH="$HOME/.local/bin:$PATH"
    script:
      - vibe audit
  ```

## 5. Move the guardrails to `vibe-guard`

Keep the old guard working until the new settings are in place.
`.claude/settings.json` and `.claude/hooks/` are guardrail paths: a
guard running in this session refuses your edits there.

1. `.claude/hooks/policy.json` version 2: `reference/agents/policy.json`
   plus the merged `rules` and `exceptions`, and none of the old guard's
   keys (such as `tools`). `vibe-guard` refuses every call while the
   file has a key it does not know. A guard refuses this write even
   after an unlock, so hand it to the person: give them the exact file
   content and the command in `SKILL.md`, step 4, and wait until they
   say it is done. Do not go on without it.
2. Install `vibe-guard` (`SKILL.md`, step 5).
3. If the old guard is active in this session, ask the person to run
   `vibe unlock` in their own terminal, and wait until they say it is
   done. Then write `.claude/settings.json` from
   `reference/agents/settings.json`, keeping any other hooks the team
   added.
4. Last, remove `hook:guard` from `Taskfile.yml`, the old guard script
   and `.claude/hooks/policy.local.json`. Claude Code may keep running
   the hooks this session started with, so do it in a new session, or
   hand it to the person.

The next Stop locks the guardrails again.

## 6. Write `vibe.yaml` version 2

Replace the old `vibe.yaml` with version 2 (`SKILL.md`, step 3). Then
continue with step 4 of `SKILL.md`.
