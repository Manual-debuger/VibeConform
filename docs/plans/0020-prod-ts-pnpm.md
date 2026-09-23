# Plan 0020: `prod-ts/v1` uses pnpm

See `docs/specs/0020-prod-ts-pnpm.md` for the accepted scope. Approved at
spec review: the change lands in place in `prod-ts/v1`, not as `v2`.

Branch: `feature/prod-ts-pnpm`, off `main`, one pull request.

## Resolved open questions

The spec left three questions for the plan. Answers, checked on
2026-09-23:

- **Dependabot and pnpm.** The `npm` ecosystem covers pnpm, but only
  **pnpm v7 to v10**, according to GitHub's "Supported ecosystems and
  repositories" page. pnpm's `latest` tag is now 12.5.1, and this machine
  has 11.25.0. So `examples/typescript` pins **`pnpm@10.34.5`** (the
  `latest-10` tag). `docs/usage.md` tells adopters why: a `packageManager`
  above v10 leaves Dependabot unable to update `pnpm-lock.yaml`.
- **`pnpm/action-setup`.** Latest release is `v6.1.0`, commit
  `ea17c68df8912ef543352723c149a84f56e3d413`; the tag is annotated, so the
  pin is the dereferenced commit. `version` is optional and falls back to
  `packageManager` in `package.json`, which is what the spec wants. Its
  `standalone` input stays at the default `false`, since Node is installed
  in the job anyway.
- **Windows for the example.** Added: `examples.yml`'s TypeScript job runs
  on `ubuntu-latest` and `windows-latest`. Without it nothing proves
  `prod-ts` on Windows (principle 2). The Python job is out of scope here.

`actions/setup-node` at the already-pinned SHA accepts `cache: pnpm`.

## Order of work

Four commits, each leaving the tree green.

1. **C1: templates and tests.** `ts-repo-tooling` and `github-ci-ts`
   templates, `RequiredTools`, and their tests. Rebuild, then sync
   `examples/typescript`.
2. **C2: example becomes a pnpm project.** Lockfile swap and
   `packageManager`, then `examples.yml`.
3. **C3: docs.** `docs/usage.md`, plus anything else that became false.
4. **C4: plan checklist ticked, verification recorded.**

C1 and C2 are separate so the template change can be reviewed apart from
a 48 KB lockfile swap. C1 alone leaves the example's CI red for one commit
(its generated `ci.yml` needs `pnpm-lock.yaml`), which is acceptable inside
one pull request: the PR is reviewed and merged as a whole.

## Checklist

### C1: templates and tests

Regression tests first, watched failing:

- [x] `tsrepotooling_test.go`:
  - [x] Add `TestTaskfileUsesPnpm`: the Taskfile contains no `npx ` or
        `npm ` and runs `pnpm exec` for `prettier`, `eslint`, and `tsc`,
        and `pnpm test`.
  - [x] Add `TestLefthookUsesPnpm`: the same rules for `lefthook.yml`.
  - [x] Change `TestRequiredTools` to expect `task`, `lefthook`, and
        `pnpm`.
- [x] `githubts_test.go`:
  - [x] Add `TestWorkflowUsesPnpm`: no `npm ci`;
        `pnpm install --frozen-lockfile` in every job that installs; a
        `pnpm/action-setup` step pinned to a 40-character SHA, appearing
        before `actions/setup-node`; and `cache: pnpm`.
  - [x] Keep `TestDependabotIsNPMNotGoMod` unchanged: `npm` is still the
        correct ecosystem for pnpm.
- [x] `tstooling_test.go`: correct the comment in `TestRequiredTools` that
      calls pnpm project-local. **The assertion is not changed** —
      `ts-tooling` still requires only `node`.

Then the implementation:

- [x] `internal/module/tsrepotooling/templates/Taskfile.yml`: `npx` becomes
      `pnpm exec`, and `npm test` becomes `pnpm test`.
- [x] `internal/module/tsrepotooling/templates/lefthook.yml`: the same
      substitutions.
