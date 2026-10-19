# Spec 0055: Contract 1.1.0, GitLab CI

Status: draft (2026-10-19). Spec issue #122. Issues #74 (GitLab CI in the C rules) and
#120 Part A (R-01 exempts tool-installation tests). Builds on spec 0051
(contract v1, amended to 1.0.1), spec 0052 (static checks and their
recognition rules), spec 0053, spec 0054 (setup skill, drift test, pilot),
ADR 0025 and ADR 0026. Evidence: `docs/reports/contract-v1-pilot.md`, P1
and findings F-09 and F-12 to F-14. Settled with the user on 2026-10-19
in a grilling session; "Qn" below refers to its questions. Where this
spec and spec 0051 disagree, spec 0051 wins until this spec's changes to
it are merged.

## Problem Statement

Contract 1.0.1 reads GitHub Actions only. A repository whose CI is
GitLab CI gets "cannot decide" on C-01 to C-04 and D-03, so it can never
be conformant, whatever its CI does. The user's team works on GitLab:
pilot P1 (a Go, TypeScript and Python monorepo with GitLab CI) passed
every MUST rule the audit could read and still had no verdict.

The pilot also found three smaller GitLab gaps:

- R-03 does not recognize `.gitlab/merge_request_templates/*.md`, so P1
  failed R-03 although its template has `## Verification` (F-13).
- D-02's fix text tells a GitLab repository to add
  `.github/dependabot.yml`, and its check demands `github-actions`
  coverage, which a GitLab repository cannot have (F-14).
- The setup skill keeps a team's GitLab CI but has no GitLab reference
  CI; `migrate.md` carries a hand-written audit job that no contract
  rule has checked and that has never run (F-09).

Separately, R-01 is broader than its purpose (#120 Part A). Its purpose
is that a repository's own checks work without `vibe`. It also forbids
CI jobs that exist to test that `vibe` installs, such as this
repository's `release.yml` and `setup-action.yml`. Today those
workflows run the binary by path to get past R-01, which only works
because of the gap #120 Part B closes later.

## Solution

Contract 1.1.0, a MINOR release:

- The C rules, D-03 and R-01's CI part read GitLab CI when a repository
  has no GitHub Actions workflows. Each rule keeps its meaning; this
  spec fixes how it reads `.gitlab-ci.yml`, its local includes and its
  child pipelines. C-03 is not applicable on GitLab CI.
- R-03 recognizes GitLab merge request templates.
- D-02 asks a GitLab-CI repository for Renovate covering `gitlabci`
  instead of `github-actions`, and its fix text names Renovate.
- R-01 no longer counts `vibe --version` in a CI job that calls no
  entrypoint.
- `vibe` implements 1.1.0 and audits every repository that declares a
  1.x contract with it. A repository that was conformant under 1.0.1
  stays conformant.
- The setup skill ships a GitLab reference CI for each stack, writes
  `contract: 1.1.0`, and its drift tests cover the GitLab references.
- A real GitLab pipeline runs the audit: on the P1 merge request in
  `meeting-summarizer`, and on a scratch gitlab.com project that uses
  the reference CI.

## User Stories

### Adopters on GitLab

1. As a team lead whose repository uses GitLab CI, I want `vibe audit`
   to decide C-01 to C-04, so that my repository can be conformant.
2. As a team lead, I want the audit to read my `.gitlab-ci.yml` the way
   GitLab does (anchors, `extends:`, `default:`, `!reference`, local
   includes), so that a job defined through templates counts.
3. As a monorepo maintainer, I want verify jobs in a child pipeline
   that the parent waits for (`strategy: depend`) to count for C-01, so
   that I can keep one child pipeline per component area.
4. As a monorepo maintainer, I want a child pipeline that the parent
   does not wait for not to count, so that a red child that cannot
   block a merge request is not reported as gating it.
5. As a team lead, I want the audit to evaluate `workflow:rules`, job
   `rules:` and `only`/`except` for merge request pipelines and for
   pushes to the default branch, so that a job gated to tags or to
   schedules does not count as running on merge requests.
6. As a team lead, I want a `rules:` expression the audit cannot
   evaluate to be quoted in the finding, so that I know which line to
   change or why the job did not count.
7. As a team lead, I want a job with no rules to count for merge
   requests and for default-branch pushes, because GitLab runs it in
   every branch pipeline and shows that pipeline on the merge request.
8. As a team lead, I want a job whose rule uses `changes:` or `exists:`
   not to count for C-01, like a GitHub `paths` filter, so that C-01
   still promises that verify runs on every merge request.
9. As a security reviewer, I want every `image:` and `services:` entry
   pinned by digest, so that a retagged image cannot change what CI
   runs (C-02).
10. As a security reviewer, I want every `include:` that the audit does
    not follow pinned (a commit SHA for `project:` and `component:`,
    `integrity:` for `remote:`), and `template:` includes reported as
    unpinnable, so that C-02 closes the same hole it closes for actions.
11. As a security reviewer, I want `@latest` in a GitLab `script` to
    fail C-02, as it does in a GitHub `run:` step.
12. As a team lead, I want C-03 reported as not applicable on GitLab
    CI, with the reason, so that I know the job token is a project
    setting the audit cannot read, and the setup skill tells me which
    setting to keep off.
13. As a team lead, I want the `conformance:audit` job to satisfy C-04
    when it runs in merge request or branch pipelines, runs `vibe audit`
    and calls no verify command.
14. As a team lead who tests on Windows, I want a job with a
    `windows` runner tag recognized as Windows, a `linux` tag or an
    untagged job with an `image:` as Linux, and `parallel: matrix`
    tags expanded (C-05).
15. As a team lead, I want GitLab's own dependency scanning (the
    Dependency-Scanning template or CI/CD component) to satisfy D-03,
    as well as the scanner commands GitHub repositories use.
