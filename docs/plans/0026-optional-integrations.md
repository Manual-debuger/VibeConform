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

- [ ] C1 manifest parsing: absent vs `[]`, unknown key, bad name, duplicate
- [ ] C1 catalog: unknown name lists valid ones; intelligence message; `Requires`/`Excludes` with fakes; catalog order regardless of manifest order
- [ ] C1 defaults resolve identically for all four standards and the examples
- [ ] C2 hook tasks and guard absent with `claude` off, present with it on
- [ ] C2 `task verify` in a scratch copy with `agents: []`
- [ ] C3 `Forget`, `Remove`, `RemoveConflict` in diff, sync, audit, with exit codes
- [ ] C3 unrecorded file never deleted; empty directory removed, non-empty kept
- [ ] C3 second sync is a no-op; re-select recreates
- [ ] C3 schema 2 state loads; sync writes schema 3
- [ ] C3 ignored managed path: sync refuses, audit exits 2; no-git warning
- [ ] C4 `jsonarray` preserves bytes outside owned elements (comments, trailing commas, CRLF, indentation)
- [ ] C4 element decisions: create, no change, drift, out of date, label collision conflict, unparseable file
- [ ] C4 element pruning; created file deleted only when skeleton remains
- [ ] C4 recommendations follow declared profiles in `prod-mono`
- [ ] C5 examples: terminal-only, single editor (with a user task), multi-editor
- [ ] C5 docs synced; `vibe.yaml` examples in `docs/usage.md`
- [ ] `task verify` and `task audit` green; CI green on Windows and Linux

## Verification

To be recorded here: OS, `go`, `task`, `git` versions; `task verify`,
`task audit`; the manual deselection walk in a scratch copy of
`examples/typescript` (`agents: []` → `diff` → `sync` → `task verify` →
`sync` again → edit a file, re-select, deselect → conflict kept); and a
manual edit of `examples/typescript/.vscode/tasks.json` in VS Code with a
comment added, synced, and diffed to confirm untouched bytes.

## Explicitly still deferred

- Owned object keys and editor `settings.json` (spec 0027).
- LSP and GitNexus providers.
- General orphan pruning, `vibe init` flags, `.gitignore` editing.
