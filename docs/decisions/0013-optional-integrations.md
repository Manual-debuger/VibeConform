# ADR 0013: Optional integrations, element-owned JSON, and `sync` deleting what it wrote

## Status

Accepted. Implemented per `docs/specs/0026-optional-integrations.md`.

## Context

Through spec 0025 a standard is one fixed module list. Every standard
ends with `claude-config` and `codex-config`, and nothing in `vibe.yaml`
can turn either off. A repository whose team uses one agent, or none,
still gets the other's files, and there is nowhere to add an editor or a
code-intelligence provider without shipping it to every adopter.

The standard is also the production contract: language verification,
Git hooks, and CI must not become opt-out. What varies per repository is
the *tooling around* the contract — which editors, which agents, which
code-intelligence providers — and those vary independently of each other.

Editor configuration is shared with the people using the editor.
`.vscode/tasks.json`, `.vscode/extensions.json`, and `.zed/tasks.json` are
files teams already have and keep adding to. Whole-file ownership
(`Generated`) would make every such addition drift that `audit` fails and
the next `sync` overwrites, and VS Code offers no local-override sibling
the way Claude has `settings.local.json`. ADR 0003 reserved
`StructuredPatch` for exactly this, but nothing implements it.

Removal is the other half. Spec 0008 made "no orphan pruning" an explicit
non-goal, so a module that stops resolving leaves its files and state
entries behind forever; spec 0024 generated an empty `.codex/hooks.json`
rather than drop the file. Selection without removal would make
deselection a lie.

## Decision

1. **A standard has a core and a catalog.** `Standard.Modules` stays the
   core: always resolved, in fixed order, never selectable. A new
   `Integrations` catalog lists optional modules, each with a name, a
   category (`editors`, `agents`, `intelligence`), whether it is on by
   default, and the integrations it requires or excludes. Integrations
   resolve after the core, in catalog order — never in manifest order.

2. **`vibe.yaml` gains an optional `integrations:` map** with the three
   categories as keys, each a list of names. An absent category takes the
   standard's defaults; an empty list means none. Decoding stays strict
   (ADR 0012). Names are checked against the catalog before anything
   resolves.

3. **Defaults reproduce today.** Every existing standard defaults to
   agents `[claude, codex]`, no editors, no intelligence. A manifest
   without `integrations:` resolves byte-for-byte as before.

4. **`StructuredPatch` is implemented for JSON arrays of owned
   elements.** A structured-patch resource names a JSON(C) file, an
   array inside it, and the elements VibeConform owns, each with a stable
   identity: an object's `label`, or a string's own value. VibeConform
   reconciles each owned element three-way, exactly as it reconciles a
   generated file, and never reads, rewrites, or reorders anything else:
   bytes outside owned elements — other elements, other keys, comments,
   whitespace — are preserved exactly.

5. **`.vibe/state.yaml` moves to schema 3** so it can record, per
   structured-patch resource, the hash of each owned element and whether
   VibeConform created the file. Generated resources keep their single
   file hash. Schema 2 files load unchanged; the upgrade is the next
   `sync`.

6. **`sync` deletes, narrowly.**
   - A *generated* file is removed only when a deselected integration
     would produce it, nothing selected produces it, state records it,
     and it still hashes to the recorded value.
   - An *owned element* is removed when state records it, the current
     target no longer contains it, and it is unchanged since recorded. A
     structured-patch file is deleted only if VibeConform created it and
     nothing but its empty skeleton remains.

   Anything modified since it was recorded is kept and reported as a
   conflict. Nothing unrecorded is ever deleted, and historical orphans
   from earlier standards are not touched.

7. **A managed path that Git ignores is an error.** `audit`, `diff`, and
   `sync` report it, because a file that is written but never committed
   is missing in every fresh checkout — `Conformance / audit` would fail
   in CI while passing locally.

## Dependency

`github.com/tailscale/hujson` (BSD-3-Clause, no transitive
dependencies) parses JSONC — JSON with comments and trailing commas, the
dialect VS Code and Zed both use — into a syntax tree that round-trips
byte-for-byte. Decision 4 needs exactly that: a standard `encoding/json`
round-trip drops comments and reformats the whole file. Writing a JSONC
parser in-tree was the alternative; a small, focused, widely used
library is less code to get wrong.

## Consequences

- Spec 0008's "no orphan pruning" non-goal is narrowed: deselected
  integrations and owned elements only.
- Core modules become selection-aware: `module.Context` carries the
  selected names, so repo-tooling can omit the `hook:*` tasks and the
  guard when no agent runs them. Resolution stays a pure function of
  `vibe.yaml`.
- Removing an integration is a one-line `vibe.yaml` edit, reviewed
  through `vibe diff` (principle 3).
- `StructuredPatch` covers owned array elements only. Owned object keys
  (`.vscode/settings.json`, `.zed/settings.json`,
  `.claude/settings.json`) are spec 0027; the per-element state model
  here is built to extend to keys.
- A running binary older than the one that wrote schema 3 is already
  refused by `sync` (unless `--allow-downgrade`) and gets no verdict from
  `audit` (spec 0019), so an old binary does not silently misread new
  state. An old binary resolves no editor integrations, so a forced
  downgrade never touches their files; it does drop the element records
  from state, and the next current `sync` re-records every owned element
  that still matches its target.
