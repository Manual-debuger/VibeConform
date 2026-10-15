# Spec 0050: Guard Exceptions and Extra Guard Rules

Status: accepted (2026-10-15). Issue #80 (the umbrella), the third step after specs 0048
and 0049. Settled with the user on 2026-10-15 in the grilling of #80
(Q11, Q12, Q16). Decided by ADR 0024: rule content may be extended and
weakened through a seam, and audit reports it.

## Problem

The agent guard denies a fixed list of commands and file edits
(`internal/module/agents/claude/policy.go`, generated into
`.claude/hooks/policy.json`). An adopting repository cannot change that
list without editing a managed file, which `vibe audit` then fails:

- It cannot let one command through that a rule refuses but that is safe
  in this repository, such as `git clean -fdx build/` for a build
  directory, or an edit to a committed `.env.example`. Today the agent
  has to stop and ask the user to run it every time.
- It cannot deny a command the standard does not know is dangerous here,
  such as a deploy script or `terraform destroy`.

## Constraints

- **The floor is fixed** (ADR 0024). The guard still runs before every
  shell command and file edit, from the same `hook:guard` task, with the
  same managed `policy.json`. No seam switches the guard off.
- **Fail closed** (principle 1, spec 0021): anything the guard cannot read
  or understand denies, it never allows.
- **Three runtimes, one behaviour.** prod-go's `guard.go`, prod-ts's
  `guard.mjs` and prod-py's `guard.py` read the same files and decide the
  same way; the shared corpus test keeps them in agreement (spec 0021).
  Patterns must stay in the regex subset Go, JavaScript and Python read
  alike (spec 0021 §3).
- **Native and removable** (principle 3): the repository's file is plain
  JSON next to `policy.json`, read only by the generated guard.
- The guard is not a sandbox (AGENTS.md, `docs/usage.md`). An exception
  is a convenience the repository takes responsibility for.
- prod-mono has one guard at the repository root; so does this spec.

## Assumptions

- **A1.** The repository's file is committed, like `Taskfile.local.yml`:
  its exceptions and rules apply to everyone working in the repository.
  The managed `.gitignore` section lists only `.claude/settings.local.json`,
  so it is not ignored.
- **A2.** A guard runtime can test whether a pattern matches a whole
  string in all three languages within the portable subset (an anchored
  `^(?:…)$`).

## Desired Behavior

### The repository's file

- **D1.** The guard reads an optional `.claude/hooks/policy.local.json`
  next to `policy.json`. VibeConform never creates, writes or deletes it.
  Without it the guard behaves exactly as today.
- **D2.** Its shape follows `policy.json`: a `version` (1), an optional
  `rules` list of extra deny rules in the same shape as the managed rules
  (`id`, `field`, `pattern`, `raw`, `message`), and an optional
  `exceptions` list. Each exception has a `rule` (a rule id), a `pattern`,
  and an optional `reason` for the reader.

### Extra rules

- **D3.** An extra rule denies like a managed rule, after the managed
  rules, with its own message. Its `field` is one the managed policy
  knows (`command`, `file_path`); `raw` is matched in the same fallback.
  A pattern that does not compile denies, as for managed rules.

### Exceptions

- **D4.** An exception lifts exactly one rule, the one its `rule` names,
  for a tool call whose field value **the pattern matches in full**, not
  just in part. Every other rule still applies to that call. Matching the
  whole value is what keeps `git clean -fdx build/; rm -rf ~` denied when
  the exception names `git clean -fdx build/`.
- **D5.** Exceptions apply only to a parsed tool call. In the raw
  fallback (a payload the guard cannot parse) nothing is lifted.
- **D6.** An exception whose pattern does not compile lifts nothing: the
  call stays denied. There is no exception without a rule id, and no way
  to lift every rule at once.

### A broken file

- **D7.** If the file exists but cannot be read or parsed, has another
  `version`, or breaks the shape of D2 (an exception without `rule` or
  `pattern`, a rule without `id`, `field`, `pattern` or `message`, an
  unknown `field`), the guard denies **every** tool call. The deny message
  names `.claude/hooks/policy.local.json` and says to fix it or ask the
  user. Ignoring the file would silently drop its extra rules (Q16).

### Audit

- **D8.** `vibe audit`, `diff` and `sync` report as a conflict, without
  writing the file:
  - everything D7 makes the guard deny on;
  - an exception naming a rule id that neither file defines;
  - an exception or extra rule whose pattern does not compile;
  - an extra rule whose `id` repeats a managed or another extra rule's.
- **D9.** Audit lists each exception as a non-failing line naming the
  rule and the pattern, so a reviewer sees every place the repository
  weakened the guard. Extra rules are listed the same way.
- **D10.** A repository without the file reports exactly as today.

### The file is the user's

- **D12.** The guard denies an agent's Edit, MultiEdit or Write of
  `.claude/hooks/policy.json` and `.claude/hooks/policy.local.json`
  (managed rule `guard-policy-file`) and tells it to ask the user. An
  agent refused by the guard cannot write itself an exception. A shell
  write is not covered: the guard is not a sandbox. Added with plan 0050's
  approval.

### Documentation

- **D11.** `docs/usage.md` ("The agent guard") documents the file, an
  example of each kind, the whole-value match, the fail-closed behaviour
  and how to check it with the canary. The managed `policy.json` rule
  ids are the names an exception uses, so the docs list them.

## Non-goals

- A guard per prod-mono component.
- Lifting a rule by anything but its id, or disabling the guard.
- Exceptions in the raw fallback.
- A `.claude/settings.local.json`-style personal, gitignored variant.
- Codex: its hooks are suspended (ADR 0011).
- An extra check in `task verify` (a later spec, after a Task probe).

## Acceptance Criteria

- [ ] The shared guard corpus covers, for all three runtimes alike: an
      exception lifting its rule for a whole-value match; the same
      command with a dangerous suffix still denied; another rule still
      applying to an excepted command; an exception with a broken pattern
      lifting nothing; an extra rule denying; a broken local file denying
      everything with a message naming it; no local file behaving as
      today; edits of either policy file denied (D12).
- [ ] The CI guard jobs (`Hook guard / gate`) run that corpus on Linux
      and Windows.
- [ ] `vibe audit` reports each D8 problem as a conflict and each
      exception and extra rule as a non-failing line; with no file the
      output is unchanged.
- [ ] The canary in `docs/usage.md` shows, in a live session in this
      repository, an excepted command running and the same command with
      another destructive part still denied. Recorded in the ledger.
- [ ] `docs/usage.md` carries D11.
- [ ] This repository and the three examples are synced and conformant;
      `task verify` and `task audit` pass.
