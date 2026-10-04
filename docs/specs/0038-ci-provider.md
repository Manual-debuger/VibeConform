# Spec 0038: CI Provider Selection, and GitLab CI for `prod-mono`

Status: accepted and implemented (ADR 0019). Implementation plan:
`docs/plans/0038-ci-provider.md`. Open questions resolved 2026-10-02 with
the recommended answers: a child pipeline, `prod-mono` only, `none`
included, Linux only, a merge request template, image tags, a `ci:` map.
The minimum GitLab version is 18.2, the first with `strategy: mirror`.
Builds on spec 0010 (GitHub CI module), spec 0022 (conformance
isolation), spec 0025 (`prod-mono/v1`), spec 0026 (optional
integrations and pruning), spec 0029/ADR 0014 (single-valued options)
and ADR 0009 (extending a generated file without owning it).

## Problem

Every standard generates GitHub Actions CI, and nothing in `vibe.yaml`
can say a repository is hosted anywhere else. A real adopter shows the
cost. `prod-mono`, with Go, TypeScript and Python components, hosted on
GitLab:

1. **`task audit` is red for files they cannot use.** They deleted
   `.github/` and wrote `.gitlab-ci.yml` by hand. `task audit` now
   reports four missing managed files: `.github/workflows/ci.yml`,
   `.github/workflows/conformance.yml`, `.github/dependabot.yml` and
   `.github/pull_request_template.md`. The only way to go green is to
   restore files that no runner reads.
