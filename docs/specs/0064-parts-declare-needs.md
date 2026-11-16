# Spec 0064: Execution and probe parts declare what they need

Status: accepted (2026-11-16). Issue #159, from the design review #167.
Amends spec 0053 ("Order"). No contract change: the contract stays
1.2.0, and no rule's result, finding or fix changes.

## Problem

The execution and probe parts of the audit depend on each other, and
nothing in the code says so:

1. **A string list decides the order.** R-01's execution part must run
   after A-02's probe, and after every other part that runs a command.
   Only the string list `executionOrder` in the audit runner makes that
   true. If a rule is renamed, or a new part is left off the list, the
   part runs in a fallback slot, and no test or build step fails.
2. **A-02 writes a result for R-01.** A-02 gives R-01 its verdict on
   the deny cases by writing a field of the probe session. It writes
   this field in two places: once in its corpus feed and once more as a
   fallback in its check function. R-01 reads the field. If R-01 runs
   first, it reports "A-02's probe did not run" as cannot decide, and
   the audit continues.
3. **More results go through shared fields.** A-02 and A-09 append the
   guard exception uses to a field of `Env`, and the report copies that
   field. R-01 reads the list of `vibe` calls that the probe session
   collected from every run before it.
4. **The documented rule is false.** `Env`'s doc comment says "A check
   never writes". A-02 and A-09 write to it, every check fills its
   caches, and `Note` adds to its notes.

The result is poor locality. To change one rule, you must know which
other rule reads its side effects. When the order breaks, the audit does
not stop: R-01 becomes cannot decide, which looks like a problem in the
adopter's repository and not a bug in `vibe`.

## Constraints

- **No behaviour change for adopters.** For the same repository, every
  rule's result, findings and fixes stay the same, in `--json` and in
  text. The contract stays 1.2.0 (spec 0051, "Versioning").
- **Each part starts from a reset copy** (spec 0053, "Order"). That
  stays. The probe copy, setup, the precondition and `--static` keep
  their behaviour.
- **Dependency direction** (`docs/architecture/overview.md`): the change
  stays in `internal/contract`. It adds no import and no package.
- **No production dependency.** Go's standard library is enough to sort
  a small graph.
- **Test seam.** Tests for the runner go through `contract.Audit` with
  `Options.Rules`, in package `contract_test`. Tests in
  `internal/contract/probetest` and `guardprobe_test.go` already use
  this seam. No test calls a check function directly (spec 0052,
  "Testing Decisions"). Issue #165 moves the seam for all rule tests,
  and the ADR for that decision comes with #162. This spec does not
  move any CLI test.

## Assumptions

Each of these is checked during implementation:

1. Only R-01 reads a result of another part. Apart from the `vibe`
   calls that R-01 reads and the exception uses that the report reads,
   no part reads anything that another part leaves behind.
2. The baseline runs of `verify` and `verify-fast` (V-01, V-02, R-01)
   are a cache of the same run, not a hand-off. The part that asks
   first runs the baseline, and the result is the same for every part
   that asks. The cache stays on the probe session.
3. The order of the parts does not change any result. Each part starts
   from a reset copy. Ignored files stay after a reset (spec 0053), so
   a part could see files that an earlier part built. If a fixture
   shows a difference, that difference is an undeclared need. Declare
   it or report it; do not hide it.
4. No CLI test or golden file asserts the order of the progress lines.

## Desired Behavior

### A part declares its needs

- A **need** is something that an execution or probe part needs before
  it can run. A need is one of two kinds:
  - One named part, identified by a rule ID and a part kind, such as
    A-02's probe.
  - Every other execution and probe part in the audit.
- R-01's execution part declares two needs: A-02's probe, and every
  other execution and probe part. No other part declares a need.
- Static parts declare no needs. They run first, in registry order, as
  they do now.

### The runner orders by needs

- Every execution and probe part runs after the parts it needs. When
  the needs leave a choice, the parts run in the order of the rule list
  (registry order), and in part order inside a rule. The order is the
  same in every run.
- The runner checks the needs of the rule list before it builds the
  probe copy. If a part names a part that is not in the list, the audit
  panics. It also panics on a cycle and on a need declared by a static
  part. The panic message names the part and its need. In `vibe audit`
  this cannot happen, because a registry test keeps `contract.Rules()`
  valid. Only a test can reach the panic, through `Options.Rules`.
