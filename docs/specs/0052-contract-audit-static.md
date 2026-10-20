# Spec 0052: Contract audit, static checks (Phase 1)

Status: accepted (2026-10-16), amended during implementation (2026-10-16;
each amendment is marked). Spec issue #114. Tickets #98, #99, #100, #101 (milestone "Contract v1 ·
Phase 1: Static checks"). Builds on spec 0051 (contract v1.0.0) and
ADR 0025. "Qn" refers to the grilling questions the issues cite. Where
this spec and spec 0051 disagree, spec 0051 wins, and the disagreement
is a defect in this spec.

## Problem Statement

`vibe audit` compares managed files with their templates (ADR 0002).
Contract v1.0.0 (spec 0051) replaces that with 25 rules about properties
of a repository, checked whatever tools provide them. Nothing implements
those rules yet. An adopter cannot declare a contract version, cannot
find out which rules its repository breaks, and cannot see why or how to
fix them. This repository cannot show that it would pass its own
contract.

## Solution

Phase 1 delivers the static half of the contract audit:

- `vibe.yaml` version 2, parsed and validated, with errors that name
  the field.
- A rule engine in a new package, `internal/contract`, beside the old
  reconcile code. It holds all 25 rules and decides not applicable,
  waived, pass, fail, cannot decide and not run.
- Every static check of contract v1.0.0.
- A new report: a summary by category, the reason for each failure and
  which way the fix goes, `--explain <ID>`, `--json`, `--static`, and
  exit codes 0, 1 and 2.
- A non-required CI job in this repository that runs the new audit,
  plus the fixes this repository needs so that no static MUST rule
  fails.

The old audit keeps working unchanged until #111. `vibe audit` picks the
engine from the configuration it reads.

## User Stories

1. As an adopter, I want to declare `contract: 1.0.0` in `vibe.yaml`, so
   that the audit checks my repository against a fixed, versioned set of
   rules.
2. As an adopter, I want to declare my platforms, so that rules for a
   platform I do not support are reported as not applicable instead of
   failing.
3. As an adopter, I want to declare which agents I use, so that agent
   rules do not apply to a repository that uses no Claude Code.
4. As an adopter, I want to declare `verify`, `verify-fast`, `fmt` and
   `fmt-check` as commands of my own tool (Task, make, just, npm
   scripts), so that the audit does not depend on a task runner.
5. As an adopter whose CI splits verification across jobs, I want to
   declare `verify` as a list of commands, so that the audit checks
   that every part runs.
6. As an adopter of a single-language repository, I want to omit
   `components`, so that the audit finds the language from `go.mod`,
   `pyproject.toml` or `package.json`.
7. As an adopter with several manifests at the root, I want an error
   that names `components`, so that I know which field to declare.
8. As an adopter with a monorepo, I want to declare
   `components: [{path, language}]`, so that the rules that depend on a
   language check each component.
9. As an adopter, I want an invalid `vibe.yaml` to fail with the field
   path and the reason, so that I can fix it without reading the
   parser.
10. As an adopter, I want a misspelled or unknown field rejected, so that
    a typo cannot silently turn a rule off.
11. As an adopter, I want a v1 field (`standard`, `integrations`,
    `development`) in a v2 file rejected with a pointer to the change,
    so that I do not keep config the new audit ignores.
12. As an adopter, I want to waive a SHOULD rule with a reason and an
    expiry date, so that a known gap is visible and time-limited.
13. As an adopter, I want a waiver for a MUST rule rejected, so that
    the verdict cannot be weakened by configuration.
14. As an adopter, I want an expired waiver reported as "waiver
    expired" with the rule checked again, so that a waiver does not
    outlive its reason unnoticed.
15. As an adopter, I want the report to group results by category (V,
    H, C, A, D, R), so that I see at a glance where my repository
    stands.
16. As an adopter, I want each failure to name the file to change and
    the direction of the fix, so that I can act without looking up the
    rule.
17. As an adopter, I want not-applicable rules, waived rules, guard
    exceptions and local guardrail adjustments listed, so that every
    loosening is visible in one place.
18. As an adopter, I want `vibe audit --explain C-02` to show the rule,
    its level, its check and why it exists, so that I understand a
    failure before I change anything.
19. As an adopter, I want `vibe audit --static` to give a quick local
    result without running anything, so that I can check configuration
    changes in seconds.
20. As an adopter, I want `--static` to say it reached no verdict, so
    that a partial check is never mistaken for "conformant".
21. As a CI author, I want exit code 0 for conformant, 2 for a failed
    MUST rule and 1 for "cannot decide", so that a pipeline can tell a
    repository defect from a tool problem.
22. As a CI author, I want `--json` output with a documented shape, so
    that CI and the setup skill can read results without parsing text.
23. As an adopter, I want a failed SHOULD rule reported but never to
    change the verdict, so that recommendations do not block merges.
24. As an adopter, I want my contract version checked against the one
    vibe implements, so that a newer contract gives "cannot decide;
    upgrade vibe" instead of a wrong verdict.
25. As an adopter on GitHub Actions, I want every workflow checked for
    pinned actions and read-only default permissions, not only the
    generated ones, so that a hand-written workflow cannot hide an
    unpinned `@latest`.
