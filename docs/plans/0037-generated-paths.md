# Plan 0037: Declared generated-code paths

Implements `docs/specs/0037-generated-paths.md`. Status: approved and
implemented. Open questions resolved 2026-10-02: pyright still checks
generated code, Go rejects the key, the strict grammar stays, and the
doctor checks are a follow-on.

Step-0 probe record (2026-10-04, Windows; ruff 0.16.8, ESLint 10.11.0,
Prettier 3.9.8, golangci-lint v2.13.2):

- ruff: `extend-exclude` is anchored at `ruff.toml`. `force-exclude`
  skips explicitly passed files, with exit 0 and a "No Python files
  found" warning. `**` crosses directories, and **so does `*`**, which
  contradicts A1. The spec's grammar was narrowed to whole-segment `**`
  (user decision).
- ESLint: global `ignores` are relative to the config. An explicitly
  passed ignored file warns with exit 0, and `--no-warn-ignored` silences
  it. `*` stays within one directory.
- Prettier: `.prettierignore` skips explicitly passed files for
  `--check` and `--write`, with exit 0.
- golangci-lint: a file with the generated header and an `errcheck`
  finding passes, a hand-written one fails, and `gofmt -l` lists a
  misformatted generated file.

Branch: `feat/generated-paths`, off `main`. One pull request.

## Resolved design questions

**Where does the key live?** It lives on `manifest.Component` as
`Generated []string \`yaml:"generated,omitempty"\``, and on `Manifest` as
a top-level `Generated []string` for the single-language standards.

- A top-level key was chosen over a `development:` entry.
  `development:` holds single-valued option names (`ScalarKeys`), and
  this is a free-form list. It describes the code, not how people work
  on it.
- The per-component and root forms share one validator,
  `validateGenerated(where string, globs []string) error`, in
  `internal/manifest`, beside `validatePath`.

**Which checks know about the standard?** `manifest.Parse` validates
only the grammar (spec §2). It knows profiles, so it also rejects
`generated` on a `profile: go` component. The standard-dependent checks
belong with `checkComponents` in `internal/cli/plan.go`, as a new
`checkGenerated(s, m)`:

- top-level `generated` under a `TakesComponents` standard;
- top-level `generated` under `prod-go`, decided by `s.Profile == go`.

**How do tooling modules see the globs?** `module.Context` gains
`Generated []string`. buildPlan sets it from `m.Generated` for a
single-language standard.

- `monotooling` today resolves each tooling module with a nil context.
  Instead, it passes `&module.Context{Generated: c.Generated}` per
  component.
- A nil context or nil `Generated` keeps today's bytes. That keeps every
  existing isolated-module test valid, and it is the AC1 baseline.

**How is each file rendered?** The same way spec 0026 §10 and spec 0035
edit embedded templates: exact-anchor insertion through a
`replaceOnce`-style helper that refuses a template where the anchor is
missing or repeated. No template becomes a `text/template`, so the
default bytes stay the embedded file itself.

- `ruff.toml`: insert after the anchor `line-length = 100\n`:
  ```toml

  # Generated code (vibe.yaml generated:): not formatted or linted, still
  # type-checked. force-exclude applies it to files passed explicitly too.
  extend-exclude = ["src/worker/contracts/**"]
  force-exclude = true
  ```
- `eslint.config.js`: the anchor is `ignores: ['**/dist/**', '**/coverage/**'],`.
  The patterns are appended, each one single-quoted. The grammar admits
  no quote or backslash, so no escaping is needed.
- `.prettierignore`: a new `resource.ManagedSection` from `tstooling`,
  with section ID `generated`, `HashComment` markers and `Bottom`
  placement. Its content is a one-line comment followed by one pattern
  per line. It is resolved only when `Generated` is non-empty.
- `prod-ts` `lefthook.yml` (`tsrepotooling`): the anchor
  `run: pnpm exec eslint {staged_files}` becomes
  `run: pnpm exec eslint --no-warn-ignored {staged_files}`, only when
  `Generated` is non-empty.
  - `prod-mono`'s lefthook runs `task lint` (`eslint .`), which passes no
    explicit files, so it needs no change.
  - The `prod-py` lefthook needs none either, because `force-exclude`
    covers explicitly passed files.

**How does a section go away when the declaration does?** Today a
section is pruned only when its option is deselected (`planPrunes`) or
when it moved (`module.SectionMover`). An emptied `generated:` list, or a
removed component, is neither. Without a new rule, the recorded section
would become an unpruned orphan: a stale exclusion that nobody declares.

