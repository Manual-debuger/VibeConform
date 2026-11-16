# Spec 0065: The contract loads its own configuration, and rule tests go through it

Status: accepted and implemented (2026-11-16, PR #191). Issues #162
and #165, from the design review #167. ADR 0028 records the test-seam
decision and amends spec 0052's "Testing Decisions". No contract
change: the contract stays 1.2.0, and no rule's result, finding or fix
changes.

## Problem

Knowledge that belongs to `internal/contract` and `internal/doctor`
lives in `internal/cli`. The tests make it worse, because they hold the
CLI as the only way in.

1. **Configuration loading is CLI code, written three times.** The
   steps are: read the file, tell version 1 from version 2, open the
   repository, parse. `vibe audit` does them, `vibe doctor` does them
   again, and `vibe init` repeats the version 1 detection. That a
   missing `vibe.yaml` is different from a version 1 file is policy,
   and it lives in the CLI.
2. **`vibe doctor`'s orchestration is in the CLI.** The CLI chooses the
   checks, their order and the failure count. Its test seam is a
   mutable package variable, so the doctor tests cannot run in
   parallel. `internal/doctor` copies the contract's private order of
   the entrypoints.
3. **Rule IDs leak into the renderer.** `--explain` adds the guard
   corpus counts when the rule ID is A-02 or A-03, although
   `contract.RuleDetails` exists for per-rule extras.
4. **A hand-kept flag list.** `--explain` conflicts with every other
   audit flag, but the CLI checks a fixed list. A new flag is silently
   allowed with `--explain`.
5. **The JSON report drops a field.** The text report prints "waiver
   unused", and the JSON report has no way to say it.
6. **Rule tests go through cobra and JSON** (issue #165). About 4,850
   lines of rule tests live in `internal/cli`. The CLI carries the rule
   list, a report hook, the probe environment and the repository opener
   only for them, and the hook checks a contract invariant. The JSON
   report is mirrored three times in test code.

## Constraints

- **No behaviour change for adopters**, except two: the new JSON key
  (Desired Behavior, "The JSON report says when a waiver is unused")
  and the order of the flags in one `--explain` error (Desired
  Behavior, "`--explain` refuses every other flag"). Every other
  output, error message and exit code stays the same byte for byte.
  The contract stays 1.2.0 (spec 0051, "Versioning"), and the JSON
  report stays schema 1.
- **Dependency direction** (`docs/architecture/overview.md`). The CLI
  imports `internal/contract` and `internal/doctor`. `internal/doctor`
  imports only `internal/contract` among this module's packages (the
  `depguard` rule from spec 0063). `internal/contract` imports neither.
  The migration pointer names the setup skill's path, so it stays in
  the CLI, and `internal/doctor` never sees it.
- **No production dependency.**
- **No test is weakened.** Every case that the CLI suite checks today
  is checked after the move, through the new seam or through the CLI's
  thin suite. The move does not drop cases.
- **ADR 0028** governs where tests go. No test calls a check function
  directly.

## Assumptions

Each of these is checked during implementation:

1. No CLI-level behaviour needs a stand-in rule list or a narrowed probe
   `PATH`. The golden reports run with the registry's rules. If a CLI
   case does need a stand-in, it keeps that seam, and the
   implementation notes say why.
2. The `vibe audit` command has no persistent flags from the root
   command other than cobra's own help flag. So "every flag the user
   set, except `--explain`" is the same set that the hand-kept list
   holds today, plus any flag added later.
3. Only one `--explain` conflict test asserts the text for more than
   one flag. The order of the flags in that error changes (Desired
   Behavior), and no adopter parses it.
4. The fixtures under the CLI's test data, except the report goldens,
   are used only by rule tests. They can move to `internal/contract`
   without a copy staying behind.
5. `vibe init` uses only the version detection from the steps in item
   1. It does not need to parse the configuration that it refuses to
   overwrite.

## Desired Behavior

### The contract loads its configuration

- `internal/contract` has one way to go from a repository root and an
  optional configuration path to a parsed configuration and its
  repository: `contract.Load`. It reads the file, detects the schema,
  opens the repository and parses.
- Its errors say what happened, and callers tell them apart without
  reading text:
  - **No configuration**: no path was given and `vibe.yaml` is missing
    at the root (`ErrNoConfig`). A missing file named by `--config` is
    a read error, as it is today.
  - **Version 1**: the file has a top-level `standard` key
    (`ErrConfigV1`). The error names the file.
  - **Invalid**: a `*ConfigError` with the file and the field path, as
    `contract.Parse` returns today.
  - Any other error (an unreadable file, a directory that is not a
    repository) is returned wrapped, as today.
- The read-and-detect step is available on its own for `vibe init`. It
  is the one place where version 1 is told from version 2.
- `vibe audit` and `vibe doctor` call `contract.Load`. The CLI turns
  `ErrNoConfig` into "run `vibe init`" and `ErrConfigV1` into the
  migration pointer, with today's text.
- `vibe init` decides "no configuration", "version 1" and "some other
  file exists" from the same step, with its own messages, unchanged.

### Doctor runs its own checks

- `internal/doctor` has `doctor.Run`. It takes the machine environment,
  the repository root and the outcome of loading the configuration,
  and it returns the report. It chooses the checks, their order and
  what each one needs. The CLI loads the configuration, maps a load
  error to its text, calls `doctor.Run`, prints the report and turns a
  failure count into exit 1.
- The output of `vibe doctor` is unchanged: the same lines in the same
  order, and the same exit codes.
- The order in which doctor checks the entrypoints comes from
  `internal/contract`, which exports it once. Doctor keeps no copy.
- The CLI takes doctor's environment through its seams struct, not
  through a package variable.

### `--explain` holds no rule IDs

- The guard corpus counts for A-02 and A-03 come from
  `contract.RuleDetails`, like every other per-rule extra. The renderer
  prints what `RuleDetails` returns and names no rule.
- `vibe audit --explain A-02` and `--explain A-03` print the same text
  as today.

### `--explain` refuses every other flag

- `--explain` with any other `vibe audit` flag that the user set is an
  error, whether or not the flag exists today. The message is today's
  message. When more than one flag is set, they are listed in the
  order cobra visits them (sorted by name), not in a hand-kept order.

### The JSON report says when a waiver is unused

- The `waiver` object of a rule in `--json` has a boolean key `unused`.
  It is true exactly when the text report prints "waiver unused". It is
  a new key, so the schema stays 1.

### Rule tests go through the contract (ADR 0028)

- The rule tests run `contract.Load` and `contract.Audit` in package
  `contract_test` and assert on `*contract.Report`.
- Every report that a rule test produces is checked: every failed part
  has a finding, and every finding names a file and a fix. There is no
  list of tests to keep.
- The CLI suite tests flags, exit codes, the CLI's error text and the
  golden text and JSON reports. It mirrors the JSON report once.
- The CLI's seams keep only what its own suite needs: the repository
  opener and doctor's environment. The rule list, the report hook and
  the probe environment leave it (Assumption 1).
- The doctor tests run `doctor.Run` in package `doctor_test` with a
  stub environment, in parallel. The CLI keeps one doctor case for
  printing and the exit code.

## User Stories

1. As a maintainer adding a rule, I want to write its test cases
   against `contract.Audit`, so that the test reads the report the rule
   produced and not a JSON rendering of it.
2. As a maintainer adding a rule, I want every failing case of my rule
   checked for a file and a fix without listing it anywhere, so that a
   finding without a fix cannot slip through.
3. As a maintainer changing the JSON report, I want only the CLI suite
   and its goldens to change, so that a schema change does not ripple
   into dozens of rule tests.
4. As a maintainer, I want rule tests to stop paying for cobra and a
   JSON round trip, so that the suite runs faster.
5. As a maintainer reading `internal/cli`, I want its seams to exist
   for the CLI's own tests only, so that contract internals do not leak
   into the command layer.
6. As a maintainer, I want one function that loads the configuration,
   so that `audit` and `doctor` cannot drift apart in how they read
   `vibe.yaml`.
7. As a maintainer, I want the version 1 detection in one place, so
   that `audit`, `doctor` and `init` refuse a version 1 file for the
   same reason.
8. As a caller of `contract.Load`, I want to tell "no configuration"
   and "version 1" apart by error value, so that I can choose my own
   text without parsing messages.
9. As an adopter running `vibe audit` without a `vibe.yaml`, I want the
   same "run `vibe init`" message as before, so that nothing I rely on
   changes.
10. As an adopter with a version 1 `vibe.yaml`, I want the same
    migration pointer from `audit`, `doctor` and `init` as before.
11. As an adopter with an invalid `vibe.yaml`, I want the same error,
    naming the file and the field path, and exit 1, as before.
12. As an adopter running `vibe doctor`, I want the same lines in the
    same order and the same exit code as before.
13. As a maintainer, I want doctor's choice and order of checks in
    `internal/doctor`, so that I can test it without cobra.
14. As a maintainer, I want the doctor tests to run in parallel, so
    that no test changes a package variable that another test reads.
15. As a maintainer, I want doctor to read the entrypoint order from
    the contract, so that a new entrypoint reaches doctor without a
    second edit.
16. As a maintainer adding a per-rule extra to `--explain`, I want to
    add it in `contract.RuleDetails` only, so that the renderer never
    names a rule.
17. As an adopter running `vibe audit --explain A-02`, I want the same
    guard corpus counts as before.
18. As a maintainer adding a flag to `vibe audit`, I want `--explain`
    to refuse it with no list to update, so that a new flag cannot be
    silently ignored.
19. As an adopter combining `--explain` with another flag, I want the
    same error as before, naming the flags I set.
20. As an adopter reading `--json`, I want to see that a waiver is
    unused, so that the JSON report says what the text report says.
21. As an adopter with a JSON consumer, I want the schema number to stay
    1, so that my consumer keeps working when a key is added.
22. As a maintainer, I want the fixture builder in one package that both
    suites import, so that the fixtures do not exist twice.
23. As a maintainer reading the tests, I want ADR 0028 to say why rule
    tests left the CLI, so that nobody moves them back by following
    spec 0052.
24. As a maintainer of the setup skill, I want the static audit of the
    reference files to read the report model, so that it does not keep
    its own copy of the JSON report.
25. As a reviewer, I want every test case of the CLI suite to have a
    place after the move, so that I can see that nothing was dropped.

## Implementation Decisions

### `contract.Load`

- New in `internal/contract`: `Load(ctx, root, configPath, open)`
  returns `(*Config, *Repo, error)`. `open` is the repository opener,
  and nil means `OpenRepo`. A test passes the fixture opener, so a
  listed fixture needs no git (spec 0056).
- A read-and-detect step is exported for `vibe init`. It returns the
  file's label and content for a version 2 file, and otherwise
  `ErrNoConfig`, `ErrConfigV1` or the error. `Load` is that step, then
  `open`, then `Parse`.
- `ErrNoConfig` and `ErrConfigV1` are sentinel errors, wrapped so that
  `errors.Is` finds them. The text of the `ErrConfigV1` error names the
  file and says that this `vibe` no longer reads version 1. The CLI
  appends the migration pointer. The text that users see is unchanged.
- `DetectSchema` stays exported. `Load` and the read step are its only
  callers outside its own tests.

### CLI

- `vibe audit`: `readV2Config` and the inline open-and-parse are
  replaced by `contract.Load`. One function in the CLI maps the two
  sentinels to their text, and `audit` and `doctor` share it.
- `vibe init`: `refuseExisting` uses the read-and-detect step. Its
  messages do not change.
- `--explain`: the conflict check visits the flags that the user set
  and refuses all of them except `--explain`.
- The renderer's A-02 and A-03 branch is removed.
- The seams struct holds the repository opener and doctor's
  environment, and nothing else (Assumption 1). The doctor environment
  package variable is removed.

### Doctor

- New in `internal/doctor`: `Run(ctx, env, root, loaded)` returns a
  `Report`. `loaded` carries either the configuration and the
  repository, or the load error with the text the CLI chose. `Run`
  orders the checks as `runDoctor` does today: git, `vibe.yaml`, the
  entrypoints, then the hooks. The hook checks move with it.
- `internal/contract` exports the entrypoint order (the required
  entrypoints, then `setup`) as a function that returns a copy. V-04
  and doctor both read it. Doctor's own copy is removed.

### Report and explain

- `RuleDetails` for A-02 and A-03 includes the guard corpus counts, in
  today's text. `GuardCorpusCounts` stays exported if a test still
  needs it. Otherwise it becomes unexported.
- The JSON waiver gains `unused`, next to `expired`.

### Test move (ADR 0028)

- A helper package under `internal/contract` (`contracttest`) holds the
  fixture builder (copy a test-data tree, apply edits, `git init` or a
  listed file set), the listed-fixture opener, an audit helper that
  runs `contract.Load` and `contract.Audit`, and the report check. It
  finds its test data from its own source location, so that both
  suites can import it.
- The fixture trees (baseline and the probe fixtures) move to
  `internal/contract/testdata`. The report goldens stay with the CLI.
- These CLI test files move to `internal/contract` as package
  `contract_test`, and assert on the report model: the rules tests
  (core, agents, CI, GitLab), components, engine (waivers and
  verdicts), findings (the report check), config validation, and the
  probe and guard-probe tests. Each file keeps its case names, so that
  the move can be reviewed file by file.
- The CLI keeps the dispatch, explain, golden, exit-code and report
  renderer tests. It keeps one case for each sentinel error's text, one
  invalid-configuration case (exit 1, file and field path on stderr),
  and the JSON mirror for the golden and JSON tests.
- The setup skill's static audit test stays in `internal/cli`, because
  it needs `internal/setupskill`. It runs `contract.Load` and
  `contract.Audit` through the helper and reads the report model, and
  its JSON mirror is removed.
- The doctor tests move to `internal/doctor` as package `doctor_test`,
  run `doctor.Run` with a stub `Env` and use `t.Parallel`.

### Documentation

- Spec 0052's "Testing Decisions" gets a note that ADR 0028 supersedes
  its first bullet.
- `docs/architecture/overview.md`: the "Configuration" bullet names
  `contract.Load`, and the `vibe doctor` bullet names `doctor.Run`.
- `docs/usage.md`: the JSON report's description lists `unused`.

## Testing Decisions

- **Good tests assert on external behaviour**: the report a rule
  produces, the error a load returns, the lines doctor reports, and
  the exit code and text of a command. A test never asserts on a check
  function, a cache or the order in which internal steps run.
- **Seams** (ADR 0028), confirmed with the user on 2026-11-16:
  1. `contract.Load` and `contract.Audit` in package `contract_test`,
     for rule behaviour, configuration errors and probes. Prior art:
     `runner_test.go` and `guardprobe_test.go` in `internal/contract`,
     and `internal/contract/probetest`.
  2. `doctor.Run` in package `doctor_test` with a stub `Env`. Prior
     art: the stub in the CLI's doctor tests today, and
     `internal/doctor`'s own tests of single checks.
  3. The `vibe` root command in-process, for flags, exit codes, the
     CLI's error text and the goldens. Prior art: the CLI suite today.
- **New cases**:
  - `contract.Load`: no `vibe.yaml` (`ErrNoConfig`); a missing
    `--config` file (a read error, not `ErrNoConfig`); a version 1
    file (`ErrConfigV1`, naming the file); both keys and neither key
    (`*ConfigError`); an invalid version 2 file (`*ConfigError` with
    the field path); a valid file (the configuration and the
    repository).
  - `vibe init` refuses a version 1 file and an existing version 2
    file with today's messages.
  - `--explain` with a flag added only in the test's command tree is
    refused, which proves that no list is kept.
  - `--explain A-02` and `--explain A-03` print the corpus counts. The
    golden or explain test already asserts this, and it keeps passing
    with the renderer branch removed.
  - The JSON report of a fixture with an unused waiver has
    `"unused": true`, and one with a used waiver has `"unused": false`.
  - `doctor.Run` with a load error reports the `vibe.yaml` line as FAIL
    with the given text, and skips the checks that need the
    configuration.
- **Regression guard for the move.** Before the move, record the names
  of the CLI's contract and doctor test cases. After the move, each
  name is found in the new location, or it is listed in the
  implementation notes with the reason it was merged or dropped.
- **Speed.** Rule cases run with `t.Parallel`, as they do today. The
  probe tests keep their `-short` skip.

## Out of Scope

- Issue #161 (language records). It builds on `contract.Load`, but it
  is the next spec.
- Issue #160 (C rules through the CI model).
- Issue #163 and the guard items of #166. They are the guard track.
- Any rule's behaviour, any new rule, and any change to the contract
  version.
- A JSON schema version 2.
- Moving the setup skill's drift test out of `internal/cli`.
- #166's items that do not touch the files changed here.

## Acceptance Criteria

- [x] `contract.Load` exists, and `vibe audit` and `vibe doctor` use
      it. The CLI has no read, detect, open and parse sequence of its
      own.
- [x] `ErrNoConfig` and `ErrConfigV1` are found with `errors.Is`, and
      `contract_test` has a case for each `Load` outcome in "Testing
      Decisions".
- [x] `vibe init`'s version detection uses the contract's
      read-and-detect step, and its messages are unchanged.
- [x] Every message from `audit`, `doctor` and `init` for a missing,
      version 1 or invalid `vibe.yaml` is byte for byte the same as
      before. The CLI suite asserts this.
- [x] `doctor.Run` exists. The CLI's doctor command only loads, calls
      it, prints and sets the exit code. The doctor environment package
      variable is gone.
- [x] `internal/doctor` holds no copy of the entrypoint order.
- [x] The CLI's explain renderer names no rule ID, and
      `--explain A-02` and `--explain A-03` output is unchanged.
- [x] `--explain` refuses a flag that is not in any list, proven by a
      test.
- [x] `--json` has `waiver.unused`, and `schema` is still 1.
- [x] Rule, engine, config-validation, findings, component and probe
      tests run in package `contract_test` against `*contract.Report`.
- [x] The report check (a file and a fix for every failure) runs on
      every report that a rule test produces, and a test proves it
      runs.
- [x] The CLI's seams struct holds only the repository opener and
      doctor's environment, or the implementation notes say why another
      seam stayed.
- [x] The JSON report is mirrored once in test code.
- [x] Each CLI contract and doctor test case before the move is found
      after it, or is listed in the implementation notes with a reason.
- [x] Doctor tests run in package `doctor_test` with `t.Parallel`.
- [x] ADR 0028 is committed, and spec 0052's "Testing Decisions" points
      to it.
- [x] `docs/architecture/overview.md` and `docs/usage.md` are updated.
- [x] `task verify` passes.
- [x] `task audit` says `Verdict: conformant`.

## Further Notes

- Spec 0064 kept its runner tests in `internal/contract` and noted that
  #165 would move the rest. This spec does that.
- The move is large but mechanical. `/to-tickets` should cut it by test
  file family, so that the moves can run in parallel once the helper
  package and `contract.Load` exist.
- Issue #161 needs `contract.Load`, so it waits for this spec.

## Implementation Notes

- **Baseline of the CLI's contract test cases** (Testing Decisions,
  "Regression guard for the move"). Before any test moved, the 955
  test cases (top-level tests and subtests) of `internal/cli`'s
  `contract_*_test.go`, `config_messages_test.go` and
  `skill_static_audit_test.go` were recorded on 071357f in
  [`0065-baseline-test-names.txt`](0065-baseline-test-names.txt), one
  per line with its file. The doctor test cases are recorded with the
  doctor move. After the move, each name is found in its new place, or
  this section lists it with the reason it was merged or dropped.
