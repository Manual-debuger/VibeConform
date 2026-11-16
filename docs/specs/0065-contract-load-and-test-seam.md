# Spec 0065: The contract loads its own configuration, and rule tests go through it

Status: accepted (2026-11-16). Issues #162 and #165, from the design review
#167. ADR 0028 records the test-seam decision and amends spec 0052's
"Testing Decisions". No contract change: the contract stays 1.2.0, and
no rule's result, finding or fix changes.

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

- [ ] `contract.Load` exists, and `vibe audit` and `vibe doctor` use
      it. The CLI has no read, detect, open and parse sequence of its
      own.
- [ ] `ErrNoConfig` and `ErrConfigV1` are found with `errors.Is`, and
      `contract_test` has a case for each `Load` outcome in "Testing
      Decisions".
- [ ] `vibe init`'s version detection uses the contract's
      read-and-detect step, and its messages are unchanged.
- [ ] Every message from `audit`, `doctor` and `init` for a missing,
      version 1 or invalid `vibe.yaml` is byte for byte the same as
      before. The CLI suite asserts this.
- [ ] `doctor.Run` exists. The CLI's doctor command only loads, calls
      it, prints and sets the exit code. The doctor environment package
      variable is gone.
- [ ] `internal/doctor` holds no copy of the entrypoint order.
- [ ] The CLI's explain renderer names no rule ID, and
      `--explain A-02` and `--explain A-03` output is unchanged.
- [ ] `--explain` refuses a flag that is not in any list, proven by a
      test.
- [ ] `--json` has `waiver.unused`, and `schema` is still 1.
- [ ] Rule, engine, config-validation, findings, component and probe
      tests run in package `contract_test` against `*contract.Report`.
- [ ] The report check (a file and a fix for every failure) runs on
      every report that a rule test produces, and a test proves it
      runs.
- [ ] The CLI's seams struct holds only the repository opener and
      doctor's environment, or the implementation notes say why another
      seam stayed.
- [ ] The JSON report is mirrored once in test code.
- [ ] Each CLI contract and doctor test case before the move is found
      after it, or is listed in the implementation notes with a reason.
- [ ] Doctor tests run in package `doctor_test` with `t.Parallel`.
- [ ] ADR 0028 is committed, and spec 0052's "Testing Decisions" points
      to it.
- [ ] `docs/architecture/overview.md` and `docs/usage.md` are updated.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.

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
