# Plan 0048: ESLint lints every file the standard formats

Implements `docs/specs/0048-eslint-tsx.md`.
Status: approved 2026-10-15.

## Repository impact

| Path | Change |
|---|---|
| `internal/module/tstooling/templates/eslint.config.js` | typed set `**/*.{ts,tsx}`; untyped set `**/*.{js,mjs,jsx}` with JSX parsing; comments |
| `internal/module/tstooling/eslint_tsx_test.go` | new: the regression test (runs real ESLint) |
| `internal/module/tstooling/eslint_gitignore_test.go` | `runESLint` also returns the rule ids, for the new test |
| `.github/workflows/examples.yml` | the `examples/typescript` job runs `-run 'ESLint'`, so it picks up the new test |
| `examples/typescript/eslint.config.js`, `examples/monorepo/apps/web/eslint.config.js`, their `.vibe/state.yaml` | synced |
| `docs/usage.md` | D4 |
| `docs/specs/0048-*.md`, this plan | status, deviations |

The `eslintIgnores` anchor that `ignoreInESLint` replaces (spec 0037) is
not touched. `tstooling_test.go` asserts none of the globs, so it does
not change. `examples.yml` is not a managed file, so it is edited
directly. Dependency direction is unchanged.

## Template

```js
// Every .ts and .tsx file is type-checked. ...existing comment...
const typedFiles = ['**/*.{ts,tsx}']
```

and the JavaScript entry:

```js
{
  ...js.configs.recommended,
  files: ['**/*.{js,mjs,jsx}'],
  languageOptions: {
    globals: globals.node,
    parserOptions: { ecmaFeatures: { jsx: true } },
  },
},
```

`configFiles` stays `['**/*.config.ts', '**/*.config.*.ts']` (D3). A
comment above `typedFiles` says that a `.tsx` file must be in the
`tsconfig.json` include, and that `jsx` is set there, not in the base.

## Regression test

`TestESLintJSX`, built like `TestESLintGitignore`: a fixture directory
under `examples/typescript/node_modules/.cache`, the template under test
written into it, real ESLint run with `--format json`.

- **Typed `.tsx`.** The fixture holds a `tsconfig.json` with
  `"include": ["*.tsx"]`, `"jsx": "preserve"`, `"strict": true`, and
  `app.tsx`:

  ```tsx
  async function load(): Promise<void> {}
  export function App() {
    load()
    return <div />
  }
  ```

  Expected: `@typescript-eslint/no-floating-promises` on `app.tsx`.
- **Untyped `.jsx`.** `view.jsx`: `const unused = <div />`. Expected:
  `no-unused-vars` on `view.jsx`, and no parse error (a parse error comes
  back with no rule id, and the test treats it as a failure).
- **Control.** A `.ts` file with a floating promise is still reported, so
  the typed setup in the fixture is shown to work.

`runESLint` gains the rule ids per file. The old callers keep checking
for "any problem".

## Steps

1. Write the test. Confirm it fails on the current template, for the
   reason expected (no report for `.tsx` and `.jsx`, `.ts` control
   reported). Widen the `examples.yml` step from `-run ESLintGitignore`
   to `-run ESLint`.
2. Change the template and its comment. The test passes. `go test
   ./internal/module/tstooling/...`.
3. Commit the fix. Build `vibe` from a clean worktree at that commit,
   install it, and sync this repository and the three examples. Commit
   the sync. `task audit` in each, `task lint` and `task fmt:check` in
   `examples/typescript` and `examples/monorepo`.
4. `docs/usage.md`: next to the `tsconfig.base.json` bullet, D4: the
   adopter's `tsconfig.json` includes the `.tsx` files and sets `jsx` when
   they hold JSX. Spec and plan status. Commit.
5. `task verify`, `task audit`, push, PR "Fixes #91", CI.

## Risks

- **Typed linting of `.tsx` in an adopter whose `tsconfig.json` does not
  include them.** Under `projectService`, such a file becomes a parse
  error, as an excluded `.ts` file does today. That is louder than
  silence, and D4 documents it. The examples include only `.ts`, and
  hold no `.tsx`, so they are not affected.
- **prod-mono `apps/web`.** It gets the same template. It holds no `.tsx`
  either, so its lint result does not change.
