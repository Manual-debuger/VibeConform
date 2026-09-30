# Plan 0026: Optional integrations

Implements `docs/specs/0026-optional-integrations.md` and
`docs/decisions/0013-optional-integrations.md`. Tracks issue #38.
Spec accepted at review (2026-09-30), with the narrow JSON merge pulled
into scope and the Git-ignored managed path check added.

Branch: `feature/optional-integrations`, off `main`, one pull request,
merged with a merge commit.

## Resolved design questions

**Where does a single-language standard's profile come from?**
`Standard` gains `Profile manifest.Profile` (`go`, `ts`, `py`; empty for
`prod-mono`). `buildPlan` fills `module.Context.Profiles` from it, or
from the components' profiles in `manifest.Profiles` order. Editor
modules read only `Context.Profiles`, so they need no per-standard code.

**Where does the JSONC code live?** A new `internal/jsonarray` package:
parse with `hujson`, find an array by key (or the top level), locate
elements by identity, insert, replace, and remove them, and pack the
result. It knows nothing about state or reconciliation. Dependency
direction: `cli → jsonarray → hujson`; `resource` stays dependency-free.

**How does a module describe a structured patch?** `resource.Resource`
gains `Patch *ArrayPatch` (`Array` key, `Skeleton` bytes, `Elements`
with `ID` and canonical `Value`). `Content` is unused for patch
resources. `Ownership: StructuredPatch` with a nil `Patch` is a resolve
error, so the two cannot disagree.

**How is the prune set for generated files computed?** `buildPlan`
resolves the selection, then once more per unselected integration with
the selection plus that one (skipping `Requires` and `Excludes`, since
that resolve is never applied). Recorded paths such a resolve produces
and the selection does not are prune candidates, attributed to that
integration — including core-module paths it switches on, like the
guard for `claude`. The second resolve is
pure and cheap — modules already must not do I/O beyond `RepoRoot`
reads.

**How do tests avoid shelling out to Git?** `checkIgnored` is a package
variable seam in `internal/cli`, as `installGitHooks` is. The default
runs one `git check-ignore --stdin -z` in the repo root; tests replace
it.

**What happens to this repository's committed state?** The next `sync`
rewrites `.vibe/state.yaml` at schema 3 with no resource changes. That
lands in C3, together with the schema change.

## Repository impact

| Area | Change |
|---|---|
| `go.mod`, `go.sum` | add `github.com/tailscale/hujson` (ADR 0013, "Dependency") |
| `internal/manifest/` | `Integrations` with `Editors`, `Agents`, `Intelligence` as `*[]string`; name and duplicate validation; tests |
| `internal/standard/` | `Profile`, `Integration`, `Category`, catalog on all four standards; `Select` (defaults, unknown names, `Requires`, `Excludes`, catalog order); `standard_test.go` pins core plus defaults to today's order; `wiring_test.go` both directions of `claude` |
| `internal/module/module.go` | `Context.Integrations`, `Context.Profiles`, nil-safe `Selected` |
| `internal/module/{repotooling,tsrepotooling,pyrepotooling,monorepotooling}` | `hook:*` tasks and guard only when `claude` is selected; templates gain a conditional block; tests for both |
| `internal/module/editors/vscode`, `editors/zed` | new modules, array patches, tests |
| `internal/resource/` | `ArrayPatch`, `Element`, validation |
| `internal/jsonarray/` | new package, table and round-trip tests (comments, trailing commas, CRLF, 4-space indent preserved) |
| `internal/state/` | schema 3: `Created`, `Elements map[string]ElementState`; schema 2 load test |
| `internal/reconcile/` | `DecideRemoval` (`Forget`, `Remove`, `RemoveConflict`); per-element use of `Decide` |
| `internal/cli/plan.go` | selection, profiles, patch planning, prune candidates, ignored-path check |
| `internal/cli/{diff,sync,audit}.go` | report and apply removals and element decisions; exit codes per spec §7–§8 |
| `internal/cli/gitignore.go` | `checkIgnored` seam and default |
| `.gitignore` | `.vscode/` → `.vscode/*` plus four exceptions |
| `examples/python`, `examples/typescript`, `examples/monorepo` | `vibe.yaml` selections, re-synced files, `.vibe/state.yaml`; one user task in `examples/typescript/.vscode/tasks.json` |
| Root generated files | `.vibe/state.yaml` only (schema 3) |
| Docs | `docs/usage.md`, `docs/architecture/overview.md`, `README.md`, spec 0026 status, ADR 0013 status |