- `executionOrder` and `partOrder` are removed. The runner keeps no list
  of rule IDs.

### Results come back through return values

- A part returns all of its output in its `PartResult`. In addition to
  its result and findings, A-02 returns its verdict on the deny cases.
  If A-02 decides before it feeds a case, that verdict is A-02's whole
  verdict, which is what the fallback write does now. A-02 and A-09
  return the guard exception uses that they found.
- The runner keeps a results store keyed by part. For each part that
  ran, it stores the `PartResult` and the `vibe` calls that the shim
  recorded while that part ran.
- A part gets the results of its needs as an argument, never through
  `Env` or the probe session. It sees only the needs that it declared.
  If a needed part did not run (its rule is not applicable, `--static`,
  or the precondition failed), that need is absent from the argument.
  The dependent part then decides on its own, as R-01 does now.
- The report's guard exception uses come from the stored results, in
  run order. A-02's uses come before A-09's, as they do now.
- Each `vibe` call is reported once, by R-01, in the run that made it.
  This includes a call made during a baseline run that another part
  started first.

### Env's doc comment matches what Env does

- `Env` holds the configuration, the tracked files, the models built
  from them (computed once and cached), the notes, and the probe session
  that execution and probe parts run in.
- A check reads `Env` and returns what it decided. `Note` is the only
  record that a check adds outside its result. The fields that A-02
  writes for R-01 and for the report (the deny verdict, the exception
  uses, and the shim's call list on the session) are removed.

## User Stories

1. As a contributor to `vibe`, I want each part to state what it needs, so that I can read a rule's dependencies at the place where the rule is defined.
2. As a contributor, I want the audit order to follow from the declared needs, so that I do not keep a separate list of rule IDs in step with the registry.
3. As a contributor who renames a rule, I want every need that names the old ID to fail at once, so that the rename cannot leave a part in a fallback slot without anyone noticing.
4. As a contributor who adds an execution or probe part, I want it to run in a deterministic place without changes elsewhere, so that adding a rule touches only that rule.
5. As a contributor who adds a part that reads another part's result, I want to declare the need and receive the result as an argument, so that the dependency is in the code and not hidden in a shared field.
6. As a contributor, I want a need on a part that is not in the rule list to panic with the part and the need named, so that a broken test setup fails before any copy is built.
7. As a contributor, I want a cycle of needs to panic, so that two parts can never wait for each other.
8. As a contributor, I want a registry test that proves every need in `contract.Rules()` resolves and has no cycle, so that `vibe audit` itself can never reach the panic.
9. As a contributor who changes A-02, I want everything that A-02 gives to other code to be in its return value, so that I can see the whole effect of A-02 in one function.
10. As a contributor who changes R-01, I want R-01's inputs (A-02's deny verdict and the `vibe` calls of the earlier runs) to arrive as arguments, so that I can understand R-01 without reading A-02.
11. As a contributor, I want A-02's deny verdict to be set in one place, so that no second write can override the first.
12. As a contributor, I want the guard exception uses that A-02 and A-09 find to reach the report from their results, so that the report does not read a field of `Env` that checks append to.
13. As a contributor, I want each `vibe` call that the shim records to belong to the part that was running, so that R-01 reports each call once, even when the baseline that made the call was cached.
14. As a contributor, I want `Env`'s doc comment to describe what checks can change, so that the comment is true and I can rely on it.
15. As a contributor, I want a part to see only the needs that it declared, so that an undeclared dependency cannot slip back in through the results store.
16. As a contributor writing a runner test, I want stand-in parts that declare needs through `Options.Rules`, so that I can test ordering and the hand-off without running real entrypoints.
17. As a contributor, I want the runner tests to assert on `*contract.Report`, so that they test behaviour that can be seen and not the internals of the sorter.
18. As an adopter, I want every rule's result, findings and fixes unchanged, so that a refactor in `vibe` does not change my verdict.
19. As an adopter, I want R-01 to continue to report "A-02's probe did not run" only when A-02 actually did not run, so that the message is true.
20. As an adopter whose repository uses no Claude Code, I want R-01's guard half to continue to pass without A-02, so that a not-applicable A-02 does not block R-01.
21. As an adopter who runs `vibe audit --static`, I want every execution and probe part to stay "not run" and no probe copy built, so that the quick run stays quick.
22. As an adopter whose setup fails, I want every execution and probe part to stay cannot decide with the setup finding, so that the dependency change does not hide the real cause.
23. As an adopter who reads the progress lines, I want the parts to run in a stable, documented order, so that two runs on the same repository look the same.
24. As a reviewer, I want spec 0053's "Order" bullet amended to describe the order by needs and registry, so that the approved spec and the code do not disagree.
25. As a maintainer of this repository, I want `task audit` to say `Verdict: conformant` with the same JSON report as before the change, so that I have evidence that nothing changed.
26. As an agent that implements a later rule, I want one documented way to read another part's output, so that I do not add a new shared field.

