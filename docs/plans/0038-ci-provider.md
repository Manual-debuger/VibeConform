# Plan 0038: CI provider selection, and GitLab CI for `prod-mono`

Implements `docs/specs/0038-ci-provider.md`. Status: approved and
implemented. Two deviations from the steps below:

- `Taskfile.vibe.yml`'s header is rewritten by exact anchor
  (`module.ReplaceOnce`) rather than becoming a `text/template`, so the
  GitHub bytes are the embedded file itself.
- The `monorepotooling` lefthook template needs no provider field: spec
  0036 made its header provider-neutral.

Branch: `feat/ci-gitlab`, off `main`. One pull request.

The plan assumes the spec's recommended answers to its open questions:
a child-pipeline seam, `prod-mono` only, `none` included, Linux only, a
merge request template, tag-pinned images, and a `ci:` map. Where a
different answer changes the plan, the step says how.

## Resolved design questions

**Where the selection lives.** A new option group, following the path
spec 0030 took for `development:`:

- In `internal/manifest`:
  - add `MapCI = "ci"`, a `CI` struct with `Provider *string`, and
    `CIProvider = "provider"`;
  - add `{MapCI, CIProvider}` to `ScalarKeys`, last, and a `Scalar()`
    case for it.
- In `internal/standard`, `noun()` already says "setting" for any
  non-policy map, so the error reads "prod-go/v1 offers no provider
  setting".

`Select` already gives a scalar group its `Default` option. This is the
first catalog entry to use that branch, so it gets a test of its own.

**How the core reacts.** Provider options resolve nothing themselves,
like `workflow.NewDocsDir`. The core modules read the value instead, so
`planPrunes`' trial selection finds every file a deselected provider
would produce, through whichever module produces it. There is no new
prune code. A helper, `module.CIProvider(mctx) string`, returns
`github` when the context, its `Policies`, or the key is nil or empty.

Absent means today's behaviour, so the helper must default to `github`
for a nil context. Unlike `module.Selected`, failing open is the
compatible direction here. It keeps every existing module test, which
resolves with a nil or bare context, byte-identical.

**Catalog per standard.** `catalog()` becomes `catalog(extra ...Option)`.
Only `prod-mono` passes the three `ci.provider` options:

- `github`, with `Default: true`;
- `gitlab`;
- `none`.

They sit at the end of the catalog, after the development settings, so
existing catalog order, and so report order, is unchanged. `prod-go`,
`prod-ts` and `prod-py` pass nothing and keep their current catalog.

**Module order.** `prod-mono`'s core becomes:

```go
monotooling.New(), githubmono.New(), gitlabmono.New(), conformance.New(), monorepotooling.New()
```

`gitlab-ci-mono` resolves nothing under `github`. `github-ci-mono`
resolves nothing under `gitlab` or `none`. Reports stay in module order,
and the default report is unchanged because the new module is silent.

An alternative is one `ci-mono` module that switches internally. It
would keep a single module slot, but change the module name `audit`
prints for today's files. Separate packages follow spec 0010's
`ci/gitlab` note.

**Conformance module.** `vibe-conformance` is shared by all four
standards. It switches on `module.CIProvider`:

| Provider | Resolves |
|---|---|
| `github` | `conformance.yml`, then `Taskfile.vibe.yml`; unchanged |
| `gitlab` | `.gitlab-ci.vibe.yml`, then `Taskfile.vibe.yml` |
| `none` | `Taskfile.vibe.yml` only |

`Taskfile.vibe.yml`'s header comment names
`.github/workflows/conformance.yml`. That comment is rendered per
provider: the GitHub bytes stay exactly as they are, `gitlab` names
`.gitlab-ci.vibe.yml`, and `none` names no CI file. The template becomes
a `[[ ]]` `text/template`. A golden test pins the GitHub rendering to
today's bytes.

**Root Taskfile and lefthook.** In `monorepotooling`:

- the template gets a `.Provider` field;
- `workflows:lint` and its `verify` step are emitted only under
  `github`;
- the lefthook and Taskfile comments that name
  `.github/workflows/ci.yml` render per provider;
- `RequiredTools` appends `actionlint` only under `github`.

`doctor` reads `RequiredTools`, so it follows with no change.

**Reserved component ids.** These are checked in `gitlab-ci-mono`'s
`Resolve`, not in `manifest`. A manifest-level reservation would newly
reject an existing `github` repository with a component named `cache`
or `pages`. A resolve error is still reported before anything is
written, and names the component and the keyword.

**Installing Task in non-Go images.** GitHub installs Task with
`go install` after `setup-go` in every job. GitLab jobs run in one image
each:

