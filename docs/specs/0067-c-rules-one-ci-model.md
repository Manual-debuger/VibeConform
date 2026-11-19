# Spec 0067: Each C rule has one implementation over the CI model

Status: approved (2026-11-19). Issue #160, from the design review #167.
Builds on spec 0055 (the CI model and its GitLab CI reader) and on the
rule-test seam of ADR 0028. This is a refactor. The contract stays
1.2.0. No rule's result changes. Every finding stays byte for byte the
same, except for the two C-02 changes on GitHub Actions that are named
in "C-02".

## Problem

Spec 0055 added a provider-neutral CI model, with a GitHub Actions
reader and a GitLab CI reader. Only the GitLab checks read the model.
The GitHub checks still read the parsed workflows directly, so that
their contract 1.0.1 results and wording do not change. Because of
this, each CI rule has two implementations:

| Rule | GitHub Actions | GitLab CI |
|---|---|---|
| C-01 | reads the workflows and the trigger walk | reads `CI.Run` per pipeline kind |
| C-02 | reads `uses:` and `run:` steps | reads images, includes, scripts |
| C-03 | reads the workflows' permissions | not applicable (the registry decides), but the check compares the provider again |
| C-04 | its own audit-job search | `ciAuditJobs` over `CI.Run` |
| C-05 | GitHub runner platforms | GitLab tag platforms |
| D-03 | reads the jobs' commands | reads the scripts and includes |

Each check starts with `if ci.Provider == CIGitLab`. This is six
branches. The model is a seam with two adapters, but only the GitLab
adapter crosses it.

What this costs:

1. A change to a C rule needs two edits and two sets of tests. When the
   two paths drift apart, no test finds it.
2. The logic that both paths share is written twice. For example, C-05's
   "lacking Linux or Windows" loop and C-01's "verify command not
   called" loop.
3. The GitHub runs are decided lazily in `CI.Run`, and a map from the
   GitHub jobs to the model's jobs exists only for that. GitLab runs are
   decided when the configuration is read. `CI.Run` panics for any
   provider other than GitHub that has no run, so the model has two
   lifecycles.
4. The rule tests check findings by substring. If the wording or the
   order of a finding drifts, these tests do not find it.

## Solution

The GitHub Actions reader fills the whole CI model, as the GitLab reader
does. This includes the runs for each pipeline kind, each job's runner
platforms and the references that must be pinned. Each C rule and D-03
then has one implementation that reads only the model. The words that
differ by provider come from the model's provider wording, so each
finding keeps its exact text. Before any code moves, golden files
record the full findings of every CI rule case for both providers.
They are the regression net.

## User Stories

1. As a maintainer of `vibe`, I want each C rule written once, so that
   a change to a rule is one edit.
2. As a maintainer, I want the rules to have no provider comparisons,
   so that the rules and the readers are separated at the model.
3. As a maintainer, I want the GitHub reader to fill the same model as
   the GitLab reader, so that the model has one lifecycle.
4. As a maintainer, I want the runs of both providers decided when the
   CI is read, so that `CI.Run` is a lookup that never panics on a
   valid pipeline kind.
5. As a maintainer, I want each job's runner platforms in the model, so
   that C-05 reads one list for any provider.
6. As a maintainer, I want the references that must be pinned in the
   model, so that C-02 reads one list for any provider.
