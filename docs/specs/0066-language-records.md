# Spec 0066: Each component language is one record, and the contract detects components

Status: accepted and implemented (2026-11-17, PR #200). Issue #161, from
the design review #167. Builds on spec 0065 (`contract.Load`, and the
rule-test seam of ADR 0028). This is a refactor. The contract stays 1.2.0, and no rule's
result, finding or fix changes.

## Problem

What `vibe` knows about a component language is in about ten places:

| Knowledge | Where it is today |
|---|---|
| manifest, dependency reader, lockfile names for a finding, test file patterns | the contract's language table |
| the check commands of each lockfile | D-01's lockfile list |
| the fix command of each lockfile | a `switch` on the file suffix |
| D-01's check text, which repeats each lockfile's commands | the rule registry, twice (rule and part) |
| planted defect samples | the defect sample table |
| complexity defect layout | the complexity layout table |
| Dependabot ecosystems and Renovate managers | two tables in the update-bot checks |
| manifest → language for the root | the configuration parser |
| the valid `language` values | the configuration parser, a separate list |
| the language names that `--explain` prints | the explain renderer |
| manifest → language for `vibe init` | `internal/cli`, in `vibe init` |

The language table's doc comment says that it holds the facts "in one
place", but it does not. To add a language or a lockfile, such as
`yarn.lock` or `bun.lock`, a developer must edit each of these places.
No test tells the developer about a place that they did not edit.

Two rules decide the language of a manifest. They differ on purpose:

- The audit reads a root `package.json` alone as TypeScript. When the
  root has more than one root manifest, the audit refuses to guess.
- `vibe init` also wants a `tsconfig.json` beside `package.json`. When a
  directory has more than one manifest, it takes the first in the order
  Go, Python, TypeScript.

Because the two rules are in two packages, `vibe init` parses an
implicit configuration and compares the result, only to learn whether
the audit would detect the same root component. That cross-check exists
only because two modules hold the same knowledge.

## Constraints

- **No behaviour change for adopters**, with one exception: the
  `vibe init --help` text names the manifests with the same phrase as
  the "found no component" error (Desired Behavior, "`vibe init` only
  renders"). Every other output, error message and exit code stays the
  same byte for byte. This includes:
  - the rule texts that `--explain` and the reports print,
  - the planted defect samples (spec 0053 quotes them),
  - the D-01 and update-bot findings and fixes,
  - the configuration errors,
  - `vibe init`'s output and the file it writes.
  The contract stays 1.2.0 (spec 0051, "Versioning"), and the JSON
  report stays schema 1.
- **Dependency direction** (`docs/architecture/overview.md`). The CLI
  imports `internal/contract`. `internal/contract` does not import the
  CLI. Detection moves down into the contract, and nothing moves up.
- **No production dependency.**
- **No test is weakened.** Each `vibe init` case that the CLI suite
  checks today is checked after the change, either through
  `contract.DetectComponents` or through the CLI.
- **ADR 0028** governs where tests go. No test calls a check function
  directly. The one internal test is the completeness test of the
  language records (Testing Decisions), and its prior art is the
  registry test.

## Assumptions

Each of these is checked during implementation:

1. D-01 checks every recognized lockfile in a component's directory,
   whatever the component's language is. For example, a Go component
   with a stray `package-lock.json` runs `npm ci --dry-run`. The
   records keep this behaviour: D-01 walks every language's lockfiles
   in registry order, which is today's order (`go.sum`, `uv.lock`,
   `poetry.lock`, `pnpm-lock.yaml`, `package-lock.json`). A
   characterization test pins this before the move.
2. Today's parse-and-compare in `vibe init` gives the same answer as
   this rule: "there is exactly one component, at the root, and the
   audit's root detection (exactly one root manifest) gives the same
   language". The `vibe init` cases that cover the components key keep
   passing.
3. After the change, the language constants, the manifest names and the
   lockfile names are only in the language records and in comments. A
   search of the non-test code proves this. V-05's Python filter is the
   one exception, and Out of Scope explains why.
4. The setup skill's reference files (per-language Taskfiles and
   configurations) are content that the skill writes. They are not
   knowledge that `vibe` uses, so they stay out of the records.

## Desired Behavior

### One record per language

Each component language has one record that holds everything `vibe`
knows about it:

