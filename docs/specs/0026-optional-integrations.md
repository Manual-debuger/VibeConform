# Spec 0026: Optional Integrations

Status: accepted and implemented. Tracks issue #38. Per ADR 0013.

## Problem

- Every standard hard-codes `claude-config` and `codex-config`. A
  repository cannot drop an agent it does not use, and cannot add an
  editor at all.
- Nothing distinguishes the production contract (verification, Git
  hooks, CI) from the tooling around it, so adding an optional adapter
  would ship it to every adopter.
- Editor files are shared with their users. With whole-file ownership, a
  team's own task or extension recommendation would be drift that
  `audit` fails and `sync` overwrites.
- `vibe sync` never deletes (spec 0008). Deselecting anything would leave
  its files and state entries behind, silently.
- The LSP and GitNexus issues need a selection and lifecycle contract to
  plug into.

## Scope

### 1. `integrations:` in `vibe.yaml`

An optional map with three category keys, each a list of names:

```yaml
standard: prod-ts
version: v1
integrations:
  editors: [vscode]        # absent: []          (terminal-only)
  agents: [claude]         # absent: [claude, codex]
  intelligence: []         # absent: []
```

- A category that is **absent** takes the standard's defaults. An
  **empty list** means none. `integrations:` itself absent means all
  defaults.
- Decoding is strict: `editor:` or any other unknown key is an error.
- Each name matches `^[a-z][a-z0-9-]*$` and appears once per category.
  Errors name the entry: `integrations.editors[1] (zedd): ...`.
- Names are checked against the standard's catalog before anything
  resolves. An unknown name is an error listing the valid ones for that
  category. The `intelligence` catalog is empty in this spec, so any
  entry fails with "no code-intelligence providers are available yet".
- `vibe init` is unchanged and writes no `integrations:`.

### 2. The catalog

A standard keeps its core module list and gains a catalog of
integrations: name, category, module, default on/off, `Requires`, and
`Excludes`. All four standards register the same catalog:

| Category | Name | Default | Resources |
|---|---|---|---|
| `editors` | `vscode` | off | `.vscode/tasks.json`, `.vscode/extensions.json` (structured patch, §5) |
| `editors` | `zed` | off | `.zed/tasks.json` (structured patch, §5) |
| `agents` | `claude` | **on** | `.claude/settings.json`, `.claude/hooks/policy.json`, plus (from the core repo-tooling module) the guard program and the `hook:*` tasks |
| `agents` | `codex` | **on** | `.codex/config.toml`, `.codex/hooks.json` |

Selected integrations resolve after the core modules, in the catalog
order above, whatever order the manifest lists them in. `Requires` and
`Excludes` are validated — an unmet requirement or a declared exclusion
is an error naming both integrations. No shipped integration declares
either yet; tests cover them with fake integrations so the LSP and
GitNexus issues can use them.

### 3. Selection-aware core modules

`module.Context` carries the selected integration names. The four
repo-tooling modules emit the agent-hook surface only when `claude` is
selected:

- the `hook:guard`, `hook:context`, `hook:format`, `hook:check`, and
  `hook:done` tasks in `Taskfile.yml`;
- the guard program (`.claude/hooks/guard.go`, `guard.mjs`, or
  `guard.py`).

With `claude` off, `Taskfile.yml`, `lefthook.yml`, and `ci.yml` still
carry the full verification interface. Selecting an editor changes no
core file, except that `prod-ts` keeps Prettier off the editor's owned
files (§10).

### 4. Editor adapters

Both are profile-aware: a single-language standard passes its profile,
`prod-mono` its components' profiles. Selecting an editor configures it
for humans; it does not give an agent access to the editor's language
server (that is the LSP issue).

**`vscode`** owns, in `.vscode/tasks.json`, the elements of `tasks`
labelled `task fmt`, `task lint`, `task test`, and `task verify`:

```jsonc
{ "label": "task verify", "type": "shell", "command": "task",
  "args": ["verify"], "problemMatcher": [] }
```

and, in `.vscode/extensions.json`, the entries of `recommendations` for
the declared profiles: Go `golang.go`; TS `dbaeumer.vscode-eslint`,
`esbenp.prettier-vscode`; Python `charliermarsh.ruff`,
`ms-python.python`.

**`zed`** owns, in `.zed/tasks.json` (a top-level array), the elements
labelled `task fmt`, `task lint`, `task test`, and `task verify`:

```jsonc
{ "label": "task verify", "command": "task", "args": ["verify"] }
```

Every command goes through Task, never `vibe`.

### 5. Structured patch: owned array elements

A structured-patch resource names a file, an array in it (`tasks`,
`recommendations`, or the top level), and its owned elements. An
element's identity is its `label` (objects) or its value (strings).

**Parsing.** Files are JSONC, parsed with `github.com/tailscale/hujson`
(ADR 0013). Bytes outside owned elements — user elements, other keys,
comments, whitespace — are preserved exactly. An unparseable file, or
one where the array is not an array, is a conflict for that resource.