7. As a maintainer, I want the provider-specific words of findings
   (such as "workflow" or "job", "pull requests" or "merge request
   pipelines", and the trigger hint) to be in one place for each
   provider, so that the rules contain no provider wording.
8. As a maintainer, I want golden files with the full findings of each
   CI rule case, so that wording and order cannot drift silently.
9. As a maintainer, I want the golden files recorded from `main` before
   the refactor, so that they show the behavior before the change, not
   after it.
10. As a reviewer, I want a golden change to appear only for the two
    named C-02 cases, so that I can confirm that the refactor changes
    nothing else.
11. As a repository owner on GitHub Actions, I want my audit to give the
    same verdict, rule results, finding files, messages and fixes as
    before, so that the refactor does not affect me.
12. As a repository owner on GitLab CI, I want my audit to give the same
    output as before, finding order included, so that the refactor does
    not affect me.
13. As a repository owner on GitHub Actions, I want a repeated `@latest`
    word in one `run:` step reported once, as GitLab CI reports it, so
    that the two providers report the same problem the same way.
14. As a repository owner on GitHub Actions, I want C-02 to list the
    unpinned references and then the `@latest` words, as GitLab CI does,
    so that the order of the findings has one rule.
15. As a maintainer who adds a third CI provider later, I want to write
    a reader and a wording only, so that the rules need no change.
16. As a maintainer, I want C-03 to stay a rule about GitHub workflow
    permissions without a provider branch of its own, because the
    registry already makes it not applicable on GitLab CI.
17. As a maintainer, I want R-01's CI part unchanged in behavior, so
    that the `vibe` calls found and the audit job exempted stay the
    same.
18. As a maintainer, I want the code that only the old GitHub path used
    (the trigger-walk results in the checks, the audit-job search over
    workflow jobs, the job-to-model map) removed, so that the package
    has one way to answer each question.
19. As a reader of `docs/architecture/overview.md`, I want the "One CI
    model, two readers" section to say that every C rule reads the
    model, so that the documentation agrees with the code.
20. As a maintainer, I want no new complexity suppression, and the
    suppressions that the merged checks no longer need removed.
21. As a CI user of this repository, I want `task verify` and
    `task audit` to pass, so that the refactor keeps the repository
    conformant.

## Implementation Decisions

### The model

- **The GitHub reader decides the runs when it reads the CI.** It
  computes the runs for the merge request kind (pull requests) and the
  default-push kind with the existing trigger walk. The trigger walk
  follows local reusable workflows with the caller's triggers, and it
  decides the default branch as it does today. `CI.Run` becomes a
  lookup for both providers. The lazy GitHub branch and the map from
  workflow jobs to model jobs are removed. A GitHub run has no skipped
  jobs. Reasons for skipped GitHub jobs would add C-01 and C-04
  findings, which is a behavior change and out of scope.
- **Each job has its runner platforms.** `CIJob` gets the platforms the
  job runs on and the descriptions of its runners whose platform is
  unknown. Each reader fills them with its existing logic: GitHub
  `runs-on` with the matrix expanded, and GitLab `tags:` with
  `parallel: matrix` expanded and the image rule. The unknown
  descriptions keep their present text for each provider. The GitLab
  platform logic moves out of the GitLab checks into the GitLab reader.
- **The references that must be pinned are in the model.** Each job gets
  its GitHub `uses:` references, job-level and step-level, with their
  file, line and the place a finding names ("job build" or "job build,
  step 2"). GitLab jobs keep `Images`, and the CI keeps `Includes`. A
  reader fills only the kinds that its provider has. Local `./`
  references are not left out by the reader. C-02 decides about them,
  as it does today.
- **The model holds the provider wording.** Each provider supplies one
  unexported wording value. It holds every part of a finding that
  differs by provider: the phrase for each pipeline kind ("pull
  requests" or "merge request pipelines", and "pushes to <branch>"),
  the hint that says what makes a job not count, and the messages and
  fixes of C-01, C-04, C-05 and D-03 that are not the same on both
  providers. The rules take their words from it. The two providers'
  wordings are next to each other in one file, so a reviewer can
  compare them. `CI.Dir` stays the file of a finding about the CI as a
  whole.
- **`CI.Workflows` stays, for C-03 only.** C-03 reads GitHub workflow
  permissions, which no other provider has. The GitHub reader alone
  fills `CI.Workflows`. C-03 drops its provider comparison, because
  `notOnGitLab` already makes it not applicable before it runs.

### The rules

- **One implementation each** of C-01, C-02, C-04, C-05 and D-03. Each
  reads only the model and its provider wording. No rule in the C
  checks compares `ci.Provider`. Outside the C rules, C-03's
  applicability and D-02's update-bot check still ask `gitLabCI()`.
  This is correct: they read things that differ by provider and that
  the model does not cover.
- **C-01** reads `CI.Run` for each kind. When a kind has no jobs, it
  gives the "no job runs" finding. For each verify command that no job
  of the run calls, it gives the "not called" finding. Skipped jobs
  that call the command get a skip finding, which only GitLab runs have.
- **C-04** uses `ciAuditJobs` over the merge request run, for both
  providers. R-01 already uses it. The "runs vibe audit and also calls
  verify" findings, the skip findings and the fallback finding keep
  their present wording for each provider.
- **C-05** reads every job's commands and runner platforms. The
  messages that are the same on both providers today ("the verify
  command ... runs on no ... runner (called in ...)") come from one
  template. The "called by no job" message and the fix come from the
  wording.
- **D-03** reads the commands of every script of every job (on GitHub,
  these are exactly the `run:` steps) and the includes (GitHub has
  none). Its failure message comes from the wording.
- **C-02** reads, in this order: the `uses:` references (a full
  40-character commit SHA, except local `./`), the images (pinned by
  digest, with jobs grouped as today), the includes (the existing
  include decision) and the `@latest` words of every script, with one
  finding for each file, line and word. Each finding's message and fix
  stay as they are for its kind. On GitHub Actions, this changes two
  things, and they are the only changes to the output in this spec:
  1. The findings are in a different order. All unpinned `uses:`
     findings come first, then the `@latest` findings. Before, the two
     kinds were mixed step by step.
  2. A `@latest` word that is repeated in one `run:` step gives one
     finding. Before, it gave one finding for each time it occurred.
  The result of C-02 does not change.

### Code that goes

- The GitLab-only check functions merge into the one implementation of
  each rule. The GitHub-only helpers that only the old path used are
  removed: the trigger-walk command list in the checks, the audit-job
  search over workflow jobs and the workflow-job audit test. The
  trigger walk itself stays, because the GitHub reader uses it.
- No new production dependency, and no new import of the contract
  package.

### Documentation

- `docs/architecture/overview.md`, "One CI model, two readers": the
  GitHub reader fills the whole model, every C rule and D-03 reads only
  the model, the wording is per provider, and C-03 reads the GitHub
  workflows.
- The CI model's package comment changes in the same way.
- `GLOSSARY.md`: no new term. "CI provider" and "Pipeline kind" already
  exist. The wording is an implementation detail and not a domain term.

## Testing Decisions

- **One seam, which already exists.** The rule tests go through
  `contract.Audit` with the `contracttest` fixtures (ADR 0028). No test
  calls a C check or builds a CI model by hand.
- **Characterization first.** The first ticket adds golden files for
  every case of the C-01 to C-05, D-03 and R-01 tables of both
  providers. Each file holds the rule's full findings: file, message
  and fix, in report order. The ticket records them from `main`, before
  any code moves. The comparison runs with the existing case tables. A
  case fails when its findings differ from its golden. The goldens are
  rewritten with the package's existing `-update` flag. They are not
  rewritten with a second flag of the same name, because a flag that is
  defined twice in one test binary panics. The existing substring checks
  stay.
- **The two C-02 changes are made visible first.** The characterization
  ticket adds two GitHub C-02 cases that record the present behavior:
  a job with a `@latest` step before an unpinned `uses:` step, and a
  step that repeats one `@latest` word. The C-02 ticket changes exactly
  these two goldens. Its implementation notes show the diff.
- **Every other golden stays byte for byte the same** in each refactor
  ticket. A golden change outside the two C-02 cases is a regression.
  It is not a reason to run `-update`.
- **What makes a good test:** each test checks behavior that the report
  shows (results, files, messages, fixes and order). No test checks a
  field of the model or the name of a function.
- **Prior art:** `explain_golden_test.go` (goldens through the
  `-update` flag, with "review the diff" in its doc comment), the
  contract report goldens in `internal/cli`, and spec 0066's
  characterization tickets.

## Out of Scope

- Reasons for skipped GitHub jobs (which workflow trigger left a job
  out). They would add findings, so they would be a behavior change.
- D-02's update-bot check and its `gitLabCI()` branch, and any other
  rule outside C-01 to C-05 and D-03, except to keep R-01's CI part
  working.
- A third CI provider.
- Any change to a rule's result, to the contract version or to the JSON
  schema.
- Issue #163 and the guard items of #166 (the guard track).
- #166's items that do not touch the files changed here.

## Acceptance Criteria

- [ ] Goldens record the full findings of every C-01 to C-05, D-03 and
      R-01 case of both providers. They are recorded from `main` before
      the refactor, together with the two new C-02 cases.
- [ ] The GitHub reader fills the runs, the runner platforms and the
      `uses:` references when it reads the CI. `CI.Run` is a lookup,
      and the job-to-model map is gone.
- [ ] C-01, C-02, C-04, C-05 and D-03 each have one implementation that
      reads only the model and the provider wording.
- [ ] No C check compares `ci.Provider`. C-03 has no provider branch.
- [ ] Every golden is unchanged, except the two C-02 GitHub cases, which
      change as stated in "C-02". The implementation notes show their
      diff.
- [ ] Every existing rule test, the explain goldens and the contract
      report goldens pass unchanged.
- [ ] The removed helpers are gone, and no new complexity suppression is
      added. Suppressions that are no longer needed are removed.
- [ ] `docs/architecture/overview.md` and the CI model's package comment
      are updated.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.

## Further Notes

- This is the last item of #167's contract track: #164 (spec 0063),
  #159 (spec 0064), #162 with #165 (spec 0065), #161 (spec 0066), then
  this spec (#160).
- `/to-tickets` can cut this into these tickets:
  1. the characterization goldens and the two C-02 cases;
  2. the GitHub reader fills the whole model (runs at read time, runner
     platforms for both readers, `uses:` references), with no change to
     the rules' output;
  3. the provider wording and one implementation each of C-01, C-04,
     C-05 and D-03;
  4. one C-02, with the two named golden changes;
  5. C-03's branch, removal of the dead code, and documentation.
  Tickets 3 and 4 both need ticket 2, and they can run in parallel.
  Ticket 5 needs 3 and 4.
