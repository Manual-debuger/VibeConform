# ADR 0024: The floor is fixed; rule content may be extended and weakened

## Status

Accepted (2026-10-15). Implemented first by `docs/specs/0049-lint-base-configs.md`
(lint base configs), then spec 0050 (guard exceptions). Issue #80.

## Context

Every generated file has so far been all or nothing: the repository
takes it byte for byte, or `vibe audit` fails. That is right for the
files that decide what the gate runs, and wrong for rule content. An
adopter that needs a React plugin, a rule switched off for one
directory, or one command the agent guard refuses has to edit a
managed file, or build a second config beside the standard that no
gate runs (issue #80, spec 0048).

Earlier seams already split the two. `Taskfile.local.yml` (ADR 0009)
adds tasks but cannot redefine one. `lefthook.local.yml` (ADR 0017) may
replace or skip a hook command, because lefthook is not the gate.
`.gitlab-ci.defaults.yml` (ADR 0021) sets a job's environment, never
what it runs. None of them states the rule they share.

## Decision

1. **The floor is fixed.** The floor is what decides what the gate runs
   and whether its result counts: `task verify` and its subtasks, `task
   audit`, the agent hook tasks, and the CI floor (ADR 0021). No seam
   can change it. A seam that could would make a guardrail that can be
   switched off without anything noticing (principle 1).
2. **Rule content may be extended and weakened.** Which lint rules
   apply, and which commands the agent guard refuses, are the
   repository's to adjust. Adding is the common case; switching a rule
   off is allowed too, because the alternative is a second config the
   gate never runs.
3. **A weakening is reported, not refused.** `vibe audit` shows each
   place where the repository changed rule content as a non-failing
   line, so a reviewer sees it. What fails is losing the link to the
   managed content: a repository file that no longer extends its base.
4. **Each seam is the tool's own mechanism** (principle 3). ESLint
   imports `eslint.base.config.js` from `eslint.config.js`; Ruff
   `extend`s `ruff.base.toml` from `ruff.toml`; TypeScript `extends`
   `tsconfig.base.json` from `tsconfig.json`. The repository's file
   carries the tool's usual name, and the managed half carries `base`.
   Removing VibeConform leaves configs that still work.
5. **A repository file is created once.** When it is missing, `sync`
   writes a minimal file that only extends the base; once it exists,
   VibeConform never writes it again (ADR 0003). Audit checks only that
   it still names its base, as it checks `.gitlab-ci.defaults.yml`'s
   shape (ADR 0021).

## Consequences

- An adopter extends the standard in the file a tool's documentation
  points to, and the gate runs the result.
- A repository can switch off every rule in its `eslint.config.js` and
  audit still passes, with an `extended` line. That is the cost of
  decision 2; review, not tooling, catches it.
- The text check of decision 5 is defeated by a comment naming the
  base. It stops accidents, as the guard does.
- golangci-lint has no `extends`, so prod-go keeps one whole
  `.golangci.yml`. Prettier and pyright get a seam when an adopter
  asks.
- Existing adopters move over in one `sync` when they never edited the
  managed config; an edited one is a conflict they resolve once.

## Alternatives considered

- **Add-only seams.** Safer, and the maintainer's first recommendation.
  Rejected by the user: an adopter that must switch a rule off would
  still build a second config outside the gate, which is worse than a
  reported weakening.
- **Merge patches (RFC 7386) or `.vibe/overrides/` applied by `sync`.**
  A merge patch replaces a list whole, so it cannot add one item; the
  result exists only after `sync`, so removing VibeConform loses it.
- **Rule settings in `vibe.yaml`.** Every tool option would need a
  manifest key and a release.
- **The managed file keeps the tool's name and imports a repository
  file.** The repository's half would then need a name each tool does
  not look for, and removing VibeConform would leave the managed half
  behind as the live config.
