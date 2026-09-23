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

- [ ] `tsrepotooling_test.go`:
  - [ ] Add `TestTaskfileUsesPnpm`: the Taskfile contains no `npx ` or
        `npm ` and runs `pnpm exec` for `prettier`, `eslint`, and `tsc`,
        and `pnpm test`.
  - [ ] Add `TestLefthookUsesPnpm`: the same rules for `lefthook.yml`.
  - [ ] Change `TestRequiredTools` to expect `task`, `lefthook`, and
        `pnpm`.
- [ ] `githubts_test.go`:
  - [ ] Add `TestWorkflowUsesPnpm`: no `npm ci`;
        `pnpm install --frozen-lockfile` in every job that installs; a
        `pnpm/action-setup` step pinned to a 40-character SHA, appearing
        before `actions/setup-node`; and `cache: pnpm`.
  - [ ] Keep `TestDependabotIsNPMNotGoMod` unchanged: `npm` is still the
        correct ecosystem for pnpm.
- [ ] `tstooling_test.go`: correct the comment in `TestRequiredTools` that
      calls pnpm project-local. **The assertion is not changed** —
      `ts-tooling` still requires only `node`.

Then the implementation:

- [ ] `internal/module/tsrepotooling/templates/Taskfile.yml`: `npx` becomes
      `pnpm exec`, and `npm test` becomes `pnpm test`.
- [ ] `internal/module/tsrepotooling/templates/lefthook.yml`: the same
      substitutions.
- [ ] `tsrepotooling.go`: add `pnpm` to `RequiredTools` (why: "every
      Taskfile and lefthook command runs through pnpm exec"), and update
      the package comment.
- [ ] `internal/module/ci/githubts/templates/ci.yml`, in both `lint` and
      `typecheck_test`:
  - [ ] add `pnpm/action-setup@ea17c68… # v6.1.0` before Setup Node;
  - [ ] add `cache: pnpm` to Setup Node;
  - [ ] replace `npm ci` with `pnpm install --frozen-lockfile`.
- [ ] `githubts.go`: package comment mentions pnpm.
- [ ] Check every edited template for CR bytes, with
      `tr -cd '\r' < FILE | wc -c`. `TestTemplatesAreLF` catches them
      too.
- [ ] Rebuild with `task build`, then run
      `./bin/vibe sync --repo-root examples/typescript` and
      `./bin/vibe audit` on all three roots.

### C2: example becomes a pnpm project

- [ ] Add `"packageManager": "pnpm@10.34.5"` to
      `examples/typescript/package.json`.
- [ ] In `examples/typescript`: run `pnpm import` to create
      `pnpm-lock.yaml` from `package-lock.json`, delete
      `package-lock.json`, then run `pnpm install` and confirm the lockfile
      doesn't change.
- [ ] Confirm the resolved devDependency versions match what was pinned
      before. `pnpm import` preserves them, and the diff should show no
      version changes.
- [ ] Run `task verify` inside `examples/typescript` with pnpm. Every rule
      that ran under npm still runs (fmt:check, lint, typecheck, test).
- [ ] `.github/workflows/examples.yml`, TypeScript job:
  - [ ] add the `pnpm/action-setup` step, with the same pin;
  - [ ] add `cache: pnpm`;
  - [ ] change `npm ci` to `pnpm install --frozen-lockfile`;
  - [ ] add `strategy.matrix.os: [ubuntu-latest, windows-latest]`.

  The job's `Build vibe from source` step uses `mkdir -p` and
  `$RUNNER_TEMP`. Set `shell: bash` so it runs the same way on Windows
  runners.
- [ ] `.gitignore`: check that `node_modules/` still covers pnpm's layout
      (it does; pnpm writes only `node_modules/`).

### C3: documentation

- [ ] `docs/usage.md`, "What `prod-ts/v1` and `prod-py/v1` manage":
  - [ ] replace `npx` with `pnpm exec`;
  - [ ] state the `packageManager` requirement;
  - [ ] note that Dependabot supports pnpm only up to v10;
  - [ ] add a migration subsection with the spec's three manual steps.
- [ ] `docs/usage.md`, "Worked examples": `package.json`/`pnpm-lock.yaml`.
- [ ] `examples/typescript` README or comments, if any mention npm.
- [ ] `grep -rn "npm ci\|npx \|package-lock" docs/ README.md` for anything
      else that became false. Leave historical specs and plans alone.

### C4: close out

- [ ] Tick this checklist and record verification output below.
- [ ] Spec 0020 status: "accepted and implemented."

## Verification

Required before the pull request:

- `task verify` and `task audit` at the repository root. Since spec 0017,
  `verify` alone does not catch a hand-edited managed file.
- `./bin/vibe audit --repo-root examples/typescript` and
  `--repo-root examples/python` both conformant.
- `task verify` inside `examples/typescript`, run locally on Windows.
- CI: `CI / gate` green, and both OS legs of `examples.yml`'s TypeScript
  job green.

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