26. As an adopter on another CI provider with no GitHub workflows, I
    want the CI rules to report "cannot decide", so that the audit does
    not claim my CI is fine or broken when it cannot read it.
27. As an adopter, I want the audit to check that CI runs `verify` on
    pull requests and on pushes to the default branch, so that the gate
    cannot be skipped by a trigger filter.
28. As an adopter, I want the audit to check that the conformance audit
    runs in its own job, so that a failing audit is never confused with
    failing tests.
29. As an adopter using lefthook, pre-commit or husky, I want my hook
    configuration recognized, so that I keep my hook manager.
30. As an adopter, I want the Claude Code settings checked for guard,
    Stop, PostToolUse and SessionStart hooks, so that the agent
    guardrails exist before the probes test them.
31. As an adopter, I want guard exceptions read from the guard policy
    and listed one by one, so that loosening the guard is never silent.
32. As an adopter, I want my update bot (Dependabot or Renovate) checked
    for coverage and automerge, so that dependency updates are reviewed.
33. As an adopter, I want the audit to check that CI runs a recognized
    vulnerability scanner, so that known vulnerabilities are reported.
34. As an adopter, I want the audit to check that `AGENTS.md` names my
    verify commands and that `CLAUDE.md` points to it, so that agents
    run the right checks.
35. As an adopter, I want the audit to check that my pull request
    template asks for verification, so that reviewers see it was done.
36. As an adopter, I want the audit to check that nothing but the audit
    job calls `vibe`, so that removing VibeConform leaves working
    tooling.
37. As an adopter, I want the audit to read only tracked files, so that
    a local untracked file cannot make my repository look conformant.
38. As an adopter, I want the audit never to write a file, so that
    running it is always safe.
39. As the maintainer, I want the old audit to keep its output and exit
    codes while the new one is built, so that the required
    `Conformance / audit` check keeps working.
40. As the maintainer, I want the new audit to run in this repository's
    CI as a non-required job, so that I see its results on every pull
    request before #111 makes it required.
41. As the maintainer, I want a record of where the old and new audits
    disagree, so that #111 knows what the cutover changes.
42. As the maintainer, I want this repository to have no failed static
    MUST rule, so that the contract is shown to be satisfiable by its
    own author.
