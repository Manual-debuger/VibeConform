# Spec 0049: Base Configs for ESLint and Ruff

Status: accepted and implemented (2026-10-15). Issue #80 (the umbrella), the second step after spec
0048. Settled with the user on 2026-10-15 in the grilling of #80
(Q1–Q18). Decided by ADR 0024.

## Problem

VibeConform owns `eslint.config.js` and `ruff.toml` as whole generated
files. An adopting repository that needs one more rule, a framework
plugin (React hooks, JSX a11y), browser globals, or one rule switched
off for a directory must edit the managed file. `vibe audit` then fails
on it, and the next `sync` reports a conflict. Its only other choice is a
second config, a task and a CI job beside the standard, which nothing in
`task verify` or the agent hooks runs (spec 0048 describes one).

`tsconfig.json` already has the shape that works: the standard owns
`tsconfig.base.json`, and the repository owns a `tsconfig.json` that
extends it. But VibeConform does not create that `tsconfig.json`, so a
fresh prod-ts repository has no TypeScript project until the adopter
writes one from `docs/usage.md`.

## Constraints

- **Native mechanisms only** (principle 3, ADR 0024). The repository's
  file extends the base with the tool's own mechanism: an ES module
  import for ESLint, `extend` for Ruff, `extends` for TypeScript. No
  merge patches and no `.vibe/overrides/`. Removing VibeConform leaves
  working configs.
- **ESLint reads only `eslint.config.js`** (and its `.mjs`/`.cjs`/`.ts`
  variants) by default; the repository's file must have that name.
- **Ruff's `extend` names a file that must exist.** A missing base is an
  error from Ruff, not a silent fallback.
- **The floor is fixed** (ADR 0024). `task verify`, `task audit`, the
  agent hook tasks and the CI floor still run the same commands on the
  same files. A seam changes which rules apply, not whether lint runs.
- **One base per tool serves prod-ts/prod-py and each prod-mono
  component of that profile, byte for byte**, as the whole files did
  (spec 0045 and earlier), with the `generated:` exclusions added as
  today (spec 0037).
- **Spec 0045** (`.gitignore` read next to the config) and **spec 0048**
  (the file sets) keep working.
- **ADR 0003:** VibeConform never rewrites a file the repository owns.
  ADR 0013 §6: nothing unrecorded is ever deleted, and a file modified
  since it was recorded is kept and reported.
- Windows and Linux alike (principle 2).

## Assumptions

Each was probed before the plan (plan 0049, "Probes"), and holds.

- **A1.** Ruff resolves `extend-exclude` paths in an extended file
  relative to that file's directory, so the `generated:` exclusions in
  `ruff.base.toml` still match when `ruff.toml` extends it. If Ruff
  resolves them relative to the extending file, the result is the same,
  because both files sit in the same directory.
- **A2.** `import.meta.dirname` in `eslint.base.config.js` is the
  directory of the base, which is also the directory of
  `eslint.config.js`, so `tsconfigRootDir` and the `.gitignore` lookup
  are unchanged.
- **A3.** No task, hook or CI job names `eslint.config.js` or
  `ruff.toml` with `--config`; each tool finds its config by name.
  Checked by `grep` on 2026-10-15: no generated file passes either name.
- **A4.** The examples' managed `eslint.config.js` and `ruff.toml` are
  unchanged since their last sync, so they convert without a conflict.

## Desired Behavior

### Base configs

- **D1.** prod-ts (and each prod-mono TypeScript component) gets a
  managed `eslint.base.config.js` holding today's rules, file sets,
  ignores and `generated:` exclusions. prod-py (and each prod-mono
  Python component) gets a managed `ruff.base.toml` holding today's
  settings and exclusions. Both are generated files with the usual drift
  rules. `tsconfig.base.json` is unchanged.
- **D2.** In prod-mono, each component's base and repository file sit in
  the component's directory, e.g. `apps/web/eslint.base.config.js` and
  `apps/web/eslint.config.js`.

### The repository's own file

- **D3. Created once.** When `eslint.config.js`, `ruff.toml` or
  `tsconfig.json` is missing where the base is, `sync` creates it with a
  minimal content that only extends the base (the seed). Once it exists,
  VibeConform never writes it again, whatever it holds. If it is deleted,
  the next `sync` creates the seed again. The seeds:
  - `eslint.config.js` imports the base and exports its entries, with a
    place after them for the repository's own entries, which win;
  - `ruff.toml` sets `extend = "ruff.base.toml"` and nothing else;
  - `tsconfig.json` extends `./tsconfig.base.json` and includes `src`,
    as `docs/usage.md` already tells the adopter to write it.