- In Go jobs, Task is installed with `go install`, as on GitHub.
- In `node` and `uv` jobs, the pinned release archive
  (`task_linux_amd64.tar.gz` at `TASK_VERSION`) is downloaded, checked
  against a pinned `TASK_SHA256` variable, and unpacked onto `PATH`.

A checksum mismatch fails the job. A test checks that
`TASK_SHA256` is set whenever `TASK_VERSION` is used outside a Go job.
Bumping `TASK_VERSION` now means bumping both values. `docs/usage.md`
and a template comment say so. Installing Go into every job was
rejected: it is heavier, and the TypeScript and Python jobs would then
need two toolchains.

**Images.**

| Profile | Image | Toolchain setup |
|---|---|---|
| Go | `golang:${GO_VERSION}` | as on GitHub |
| TypeScript | `node:${NODE_VERSION}` | `corepack enable`, then pnpm from `packageManager` |
| Python | `ghcr.io/astral-sh/uv:python${PYTHON_VERSION}-bookworm` | `uv sync` |

Each job keeps its caches under `$CI_PROJECT_DIR/.ci-cache/<id>/`:

- `GOMODCACHE` and `GOCACHE` for Go, keyed on `<path>/go.mod`;
- the pnpm store for TypeScript, keyed on `pnpm-lock.yaml`;
- `UV_CACHE_DIR` for Python, keyed on `uv.lock`.

`.ci-cache/` is at the repository root, which is never a component
(ADR 0012). So no component's `gofmt`, Prettier or Ruff walk ever sees
it.

**Hardening keys.** Every generated job sets all of the following:

- `stage: test`
- `image`
- `needs: []`
- `rules: [{when: on_success}]`
- `allow_failure: false`
- `before_script: []` (setup goes in `script`)
- `script`
- `interruptible: true`

No `stages:` is declared, so GitLab's defaults apply. A
`.gitlab-ci.local.yml` runs in its own child pipeline and declares its
own stages.

**Templating.** `.gitlab-ci.yml` uses `text/template` with `[[ ]]`
delimiters, like `ci.yml.tmpl`. It leaves GitLab's own `$VAR` and
`${VAR}` untouched.

## Repository impact

| Area | Change |
|---|---|
| `internal/manifest` | `CI` map, `MapCI`, `CIProvider`, `ScalarKeys` entry, `Scalar()` case; tests |
| `internal/standard` | `catalog(extra ...Option)`; `prod-mono` passes three provider options and gains `gitlabmono.New()` in its core; tests for the scalar default and the per-standard error |
| `internal/module` | `CIProvider(mctx)` helper and provider constants; a no-op provider module; verify-independence tests extended to resolved GitLab output |
| `internal/module/ci/githubmono` | returns nothing unless the provider is `github`; one test |
| `internal/module/ci/gitlabmono` (new) | module, `.gitlab-ci.yml.tmpl`, the MR template (reused from `ci/github`, as `githubmono` already does), tests |
| `internal/module/conformance` | provider switch; `.gitlab-ci.vibe.yml` template; `Taskfile.vibe.yml` as a template with a byte-golden test for GitHub |
| `internal/module/monorepotooling` | `.Provider` in both templates; conditional `workflows:lint`; conditional `actionlint` |
| `internal/cli` | no production change expected: selection, pruning and conflicts are spec 0026's. New end-to-end tests for switching and the hand-written-file conflict. |
| `internal/doctor` | none (follows `RequiredTools`) |
| `internal/reconcile`, `state` | none; no state schema change |
| `.github/workflows/examples.yml` | one job: copy `examples/monorepo` to a temp dir, set `provider: gitlab`, sync with a `vibe` built from source, then run `task verify` (no `actionlint` on `PATH`) and `task audit` |
| Examples | unchanged; `examples/monorepo` stays `github` and is the byte-identity guard |
| This repository | unchanged (`prod-go`) |
| Docs | ADR 0019; `usage.md`; `overview.md`; spec 0026's non-goal gets a pointer to spec 0038; `README.md` status line |

**Blast radius for existing adopters.** No generated byte changes under
the default. The prune path gains new candidates only when `ci:` is set.
`prod-go`, `prod-ts` and `prod-py` reject `ci:`, which no existing
manifest contains, because strict decoding rejected it until now.

**New dependency.** None. YAML parsing in tests uses `gopkg.in/yaml.v3`,
already a dependency.

## Steps

Each step ends with `task verify:fast` green.

1. **ADR 0019.** Record three decisions: the provider is a single-valued
   core setting, not an integration; the child-pipeline seam and the
   residual gaps; and the narrowing of spec 0026's non-goal for `none`.
   Under the `include: local` answer to open question 1, the ADR records
   the merge hole instead, and the hardening keys become the only
   mitigation.