- its identifier (`go`) and its display name (`Go`),
- its manifest, and the reader that tells whether the manifest declares
  dependencies,
- the files that `vibe init` also wants beside the manifest (for
  TypeScript, `tsconfig.json`),
- its lockfiles, each with its file name, its check commands and its
  fix command,
- its test file patterns,
- its planted defect samples and its complexity layout,
- its Dependabot ecosystems and its Renovate managers.

The records are in one ordered registry, in the contract's language
order: Go, Python, TypeScript. Each list that depends on the language
order is derived from that registry:

- the valid `language` values and their error text,
- the root manifests and the "no root manifest" and "ambiguous root"
  errors,
- D-01's lockfile order,
- the order in which `--explain` prints the samples.

### Text is derived from the records

- D-01's check text, for the rule and for its part, is built from the
  lockfile records. A lockfile with one command reads "For `X`:
  `cmd`." A lockfile with more than one reads "For `X`: run `a`, then
  `b`." The result is byte for byte today's text.
- D-01's "no recognized lockfile (…)" finding names the lockfiles of
  the component's language, joined with "or" as today.
- D-01's stale-lockfile fix names the fix command of the lockfile that
  failed. The suffix `switch` is gone.

### The contract detects components

`contract.DetectComponents` takes the repository's tracked files and
returns what `vibe init` detects:

- the components, sorted by path;
- the nested manifests, each with the component that holds it;
- whether the audit detects exactly these components when `vibe.yaml`
  has no `components` key.

The depth limit (three directories), the dependency directories that
detection skips (`node_modules`, `vendor`), the priority between
manifests, and the rule that components do not nest all move into the
contract with it. The audit's own root detection and the "audit detects
the same" answer use one function, so they cannot drift apart.

### `vibe init` only renders

`vibe init` does these steps in this order:

1. It refuses an existing configuration.
2. It opens the repository.
3. It calls `contract.DetectComponents`.
4. It renders the file, without a `components` key when the detection
   says that the audit detects the same components.
5. It parses what it would write, as spec 0059 requires, so that it
   never writes a file that `vibe audit` cannot read.
6. It writes the file and prints what it found.