- **D4.** `.vibe/state.yaml` records no hash for these files; they are
  not managed files.

### Audit

- **D5. Fail: the base is no longer referenced.** `vibe audit` (and
  `diff`, `sync`) reports a conflict when the repository's file no longer
  names its base. This is a text check, like the
  `.gitlab-ci.defaults.yml` check (ADR 0021): it stops an accident, not a
  determined maintainer.
- **D6. Info: extended.** When the file differs from its seed, audit
  prints a non-failing `extended  <path>` line, so a reviewer sees that
  the repository changed the rules there. Audit does not compare rule by
  rule: `eslint.config.js` is a program.
- **D7.** A missing repository file counts as drifted, as a missing
  managed file does: `sync` would create it.
- **D8.** A repository file that Git ignores is a conflict, as a
  gitignored managed path is (ADR 0013 §7): a fresh checkout would lack
  it.

### Moving existing adopters over

- **D9.** On the first `sync` with this version, a managed
  `eslint.config.js` or `ruff.toml` that still hashes to its recorded
  value is replaced by the seed, the base is written next to it, and the
  state record is dropped. The repository's lint result is unchanged.
- **D10.** One that was edited since it was recorded is kept and
  reported as a conflict; the base is not written next to it. The
  conflict message and `docs/usage.md` say how to resolve it: move the
  edits into a seed-shaped file below the base import, or delete the file
  and sync.
- **D11.** A `tsconfig.json` that exists is never touched, migrated or
  recorded. If it does not name `tsconfig.base.json`, D5 reports it.

### Documentation

- **D12.** ADR 0024 records the rule: rule content may be extended and
  weakened through a seam, and audit reports it; the floor may not be
  changed through any seam. `GLOSSARY.md` defines Floor, Base config and
  Guard exception, and CI floor as the floor's part in CI.
- **D13.** `docs/usage.md` documents the three files: what the base
  holds, what the seed looks like, how to add and to switch off rules,
  the `extended` line, the conflict on a lost reference, and the
  migration. The principles table cites the base configs under
  principle 3.

## Non-goals

- **Seams for golangci-lint, Prettier and pyright** (Q3). golangci-lint
  has no `extends`; the others wait for an adopter who asks.
- **The guard seam** (`policy.local.json`, guard exceptions): spec 0050.
- **An extra check in `verify`**: a later spec, after a probe of Task's
  optional includes.
- **Rule-by-rule reporting** of what a repository weakened.
- **Deleting the base when a module is switched off** (Q17). Lint modules
  are part of a standard, not optional integrations, and ADR 0013 §6
  deletes only a deselected integration's files. Nothing can switch
  `ts-tooling` or `python-tooling` off today, and a removed prod-mono
  component's files are left in place like every other orphan. Q17's
  rule (delete the base, keep the repository's file, warn) applies once
  a deselection path exists; this spec adds none (confirmed with the
  spec's approval).
- `.mts`/`.cts`, browser globals, framework plugins: the seam is where
  an adopter adds them.

## Acceptance Criteria

- [x] A fresh prod-ts, prod-py and prod-mono repository gets the bases
      and the seeds; `task verify` and `vibe audit` pass in it, and the
      lint result equals today's whole-file configs on the same sources.
- [x] A test runs the real ESLint on a fixture with the seed and the
      base: a typed rule, an untyped rule and a `generated:` exclusion
      behave as today; an entry added after the base turns a rule off.
- [x] A test runs the real Ruff on a fixture with the seed and the base:
      a selected rule fires, a `generated:` path is skipped, and an
      `extend-select`/`ignore` in `ruff.toml` adds and removes a rule.
- [x] `sync` creates a missing seed, recreates a deleted one, and never
      writes an existing file (tests for each file).
- [x] Audit fails on a repository file that no longer names its base,
      prints `extended` for one that differs from its seed, counts a
      missing one as drifted, and reports a gitignored one as a conflict.
- [x] Migration: an unchanged managed `eslint.config.js`/`ruff.toml` is
      converted and its state record dropped; an edited one stays and is
      a conflict with the resolution in the message.
- [x] The three examples are synced through the migration and
      conformant; this repository is unaffected (prod-go).
- [x] ADR 0024, `GLOSSARY.md`, `docs/usage.md` and the principles table
      carry D12 and D13.
- [x] `task verify` and `task audit` pass, here and in the examples.
