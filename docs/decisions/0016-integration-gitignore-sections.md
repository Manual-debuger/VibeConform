# ADR 0016: Integration-owned sections in `.gitignore`

## Status

Accepted. Implemented per `docs/specs/0035-graphify.md`. Narrows spec
0026's non-goal "no `.gitignore` editing". Amended by ADR 0020, which also
allows a section for a runtime's per-developer file (`claude-config` and
`.claude/settings.local.json`).

## Context

Spec 0026 kept `.gitignore` out of VibeConform's reach. It is a file
every repository already has and keeps adding to. In 2026-09, nothing
VibeConform generated produced output that needed ignoring.

Graphify (spec 0035) changes that. Its graph, report and cache land in
`graphify-out/`. That output is derived and machine-local, and it is
large. A committed copy goes stale while looking current, which is
exactly the failure issue #52 asks us to prevent ("do not claim stale
results are current"). If the ignore entry is left to the user, the
first `git add -A` after selecting graphify commits the graph.

ADR 0014 already gives VibeConform a way to own part of a shared text
file without owning the file: a marker-delimited managed section, which
is reconciled three-way, pruned on deselection, and deleted with the
file only when VibeConform created the file and nothing else remains.
The line-ending policy uses one in `.gitattributes`.

## Decision

1. **An integration may own a managed section of the root
   `.gitignore`**, with hash-comment markers, holding the ignore entries
   for output that it alone causes. The rest of the file stays the
   project's, byte for byte.
2. **The section goes at the bottom.** Git applies the last matching
   pattern, so a project rule placed below the section can still
   re-include a path. That is a deliberate choice, and `vibe doctor`
   reports it (the `graphify ignore` line).
3. **Only integration output.** No core module and no policy writes
   `.gitignore`. Standard build artifacts (`bin/`, `node_modules/`,
   `.venv/`) remain the project's to ignore, because a repository's
   layout decides them, and VibeConform cannot.
4. Deselecting the integration removes the section under the existing
   prune rules: unchanged, it is removed; edited, it is a conflict and is
   kept; a `.gitignore` VibeConform created, with nothing else in it, is
   deleted.

## Consequences

- The managed-path check (spec 0026 §8) is unaffected. It asks whether
  Git ignores a *managed* path, and no integration owns a file under a
  directory its section ignores.
- An adopter sees a `# vibeconform:begin <id>` block in `.gitignore`.
  The markers are plain comments, and removing VibeConform leaves a
  working ignore file (principle 3).
- Future providers, such as GitNexus's `.gitnexus/`, follow this ADR
  rather than relitigating it.
