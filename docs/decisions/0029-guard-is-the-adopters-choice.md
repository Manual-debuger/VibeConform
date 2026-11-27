# ADR 0029: The Agent Guard Is the Adopter's Choice

## Status

Accepted (2026-11-27). Issue #250, ticket #254. Amends decisions 9 to 11
of ADR 0025: they hold whenever a repository requires the guard set. The
rules are in `docs/specs/0051-contract-v1.md` ("A: agent guardrails")
and `docs/specs/0074-contract-2-1-guard-optional.md`.

## Context

ADR 0025 made the agent guard part of the contract's floor. A-01 to
A-04 and A-08 were MUST rules: every PreToolUse call passes through a
guard that refuses the baseline corpus, fails closed, is loosened only
through listed exceptions, and locks the guardrail settings until a
person unlocks them.

In practice only `vibe-guard` passes the guard corpus. So every
conformant Claude Code repository runs one maintainer's binary before
every Bash, PowerShell, Write and Edit call, and lets it lock
`.claude/settings.json`. Some adopters will not run an outside binary in
that position. Their only way out was `agents: []`, which also drops
A-05 to A-07, the hook rules they do want, and ADR 0025 forbids waiving
a MUST rule.

The contract's promise does not depend on the guard. "Conformant" means
that the agent's work is verified, at the end of each turn and in CI,
with the repository's own tools. The guard limits what the agent can
do between those checks; it is a second line, and its cost falls on the
adopter.

## Decision

1. **The guard set.** A-01, A-02, A-03, A-04 and A-08 form the *guard
   set*. Under contract 2.1.0 they are SHOULD rules; under 1.x they stay
   MUST. A rule's level depends on the audited contract (spec 0074), so
   a 1.x repository's verdict does not change.
2. **A repository opts in by requiring the whole set.** `require` (spec
   0063) lists all of the guard set or none of it. A repository that
   requires it gets the verdict it had when the set was MUST. Part of
   the set is a configuration error: a guard that is hooked up but not
   tested proves nothing, and a tested guard that is not hooked up does
   nothing. `require` with A-09 needs the whole set too, so that a
   skip-hooks check does not stand in for a guard.
3. **Guarded is the default.** `vibe init` and the setup skill's
   reference `vibe.yaml` files write the guard set in `require`. An
   adopter that does not want the guard deletes that line, which makes
   the choice visible in its own file. This repository requires the
   guard set.
4. **ADR 0025's decisions 9 to 11 hold whenever the guard set is
   required.** A missing guard fails closed, loosening is explicit and
   listed, and the guardrail settings need a person's unlock. They are
   no longer part of every conformant repository.
5. **The core stays MUST.** V-01 to V-04, C-01 to C-04, A-05, D-01 and
   R-01 do not change. R-01's guard half (the A-02 deny cases are still
   refused without `vibe`) applies where A-02 counts in the verdict:
   under 1.x, or when the guard set is required. Otherwise a MUST rule
   would require the guard through another door.
6. **No guard is reported, not hidden.** A-01 still reports its result,
   so the report of an unguarded repository says that its agent is not
   guarded. The rules that test a guard become not applicable when
   there is none (spec 0074, ticket #255).

## Consequences

- Lowering a MUST rule to SHOULD is a relaxed rule, so 2.1.0 is a MINOR
  release (spec 0051, "Versioning"). One configuration that 2.0.0
  accepted is refused: `require` with A-09 and not the guard set. Adding
  the guard set fixes it and changes no verdict that 2.0.0 gave.
- An adopter can waive a guard rule under 2.x like any SHOULD rule, and
  cannot waive one it requires.
- `vibe-guard` stays the reference guard and the default. Its trust
  model (a pinned, checksum-checked install, no network calls) is a
  separate change.
- An unguarded repository depends on the Stop hook and CI alone to
  catch an agent's mistakes. The report states that, and a reviewer
  sees the missing `require` line in `vibe.yaml`.