2. **`task verify` is red for a check that does not apply.** The
   managed root `verify` runs `workflows:lint`, which is `actionlint`.
   Without `.github/workflows/`, `actionlint` exits 3 ("no project
   found ... workflows directory"). `actionlint` is also a required tool,
   so `vibe sync` and `vibe doctor` report it missing.
3. **Their hand-written CI has no generated part, and no seam.** The
   jobs that matter to the standard (each component's `task <id>:verify`
   and the conformance audit) are hand-copied and will drift from the
   standard. The jobs that are theirs (a migration check against a
   Postgres service, contract-freshness checks, an image build) have
   nowhere to live next to generated jobs without editing a managed
   file, which `audit` would then report.

Spec 0010 already named the gap: the `ci/github` package path left
"room for a future `ci/gitlab` without a rename". Spec 0026 named the
other side: "No core deselection. Verification, hooks, CI, and
conformance are not integrations." CI must stay part of the contract.
What varies is *which* CI system carries it.

## Constraints that apply

- **Backward compatibility.** A `vibe.yaml` without the new key must
  resolve every resource byte for byte as today, in all four standards.
  `examples/` and this repository must be unchanged apart from the
  recorded `vibe_version`.
- **Principle 1.** A generated seam must not become a way to weaken the
  generated checks without anyone noticing. ADR 0009 accepted
  `Taskfile.local.yml` only because Task makes a name collision a hard
  error. The GitLab seam needs an equivalent property, or a documented
  gap.
- **Principle 2.** Generated commands go through Task. CI jobs call
  `task <id>:verify`, never a duplicated command list.
- **Principle 3.** Only the conformance pieces may run `vibe`. They must
  stay in files of their own, so that removing VibeConform means
  deleting files (spec 0022). This holds on GitLab too.
- **Spec 0026 / ADR 0013.** Selection is a pure function of
  `vibe.yaml`. Deselected output is removed only through the existing
  two-way prune path: recorded and unmodified is removed, absent is
  forgotten, modified is a conflict. Nothing unrecorded is deleted.
- **ADR 0014.** A single-valued setting is an option group under its
  own top-level map. Keys are unique across maps.
- **Spec 0025.** `prod-mono` CI is one job per component, named by the
  component's `id`. Go also runs the race detector.
- **ADR 0012.** Manifest decoding stays strict.

## Scope

### 1. `ci:` in `vibe.yaml`

A new top-level map with one single-valued key:

```yaml
ci:
  provider: gitlab   # github | gitlab | none; absent: github
```

- An absent `ci:`, or an absent `provider`, selects `github`. This is the
  first single-valued option with a default. It is also the first group
  whose default is on, so that absent means exactly today's behaviour.
- Decoding is strict: `ci: {providr: gitlab}` is an error.
- An unknown value is an error that lists the valid values:
  `ci.provider (bitbucket): unknown value (valid: github, gitlab, none)`.
- `vibe init` does not write `ci:`.

### 2. Which standards offer it (v1)

v1 offers `ci.provider` in `prod-mono/v1` only, with all three values.
`prod-go`, `prod-ts` and `prod-py` do not offer it, and their output is
unchanged. A `ci:` key in their `vibe.yaml` is an error before anything
resolves. The error names the standard and says that it offers no
provider setting. The reasons:

- The adopter that needs it is on `prod-mono`.
- `prod-mono`'s CI is already rendered per component. A single-language
  GitLab template would be a fourth, separate design (open question 2).
- The manifest key and the selection mechanism are standard-agnostic.
  Extending to the other standards later adds catalog entries and
  templates, and does not change `vibe.yaml`'s shape.

### 3. `provider: github` (the default)

Exactly today's `prod-mono` output, byte for byte. That covers
`.github/workflows/ci.yml`, `.github/dependabot.yml`,
`.github/pull_request_template.md` and
`.github/workflows/conformance.yml`, the root `Taskfile.yml` with
`workflows:lint` in `verify`, and `actionlint` as a required tool.

### 4. `provider: gitlab`

These managed files replace the GitHub ones. All are `Generated`:

| Path | Module | Contents |
|---|---|---|
| `.gitlab-ci.yml` | `gitlab-ci-mono` | the pipeline: one job per component, plus the two optional extension points below |
| `.gitlab/merge_request_templates/Default.md` | `gitlab-ci-mono` | the same text as the GitHub pull request template |
| `.gitlab-ci.vibe.yml` | `vibe-conformance` | VibeConform's conformance job, the only GitLab file that runs `vibe` |

The three names mirror `Taskfile.yml`, `Taskfile.vibe.yml` and
`Taskfile.local.yml`.

**Component jobs.** There is one job per component, keyed by its `id`.
Each job uses the toolchain image for the component's profile, installs
the pinned Task version and the component's dependencies in its
directory, and runs `task <id>:verify`. Go components also run
`task <id>:test:race`. The pinned versions are the GitHub template's
(`GO_VERSION`, `TASK_VERSION`, `GOLANGCI_LINT_VERSION`, `NODE_VERSION`,
`PYTHON_VERSION`). TypeScript takes pnpm from the component's
`packageManager` field (spec 0020). Python uses `uv sync`. Jobs run on
Linux only; see "Explicit non-goals".

**Hardening against configuration merges.** GitLab deep-merges included
configuration into jobs of the same name, and the main file wins only
for the keys it declares. So every generated job explicitly declares the
keys that decide whether and how it runs and passes: `stage`, `image`,
`needs`, `rules`, `allow_failure: false`, `before_script`, `script` and
`interruptible`.

**Pipeline rules.** A `workflow:rules` block runs merge request
pipelines, and branch pipelines for branches without an open merge
request. This avoids duplicate pipelines. It is the GitLab equivalent
of `on: pull_request` plus `push: [main]`.

**No gate job.** GitLab's "Pipelines must succeed" merge check covers
the whole pipeline. So no `CI / gate` job is generated: the
required-check list is already fixed, whatever the components are.

**Conformance include.** `.gitlab-ci.yml` includes `.gitlab-ci.vibe.yml`
with `rules: exists` on that path. When the file is absent, the include
does nothing. The included file holds one job, `conformance:audit`. It
sets up Go and Task at the pinned versions, runs the ADR 0008
self-hosting probe (building `./cmd/vibe` only if that directory
exists), and runs `task audit`. It has no `vibe` install step and no
`vibe@latest`. Spec 0022's pinning rule applies unchanged.

**Project-owned extension point: `.gitlab-ci.local.yml`.** A generated
job named `local` triggers a **child pipeline** from
`.gitlab-ci.local.yml`, if that file exists (`rules: exists`). Its
status mirrors the child pipeline's status, so a failing project job
fails the pipeline. VibeConform never creates, reads, validates or
audits `.gitlab-ci.local.yml`.

A child pipeline is chosen over `include: local` because it keeps the
property ADR 0009 relies on: a project can *add* jobs, but cannot
redefine generated ones. Jobs in a child pipeline are a separate
configuration. They cannot deep-merge into `api` or `conformance:audit`
to inject `allow_failure`, `variables` (for example `GOFLAGS=-run=^$`)
or `rules: when: never`. `include: local` would allow exactly that.
Open question 1 records the trade-off.

**Reserved names.** A component `id` that is a GitLab top-level keyword
or a reserved job name is an error under `gitlab`, and only under
`gitlab`, so existing manifests stay valid. The error names the
component and the keyword. The reserved names are `default`, `include`,
`stages`, `variables`, `workflow`, `image`, `services`, `cache` and
`pages`. Names already reserved by ADR 0012, including `local`, stay
reserved.

**Root files.**

- The root `Taskfile.yml` has no `workflows:lint` task, and `verify`
  does not call it.
- `actionlint` is not a required tool, so `vibe sync` and `vibe doctor`
  stop reporting it.
- `lefthook.yml` and the root `Taskfile.yml` name `.gitlab-ci.yml`
  wherever they named a GitHub path.
- No generated file names a `.github/` path.

**No dependency updates.** No Dependabot equivalent is generated
(see "Explicit non-goals").

### 5. `provider: none`

No CI files. `Taskfile.vibe.yml` (`task audit`) is still generated, as
are `verify` and `verify-ci`. These are the entry points any CI system
calls. The root `Taskfile.yml` loses `workflows:lint`, and `actionlint`
is not required, as under `gitlab`.

`none` narrows spec 0026's "no core deselection" non-goal, and this
spec says so on purpose:

- It is an explicit, reviewable line in `vibe.yaml`, not a silent
  removal.
- Even with `github`, VibeConform cannot ensure that CI actually gates
  merges. Branch protection is a repository setting (spec 0022). So
  `none` gives up a convenience, not a mechanical guarantee.
- Without `none`, a repository on Bitbucket, Gitea or Jenkins would be
  in this adopter's position today: red `audit` for files it cannot use.

`docs/usage.md` states what a `none` repository's CI must run itself:
`task verify-ci` and `task audit`.

### 6. Switching provider

Switching reuses spec 0026 §7 unchanged. The resources of the provider
no longer selected are prune candidates. The same applies to resources
a core module produced only for that provider, such as
`.github/workflows/conformance.yml` or `.gitlab-ci.vibe.yml`.

| File of the old provider | `sync` |
|---|---|
| absent (the adopter already deleted it) | forgotten |
| unmodified | removed, with any directory it leaves empty |
| modified | kept, reported as a conflict, non-zero exit |

The new provider's files are written through `Create`. If a file
already exists at a new path and VibeConform never recorded it (the
adopter's hand-written `.gitlab-ci.yml`), it is a conflict. It is never
overwritten (reconcile truth table, P absent and C ≠ T).

`docs/usage.md` gets a migration recipe for a repository like this
adopter's:

1. Move the project's own jobs into `.gitlab-ci.local.yml`.
2. Delete the hand-written `.gitlab-ci.yml`.
3. Set `ci: {provider: gitlab}`.
4. Run `vibe diff`, then `vibe sync`, then `task audit`.

Switching back to `github` is symmetric.

### 7. Removing VibeConform (GitLab)

Removal stays deletion only: `vibe.yaml`, `.vibe/`,
`.gitlab-ci.vibe.yml` and `Taskfile.vibe.yml`. The `include` of
`.gitlab-ci.vibe.yml` in `.gitlab-ci.yml` then matches nothing and does
nothing, like the `vibe` include in `Taskfile.yml`. Everything else is
ordinary GitLab configuration.

### 8. Documentation

- `docs/usage.md`:
  - a "CI provider" section with the three values, the GitLab file set,
    `.gitlab-ci.local.yml` and its child-pipeline semantics, the
    migration recipe, and the known gaps;
  - updates to "What `prod-mono/v1` manages", "Removing VibeConform"
    and the required-tools list.
- `docs/architecture/overview.md`: the `ci:` map in the option catalog,
  `ci/gitlabmono` in the package layout, and the GitLab seam next to
  ADR 0009.
- A new ADR records the provider setting and the child-pipeline seam.
  It also records the narrowing of spec 0026's non-goal.

## Behavior

- A manifest without `ci:` resolves byte-identically to today in every
  standard. The one change a sync makes is the recorded `vibe_version`.
- The same `vibe.yaml` always resolves the same bytes. Output does not
  depend on the platform `sync` runs on.
- Under `gitlab`, `task verify` passes on a correct repository with no
  `actionlint` and no `.github/`.
- No generated file except `Taskfile.vibe.yml`, `conformance.yml` and
  `.gitlab-ci.vibe.yml` runs `vibe` or `task audit`, whichever provider
  is selected.
- `sync` followed by `audit` is conformant, and a second `sync`
  reports nothing, for every provider and after every switch.

## Explicit non-goals

- **`gitlab` and `none` for `prod-go`, `prod-ts` and `prod-py`.** These
  come in a follow-up spec (open question 2).
- **Dependency updates on GitLab.** There is no Dependabot. Renovate is
  the common choice, but it is a bot with its own configuration and
  hosting decisions, so it gets its own spec if wanted.
- **A Windows job on GitLab.** GitLab.com's hosted Windows runners are
  beta, and self-managed instances may have none. `prod-mono`'s GitHub
  CI tests Go on Windows. GitLab v1 does not, and `docs/usage.md` says
  so (principle 2: "Windows where a workflow's matrix says so").
- **Linting the GitLab configuration in `task verify`.** There is no
  offline equivalent of `actionlint`; GitLab's lint API needs a token
  and network access. VibeConform's own tests parse and check the
  generated YAML, and GitLab rejects invalid configuration loudly at
  pipeline creation.
- **Other providers** (Bitbucket Pipelines, Gitea/Forgejo Actions,
  Azure Pipelines, Jenkins). `none` covers them for now.
- **Configuring GitLab project settings**, such as "Pipelines must
  succeed" or auto-cancelling redundant pipelines. These are documented,
  not written, like branch protection in spec 0022.
- **Per-component CI overrides.** Project jobs go in
  `.gitlab-ci.local.yml`, as project tasks go in `Taskfile.local.yml`.
- **Image digests and automated image updates.** Images are pinned by
  version tag (open question 6).

## Acceptance criteria

Unit and end-to-end tests use the repository's existing style:
structural YAML assertions in module tests, `t.TempDir()` sync, diff
and audit runs in `internal/cli`, and `TestExamplesAreConformant`.

1. **Default is byte-identical.** `examples/monorepo`, `examples/*` and
   this repository stay conformant with no re-sync
   (`TestExamplesAreConformant`, `task audit`). A test resolves
   `prod-mono` with `ci:` absent and with `provider: github`, and gets
   identical resources.
2. **Manifest.** Strict decoding rejects unknown keys under `ci:`. An
   unknown provider fails with the valid list. `ci:` under
   `prod-go`/`prod-ts`/`prod-py` fails with an error that names the
   standard. Each case exits 1 before anything resolves.
3. **GitLab resource set.** `provider: gitlab` resolves exactly the §4
   paths in place of the four `.github/` paths, in a fixed order, with
   the same component and editor/agent resources as before.
4. **`.gitlab-ci.yml` structure.** Parsed with `yaml.v3`:
   - one job per component `id`, whose script runs `task <id>:verify`;
   - Go jobs also run `task <id>:test:race`;
   - each job uses its profile's image and the pinned versions;
   - every generated job declares the §4 hardening keys;
   - `workflow:rules` is present;
   - the `.gitlab-ci.vibe.yml` include and the `local` trigger each
     carry `rules: exists` on their own path;
   - the `local` trigger's status mirrors the child pipeline.
5. **Conformance stays isolated.**
   - No job in `.gitlab-ci.yml` runs `vibe` or `task audit`. Its only
     mention of `vibe` is the include path.
   - `.gitlab-ci.vibe.yml` has exactly one job, which runs `task audit`
     and holds the self-hosting probe.
   - No generated file contains `vibe@latest`.
   - The verify-independence tests cover the GitLab output.
6. **Root files under `gitlab` and `none`.**
   - No `workflows:lint` task exists, and `verify` does not reach it.
   - `RequiredTools` and `vibe doctor` do not list `actionlint`.
   - No generated file contains `.github/`.
7. **Reserved names.** Under `gitlab`, a component `id` from the §4 list
   fails with the component and the keyword named. Under `github`, the
   same manifest still resolves.
8. **Switching.** In a temp repository synced with `github`, switching
   to `gitlab` and syncing:
   - removes the unmodified `.github/` files;
   - forgets the files already deleted;
   - keeps a modified one as a conflict (non-zero exit);
   - creates the GitLab files.

   A second `sync` reports nothing, and `audit` is conformant. Switching
   back to `github` round-trips the same way. The same holds for `none`.
9. **Hand-written file.** An unrecorded `.gitlab-ci.yml` present at
   switch time is reported as a conflict and left byte-for-byte
   unchanged.
10. **Runtime, CI.** A scratch copy of `examples/monorepo` with
    `provider: gitlab` runs `task verify` and `task audit` green, with
    `actionlint` absent from `PATH`. This is a job in `examples.yml`.
11. **Real integration (not in unit CI; a separate ledger line).** On a
    GitLab project holding that copy:
    - the pipeline runs every component job and `conformance:audit`
      green;
    - adding a failing job to `.gitlab-ci.local.yml` fails the
      pipeline;
    - a `.gitlab-ci.local.yml` job named `api` that sets
      `allow_failure: true` does not change the parent `api` job;
    - deleting `.gitlab-ci.vibe.yml` and `.gitlab-ci.local.yml` still
      yields a valid pipeline.
12. **Documentation.** The §8 documentation, and the ADR, are written.

## Assumptions not yet verified

Verified against GitLab's documentation (docs.gitlab.com, 2026-10-02):

- `include` accepts `rules` with `exists`.
- The main `.gitlab-ci.yml` takes precedence over included files, and
  among included files the last one wins. Hashes are deep-merged and
  arrays (`rules`, `script`) are replaced.
- Default stages are `.pre`, `build`, `test`, `deploy` and `.post`.
- A child pipeline sees `CI_PIPELINE_SOURCE=parent_pipeline` and
  receives the parent's `CI_MERGE_REQUEST_*` variables.
- `trigger:strategy: mirror` makes the trigger job's status equal the
  child pipeline's. The docs call `depend` "not recommended", because
  its status does not always match.

Not verified:

- **`rules: exists` behaviour.** That an `include: local` with
  `rules: exists` on a missing file is a silent no-op, and that a
  trigger job with `rules: exists` is skipped cleanly. Both need a real
  pipeline (acceptance 11).
- **GitLab versions.** The minimum version that has `include:rules:
  exists`, `trigger:include:local` and `strategy: mirror`, and whether
  the adopter's instance (GitLab.com or self-managed, and which version)
  meets it.
- **Images.** That the `node:<NODE_VERSION>` image ships `corepack` for
  pnpm, and that the `golang` image has the C toolchain `-race` needs.
- **Merge request template.** That
  `.gitlab/merge_request_templates/Default.md` is applied by default on
  the adopter's tier. Otherwise it is only selectable.

## Open questions for the user

1. **Seam: child pipeline (recommended) or `include: local`.**
   - **Child pipeline.** It keeps ADR 0009's property that projects can
     add jobs but cannot redefine generated ones. But project jobs
     cannot `needs:` a generated job, cannot share its artifacts, and
     show up as a downstream pipeline.
   - **`include: local`.** It puts everything in one pipeline. But it
     lets the project file deep-merge keys into generated jobs. That is
     a hole, documented rather than closed.
2. **Scope.** `prod-mono` only (recommended), or all four standards in
   v1.
3. **`none` in v1** (recommended: yes, with the §5 rationale), or defer
   it and accept only `github` and `gitlab`.
4. **Go on Windows under GitLab.** Linux only (recommended), or a
   `saas-windows-medium-amd64` job that works only on GitLab.com.
5. **Merge request template.** Generate it (recommended, for parity), or
   generate nothing under `.gitlab/`.
6. **Image pinning.** Version tags (recommended: nothing would update
   digests without Dependabot or Renovate), or digests for parity with
   the SHA-pinned Actions.
7. **Manifest shape.** A new `ci:` map (recommended: CI hosting is
   neither a repository policy nor a development setting, and it leaves
   room for later keys), or `policy: {ci_provider: ...}`.
8. **Minimum GitLab version** that VibeConform documents as supported.