- **`contracttest`** (#184). The fixture builder, the listed-fixture
  opener (`contracttest.OpenRepo`), the audit helper
  (`contracttest.Audit`) and the report check (`contracttest.Check`,
  `contracttest.Problems`) live in `internal/contract/contracttest`,
  with the shared assertions and the baseline `vibe.yaml` builder
  (`ConfigWith`, `Entrypoints`, `AssertRule`, `AssertPart`,
  `RunRuleCases`). The CLI's fixture helpers, the report hook,
  `internal/contract/probetest`, the guard probe tests, the runner
  tests and the `contract.Load` tests build their fixtures through it,
  so the builder exists once. The probe and guard probe reports now
  pass the report check too, which they did not before.
- **Moved before the rule tests:** `TestFixturePathMapsDotClaude`
  (baseline: `contract_fixture_test.go`) is now in
  `internal/contract/contracttest/fixture_test.go`, beside the function
  it tests. Its assertions are unchanged.
- **`contract.Load`** (#181). `contract.ReadConfig` is the
  read-and-detect step, and `contract.Load` is `ReadConfig`, the opener
  (`contract.RepoOpener`; nil means `contract.OpenRepo`) and
  `contract.Parse`. The CLI's `configLoadError` adds the "run
  `vibe init`" text and the migration pointer for `audit` and `doctor`.
  One ordering is kept from before: with a `--probe-timeout` that is not
  positive, `vibe audit` runs `contract.ReadConfig` and then reports the
  flag. The file is read once on that path: `ReadConfig` reports a
  missing, unreadable, version 1, both-key or neither-key `vibe.yaml`,
  and otherwise the flag error follows. Nothing opens the repository or
  parses the file there, so a version 2 file that does not parse gets
  the flag error, as before.
  `TestAuditBadProbeTimeoutAndConfigErrorKeepTheirOrder`
  (`internal/cli/config_messages_test.go`) holds that order.
- **`--explain` and the JSON waiver** (#182). The guard corpus counts
  are in `contract.RuleDetails("A-02")` and `("A-03")`, and the
  functions that count them are unexported. `--explain` visits the set
  flags with `pflag`, so `go.mod` lists `github.com/spf13/pflag` as a
  direct requirement. It is the same version that cobra already
  required, so no dependency is new. The repository has no JSON golden
  file, so only the text goldens exist. The phrase "golden text and JSON
  reports" in Desired Behavior and ADR 0028 means the text goldens
  (`TestContractReportGolden`) and, for JSON, the CLI's JSON tests:
  `TestContractJSONReportFromTheModel`, which renders a report model
  and decodes it through the one JSON mirror, and the CLI tests that
  run `vibe audit --json` (`TestContractJSONRuleHasRequiredKey`,
  `TestListedFixtureIsAuditedWithoutGit`, the `--json` cases of the
  dispatch tests and of `TestContractAuditProbeFlagsReachTheAudit`).
- **`doctor.Run`** (#183). The `depguard` rule `doctor` covers
  `internal/doctor`'s test files too, and its allow list gained
  `internal/doctor` itself, so that the external `doctor_test` package
  can import the package under test. Non-test code still imports only
  `internal/contract`. Doctor test cases that were merged or split, all
  25 of them recorded before the move and found after it:
  - `TestDoctorConfigurationLikeAudit` (`no_file`, `version_1`) is in
    `doctor_test`: a load error is the `vibe.yaml` FAIL line with the
    given text, and the checks that need the configuration are skipped.
    It no longer runs `vibe audit` beside doctor. The CLI's
    `TestDoctorConfigMessagesAreUnchanged` and
    `TestAuditConfigMessagesAreUnchanged` assert the exact text of both.
  - `TestDoctorChecksTheWorkflow` is in both places: `doctor_test`
    asserts the full result list, and the CLI keeps it as the one case
    for printing and the exit code (exact stdout and exit 0; exit 1 with
    `doctor: 1 check failed`).
  - `TestDoctorReportsWhatIsMissing` (`task`, `bash`, `vibe-guard`) is
    in `doctor_test`; each case asserts the exact FAIL result and that
    it is the only failure. The exit 1 of each case is the CLI's "a
    check fails" case.
  - `TestDoctorAfterInit` is in `doctor_test` with the `vibe.yaml` that
    `vibe init` writes as literal text, because `internal/doctor`
    cannot import the CLI.
  - `TestDoctorEntrypointPrograms` gained a `setup` entrypoint, so that
    it checks the contract's order. `TestGit` and
    `TestEntrypointsAbsolutePOSIXPath` keep their cases and run through
    `doctor.Run`.
- **Case names after the move** (#185 to #189). All 955 baseline names
  are found by `go test -v ./internal/contract/... ./internal/cli/
  ./internal/doctor/` on the integration branch, none skipped, and so
  are the 25 doctor cases. No case was dropped. These kept their name
  but changed place or assertions:
  - Stayed in the CLI because they test rendering or flags:
    `TestContractJSONRuleHasRequiredKey` and `TestContractAuditTextReport`
    (now in `contract_report_test.go`),
    `TestContractAuditProbeTimeoutMustBePositive` and
    `TestContractAuditProbeFlagsOnV1PointAtTheGuide`.
  - The engine subtests "... exits 0/2/1" assert `rep.Verdict`,
    `rep.FailingReason()` and `rep.Reason`; the exit codes are the CLI's
    `TestVerdictErrorExitCode`. The schema, `vibe` version, JSON
    `unused` and "waiver unused" text asserts of
    `TestContractAuditReportsEveryRule` and the unused-waiver subtest
    are the CLI's `TestContractJSONReportFromTheModel`.
  - `TestFailingFixturesAreWatched` audits a listed static fixture and
    a Git full fixture through `contracttest.Audit`, each with a
    stand-in whose finding names no file, and asserts that the report
    check flags that stand-in once per report. `TestFailureProblems`
    calls `contracttest.Problems`.
  - The CLI's invalid-configuration case is
    `TestAuditConfigErrorsNameTheConfigFile` (exit 1, the file and
    `platforms[0]` on stderr).
  - `TestRuleA02Reporting`'s subtests "an exception's cases are listed
    in JSON and text" and "without exceptions guard_exception_uses is
    an empty list" assert the report model; the rendering is the CLI's
    `TestContractReportGuardExceptionUses`, with the same subtest
    names. `TestContractAuditKeepProbeCopy` and
    `TestContractAuditProgress` check `ProbeOptions`; the CLI wiring is
    `TestContractAuditProbeFlagsReachTheAudit`.
  - New CLI renderer test from the GitLab move:
    `TestContractTextReportCIProvider`.
  - contracttest gained `StandIn` (it clones `Parts`; the CLI's version
    wrote into the registry's slice), `RuleIDs` and `AgentsConfig` for
    the families that share them.
- **The CLI's seams** (#190, Assumption 1). `contractSeams` holds the
  repository opener, doctor's environment and the probe environment.
  The rule list and the report hook are gone: no CLI case needed a
  stand-in rule, and the CLI's reports are no longer watched, because
  the rule tests that the hook checked run `contracttest.Audit`. The
  probe environment stays, so Assumption 1 holds only for the rule
  list. The CLI's full audits (the `baseline`, `mixed` and `required`
  goldens, `TestContractAuditValidConfigReachesEngine` and
  `TestContractAuditProbeFlagsReachTheAudit`) print what the probe runs
  did not find, such as "`task` is not installed on this machine" and
  "no bash found". With this process's environment that output would
  depend on the machine. `t.Setenv` cannot narrow `PATH` instead: the
  CLI cases run in parallel, and the audit's own git calls need git on
  `PATH`.
- **One JSON mirror** (#190). `jsonReport` and `jsonRule` in
  `internal/cli/contract_fixture_test.go`, read through
  `decodeJSONReport` and `auditJSON`, are the only mirror of the JSON
  report in test code. The partial mirrors of `guard_exception_uses`
  (`contract_probe_test.go`) and of the `required` key
  (`contract_report_test.go`) were folded into it. #186 removed the
  listings mirror, and the setup skill's static audit test lost its
  own (next item).
- **The setup skill's static audit** (#190). It stays in
  `internal/cli`, builds the reference repository with
  `contracttest.Fixture`, runs `contracttest.Audit` with
  `contract.Options{Static: true}` and reads the report model, so its
  reports pass the report check as they did through the CLI's hook.
- **Code review fixes** (after #190). No test case was dropped or
  renamed, and no assertion was weakened:
  - The fixtures exist once (User Story 22). `contracttest.Dependabot`
    replaces the CLI's `dependabot` and D-02's `agentsDependabot`;
    `contracttest.TreeHash` replaces the CLI's `treeHash` and the probe
    tests' `probeTreeHash`. The `depguard` rule `doctor` allows
    `internal/contract/contracttest`, and the doctor tests build their
    fixtures from the contract baseline through it instead of their own
    builder and baseline `vibe.yaml`. The rule `contracttest-importers`
    still keeps it out of doctor's non-test files.
  - Package `contract_test` has one part-case runner,
    `contracttest.RunPartCases` (it replaces `ciRunStaticPartCases`,
    `glRunStaticPartCases` and `agentsRunPartCases`; the C and GitLab
    families now also get its check that a failing part fails the
    rule), one GitLab baseline (`glBase`, `gitlab`) and one pair of
    verify entrypoints (`ciListVerify`, `ciNoVerify`).
  - The CLI tests call contracttest directly. The forwarding helpers
    (`withFile`, `withoutFile`, `withUntracked`, `contractFixture`,
    `contractGitFixture`, `fixtureGit`, `baselineFixture`,
    `fixtureEdit`, `openFixtureRepo`) and the `section`/`configWith`
    copy of `contracttest.Section` are gone.
  - `contracttest.StandIn` returns a new slice and leaves its input as
    it was (`TestStandInLeavesItsInputUntouched`).
  - `contract.RepoOpener.Open` holds the nil-opener default.
    `contract.Load` and `vibe init` pass a possibly nil opener through
    it, so the CLI's `contractSeams.open` is gone.
  - `contracttest.MustRules` is inlined: its five callers in
    `engine_test.go` call `RuleIDs(contract.Must, ...)`.
  - `vibe init`'s `refuseExisting` keeps `errors.As(err,
    *fs.PathError)`. `ReadConfig` documents its errors as
    `ErrNoConfig`, `ErrConfigV1`, `*ConfigError` and a wrapped
    `*fs.PathError`, and init's message prints the `*fs.PathError`
    alone. A new sentinel would still need that error for the
    message, so it would add an export and simplify nothing.
  - `TestContractAuditProbeFlagsReachTheAudit` keeps two full audits on
    one fixture, in parallel. Progress lines are printed in text mode
    and not with `--json`, so one run cannot show both, and no other
    CLI test runs a full `--json` audit to share.
