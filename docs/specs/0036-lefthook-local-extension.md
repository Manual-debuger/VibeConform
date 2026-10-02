# Spec 0036: A Project-Owned Extension Point for `lefthook.yml`

Status: proposed. Not yet approved. Implementation plan:
`docs/plans/0036-lefthook-local-extension.md`. Builds on ADR 0009
(`Taskfile.local.yml`), spec 0019 (drift classification) and spec 0035
(Graphify).

## Problem

- `lefthook.yml` is `generated`: VibeConform owns the whole file
  (ADR 0003). A repository that wants a Git hook of its own (a
  `post-merge` dependency install, a `commit-msg` linter, a
  repository-specific `pre-commit` check) has nowhere documented to put
  it. Adding it to `lefthook.yml` makes `vibe audit` report the file as
  drifted, and CI's `Conformance / audit` fails.
- `Taskfile.yml` solved the same problem with ADR 0009: an optional
  include of a project-owned `Taskfile.local.yml`. `lefthook.yml` has no
  equivalent, and `docs/usage.md` does not mention one.

### The reported case

An adopter (a `prod-mono/v1` repository) reported that `lefthook.yml`
drifted after they "added post-commit and post-checkout `graphify-update`
hooks running `task graph:update`". Investigation shows that report is
partly a misdiagnosis:

- Those exact jobs are what spec 0035 §2 generates when
  `integrations: {intelligence: [graphify]}` is selected
  (`module.GraphHookJobs`, appended by `module.AddGraphify`, monorepo
  included). The adopter's `lefthook.yml` history shows them present in
  the very first commit that applied VibeConform, and the adopter's
  `AGENTS.md` carries the Graphify bullet that only a graphify-selected
  workflow section generates.
- The only hand edit in the file's history is a comment: the header's
  `CI (.github/workflows/ci.yml)` was changed to `CI (.gitlab-ci.yml)`
  when the adopter moved to GitLab CI. That byte change is what `audit`
  reports. An extension point would not have absorbed it, because it is
  an edit to the managed file itself, not an addition beside it.

So the reported drift is fixed by restoring the header (`vibe sync`), not
by this spec. The header naming a CI the repository does not use is a
separate defect (see "Open questions"). The underlying need is still
real: had the graphify jobs *not* been generated, the adopter would have
had no conformant place to put them. This spec provides that place.

## What lefthook already does (verified)

Verified empirically with lefthook 2.1.16 (`lefthook dump`,
`lefthook install`, a real commit) and by reading the loader source of
1.13.6 and 2.1.16 (`internal/config/load.go` / `loader.go`):

1. **`lefthook-local.yml` is auto-loaded**, with no reference from
   `lefthook.yml`. The variants are `lefthook-local.yml`,
   `.lefthook-local.yml` and `.config/lefthook-local.yml`, in every
   supported extension. Upstream documents it as a per-developer file and
   recommends gitignoring it.
2. **`extends:` merges listed files into `lefthook.yml`.** Paths are
   relative to the repository root, and each entry is passed through a
   glob. A **missing file matches nothing and is silently skipped**: no
   warning, no error, and the hooks still run (verified with a literal
   path and a glob). A present but malformed file makes every hook fail
   to load (`couldn't load config`, exit 1), which is loud.
3. **Merge order is main → `extends` → `remotes` → `lefthook-local`.**
   Each later layer overrides the earlier one.
4. **Merging is per hook, then per command name, then per field.** A new
   hook (`post-commit`) is added. A new command in an existing hook
   (`pre-commit.commands.extra`) is added beside the managed ones. A
   command with an existing name is merged field by field: `run:` can be
   replaced, and `skip: true` disables it.
5. **`lefthook install` registers hooks that exist only in an extended
   file** (verified: `post-commit` defined only in the extends file was
   installed and ran on commit).

