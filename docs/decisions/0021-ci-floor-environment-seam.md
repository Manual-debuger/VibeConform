# ADR 0021: The CI floor, and a `default:`-only include for GitLab

## Status

Accepted. Implemented per `docs/specs/0040-ci-floor-environment-seam.md`.
Amends ADR 0019 decision 4.

## Context

ADR 0019 made every generated GitLab job unchangeable by the project:
the child pipeline can add jobs but not touch parent ones, and
`include: local` was rejected because an included file deep-merges into
jobs of the same name. That protected the jobs the standard requires,
but also settings that are only the runner's business. Issue #65 is the
first case: the project could not stop merge request pipelines from
re-uploading an unchanged cache. Runner `tags` on a self-managed GitLab
are the next.

The byte-for-byte guard was never absolute. Project CI/CD variables
override anything in the file, and branch protection is outside
VibeConform (spec 0022). It stops accidental drift, not a determined
maintainer.

## Decision

1. **A required job has a floor.** Contract keys decide what the job
   runs and whether its result counts: `stage`, `image`, `needs`,
   `rules`, `allow_failure`, `before_script`, `script`,
   `interruptible`, the declared `variables`, and the cache key and
   paths. They stay generated and declared on every job. Environment
   keys decide where and how fast it runs: runner `tags`, `retry`,
   `services`, `artifacts`, `after_script`, `hooks`, `id_tokens` and
   the cache policy. The project owns those.
2. **A `default:`-only include.** `.gitlab-ci.yml` includes the
   project-owned `.gitlab-ci.defaults.yml` when it exists. GitLab's
   rule that a job's own keyword beats `default:` means the file can set
   only what a required job leaves undeclared, which is the environment
   keys. The `local` trigger job declares `inherit: default: false`.
3. **The guard moves from bytes to shape.** `vibe audit` reads
   `.gitlab-ci.defaults.yml` and requires its only top-level key to be
   `default`. A job, `variables`, `include` or anything else there would
   reopen the merge hole ADR 0019 closed, so it is a conflict. Audit
   does not check what is under `default:`. This is the first check of
   a file VibeConform does not own. It goes through a new optional
   module interface, `ProjectFileGuard`, and never writes the file
   (ADR 0003).
4. **Declared environment keys become variables.** The cache policy
   sits inside the declared `cache:`, so it becomes
   `$VIBE_CACHE_POLICY` (default `pull-push`), which a project overrides
   in its CI/CD settings. Setting `VIBE_MR_CACHE_POLICY=pull` makes merge
   request pipelines pull-only, which is issue #65.

## Consequences

- With no defaults file and no new variable, every job behaves as
  before.
- Known gaps, documented rather than closed: project CI/CD variables
  can still change tool behaviour; `default: retry` can turn a flaky
  failure into a pass; `hooks` and `services` share a required job's
  environment.
- Opting in to `pull` on merge requests can mean no cache at all for a
  Developer's pipelines, because only Maintainer/Owner or protected-ref
  pipelines use `-protected` keys. That is why it is opt-in.
- GitHub is unchanged. A project there already adds workflows freely.
  A runner-selection seam waits for an adopter who needs it.

## Alternatives considered

- **Rules over a project-owned `.gitlab-ci.yml`.** VibeConform would
  check that a job runs `task <id>:verify` and is not allowed to fail.
  Static checks of CI YAML are easy to defeat (`|| true`, `when:
  manual`, a workflow rule that never matches), and fixes such as #64
  would stop reaching adopters.
- **`vibe.yaml` keys for each environment setting.** Each new runner
  setting would need a manifest key and a release.
- **Variables for everything, no include.** These cover only the
  settings someone thought of ahead of time.
- **An image-registry prefix variable.** Deferred until an adopter needs
  it. Runner-level Docker mirrors already cover Docker Hub.