## Implementation Decisions

- **The need on `Part`.** `Part` gets an exported field that lists its
  needs. A need is a value that names either one part (rule ID and
  part kind) or "every other execution and probe part". The field is
  exported so that stand-in parts in `contract_test` can declare needs.
  The registry declares needs only on R-01's execution part.
- **One check signature.** `CheckFunc` takes the env and a read-only
  view of the results of the declared needs. The view returns a needed
  part's `PartResult` and whether the part ran. Unexported methods of
  the view give the typed outputs to the code in the package: A-02's
  deny verdict and the `vibe` calls of the earlier runs. Every part uses
  the same signature, and static parts get an empty view. The change is
  mechanical across the registry. It means one way to run a part, not
  two.
- **Outputs on `PartResult`.** `PartResult` gets unexported fields for
  the outputs that leave a part: the deny verdict (A-02) and the guard
  exception uses (A-02, A-09). The exported `Result` and `Findings`
  fields do not change. `Pass`, `Fail`, `Undecided` and `Merge` do not
  change.
- **The results store.** The runner owns a map keyed by part. For each
  part, the map holds the part's `PartResult` and the shim calls that
  were recorded while the part ran. The probe session gives the runner
  the calls recorded since a mark, instead of keeping one growing list
  that the parts read. The store lives for one audit and is never
  reachable from `Env`.
- **The order.** First, static parts in registry order (unchanged).
  Then execution and probe parts in a stable topological order. The
  runner always selects the first ready part in rule-list order, and
  inside a rule the first in part order. "Every other part" makes the
  declaring part run after all the parts that do not declare it. If two
  parts declare it, that is a cycle. The precondition check and the
  reset before each part do not change.
- **Fail fast.** The runner validates the needs before
  `startProbeSession` and panics with `contract: <rule>/<kind> needs
  <rule>/<kind>, which is not in the rule list` (or names the cycle, or
  the static part that declares a need). The validation runs in every
  audit, `--static` included. It does not depend on configuration, so
  applicability does not change the result.
- **Removed.** `executionOrder`, `partOrder`, `pendingPart.order`, the
  deny-verdict field on the probe session and its two writes, the
  session's shim-call list as a field that parts read, and `Env`'s
  guard-uses field.
- **R-01's behaviour.** R-01 reads A-02's deny verdict from its need.
  If A-02 is absent and Claude Code is in use, R-01 reports the same
  cannot-decide finding as now. If Claude Code is not in use, the guard
  half passes, as now. R-01 reports the `vibe` calls of every earlier
  part and of its own runs, each call once.
- **Spec 0053 amendment.** The "Order" bullet becomes: setup first;
  then the execution and probe parts in registry order, each after the
  parts it needs; R-01's execution part last. Spec 0053 gets a one-line
  pointer to this spec. The progress lines in text mode follow the new
  order. Results and findings do not change.
- **Docs.** `Env`'s doc comment is rewritten as described under
  "Desired Behavior". `docs/architecture/overview.md`, "Contract audit",
  gets one bullet: parts declare needs, the runner orders by them, and
  results come back through return values. `GLOSSARY.md` gets **Part**
  (one kind's share of a rule's check) and **Need** (what an execution
  or probe part requires to have run before it).
- **No ADR.** The change is internal to one package and adds no
  dependency. This spec records the decision.

## Testing Decisions

- **What a good test is here.** It runs `contract.Audit` and asserts on
  the `*contract.Report`: part results, findings, guard exception uses,
  and, for ordering, the order in which stand-in parts were called. It
  does not call the sorter, the store, or a check function directly.