**Creating.** A missing file is written from a fixed skeleton
(`{"version": "2.0.0", "tasks": [...]}`, `{"recommendations": [...]}`,
`[...]`), byte-deterministic, and recorded as `created`. A file that
exists without the array gets the array appended as a new key.

**Reconciling.** Each owned element is decided three-way, with the
per-element recorded hash (P), the current element with the same
identity (C), and the target (T), using the existing decision table:

| Element | Decision | Effect |
|---|---|---|
| no element with that identity | `Create` | appended to the end of the array |
| C == T | `NoChange` | recorded |
| C != T, T == P | `LocalDrift` | replaced in place |
| C != T, C == P | `OutOfDate` | replaced in place |
| C != T, P absent | `Conflict` | a user element already has that label or value |
| otherwise | `Conflict` | |

Hashes are over the element's canonical JSON value (comments and
whitespace inside it do not count). A resource with any conflicting
element is not written at all; its other owned elements wait with it.
`diff` reports each element: `.vscode/tasks.json: would add "task
verify"`.

**Pruning elements.** A recorded element that the target no longer
contains (a deselected integration, or a profile no longer declared) is
removed if unchanged, forgotten if already gone, and a conflict if
modified. If the file was `created` and only its empty skeleton would
remain, the file is deleted instead.

### 6. State schema 3

```yaml
schema: 3
resources:
  Taskfile.yml:
    sha256: 5c1f...
  .vscode/tasks.json:
    created: true
    elements:
      tasks/task fmt:
        sha256: 91ab...
      tasks/task verify:
        sha256: 0e7d...
```

Generated resources are unchanged. Structured-patch resources record
`created` and a hash per owned element keyed `<array>/<identity>`.
Schema 2 files load as before; the next `sync` writes schema 3.

### 7. Deselecting generated files

A recorded generated path that an unselected integration would produce,
and that the current selection does not, is a prune candidate. Paths
with no state entry are never candidates.

| File on disk | Decision | `diff` | `sync` | `audit` |
|---|---|---|---|---|
| absent | `Forget` | `would forget (already removed)` | drops the state entry | out of date (3) |
| hash equals recorded | `Remove` | `would remove (<name> deselected)` | deletes the file and any directory it leaves empty; drops the entry | out of date (3) |
| hash differs | `RemoveConflict` | `conflict: <name> deselected but file modified since sync; kept` | keeps file and entry; counts as a conflict (non-zero exit) | conflict (2) |

Element pruning (§5) reports the same way. Prune lines follow the
resolved resources, in catalog order. A second `sync` after a clean one
reports nothing; re-selecting recreates through `Create`.

### 8. Managed paths ignored by Git

Before reporting, `audit`, `diff`, and `sync` ask Git which managed paths
it ignores (`git check-ignore --stdin`, one call). Each ignored,
untracked path is an error for that resource: `sync` does not write it
and exits non-zero; `audit` exits 2. The message names the path and the
fix (`add !<path> to .gitignore`). Outside a Git repository, or without
`git` on `PATH`, the check is skipped with one warning.

### 9. Examples

| Repository | `integrations:` | Shows |
|---|---|---|
| this repository | absent | defaults, zero diff |
| `examples/python` | `editors: []` | terminal-only, default agents |
| `examples/typescript` | `editors: [vscode]` | one editor, default agents |
| `examples/monorepo` | `editors: [vscode, zed]`, `agents: [claude]` | two editors, one agent |

The root `.gitignore`'s `.vscode/` becomes `.vscode/*` with exceptions
for `tasks.json`, `extensions.json`, `settings.json`, and `launch.json`,
following the common convention: shared project configuration is
committed, personal state is not. `examples/typescript` also carries one
user task in `.vscode/tasks.json`, proving unowned content survives
`sync`.

### 10. Formatters and owned editor files

Added after implementation: the `examples/typescript` CI jobs on PR #43
failed `fmt:check`. Prettier rewrites the layout of owned elements
(`tasks` objects expanded one member per line, `recommendations`
collapsed onto one line when it fits `printWidth`), so no fixed bytes the
adapter writes are Prettier-stable for every repository's configuration.
A `prod-ts` repository that selected `vscode` failed its own
`task verify` straight after `vibe sync`.

`prod-ts` (`tsrepotooling`) therefore leaves the owned files of each
selected editor out of Prettier's reach: `.vscode/tasks.json` and
`.vscode/extensions.json` for `vscode`, `.zed/tasks.json` for `zed`. The
exclusion is exactly those paths, not the directories, and it applies
wherever the generated tooling runs Prettier:

- `fmt` and `fmt:check`: a `!<path>` pattern per excluded file after the
  glob;
- lefthook's `prettier` pre-commit command: an `exclude` of those paths;
- `hook:format` (present only with `claude`): those paths are dropped
  from the changed-file list before Prettier runs.

With no editor selected (the default), the rendered files are
byte-identical to what they were without this section. `prod-mono` is
unaffected: Prettier runs per component (`root:` and the component
Taskfile), and editor files live at the repository root, outside every
component. `prod-go` and `prod-py` run no JSON formatter.

