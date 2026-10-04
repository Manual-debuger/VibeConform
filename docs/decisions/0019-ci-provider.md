# ADR 0019: CI provider as a core setting, and the GitLab child-pipeline seam

## Status

Accepted. Implemented per `docs/specs/0038-ci-provider.md`. Narrows spec
0026's non-goal "no core deselection".

## Context

Every standard generated GitHub Actions. A `prod-mono` adopter on
GitLab had a red `task audit` (four missing `.github/` files) and a red
`task verify` (`actionlint` found no workflows). Their hand-written
`.gitlab-ci.yml` had no generated part and no seam for their own jobs.

## Decision

1. **`ci.provider` is a single-valued core setting, not an integration.**
   It lives in a new top-level `ci:` map (ADR 0014's shape). Its values
   are `github` (the default), `gitlab` and `none`. It is the first
   setting with a default, so an absent `ci:` resolves byte for byte as
   before. v1 offers it in `prod-mono` only. Other standards reject the
   key.
2. **The options resolve nothing.** `githubmono`, `gitlabmono`,
   `vibe-conformance` and `monorepotooling` read `module.CIProvider`.
   This keeps pruning on spec 0026's existing path: the trial selection
   of a deselected provider finds every file that provider would write.
3. **GitLab files mirror the Task layout.** `.gitlab-ci.yml` is
   generated. `.gitlab-ci.vibe.yml` holds the only job that runs `vibe`.
   `.gitlab-ci.local.yml` is the project's own and is never written.
4. **The seam is a child pipeline.** A generated `local` job triggers
   `.gitlab-ci.local.yml` with `strategy: mirror` when it exists.
   - A child pipeline is separate configuration, so it cannot
     deep-merge into a generated job to add `allow_failure`, variables or
     `rules: when: never`. This is the property ADR 0009 got from Task's
     collision error.
   - `include: local` would allow exactly those merges. It was rejected.
5. **Hardening.** Every generated job declares `stage`, `image`, `needs`,
   `rules`, `allow_failure: false`, `before_script`, `script` and
   `interruptible`. Even the conformance include (the one included file)
   then cannot change how a generated job runs.
6. **`none` narrows spec 0026.** It is an explicit line in `vibe.yaml`,
   and `verify`, `verify-ci` and `task audit` stay generated for any CI
   to call. Branch protection was never VibeConform's to guarantee
   (spec 0022), so `none` gives up a convenience, not a guarantee.

## Consequences

- Under `gitlab` and `none`, there is no `workflows:lint`, and
  `actionlint` is not required.
- Component ids that are GitLab keywords (`default`, `include`,
  `stages`, `variables`, `workflow`, `image`, `services`, `cache`,
  `pages`) are refused under `gitlab` only.
- Project jobs in the child pipeline cannot `needs:` generated jobs or
  share their artifacts.
- Requires GitLab 18.2 or newer (`strategy: mirror`).
- Known gaps: no Windows job, no dependency bot, no offline lint of the
  GitLab configuration, and image tags rather than digests.
- `TASK_SHA256` must change with `TASK_VERSION`. The Node and uv images
  install Task from its release archive.

## Alternatives considered

- **One `ci-mono` module switching internally.** It would rename the
  module that today's reports print. Separate packages follow spec
  0010's `ci/gitlab` note.
- **`policy: {ci_provider: ...}`.** CI hosting is not a repository policy.
- **A reservation of GitLab keywords in the manifest.** It would newly
  reject valid GitHub repositories.
- **Installing Go in every GitLab job to `go install` Task.** Heavier,
  and it puts two toolchains in each job.
