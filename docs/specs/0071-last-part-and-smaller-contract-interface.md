# Spec 0071: One last part instead of needs, and a smaller `contract` interface

Status: draft (2026-11-26). A refactor of `internal/contract` from a
design review on 2026-11-26 (`main` at 8d66ee5). Supersedes spec 0064's
needs ("A part declares its needs", "The runner orders by needs") and
keeps the rest of spec 0064 ("Results come back through return
values"). No contract change: the contract stays 1.2.0, and no rule's
result, finding or fix changes.

## Problem

`internal/contract` is the largest package of this repository: about
12,000 lines in 50 files. Two parts of its design are more complex than
what they do.

1. **A general dependency graph serves one part.** Spec 0064 lets every
   execution and probe part declare needs: one named part, or every
   other execution and probe part. The runner sorts the parts by their
   needs, finds cycles and panics on four kinds of invalid needs. In
   the registry, one part declares needs: R-01's execution part, which
   needs A-02's probe and every other part. The other parts declare
   none. The cost of the general graph:
   - Every check function takes a `Needed` argument. 33 of the 34
     check functions ignore it (`_ Needed`).
   - A reader must learn `Need`, `NeedPart`, `NeedEveryOther`, `Needed`
     and the run order to find out that R-01's execution part runs last
     and reads the earlier results.
   - Seven runner tests and one registry test test the graph: named
     needs, cycles, a part that sees only its declared needs. None of
     this behaviour is used by a rule.

   If the graph is deleted, its complexity does not come back somewhere
   else, because there is one consumer. It is a seam with one adapter.

2. **The package interface is much larger than its callers use.** About
   50 exported identifiers of `internal/contract` have no caller
   outside the package: no other package, not `contracttest` or
   `probetest`, and no test in package `contract_test`. Examples: the
   CI model (`CI`, `CIJob`, `CIRun`, `CISkip`, `CIScript`, `CIUses`,
   `CIImage`, `CIInclude`, `PipelineKind`, `Env.CI`), the GitHub
   workflow types (`Workflows`, `Workflow`, `Job`, `Step`,
   `Env.Workflows`), the command helpers (`Called`, `Uncalled`,
   `CallsAny`, `ParseCommands`, `ParseRunStep`, `ParseShellScript`,
   `Script`), `Env.Entrypoint`, `Env.EntrypointSource`, `Merge`,
   `Undecided`, `ParseFailure`, `Repo.FilesUnder`, `Repo.HasDir`,
   `Config.Requires`, `Rule.Applies`, `RuleResult.Binding` and
   `Category.Intro`. They are the vocabulary of the check functions.
   The package documentation shows them to `cli`, `doctor` and
   `contracttest` as if those callers had to know them.

## Solution

- The registry has at most one **last part**. The runner runs it after
  every other execution and probe part and gives it all their results.
  R-01's execution part is the last part. Every other part has a check
  function that takes only `*Env`. Needs, the graph sort and the cycle
  checks are deleted.
- What spec 0064 got right stays: a part returns all of its output in
  its `PartResult`, no result goes through `Env` or the probe session,
  and the runner keeps no list of rule IDs.
- Every exported identifier of `internal/contract` that no code outside
  the package uses becomes unexported. Identifiers that become dead
  code are deleted.

## Constraints and Assumptions

Constraints:

- **No behaviour change for adopters.** For the same repository, every
  rule's result, findings and fixes stay the same, in `--json` and in
  text, and the progress lines come in the same order. The CLI's report
  goldens and the contract's golden tests do not change.
- **Each part starts from a reset copy** (spec 0053, "Order"). The
  probe copy, setup, the precondition, `--static`, waivers and
  not-applicable rules keep their behaviour.
- **Dependency direction** (`docs/architecture/overview.md`): the change
  stays in `internal/contract` and its tests, plus the stand-in code in
  `contracttest`. It adds no package and no import. The `depguard`
  rules do not change.
- **Test seam** (spec 0064, spec 0065, ADR 0028). Runner tests go
  through `contract.Audit` with `Options.Rules`, in package
  `contract_test`. No test calls a check function directly.
- **No production dependency.**

Assumptions, each checked during implementation:

1. R-01's execution part is the only part that reads another part's
   result. The registry declares needs on that part only, and spec
   0064 checked the same assumption.
2. The run order does not change. R-01's execution part already runs
   last, because it needs every other part. The other parts already run
   in registry order, because they declare no needs.
3. R-01 sees the same results. It needs every other part, so it already
   sees every earlier result and setup's `vibe` calls.
4. Only Go code in this module imports `internal/contract`, so
   unexporting an identifier can break only code that the build shows.

## User Stories

1. As a maintainer who adds a rule, I want a check function to take
   only `*Env`, so that I do not have to learn a needs mechanism that my
   check does not use.
2. As a maintainer who reads the runner, I want the order to be "static
   parts, then the other parts in registry order, then the last part",
   so that I can see the order without solving a graph.
3. As a maintainer who changes R-01, I want R-01's check to get every
   earlier result as an argument, so that its inputs stay explicit, as
   spec 0064 made them.
4. As a maintainer who changes A-02, I want A-02 to keep returning its
   deny verdict and its exception uses in its `PartResult`, so that no
   other rule reads A-02's side effects.
5. As a maintainer, I want the audit to panic before it builds the
   probe copy when a rule list has two last parts, or when a static
   part is the last part, so that a wrong rule list fails at once in a
   test and never looks like a problem in the adopter's repository.
6. As a maintainer, I want a part to set exactly one check function,
   the normal one or the last one, and the audit to panic when a part
   sets both or neither, so that a part cannot be ambiguous.
7. As a maintainer, I want a registry test to require that R-01's
   execution part is the only last part, so that a change to the
   registry cannot move R-01 out of the last place without a test
   failure.
8. As a maintainer, I want the last part to see no result of a part
   that did not run (its rule is not applicable or waived, or the
   precondition failed), so that R-01 decides on its own in those
   cases, as it does now.
9. As a maintainer, I want `vibe audit --static` to validate the rule
   list and run no execution or probe part, as it does now.
10. As a maintainer who writes a runner test, I want to give a stand-in
    rule list to `contract.Audit` through `Options.Rules` and see the
    order and the results through the report, so that my test goes
    through the package interface.
11. As a maintainer who writes a runner test, I want to look up an
    earlier result by rule ID and part kind, so that a stand-in last
    part can show what it saw.
12. As a maintainer who uses `contracttest`'s stand-ins, I want them to
    keep working with the new check function type, so that the rule
    tests need no change apart from the type.
13. As a maintainer of `cli` or `doctor`, I want the documentation of
    `internal/contract` to list only what I can use, so that I learn the
    package from its real interface.
14. As a maintainer of a check, I want the CI model, the workflow types
    and the command helpers to stay available inside the package, so
    that the checks do not change apart from the names.
15. As a maintainer, I want code that becomes unused after the
    unexport to be deleted, so that the `unused` linter stays clean and
    no dead code stays behind.
16. As a reader of `docs/architecture/overview.md` and `GLOSSARY.md`, I
    want them to describe the last part and not needs, so that the
    documentation matches the code.
17. As an adopter, I want every rule's result, findings, fixes and the
    progress order to stay the same, so that this refactor has no
    effect on my audit.
18. As a maintainer, I want `task audit` to stay conformant and the CI
    check `Contract / audit` to stay green, so that this repository
    still follows its own contract.

## Implementation Decisions

### The last part

- The check function type takes only the environment. Its shape:

  ```go
  type CheckFunc func(env *Env) PartResult
  type LastFunc  func(env *Env, earlier Earlier) PartResult

  type Part struct {
      Kind    PartKind
      Check   string
      Run     CheckFunc // every part but the last part
      RunLast LastFunc  // the last part only
  }
  ```

- A part sets exactly one of `Run` and `RunLast`. A part with `RunLast`
  is the **last part**. It is an execution or probe part. A rule list
  has at most one last part.
- `Earlier` replaces `Needed`. It holds every execution and probe part
  that ran before the last part, in run order, each with its
  `PartResult` and the `vibe` calls the shim recorded while it ran, plus
  the `vibe` calls of setup. Its exported method is
  `Result(id string, kind PartKind) (PartResult, bool)`. The helpers
  that R-01 uses (A-02's deny verdict, every `vibe` call) stay
  unexported. A part that did not run is absent.
- The runner decides the static parts first, as now. Then, in the probe
  copy, it runs the execution and probe parts that have `Run`, in
  registry order and in part order inside a rule. Then it runs the last
  part, when the last part is pending. The precondition and the reset
  before each part stay.
- The runner validates the rule list before it builds the probe copy,
  also with `--static`. It panics with a message that names the part
  when two parts are last parts, when a static part is the last part,
  or when a part sets both or neither of `Run` and `RunLast`. In
  `vibe audit` this cannot happen, because the registry test keeps
  `contract.Rules()` valid.
- R-01's execution part sets `RunLast`. Its check reads A-02's deny
  verdict and the `vibe` calls from `Earlier`. When A-02's probe did not
  run, R-01 reports the same "cannot decide" finding as now.
- The report's guard exception uses still come from the stored results
  in run order: A-02's uses come before A-09's.
- Deleted: `Need`, `NeedPart`, `NeedEveryOther`, `Part.Needs`,
  `Needed`, the graph sort, the cycle text and the four needs panics.
  The registry helper that builds an execution part with needs takes no
  needs.

### The smaller interface

- The rule: an exported identifier of `internal/contract` (a type,
  function, method, field or constant) becomes unexported when no code
  outside the package refers to it. "Outside" means another package of
  this module, `contracttest`, `probetest` and the tests in package
  `contract_test`. A type that only such identifiers expose becomes
  unexported with them.
- Exceptions: methods that satisfy an interface of the standard library
  or of a dependency (`Error`, `Unwrap`, `String`, `Write`) stay as they
  are. Methods on types that are already unexported (the probe session,
  the run outcome) stay as they are, because they are not on the
  package interface.
- The method: unexport, then build and run the tests. What a caller
  needs stays exported. The pull request lists the identifiers that
  became unexported and the code that was deleted because it became
  unused.
- An unexported name that clashes with an existing name gets a
  different, clear name. For example, `Env.CI` cannot become `Env.ci`,
  because `Env` has a field `ci`.
- `revive`'s `exported` rule and the `unused` linter stay on. Code that
  `unused` reports after the unexport is deleted.

### Documentation

- `docs/architecture/overview.md`: "Parts declare their needs" becomes
  a paragraph on the last part. The CI model paragraph uses the new,
  unexported names, or describes the model without names where a name
  adds nothing.
- `GLOSSARY.md`: the entry **Need** is replaced by **Last part**: the
  one execution or probe part that runs after every other execution and
  probe part and gets their results. _Avoid_: need, final step.
- Spec 0064 gets a status note: its needs are superseded by spec 0071,
  and its return-value results stay.

## Testing Decisions

- A good test here goes through `contract.Audit` with `Options.Rules`
  and asserts on the report: part results, findings and the order in
  which stand-in parts ran. It does not call the runner's helpers or a
  check function.
- Runner tests in package `contract_test` (prior art: the current
  runner tests and spec 0064's tests):
  - parts with `Run` run in registry order, and in part order inside a
    rule;
  - the last part runs after every other execution and probe part, even
    when its rule comes first in the list, and sees all their results
    and setup's `vibe` calls;
  - a part of a not-applicable or waived rule is absent from `Earlier`;
  - the audit panics before the probe copy on two last parts, a static
    last part, and a part that sets both or neither of `Run` and
    `RunLast`;
  - `--static` validates the list and runs no execution or probe part.
- Most of the seven runner tests stay and change to the last part. The
  tests of named needs, cycles and "a part sees only its declared
  needs" are deleted, because that behaviour is deleted.
- The registry test that checks R-01's needs is replaced by one that
  requires exactly one last part in `contract.Rules()`, R-01's
  execution part.
- `contracttest`'s stand-in parts and their tests change to the new
  check function type only.
- The behaviour proof: the CLI's report goldens, the contract's golden
  tests (`ci_golden_test`, `explain_golden_test`), the rule tests,
  `probetest` and the guard probe tests pass without a change to any
  expected value.

## Out of Scope

- Simplifying the hooks and `vibe-guard`. The user named them as a
  later candidate; they get their own review and spec.
- The CI wording (`ci_wording.go`). Its GitHub and GitLab texts differ
  in substance, so it is a real seam with two adapters.
- Running parts concurrently.
- Renaming exported identifiers that callers use, or changing any
  report text.
- Unexporting methods of types that are already unexported.

## Acceptance Criteria

- [ ] `contract.CheckFunc` takes only `*Env`. No check function takes a
      `Needed` or `Earlier` argument except R-01's execution part.
- [ ] `Need`, `NeedPart`, `NeedEveryOther`, `Part.Needs` and `Needed`
      do not exist. The runner has no graph sort and no cycle check.
- [ ] R-01's execution part is the only part with `RunLast`, and a
      registry test requires that.
- [ ] Runner tests through `contract.Audit` cover: registry order, the
      last part runs last and sees every earlier result and setup's
      calls, absent results of parts that did not run, the panics, and
      `--static`.
- [ ] No exported identifier of `internal/contract` lacks a caller
      outside the package, apart from the exceptions above. The pull
      request lists what became unexported and what was deleted.
- [ ] No golden file and no expected value in an existing rule test,
      CLI test or probe test changes.
- [ ] `docs/architecture/overview.md`, `GLOSSARY.md` and spec 0064's
      status describe the last part.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.

## Further Notes

- Measured at 8d66ee5: the needs code is about 220 lines, plus eight
  tests. 33 of the 34 check functions have the parameter `_ Needed`.
- Spec 0064's problem (an order kept in a string list, and results
  passed through shared fields) stays solved: the last part is declared
  on the `Part`, not by rule ID in the runner, and results still come
  back through return values.
