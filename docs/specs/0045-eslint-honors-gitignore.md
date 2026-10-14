# Spec 0045: The Generated ESLint Config Honors `.gitignore`

Status: accepted (2026-10-14). Issue #87. Settled with the user on 2026-10-14, after #85
was closed: keeping `.gitignore` up to date is the adopting repository's
job, and the generated tools should honor it. Followed by the fixes for
#82 and #84. One tag after all three are merged.

## Problem

In a `prod-ts` repository, `task lint` runs `eslint .` from the
repository root. ESLint flat config does not read `.gitignore`, and the
generated `eslint.config.js` ignores only `**/dist/**`, `**/coverage/**`
and the generated paths (spec 0037). So ESLint lints everything the
repository ignores, including Claude Code worktrees at
`.claude/worktrees/<name>/`. Each worktree is a full checkout.

Probed on `v0.9.0-alpha.1` in `examples/typescript` with a real
worktree: `eslint .` linted 10 files, and 7 of them were in the worktree.
That stayed true with `.claude/worktrees/` in `.gitignore`. Prettier,
which reads `.gitignore`, skipped the worktree once the line was there.

The result: an agent's work in progress in a worktree can fail the main
checkout's `task lint`, and with it `verify`, CI-equivalent local runs
and the `Stop` hook. The adopter cannot fix this through `.gitignore`.
The only way is to edit the managed `eslint.config.js`, which is drift.

## Constraints

- `eslint.config.js` is a generated resource of `ts-tooling`. It is
  shared by `prod-ts` and by the TypeScript components of `prod-mono`.
- No new production dependency, and no new dev dependency in the
  adopter's `package.json`. ESLint core exports `includeIgnoreFile` from
  `eslint/config` since **ESLint 10.4.0** (2026-05-15). This was checked
  against the published packages: 10.0.0 to 10.3.0 and every 9.x export
  only `defineConfig` and `globalIgnores`, and 9.21 and older have no
  `eslint/config` at all.
- The standard pins no ESLint version (`docs/usage.md`). The config must
  load on every ESLint version that the standard supports. So a static
  `import { includeIgnoreFile }` is not allowed: on ESLint older than
  10.4.0 it fails the import, and with it every lint run.
- Spec 0037's generated-path ignores and their anchor stay as they are.
- The adopter owns `.gitignore` (the decision on #85). VibeConform does
  not add lines to it for this.
- Every bug fix needs a regression test (AGENTS.md).

## Assumptions

- `prod-mono` is not affected. Its components lint from their own
  directories, and the worktrees are at the repository root. A component
  directory seldom has its own `.gitignore`. When it has none, nothing
  changes there (see D2).
- Prettier 3 reads only `.gitignore` and `.prettierignore` in its working
  directory. It does not read `.git/info/exclude` or global excludes.
  ESLint now matches that.

## Desired Behavior

- **D1.** The generated `eslint.config.js` ignores what the root
  `.gitignore` next to it ignores, as global ignores. The existing
  `dist`, `coverage` and generated-path ignores stay.
- **D2.** Without a `.gitignore` next to `eslint.config.js`, the config
  loads and behaves exactly as before.
- **D3.** Only that one `.gitignore` is read. Nested `.gitignore` files,
  `.git/info/exclude` and the global excludes file are not read. This is
  the same scope as Prettier's.
- **D4.** In a `prod-ts` repository with `.claude/worktrees/` in its
  `.gitignore`, `eslint .` lints no file under `.claude/worktrees/`.
- **D5.** `docs/usage.md` states D1 to D3 and D6. It tells adopters who
  use agent worktrees to put `.claude/worktrees/` in `.gitignore`, not in
  `.git/info/exclude`, because neither ESLint nor Prettier reads that
  file.
- **D6.** D1 needs ESLint 10.4.0 or newer. On an older ESLint, the
  generated config still loads and lints exactly as before this spec.
  The fallback lints more than intended, never less, and it is silent.
  `docs/usage.md` names the minimum version.

## Non-goals

- Adding `.claude/worktrees/` or any other line to `.gitignore` (#85,
  closed).
- Hard-coding worktree paths in the ESLint ignores.
- Reading nested `.gitignore` files or `.git/info/exclude`.
- Any change to Prettier, `tsc` or the test runner. They are unaffected
  or already honor `.gitignore`.
- `prod-mono`-specific behavior beyond what the shared template gives.

## Acceptance Criteria

- [ ] A regression test fails before the fix and passes after it. In a
      `prod-ts` layout with a gitignored directory that holds a file
      with a lint error, `eslint .` with the generated config reports
      nothing from that directory.
- [ ] With no `.gitignore`, the generated config loads and lints as
      before. A test covers this.
- [ ] The existing `ts-tooling` tests for generated paths (spec 0037)
      pass unchanged.
- [ ] D6 is checked once by hand: the generated config loads under
      ESLint 10.3.0 in a scratch copy of `examples/typescript`, and
      `eslint .` lints as before. The result is recorded here. CI does
      not repeat this, because it installs only the pinned ESLint.
- [ ] `docs/usage.md` names ESLint 10.4.0 as the minimum for D1.
- [ ] `examples/typescript` and `examples/monorepo` are synced and
      conformant, and their `task lint` passes.
- [ ] The probe from the Problem section, repeated with a real worktree
      and the `.gitignore` line, shows 0 worktree files linted.
- [ ] `docs/usage.md` documents D1 to D3, D5 and D6.
- [ ] `task verify` and `task audit` pass.