16. As a team lead, I want my `.gitlab/merge_request_templates/*.md` to
    satisfy R-03 when it has a verification heading.
17. As a team lead, I want D-02 to accept Renovate covering my
    components and the `gitlabci` manager, and its fix text to name
    Renovate, not Dependabot.
18. As a team lead, I want R-01 to read my GitLab `script`,
    `before_script` and `after_script`, except in the C-04 audit job,
    so that a job that needs `vibe` is found.
19. As a team lead, I want the audit to say which CI provider it read,
    so that a repository with both GitHub and GitLab files knows which
    one decided the C rules.

### Adopters on GitHub

20. As a maintainer of a conformant GitHub repository, I want the same
    verdict under 1.1.0, so that upgrading `vibe` changes nothing for
    me.
21. As a maintainer of a GitHub repository with a GitLab mirror file, I
    want the audit to keep reading GitHub only, so that the mirror
    cannot fail my C rules.
22. As a maintainer who tests that `vibe` installs, I want
    `vibe --version` in a CI job that calls no entrypoint not to fail
    R-01, so that I can check the installed binary by name.
23. As a maintainer, I want `vibe --version` in an entrypoint, a hook or
    a job that calls an entrypoint still to fail R-01, because there it
    makes my checks need `vibe`.

### The setup skill

24. As an agent running the setup skill on a GitLab repository, I want
    a reference `.gitlab-ci.yml` for the repository's stack, so that I
    can make the team's jobs call the entrypoints and add the audit job
    without inventing them.
25. As an agent, I want `rules.md` to say how each C rule, D-02, D-03
    and R-03 reads GitLab CI, so that I can fix a finding without
    reading the audit's code.
26. As an agent, I want `migrate.md` to point to the reference GitLab
    audit job instead of carrying its own copy, so that the two cannot
    drift.
27. As an agent, I want `migrate.md` to tell me how to keep a monorepo's
    child pipeline (`strategy: depend`) instead of always merging it
    into `.gitlab-ci.yml`.
28. As an agent, I want the skill to write `contract: 1.1.0`, so that a
    GitLab repository set up now is decided.
29. As an agent, I want the skill to tell the person which GitLab
    project settings the audit cannot read (pipelines must succeed,
    protected branches, job-token push) and what to set them to.

### Maintainers of VibeConform

30. As the maintainer, I want the drift test to fail when a GitLab
    reference CI stops passing the static audit, so that the skill and
    the contract cannot drift apart.
31. As the maintainer, I want the GitLab rules tested through the
    `vibe audit` seam with fixture repositories, one change per case,
    as the GitHub C rules are.
32. As the maintainer, I want a real GitLab pipeline to have run the
    reference CI and the audit before the release, so that "decided"
    does not rest on fixtures alone.
33. As the maintainer, I want `release.yml` and `setup-action.yml` to
    call `vibe --version` by name again, so that they test what a user
    types.
34. As the maintainer, I want spec 0051's Changes to explain why each
    1.1.0 change is MINOR, so that the versioning rule stays auditable.

## Implementation Decisions

