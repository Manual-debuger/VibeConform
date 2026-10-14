# Plan 0045: The generated ESLint config honors `.gitignore`

Implements `docs/specs/0045-eslint-honors-gitignore.md`.
Status: approved 2026-10-14.

## Repository impact

| Path | Change |
|---|---|
| `internal/module/tstooling/templates/eslint.config.js` | load `includeIgnoreFile` tolerantly, add the `.gitignore` ignores |
| `internal/module/tstooling/eslint_gitignore_test.go` | new: the regression test (runs real ESLint) |
| `internal/module/tstooling/tstooling_test.go` | content assertions for the new lines |
| `.github/workflows/examples.yml` | run the regression test in the `examples/typescript` job |
| `examples/typescript/.gitignore` | `.claude/worktrees/` outside the managed section, as an adopter would add it |
| `examples/typescript/eslint.config.js`, `examples/monorepo/apps/web/eslint.config.js`, their `.vibe/state.yaml` | synced |
| `docs/usage.md` | D1–D3, D5, D6 |
| `docs/specs/0045-*.md`, this plan | status, D6 result |

`examples.yml` is not a managed file, so it is edited directly. The spec
0037 anchor line (`eslintIgnores`) is not touched, so `ignoreInESLint`
and its tests stay as they are. Dependency direction is unchanged.

## Template

Added after the existing imports, before `typedFiles`:

```js
import { existsSync } from 'node:fs'
import path from 'node:path'

// ESLint ignores what the repository's .gitignore ignores, as Prettier
// does (spec 0045), so an agent worktree under .claude/worktrees/ is not
// linted from the main checkout once .gitignore lists it. eslint/config
// exports includeIgnoreFile from ESLint 10.4.0; on an older ESLint, or
// without a .gitignore, nothing is added and the config lints as before.
const { includeIgnoreFile } = await import('eslint/config').catch(() => ({}))
const gitignore = path.join(import.meta.dirname, '.gitignore')
const gitignored = includeIgnoreFile && existsSync(gitignore) ? [includeIgnoreFile(gitignore)] : []
```

`...gitignored,` goes right after the existing global `ignores` object.
A dynamic import, not a static one: a static named import of a missing
export fails the whole config on ESLint < 10.4.0 (spec, Constraints).
Final formatting follows `prettier --check` in the example.

## Regression test

`eslint_gitignore_test.go`. It is skipped when `pnpm` is not on `PATH`
or `examples/typescript/node_modules` is absent, as the guard-corpus
tests skip a missing runtime. So `ci.yml`'s Go job skips it and the
`examples/typescript` job runs it.

It writes the module's resolved `eslint.config.js`, the template under
test rather than the synced copy, into `examples/typescript` under a
unique name, and runs `pnpm exec eslint . --no-cache -c <that file> -f
json`. The names are unique per run, and `t.Cleanup` removes everything
the test writes. It uses `.mjs` files with an unused variable, so the
result does not depend on the TypeScript project service.

- **gitignored**: write `bad.mjs` under `.claude/worktrees/<unique>/` and a
  control `<unique>-bad.mjs` at the root. Expect the control to report
  `no-unused-vars` (ESLint ran) and nothing to be reported from the
  worktree file. Today both are reported: red.
- **no .gitignore** (D2): copy the config into a fresh subdirectory with
  no `.gitignore`, put `bad.mjs` in a `.claude/worktrees/` below it, and
  run from that subdirectory. Expect `bad.mjs` to be reported, and the
  config to load.

## Steps (one commit each, in order)

1. Commit this plan (the spec is already in `d998d59`).
2. **Red.** Add the regression test and the `examples/typescript/.gitignore`
   line. Run the test and record that the gitignored case fails. Do not
   commit yet.
3. **Green.** Change the template and the content assertions in
   `tstooling_test.go`. The regression test passes. Commit steps 2 and 3
   together, so no commit on the branch has a failing test. The red run
   goes in the PR description.
4. **CI.** Add the `go test ./internal/module/tstooling -run ESLintGitignore`
   step to the `examples/typescript` job, after `pnpm install`, on both
   Linux and Windows.
5. **Sync.** Build `vibe` from a clean worktree at the step 3 commit.
   `vibe sync` in `examples/typescript`, `examples/monorepo` and this
   repository (this one is unaffected; its state only). Run `task lint` in
   both examples and `task audit` in each. Commit as
   `chore: sync this repository and the examples (spec 0045)`.
6. **D6 by hand.** In a scratch copy of `examples/typescript` outside the
   repository, `pnpm add -D eslint@10.3.0`, run `eslint .`, and confirm
   the config loads and lints as before. Record the result in the spec.
   Then repeat the spec's real-worktree probe in the example: expect 0
   worktree files linted.
7. **Docs.** `docs/usage.md`, in the `prod-ts` section: D1–D3, the
   `.gitignore` advice (D5) and the 10.4.0 minimum (D6). Spec and plan
   status: implemented.
8. `task verify`, `task audit`, push, open the PR, and wait for CI,
   including both `examples/typescript` jobs.

## Verification ledger (to fill in)

- Unit tests (`task verify`)
- Regression test, local (Windows)
- Regression test, CI (Linux and Windows `examples/typescript`)
- `task audit` in this repository and both examples
- D6, ESLint 10.3.0, by hand
- Real-worktree probe
