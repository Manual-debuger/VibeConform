# Spec 0020: `prod-ts/v1` Uses pnpm

Status: accepted and implemented.

## Problem

`prod-ts/v1` runs everything it configures through npm:

- `ts-repo-tooling`'s `Taskfile.yml` and `lefthook.yml` call `npx eslint`,
  `npx prettier`, `npx tsc`, and `npm test`.
- `github-ci-ts`'s `ci.yml` installs dependencies with `npm ci` in both
  language jobs.
- `examples/typescript` commits a `package-lock.json`.

Spec 0016 explicitly deferred this choice ("No package-manager opinion
beyond what's needed to run the tools: no npm vs. pnpm vs. yarn decision").
This spec makes it: pnpm is the package manager for `prod-ts`.

Why pnpm over npm, as a standard's opinion rather than a preference:

- **Strict `node_modules`.** A package can only import what its own
  `package.json` declares. With npm, anything hoisted is importable, so a
  missing dependency declaration passes locally and in CI until the
  hoisting changes. That is a rule pnpm enforces mechanically, and
  principle 1 (`docs/architecture/principles.md`) prefers exactly that.
- **Frozen lockfile by default in CI.** `pnpm install` refuses to update
  `pnpm-lock.yaml` when `CI` is set, so an out-of-sync lockfile fails the
  job. npm's equivalent is `npm ci`, which the standard would otherwise
  have to keep choosing on purpose.
- **Cross-platform.** pnpm runs natively on Windows and Linux, and
  `pnpm exec` resolves local binaries the same way on both (principle 2).
- **Easy to remove.** pnpm is a standard third-party tool. After removing
  VibeConform the repository is an ordinary pnpm project (principle 3).

## Scope

The change is made **in place in `prod-ts/v1`**, not as a new `v2`. That
follows the precedent of spec 0016 (added modules to `v1`) and spec 0018
(removed `build`/`run` from `prod-go/v1`). The project is pre-alpha, and
the only known adopter is `examples/typescript`.

### 1. `ts-repo-tooling`: pnpm entry points

`Taskfile.yml`:

| Task | Today | After |
|---|---|---|
| `fmt` | `npx prettier --write …` | `pnpm exec prettier --write …` |
| `fmt:check` | `npx prettier --check …` | `pnpm exec prettier --check …` |
| `lint` | `npx eslint .` | `pnpm exec eslint .` |
| `typecheck` | `npx tsc --noEmit` | `pnpm exec tsc --noEmit` |
| `test` | `npm test` | `pnpm test` |

`lefthook.yml` makes the same substitutions (`pnpm exec prettier --check
{staged_files}`, `pnpm exec eslint {staged_files}`, `pnpm exec tsc
--noEmit`, `pnpm test`).

`pnpm exec`, not `pnpm dlx` and not bare `npx`: it runs only what the
repository installed, so a missing devDependency fails loudly instead of
fetching whatever version is current.

`RequiredTools` gains `pnpm`, alongside `task` and `lefthook`. pnpm is
installed globally (standalone installer, `npm install -g pnpm`, or
Corepack), so on a correctly configured machine it is on `PATH`, and its
absence is a real finding. This is the same test the existing modules
apply.

### 2. `github-ci-ts`: pnpm in CI

In both `lint` and `typecheck_test`:

- Add a `pnpm/action-setup` step, pinned by SHA like every other action,
  **before** `actions/setup-node`.
- `actions/setup-node` gains `cache: pnpm`.
- `npm ci` becomes `pnpm install --frozen-lockfile`. It is explicit even
  though `CI` makes it the default, so the job doesn't depend on an
  environment variable a reader can't see.

The pnpm version is **not** pinned by the standard. `pnpm/action-setup`
reads it from the `packageManager` field of the repository's own
`package.json`. That keeps the version with the project, which owns
`package.json` and its lockfile, and matches how the standard already
leaves eslint, prettier, and typescript versions to the repository (spec
0014). A repository without a `packageManager` field fails the setup step
loudly, which is the intended behavior.

`dependabot.yml` keeps `package-ecosystem: npm`. Dependabot's npm
ecosystem covers pnpm lockfiles; there is no separate `pnpm` ecosystem.
The plan verifies this against current Dependabot documentation before
relying on it.

### 3. `examples/typescript` becomes a pnpm project

- Replace `package-lock.json` with `pnpm-lock.yaml` (`pnpm import`, then
  `pnpm install`), resolving the same versions.
- Add `"packageManager": "pnpm@<version>"` to `package.json`.
- Re-sync with a rebuilt `vibe` and commit the regenerated files and
  `.vibe/state.yaml`.

`.github/workflows/examples.yml` is hand-authored. Its `typescript` job
gets the same `pnpm/action-setup` + `cache: pnpm` + `pnpm install
--frozen-lockfile` change.

### 4. Docs

- `docs/usage.md`: the `prod-ts/v1` section says pnpm instead of `npx`,
  names `packageManager` as a requirement, and adds a migration note (see
  Behavior).
- `README.md` and `docs/architecture/principles.md`: no change beyond the
  listed tools, which already name pnpm.

## Behavior

For a repository already on `prod-ts/v1`:

1. After upgrading `vibe`, `vibe audit` reports `Taskfile.yml`,
   `lefthook.yml`, and `.github/workflows/ci.yml` as **out of date** (exit
   3). They were not edited; the standard moved (spec 0019).
2. `vibe sync` rewrites them, as it would for any template change.
3. The repository has to do three things `vibe` cannot do for it, since
   `package.json` and lockfiles are project-owned:
   - run `pnpm import` to turn `package-lock.json` into `pnpm-lock.yaml`,
     then delete `package-lock.json`;
   - add a `packageManager` field to `package.json`;
   - declare any dependency it was importing only because npm hoisted it.
     pnpm's strict layout surfaces these as `Cannot find module` errors.

`docs/usage.md` states all three. `vibe sync` does not print them: it has
no way to know whether they have already been done, and a warning that
fires on a repository that already migrated teaches people to ignore
warnings (the same reasoning as `ToolRequirer`).

`prod-go` and `prod-py` are unchanged.

## Explicit non-goals

- **No `prod-ts/v2`.** Registering two versions of one standard is its own
  decision (what `vibe init` defaults to, how `v1` is retired). It isn't
  needed while the only adopter lives in this repository.
- **No managing `package.json`**, including the `packageManager` field. It
  stays project-owned until `StructuredPatch` ownership exists.
- **No pnpm workspaces.** Monorepo layout belongs to the future component
  graph (`docs/architecture/overview.md`), not to this change.
- **No yarn or bun support**, and no per-repository package-manager
  choice. A standard is one opinion.
- **No Node version change.** `NODE_VERSION` stays `"22"`.

## Design notes

- **Why not Corepack in CI.** Corepack is bundled with Node up to 24 and
  no longer ships with Node 25 and later. A CI template that relied on it
  would break on a routine Node bump. `pnpm/action-setup` is a maintained
  third-party action and survives that bump (principle 3).
- **An existing test's reasoning changes, not its assertion.**
  `internal/module/tstooling/tstooling_test.go`'s `TestRequiredTools`
  treats `npm` and `pnpm` as project-local and asserts `ts-tooling` does
  not require them. That assertion still holds: `ts-tooling` owns lint
  config, not entry points, and keeps requiring only `node`. The *comment*
  calling pnpm project-local becomes wrong once pnpm is the global entry
  point, and is corrected. The requirement moves to `ts-repo-tooling`,
  which gets its own test. Nothing is weakened.
- **Overlap with issue #26, item 8.** That item asks `ts-repo-tooling` to
  declare `npm`/`npx`. This spec supersedes it: the tool to declare is
  `pnpm`.
- **Windows.** `pnpm exec` resolves `.cmd` shims on Windows, so the
  Taskfile needs no per-OS branching. The plan should add `windows-latest`
  to the example's CI job, or record why not. Right now nothing proves the
  TS standard on Windows at all.

## Open questions for the plan

- Confirm that Dependabot's `npm` ecosystem updates `pnpm-lock.yaml` for
  the pnpm major version the example pins.
- Confirm the `pnpm/action-setup` version and SHA to pin, and that it
  reads `packageManager` when no `version` input is given.
- Whether `examples.yml`'s TypeScript job should also run on
  `windows-latest` (see Design notes).

## Follow-on work

- `prod-ts/v2` and a policy for retiring standard versions, when there is
  an external adopter to protect.
- Managing `packageManager` through `StructuredPatch`.
