# Spec 0051: Contract v1.0.0

Current version: 1.0.1 (see "Changes" under "Versioning").

Status: accepted (2026-10-16). Issue #96. Decided by ADR 0025. Settled
with the user on 2026-10-16 in a grilling session; "Qn" refers to its
questions, which the issues also cite. The rules come from the contract
draft v0 (2026-10-15). Every rule in it was drawn from an existing spec,
ADR or principle, and no requirement was added.

## Problem

`vibe audit` today checks that managed files match their templates
(ADR 0002). ADR 0025 replaces that with a contract: properties a
repository must have, checked deterministically, whatever tools provide
them. This spec is that contract. The audit (#98–#104, #111), the guard
(#105, #106) and the setup skill (#108, #109) all implement or follow
it. Where they disagree with this spec, this spec wins until it is
changed by a versioned release.

## Terms

The terms are defined in `GLOSSARY.md`: contract, rule, level, check,
probe, guard corpus, entrypoint, waiver, not applicable, conformant,
component, guard exception, unlock.

## The verdict

A repository is **conformant** under contract version X.Y.Z when no MUST
rule fails. Other outcomes:

- A failed SHOULD rule is reported and never changes the verdict.
- A rule that is not applicable or waived is reported and not checked.
- If any MUST rule cannot be decided, there is no verdict. Reasons
  include an unsupported CI provider, an unsupported contract version, a
  missing declaration, and a probe that could not run. "Cannot decide"
  is never reported as conformant.

## `vibe.yaml`

`vibe.yaml` is the only file in an adopting repository that belongs to
VibeConform. Version 2 has these fields:

| Field | Required | Meaning |
|---|---|---|
| `contract` | yes | The contract version the repository follows, such as `1.0.0` |
| `platforms` | yes | Non-empty subset of `linux`, `windows`, `macos` |
| `agents` | yes | Subset of `claude`, `codex`; may be empty |
| `components` | no | List of `{path, language}`; `language` is `go`, `python` or `typescript` |
| `entrypoints` | yes | `verify`, `verify-fast`, `fmt`, `fmt-check` are required; `setup` is optional |
| `waivers` | no | List of `{rule, reason, until}` |

Rules for the fields:

- **Components** (Q11): when `components` is omitted, the repository is
  one component at its root. Its language comes from the manifest found
  there: `go.mod` means go, `pyproject.toml` means python, and
  `package.json` means typescript. If none of these is found, or more
  than one is, the configuration is invalid and the error names
  `components`.
- **Entrypoints** are repository-wide, not per component. Each value is
  either one command or a list of commands.
  - A list runs in order and fails as soon as one command fails.
  - The same value is used on every platform.
  - Commands are parsed as POSIX shell.
- **The `setup` entrypoint**, when declared, runs once in each probe
  copy before any other entrypoint. An example is installing
  dependencies with a frozen lockfile.
- **Waivers**: `rule` must name a SHOULD rule, `reason` must be
  non-empty, and `until` must be a date. A waiver for a MUST rule is
  invalid. An expired waiver is ignored: the rule is checked, and the
  report marks it "waiver expired" (Q9).
- **`agents`**: listing `codex` is valid, but contract v1 has no Codex
  rules.
- There are no `integrations`, `development` or `standard` fields (Q23).

## Checks

- **Static** checks read committed configuration and run nothing.
- **Execution** checks run entrypoints or other commands and look at
  their exit codes and side effects.
- **Probes** feed a known input and check how the repository reacts.

Execution checks and probes run in a **probe copy**: a temporary
directory holding the working tree's tracked files, including
uncommitted changes. Untracked and ignored files are not copied. The
copy is deleted afterwards, and the audit never writes inside the
repository.

### Planted defects

Planted defects are the probe inputs for V-01, V-02 and A-05 (Q16, Q17).
For each component, the audit plants one defect at a time and resets
the copy between defects.

| Defect | Go | Python | TypeScript |
|---|---|---|---|
| Unformatted file | A `.go` file with wrong indentation and spacing | A `.py` file with wrong spacing | A `.ts` file with wrong spacing |
| Static-analysis defect | A `fmt.Printf` call whose arguments do not match its format (caught by `go vet`) | A reference to an undefined name | A reference to an undefined name |
| Failing test | A `_test.go` with a test that fails | A `test_*.py` with a test that fails | A `*.test.ts` with a test that fails |

Every defect file is created next to an existing test file in the
component:

- Go: in a package directory that already has a `_test.go` file.
- Python: next to a `test_*.py` or `*_test.py` file.
- TypeScript: next to a `*.test.ts`, `*.spec.ts`, `*.test.tsx` or
  `*.spec.tsx` file.

A component with no such file fails V-01 with "component <path> has no
tests". The probe only checks that the entrypoint fails, not which tool
caught the defect. The contract therefore implies one lint floor: verify
must catch the static-analysis defect above.

### The guard corpus

The guard corpus is a fixed list of tool calls that belongs to this
contract version (Q5). Each case has:

- a Claude Code tool payload
- an expected result: `deny` or `allow`
- for `deny` cases, a **guard rule ID** naming the baseline rule the
  case belongs to

The baseline rules and their IDs are:

| Guard rule ID | Refuses |
|---|---|
| `rm-rf` | Recursive forced deletion in a POSIX shell, in any spelling the corpus lists |
| `remove-item-recurse-force` | Recursive forced deletion in PowerShell |
| `git-reset-hard` | `git reset --hard` |
| `git-clean-fd` | `git clean` with `-f` and `-d` |
| `git-push-force` | `git push --force` and its spellings. `--force-with-lease` is allowed. |
| `git-branch-force-delete` | `git branch -D` |
| `git-discard-worktree` | `git restore .` and equivalent whole-tree discards |
| `secret-file` | Writing or editing credential files (`.env`, keys, `*.pem`, …) |
| `guard-policy-file` | Writing or editing the guard policy files |
| `guardrail-settings` | Writing or editing `.claude/settings.json` or `.claude/hooks/` without a valid unlock (A-08) |

The first nine IDs are the rule IDs the guard uses today
(`internal/module/agents/claude/policy.go`). Exceptions written under
spec 0050 therefore keep their meaning. `guardrail-settings` is new.

The corpus starts from the 162 cases now in
`internal/module/agents/claude/testdata/guard_corpus.json` (101 deny,
61 allow). It also adds variant spellings such as `rm -r -f`, and cases
for A-08.

Adding a deny case is MAJOR. Adding an allow case, or removing a deny
case, is MINOR.

### Guard policy and exceptions

Any guard implementation may be used, but its loosening must be
visible to the audit (A-04). The repository declares guard exceptions in
`.claude/hooks/policy.json`, version 2, under `exceptions`. Each
exception has:

- `rule`: a guard rule ID
- `match`: a command prefix or a path glob
- `reason`: non-empty

The file may also hold the repository's own deny rules, in any shape
the guard reads. The contract only reads `version` and `exceptions`,
with one addition since 1.0.1: when a PreToolUse hook runs
`vibe-guard hook`, the guard is `vibe-guard`, and A-04 also checks that
`vibe-guard` can load the file (spec 0053, "policy.json version 2").
Another guard's keys stay the guard's.

A deny case counts as satisfied in either of two ways:

- the guard refuses it, or
- the guard allows it and an exception for the case's guard rule ID
  matches the case's command or path.

The audit lists every case that an exception lets through.

### The unlock marker

An unlock (A-08) is a file named `vibe-unlock` in the repository's
common Git directory (`git rev-parse --git-common-dir`). It holds one
line: an RFC 3339 expiry time. A marker is valid while that time is in
the future. A missing, unreadable or expired marker is not valid.

## Rules

Contract v1.0.0 has 25 rules: 16 MUST and 9 SHOULD.

"Called" means a hook command, CI `run:` step or entrypoint that, when
parsed as shell, contains the declared command as a whole simple
command. For a list entrypoint, "called" means every command in the
list is called, in any steps or jobs.

### V: verification entrypoints

| ID | Rule | Check | Level | Source |
|---|---|---|---|---|
| V-01 | `verify` covers formatting, static analysis and tests | Execution and probe: in the probe copy, `verify` exits 0. With each planted defect in each component, it exits non-zero. The report names each defect kind (format, static analysis, test) that did not make it fail. | MUST | spec 0011, principle 2 |
| V-02 | `verify-fast` catches the same defects | Probe: with each planted defect in each component, `verify-fast` exits non-zero. The contract does not measure speed. | MUST | spec 0023 |
| V-03 | `fmt-check` writes no file | Execution: in the probe copy with the unformatted-file defect planted, after `fmt-check` runs, `git status --porcelain` lists only the planted file, with its content unchanged | MUST | spec 0011 |
| V-04 | The entrypoints are declared and used | Static: `vibe.yaml` declares every required entrypoint, each non-empty. The other rules check that CI and hooks call them. | MUST | ADR 0025 (replaces ADR 0009's protected task names) |

### H: local Git hooks

Recognized hook managers: lefthook (`lefthook.yml`, `.lefthook.yml`),
pre-commit (`.pre-commit-config.yaml`) and husky (`.husky/`). Adding one
is MINOR. A repository that uses another manager gets "not recognized"
on both rules.

| ID | Rule | Check | Level | Source |
|---|---|---|---|---|
| H-01 | Hook configuration is committed and installs with one command | Static: a recognized hook manager's configuration is committed | SHOULD | spec 0011 |
| H-02 | pre-commit checks formatting; pre-push runs verification | Static: the pre-commit hook calls `fmt-check` or `fmt`; the pre-push hook calls `verify-fast` or `verify` | SHOULD | spec 0011, ADR 0017 |

### C: CI

Contract v1 supports GitHub Actions only (Q19). If the repository has
CI configuration for another provider and no `.github/workflows/`, the
C rules cannot be decided.

| ID | Rule | Check | Level | Source |
|---|---|---|---|---|
| C-01 | CI runs `verify` on pull requests and on pushes to the default branch | Static: workflows triggered by `pull_request` and by `push` to the default branch call `verify` | MUST | spec 0010 |
| C-02 | Toolchains and actions are pinned | Static, across every workflow (not only generated ones): every `uses:` other than a local path is pinned to a full 40-character commit SHA, and no `run:` step contains a command word ending in `@latest` | MUST | spec 0044 |
| C-03 | Workflow permissions default to read | Static: every workflow has top-level `permissions` that grant no write scope (`contents: read`, `read-all` or `{}`). Jobs may widen them. | MUST | spec 0010 |
| C-04 | The audit runs in its own job | Static: a job triggered on pull requests runs `vibe audit` (or the setup action followed by it) and does not call `verify` | MUST | spec 0022 |
| C-05 | Tests run on Linux and Windows | Static: the steps that call `verify` run on a Linux runner and on a Windows runner, after matrix expansion. Not applicable when `platforms` does not include `windows`. | SHOULD | principle 2 |

### A: agent guardrails

The A rules cover Claude Code only. Codex is exempt; bringing it in is
MAJOR. Personal, uncommitted settings (`.claude/settings.local.json`)
may override any guardrail, and the audit does not read them.

| ID | Rule | Check | Level | Source |
|---|---|---|---|---|
| A-01 | Shell commands and file edits pass through the guard | Static: in `.claude/settings.json`, PreToolUse hooks match each of `Bash`, `PowerShell`, `Write`, `Edit` and `MultiEdit` | MUST | spec 0021 |
| A-02 | The guard refuses the baseline unless an exception says otherwise | Probe: every corpus case is fed to every PreToolUse hook command that matches its tool. A deny case is satisfied as described under "Guard policy and exceptions". Every allow case must be allowed. A hook "refuses" by exiting 2. | MUST | specs 0012, 0047 |
| A-03 | The guard fails closed | Probe: every deny case is still refused in two situations: with `PATH` reduced to the operating system's directories, and with `.claude/hooks/policy.json` replaced by invalid JSON | MUST | spec 0021 |
| A-04 | The guard is loosened only through listed exceptions | Static: `.claude/hooks/policy.json` is valid version 2; every exception names a guard rule ID and has a non-empty reason; when a PreToolUse hook runs `vibe-guard hook`, vibe-guard can load it; the audit lists them. Tightening is not restricted. | MUST | spec 0050, ADR 0024 |
| A-05 | The agent's turn ends with `verify-fast` | Static: a Stop hook calls `verify-fast`. Probe: with a planted defect, the Stop hook exits 2 for a payload with `stop_hook_active: false`, and exits 0 for one with `stop_hook_active: true` | MUST | spec 0023 |
| A-06 | Edited files are formatted | Static: a PostToolUse hook that matches `Write` and `Edit` calls `fmt` | SHOULD | spec 0023 |
| A-07 | Session start injects facts and changes nothing | Static: a SessionStart hook exists. Execution: running it in the probe copy exits 0, and `git status --porcelain` stays empty. | SHOULD | spec 0023 |
| A-08 | An agent changes guardrail settings only after a person unlocks them | Probe: a `Write` to `.claude/settings.json` and an `Edit` under `.claude/hooks/` are refused with no marker, allowed with a valid marker, and refused with an expired marker. A `Bash` call that runs `vibe unlock`, and any `Write` or `Bash` that creates the marker, are refused. After the Stop hook runs, the marker is gone. | MUST | ADR 0025 |

### D: dependencies and supply chain

| ID | Rule | Check | Level | Source |
|---|---|---|---|---|
| D-01 | Lockfiles are committed and match the declared dependencies | Execution, per component, by the lockfile found. For `go.sum`: run `go mod tidy`, then `git diff --exit-code go.mod go.sum`. For `uv.lock`: `uv lock --check`. For `poetry.lock`: `poetry check --lock`. For `pnpm-lock.yaml`: `pnpm install --frozen-lockfile --lockfile-only`. For `package-lock.json`: `npm ci --dry-run`. A component with dependencies and no recognized lockfile fails. | MUST | specs 0010, 0020 |
| D-02 | An update bot covers dependencies and actions, without automerge | Static: `.github/dependabot.yml` or a Renovate configuration covers each component's ecosystem and `github-actions`, and enables no automerge | SHOULD | spec 0010 |
| D-03 | CI scans for known vulnerabilities | Static: a workflow runs a recognized scanner: `govulncheck`, `pip-audit`, `pnpm audit`, `npm audit`, `osv-scanner` or `trivy`. Adding one is MINOR. | SHOULD | spec 0010 |

### R: removability and repository hygiene

| ID | Rule | Check | Level | Source |
|---|---|---|---|---|
| R-01 | Everything but the audit works without vibe | Static: no entrypoint, hook command or CI step calls `vibe`, except the audit job of C-04. Execution: with `vibe` not on `PATH`, `verify` passes in the probe copy and the A-02 deny cases are still refused. | MUST | principle 3, spec 0017 |
| R-02 | Agents are told the verification entrypoints | Static: `AGENTS.md` exists and contains every `verify` command, and `CLAUDE.md` exists and references `AGENTS.md` | SHOULD | spec 0043 |
| R-03 | Pull requests report verification | Static: a pull request template exists and has a heading containing "verif" (case-insensitive) | SHOULD | spec 0010 |

## Versioning

- The contract uses semver.
  - MAJOR: a change that makes a conformant repository non-conformant.
    Examples: a new MUST rule, a tightened check, a new deny case in the
    corpus.
  - MINOR: a new SHOULD rule, a relaxed check, or a newly recognized
    tool or CI provider.
  - PATCH: a wording fix that changes no repository's verdict.
- Rule IDs and guard rule IDs are never reused.
- A vibe release implements the latest MINOR of each MAJOR it supports,
  and audits every repository that declares that MAJOR with it (Q15).
- If a repository declares a MINOR newer than vibe implements, or a
  MAJOR vibe does not support, there is no verdict: "cannot decide;
  upgrade vibe".

### Changes

- **1.0.1** (2026-10-19, pilot #110, spec 0054). A-04 passed a
  `.claude/hooks/policy.json` that `vibe-guard` cannot load: in pilot P2
  a version 2 file that kept the old `tools` key. A-04 now fails such a
  file when a PreToolUse hook runs `vibe-guard hook`. The finding names
  `policy.json` and quotes the guard's own loader error. The allow cases
  that A-02 and A-08 then see refused name `policy.json` too, not
  `.claude/settings.json`.

  This is a PATCH, not a tightening. A-04's stated purpose is a policy
  that the guard applies. A `vibe-guard` that cannot load its policy
  refuses every call, allow cases included, so A-02 already failed for
  every such repository. No repository's full-audit verdict changes; the
  check now says why at the file that causes it. A missing `policy.json`
  still fails A-04, as in 1.0.0.
- **1.0.0** (2026-10-16). The first version.

## Later versions

These are not in v1.0.0:

- **Refuse agent reads of credential files** through `permissions.deny`.
  This would be a SHOULD rule, so a MINOR release (Q6).
- **Codex in the A rules.** A MAJOR release, once Codex hooks are stable.
- **GitLab CI for the C rules.** A MINOR release (Q19).

## Not in the contract

These are left to the repository and the setup skill (Q7, Q8):

- which tools are used
- lint rule content, including hold-the-line baselines
- hook task names
- CI job layout
- the development workflow
- editor and other integrations

There are no tiers. The report shows SHOULD results beside the MUST
verdict.

## Decisions made while writing this spec

These go beyond the contract draft and the grilling. Approving this spec
approves them.

1. **An entrypoint may be a list.** This repository's CI runs `task
   lint`, `task test` and the others as separate jobs, not `task verify`.
   A list lets CI split verify across jobs while C-01 still checks that
   every part runs.
2. **The optional `setup` entrypoint.** A probe copy has no
   `node_modules` or virtual environment. `setup` installs them once per
   copy before the probes run.
3. **The contract fixes the exception format and the guard rule IDs.**
   Without them, the audit cannot tell an allowed deny case from a
   broken guard, or list the loosenings. This holds for any guard
   implementation.
4. **The contract fixes the unlock marker's location and format.** A-08
   is a probe, so any guard must read the same marker.
5. **Recognized tool lists** for hook managers, lockfiles, update bots
   and scanners. They keep the static checks executable; extending a
   list is MINOR.
6. **The probe copy uses tracked files only, including uncommitted
   changes.** A local audit checks what is about to be committed, and
   untracked files cannot leak in.
7. **A-01's tool list** is `Bash`, `PowerShell`, `Write`, `Edit` and
   `MultiEdit`: the tools the guard reads a command or a path from.

## Acceptance Criteria

1. The spec is merged with all 25 rules, each with ID, rule, check,
   level and source.
2. Every check names what it reads or runs and what counts as pass or
   fail, with no human judgment.
3. `GLOSSARY.md` defines every term in "Terms".
4. #98–#112 can each point to the rules or sections they implement.