It no longer parses an implicit configuration to compare the result.
The "found no component" error and the `--help` text get the manifest
phrase ("go.mod, pyproject.toml, or package.json beside a
tsconfig.json") from the contract. The error's bytes are unchanged. The
help text changes from "package.json with tsconfig.json" to that
phrase, which is the one allowed wording change.

## User Stories

1. As a VibeConform maintainer, I want to add a component language by
   writing one record, so that I do not have to search ten places.
2. As a VibeConform maintainer, I want to add a lockfile such as
   `yarn.lock` or `bun.lock` by adding one entry with its file, check
   and fix, so that the check, the fix and the rule text cannot
   disagree.
3. As a VibeConform maintainer, I want a test to fail when a language
   record has an empty fact, so that a half-added language never
   ships.
4. As a VibeConform maintainer, I want D-01's check text to be built
   from the lockfile records, so that `--explain D-01` always tells the
   truth about the commands that run.
5. As a VibeConform maintainer, I want component detection to live in
   `internal/contract`, so that `vibe init` and the audit read the same
   source when they decide what a component is.
6. As a VibeConform maintainer, I want `vibe init` to stop parsing an
   implicit configuration only to compare the result, so that its code
   says what it means.
7. As a VibeConform maintainer, I want to test detection (depth, skipped
   directories, nesting, priority) on a file list, so that these cases
   run without a fixture repository or cobra.
8. As a VibeConform maintainer, I want the valid `language` values, the
   root manifests and the explain names to come from the registry, so
   that a new language appears in all of them at once.
9. As an adopter, I want every rule result, finding, fix and sample to
   stay byte for byte the same, so that my CI and my scripts do not
   notice the refactor.
10. As an adopter who runs `vibe init`, I want the same components, the
    same output lines and the same `vibe.yaml` as before, so that the
    refactor changes nothing for me.
11. As an adopter who reads `vibe init --help`, I want it to name the
    manifests with the same words as the error, so that the two texts
    do not disagree.
12. As an adopter whose Go component holds a stray `package-lock.json`,
    I want D-01 to check it as it does today, so that the refactor does
    not quietly drop a check.
13. As an agent that works in this repository, I want one record per
    language, so that I find all of a language's facts in one read.
14. As a reviewer, I want a search of the non-test code to find the
    language constants and the manifest names only in the records, so
    that I can see that the knowledge is in one place.

## Implementation Decisions

### The language record

- The record type is the existing unexported language facts type,
  extended with the facts in Desired Behavior. Lockfiles become a list
  of entries, each with a file name, a list of check commands (argv
  lists) and a fix command. These replace three parallel structures:
  the lockfile display string, the lockfile check list and the fix
  `switch`.
- The registry is an ordered list, not a map, because order is part of
  the behaviour. A lookup by identifier goes through one helper. The
  exported language constants stay, because callers and the
  configuration use them.
- Each language's record is in a source file of its own, together with
  its defect samples and its dependency reader. To add a language, a
  developer adds one file and one registry entry.
- The defect samples keep their content byte for byte. Only their home
  changes. The secret sample is the same for every language, so each
  record refers to the one shared value.
- The update-bot checks read the ecosystems and managers from the
  record. Their findings and fixes are unchanged, including "add the
  first manager" in the Renovate fix.

### Derived lists and text

- The valid `language` values, the root manifests (with their error
  texts) and the explain sample order and names are computed from the
  registry. They are not separate lists.
- D-01's check text is computed once from the lockfile records and used
  by both the rule and its part. This removes one of the cases that
  #166's "Check text is written twice" names, and only for D-01.

### Detection

- `contract.DetectComponents(files []string)` returns a detection value
  with three fields: the components, the nested manifests, and whether
  the audit's implicit root detection matches. The nested-manifest type
  becomes exported, because `vibe init` prints it.
- Detection reads only the file list. It does not open the repository
  or read file contents, as today.
- The audit's implicit root detection in the configuration parser and
  the "audit detects the same" field call one function. That function
  returns the root language, or why the root has none or more than one.
- The contract exports the detection depth and the manifest phrase,
  which are built from the records. The CLI uses them in the error and
  in the help text.

### CLI

- `vibe init` keeps refusal, opening the repository, rendering, the
  safety parse, the exclusive create and its output. Detection, the
  depth, the skipped directories and the implicit-config cross-check
  leave the CLI.
- When the cross-check has gone, the init command's complexity
  suppression is removed if it is no longer needed. The moved detection
  gets no new complexity suppression (#166, "Complexity baselines").

### Documentation

- `docs/architecture/overview.md`: the contract owns the language
  records and component detection.
- `docs/usage.md`: the `vibe init` help text, if it is quoted there.
- `GLOSSARY.md`: "Component" mentions that detection belongs to the
  contract, if the entry describes detection.

## Testing Decisions

- **Good tests assert on external behaviour**: the report that a rule
  produces, the detection that `DetectComponents` returns, the `--explain`
  text, and the file and output of `vibe init`. A test never asserts on
  how a record is stored.
- **Seams**, confirmed with the user on 2026-11-16:
  1. `contract.Audit` and `contract.Load` in package `contract_test`
     (ADR 0028), for D-01, D-03, the V rules' samples, the configuration
     errors and the `--explain` text. Prior art: `TestRuleD01` in the
     contract's probe tests, the update-bot rule tests,
     `internal/contract/probetest`, and the explain and report goldens.
  2. `contract.DetectComponents` in package `contract_test`, for
     detection. This is a new exported function in the existing
     contract seam. It gets init's detection cases: depth, skipped
     directories, nesting, priority between manifests, `package.json`
     without `tsconfig.json`, and the "audit detects the same" answer.
     Prior art: the fixture lists in the CLI's `vibe init` tests.
  3. The `vibe` root command in-process, for `vibe init`: the file it
     writes, the `components` key choice, the output lines, the
     refusals and the race. Prior art: the CLI's `vibe init` tests
     today.
- **One internal test**: every language record fills every fact. It
  has a display name, a manifest, a dependency reader, at least one
  lockfile with a check and a fix, test file patterns, a sample for
  each language-dependent defect kind, a complexity layout, and at
  least one Dependabot ecosystem and one Renovate manager. Manifest and
  lockfile names are unique across languages. Prior art: the registry
  test, which keeps the rule and part texts in step.
- **Characterization before the move**, added in the first ticket and
  passing on today's code:
  - D-01 with an empty probe `PATH`, one case per lockfile: the result
    is undecided and names the first program of that lockfile's check.
    This pins each lockfile's commands.
  - D-01 for a Go component with a stray `package-lock.json`: it runs
    the npm check (Assumption 1).
  - D-01's stale-lockfile fix for each lockfile names its fix command.
    A fake program on the probe `PATH` exits non-zero to force the
    failure.
  - `--explain D-01` and the explain samples of V-01 to V-08 match the
    goldens. If a golden does not cover a sample, add one.
  - `DetectComponents` cases equal to today's `vibe init` cases. They
    can only run after the function exists, so the first ticket records
    the init cases' expected components, and the detection tests reuse
    them.
- **Speed.** Detection tests are pure and run with `t.Parallel`. The
  probe tests keep their `-short` skip.

## Out of Scope

- V-05's Python filter. V-05 is a Python rule in the contract (spec
  0051). To derive it from "the language has a type-error sample" would
  let a new sample widen a rule's scope without a contract change.
- Adding a language or a lockfile (`yarn.lock`, `bun.lock`). This spec
  makes that a one-record change. The change itself is separate work,
  and it needs a contract version.
- Issue #160 (C rules through the CI model), which is next on the
  contract track.
- Issue #163 and the guard items of #166 (the guard track).
- The setup skill's per-language reference files, and the drift
  checker's scraping (#166, "setupskill and tests").
- #166's "Check text is written twice" for rules other than D-01.

## Acceptance Criteria

- [x] Each language has one record with all the facts in Desired
      Behavior, in an ordered registry.
- [x] The lockfile check list, the fix `switch`, the separate
      update-bot tables, the root-manifest list, the `languages` list
      and the explain names list are gone. Each is derived from the
      registry.
- [x] D-01's check text is built from the lockfile records, and
      `--explain D-01` is byte for byte unchanged.
- [x] `contract.DetectComponents` exists, and `contract_test` covers
      depth, skipped directories, nesting, priority, `package.json`
      without `tsconfig.json`, and the "audit detects the same" answer.
- [x] `vibe init` has no detection code and does not parse an implicit
      configuration. Its output and the file it writes are unchanged
      for every CLI case.
- [x] `vibe init --help` and the "found no component" error use the
      contract's manifest phrase, and the error is byte for byte
      unchanged.
- [x] The completeness test fails when any fact of any record is empty.
      The implementation notes show one red run of it.
- [x] The characterization tests in Testing Decisions pass before and
      after the move.
- [x] A search of the non-test code finds the language constants,
      manifest names and lockfile names only in the records, comments
      and V-05's filter. The implementation notes record the search.
- [x] The goldens, the explain tests and every existing rule test pass
      unchanged.
- [x] No new complexity suppression, and the ones that the moved code no
      longer needs are removed.
- [x] `docs/architecture/overview.md` is updated, and `docs/usage.md`
      and `GLOSSARY.md` where they describe detection.
- [x] `task verify` passes.
- [x] `task audit` says `Verdict: conformant`.

## Further Notes

- Order of #167's contract track: #164, #159 (spec 0064), #162 with
  #165 (spec 0065), this spec (#161), then #160.
- `/to-tickets` can cut this into these tickets:
  1. the characterization tests;
  2. the record and registry, with derived lists and text;
  3. `DetectComponents` and the `vibe init` change;
  4. documentation.
  Ticket 3 needs only the records' manifests, so it can start after
  ticket 2's record type exists.

## Implementation Notes

- **Tickets.** The work was cut into five tickets, not four: 0066-1 the
  characterization tests (#195), 0066-2 the lockfile entries (#196),
  0066-3 the record and registry (#197), 0066-4 `DetectComponents` and
  the `vibe init` change (#198), and 0066-5 documentation and the
  search (#199).
- **Init cases for detection** (Testing Decisions, "Characterization
  before the move"). The first ticket did not record the init cases'
  expected components. 0066-4 did it instead:
  `TestDetectComponentsInitLayouts` (`internal/contract/detect_test.go`)
  has the layouts of the CLI's `vibe init` cases before this spec,
  with the components `vibe init` wrote for each. The manifests that
  are not components (`node_modules`, `vendor`, below the depth,
  nested, and a `package.json` without `tsconfig.json`) moved out of
  the CLI's monorepo case into these contract cases. The CLI's monorepo
  case keeps its three components and still checks the file and the
  output end to end.
- **Explain goldens.** The goldens of 0066-1 are in
  `internal/contract/testdata/explain/`, one per rule. They hold the
  contract's text without wrapping, not the CLI's wrapped `--explain`
  output. V-07's golden writes the PEM dashes as `{{five dashes}}`, so
  that gitleaks does not flag the file. No golden of
  `internal/cli/testdata` changed.
- **Fake programs on the probe `PATH`.** The D-01 tests that put a fake
  program on the probe `PATH` (`fakeProgramPath` in
  `internal/contract/d01_lockfiles_test.go`) write a shell script, or a
  batch file on Windows. They ran on Windows only before the PR. CI is
  their first run on Linux.
- **Existing tests that changed.** No assertion was weakened:
  - `TestContractConfigComponents` ("language unknown") and
    `TestContractConfigComponentDetection` ("no manifest", "two
    manifests", "three manifests") assert the full error sentence
    instead of fragments of it.
  - `TestSecretSample` is an internal test. It read the old sample
    table, so it now reads each record's secret sample and compares it
    with the one shared value.
  - `TestInitNeedsAComponent` asserts the full error, byte for byte,
    and `TestInitHelpNamesTheManifests` is new.
- **Completeness test, red run.** With `renovateManagers` set to nil in
  the TypeScript record, `go test -run TestLanguageRecordsAreComplete
  ./internal/contract/` fails:

  ```text
  --- FAIL: TestLanguageRecordsAreComplete (0.00s)
      languages_test.go:63: language "typescript" has no Renovate manager
  FAIL
  FAIL	github.com/Manual-debuger/VibeConform/internal/contract	4.589s
  ```

  0066-5 repeated the run on the integration branch and got the same
  failure line. The record was restored after each run.
- **Complexity suppressions.** The `gocognit` and `gocyclo`
  suppressions of `runInitConfig` and the `gocognit` suppression of the
  CLI's `detectComponents` are gone with that code. `detect.go` and the
  record files have none.
- **The search** (Acceptance Criteria, Assumption 3). On the
  integration branch, from the repository root:

  ```sh
  git grep -nE 'LanguageGo|LanguagePython|LanguageTypeScript|"(go|python|typescript)"|go\.mod|go\.sum|pyproject\.toml|package\.json|tsconfig\.json|uv\.lock|poetry\.lock|pnpm-lock\.yaml|package-lock\.json' \
    -- '*.go' ':!*_test.go' ':!**/testdata/**'
  ```

  Every hit is in `internal/contract`. Nothing in `internal/cli`, the
  doctor, the guard or the setup skill's Go code names a language, a
  manifest or a lockfile. The hits are:
  - the three record files, `language_go.go`, `language_python.go` and
    `language_typescript.go`;
  - comments in `detect.go`, `languages.go`, `probe.go`, `toml.go` and
    `updatebots.go`;
  - V-05's filter, `c.Language == LanguagePython` in
    `checks_defects.go`;
  - the declarations of the exported constants `LanguageGo`,
    `LanguagePython` and `LanguageTypeScript` in `config.go`. The
    Implementation Decisions keep these constants, and the records use
    them as their identifiers. So the declarations are a fourth kind of
    allowed hit, which the acceptance criterion does not name;
  - `const pkg = "package.json"` in `updatebots.go`. This is not a fact
    about TypeScript. Renovate reads a `renovate` key in a root
    `package.json` in every repository, whatever the components'
    languages are, after the files in `renovateFiles`. It is a fact
    about Renovate, so it stays with the other Renovate files.

  A second search for the display names (`"Go"`, `"Python"`,
  `"TypeScript"`) finds only the three records.
- **Outside the search.** The dependency directories `node_modules`
  and `vendor` are in two lists: `detectionSkipped` (`detect.go`) and
  `underVendor` (`defects.go`, where the planted test file goes, spec
  0053). They are not language records, and this spec's search does
  not cover them. To use one list in both places is a possible
  follow-up.
- **Documentation.** `docs/architecture/overview.md` has a "Language
  records" and a "Component detection" item in "Contract audit", and
  its `vibe init` item says that the contract owns both. `docs/usage.md`
  does not quote `vibe init --help`. Its `vibe init` section now says
  that detection is the contract's, and when the root's component gets
  no `components:` key. The glossary's "Component" entry does not
  describe detection, so it did not change.