43. As the setup skill (#108), I want `--json` results with rule IDs,
    files and fix directions, so that I can fix a repository and audit
    again.
44. As a contributor, I want every implemented rule to have one passing
    and one failing fixture, so that a change to a rule cannot pass
    unnoticed.

## Implementation Decisions

### Package and command

- **New package `internal/contract`.** It holds the `vibe.yaml` v2
  model and parser, the rule registry, the static checks, result
  combination and the report model. It imports nothing from the old
  system (`reconcile`, `state`, `resource`, `standard`, `module`,
  `manifest`). The CLI imports it. Its rendering of text and JSON lives
  in the CLI or in a sub-package; either way the report model is plain
  data.
- **The old code is not changed** (#99), with two exceptions this spec
  needs: the CLI dispatch below, and the template fixes in "This
  repository" below. The old audit's output and exit codes, including
  exit 3, stay byte-identical.
- **Dispatch on the configuration.** `vibe audit` reads the
  configuration file (`vibe.yaml` at `--repo-root` by default, or the
  file named by a new `--config <path>` flag):
  - a top-level `contract` key runs the new engine;
  - a top-level `standard` key runs the old audit, exactly as today;
  - both, or neither, is an error that names the two keys (exit 1).
  - `--config` must name a v2 file. `--static`, `--json` and
    `--explain` with a v1 file are an error (exit 1).
- **`--explain <ID>`** reads no repository and no configuration. It
  prints the rule's ID, level, category, rule sentence, check kinds,
  source, rationale, the check as spec 0051 states it, when the rule is
  not applicable, and whether it can be waived. The rationale is defined
  under "Rule registry and results". An unknown ID is exit 1
  and lists the valid IDs. Combined with any other audit flag it is an
  error.
- **Contract versions.** This vibe implements contract 1.0.0. A
  declared version must be `X.Y.Z`. MAJOR other than 1, or MINOR above
  0, is "cannot decide; upgrade vibe" (exit 1, Q15). A newer PATCH
  (1.0.n) is audited as 1.0.0, because PATCH changes no verdict; the
  report shows both versions.
  - Amended 2026-10-19 (spec 0054, contract 1.0.1): spec 0051's PATCH
    definition now also allows a fix that makes a check match its
    stated purpose, where no conformant verdict becomes
    non-conformant. A vibe audits every 1.0.n as the PATCH it
    implements.
  - Amended 2026-10-19 (spec 0055, contract 1.1.0): this vibe
    implements contract 1.1.0. A declared 1.0.n or 1.1.n is audited as
    1.1.0, since a vibe implements the latest MINOR of its MAJOR (spec
    0051, "Versioning"). MINOR above 1 is "cannot decide; upgrade
    vibe".

### `vibe.yaml` version 2 (#98)

- Fields exactly as spec 0051: `contract`, `platforms`, `agents`,
  `components`, `entrypoints`, `waivers`. Decoding is strict: unknown
  keys at any level are errors. `standard`, `version`, `integrations`,
  `development`, `policy` and `ci` give an error that says the v2
  schema has no such field and where the content went (Q23).
- **Errors** are `vibe.yaml: <field path>: <why>`, with paths like
  `entrypoints.verify[2]` or `waivers[0].until`. Every invalid
  configuration exits 1. The parser reports the first error in document
  order.
- `platforms`: a non-empty list of distinct values from `linux`,
  `windows`, `macos`. `agents`: a list of distinct values from
  `claude`, `codex`; may be empty; must be present.
- **Components** (Q11): each `path` is a clean, relative, slash
  separated directory that exists and is tracked; paths are distinct
  and do not nest. `.` is the root. `language` is `go`, `python` or
  `typescript`. When `components` is omitted, the root's manifests
  decide: exactly one of `go.mod`, `pyproject.toml`, `package.json`
  gives the language; zero or more than one is an error naming
  `components`, with the manifests found.
- **Entrypoints**: each value is a string or a non-empty list of
  strings. `setup` is optional. A missing or empty required entrypoint
  is not a configuration error: V-04 fails, so the rest of the report
  still runs. Each command must parse as POSIX shell into exactly one
  simple command (words with optional leading assignments, no
  operators, pipes or redirections). A command that does not is a V-04
  failure telling the adopter to split it into a list. This follows
  from spec 0051's "called" definition: a command that is not one
  simple command can never be called.
- **Waivers** (Q9): `rule` must be a rule ID of contract 1.0.0 and a
  SHOULD rule; `reason` non-empty after trimming; `until` a date
  (`YYYY-MM-DD`). At most one waiver per rule. A waiver is in force
  through the end of its `until` day in UTC. An expired waiver is
  ignored, the rule is checked, and the report marks it "waiver
  expired". A waiver for a rule that is not applicable is reported as
  unused.

### Rule registry and results (#99)

- The registry holds all 25 rules of contract 1.0.0. Each has ID,
  level, category, rule sentence, its check parts (static, execution,
  probe), source, rationale, an applicability decision and a check
  function per part. `--explain` and the report read only the registry.
- **Rationale** (amended during implementation, approved 2026-10-16):
  spec 0051 has no rationale column. A rule's rationale is its Source
  from spec 0051, followed by the introduction of its category where
  spec 0051 has one (H, C and A). No other text is written for it.
- **Results** per rule: `pass`, `fail`, `not applicable`, `waived`,
  `cannot decide`, `not run`.
  - Not applicable, then waived, are decided first and stop the check.
  - A rule with several parts fails if any part that ran failed;
    otherwise it is `cannot decide` if any part could not be decided;
    otherwise `not run` if any part did not run; otherwise `pass`.
  - Without `--static`, a part that Phase 1 does not implement is
    `cannot decide` with "not implemented in this vibe" and the issue
    that adds it (#102, #103, #104, #106). With `--static`, every
    execution and probe part is `not run` (Q18).
- **Not applicable** in contract 1.0.0: C-05 when `platforms` has no
  `windows`; every A rule when `agents` has no `claude`; R-02 when
  `agents` is empty, and R-02's `CLAUDE.md` half when `agents` has no
  `claude`.
- **Verdict** (spec 0051, "The verdict"): `conformant` when every MUST
  rule passed, is not applicable, or is waived (never, since MUST rules
  cannot be waived). Otherwise, if any MUST rule is `cannot decide` or
  `not run`, there is no verdict: "cannot decide", exit 1. Otherwise
  "not conformant", exit 2. A failed MUST rule beside an undecided one
  therefore gives exit 1, and the report says how many MUST rules
  already fail. SHOULD results never change the verdict.
- **Missing entrypoint.** A rule that needs an entrypoint V-04 found
  missing fails, with a finding that points at V-04: "CI runs verify"
  cannot hold when there is no verify.

### What static checks read

- **Tracked files only.** Static checks read the working-tree content
  of files `git ls-files` lists, the same set the probe copy will use
  (spec 0051, decision 6). An untracked file does not count. Outside a
  Git work tree the audit exits 1.
- **A file that does not parse** fails every rule that needs it, and
  the finding names the file and the parse error. A tool that reads it
  would not get the property either.
- **Shell parsing.** Hook commands, CI `run:` steps, entrypoints and
  husky scripts are parsed as POSIX shell with `mvdan.cc/sh`. That is a
  new production dependency of `vibe`, recorded in a new ADR 0026; ADR
  0025 decision 8 only covers `vibe-guard`. Before parsing a `run:`
  step, each GitHub expression `${{ … }}` is replaced with a placeholder
  that matches no entrypoint word and does not end in `@latest`. A
  `run:` step that does not parse is treated conservatively: for C-02 it
  is split on whitespace and every word is checked for `@latest`; for
  every "called" check it calls nothing.
- **"Called"** (spec 0051): a simple command calls an entrypoint
  command when its words, after quote removal and with leading
  assignments and redirections dropped, equal the entrypoint's words
  exactly. Simple commands are found anywhere in the parse tree: in
  lists, pipelines, conditionals, subshells and functions. A string
  passed to `sh -c` or `bash -c` is not parsed again. A list entrypoint
  is called when each of its commands is called somewhere in the set of
  places the rule looks at.

### Recognition rules

These decide what the static checks see. Extending any of them is a
MINOR contract change (spec 0051, decision 5); none adds a requirement
beyond spec 0051.

- **GitHub workflows**: tracked `.github/workflows/*.yml` and `*.yaml`.
  - Since contract 1.1.0, the C rules, D-03 and R-01's CI part read
    GitLab CI when there are no GitHub workflows and `.gitlab-ci.yml` is
    tracked (see **GitLab CI** below). They cannot be decided when there
    are no GitHub workflows, no `.gitlab-ci.yml`, and a tracked file of
    another provider exists: `.circleci/config.yml`, `Jenkinsfile`,
    `azure-pipelines.yml`, `.travis.yml`, `bitbucket-pipelines.yml`,
    `.buildkite/`, `.drone.yml`. With no CI configuration at all, they
    fail (Q19).
  - **Triggers.** A workflow counts for "pull requests" when it is
    triggered by `pull_request` and its `branches` or `branches-ignore`
    filter admits the default branch. It counts for "pushes to the
    default branch" when it is triggered by `push` and its branch
    filters admit the default branch. A `paths`, `paths-ignore`, or a
    `tags` filter without `branches`, makes it not count. A `types`
    filter counts only if it includes `opened`, `synchronize` and
    `reopened`. Job and step `if:` conditions are ignored.
  - **Default branch**: the target of `refs/remotes/origin/HEAD` when
    the clone has one. Otherwise a filter must admit `main` or
    `master`, and the report says which it assumed. A git failure
    reading `refs/remotes/origin/HEAD`, other than its absence, is a
    tool error.
  - **Reusable workflows**: a job that `uses:` a local workflow
    (`./.github/workflows/…`) is followed, with the caller's triggers.
    Remote reusable workflows and composite actions are not followed:
    their steps call nothing.
  - **Runners** (C-05): `runs-on` as a string or list of labels; a
    `${{ matrix.<key> }}` value expands to the literal values of that
    matrix key, plus `include`, minus `exclude`. A label `ubuntu-*` or
    `linux` is Linux, `windows-*` or `windows` is Windows, `macos-*` or
    `macos` is macOS. Any other expression or label is an unknown
    platform. For a list `verify`, each command must run on a Linux and
    on a Windows runner.
  - **C-02**: every step-level and job-level `uses:` that is not a
    local `./` path must end in `@` and 40 hexadecimal characters. A
    `docker://` reference fails under v1's wording; relaxing that would
    be MINOR. No word of a `run:` simple command may end in `@latest`.
  - **C-03**: a top-level `permissions` of `read-all`, `{}`, or a map
    whose every value is `read` or `none`. Missing top-level
    `permissions` fails, because the repository default may be write.
  - **C-04**: a job in a workflow that counts for pull requests has a
    step whose simple command starts with the words `vibe audit` (any
    further arguments), and no step of that job calls any `verify`
    command.
  - **D-03**: a simple command whose first word is `govulncheck`,
    `pip-audit`, `osv-scanner` or `trivy`, or whose first two words are
    `pnpm audit` or `npm audit`, in any workflow.
- **GitLab CI** (contract 1.1.0): tracked `.gitlab-ci.yml`, read only
  when there are no GitHub workflows. How it is resolved (anchors,
  merge keys, local includes, `default:`, `extends:`, `!reference`,
  child pipelines with `strategy: depend`), which commands count, the
  recognized `rules:` and `only`/`except` conditions, and the GitLab
  reading of C-01 to C-05, D-02, D-03 and R-01 are stated in spec
  0051, "Reading GitLab CI" under "C: CI" and the paragraphs after the
  D and R tables. Spec 0055 records why. The default branch is read as
  for GitHub workflows above.
- **Hook managers** (H rules). Only tracked files count.
  - lefthook: `lefthook.yml` or `.lefthook.yml`, the `run` of
    `commands`, of `jobs` (including nested groups) and of `scripts`
    entries under `pre-commit` and `pre-push`. Tracked files named in
    `extends` are merged in; `lefthook-local.yml` is personal and not
    read.
  - pre-commit: `.pre-commit-config.yaml`; hooks of `repo: local` with
    their `entry` and `args` as words, in the stages they declare (or
    `default_stages`, or `pre-commit`).
  - husky: `.husky/pre-commit` and `.husky/pre-push`, parsed as shell.
  - No recognized manager: H-01 fails with "no recognized hook manager
    (lefthook, pre-commit, husky)", and H-02 is `cannot decide`.
- **Claude Code settings** (A rules): tracked `.claude/settings.json`,
  `hooks.<Event>[]` with `matcher` and `hooks[]` of `type: command`.
  - A matcher that is missing, empty or `*` matches every tool.
    Otherwise it matches a tool when the whole tool name matches it as
    a regular expression. So `Write|Edit` does not cover `MultiEdit`.
    This is the conservative reading; the repository's own policy code
    assumes unanchored matching, and that assumption is not verified.
  - A-01: for each of `Bash`, `PowerShell`, `Write`, `Edit`,
    `MultiEdit`, some PreToolUse entry matches it and has a command
    hook.
  - A-05 (static part): a Stop hook calls `verify-fast`.
  - A-06: a PostToolUse entry that matches both `Write` and `Edit`
    calls `fmt`.
  - A-07 (static part): a SessionStart entry with a command hook exists.
- **Guard policy** (A-04): `.claude/hooks/policy.json` is a JSON object
  with `version: 2`. `exceptions` is optional and is a list. Each entry
  has `rule` (one of the ten guard rule IDs of spec 0051), non-empty
  `match` and non-empty `reason`. Other keys are the guard's and not
  read. A missing file fails A-04. The report lists every exception.
- **Local guardrail adjustments**: a tracked
  `.claude/hooks/policy.local.json` (spec 0050) has its exceptions and
  extra rules listed in the report for review. No rule reads it in
  Phase 1; A-02 (#103) will judge what it lets through.
  `.claude/settings.local.json` is never read (spec 0051).
- **Update bots** (D-02):
  - Dependabot (`.github/dependabot.yml` or `.yaml`): each component
    has an `updates` entry with its ecosystem (go: `gomod`; python:
    `pip` or `uv`; typescript: `npm` or `bun`) whose `directory` or
    `directories` (with Dependabot's globs) covers the component's
    path. One entry has ecosystem `github-actions`. Automerge is enabled
    when a workflow runs `gh pr merge` with `--auto`.
  - Renovate: `renovate.json`, `.github/renovate.json`,
    `.gitlab/renovate.json` (contract 1.1.0), `.renovaterc`,
    `.renovaterc.json`, the JSON5 forms read as JWCC with the existing
    `hujson` dependency (a file it cannot read makes D-02 `cannot
    decide`), or the `renovate` key of a root `package.json`. Coverage
    holds unless `enabledManagers` leaves out a component's manager or
    `github-actions`. Automerge is enabled by any `automerge: true`, or
    by an `extends` preset whose name contains "automerge", except
    Renovate's built-in presets that do not enable it:
    `:automergeDisabled` (sets `automerge: false`), `:automergeBranch`,
    `:automergePr` and `:automergeRequireAllStatusChecks` (they only
    choose how automerge works), each also as `default:<name>`. A
    third-party preset of the same name still counts (amended during
    implementation, approved 2026-10-17: the name match alone failed a
    repository that turns automerge off). Other presets are not
    resolved.
  - When the C rules read GitLab CI (contract 1.1.0), coverage needs
    Renovate's `gitlabci` manager instead of `github-actions`, and
    Dependabot does not satisfy D-02 (spec 0051, after the D table).
- **R-01 (static part)**: no entrypoint, hook command or CI step calls
  `vibe` (a simple command whose first word is `vibe`), except in a job
  that satisfies C-04. #99 does not list it, but it is static, so this
  spec includes it. Hook commands are read as the H and A rules read
  them (same files, `extends` merging, `entry` with `args`, strict
  settings), for every Git hook of githooks(5) a recognized manager
  configures (husky: `.husky/<hook>`) and every Claude Code hook event.
  Since contract 1.1.0, `vibe --version` in a CI job that calls no
  entrypoint is not a call, on any provider; on GitLab CI the CI part
  reads `before_script`, `script` and `after_script` (spec 0051, after
  the R table).
- **R-02**: `AGENTS.md` contains each `verify` command as a substring,
  and `CLAUDE.md` contains the string `AGENTS.md`.
- **R-03**: every tracked pull request template
  (`pull_request_template.md` in the root, `.github/` or `docs/`, case
  insensitive, or any file under `.github/PULL_REQUEST_TEMPLATE/`, or,
  since contract 1.1.0, any `.md` file directly under
  `.gitlab/merge_request_templates/`; spec 0051, after the R table) has
  an ATX heading containing "verif", case-insensitive.

### Report (#100)

The ticket's done-when names `vibe-audit-output-example.txt` from the
contract draft. No such file exists in the repository or the drafts, so
this spec fixes the format, and a golden file committed with the tests
replaces the example. The shape of the text report, with illustrative
content:

```text
vibe audit: contract 1.0.0 (vibe v0.12.0), static checks only

  V  Verification entrypoints        1 pass, 3 not run
  H  Local Git hooks                 1 pass, 1 fail
  C  CI                              4 pass, 1 waived
  A  Agent guardrails                2 pass, 1 FAIL, 5 not run
  D  Dependencies and supply chain   2 pass, 1 not run
  R  Removability and hygiene        2 pass, 1 not run

Failed
  A-01  MUST    Shell commands and file edits pass through the guard
        .claude/settings.json: no PreToolUse hook matches MultiEdit.
        Fix: add MultiEdit to the matcher of the hook that runs the guard.
  H-02  SHOULD  pre-commit checks formatting; pre-push runs verification
        lefthook.yml: pre-push calls neither verify-fast (task verify:fast) nor verify.
        Fix: make pre-push run `task verify:fast`.

Waived
  C-05  SHOULD  until 2026-12-31: lint is platform-independent

Not applicable
  (none)

Not run (--static)
  V-01 V-02 V-03 A-02 A-03 A-05 (probe) A-07 (execution) A-08 D-01 R-01 (execution)

Guard exceptions
  (none)

Local guardrail adjustments
  .claude/hooks/policy.local.json: exception git-push-force "^git push --force origin feat/" (rebasing feature branches)

Verdict: cannot decide. 10 rules did not run (--static); 1 MUST rule already fails.
```

- The sample is illustrative. Category long names follow spec 0051's
  headings, so R is "Removability and repository hygiene" (amended
  during implementation, approved 2026-10-16).
- The category line counts results; a failed MUST rule is in capitals.
  Sections with nothing in them print "(none)". Failures are listed
  MUST first, then by rule ID.
- Every finding names a file: the one to change, or the one to create
  (`vibe.yaml`, `.github/workflows/`, `.claude/settings.json`), and a
  fix direction. A failure with no file named is a defect.
- The verdict line is one of "Verdict: conformant", "Verdict: not
  conformant", "Verdict: cannot decide. <reason>" (Q14).
- **JSON** (`--json`, stdout, nothing else on stdout):

  ```json
  {
    "schema": 1,
    "vibe": "v0.12.0",
    "contract": {"declared": "1.0.0", "audited": "1.0.0"},
    "static_only": true,
    "verdict": "undecided",
    "reason": "10 rules did not run (--static)",
    "summary": {"MUST": {"pass": 0, "fail": 0, "not_applicable": 0, "waived": 0, "undecided": 0, "not_run": 0},
                "SHOULD": {"pass": 0, "fail": 0, "not_applicable": 0, "waived": 0, "undecided": 0, "not_run": 0}},
    "rules": [
      {"id": "A-01", "level": "MUST", "category": "A", "title": "…",
       "result": "fail",
       "parts": [{"check": "static", "result": "fail"}],
       "findings": [{"file": ".claude/settings.json", "message": "…", "fix": "…"}],
       "waiver": null}
    ],
    "guard_exceptions": [{"rule": "…", "match": "…", "reason": "…"}],
    "local_adjustments": [{"file": "…", "kind": "exception", "rule": "…", "pattern": "…", "reason": "…"}]
  }
  ```

  Result tokens are `pass`, `fail`, `not_applicable`, `waived`,
  `undecided`, `not_run`; verdict tokens are `conformant`,
  `not_conformant`, `undecided`. `waiver` is null or
  `{"reason", "until", "expired"}`. `rules` lists all 25, in ID order.
  `schema` versions the JSON shape, separately from the contract.
- Configuration errors go to stderr, with exit 1, and no report.
- **Exit codes**: 0 conformant; 2 not conformant; 1 cannot decide,
  invalid configuration, unsupported contract version, unknown
  `--explain` ID, or any tool error. Exit 3 does not exist in the new
  engine.

### This repository (#101)

- **Configuration.** This repository keeps its v1 `vibe.yaml` for the
  old audit and commits its v2 configuration as `vibe.v2.yaml`; #111
  renames it. Expected content: contract `1.0.0`; platforms `linux`,
  `windows`; agents `claude`, `codex`; no `components` (one Go
  component at the root); `verify` as the list of the tasks CI runs
  (`task fmt:check`, `task typecheck`, `task lint`, `task test`,
  `task mod:verify`, `task security`, `task workflows:lint`);
  `verify-fast` `task verify:fast`; `fmt` `task fmt`; `fmt-check`
  `task fmt:check`.
- **The job.** A new, repository-owned workflow (not the generated
  `conformance.yml`, so that adopters do not receive it) has one job,
  "Contract / audit (static)". It is not a required check. It runs on
  pull requests and pushes to `main`, builds `vibe` from source as the
  conformance job does, runs `vibe audit --static --config vibe.v2.yaml`
  and prints the text report. It passes when the JSON shows no MUST rule
  with `fail` or `undecided`; `not_run` is expected under `--static`.
  The ticket says "a job in the conformance workflow"; a separate file
  keeps the generated workflow unchanged for adopters. The file is
  `.github/workflows/contract-audit.yml`, workflow "Contract", job
  "Contract / audit (static)" (amended during implementation, approved
  2026-10-16).
- **Static MUST failures to fix.** A static run today would fail
  these. Managed files change through their module templates and
  `vibe sync`; other files are edited directly.
  - A-01: the generated PreToolUse matcher gains `MultiEdit`.
  - A-04: the generated `policy.json` becomes version 2 with
    `exceptions: []`. The Go, Python and Node guard templates accept
    version 2. A version 2 `policy.json` with a non-empty `exceptions`
    makes them refuse every call, with a message that points to
    `policy.local.json` until #105 implements contract exceptions.
    `policy.local.json` keeps version 1 (spec 0050).
  - A-05: the generated Stop hook command calls `task verify:fast` as a
    simple command. It keeps spec 0023's behaviour: exit 2 once on
    failure, exit 0 when `stop_hook_active` is true. It works in the
    shells Claude Code runs hooks in on Linux and Windows.
  - C-02: the `@latest` installs in `examples.yml` and `hook-guard.yml`
    are pinned to the versions `ci.yml` already uses.
  - C-03: `codeql.yml` and `release.yml` move their write scopes from
    top level to the job that needs them.
  - C-04 is satisfied by the new job.
  - R-01 (amended during implementation, approved 2026-10-16; the new
    job alone does not satisfy it): `examples.yml`'s monorepo-gitlab job
    ran a bare `vibe sync`. It now calls the binary it built from source
    by path, `"$RUNNER_TEMP/bin/vibe" sync`, which is not a call of
    `vibe`.
- **SHOULD results stay as they are.** The expected SHOULD failures
  (H-02, C-05, A-06, D-03, R-02) are listed in the record below, not
  waived to look clean.
- **Disagreement record.** The PR adds a section "Side by side" to this
  spec: for each rule that fails or cannot be decided, and for each
  managed file whose old-audit state differs, what the old and new
  audits say and why.

### Documentation

- ADR 0026: `mvdan.cc/sh` as a production dependency of `vibe`.
- `docs/usage.md`: the new `vibe audit` flags, exit codes and JSON, and
  that a `contract:` configuration selects the new engine.
- `docs/architecture/overview.md`: `internal/contract` and its
  dependency direction (it imports none of the old packages).
- `GLOSSARY.md`: "Level" says "non-compliant", a term the glossary
  avoids; it becomes "not conformant".

## Testing Decisions

- **One seam: the `vibe audit` command, in-process.** Tests build the
  root command, point `--repo-root` (and `--config`) at a fixture
  repository and assert three things: the exit code, the rule results
  in `--json`, and, for whole-report cases, the text against a golden
  file. No test calls a check function directly. Prior art: the old
  audit's tests run `NewRootCmd` in-process against temporary
  repositories; `ExitCode` has its own table tests.
- **Fixtures are Git repositories.** A helper copies a testdata tree
  into a temporary directory, runs `git init` and `git add`, and
  applies per-test file overrides. A baseline fixture passes every
  static rule. Each rule's failing fixture is the baseline plus one
  change, so a test shows that one change flips one rule. Prior art:
  the CLI tests that already run `git` in temporary directories, and
  the guard corpus's table-driven cases.
- **Coverage.** Every rule with a static part has at least one passing
  and one failing fixture (#99). Each recognized form (each hook
  manager, Dependabot and Renovate, matrix expansion, reusable
  workflows, branch filters) has a passing case. Not applicable,
  waived, expired waiver, unused waiver, not run and cannot decide each
  have a case, as do all three exit codes and the
  failed-plus-undecided combination.
- **Configuration errors** go through the same seam: one case per
  validation rule, asserting exit 1 and the field path in stderr (#98).
- **Report.** Golden text for the baseline, for a mixed report and for
  a `--static` run. A test walks every failing fixture and asserts that
  each finding names a file and a fix (#100). `--explain` is tested for
  every rule ID and for an unknown one.
- **Goldens are LF.** The repository's CRLF hazard means a golden
  checked out with CRLF would compare wrong on Windows; testdata is
  covered by `.gitattributes`, and the comparison does not normalize
  line endings silently.
- **The old audit's tests stay unchanged and pass.** That is the check
  that the dispatch did not change it.
- **Template changes** (A-01, A-04, A-05) keep the existing module and
  guard-corpus tests passing on all three guard implementations, with a
  new case for the v2 policy, including a non-empty `exceptions`.
- **Real integration**: the new CI job on this spec's pull request. It
  is a separate ledger line from the unit tests.

## Out of Scope

- Execution checks and probes: V-01, V-02, V-03, D-01, A-02, A-03,
  A-08, and the probe and execution parts of A-05, A-07 and R-01
  (#102–#104, #106). They report "not run" or "cannot decide".
- The probe copy and planted defects.
- The reference guard `vibe-guard`, shell-parsing guard rules, and
  contract-shaped guard exceptions in the guards (#105). Phase 1 only
  makes the guards accept a version 2 policy with no exceptions.
- `vibe unlock` (#106).
- Making the new audit a required check, retiring the old audit, and
  renaming `vibe.v2.yaml` (#111).
- Removing the old system, and `vibe init` writing v2 (#112).
- A migration tool from the v1 schema (#98: no external users).
- Fixing this repository's SHOULD failures.
- Recognizing more tools, CI providers, scanner actions, Renovate
  presets or `docker://` digests. Each is a later MINOR release.
- Reading `continue-on-error` or `if:` conditions in workflows.

## Further Notes

- **A-07's execution part has no ticket.** #102 lists V-03, D-01 and
  R-01. A-07's "run the SessionStart hook in the probe copy" should be
  added to #102.
- **R-01's static part** is included here although #99 does not list
  it (see "Recognition rules").
- **A failed MUST rule beside an undecided one gives exit 1.** That is
  spec 0051's wording ("If any MUST rule cannot be decided, there is no
  verdict"). The tickets' "2 a MUST rule failed" could be read the other
  way; this spec follows spec 0051.
- **Matcher semantics are an assumption.** The audit reads a Claude
  Code matcher as an anchored regular expression. The guard policy code
  says Claude Code matches unanchored, which would make `Write|Edit`
  cover `MultiEdit`. Neither is verified against Claude Code. The
  anchored reading can only fail a repository that may be fine, never
  pass one that is broken.
- **Interpretations beyond spec 0051**, approved with this spec: a
  missing entrypoint is a V-04 failure, not a configuration error; an
  entrypoint command must be one simple command; a newer PATCH is
  audited; the A rules are not applicable without `claude` in `agents`;
  trigger filters with `paths` make a workflow not count; the default
  branch falls back to `main` or `master`.

## Side by side

The old and new audits on this repository, as recorded on 2026-10-16
for #101 and for #111's cutover.

- **Old audit** (`vibe audit`, v1 `vibe.yaml`): 22 resources checked,
  0 drifted, 0 out of date, 0 conflicts, conformant, exit 0.
- **New audit** (`vibe audit --static --config vibe.v2.yaml`): no MUST
  rule fails or is undecided; "Verdict: cannot decide. 10 rules did not
  run (--static)", exit 1. Five SHOULD rules fail.

### Before the #101 fixes

At 08b0433 the old audit said conformant while a static run of the new
audit would have failed seven MUST rules. The old audit compares
managed files with their templates, so a property the templates lack,
or a file it does not manage, is invisible to it.

| Rule | New audit | Why the old audit did not see it |
|---|---|---|
| A-01 | No PreToolUse hook matched `MultiEdit` | The template had the same matcher |
| A-04 | `policy.json` was version 1 | The template rendered version 1 |
| A-05 | The Stop hook ran `task -x hook:done`, not `task verify:fast` | The template had the same command |
| C-02 | `@latest` installs in `examples.yml` and `hook-guard.yml` | Neither workflow is managed |
| C-03 | Write scopes at top level in `codeql.yml` and `release.yml` | Neither workflow is managed |
| C-04 | No job ran `vibe audit`; `conformance.yml` runs `task audit` | The old audit has no such rule |
| R-01 | `examples.yml` ran a bare `vibe sync` | Not managed; no such rule |

All seven pass now. A-01, A-04 and A-05 changed through the templates and
`vibe sync`, so the old audit still reports no drift.

### Rules that fail now (SHOULD)

The old audit reports every file named here as in sync with its
template, or does not read it. These are recommendations the templates
do not meet, and they are left as they are (see "This repository").

| Rule | New audit | Why |
|---|---|---|
| H-02 | `lefthook.yml`: pre-commit calls neither `task fmt:check` nor `task fmt`; pre-push calls neither `task verify:fast` nor `task verify` | The hooks run the tools directly (`gofmt -l`, `golangci-lint run`, `go test ./...`), which is not "calling" an entrypoint |
| C-05 | `task lint`, `task mod:verify`, `task security` and `task workflows:lint` run on no Windows runner | `ci.yml` runs only `typecheck` and `test` on a Windows matrix; `fmt:check` reaches Windows through `examples.yml` |
| A-06 | No PostToolUse hook that matches `Write` and `Edit` calls `task fmt` | The generated hook runs `task -x hook:format` |
| D-03 | No workflow step runs a recognized scanner | `govulncheck` runs inside `task security`; the step's first word is `task`, and recognition reads only the step |
| R-02 | `AGENTS.md` names none of the seven `verify` commands | It names `task verify`, which is not this repository's `verify` entrypoint (a list of seven tasks) |

### Rules not run (`--static`)

MUST: V-01, V-02, V-03, A-02, A-03, A-05 (probe), A-08, D-01, R-01
(execution). SHOULD: A-07 (execution). They give no verdict until
#102–#104 and #106. The old audit's nearest equivalents are that the
guard files and the verification tasks match their templates; it never
runs them.

### Managed files

No managed file's old-audit state differs: all 22 resources are in
sync. The new audit does not read `.vibe/state.yaml` or template
hashes. What #111's cutover changes:

- A hand edit to a managed file that keeps every property passes the
  new audit; the old audit reports it as drift (exit 2).
- Files the old audit never read are judged: every workflow,
  `AGENTS.md`, the pull request template and the update bot
  configuration.
- A property the templates lack fails the new audit even when every
  managed file is in sync, as the seven MUST failures above did.

## Acceptance Criteria

1. `vibe audit` on a v2 configuration runs the new engine, and on a v1
   `vibe.yaml` runs the old audit with unchanged output and exit codes.
2. Every invalid `vibe.yaml` v2 exits 1 with an error naming the field
   path and the reason (#98).
3. The registry holds all 25 rules. `--explain` works for each of them.
4. Every rule with a static part has a passing and a failing fixture
   test through the `vibe audit` seam (#99).
5. The report matches the committed goldens. Every failing finding
   names a file and a fix direction (#100).
6. Exit codes are 0, 1 and 2 as decided above, including `--static`
   (always 1) and the failed-plus-undecided case.
7. `internal/contract` imports none of the old packages.
8. In this repository, `vibe audit --static --config vibe.v2.yaml`
   reports no MUST rule as `fail` or `undecided` (#101).
9. The "Contract / audit (static)" job runs on this spec's pull request
   and passes. It is not a required check.
10. This spec has a "Side by side" section recording where the old and
    new audits disagree.
11. ADR 0026 is merged, and `docs/usage.md`, the architecture overview
    and `GLOSSARY.md` are updated.
12. `task verify` and `task audit` pass.