## Order of work

Five commits, each leaving `task verify` and `task audit` green.

1. **C1: selection, no behaviour change.** Manifest `integrations:`,
   the catalog with `claude` and `codex` only, `Context.Integrations`
   and `Context.Profiles`, `buildPlan` wiring and error messages. Tests
   prove every standard resolves to identical resources with defaults,
   and `vibe audit` on the root and every example is clean without a
   sync.
2. **C2: selection-aware core.** Repo-tooling hook tasks and guard
   follow `claude`; `wiring_test.go` covers `claude` on and off; a test
   that with `agents: []` no resource mentions `hook:` or `.claude/`.
3. **C3: removal and state schema 3.** `DecideRemoval`, prune
   candidates, `diff`/`sync`/`audit` reporting, empty-directory cleanup,
   the schema 3 state struct (element fields unused yet), the
   ignored-path check. Rebuild, sync the root and examples (state only).
4. **C4: structured patch and editors.** `hujson`, `internal/jsonarray`,
   `resource.ArrayPatch`, per-element planning and applying, element
   pruning, the `vscode` and `zed` modules, their catalog entries.
5. **C5: examples, docs, verification.** `.gitignore`, example
   selections and re-sync, `docs/usage.md` "Selecting integrations" with
   the spec's six `vibe.yaml` examples, the other docs, this plan's
   checklist and verification record, spec and ADR marked implemented.

**Dogfooding hazard.** This repository keeps the defaults, so its
`.claude/settings.json` and guard never change; the session doing the
work keeps its guard throughout. C2 is the risky commit: a bug that
drops the hook tasks with defaults would disable the guard on the next
`sync`. C1's identical-resources test runs before C2 and catches it.

## Checklist

- [x] C1 manifest parsing: absent vs `[]`, unknown key, bad name, duplicate (`internal/manifest/integrations_test.go`)
- [x] C1 catalog: unknown name lists valid ones; intelligence message; `Requires`/`Excludes` with fakes; catalog order regardless of manifest order (`internal/standard/integrations_test.go`)
- [x] C1 defaults resolve identically for all four standards and the examples (`TestPlanDefaultsKeepAgentsAndHooks`, `TestExamplesAreConformant`, pinned module order)
- [x] C2 hook tasks and guard absent with `claude` off, present with it on (`TestAgentHooksFollowClaude`, all four standards, byte-identical with `claude` selected)
- [x] C2 `task verify` in a scratch repository with `agents: []`
- [x] C3 `Forget`, `Remove`, `RemoveConflict` in diff, sync, audit, with exit codes (`internal/cli/prune_test.go`)
- [x] C3 unrecorded file never deleted; empty directory removed, non-empty kept
- [x] C3 second sync is a no-op; re-select recreates
- [x] C3 schema 2 state loads; sync writes schema 3
- [x] C3 ignored managed path: sync refuses, audit exits 2; no-git warning; real `git check-ignore` test
- [x] C4 `jsonarray` preserves bytes outside owned elements (comments, trailing commas, CRLF, indentation)
- [x] C4 element decisions: create, no change, drift, out of date, label collision conflict, unparseable file (`internal/cli/patch_test.go`)
- [x] C4 element pruning; created file deleted only when skeleton remains
- [x] C4 recommendations follow declared profiles in `prod-mono`
- [x] C5 examples: terminal-only (`python`), single editor with a user task (`typescript`), multi-editor (`monorepo`)
- [x] C5 docs synced; `vibe.yaml` examples in `docs/usage.md`
- [x] `task verify` and `task audit` green locally; CI not yet observed (branch not pushed)

## Found during implementation

- **The hook block is removed from the Taskfile templates, not templated
  in.** Keeping each template byte-identical to today's output was the
  requirement that mattered (a slip there disables this repository's own
  guard on the next sync), so `module.StripAgentHooks` cuts the block from
  its first comment to the end of the last `hook:*` task, and refuses a
  template where that block is missing, duplicated, or contains any other
  task. `module.WantsAgentHooks` fails safe: an unknown selection keeps
  the hooks.
