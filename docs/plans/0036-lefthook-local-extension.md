# Plan 0036: A project-owned extension point for `lefthook.yml`

Implements `docs/specs/0036-lefthook-local-extension.md`. Status:
approved and implemented. The header line of every template was also made
provider-neutral (spec open question 3). The golden test keeps its
goldens.

Branch: `feat/lefthook-local`, off `main`. One pull request.

## Resolved design questions

**Native mechanism or VibeConform code?** Native. The change is one
`extends:` entry in each lefthook template. `vibe` gains no Go code apart
from tests. This is the same shape as ADR 0009.

**Why `extends` and not `lefthook-local.yml`?** `lefthook-local.yml` is
upstream's per-developer layer. It is gitignored by convention and
applies last. A committed project layer belongs between the managed file
and the developer's override, which is exactly where `extends` sits
(main → extends → remotes → local).

**Where does the entry go?** After the header comment and before
`pre-commit:`, with a one-line comment. It is not at the end, because
`module.AddGraphify` appends the graphify jobs to the end and its
"must end in a newline" contract should stay untouched. The monorepo
template uses the same literal lines outside any `[[range]]` block.

**Does any code path need to know?** No. `RequiredTools` is unchanged
(lefthook is already required). `sync` already runs `lefthook install`,
which picks up hooks defined only in the extended file. The TS prettier
rewrite (`excludeFromPrettierLefthook`) anchors on the prettier `glob:`
line, which is unaffected.

## Steps

1. **ADR 0017**, `docs/decisions/0017-lefthook-local-extension.md`:
   - context: ADR 0009's seam, applied to `lefthook.yml`;
   - decision: `extends: [lefthook.local.yml]`, committed and
     project-owned;
   - consequences: missing file is a no-op, merge order, per-command
     override with no collision error (and why that is acceptable: CI is
     the gate, and the hooks are already bypassable), the verified
     lefthook versions;
   - alternatives: docs-only `lefthook-local.yml`, `remotes`, a managed
     section in `lefthook.yml`, `structured-patch` ownership.
2. **Templates.** Add the block below to
   `internal/module/{repotooling,tsrepotooling,pyrepotooling}/templates/lefthook.yml`
   and `internal/module/monorepotooling/templates/lefthook.yml.tmpl`,
   after the header comment:

   ```yaml
   # Repository-specific hooks go in lefthook.local.yml (optional, yours;
   # see VibeConform's docs/usage.md). Lefthook merges it over this file.
   extends:
     - lefthook.local.yml
   ```

   The wording is settled at implementation and stays identical across
   all four templates.
3. **Rebuild and sync.** Run `task build`, then `./bin/vibe sync` for the
   repository root and `examples/{typescript,python,monorepo}`. Commit
   each `lefthook.yml` together with its `.vibe/state.yaml`.
4. **Docs.**
   - `docs/usage.md`: a new "Adding your own Git hooks:
     `lefthook.local.yml`" section after "Adding your own tasks",
     containing the example, the semantics, the per-developer
     `lefthook-local.yml`, migration from a hand-edited `lefthook.yml`
     (move the additions, `git checkout lefthook.yml`, `vibe sync`), and
     the Graphify note.
   - `docs/usage.md`, "Removing VibeConform": the `extends` line may stay.
   - Cross-references in `docs/adopting.md` where it lists
     `Taskfile.local.yml`.
   - `docs/architecture/overview.md`: the extension-point paragraphs.
   - `docs/architecture/principles.md`: the principle-3 table row.
5. **Spec status.** Mark spec 0036 accepted and implemented, and link
   ADR 0017.

## Tests (by acceptance criterion)

1. `TestLefthookExtendsLocal` in `internal/standard` (table over all
   four standards × graphify on/off). It parses the resolved
   `lefthook.yml` and asserts `extends == ["lefthook.local.yml"]`.
2. `TestLocalLefthookIsNotManaged`: no resolved resource path equals
   `lefthook.local.yml`. A grep-style test asserts that no non-test,
   non-template `.go` file under `internal/` contains the string. This
   follows the precedent for `Taskfile.local.yml`.
3. `TestLefthookOnlyGainsExtends`: strip the exact added block from each
   resolved file and compare it to a golden copy of today's template
   output. The goldens are captured before step 2 and committed as test
   data.
4. & 5. `TestLefthookLocalMerge` in `internal/module`, with a real
   lefthook binary (`t.Skip` when `lefthook` is not on PATH, the same
   pattern as the `task` runtime tests). It runs in a temporary git repo,
   once without and once with a `lefthook.local.yml`, and checks
   `lefthook dump` output (merged hooks present, managed commands
   intact). The audit half of AC 5 is a `cli` test: sync, write
   `lefthook.local.yml`, audit, and expect `lefthook.yml: ok`.
6. Covered by the existing spec 0019 decision tests. No new test is
   needed. The out-of-date path is exercised manually in Verification.
7. `TestExamplesAreConformant`, plus `task audit` at the root.
8. The existing verify-independence tests (`TestVerifyNeverInvokesVibe`
   and the spec 0035 closure tests) stay green and unmodified.
9. Docs review in the PR. There is no automated check.

## Repository impact analysis

| Area | Files | Effect |
|---|---|---|
| Templates | 4 lefthook templates | +4 lines each |
| Generated, this repo | `lefthook.yml`, `.vibe/state.yaml` | re-synced |
| Examples | `examples/{typescript,python,monorepo}/lefthook.yml` and state | re-synced; CI `examples.yml` must stay green |
| Go code | none outside tests | no package-boundary or dependency change |
| Tests | `internal/standard`, `internal/module`, `internal/cli` | new tests; golden test data |
| Docs | `usage.md`, `adopting.md`, `overview.md`, `principles.md`, ADR 0017, spec 0036 | additions |
| Adopters | every `lefthook.yml` | one-time `out of date` (exit 3) after upgrading `vibe`; hand-edited files become `conflict` |
| Dependencies | none | lefthook is already required; no new tool |

Risks:

- **A lefthook version that errors on a missing `extends` target** would
  break every hook for adopters without the file. This was verified not
  to happen on 1.13.6 and 2.1.16 only. Mitigation: probe the oldest
  lefthook in CI's install path. If an older version errors, document a
  floor (or choose a glob entry, `lefthook.local.y*ml`; both forms were
  verified as tolerated).
- **Windows path handling** of a relative `extends` entry is unverified.
  This is covered by the manual check below.
- **Golden tests are brittle** to future template edits. They are
  deliberately scoped to this change and could be removed afterwards.
  Open question: keep or drop them after merge.

## Verification

- `task verify:fast` while working. `task verify` and `task audit` before
  declaring done.
- After a rebuild, `vibe audit` for the root and every example.
- Manual, Linux and Windows, in a scratch copy of `examples/python`:
  - without `lefthook.local.yml`: `lefthook install`, then a commit
    runs only the managed hooks;
  - with a `post-merge` job in `lefthook.local.yml`: `lefthook install`
    registers it, and `lefthook run post-merge` runs it;
  - `vibe audit` stays clean throughout.
- Manual compatibility: audit a scratch repository synced by the current
  release with the new binary. Expect `out of date`, exit 3, and a clean
  result after sync.
- CI: the Ubuntu and Windows test matrix and `examples.yml`.

The ledger lists unit tests, CI, the manual Linux and Windows lefthook
checks and the compatibility check as separate lines.