- **Runner tests** (new, package `contract_test`, in `internal/contract`):
  - A stand-in part that needs another part runs after it, even when it
    comes first in `Options.Rules`, and it receives the other part's
    result.
  - A part with no needs runs in rule-list order.
  - A part that needs every other part runs last, and its view has the
    results of all the parts that ran.
  - A need on a part that is not in the list panics, and the message
    names both parts. A cycle panics. A static part that declares a need
    panics. Use `recover`, or `panics` helpers, inside the test.
  - A needed part whose rule is not applicable is absent from the
    dependent's view, and the dependent still runs.
  - With `Static: true`, the needs are still validated (a missing need
    still panics), no probe copy is built, and every execution and probe
    part is not run.
  - A part sees only its declared needs. A stand-in that asks for an
    undeclared part gets "absent".
- **Registry test** (extends `registry_test.go`): every need in
  `contract.Rules()` names a part that exists, no static part declares a
  need, and the graph has no cycle. R-01's execution part needs A-02's
  probe and every other part.
- **Regression net, unchanged in intent:**
  - `guardprobe_test.go`: R-01's guard half passes with the reference
    guard and fails with a guard that allows `rm -r -f`. The
    `GuardExceptionUses` slices stay equal.
  - `probetest/execution_test.go`: the R-01 cases. The cases that list
    R-01 alone add A-02 to the list, because an unlisted need now panics.
    Without Claude Code, A-02 is not applicable and costs nothing.
  - The A-07 case in which a hook calls `vibe`: R-01 reports the call
    once.
  - The CLI's `contract_probe_light_test.go` case "R-01 execution fails
    when A-02's deny cases are not refused". The CLI stand-in helpers
    (`standIn`) change only for the new `CheckFunc` signature.
- **Prior art:** `internal/contract/probetest/fixture_test.go`
  (`audit` over `Options.Rules`), `guardprobe_test.go` (`part`,
  `assertPart`), `registry_test.go`.
- **Before/after check:** run `vibe audit --json` on this repository
  and on the probetest fixtures before and after the change. Compare the
  reports and record the result in the implementation notes.

## Out of Scope

- Running parts concurrently. Needs would allow it, but the probe copy
  is one tree and each part resets it.
- Passing the probe session as an argument instead of through
  `Env.probe`. The session is where runs happen, not a result that one
  part gives to another.
- Moving notes out of `Env`.
- Moving the CLI's rule tests to `internal/contract` (#165, with #162).
- Deriving `Rule.Check` from its parts (#166, "Check text is written
  twice").
- R-01 counting calls of `vibe` by path (#120 Part B, ships with #50).
- Isolating ignored files between parts. If assumption 3 fails, report
  it as a separate issue.

## Acceptance Criteria

1. `executionOrder` and `partOrder` no longer exist. No code in
   `internal/contract` orders parts by a list of rule IDs.
2. R-01's execution part declares its needs on its `Part`. No other
   registry part declares a need. A registry test proves every need
   resolves, no static part declares one, and there is no cycle.
3. The probe session has no deny-verdict field. `Env` has no guard-uses
   field. No part reads the session's shim-call list directly.
4. A-02's deny verdict is set in one place, its return value.
5. `contract.Audit` panics, naming the part and the need, when
   `Options.Rules` lacks a needed part, has a cycle, or has a static
   part with a need. It does not panic for `contract.Rules()`.
6. Runner tests through `contract.Audit` cover: ordering by needs,
   rule-list tie-break, "every other part" last, an absent need for a
   not-applicable rule, declared-only visibility, and `--static`.
7. For this repository and the probetest fixtures, `vibe audit --json`
   gives the same rules, results, findings, fixes and guard exception
   uses before and after the change.
8. Each `vibe` call is reported once by R-01, including a call made
   during a cached baseline run.
9. `Env`'s doc comment no longer says "A check never writes", and it
   names `Note` as the only record a check adds.
10. Spec 0053's "Order" bullet, the architecture overview and the
    glossary match the code.
11. `task verify` and `task audit` pass. `task audit` says
    `Verdict: conformant`.

## Further Notes

- The issue suggested string needs such as `"A-02/probe"`. A typed
  reference (rule ID and `PartKind`) checks the kind at compile time,
  and the registry test checks the ID. A wrong ID fails the registry
  test, not a run.
- The progress lines in text mode change order. Before, they started
  with A-07 and V-03. After, they follow registry order. This is the
  only visible difference, so the spec amends spec 0053 instead of
  calling the change "no behaviour change" without qualification.
- Order in #167: this is item 2 of the contract track, after #164
  (spec 0063, merged in #173) and before #162 and #165.