- **`task audit` answers with the `vibe` on `PATH`.** Once C3 recorded
  state with a development build, the released v0.3.0-alpha.1 in
  `~/go/bin` was older than the recorded writer and gave no verdict, as
  spec 0019 intends. The dev build was installed with
  `go install ./cmd/vibe` for the rest of the work.
- **`.vscode/*` in `.gitignore` is anchored at the root.** A pattern with
  a slash before its end matches relative to the `.gitignore` itself, so
  the examples' `.vscode/` files were never ignored; the root's own
  pattern was switched to `.vscode/*` plus exceptions anyway, following
  the convention `docs/usage.md` now recommends.
- **No committed example can drop `claude`.** `hook-guard.yml` runs the
  guard corpus in `examples/python` and `examples/typescript` through
  `task -x hook:guard`, and `examples.yml` runs `hook:context` and
  `hook:format` in all three examples. Spec 0026 §9 planned
  `examples/python` with `agents: []`; that would have passed locally
  (those tests skip without their environment variables) and failed CI.
  `examples/python` is terminal-only with default agents instead, and the
  no-agents case is covered by `TestAgentHooksFollowClaude`,
  `TestDeselectAllAgents`, and the scratch walk below.
- **The sync summary gains `, N removed` only when something was
  removed**, so every existing repository's output is unchanged.
- **A selection change reports `Taskfile.yml` as "standard moved".** From
  the file's point of view that is what happened — the target changed and
  the file did not — so no new wording was added.

## Verification

Observed locally (Windows 11 Pro 10.0.26200, 2026-09-30; go 1.27.0,
Task 3.53.1, git 2.53.0.windows.1, lefthook 2.1.14):

- `task verify` and `task audit` pass at every commit (C1–C5); `task audit`
  reports the root conformant with 13 resources.
- `go test ./...` passes, including every new test named above.
- `vibe sync` on the root with no `integrations:` key changes no managed
  file; `.vibe/state.yaml` changes only `schema` and `vibe_version`.
- Scratch prod-go repository, `agents: []` from the start: synced with no
  `.claude/` or `.codex/`; `task verify` passes.
- Same repository, then `editors: [vscode]` with default agents: `diff`
  previewed every addition; `sync` created `.vscode/tasks.json` and
  `extensions.json` and the agent files; `audit` conformant. Then
  `agents: []` with no editors: `diff` listed every removal, including
  "would remove the file (vscode deselected; nothing else is in it)";
  `sync` removed 7 resources and the emptied `.vscode/` and `.claude/`;
  a second `sync` changed nothing; `task verify` passes.
- `examples/python` synced with `editors: []`, `agents: []` passed
  `task verify` with no agent configuration; it was then set back to
  default agents (see "Found during implementation").
- `examples/typescript/.vscode/tasks.json` keeps its own commented task
  byte for byte after `sync` added the four owned tasks.

Not observed locally: CI on Linux; editing the files inside VS Code or
Zed themselves.

## C6: formatters and owned editor files (spec 0026 §10)

Found on PR #43: `examples/typescript (ubuntu-latest)` and
`(windows-latest)` failed `task verify` at `fmt:check`, with Prettier
reporting `.vscode/extensions.json` and `.vscode/tasks.json`. The root
`task verify` does not run the examples' own tooling, so the
verification above missed it. Spec 0026 §10, approved 2026-09-30.

### Resolved design questions

**Where do the owned paths come from?** `editors.OwnedPaths`
(`map[string][]string`, integration name → owned file paths) in
`internal/module/editors`. `tsrepotooling` reads it for the selected
integrations in catalog order; a test checks the map against the paths
the `vscode` and `zed` modules actually resolve, so the two cannot
drift. The dependency is `tsrepotooling → editors → module`, with no
cycle.

**How are the templates changed?** As C2 did: the embedded templates
stay the default bytes, and a function inserts the exclusions at exact
anchors, refusing a template where an anchor is missing or appears more
than once. With no owned paths it returns the template unchanged, which
keeps the default output byte-identical by construction.

- `fmt` and `fmt:check`: after the quoted glob, `"!<path>"` for each
  path (Prettier 3 CLI negation patterns).
- `lefthook.yml` `prettier`: `exclude:` with the paths as a glob list
  (lefthook 2 syntax; observed 2.1.14 locally).
