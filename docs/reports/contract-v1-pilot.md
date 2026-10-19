# Contract v1 pilot report

The pilot of spec 0054 (#110): the `vibeconform-setup` skill run in one
real team repository (P1) and two stand-ins built from the examples (P2,
P3), plus the same-MAJOR upgrade run and the real-session checks carried
over from spec 0053. Each finding is classified as a contract change, a
skill change or no change.

The pilots ran on 2026-10-19 with `vibe` and `vibe-guard`
v0.12.0-alpha.1 (contract 1.0.0) from the release. The SHA-256 of every
downloaded archive matched `checksums.txt`.

## Summary

| Pilot | Stack, platform | Starting point | Verdict | Rounds | Manual changes | Files written / changed / removed | Time |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P1 | `meeting-summarizer`: Go, TypeScript (pnpm), Python (uv) monorepo; GitLab CI; WSL2 Ubuntu | template-sync v0.9.0-alpha.1 (`prod-mono`), migration path | undecided (C-01 to C-04: GitLab) | 3 full (2 by the skill, 1 after the manual changes) | 4 | 0 / 21 / 8, plus 94 skill files | about 60–70 min active (3 h 07 min wall clock, with a rate-limit stop); full audit 5 min 14 s |
| P2 | `examples/typescript`: one pnpm component; GitHub Actions; Windows 11, Git Bash | template-sync (`prod-ts`), migration path | conformant | 2 static, 3 full | 2 | 4 / 13 / 8, plus 94 skill files | about 8 min of agent time; full audit about 60 s |
| P3 | source of `examples/python`: one uv component; GitHub Actions; Windows 11, Git Bash | 4 files, no tooling, from scratch | conformant | 1 static, 2 full | 1 | 16 / 0 / 0, plus 96 skill files | about 2 min of agent time; full audit about 32 s |

The file counts leave out the skill directories and include
`.claude/hooks/policy.json`, which a person wrote in every pilot (see
F-01). Times leave out the person's steps. No workflow or pipeline ran:
nothing was pushed in any pilot.

## P1: meeting-summarizer

- **Stack and platform:** a monorepo with `services/api` (Go 1.27.2),
  `apps/web` (TypeScript, pnpm 12.8.1) and `services/worker` (Python
  3.12, uv), plus contract schemas, OpenTofu and Docker deploy files, and
  GitLab CI. WSL2 Ubuntu (x86_64), driven from Windows through
  `wsl -e bash -lc`.
- **Starting point:** branch `chore/vibeconform-v0.9` at `d0ff6e4`, set up
  by the template-sync system v0.9.0-alpha.1 (`standard: prod-mono`,
  `ci.provider: gitlab`). Seams: `.gitlab-ci.local.yml` (a child
  pipeline), `.gitlab-ci.vibe.yml`, `Taskfile.local.yml`,
  `Taskfile.vibe.yml`, `apps/web/tsconfig.base.json`, an `extends:` entry
  in `lefthook.yml` for an absent `lefthook.local.yml`, the old guard
  (`guard.go`, policy v1), the old skill, and managed-region markers in 5
  files. The pilot branch is `vibeconform/contract-v1-pilot` (commit
  `3e72dbc`). Nothing was pushed or merged; the branch is the owner's to
  decide about.
- **Verdict:** undecided. MUST: 12 pass, 4 undecided. SHOULD: 5 pass, 2
  fail, 1 undecided, 1 not applicable.

| Rule | Level | Result | Note |
| --- | --- | --- | --- |
| V-01 to V-04 | MUST | pass | `verify` is a list: `task api:verify`, `task web:verify`, `task worker:verify` |
| H-01, H-02 | SHOULD | pass | the existing `lefthook.yml` was kept |
| C-01 to C-04 | MUST | undecided | contract 1.0.0 reads GitHub Actions workflows only |
| C-05 | SHOULD | not applicable | `platforms: [linux]` |
| A-01 to A-05, A-08 | MUST | pass | A-02, A-04 and A-08 failed until a person wrote the v2 policy |
| A-06, A-07 | SHOULD | pass | |
| D-01 | MUST | pass | |
| D-02 | SHOULD | fail | no Dependabot or Renovate; the fix text names `.github/dependabot.yml` |
| D-03 | SHOULD | undecided | GitLab CI is not read (the api job runs `govulncheck`) |
| R-01 | MUST | pass | |
| R-02 | SHOULD | pass | |
| R-03 | SHOULD | fail | `.gitlab/merge_request_templates/Default.md` has `## Verification`; the rule reads only `.github/pull_request_template.md` |

- **Audit rounds:** before the migration, `vibe audit --static --json`
  refused the v1 configuration, as expected. Full round 1 failed V-01 and
  R-01 (the team's golangci-lint pin, F-18) and A-02, A-04, A-08. Full
  round 2, after an environment fix only, failed A-02, A-04 and A-08: the
  policy was still v1, and the skill stopped because the fix needs a
  person. Round 3, after the manual changes, gave the verdict above.
- **Manual changes (4):** `docs/development/README.md`, `README.md` and
  `docs/architecture/OVERVIEW.md` still described the old system (F-09);
  `.claude/hooks/policy.json` v1 to v2, which the user wrote by hand as a
  one-line v2 JSON in WSL (F-01).
- **Files:** 0 written; 21 changed (`.claude/CLAUDE.md`,
  `.claude/hooks/policy.json`, `.claude/settings.json`, `.gitignore`,
  `.gitlab-ci.yml`, `AGENTS.md`, `CLAUDE.md`, `README.md`, `Taskfile.yml`,
  `apps/web/Taskfile.yml`, `apps/web/eslint.tsx.config.js`,
  `apps/web/tsconfig.json`, `docs/README.md`,
  `docs/architecture/OVERVIEW.md`, `docs/development/README.md`,
  `docs/specs/README.md`, `lefthook.yml`, `services/api/Taskfile.yml`,
  `services/worker/Taskfile.yml`, `services/worker/ruff.toml`,
  `vibe.yaml`); 8 removed (`.claude/hooks/guard.go`, the old skill,
  `.gitlab-ci.local.yml`, `.gitlab-ci.vibe.yml`, `.vibe/state.yaml`,
  `Taskfile.local.yml`, `Taskfile.vibe.yml`,
  `apps/web/tsconfig.base.json`). The skill added 94 files, 47 per copy.
  The configuration now has one CI file instead of three and one root
  Taskfile instead of three.
- **The user's feedback on the number of files and their readability:**
  see [the user's feedback](#the-users-feedback).

## P2: examples/typescript, migration

- **Stack and platform:** one TypeScript component at the root (pnpm
  10.34.5, Node 24.20.0, Task 3.53.1, lefthook 2.1.14). Windows 11, Git
  Bash. GitHub Actions.
- **Starting point:** `examples/typescript` at `2458c63`, exported with
  `git archive` (29 tracked files) into a fresh repository. Set up by the
  template-sync system (`standard: prod-ts`): `.vibe/state.yaml`,
  `Taskfile.vibe.yml`, seam pairs for ESLint and tsconfig, the old
  `guard.mjs`, and the old skill. No `AGENTS.md`; the `test` script was
  `echo "no tests yet"`; `.gitignore` did not ignore `node_modules/`.
- **Verdict:** conformant. MUST 16/16, SHOULD 9/9 pass.
- **Audit rounds:** static round 1 failed C-04 and R-02 because the new
  files were not tracked yet (F-04); static round 2, after `git add -A`,
  had no static failure. Full round 1 failed A-02 and A-08: the old
  `policy.json` ("version 2" with the old `tools` key) cannot be loaded by
  the new guard, and the static A-04 check had accepted it (F-02). The
  skill stopped there. Full round 2, after the user wrote the policy,
  failed V-01 and R-01 and left V-02 undecided: the one-line
  `policy.json` failed `fmt:check` (F-06). Full round 3, after the user
  ran `pnpm exec prettier --write` on it, was conformant.
- **Manual changes (2):** `.claude/hooks/policy.json` replaced by hand,
  then formatted with Prettier.
- **Files:** 4 written (`.github/workflows/audit.yml`, `.prettierignore`,
  `AGENTS.md`, `src/index.test.ts`); 13 changed
  (`.claude/hooks/policy.json`, `.claude/settings.json`, `.gitattributes`,
  `.github/workflows/ci.yml`, `.gitignore`, `.vscode/tasks.json`,
  `CLAUDE.md`, `Taskfile.yml`, `eslint.config.js`, `lefthook.yml`,
  `package.json`, `tsconfig.json`, `vibe.yaml`); 8 removed
  (`.vibe/state.yaml`, `Taskfile.vibe.yml`, `.claude/hooks/guard.mjs`,
  `.github/workflows/conformance.yml`, `eslint.base.config.js`,
  `tsconfig.base.json`, both old skill files). The skill added 94 files.
  Both seam pairs were merged. The `.prettierignore`, the `node:test`
  test and the `node_modules/` ignore were the agent's judgment calls
  (F-05, F-10, F-20).
- **The user's feedback on the number of files and their readability:**
  see [the user's feedback](#the-users-feedback).

## P3: examples/python, from scratch

- **Stack and platform:** one Python component at the root (uv, CPython
  3.14.3 locally, `requires-python >=3.12`, Task 3.53.1, lefthook).
  Windows 11, Git Bash. GitHub Actions.
- **Starting point:** `src/example/{__init__,greet}.py`,
  `tests/test_greet.py` and `pyproject.toml` from `examples/python` at
  `2458c63`. No Taskfile, CI, hooks, agent settings, `vibe.yaml`,
  `uv.lock` or `.gitignore`.
- **Verdict:** conformant. MUST 16/16, SHOULD 9/9 pass. #108's
  clean-repository done-when is met, with one manual change.
- **Audit rounds:** everything was staged before the first round. Static
  round 1 and full round 1 failed only A-04: `.claude/hooks/policy.json`
  was not tracked, because the agent's write was refused (F-01). Full
  round 2, after the user wrote the policy, was conformant.
- **Manual changes (1):** `.claude/hooks/policy.json`. The directory did
  not exist, so the user ran `mkdir -p .claude/hooks` first.
- **Files:** 16 written (`.claude/hooks/policy.json`,
  `.claude/settings.json`, `.gitattributes`, `.github/dependabot.yml`,
  `.github/pull_request_template.md`, `.github/workflows/audit.yml`,
  `.github/workflows/ci.yml`, `.gitignore`, `AGENTS.md`, `CLAUDE.md`,
  `Taskfile.yml`, `lefthook.yml`, `pyrightconfig.json`, `ruff.toml`,
  `uv.lock`, `vibe.yaml`); 0 changed; 0 removed. The skill added 96 files
  (48 per copy). For 3 source files, the setup added 16 files. The
  largest is `Taskfile.yml` (143 lines, about 90 of them the
  `hook:context` and `hook:done` scripts). Each configuration is one
  file, with no seam files.
- **The user's feedback on the number of files and their readability:**
  see [the user's feedback](#the-users-feedback).

## The user's feedback

Given on 2026-10-19, for all three pilots:

- **Readability:** "pilots look much better".
- **Number of files:** on the skill files (two copies of 47 or 48),
  "Quite a lot of skills. Let's just assume it works. I will open a new
  issue if I find they are not during future uses." No change now
  (F-17). Candidates for #112: write only the detected stacks'
  reference files, or one copy instead of two.

## Upgrade run

On P3, with `vibe` and `vibe-guard` built from `contract/phase-3` at
`f651278` (`v0.12.0-alpha.1.0.20261019031719-f65127836450`, contract
1.0.0). A pseudo-version renders the skill's version placeholders as
`latest` (F-07).

- `vibe init --skill` rewrote 14 skill files (7 per copy) and pruned 4
  untracked `.ruff_cache` files from the skill's reference directory
  (F-05). No repository file outside the skill directories changed.
- `upgrade.md` 3a (same MAJOR): `contract:` was already 1.0.0;
  `vibe-guard` was replaced by the tip build; the setup action stayed
  pinned to v0.12.0-alpha.1, because the rendered skill named `latest`,
  which the action refuses.
- **Result:** conformant, MUST 16/16, SHOULD 9/9.
- **Order caveat:** the upgrade was applied while `policy.json` was still
  missing, so the audits right after it failed A-04 only, as before. The
  user added the policy afterwards, and then the audit passed. The run
  shows that the upgrade changed no rule result; it did not start from a
  conformant repository.
- **MAJOR path:** UNVERIFIED. No contract 2.0.0 exists; the drift test
  checks only that `upgrade.md` names `vibe unlock`.

## Real-session checks

The checks carried over from spec 0053.

| Platform | Check | Result |
| --- | --- | --- |
| Windows | the Stop hook blocks once with a failing `verify:fast`, then lets the stop through | PASS: with a deliberately unformatted Go file, the Stop hook refused the stop once ("hook:done: verify:fast fails; fix it before ending the turn."); the next stop, with the file still present, went through |
| Windows | after `vibe unlock` in the user's terminal, `.claude/settings.json` can be edited | PASS: `vibe sync` rewrote it |
| Windows | after the next Stop, the edit is refused again | PASS: after the Stop (which runs `vibe-guard relock`), an Edit of `.claude/settings.json` was refused by `guardrail-settings`, although the user's 5-minute unlock had not expired (see F-22) |
| Windows | `MultiEdit` and the `Write\|Edit` matcher | recorded: this Claude Code version does not offer `MultiEdit` as a tool |
| Windows | the hook shell | Git Bash: the hook commands use POSIX syntax and run |
| Linux | all four checks, and the hook shell | UNVERIFIED: no Claude Code session ran in WSL. The user runs them later in WSL and opens an issue if something is wrong. |
| macOS | all four checks | UNVERIFIED: no machine is available |

## Findings

| ID | Finding | Classification | Evidence | Resolution |
| --- | --- | --- | --- | --- |
| F-01 | The agent's own `vibe-guard` refuses to write `.claude/hooks/policy.json` in the target repository but allows `.claude/settings.json`, so the skill's order (policy first, settings last) breaks: settings get written and the policy does not. In a migration with an old guard active, removing the old guard before replacing settings could lock the session (by inspection, P2). | skill change | all three pilots: `guard-policy-file` refused the write (by design); the user wrote the file by hand each time | skill on this branch: the policy is written before settings and handed to the person; the guard rule is unchanged |
| F-02 | The static A-04 check accepts a `policy.json` that the guard cannot load. The A-02 and A-08 findings then name `.claude/settings.json`, not the policy. | contract change (PATCH) | P2: A-04 passed with the old policy; A-02 and A-08 failed with `policy-error: tools is not a known key` | contract 1.0.1, being implemented on this branch |
| F-03 | A missing `policy.json` fails A-04, though the reference policy adds nothing to the guard's built-in rules. | no change | P3: A-04 was the only failure; A-02, A-03 and A-08 passed without the file | decided with the user: a missing policy keeps failing A-04 |
| F-04 | The audit reads only tracked files, and the skill does not say so. | skill change | P2 static round 1: "AGENTS.md is missing" and C-04 failed while both files existed, untracked | skill on this branch: `git add` before each audit round |
| F-05 | The skill's own files fall in the repository's format and lint scope. | skill change | P2: `prettier --check` failed on 8 skill files; P3: ruff made `.ruff_cache/` in the skill's reference directory | skill on this branch: ignores for the skill's own files |
| F-06 | The hand-written one-line `policy.json` failed the repository's formatter, which failed V-01 and R-01 and left V-02 undecided. | skill change | P2 full round 2; passed after `pnpm exec prettier --write` | with F-01: the skill hands the person the policy file |
| F-07 | A pseudo-version `vibe` renders the skill's placeholders as `latest`, which the setup action refuses; `upgrade.md` 3a step 3 then cannot be followed. | skill change | upgrade run: rendered `audit.yml` with `version: "latest"` | skill on this branch: no `latest` pin |
| F-08 | A monorepo's CI calls per-component verify; the skill does not describe a `verify` list. | skill change | P1: `verify` as a list of the three component commands passed V-01 | skill on this branch: monorepo verify lists |
| F-09 | `migrate.md` gaps: prose about the old system in the team's docs, the `extends:` entry for `lefthook.local.yml`, missing component `setup` tasks, and merging a GitLab child pipeline into `.gitlab-ci.yml`. | skill change | P1: 3 of the 4 manual changes were stale docs | skill on this branch: `migrate.md` gaps; a reference GitLab audit job is evidence for #74 |
| F-10 | No guidance for a component without a real test runner. | skill change | P2: `test` was `echo "no tests yet"`; `node --test` needed `rewriteRelativeImportExtensions` | skill on this branch: no-test-runner guidance |
| F-11 | Tool detection in a login shell does not see `~/go/bin`, where a stale `vibe` v0.9.0 was installed. | skill change | P1: `~/.bashrc` adds `~/go/bin` only in interactive shells | skill on this branch: login-shell `PATH` note |
| F-12 | Contract 1.0.0 cannot decide C-01 to C-04 or D-03 for GitLab CI. | contract change (MINOR) | P1: 4 MUST and 1 SHOULD undecided | evidence on #74, contract 1.1.0 |
| F-13 | R-03 does not recognize GitLab merge request templates. | contract change (MINOR) | P1: `.gitlab/merge_request_templates/Default.md` has `## Verification` and R-03 fails | evidence on #74, contract 1.1.0 |
| F-14 | The D-02 fix text assumes GitHub (`.github/dependabot.yml`, `github-actions`). | contract change (MINOR) | P1: the fix text for a GitLab repository | evidence on #74, contract 1.1.0 |
| F-15 | A-06 needs a whole-repository `task fmt` after each edit; in a monorepo that is slow, and the team's per-component `fmt:changed` tasks become unused. | no change | P1: `task fmt` runs Prettier, ruff and gofmt/goimports over everything | evidence on #81 |
| F-16 | `vibe audit --static` reports R-01 as `not_run`, and the scope of R-01 is unsettled. | no change | P1 static rounds; spec 0054, "Known deviation" | R-01's scope: #120 |
| F-17 | `vibe init --skill` writes 94 files per repository, most of them references for stacks the repository does not use. | skill change (deferred) | P1 and P2: 94 files; P3: 96 | known issue for #112; the user accepts the count for now (see "The user's feedback") |
| F-18 | The team's golangci-lint pin (v2.13.2) cannot analyze code built by Go 1.27.2. | no change | P1 full round 1: "export data version 5 is greater than maximum supported version 4"; V-02 was reported as undecided, not as a false fail | the team's issue |
| F-19 | Choices left to judgment: `platforms` (P1 `[linux]`, chosen by an agent that could not ask; P2 and P3 `[linux, windows]`), `agents`, replacing P2's CI layout with the reference matrix, the `pip` ecosystem in the reference `dependabot.yml`. | no change | the pilot notes | recorded |
| F-20 | P2's `.gitignore` did not ignore `node_modules/`. | no change | `git add -A` failed with "Filename too long" | a stand-in artifact: the example relies on this repository's root `.gitignore` |
| F-21 | `upgrade.md` does not mention the install-hint text in `settings.json`, which differs per version. | no change | the audit passed with the old text; A-01 to A-08 do not compare it | none |
| F-22 | `vibe-guard relock` prints nothing, whether or not a marker existed, so a person cannot tell from its output whether it locked anything. | no change | Windows real-session check: relock after the Stop worked, with no output | a usability note |
