# Plan 0049: Base configs for ESLint and Ruff

Implements `docs/specs/0049-lint-base-configs.md` under ADR 0024.
Status: approved 2026-10-15.

## Probes (done 2026-10-15, before this plan)

- **A1 holds.** Ruff 0.16.8 in a scratch directory: `ruff.base.toml`
  with `extend-exclude = ["src/gen/**"]` and `force-exclude = true`,
  `ruff.toml` holding only `extend = "ruff.base.toml"`. `src/a.py` is
  reported and `src/gen/bad.py` is not, run from the directory, from its
  parent, and with the generated file passed explicitly. `[lint] ignore`
  in `ruff.toml` removes a rule and `extend-select` adds one.
- **A2 holds.** ESLint 10.11.0 in `examples/typescript/node_modules/.cache`:
  the template as `eslint.base.config.js`, a seed that spreads it. The
  floating promise in `src/a.ts` is reported; the same file under a
  `generated:` path and under a `.gitignore` entry is not. An entry after
  the base turning the rule off gives no problems.
- **A3 holds.** No task, hook or CI template names either config.

## Repository impact

| Path | Change |
|---|---|
| `internal/resource/resource.go` | new ownership `Seeded`; new field `Base` |
| `internal/cli/seed.go` (new) | `seedPlan`: planning, the base-reference check, the sync action, the lines for each command |
| `internal/cli/plan.go` | `planResource` dispatches `Seeded`; `holdBases` after planning |
| `internal/cli/sync.go`, `audit.go`, `diff.go` | apply and report seed plans; a held base is not written |
| `internal/module/tstooling/` | templates `eslint.base.config.js` (renamed), `eslint.config.js` (seed), `tsconfig.json` (seed); `Resolve` |
| `internal/module/pythontooling/` | templates `ruff.base.toml` (renamed, comment and anchor kept), `ruff.toml` (seed); `Resolve` |
| `internal/module/monotooling/` | nothing in code: it re-roots every resource; tests updated |
| tests | see Tests |
| `.github/workflows/examples.yml` | the `examples/python` job runs the new Ruff test |
| `examples/*` and their `.vibe/state.yaml` | synced through the migration |
| `docs/usage.md`, `docs/architecture/overview.md`, `docs/architecture/principles.md`, ADR 0003 | D13; the new ownership |
| `docs/specs/0049-*.md`, this plan | status, deviations |

Dependency direction is unchanged: `resource` gains a constant and a
field; the CLI reads them. State schema stays 4: a seeded file is never
recorded (D4). This repository is prod-go and resolves no seeded file.

## Resource model

```go
// Seeded resources belong to the repository. VibeConform writes Content
// (the seed) only when the file is missing, and otherwise only checks
// that the file still names Base (ADR 0024).
Seeded
```

`Resource.Base` is the file name of the base config the seeded file
must name, in the same directory (`eslint.base.config.js`,
`ruff.base.toml`, `tsconfig.base.json`). It is a name, not a path, so
monotooling's re-rooting needs no change. Empty for other ownerships;
`planResource` refuses a `Seeded` resource without it, and a `Base` on
any other ownership.

ADR 0003 gets a short amendment note: a fourth written ownership,
created once, under ADR 0024. `ProjectOwned` (never written) stays as
it is.

## Planning a seeded resource

`planSeed` reads the file (C) and the state record for its path (P, only
ever a `generated` record from before this version):

| File | Record | Outcome | sync | audit |
|---|---|---|---|---|
| missing | – | **create** | write the seed | drifted, `missing (run vibe sync)` |
| missing | yes | **create** | write the seed, drop the record | drifted |
| exists, C = P | yes | **convert** (D9) | write the seed, drop the record | out of date, `out of date (now yours: run vibe sync to convert it to extend …)` |
| exists, C ≠ P, names base | yes | **adopt** | drop the record only | out of date |
| exists, C ≠ P, no base | yes | **conflict** (D10) | keep file and record | conflict, with the resolution |
| exists, names base | – | **ok**, or **extended** if C ≠ seed | nothing | `ok (project-owned)` or `extended (project-owned; differs from the seed)`, not counted |
| exists, no base | – | **conflict** (D5) | nothing | conflict: `does not name <base>; …` |

"Adopt" is how a conflict resolves: once the adopter's edited file
imports the base, the next sync forgets the old record instead of
reporting the conflict forever. The conflict message (D10) reads:
`conflict: edited since vibe last wrote it; it is now yours and must
name <base>. Move your changes after the base (docs/usage.md, "Base
configs"), or delete the file and run vibe sync`. `tsconfig.json` has no
record anywhere, so only the last three rows reach it (D11).

"Names its base" is `bytes.Contains(content, []byte(Base))`, as ADR 0024
and the spec allow.

