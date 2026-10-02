# ADR 0017: A project-owned `lefthook.local.yml` through `extends`

## Status

Accepted. Implemented per `docs/specs/0036-lefthook-local-extension.md`.
Applies ADR 0009's seam to `lefthook.yml`.

## Context

`lefthook.yml` is generated: VibeConform owns the whole file (ADR 0003).
A repository that needs a Git hook of its own (a `post-merge` install, a
`commit-msg` linter, an extra `pre-commit` check) had nowhere conformant
to put it. Adding it to `lefthook.yml` made `vibe audit` report drift.
ADR 0009 solved the same problem for `Taskfile.yml` with an optional
include of a project-owned `Taskfile.local.yml`.

Lefthook already has two merge mechanisms, both verified on 1.13.6
(source), 2.1.14 (Windows) and 2.1.16 (Linux):

- `lefthook-local.yml` is auto-loaded, last, and is upstream's
  per-developer, gitignored file.
- `extends:` merges listed files after the main config and before
  `lefthook-local.yml`. A missing file matches nothing and is skipped
  silently. A malformed one fails every hook loudly.

## Decision

1. Every lefthook template carries one unconditional top-level entry:
   `extends: [lefthook.local.yml]`, after the header comment. The file is
   committed and project-owned.
2. `vibe` never creates, writes, reads, validates or audits
   `lefthook.local.yml`. No Go code outside templates and tests names it.
3. The template header no longer names a CI file. It says "CI is the
   authoritative full verification gate", so it is true whichever CI
   system the repository uses (spec 0038).
4. VibeConform does not add `lefthook-local.yml` to `.gitignore`
   (ADR 0016 limits managed ignores to integration output). The docs
   recommend ignoring it.

## Consequences

- Merge order is `lefthook.yml` → `lefthook.local.yml` → `remotes` →
  `lefthook-local.yml`. It merges per hook, then per command name, then
  per field. A project can add hooks and commands, and can also replace
  a managed command's `run:` or set `skip: true` on it.
- That differs from ADR 0009, where Task makes a name collision a hard
  error. It is accepted because lefthook is not the gate: CI is
  (spec 0017), and every developer can already bypass hooks with
  `LEFTHOOK=0`, `--no-verify` or a gitignored `lefthook-local.yml`. The
  override costs feedback speed, never conformance.
- Without `lefthook.local.yml`, hook behaviour is unchanged.
- The template change moves every untouched adopter's `lefthook.yml` to
  `out of date` once, cleared by one `vibe sync` (spec 0019). A
  hand-edited `lefthook.yml` becomes a `conflict`. The documented path is
  to move the additions into `lefthook.local.yml`, `git checkout
  lefthook.yml`, and `vibe sync`.
- Removing VibeConform can leave the `extends` entry in place.

## Alternatives considered

- **Document committing `lefthook-local.yml`.** No template change, but it
  takes the per-developer slot away and makes a committed file apply
  last, over everything. Rejected.
- **`remotes:`.** Meant for shared configuration fetched from another
  repository. Unverified inside an extended file. Rejected.
- **A managed section in `lefthook.yml`** (ADR 0014). YAML has no markers
  that survive reformatting reliably, and a section would still mean
  editing the managed file. Rejected.
- **`structured-patch` ownership of `lefthook.yml`.** It would make VibeConform
  reason about the project's hooks in its own file. Rejected for the
  reason ADR 0009 rejected the same for `Taskfile.yml`.