### Contract text (spec 0051)

- Spec 0051 becomes "Current version: 1.1.0". The C section says the C
  rules read GitHub Actions or GitLab CI, each rule's table row keeps
  its GitHub wording, and a GitLab column or paragraph per rule states
  the GitLab reading below. "Later versions" drops the GitLab item.
- Changes gets a **1.1.0** entry listing each change in this spec and
  why it is MINOR: a newly recognized CI provider, a newly recognized
  template location, update bot manager and scanner, and a relaxed R-01.
  It also states the one verdict that can move from undecided to not
  conformant: a GitLab-only repository whose CI now fails a C rule or
  R-01's CI part (Q14). No conformant verdict becomes non-conformant.
- `contract.Implemented` becomes 1.1.0. Repositories declaring 1.0.0 or
  1.0.1 are audited with 1.1.0 (spec 0051, "Versioning").

### Which provider is read (Q1)

- GitHub wins: when tracked `.github/workflows/*.yml` files exist, the C
  rules, D-03 and R-01 read them only, exactly as in 1.0.1. GitLab CI is
  read when there are none and `.gitlab-ci.yml` is tracked. Other
  providers stay "cannot decide"; no CI at all still fails.
- The report names the provider the C rules read.

### The GitLab pipeline model

- The audit resolves `.gitlab-ci.yml` the way GitLab does, statically:
  YAML anchors and merge keys, `include: local` (including GitLab's
  wildcard paths) merged in GitLab's order, `default:` keys,
  `extends:` (deep merge, up to GitLab's nesting limit), and
  `!reference` tags that point at locally defined keys. Keys starting
  with `.` are templates, not jobs. A file that does not parse, an
  `extends:` or `!reference` that points at nothing, or a cycle fails
  every rule that needs the file, like a GitHub workflow that does not
  parse.
- **Includes (Q2).** Only `local:` is followed. `project:`, `remote:`,
  `component:` and `template:` are not followed; their jobs call
  nothing, as remote reusable workflows on GitHub. An include with
  `rules:` is followed only when those rules admit the pipeline kind
  being read.
- **Child pipelines (Q3).** A job with `trigger: include:` naming local
  files is followed when it has `strategy: depend` (or GitLab's newer
  equivalent that makes the parent mirror the child's status). The
  child's jobs are read with the trigger job's rules as their outer
  condition, as a reusable workflow is read with its caller's triggers.
  A child without such a strategy is not followed. Dynamic children (`include: artifact:`) and
  `trigger: project:` are never followed.
- **Commands (Q15).** A job's commands are its `before_script` and
  `script` lines, parsed as POSIX shell like a GitHub `run:` step.
  `after_script` is read by R-01 and D-03 but does not count as calling
  verify (its failure does not fail the job). `allow_failure` is
  ignored, as `continue-on-error` is on GitHub.
- **Pipeline kinds and rules (Q4).** The audit asks two questions of
  each job: does it run in a merge request pipeline, and does it run on
  a push to the default branch? It evaluates `workflow:rules`, then the
  job's `rules:` (first match wins, `when: never` excludes), or its
  `only`/`except` keywords. The closed list of recognized conditions:
  - `$CI_PIPELINE_SOURCE ==` or `!=` a quoted literal (`merge_request_event`,
    `push`, `schedule`, `web`, `api`, `trigger`, `pipeline`, `parent_pipeline`, …);
  - `$CI_MERGE_REQUEST_IID` and `$CI_COMMIT_BRANCH`, `$CI_COMMIT_TAG` as
    presence tests;
  - `$CI_COMMIT_BRANCH` or `$CI_COMMIT_REF_NAME` compared with
    `$CI_DEFAULT_BRANCH` or with a literal branch name, compared with
    the default branch as spec 0052 reads it;
  - `&&`, `||` and parentheses over these;
  - `only`/`except` with `merge_requests`, `branches`, `pushes`, `tags`,
    `schedules` and literal branch names.
  A rule with any other condition, or with `changes:` or `exists:`,
  does not admit the job for that pipeline kind; the finding quotes the
  condition and its line. A job with no rules and no `only`/`except`
  runs in both kinds, branch pipelines included (GitLab shows them on
  the merge request).

### The rules on GitLab CI

- **C-01**: some job (or, for a list `verify`, jobs covering every
  command) that runs in merge request pipelines calls `verify`, and
  likewise for pushes to the default branch. `changes:`-gated jobs do
  not count (Q7).
- **C-02 (Q5)**: every `image:` and `services:` entry (job and
  `default:`, after resolution; string or `name:` form) ends in
  `@sha256:` and 64 hexadecimal characters. Every unfollowed include is
  pinned: `project:` with a `ref:` of 40 hexadecimal characters,
  `component:` with a version of 40 hexadecimal characters, `remote:`
  with `integrity:`. A `template:` include cannot be pinned and fails.
  No word of a `script`, `before_script` or `after_script` simple
  command ends in `@latest`.
- **C-03 (Q6)**: not applicable on GitLab CI. Reason: GitLab job
  tokens have no per-file permissions, and job-token push access is a
  project setting the audit does not read.
- **C-04 (Q8)**: a job that runs in merge request pipelines has a
  simple command starting with `vibe audit` and calls no verify command.
  Project settings (pipelines must succeed, protected branches) are not
  read, as GitHub branch protection is not.
- **C-05 (Q9)**: a job's platforms come from its `tags:` after
  `parallel: matrix` expansion: a tag containing `windows` is Windows,
  `linux` Linux, `macos` macOS (case-insensitive). A job with no tags
  and an `image:` is Linux. Anything else is an unknown platform.
- **D-03 (Q10)**: the 1.0.1 scanner commands in any job's scripts,
  plus an include of GitLab's Dependency-Scanning template
  (`Jobs/Dependency-Scanning*.gitlab-ci.yml`,
  `Security/Dependency-Scanning*.gitlab-ci.yml`) or of the GitLab
  `dependency-scanning` CI/CD component. Recognizing a template here
  does not exempt it from C-02.
- **R-01**: reads every job's scripts except those of a job that
  satisfies C-04.

### R-01 and tool-installation tests (#120 Part A, Q13)

- On any provider, a simple command whose words are exactly
  `vibe --version` is not a call of `vibe` when it is in a CI job that
  calls none of the declared entrypoints (`verify`, `verify-fast`,
  `fmt`, `fmt-check`, `setup`). In entrypoints and hook commands, and in
  jobs that call an entrypoint, it is still a call. `vibe-guard` was
  never `vibe`. There is no `vibe version` command, so none is exempt.
- Part B (calls by path) is not changed; it ships with #50.
- This repository's `release.yml` and `setup-action.yml` call
  `vibe --version` by name again, and spec 0054's known deviation is
  marked resolved.

### R-03 (Q11)

- Every tracked `.md` file directly under
  `.gitlab/merge_request_templates/` is a pull request template, on any
  repository, checked like the GitHub templates.

### D-02 (Q12)

- When the C rules read GitLab CI, D-02 requires coverage of Renovate's
  `gitlabci` manager instead of `github-actions` (and of
  `gitlabci-include` when `enabledManagers` is set and includes are
  used). Dependabot cannot satisfy it on a GitLab repository.
- `.gitlab/renovate.json` and its JSON5 form join the recognized
  Renovate locations.
- The fix text names Renovate and `renovate.json` for GitLab
  repositories and Dependabot for GitHub ones.

### The setup skill

- `reference/ci/<stack>/` gains a `.gitlab-ci.yml` for go, python,
  typescript and monorepo: verify jobs calling the entrypoints, images
  pinned by digest, a `conformance:audit` job (the migrate.md job,
  moved), and dependency scanning. The monorepo reference shows a
  `strategy: depend` child pipeline as an option.
- `rules.md` gives the GitLab reading of C-01 to C-05, D-02, D-03 and
  R-03, and the project settings the audit does not read.
- `migrate.md` points to the reference job, and offers keeping a child
  pipeline with `strategy: depend` as the alternative to merging it.
- The skill writes `contract: 1.1.0`. This repository's `vibe.yaml`
  and the examples move to 1.1.0.

### Documentation

- `docs/usage.md` and the architecture overview describe GitLab
  reading. `GLOSSARY.md` gains any new term the implementation needs
  (for example "pipeline kind"), through `/domain-modeling`.
- #74's verification levels: this spec covers level 4 manually; the
  automated `gitlab-e2e` workflow stays deferred on #74.

## Testing Decisions

- A good test drives `vibe audit` on a fixture repository and checks
  rule results and findings, not the GitLab reader's internals.
- **Seam 1: the `vibe audit` seam.** A GitLab twin of the GitHub C-rule
  tests: a baseline `.gitlab-ci.yml` that passes C-01, C-02, C-04,
  C-05, D-03 and R-01 (C-03 not applicable), and one change per case:
  each recognized and unknown rule condition, `changes:`, `workflow:rules`,
  `extends`/`default`/`!reference`/anchors, local includes and their
  `rules:`, each unfollowed include kind pinned and unpinned, a child
  pipeline with and without `strategy: depend`, `after_script`,
  image and service digests, `@latest`, tags and matrix expansion,
  provider precedence with both GitHub and GitLab files, and parse
  failures. R-03, D-02 and #120 Part A cases go in their existing test
  files. Prior art: `internal/cli/contract_rules_ci_test.go` and the
  D-02 and R-03 audit tests.
- **Seam 2: the drift tests.** The spec 0054 drift test and the
  "reference implementations pass the static audit" test cover each
  GitLab reference CI, with planted-problem fixtures as in spec 0054.
  Prior art: `internal/cli/skill_drift_test.go`,
  `internal/cli/skill_static_audit_test.go`.
- **Seam 3: real pipelines (manual).** Recorded in the PR with links:
  (1) the P1 merge request in `meeting-summarizer` runs a pipeline
  whose `conformance:audit` job prints a decided C verdict under 1.1.0;
  (2) a scratch gitlab.com project with the monorepo reference CI runs
  green, its MR pipeline runs the audit, and C-01 to C-04 pass. No CI
  secret is added.
- A regression test covers each pilot finding fixed here (F-12 to F-14
  and #120 Part A).

## Out of Scope

- #120 Part B (calls by path count): MAJOR, ships with #50.
- Following `project:`, `remote:`, `component:` and `template:`
  includes, `trigger: project:` and dynamic child pipelines.
- Reading GitLab project settings (pipelines must succeed, protected
  branches, job-token scope) through the API.
- CI providers other than GitHub Actions and GitLab CI.
- The automated `gitlab-e2e` workflow (#74, level 4) and the CI Lint
  API (level 2).
- Changing any rule's GitHub reading, except R-01's `vibe --version`
  exemption, which applies to both.

## Further Notes

- MINOR check: every change either recognizes something new (GitLab CI,
  GitLab templates, Renovate's GitLab location and manager, GitLab
  scanning) or relaxes a check (R-01). GitHub repositories keep their
  1.0.1 reading. GitLab-only repositories were undecided, so no
  conformant verdict exists for them to lose.
- The rules evaluator's closed list is the riskiest part. Extending it
  later is MINOR (it can only make more jobs count).
- A team that wants to skip unchanged components on merge requests
  (`changes:`) gets a C-01 finding, as on GitHub with `paths`. If that
  proves too strict in practice, relaxing it is MINOR.

## Acceptance Criteria

- [ ] Spec 0051 is at 1.1.0 with the GitLab readings, the R-03, D-02
      and R-01 changes and a 1.1.0 Changes entry; spec 0052's
      recognition rules point to them.
- [ ] `vibe audit` on a GitLab-only fixture decides C-01, C-02, C-04,
      C-05 and D-03, reports C-03 not applicable, and names GitLab CI
      as the provider read.
- [ ] Each decision under "The GitLab pipeline model" and "The rules on
      GitLab CI" has a passing audit-seam test.
- [ ] A repository with both `.github/workflows/` and `.gitlab-ci.yml`
      gets the same results as under 1.0.1.
- [ ] Every audit fixture that was conformant under 1.0.1 is conformant
      under 1.1.0.
- [ ] R-03 passes on a fixture with only
      `.gitlab/merge_request_templates/Default.md` having
      `## Verification`.
- [ ] D-02 passes on a GitLab fixture with Renovate covering each
      component and `gitlabci`; its fix text on a GitLab fixture names
      Renovate.
- [ ] R-01 passes with `vibe --version` in a CI job that calls no
      entrypoint, and fails with it in an entrypoint, a hook or a job
      that calls `verify`.
- [ ] `release.yml` and `setup-action.yml` call `vibe --version` by
      name and pass the audit.
- [ ] The setup skill has a GitLab reference CI per stack, `rules.md`
      and `migrate.md` are updated, it writes `contract: 1.1.0`, and
      the drift tests cover the GitLab references.
- [ ] Real pipeline: the P1 MR's `conformance:audit` job ran and printed
      a decided C verdict (link recorded).
- [ ] Real pipeline: a scratch gitlab.com project with the monorepo
      reference CI ran green and its audit passed C-01 to C-04 (link
      recorded).
- [ ] `task verify` and `task audit` pass; documentation is synced.