**Held base (D10).** After planning, `holdBases` marks the base resource
next to a seeded file in **conflict** as held: sync prints
`held: <seeded path> must be resolved first` and does not write it;
audit counts it as it would count it otherwise (missing, so drifted).
Without this, the base would land next to an edited standalone config
that never reads it.

`markIgnored` already covers every resolved resource, so a gitignored
seeded file is a conflict in audit and is not written by sync (D8).
`Supported` is true for seed plans.

**Output format.** The spec writes the info line as `extended  <path>`.
Every other audit line is `<path>: <status>`, so it is
`<path>: extended (project-owned; differs from the seed)`. Same
information, the repository's existing format.

## Templates

- `templates/eslint.base.config.js`: today's `eslint.config.js`,
  unchanged apart from a first comment naming it the base and pointing
  to `eslint.config.js`. `ignoreInESLint` keeps its anchor; its
  messages name the new file.
- `templates/eslint.config.js` (seed, Prettier-clean):

  ```js
  import base from './eslint.base.config.js'

  // The standard's rules are in eslint.base.config.js, which VibeConform
  // manages. Add your own entries after them: a later entry wins, so one
  // can add a plugin, add a rule or switch one off. Keep the import:
  // vibe audit fails without it, and reports this file as extended once
  // it differs from what vibe created (ADR 0024).
  export default [
    ...base,
  ]
  ```

- `templates/ruff.base.toml`: today's `ruff.toml`, its header saying it
  is the base that `ruff.toml` extends. `ruffAnchor` unchanged.
- `templates/ruff.toml` (seed): a comment in the same spirit, then
  `extend = "ruff.base.toml"`. Ruff settings in the file win;
  `[lint] extend-select` adds, `ignore` and `per-file-ignores` remove.
- `templates/tsconfig.json` (seed):
  `{ "extends": "./tsconfig.base.json", "include": ["src"] }`, formatted
  as Prettier writes it.

Resolve order, prod-ts: `eslint.base.config.js`, `eslint.config.js`,
`.prettierrc.json`, `tsconfig.base.json`, `tsconfig.json`, then the
conditional `.prettierignore` section (last, as `TestGeneratedSectionIsConditional`
expects). prod-py: `ruff.base.toml`, `ruff.toml`, `pyrightconfig.json`.

## Tests

- `resource`: `Seeded.String()`.
- `tstooling`, `pythontooling`, `monotooling`: resolved paths,
  ownerships and `Base`; LF and anchor tests on the renamed templates;
  `TestTypedFilesGlob…` and `TestESLintConfigLoadsGitignoreTolerantly`
  read the base. A test that each seed names its `Base`.
- ESLint, real (`TestESLint*`, run by `examples.yml`): the helpers write
  the base and the seed instead of one file, so `TestESLintGitignore`
  and `TestESLintJSX` now run through the seam. New `TestESLintSeam`:
  the floating promise is reported through the seed, and not once an
  entry after the base turns the rule off.
- Ruff, real (new `TestRuffSeam` in `pythontooling`, skipped without
  `examples/python/.venv`'s ruff, run by `examples.yml`'s python job with
  `-run Ruff`): a selected rule fires through the seed, a `generated:`
  path is skipped (also when passed explicitly), `extend-select` adds and
  `ignore` removes a rule.
- CLI (`seed_test.go`, temp repositories, as the existing sync and
  audit tests): every row of the planning table, for sync, audit and
  diff; a seeded file is never written once it exists, whatever it
  holds; a deleted one is recreated; a held base; a gitignored seed;
  state has no record for a seeded path after any sync; a second sync is
  a no-op.
- Migration end to end: a temp prod-ts repository synced with the
  previous resources (state records `eslint.config.js` as generated,
  content as today's template) converts on sync; the same with one
  edited line is a conflict, and resolving it by importing the base
  adopts it.
- `TestExamplesAreConformant` after the sync commit.
- A fresh prod-ts, prod-py and prod-mono repository with one source file
  passes `vibe sync`, `task verify` and `vibe audit` (acceptance 1),
  run by hand in the scratchpad; the ledger records it. `tsc` with the
  seed needs a `src` with one file in it.

## Commits

1. `docs: plan 0049` (this file, approved).
2. `feat: Seeded ownership and the planner, sync, audit and diff for it`
   with the CLI tests.
3. `feat: ESLint and Ruff base configs, seeded eslint.config.js, ruff.toml
   and tsconfig.json` with the module and real-tool tests and
   `examples.yml`.
4. `chore: sync the examples (spec 0049)`: vibe built from a clean
   worktree at commit 3. Expect the examples' `eslint.config.js` and
   `ruff.toml` to convert (A4); if any is a conflict, stop and report.
5. `docs: base configs in usage, overview, principles, ADR 0003; spec
   0049 status`.

Then `task verify`, `task audit` here and in the three examples, push,
and a PR for #80 (not closing it; spec 0050 follows).