2. **Manifest and selection.** Add the `ci:` map, `ScalarKeys`,
   `catalog(extra...)`, the `prod-mono` options and the
   `module.CIProvider` helper. Tests cover:
   - strict decoding;
   - an unknown value;
   - `ci:` under `prod-go`;
   - the default selection;
   - `Selection.Has` for the default.
3. **GitHub path stays put.** Make `githubmono`, `conformance` and
   `monorepotooling` provider-aware. Before any GitLab content exists,
   prove default byte-identity:
   - `TestExamplesAreConformant`;
   - a resolution-equality test for absent versus `github`;
   - the `Taskfile.vibe.yml` golden test.
4. **`none`.** Under `none`, there are no CI resources, no
   `workflows:lint` and no `actionlint`. Add the end-to-end switch test
   from `github` to `none` and back: Remove, Forget, RemoveConflict, and
   a second sync that is a no-op.
5. **`gitlab-ci-mono`.** Add the template, the module, reserved ids and
   the MR template. Add structural tests for acceptance 4, 5 and 7.
6. **`.gitlab-ci.vibe.yml`.** Add the conformance job with the
   self-hosting probe. The probe builds into
   `$CI_PROJECT_DIR/.ci-cache/bin`, which is on `PATH` for `task audit`
   in the same script, because GitLab has no `$GITHUB_PATH`. Add the
   switch tests for `github` to `gitlab` and back, and the
   hand-written-file conflict test.
7. **`examples.yml` job** (acceptance 10).
8. **Docs.** `usage.md`'s "CI provider" section covers:
   - the provider values and the file set;
   - `.gitlab-ci.local.yml`, with a sketch of the adopter's Postgres
     `services:` migration job;
   - the migration recipe;
   - "Removing VibeConform" for GitLab;
   - the known gaps: no Windows, no dependency bot, no offline config
     lint, and project settings not written.

   Also update `overview.md` and the README.
9. **Real GitLab run** (acceptance 11), recorded in the verification
   section of this plan. This needs a GitLab project and a decision on
   which instance to use (spec open question 8).

## Tests (by acceptance criterion)

1. `TestExamplesAreConformant` (unchanged), and a new
   `TestCIProviderDefaultIsGitHub`. The latter resolves `prod-mono`
   with `ci:` absent and with `github`, and compares every resource's
   path and bytes. The golden test checks `Taskfile.vibe.yml` against
   today's bytes.
2. `manifest` table tests, plus `standard` `Select` table tests for the
   per-standard error and the value list.
3. `gitlabmono` checks its resolve order. A `cli` plan test checks the
   full path list for `prod-mono` with `gitlab`.
4. `gitlabmono` structural tests parse the YAML into job structs:
   - one job per id;
   - script lines;
   - image per profile;
   - hardening keys on every job;
   - `workflow:rules`;
   - `include` and `local` with `rules: exists`;
   - `strategy: mirror`.
5. `no_conformance` test for `.gitlab-ci.yml`. `conformance` tests for
   `.gitlab-ci.vibe.yml`. The `vibe@latest` scan is extended to every
   provider. `verify_independence_test` gains resolved
   `monorepotooling` output under each provider, not just templates.
6. `monorepotooling` tests check that the Taskfile has no
   `workflows:lint` and `verify`'s closure does not reach it, that
   `RequiredTools` has no `actionlint`, and that no resolved resource
   contains `.github/`. A `doctor` fake-env test checks that
   `actionlint` is not listed under `gitlab`.
7. `gitlabmono` reserved-id table tests. A `standard` test resolves the
   same manifest under `github` and succeeds.
8. and 9. `internal/cli` end-to-end tests in `t.TempDir()`, using the
   existing `writeVibeYAML`, `mustSync`, `runDiffIn` and `mustConform`
   helpers.
10. The `examples.yml` job.
11. A manual GitLab run. It is not automated.

## Verification

- `task verify:fast` while working; `task verify` and `task audit`
  before declaring done.
- After a rebuild, `vibe audit` at the root and in every example: no
  change expected.
- Ledger lines, kept separate:
  - unit tests;
  - `task verify` and `task audit` locally;
  - CI on GitHub (`ci.yml`, `examples.yml` with the new job);
  - the real GitLab pipeline (acceptance 11). That line is UNVERIFIED
    until someone runs it on a GitLab instance.

## Explicitly still deferred

As in the spec:

- `gitlab` and `none` for single-language standards;
- Renovate or any other dependency bot on GitLab;
- Windows jobs on GitLab;
- linting GitLab configuration;
- other providers;
- image digests.