- `hook:format`: `':(exclude)<path>'` pathspecs on both `git diff` and
  `git ls-files`, so an excluded file never reaches Prettier. The
  anchors sit inside the hook block, so with `claude` off
  `StripAgentHooks` runs first and those anchors are not required.

### Repository impact

| Area | Change |
|---|---|
| `internal/module/editors/editors.go` | `OwnedPaths`; test against both modules' resolved paths |
| `internal/module/tsrepotooling/` | exclusion insertion for `Taskfile.yml` and `lefthook.yml`; tests |
| `examples/typescript` | re-synced `Taskfile.yml`, `lefthook.yml`, `.vibe/state.yaml` (it selects `vscode`) |
| `examples/monorepo`, `examples/python`, root | `.vibe/state.yaml` `vibe_version` only, if the sync writes it |
| Docs | `docs/usage.md` "Selecting integrations": one paragraph on Prettier and the owned files; this plan |

`prod-go`, `prod-py`, and `prod-mono` modules are not changed.

### Tests

- Regression: `tsrepotooling` with `editors: [vscode]`, `[zed]`, and
  both. `fmt`, `fmt:check`, lefthook, and `hook:format` each name exactly
  the owned paths; with `agents: []` the Taskfile has no hook block and
  still has the `fmt` exclusions.
- Default (`integrations:` absent, and `editors: []`): the Taskfile and
  lefthook are byte-identical to the embedded templates. The existing
  defaults tests already pin this; one explicit case is added.
- Anchor refusal: a template missing an anchor, or with one duplicated,
  is an error.
- The `hook:format` pathspecs, run with real `git` in a temporary
  repository (as the `check-ignore` test does): an edited
  `.vscode/tasks.json` is not listed, and an edited
  `.vscode/settings.json` still is.
- End to end: `examples/typescript` is the regression that failed.
  Locally, `task verify` inside it with the rebuilt binary's sync, then CI.

### Order of work

One commit, `fix: keep Prettier off owned editor files in prod-ts (spec
0026 C6)`: tests first (they fail against the current templates), then
`OwnedPaths` and the insertion, rebuild (`go install ./cmd/vibe`),
`vibe sync` in the root and each example, docs.

### Checklist

- [x] Regression tests fail before the change and pass after (`TestPrettierLeavesOwnedEditorFilesAlone`, `TestHookFormatSkipsOwnedFilesWithRealGit`, `TestExcludeFromPrettierRefusesUnknownTemplates`, `TestOwnedPathsMatchResolvedPaths`)
- [x] Defaults byte-identical (`TestNoEditorKeepsTemplatesByteIdentical`); anchor refusal tested
- [x] `hook:format` pathspecs checked with real `git`
- [x] Examples re-synced with the rebuilt binary; `vibe audit` conformant in each
- [x] `task verify` inside `examples/typescript`, `examples/python`, `examples/monorepo`
- [x] Root `task verify` and `task audit`
- [x] CI on PR #43 green, including both `examples/typescript` jobs (all 24 checks on `123aaec`)

### Found during implementation

- **The anchors are replaced, not inserted after.** Both `git` lines in
  `hook:format` end in the same extension list, so each edit replaces a
  string that occurs exactly once (`'*.md' 2>/dev/null` for `git diff`,
  the whole `git ls-files` tail for the other) rather than inserting after
  a shared prefix.
- **lefthook's `exclude:` checked directly.** `examples/typescript` is not
  a Git repository of its own, so `lefthook run` there picks up the root
  configuration. A scratch repository with the generated `prettier`
  command (its `run` replaced by `echo`) and lefthook 2.1.14 matched
  `.vscode/settings.json` and `package.json`, and neither owned file.

### Verification (C6)

Observed locally (Windows 11 Pro 10.0.26200, 2026-09-30; go 1.27.0,
Task 3.53.1, lefthook 2.1.14):

- The new tests fail against templates without the exclusions (the
  real-git test listed `.vscode/tasks.json` and `extensions.json`) and pass
  with them.
- `vibe sync` in the root and every example changed only
  `examples/typescript/Taskfile.yml`, `lefthook.yml`, and the state files.
- `task verify` passes inside all three examples and at the root;
  `task audit` and `vibe audit` in each example report conformant.

## Explicitly still deferred

- Owned object keys and editor `settings.json` (spec 0027).
- LSP and GitNexus providers.
- General orphan pruning, `vibe init` flags, `.gitignore` editing.