Correctness of the files is unaffected. Owned elements are compared by
canonical JSON (§5), so a Prettier-formatted element, for example by an
editor's format-on-save, is not drift, and `audit`/`sync` still parse the
whole file and reject invalid JSONC. What is lost is only a layout check
on those files.

### Documentation

- `docs/usage.md`: a new "Selecting integrations" section with the
  examples below, and a note on which editor files to commit; "What
  `vibe.yaml` means today", the agent-hook notes, "`.vibe/state.yaml`",
  and "Removing VibeConform" updated.
- `docs/architecture/overview.md`: catalog, structured patch, pruning,
  package layout.
- `README.md`: one line on integrations.

#### `vibe.yaml` examples for `usage.md`

1. **Defaults** — `standard` and `version` only: agents `claude` and
   `codex`, no editors. Identical to before this spec.
2. **Claude only** — `integrations: {agents: [claude]}`: `vibe diff`
   shows `would remove .codex/config.toml` and `.codex/hooks.json`.
3. **No agent config** — `editors: []`, `agents: []`: no `.claude/`, no
   `.codex/`, no `hook:*` tasks; `task verify`, lefthook, and CI
   unchanged.
4. **One editor** — `editors: [vscode]`: agents stay at their defaults,
   because categories are independent; the team's own tasks and
   recommendations stay in the same files.
5. **Two editors in a monorepo** — `editors: [zed, vscode]`: resolves in
   catalog order (vscode first); recommendations cover every declared
   profile.
6. **Invalid** — a duplicate name, an unknown agent, any intelligence
   entry, a misspelled category key: each an error before anything
   resolves, exit 1.

## Behavior

- A manifest without `integrations:` resolves every resource
  byte-for-byte as before; the only change a sync makes is the state
  schema number (plus, as on every sync, the recorded `vibe_version`).
- Selection is a pure function of `vibe.yaml`: same manifest, same
  resources, same order.
- `task verify`, `task verify-ci`, lefthook, and every workflow stay
  `vibe`-free and independent of any integration (principle 3).
- Deletion and element edits use slash paths joined to the repo root and
  byte-exact writes; behavior is the same on Windows and Linux.
- A `prod-ts` repository that selects `vscode` or `zed` passes its own
  `task verify` straight after `vibe sync` (§10).

## Explicit non-goals

- **No owned object keys.** `.vscode/settings.json`, `.zed/settings.json`,
  and `.claude/settings.json` stay out of scope (spec 0027).
- **No owned element ordering.** Created elements are appended; a user
  may reorder the array freely, and that is not drift.
- **No general orphan pruning.** Files left by a standard dropping a
  resource, or by a module removed from the core, are not touched.
- **No code-intelligence providers.** The category and its validation
  exist; LSP and GitNexus are separate issues.
- **No core deselection.** Verification, hooks, CI, and conformance are
  not integrations.
- **No `vibe init` flags** for integrations, and no `.gitignore` editing.

## Design notes

- **Why a map of categories, not one flat list.** Categories vary
  independently, each has its own default, and a flat list could not say
  "no agents" without also saying "no editors".
- **Why absent differs from empty.** Absent must mean today's behaviour,
  so no guardrail disappears; empty must be expressible.
- **Why element ownership instead of whole files for editors.** Teams
  already keep their own tasks and recommendations there, and no editor
  offers a local-override file for them. Owning elements by identity is
  the narrowest ownership that lets both coexist.
- **Why identity is the label.** It is the handle users and editors show
  and invoke; a user task with the same label is a real collision, and
  surfacing it as a conflict is correct.
- **Why a conflicting element holds the whole file.** Writing half a
  resource would record a state that no single decision describes.
- **Why the generated prune set comes from the standard.** Deriving
  candidates from "what an unselected integration would produce" confines
  deletion to files this mechanism created.
- **Why an ignored managed path is an error, not a warning.** It passes
  locally and fails in CI's fresh checkout; the earliest loud failure is
  the useful one (principle 1).
- **Why the hook tasks follow `claude`.** Nothing else calls them (Codex
  hooks are suspended, spec 0024).
- **Why exclude owned editor files from Prettier rather than write
  Prettier's layout.** Prettier's output depends on each repository's
  `printWidth` and options, so no fixed bytes pass for all of them, and
  running Prettier from `sync` would make it call language tooling
  (principle 3). The files' content is already checked by `audit`; only
  layout goes unchecked.
- **Why the exclusion follows the selection and names files, not
  directories.** With no editor selected, `prod-ts` output stays
  byte-identical, and a user's own `.vscode/settings.json` or
  `launch.json` keeps Prettier.

## Follow-on work

- Spec 0027: owned object keys in structured patch, then editor
  `settings.json` and possibly `.claude/settings.json` through it.
- The LSP and GitNexus integration issues, registering into
  `intelligence` (and `Requires` where they depend on an agent).