- New optional interface in `internal/module`:
  ```go
  // ConditionalSectioner is implemented by a core module that resolves
  // a managed section only for some vibe.yaml values. A recorded section
  // with one of these IDs that the current plan does not resolve is
  // removed under the rules for a deselected option's section.
  type ConditionalSectioner interface {
      ConditionalSections() []resource.Resource // ID, Markers, Placement; no Path
  }
  ```
- A new `planConditional` in `plan.go`, called next to `planMoved`,
  walks the selected modules. For every recorded section with a declared
  ID that has no entry in `sections`, it adds a prune with the cause
  `"no longer declared in vibe.yaml"`. It reuses `prunePlan`/`sectionPlan`
  exactly as `planMoved` builds them.
- `tstooling` and `monotooling` (which wraps it) both implement it.
  `monotooling` must forward it, because the planner sees the wrapper,
  not `tstooling`.

**Why the probes come first.** Assumptions A1–A4 decide the rendered
syntax (`force-exclude`, `--no-warn-ignored`, Prettier's handling of
explicit files). If any of them fails, the spec changes before any code
does.

## Steps

0. **Probe** (scratch directory, not committed). Use the tool versions
   pinned by `examples/` (ruff and pyright from
   `examples/python/uv.lock`; eslint and prettier from
   `examples/typescript/pnpm-lock.yaml`; golangci-lint at CI's pin).
   - For each of ruff format/check, prettier --check/--write and eslint:
     run on `.` and on an explicit generated file, and record the output
     and exit codes (A1–A3, AC8).
   - Check `*` vs `**` anchoring with a nested file under
     `src/gen/sub/`.
   - golangci-lint on a file with the standard header and a deliberate
     `errcheck` finding; gofmt on a misformatted generated file (A4, AC9).
   - Record the results in the PR description and in
     `docs/usage.md`'s "verified with" line. Stop and report if any
     assumption fails.
1. **Manifest.**
   - Add the fields and `validateGenerated`, and call it from
     `validateComponents` and `Parse`.
   - Reject `generated` on `profile: go`.
   - Tests: table cases for every row of spec §2, and the valid forms
     (AC2).
2. **Plan checks.** `checkGenerated` in `plan.go`, with tests for the
   three AC3 errors and their exit code 1.
3. **Context plumbing.**
   - Add `module.Context.Generated`, set in `buildPlan`.
   - Change `monotooling` to pass a per-component context.
   - `module.GeneratedOf(mctx)` is a nil-safe accessor, like
     `ComponentsOf`.
4. **Rendering.**
   - The `pythontooling` ruff edit.
   - The `tstooling` eslint edit and `.prettierignore` section.
   - The `tsrepotooling` lefthook flag.
   - Each gets a byte-identity test for nil/empty (AC1) and a golden
     test for the §4 example (AC4, AC5). The repository has no TOML
     decoder and this adds none. The ruff test checks instead that both
     keys come before the first `[` table header, which is what makes
     them top-level. `ruff` itself parses the file in the AC7 example
     job.
   - A refusal test for a template that lacks its anchor.
5. **Pruning.**
   - `module.ConditionalSectioner`, `planConditional`, and the
     `tstooling`/`monotooling` implementations.
   - Section tests in `internal/cli/section_test.go`, mirroring
     `TestSectionPrune`: emptied list, removed component, modified
     section kept as a conflict, section-only file deleted, and user
     lines kept (AC6).
6. **ADR 0018**, "Generated-code paths in `vibe.yaml`". It amends ADR
   0012's "exactly three fields" table and records the following:
   - the grammar;
   - the format/lint versus typecheck rule;
   - the Go header decision;
   - why header detection was rejected for Python and TypeScript;
   - why a section in `.prettierignore` was chosen over CLI negation.

   It also notes that this is a declaration about the code, not one of
   the "per-component overrides" that spec 0025 lists as a non-goal.
7. **Example (AC7).**
   - `examples/monorepo/vibe.yaml`: `generated:` for `web`
     (`src/contracts/**`) and `worker` (`src/example/contracts/**`).
   - Commit one generated file in each. Write it as the generator would,
     with its header, deliberately not Prettier- or ruff-formatted, and
     with a lint finding (unused import, `any`). It must type-check, and
     it is imported by the existing code so that typecheck actually
     covers it.
   - Rebuild, run `vibe sync --repo-root examples/monorepo`, then
     `vibe audit`. Commit the files and the state.
   - `examples.yml` monorepo job: after `task verify`, add a step that
     copies each generated file to a non-generated path and asserts that
     `task fmt:check` and `task lint` now fail. This is the "did not
     stop covering everything" guard.
   - Run `task hook:format` with the generated file touched, and assert
     that it leaves the file unchanged.
8. **Docs.**
   - `docs/usage.md`: "What `vibe.yaml` means today", the prod-mono and
     prod-ts/py sections (the key, the grammar, the §3 table, the Go
     header rule, removal, the regenerate-and-diff `Taskfile.local.yml`
     recipe), and "Removing VibeConform" (the `.prettierignore` section
     stays valid configuration).
   - `docs/architecture/overview.md`: the components paragraph and the
     `ConditionalSectioner` line under managed sections.
   - Set the spec's status to implemented.

## Repository impact analysis

| Area | Files | Change | Risk |
|---|---|---|---|
| Manifest schema | `internal/manifest/manifest.go`, `manifest_test.go` | two fields, one validator | Strict decoding: an older `vibe` rejects a `vibe.yaml` using the key (exit 1, loud). `task audit` pins `vibe_version`, so CI uses the right binary. |
| Plan | `internal/cli/plan.go`, new tests | `checkGenerated`, `planConditional` | `planConditional` touches pruning; scoped to declared IDs, so recorded sections of other modules are unaffected. |
| Module API | `internal/module/module.go` | `Context.Generated`, `GeneratedOf`, `ConditionalSectioner` | additive; nil-safe |
| Tooling modules | `pythontooling`, `tstooling`, `tsrepotooling`, `monotooling` (+ tests) | anchor edits, new section resource, per-component context | Anchors tie the code to template text. The refusal tests catch template edits that break them. |
| Untouched | `gotooling`, `repotooling`, `pyrepotooling`, `monorepotooling` templates, all CI modules, `conformance`, agents, editors | none | Go output is unchanged by design. |
| Generated output, existing adopters | every standard | **none** (AC1) | covered by byte-identity tests and by unchanged examples / self-audit |
| Generated output, opting in | `ruff.toml`, `eslint.config.js`, `.prettierignore` (new section), `prod-ts` `lefthook.yml` | as spec §4 | `.prettierignore` may already exist: the section is appended and user lines are kept. |
| State | `.vibe/state.yaml` | a new section record when opted in; schema unchanged (4) | none |
| Examples / CI | `examples/monorepo/**`, `.github/workflows/examples.yml` | opt-in fixture, negative-control step | Windows: the copy-and-expect-failure step must use Task's shell or `pwsh`-free commands. |
| Docs | `docs/usage.md`, `docs/architecture/overview.md`, ADR 0018, spec 0037 status | sync | none |
| Dependency direction | `internal/module` → `internal/manifest`/`resource` only; `internal/manifest` imports nothing new | preserved | none |
| Dependencies | none added | — | — |

The adopter case (prod-mono) after this change: `worker` and `web`
declare their contract paths and stop post-formatting. `api` changes
nothing, since its header already exempts it from golangci-lint, and
its generator must keep emitting gofmt output (it already does).

## Tests (by acceptance criterion)

1. Byte identity: a table over the four standards × {absent, `[]`}
   compares against a nil-`Generated` baseline. The examples stay
   conformant, and `task audit` here is clean.
2. Manifest: table-driven `TestParseGenerated` /
   `TestParseRejectsGenerated`.
3. `TestCheckGenerated` in `internal/cli`, with exit codes through
   `exit_test` patterns.
4. and 5. Golden tests per module, plus the structural ruff check from
   step 4. Real parsing of `ruff.toml` and `eslint.config.js` happens in
   the example's `task verify` in CI.
6. Section tests in `internal/cli`.
7. `examples.yml` monorepo job, Ubuntu and Windows.
8. and 9. The step-0 probe record. The AC7 job exercises the behaviour of
   explicitly passed files through `hook:format`.
10. Existing `TestVerifyNeverInvokesVibe` and the conformance-isolation
    tests, unchanged.
11. Docs review in the PR.

## Verification

- `task verify:fast` while working; `task verify` and `task audit`
  before declaring done.
- After a rebuild, `vibe audit` for the root and every example.
- CI: the test matrix and `examples.yml`.
- The ledger lists unit tests, CI, the step-0 probe and the example
  integration on separate lines.

## Open questions for the user

1. **Pyright.** Generated Python stays type-checked (spec §3). If the
   adopter's pydantic output fails `pyright` in standard mode, they have
   no escape, because `pyrightconfig.json` is managed. Is "still
   type-checked" the right call, or should `pyright` `exclude` get the
   same globs?
2. **Go.** Should the key be rejected for Go (this plan), or accepted
   and rendered into `linters.exclusions.paths` for generators that
   omit the header? Accepting it needs a glob-to-regex translation and
   `relative-path-mode` care.
3. **Grammar strictness.** The first segment must be literal, and
   patterns need two or more segments. That rules out a generated file
   directly at the component root (`contracts.gen.ts`). Is that
   acceptable?
4. **Follow-on doctor check.** Should `vibe doctor` warn when a pattern
   matches nothing, or matches a file without a generated header?
