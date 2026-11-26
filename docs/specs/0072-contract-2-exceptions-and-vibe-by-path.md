# Spec 0072: Contract 2.0.0: an exception counts only when the guard needs it, and R-01 sees `vibe` by path

Status: approved (2026-11-26). Issue #230, for #227 and #120 (Part B). A MAJOR
contract release under spec 0051, "Versioning". #50 (Codex in the A
rules) is not part of it: it waits for stable Codex hooks (ADR 0011)
and becomes a later MAJOR.

## Problem

Two checks accept repositories that do not meet the purpose of their
rule.

1. **A-02 counts a guard exception the guard did not need** (#227).
   When the guard lets a deny case through, A-02 accepts the case if a
   `policy.json` exception matches it under the audit's reading
   (`Exception.Matches`). A-02 never checks that the exception is the
   reason the guard let the case through, and the audit's reading is
   wider than `vibe-guard`'s:
   - **The guard does not have to refuse the case at all.** A guard that
     lets `rm -rf build` through always, in a repository whose
     `policy.json` holds `{rm-rf, "rm -rf build"}`, passes A-02.
   - **Compound commands.** `vibe-guard` lifts a rule only for the
     simple command that rule refused. The audit counts an exception
     whose words prefix *any* simple command of the case. So
     `{rm-rf, "cd /tmp"}` counts for `cd /tmp && rm -rf build`, and a
     third-party guard that lets the whole command through because
     `cd /tmp` matches passes A-02. The corpus has 11 compound deny
     cases.
   - **Fixed rules.** `vibe-guard` never lifts the self-unlock command
     (`vibe unlock`, or a command that names the unlock marker) or an
     edit of the unlock marker, whatever the policy says. Both carry the
     guard rule ID `guardrail-settings`. The audit counts a
     `guardrail-settings` exception for them, so a third-party guard
     that lets an agent unlock itself through an exception passes A-02.

   A-09 reads exceptions the way A-02 does (spec 0061), so it has the
   same gaps.

   Spec 0070 kept both readings and added `TestTwoReadingsOfAnException`,
   which proves only that the guard's reading is inside the audit's on
   the corpus. It moved the tightening to the next MAJOR.

2. **R-01 sees `vibe` only by name** (#120, Part B). R-01's static part
   counts a simple command as a call of `vibe` when its first word is
   the literal `vibe`. A step that runs `"$dir/vibe" verify`,
   `./bin/vibe verify` or `vibe.exe verify` is not seen, so a
   repository can make its checks depend on `vibe`, which R-01 forbids
   (principle 3), and pass.

Each fix can make a conformant repository non-conformant, so each is
MAJOR. They ship together as contract 2.0.0.

## Solution

- **Contract 2.0.0.** `vibe` implements contract 2.0.0 and still audits
  contract 1.x. A repository that declares 1.x is audited as 1.2.0,
  exactly as today, and moves to 2.0.0 when it changes its `contract:`
  line. A repository that declares 2.0.x is audited as 2.0.0.
- **Under 2.0.0, A-02 and A-09 count an exception for a deny case only
  when all of these hold:**
  1. the guard lets the case through;
  2. an exception in `policy.json` matches the case under the audit's
     reading, which 2.0.0 narrows to the guard's: a command exception
     must prefix the simple command the case's rule refuses, and no
     exception matches a fixed case;
  3. the guard refuses the case when every exception that matches it is
     removed from `policy.json`.
  A case that the guard lets through even without those exceptions
  fails, with a finding that says so.
- **Under 2.0.0, R-01 counts a call of `vibe` by path.** A simple
  command calls `vibe` when the last path element of its first word is
  the literal `vibe` or `vibe.exe`. The directory part may hold an
  expansion. C-04 recognizes its audit job by the same reading, so an
  audit job that runs `vibe audit` by path is still exempt.

## Constraints and Assumptions

Constraints:

- **Contract 1.x verdicts do not change.** For a repository that
  declares 1.x, every rule's result, findings and fixes stay the same.
  The CLI's report goldens and the contract's golden tests do not
  change.
- **The guard corpus belongs to the contract** (GLOSSARY, "Guard
  corpus"). New corpus fields are part of 2.0.0. No deny case is added
  or removed.
- **The probe copy** (GLOSSARY): the audit may rewrite `policy.json` in
  the probe copy only, and must restore it before the next part runs.
- **The dependency direction** (`docs/architecture/overview.md`):
  `internal/contract` does not import `internal/guard`. The `depguard`
  rules do not change. Which corpus cases are fixed, and which simple
  command a case's rule refuses, are corpus data, not imported guard
  code.
- **Rule tests go through `contract.Audit`** (ADR 0028).
- **No production dependency.**

Assumptions, each checked during implementation:

1. A third-party guard reads exceptions from `policy.json` (spec 0051,
   spec 0053), so the audit can remove an exception by rewriting that
   file in the probe copy. A guard that ignores `policy.json` lifts
   nothing, and its result does not change.
2. For each of the 11 compound deny cases, exactly one simple command
   carries the case's rule. If a case has more than one, the corpus
   names each of them and an exception may prefix any one.
3. The fixed cases are exactly the corpus deny cases that
   `vibe-guard`'s fixed rules refuse: the self-unlock commands and the
   edits of the unlock marker.
4. This repository calls `vibe` by name everywhere it calls it (checked
   at 796700f: `release.yml` and `setup-action.yml` run `vibe --version`
   by name since spec 0055; the by-path lines are `go build -o` targets
   and a `for` list). So `task audit` stays conformant under 2.0.0.
5. With the narrowed reading, `vibe-guard`'s reading and the audit's are
   equal on the corpus. Any case where they still differ is listed in
   the readings test with its reason.

## User Stories

1. As an adopter, I want A-02 to fail when my guard lets a deny case
   through no matter what my policy says, so that an exception cannot
   hide a guard that does not refuse the case.
2. As an adopter, I want the A-02 finding to name the case and the
   exception that did not change the guard's answer, so that I can see
   the guard is the problem, not the policy.
3. As an adopter, I want an exception for `cd /tmp` not to count for
   `cd /tmp && rm -rf build`, so that the audit's verdict matches what
   my exception says.
4. As an adopter, I want no exception to count for the self-unlock
   command or the unlock marker, so that the audit refuses a guard that
   lets an agent unlock itself.
5. As an adopter whose exception the guard really uses, I want my
   verdict to stay conformant and my exception to stay listed in the
   report, so that 2.0.0 costs me nothing when my guard is correct.
6. As an adopter, I want A-09 to read exceptions as A-02 does, so that
   `git-skip-hooks` has the same rules as the other guard rules.
7. As an adopter, I want R-01 to fail a CI step, hook or entrypoint that
   runs `vibe` by path, so that my checks cannot depend on `vibe`
   without the audit seeing it.
8. As an adopter, I want R-01 to keep exempting `vibe --version` in a
   job that calls no entrypoint, and the C-04 audit job, when they run
   `vibe` by path, so that the exemptions do not depend on how the
   binary is called.
9. As an adopter on contract 1.x, I want my verdict to stay the same
   after I upgrade `vibe`, so that I choose when to move to 2.0.0.
10. As an adopter, I want the report header to show the declared and
    the audited contract version, so that I can see which rules I was
    audited against.
11. As an adopter who declares a contract `vibe` does not implement
    (2.1.0, 3.0.0, 0.9.0), I want "cannot decide; upgrade vibe", as
    today.
12. As an adopter, I want `upgrade.md` in the setup skill to say what
    2.0.0 changes and how to move to it, so that I can upgrade without
    reading the specs.
13. As an adopter who runs the setup skill on a new repository, I want
    it to write `contract: 2.0.0`, so that new repositories start on the
    latest contract.
14. As a guard author, I want `--explain A-02` to say how an exception
    counts under 2.0.0, so that I know what my guard must do.
15. As a maintainer, I want the corpus to say which deny cases are fixed
    and which simple command a compound case's rule refuses, so that the
    audit does not import the guard's rules.
16. As a maintainer, I want a test that fails when the corpus's fixed
    cases or refused commands and `vibe-guard`'s rules disagree, so that
    the two cannot drift.
17. As a maintainer, I want the readings test to check both directions,
    so that it proves the readings are equal, not only that one is
    inside the other.
18. As a maintainer, I want the rerun without the exception to happen
    only for the deny cases an exception let through, so that the audit
    does not get much slower.
19. As a maintainer, I want the audit to restore `policy.json` in the
    probe copy after the rerun, so that later parts see the repository's
    own policy.
20. As a maintainer, I want this repository's `vibe.yaml` to declare
    2.0.0 and `task audit` to stay conformant, so that the repository
    follows the contract it ships.
21. As a maintainer, I want spec 0051's "Changes" to list 2.0.0 and why
    each change is MAJOR, as it does for 1.1.0 and 1.2.0.

## Implementation Decisions

### Contract versions

- `vibe` implements two contract versions: 1.2.0 and 2.0.0. A declared
  `1.y.z` is audited as 1.2.0 and a declared `2.0.z` as 2.0.0. Any other
  declared version has no verdict ("cannot decide; upgrade vibe"), as
  today.
- The audited version is available to the check functions through
  `Env`. Only the checks this spec changes read it: A-02, A-09, R-01's
  static part and C-04's recognition of the audit job.
- The report header keeps its format and shows both versions when they
  differ. `vibe --version` shows the latest contract it implements,
  2.0.0.
- A rule's `Since` stays informational. No rule is added or removed.

### Guard corpus

- A deny case gains two optional fields:
  - **fixed**: no exception can lift the case. It is set on the
    self-unlock commands and the edits of the unlock marker.
  - **refuses**: the simple command or commands of the case that its
    rule refuses, written as the simple command's words. It is required
    on a command case that decomposes into more than one simple command,
    and absent otherwise.
- A corpus test in `internal/contract` requires `refuses` on every
  compound command case and checks that each value is one of the case's
  simple commands, using the `guardrule` decomposition.
- A test in `internal/guard` checks the corpus against `vibe-guard`'s
  rules: a case is `fixed` exactly when a fixed rule refuses it, and its
  `refuses` names exactly the simple commands its rule refuses.

### The audit's reading under 2.0.0

- `guardrule` keeps one exported matching function. Under 2.0.0 the
  audit calls it with the case's `refuses` commands instead of every
  simple command, and does not call it for a fixed case. Under 1.2.0 it
  behaves as today.
- Path exceptions are not changed: they already match the one path the
  case edits or writes.

### The rerun

- `feedCorpus` runs every case as today. Then, under 2.0.0, for each
  deny case that the guard let through and an exception matches, it runs
  the case again with every matching exception removed from the probe
  copy's `policy.json`:
  - refused: the exception counts and is listed in the report's
    exception uses, as today;
  - let through: the case fails with a finding naming the case, the
    exception and the hooks' exits, and the fix "make the guard refuse
    it; an exception counts only when the guard refuses the case
    without it";
  - not decided: the case is undecided, as today.
- Cases that remove the same set of exceptions are rerun together, with
  the existing concurrency and unlock-state grouping. `policy.json` is
  one file, so the sets run one after another.
- The rewrite keeps every other entry of `policy.json`. The audit
  restores the original file after the reruns, also on an error.
- R-01's execution part reads A-02's deny verdict, so it gets the new
  reading without a change.

### R-01 and C-04

- One recognition of a call of `vibe`, shared by R-01's three places
  (entrypoints, hooks, CI steps) and by C-04's audit-job test. Under
  2.0.0, a simple command calls `vibe` when the last path element of its
  first word, after `/` or `\`, is the literal `vibe` or `vibe.exe`. The
  directory part may hold an expansion. A word whose last element holds
  an expansion is not a call, as today. Under 1.2.0 the first word must
  be the literal `vibe`, as today.
- `vibe-guard` by any path is not a call of `vibe`.
- The shell model must expose a word's literal tail after its last
  expansion. Today it exposes only whether the whole word is literal.

### Documentation and configuration

- Spec 0051, "Changes": a 2.0.0 entry with each change and why it is
  MAJOR. Spec 0053, "Guard rule IDs and exceptions, shared": the audit's
  reading equals the guard's under 2.0.0. Spec 0070, "Further Notes":
  points here. Spec 0061: A-09 under 2.0.0.
- `docs/usage.md`: "Contract versions" names both versions, and A-02
  and R-01 describe their 2.0.0 behaviour.
- The setup skill: the reference `vibe.yaml` files and `SKILL.md` write
  `contract: 2.0.0`, `rules.md` describes A-02 and R-01 under 2.0.0, and
  `upgrade.md` gets a 1.x to 2.0.0 section.
- `--explain A-02` and `--explain R-01` describe the 2.0.0 reading.
- This repository's `vibe.yaml` declares `contract: 2.0.0`.
- GLOSSARY: "Guard exception" says when an exception counts. A new
  entry, "Fixed case", if the word is used outside the corpus.

## Testing Decisions

- A good test runs the audit and asserts on the report: the rule's
  result, findings and the listed exception uses. It does not call
  `feedCorpus`, the tally or the matching function directly.
- **A-02 and A-09** through the existing probe seam (`probeAuditRules`
  on a `probeFixture` with a scripted PreToolUse hook; prior art:
  `guardprobe_reporting_test.go`, `guardprobe_test.go`). Cases:
  - a hook that refuses a case without the exception and lets it
    through with it: pass, and the exception is listed;
  - a hook that lets the case through whatever `policy.json` says:
    fail under 2.0.0, pass under 1.2.0;
  - an exception that prefixes only another simple command of a
    compound case: fail under 2.0.0, pass under 1.2.0;
  - a `guardrail-settings` exception for a fixed case that the hook
    honours: fail under 2.0.0, pass under 1.2.0;
  - `policy.json` in the probe copy is the original after the probe
    (a later part sees the original);
  - a hook that does not end during the rerun: undecided.
- **R-01 and C-04** through rule tests (prior art: the R-01 tests in
  `engine_test.go`, `probe_light_test.go` and the CI goldens). Cases
  under 2.0.0: `"$dir/vibe" verify`, `./bin/vibe verify`,
  `C:\tools\vibe.exe verify` and `${RUNNER_TEMP}/bin/vibe verify` fail
  R-01; `"$dir/vibe-guard" hook` and `"$bin" verify` do not; an audit
  job that runs `"$dir/vibe" audit` satisfies C-04 and is exempt; `vibe
  --version` by path in a job that calls no entrypoint is exempt. The
  same inputs under 1.2.0 keep today's results.
- **Versions** (prior art: the version tests in `config_test.go`):
  1.0.0, 1.2.0 and 1.2.5 audit as 1.2.0; 2.0.0 and 2.0.3 audit as
  2.0.0; 2.1.0, 3.0.0 and 0.9.0 cannot be decided.
- **Readings** (`TestTwoReadingsOfAnException` in `internal/guard`):
  checks both directions under 2.0.0. For every exception it builds,
  `vibe-guard` lifts the case exactly when the audit's 2.0.0 reading
  counts the exception. The cases the readings test lists as different
  are the only ones.
- **Corpus**: the two corpus tests above.
- The behaviour proof for 1.x: every existing golden and expected value
  stays unchanged, because the fixtures declare 1.x.

## Out of Scope

- #50, Codex in the A rules. It waits for stable Codex hooks and will be
  its own MAJOR.
- New deny or allow cases in the corpus.
- R-01's execution part (the PATH shim). A call by path during the
  execution probe is not intercepted. The static part now sees it.
- Dropping contract 1.x support. A later spec can decide when `vibe`
  stops auditing 1.x.
- `vibe-guard`'s own behaviour. It already reads exceptions the
  narrowed way. Only its tests change.
- The `vibe` release version and tag. The release process picks the
  version number when it ships 2.0.0.

## Acceptance Criteria

- [ ] A repository that declares `contract: 2.0.0` is audited as 2.0.0,
      and one that declares 1.x is audited as 1.2.0. Other versions
      cannot be decided.
- [ ] Under 2.0.0, A-02 and A-09 fail a deny case that the guard lets
      through without the matching exceptions, with a finding that
      names the case and the exception.
- [ ] Under 2.0.0, an exception that prefixes only a simple command the
      case's rule does not refuse does not count, and no exception
      counts for a fixed case.
- [ ] Under 2.0.0, an exception the guard needs still counts and is
      listed in the report.
- [ ] `policy.json` in the probe copy is restored after the rerun.
- [ ] Under 2.0.0, R-01 fails a call of `vibe` or `vibe.exe` by path in
      an entrypoint, a hook or a CI step. C-04's audit job and the
      `vibe --version` exemption recognize `vibe` by path too.
- [ ] Under 1.2.0, no rule's result, finding or fix changes. No existing
      golden file or expected value changes.
- [ ] The corpus marks the fixed cases and the refused commands of the
      compound cases. A test in `internal/guard` keeps them equal to
      `vibe-guard`'s rules.
- [ ] `TestTwoReadingsOfAnException` checks both directions under 2.0.0.
- [ ] Specs 0051, 0053, 0061 and 0070, `docs/usage.md`, GLOSSARY and the
      setup skill's references describe 2.0.0. The setup skill writes
      `contract: 2.0.0`.
- [ ] This repository's `vibe.yaml` declares 2.0.0.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.

## Further Notes

- **Why not wait for #50.** #120 Part B and the issue for this spec
  planned one MAJOR batch with #50. #50 cannot start until Codex hooks
  are stable, and ADR 0011 gives no date. The user chose to ship 2.0.0
  without it (2026-11-26). Codex becomes 3.0.0, or part of whichever
  MAJOR comes next.
- **Why keep 1.x.** Spec 0051 says a `vibe` release audits every MAJOR
  it supports. If `vibe` dropped 1.x, upgrading `vibe` would leave every
  adopter's audit with no verdict until they change `contract:`. The
  user chose to keep 1.x (2026-11-26). This is the first time `vibe`
  supports two MAJORs.
- **Why the rerun alone is not enough.** The rerun catches a guard that
  does not refuse the case at all. It does not catch a guard that lifts
  a compound command because of an exception for another simple command
  of it, or lifts a fixed rule: such a guard refuses without the
  exception and lets the case through with it. The narrowed reading
  catches those. #227's "Done when" names only the rerun. This spec
  adds the narrowed reading because #227's "Why" names those two gaps.
- **Why the rerun is needed as well as the narrowed reading.** The
  narrowed reading checks what the exception says. Only the rerun shows
  that the guard acted on it (spec 0070, "Further Notes").
- **Cost.** The rerun runs only the deny cases an exception let through.
  This repository's `policy.json` has no exceptions (checked at
  796700f), so its audit runs no rerun.
- `vibe` and `vibe.exe` are compared case-sensitively, as the literal
  `vibe` is today. `VIBE.EXE` by path is not counted. This is a choice,
  not something spec 0052 decides.
