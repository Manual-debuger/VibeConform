# Spec 0048: ESLint Lints Every File the Standard Formats

Status: superseded by ADR 0025 (removed in spec 0059). Before that: accepted and implemented (2026-10-15). Issue #91. The first step of #80, settled with the user
on 2026-10-15 in the grilling of #80. A file the standard treats as
source is not left to an extension seam. It gets its own release tag.

## Problem

The managed `eslint.config.js` (prod-ts, and each TypeScript component
in prod-mono) gives ESLint three file sets:

- `**/*.ts` with the typed rules;
- `**/*.{js,mjs}` with the untyped rules;
- `**/*.config.ts` and `**/*.config.*.ts` with the untyped rules.

ESLint lints a file only if a `files` entry matches it, or if it is one
of ESLint's defaults (`*.js`, `*.mjs`, `*.cjs`). Checked on 2026-10-15
in `examples/typescript` with ESLint 10.11.0: `eslint --print-config`
returns `undefined` for `src/a.tsx`, `src/a.jsx` and `src/a.mts`, so
those files are linted by nothing. `task lint` still exits 0.

The rest of the standard already treats `.tsx` and `.jsx` as source:

- `task fmt` and `task fmt:check` run Prettier on
  `**/*.{js,jsx,ts,tsx,json,css,md}`;
- the `hook:format` agent hook formats `*.jsx` and `*.tsx`;
- the lefthook pre-commit `lint` command runs on `*.{js,jsx,ts,tsx}`,
  and ESLint then ignores the `.tsx` and `.jsx` files it started for.

An adopting prod-mono repository added a second ESLint config, a task
and a CI job to lint `.tsx`. None of these run in `task verify` or the
agent hooks.

## Constraints

- One `eslint.config.js` serves prod-ts and every prod-mono TypeScript
  component, byte for byte (spec 0045 and earlier).
- The comment in the template stays true: a file outside the typed set
  must not silently drop out of linting.
- A file the typed rules lint must be in a tsconfig project
  (`projectService`). That is the case for `.ts` today and stays the
  adopter's choice through the `include` in their `tsconfig.json`.
- The base `tsconfig.base.json` sets no JSX runtime. `jsx` (`react-jsx`,
  `preserve`, or a `jsxImportSource`) is a framework choice. The adopter
  sets it in their own `tsconfig.json`, next to `include`.
- Spec 0045's `.gitignore` handling and the `generated:` ignores keep
  working.
- Every bug fix needs a regression test (AGENTS.md).

## Assumptions

- typescript-eslint's parser reads JSX in a `.tsx` file from its
  extension, so the typed set needs no extra parser option. Verified by
  the regression test with ESLint 10.11.0: no parse error on `app.tsx`.
- ESLint's default parser (espree) reads JSX in a `.jsx` file only with
  `ecmaFeatures.jsx` set. The config sets it; the regression test shows
  `view.jsx` parses.

## Desired Behavior

- **D1. Typed TypeScript.** `.ts` and `.tsx` files get the typed rules.
- **D2. Untyped JavaScript.** `.js`, `.mjs` and `.jsx` files get the
  untyped recommended rules, and a `.jsx` file with JSX in it parses.
- **D3. Config files.** A `.tsx` file never counts as a tool config, so
  the config-file set does not change.
- **D4. Docs.** `docs/usage.md` says that the adopter's `tsconfig.json`
  must include the `.tsx` files and set `jsx` when they hold JSX.

## Non-goals

- `.mts` and `.cts` files. Nothing in the standard (Prettier, hooks,
  lefthook) handles them today. Adding them everywhere is a separate
  change.
- `.cjs` handling. ESLint lints it by default; the standard does not
  format it. Unchanged.
- Browser globals or framework plugins (React hooks, JSX a11y). Those are
  rule content, for the #80 lint seam (spec 0049).
- A JSX setting in `tsconfig.base.json`.

## Acceptance Criteria

- [x] A regression test runs the real ESLint on a fixture, in the way of
      spec 0045's test. It fails before the change and passes after it:
  - a `.tsx` file with JSX and a floating promise is reported for
    `@typescript-eslint/no-floating-promises`;
  - a `.jsx` file with JSX and an unused variable is reported for
    `no-unused-vars`.
- [x] `.ts` and `.js` behaviour is unchanged: the existing tests pass.
- [x] `docs/usage.md` carries D4.
- [x] This repository and the three examples are synced and conformant.
- [x] `task verify` and `task audit` pass.