- [x] `tsrepotooling.go`: add `pnpm` to `RequiredTools` (why: "every
      Taskfile and lefthook command runs through pnpm exec"), and update
      the package comment.
- [x] `internal/module/ci/githubts/templates/ci.yml`, in both `lint` and
      `typecheck_test`:
  - [x] add `pnpm/action-setup@ea17c68… # v6.1.0` before Setup Node;
  - [x] add `cache: pnpm` to Setup Node;
  - [x] replace `npm ci` with `pnpm install --frozen-lockfile`.
- [x] `githubts.go`: package comment mentions pnpm.
- [x] Check every edited template for CR bytes, with
      `tr -cd '\r' < FILE | wc -c`. `TestTemplatesAreLF` catches them
      too.
- [x] Rebuild with `task build`, then run
      `./bin/vibe sync --repo-root examples/typescript` and
      `./bin/vibe audit` on all three roots.

### C2: example becomes a pnpm project

- [x] Add `"packageManager": "pnpm@10.34.5"` to
      `examples/typescript/package.json`.
- [x] In `examples/typescript`: run `pnpm import` to create
      `pnpm-lock.yaml` from `package-lock.json`, delete
      `package-lock.json`, then run `pnpm install` and confirm the lockfile
      doesn't change.
- [x] Confirm the resolved devDependency versions match what was pinned
      before. `pnpm import` preserves them, and the diff should show no
      version changes.
- [x] Run `task verify` inside `examples/typescript` with pnpm. Every rule
      that ran under npm still runs (fmt:check, lint, typecheck, test).
- [x] `.github/workflows/examples.yml`, TypeScript job:
  - [x] add the `pnpm/action-setup` step, with the same pin;
  - [x] add `cache: pnpm`;
  - [x] change `npm ci` to `pnpm install --frozen-lockfile`;
  - [x] add `strategy.matrix.os: [ubuntu-latest, windows-latest]`.

  The job's `Build vibe from source` step uses `mkdir -p` and
  `$RUNNER_TEMP`. Set `shell: bash` so it runs the same way on Windows
  runners.
- [x] `.gitignore`: check that `node_modules/` still covers pnpm's layout
      (it does; pnpm writes only `node_modules/`).

### C3: documentation

- [x] `docs/usage.md`, "What `prod-ts/v1` and `prod-py/v1` manage":
  - [x] replace `npx` with `pnpm exec`;
  - [x] state the `packageManager` requirement;
  - [x] note that Dependabot supports pnpm only up to v10;
  - [x] add a migration subsection with the spec's three manual steps.
- [x] `docs/usage.md`, "Worked examples": `package.json`/`pnpm-lock.yaml`.
- [x] `examples/typescript` README or comments, if any mention npm.
- [x] `grep -rn "npm ci\|npx \|package-lock" docs/ README.md` for anything
      else that became false. Leave historical specs and plans alone.

### C4: close out

- [x] Tick this checklist and record verification output below.
- [x] Spec 0020 status: "accepted and implemented."

## Verification

Required before the pull request:

- `task verify` and `task audit` at the repository root. Since spec 0017,
  `verify` alone does not catch a hand-edited managed file.
- `./bin/vibe audit --repo-root examples/typescript` and
  `--repo-root examples/python` both conformant.
- `task verify` inside `examples/typescript`, run locally on Windows.
- CI: `CI / gate` green, and both OS legs of `examples.yml`'s TypeScript
  job green.

### Observed locally (Windows 11, 2026-09-23)

- `task verify` at the root: exit 0. Covers fmt:check, typecheck, lint,
  test, mod:verify, govulncheck ("No vulnerabilities found."), and
  actionlint.
- `task audit` in the root, `examples/typescript`, and `examples/python`:
  `conformant` in each, with a freshly stamped
  `v0.2.0-alpha.1.0.20260923081233-c2e544afdef3+dirty` first on `PATH`.
- `task verify` inside `examples/typescript`, through pnpm 10.34.5: exit 0
  (prettier, eslint, tsc, `pnpm test`).
- `pnpm import` then `pnpm install`: lockfile byte-identical, and direct
  devDependencies resolved at the versions `package.json` pins.
- The new regression tests failed against the old templates for the
  expected reasons, then passed.

### Deviations from the checklist

- **`assertNoNpm` first matched `pnpm` itself.** The substring `"npm "`
  occurs inside `"pnpm test"`, so the first version of the test failed on
  a correct template. It now matches `\bnp[mx]\b` as whole words. I
  confirmed it still fails against the old templates, with only the
  templates reverted.
- **`examples.yml` needed two things the plan didn't list:**
  - `pnpm/action-setup` reads `package.json` from the repository root by
    default, so the fixture job passes
    `package_json_file: examples/typescript/package.json`, plus a matching
    `cache-dependency-path`. The generated `ci.yml` keeps the root
    default, which is what an adopting repository has.
  - On the new Windows leg, `go build -o …/vibe` produces a file that
    `PATH` lookup won't find. The build step names it
    `vibe$(go env GOEXE)`.
- **A stale `vibe` on `PATH` fails `task audit`, and nothing catches it.**
  `~/go/bin/vibe` on this machine reports `dev`, which isn't orderable, so
  spec 0019's guard stays dormant, and it reports the root as `2 drifted`.
  This is the trap `docs/usage.md` already describes; this change doesn't
  cause it. Worth a follow-up: a `dev` binary whose templates disagree
  with a newer recorded `vibe_version` still can't be told apart.

## Pull request

Title `feat: prod-ts/v1 uses pnpm`, body links spec and plan, lists the
adopter migration steps, and calls out the Dependabot v10 ceiling.
Merged with a merge commit after an explicit go-ahead.

## Explicitly still deferred

- `prod-ts/v2` and any standard-retirement policy.
- Managing `packageManager` through `StructuredPatch`.
- Windows legs for the Python example and for the generated TS/PY
  `ci.yml` templates. Those templates ship to adopters, so adding an OS
  matrix changes every adopter's CI bill. That needs its own decision.