Point 4 differs from Task's include in ADR 0009: Task makes a name
collision a hard error (exit 203), but lefthook lets any later layer
override or skip a managed command. That is acceptable here and is not a
new hole. Lefthook is not the gate: CI is (spec 0017, the template's own
header). Any developer can already bypass the hooks with a gitignored
`lefthook-local.yml`, `LEFTHOOK=0` or `git commit --no-verify`.

## Decision

Use lefthook's own `extends:`, pointing at a committed, project-owned
`lefthook.local.yml`:

```yaml
extends:
  - lefthook.local.yml
```

Do not use `lefthook-local.yml` for project hooks. That file is upstream's
per-developer, gitignored override, and it applies last. If VibeConform
told adopters to commit it, developers would lose their per-machine
override slot, and a committed file would override everything. This
mirrors ADR 0009's "local to this repository, not to this machine", and
it also matches that ADR's file name (`Taskfile.local.yml` /
`lefthook.local.yml`).

A docs-only fix (just document `lefthook-local.yml`) was considered and
rejected for that reason. It is still documented as the per-developer
mechanism it already is.

## Scope

### 1. Templates

- All four lefthook templates (`repotooling`, `tsrepotooling`,
  `pyrepotooling`, `monorepotooling`'s `lefthook.yml.tmpl`) gain one
  top-level `extends:` entry naming `lefthook.local.yml`, placed after
  the header comment and before `pre-commit:`. It comes with a one-line
  comment pointing at `docs/usage.md`.
- The entry is unconditional: it does not depend on any integration or
  policy. Spec 0035's appended graphify jobs are unaffected.
- `lefthook.yml` stays `resource.Generated`, and its ownership is
  unchanged.

### 2. `lefthook.local.yml` is outside the managed set

As with `Taskfile.local.yml`, `vibe` never creates, writes, reads,
validates or audits `lefthook.local.yml`. No Go code outside the
templates names it. `vibe sync` does not touch it, and `vibe audit` does
not list it.

### 3. `.gitignore`

VibeConform does not add `lefthook-local.yml` to any `.gitignore`.
ADR 0016 limits managed `.gitignore` sections to integration-owned
ignores, and whether to commit a per-developer file is the repository's
call. The docs recommend ignoring it.

### 4. Backward compatibility (spec 0019)

- The templates change bytes. So every existing adopter whose
  `lefthook.yml` is untouched sees `lefthook.yml: out of date (standard
  moved; run vibe sync to update)` and `audit` exit 3 on the first
  `vibe` release that carries this. The cause is the template change,
  not anything the adopter did. One `vibe sync` clears it. This is the
  same one-time cost every template change has (spec 0019 §1).
  `task audit` pins the recorded `vibe_version` (spec 0022), so CI only
  reports it once the adopter upgrades `vibe`.
- An adopter whose `lefthook.yml` is already hand-edited (like the
  reporter) moves from `drifted` to `conflict`, and `sync` will not
  touch it. The docs give the path: move the additions into
  `lefthook.local.yml`, restore `lefthook.yml` with `git checkout`, then
  `vibe sync`.
- The behaviour of hooks is unchanged for every repository without a
  `lefthook.local.yml`, because a missing extends target is skipped.

### 5. Documentation

- `docs/usage.md` gets a new section, "Adding your own Git hooks:
  `lefthook.local.yml`", next to the `Taskfile.local.yml` section. It
  covers:
  - an example (a `post-merge` job running a `Taskfile.local.yml` task);
  - optional / yours / merged;
  - the override semantics, including that `skip: true` on a managed
    command works, and why that is acceptable (CI is the gate);
  - `lefthook-local.yml` as the per-developer, gitignored layer that
    applies last;
  - the migration path for an already-edited `lefthook.yml`;
  - a note on Graphify: the graph jobs are generated when the
    integration is selected, so they should not be copied into the local
    file.
- `docs/usage.md`, "Removing VibeConform": the `extends` entry may stay,
  because a missing file is a no-op.
- `docs/architecture/overview.md`: the "generated may delegate to an
  unmanaged sibling" paragraph names both seams.
- `docs/architecture/principles.md`: the principle-3 table row names
  `lefthook.local.yml` beside `Taskfile.local.yml`.
- A new ADR records the decision, the rejected alternatives and the
  override semantics, because ADR 0009's "collision is a hard error"
  property does not carry over.

## Non-goals

- Making the extension un-overridable. Lefthook offers no collision
  error, and enforcing one would mean `vibe` reading the local file,
  which ADR 0009 rules out.
- Generating, validating or auditing `lefthook.local.yml`.
- `remotes:` support. It was not verified that `remotes:` inside an
  extended file is honoured. The loader reads `remotes` from the main
  config only.
- Fixing the CI-provider-specific header comment (see "Open questions").
- A minimum lefthook version check in `vibe doctor`.

## Acceptance criteria

1. Every lefthook template resolves to a `lefthook.yml` whose parsed
   top-level `extends` is exactly `["lefthook.local.yml"]`. This holds in
   all four standards, with graphify selected and unselected.
2. No Go source under `internal/` other than the template files and
   tests contains the string `lefthook.local.yml`. The managed resource
   set of every standard is unchanged (no new path).
3. Apart from the added `extends` lines, every resolved `lefthook.yml` is
   byte-identical to before. A test compares against the previous
   resolution with the `extends` block stripped.
4. A repository without `lefthook.local.yml` behaves exactly as today.
   Verified with a real `lefthook` binary when one is on PATH (skipped
   otherwise): `lefthook dump` exits 0 and shows only the managed hooks.
5. With a `lefthook.local.yml` adding a `post-merge` job and a new
   `pre-commit` command, `lefthook dump` shows both alongside the managed
   commands, and `vibe audit` still reports `lefthook.yml: ok`.
6. `vibe audit` against a repository synced by the previous release
   reports `lefthook.yml` as `out of date`, exits 3, and is clean after
   `vibe sync` (spec 0019 behaviour, recorded rather than new).
7. This repository, every example, and `TestExamplesAreConformant` are
   conformant after sync.
8. `task verify`, `verify-ci` and CI neither read nor require
   `lefthook.local.yml`, and spec 0017's verify-independence tests stay
   green.
9. The docs in §5 exist. `docs/usage.md` states the merge and override
   semantics and the lefthook versions they were verified on.

## Open questions for approval

1. **Name.** The proposal is `lefthook.local.yml`, which parallels
   `Taskfile.local.yml`. It differs from upstream's per-developer
   `lefthook-local.yml` by one character, which could be confused. The
   alternative is a clearly distinct name such as `lefthook.project.yml`.
2. **Is a one-time `out of date` for every adopter acceptable?** The
   alternative is docs-only: document committing `lefthook-local.yml`.
   That needs no template change, but it costs the per-developer slot,
   and the committed file applies last.
3. **The CI header comment.** Every lefthook template hard-codes
   `.github/workflows/ci.yml`, which is wrong for a repository whose CI
   is not GitHub (the reporter's real drift). The options are to word it
   provider-neutrally ("CI is the authoritative…") in this change, since
   it shares the same one-time `out of date`, or to file a separate
   issue.

## Assumptions not yet verified

- **Lefthook version floor.** Silently skipping a missing `extends`
  target relies on the loader globbing each entry. This was verified on
  1.13.6 (source) and 2.1.16 (run). Older versions, and Windows path
  handling of a relative entry, are unverified. VibeConform does not pin
  a lefthook version today.
- **Windows.** Behaviour was verified on Linux only. The plan includes a
  Windows check.
- **The adopter's selection.** That the adopter selects graphify is
  inferred from their file history and `AGENTS.md`. Their `vibe.yaml`
  and `vibe audit` output were not inspected.
