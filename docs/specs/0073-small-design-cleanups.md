# Spec 0073: Small design cleanups from the 2026-11-15 review

Status: approved (2026-11-26). Issue #238. Implements #166, the last open item of the design
review #167. A refactor of `internal/guardrule`, `internal/contract`,
`internal/setupskill` and the tests. No contract change: the contract
versions, the guard corpus, every rule's result, finding and fix, and
every guard decision stay the same. Amends spec 0069 ("Repository rules
fail closed"): a rule with an `args` that does not compile can no longer
be built.

## Problem

The 2026-11-15 design review (#167) found ten small design faults. Spec
0069 fixed one of them: a repository command rule with an uncompilable
`args` now fails closed. The other nine are still in `main` at e280d94.
None of them is a bug that a person sees today. Each one makes a future
bug more likely, or makes a change cost more than it has to.

**`internal/guardrule`: a rule or exception is valid only by
convention.**

1. *It is parsed again on every match.* `Exception.MatchesCommand` and
   `Rule.MatchesCommand` parse the match string with a new parser for
   each simple command, rule and exception. `ParsePolicy` already parses
   it in `checkMatch` and discards the result.
2. *A glob is compiled again on every match.* `MatchGlob` builds and
   compiles a regular expression on each call.
3. *An invalid rule can exist.* `Rule` and `Exception` have exported
   fields. Code outside `ParsePolicy` can build a rule with an `args`
   that does not compile, or with both or neither of command and path.
   Spec 0069 made the first case fail closed at match time. The type
   still allows the state.
4. *State is held in strings.* `psWriter.kind` is a string. `Target`
   has three string fields, and the tool name decides which of `Command`
   and `Path` is set. `Rule` encodes "exactly one of command and path"
   as two strings, one of which must be empty.
5. *One file holds two specs.* `wrappers.go` (570 lines) holds wrapper
   unwrapping (spec 0053) and shell-write detection (spec 0058).

**`internal/contract`:**

6. *A rule's check text is written twice.* `Rule.Check` repeats each
   `Part.Check` word for word. `registry_test.go` keeps them in step, but
   the rule's text could be derived from its parts.
7. *Complexity baselines do not go down.* 64 `//nolint:… // baseline:
   pre-existing complexity` comments are in the code. The review counted
   53, so the number went up. Most are in `guardrule/powershell.go` (8),
   `contract/gitlab_rules.go` (4) and `contract/updatebots.go` (3).
   Nothing stops a new one.

**`internal/setupskill` and the tests:**

8. *A second substitution that is not documented.* `setupskill.Render`
   also changes `/releases/tag/latest` to `/releases`, a copy of what
   `guard.ReferencePreToolUse` does. The package says that
   `{{VIBE_VERSION}}` is "the only placeholder". `FS()` calls `fs.Sub`
   on each call. `Dirs` is an exported slice that any caller can change.
9. *Knowledge that tests keep for themselves.*
   - Three ways to find bash: `contract.ResolveBash`, `hookShell` in the
     setupskill tests and `findBash` in the guard tests. They find
     different programs on Windows.
   - The "run git in an isolated temporary repository" helper is copied
     into about eight test files. The copies isolate Git's configuration
     in different ways.
   - The skill drift checker scrapes. It reads every string literal of
     `internal/contract` and `internal/guardrule` as a file that the
     contract reads. It reads vibe-guard's command set from the text of
     its help. The places of the reference files (`referenceTarget`, and
     `monorepoComponents` in two forms) are in tests, not in the package
     that owns the reference files.

## Solution

- `internal/guardrule` parses and compiles each rule and exception once,
  when it is built. `Rule` and `Exception` have unexported fields and
  constructors, so an invalid one cannot exist. Kinds are typed values,
  not strings. Shell-write detection moves to a file of its own.
- `internal/contract` derives a rule's check text from its parts. A
  check in `task lint` keeps the count of complexity baselines from
  going up, and this spec lowers it.
- `internal/setupskill` names its second placeholder, builds its file
  system once and keeps `Dirs` unchangeable.
- The tests find bash through one function and run git through one
  test-only helper package. The drift checker reads data that the owning
  packages export: a placement manifest from `internal/setupskill`, the
  command table from `internal/guard` and the list of repository paths
  from `internal/contract`.

## User Stories

1. As a guard maintainer, I want a repository rule and an exception
   parsed once, when the policy is read, so that a refusal does not parse
   the same text again for every simple command.
2. As a guard maintainer, I want a path glob compiled once, so that a
   long command with many writes does not compile the same expression
   many times.
3. As a guard maintainer, I want a `Rule` with an invalid `args`, or with
   both or neither of command and path, to be impossible to build, so
   that no deny rule can fail open, not only the ones that `ParsePolicy`
   builds.
4. As a guard maintainer, I want an `Exception` whose match does not
   parse to be impossible to build, so that "lifts nothing" is decided
   when the exception is built and not in each match.
5. As a repository owner, I want every `policy.json` that vibe-guard
   accepts today to be accepted, and every one it refuses to be refused
   with the same message, so that the refactor changes nothing that I
   can see.
6. As a repository owner, I want every guard decision to stay the same,
   so that my repository's guard behaves as before.
7. As a guard maintainer, I want the kind of a PowerShell writer to be a
   typed value, so that a misspelled kind does not compile.
8. As a guard maintainer, I want a `Target` to say whether it is a
   command, a path or neither as a typed value, so that no caller has to
   know which tool sets which field.
9. As a guard maintainer, I want a `Rule` to say whether it is a command
   rule or a path rule as a typed value, so that "exactly one of" is a
   property of the type.
10. As a guard maintainer, I want wrapper unwrapping and shell-write
    detection in separate files, so that each file maps to one spec.
11. As a contract maintainer, I want to write a rule's check text once,
    in its parts, so that a rule and its parts cannot drift.
12. As a reader of `vibe audit --explain` and of the report, I want every
    rule's check text to be the same, word for word, so that the
    refactor changes no output.
13. As a contract maintainer, I want the registry test to keep each
    rule's check text equal to spec 0051's text, so that the derivation
    does not move the registry away from the spec.
14. As a maintainer, I want `task lint` to fail when a new complexity
    baseline is added, so that the number only goes down.
15. As a maintainer, I want the files that this spec touches to lose
    their complexity baselines where the refactor makes the function
    simpler, so that the ceiling goes down now.
16. As a setup-skill maintainer, I want the release-page link to be an
    explicit placeholder, so that every substitution that `Render` does
    is documented and visible in the skill's text.
17. As a person who runs `vibe init --skill`, I want the installed skill
    to be the same, byte for byte, so that the refactor changes nothing
    in my repository.
18. As a setup-skill maintainer, I want the release-page link computed
    by one function that the reference hook command also uses, so that
    the skill and the hook command cannot disagree.
19. As a maintainer, I want `setupskill.FS` to build its file system
    once, and the skill directories to be unchangeable by a caller, so
    that the package has no mutable global state.
20. As a test author, I want one function that finds the bash that
    Claude Code runs hooks with, so that the audit, `vibe doctor` and the
    tests use the same program on Windows.
21. As a test author, I want one test-only helper that runs git in an
    isolated temporary repository, so that each test gets the same
    isolation from the person's Git configuration.
22. As a test author, I want that helper kept out of every binary, so
    that test code does not become production code.
23. As a setup-skill maintainer, I want the place of each reference file
    in a repository to be data in `internal/setupskill`, so that the
    drift checker, the static-audit test and the hooks test read one
    manifest.
24. As a drift-checker maintainer, I want vibe-guard's command set from
    a table that `internal/guard` exports, so that a change to the help
    text does not change what the checker accepts.
25. As a drift-checker maintainer, I want the repository paths that the
    contract reads from a list that `internal/contract` exports, so that
    an error message or a URL is no longer counted as a file that the
    contract reads.
26. As a drift-checker maintainer, I want the checker to report the same
    problems for the skill as today, so that the change does not hide a
    drift.
27. As an auditor, I want `task audit` to say `Verdict: conformant` and
    the guard corpus to be unchanged, so that the refactor is not a
    contract change.

## Implementation Decisions

### Rules and exceptions are built valid (`internal/guardrule`)

- `Rule` and `Exception` get unexported fields and read-only accessors
  (ID, level, tools, reason, kind and pattern for a rule; rule ID, match
  and reason for an exception).
- Constructors build them and return an error for an invalid one. The
  errors are the texts that `ParsePolicy` gives today, without the
  `policy.json: <key>` prefix. `ParsePolicy` calls the constructors and
  adds the key.
  - A command rule: the command must be one simple command with no
    expansion, and `args`, when it is given, must compile.
  - A path rule: the glob is compiled. `args` is not allowed.
  - An exception: the rule ID must be a contract ID. For a command rule,
    the match must be one simple command with no expansion.
- A rule's kind is a typed value (command rule or path rule) with one
  pattern string. The two-string encoding goes away.
- At construction, a command match is parsed once in Bash and once in
  PowerShell. Each parsed form that succeeds is kept. A match is
  compared with a command in the command's shell, as today. A match that
  parses in only one shell does not match a command of the other shell,
  as today.
- At construction, a path glob is compiled once and kept. `MatchGlob`
  stays for callers that have no rule. It uses a small cache of compiled
  globs, because the guard calls it with the same few patterns.
- The fail-closed branch of `Rule.MatchesCommand` (spec 0069) goes away,
  because an uncompilable `args` cannot be built. Spec 0069's regression
  test changes to: the constructor refuses that rule. This is no weaker,
  because the state that the old test guarded cannot exist.
- The zero `Rule` and the zero `Exception` match nothing.
- The contract audit builds exceptions through the exception
  constructor. An exception that the constructor refuses lifts nothing,
  as today when its match does not parse. The audit's findings do not
  change.

### Typed state (`internal/guardrule`)

- `psWriter.kind` becomes an unexported typed value (rename, new, copy,
  move, and the kinds that the table uses today).
- `Target` gets a typed kind (none, command or path) and one text
  value, with accessors. `ParseTarget` keeps its signature and its
  errors. Constructors for a command target and a path target replace
  the struct literals in `internal/guard` and `internal/contract`.

### `writes.go`

- The shell-write detection (spec 0058) moves out of `wrappers.go` into
  `writes.go`: `writtenOperands`, the writer tables, `psWriter` and its
  helpers. `wrappers.go` keeps wrapper unwrapping (spec 0053). The move
  does not change code.

### Check text from parts (`internal/contract`)

- `Rule.Check` becomes a method. It joins the kinds of the parts
  ("Static", "Execution and probe", …), a colon and the parts' check
  texts, as spec 0051 writes them. Each part's text keeps its own
  capital letter and full stop where spec 0051 has them.
- `registry_test.go` keeps comparing each rule's check text with spec
  0051's row, and each spec 0061 rule's text with that spec. The test
  that each part's text is part of the rule's text goes away, because
  it is now true by construction.
- **Not verified:** each rule's text in spec 0051 is the kind prefix
  followed by its parts' texts. If a rule's text cannot be derived, the
  part texts are changed so that it can be. The spec text and the
  output do not change. If a rule cannot be derived without changing
  the output, the implementer stops and reports it. The spec is then
  amended before work continues.

### Complexity baselines

- A check run by `task lint` counts the `baseline: pre-existing
  complexity` comments in the Go files of this repository and fails when
  the count is higher than a ceiling recorded in the repository. The
  message tells the reader to simplify the function, not to add a
  baseline.
- The ceiling is the count after this spec. Each function that this
  spec changes loses its baseline when the change brings it under the
  linter's limit: `ParsePolicy` and `parseRule`, `psWriter.written`,
  `prefixMatch`, `referenceTarget` (which moves to `internal/setupskill`
  as data), and the registry test.
- A later change that removes a baseline lowers the ceiling in the same
  commit.

### `internal/setupskill`

- A second placeholder, `{{VIBE_RELEASE_PAGE}}`, replaces the text
  `https://github.com/Manual-debuger/VibeConform/releases/tag/{{VIBE_VERSION}}`
  in the skill (`SKILL.md` and `reference/agents/settings.json`).
- `internal/guard` exports one function that returns the release page
  for an install version: the tag page, or the releases page for
  `latest`. `ReferencePreToolUse` and `Render` both call it. The
  `/releases/tag/latest` rewrite goes away.
- The package documentation lists both placeholders. The drift checker
  fails when the skill holds a `{{…}}` that is not one of them.
- `FS()` computes the sub file system once.
- `Dirs` becomes a function that returns a new slice on each call.

### One way to find bash

- `ResolveBash` moves from `internal/contract` to `internal/guardrule`
  without a change in behavior. `internal/contract` calls it.
  `internal/doctor` calls it too, and the depguard rule for
  `internal/doctor` allows `internal/guardrule`.
- `hookShell` and `findBash` call it. Each test keeps its own skip
  policy (`VIBE_HOOKS_E2E` or `CI` for the setupskill tests,
  `VIBE_TEST_REQUIRE_TOOLS` for the guard and contract tests). A common
  skip policy is out of scope.

### One test-only git helper

- A new package, `internal/testkit`, holds the helpers for an isolated
  temporary Git repository: run git with the person's global and system
  configuration ignored, init a repository with one commit, and get the
  common Git directory. It is the union of what the copies do today.
- Each copy in a test file is replaced by a call to `internal/testkit`.
- depguard: only `_test.go` files import `internal/testkit`, and it
  imports no package of this module, in the same way as the rules for
  `contracttest` (ADR 0028).
- `docs/architecture/overview.md` adds the package and its rule.

### The drift checker reads exported data

- **Placement manifest.** `internal/setupskill` exports the stacks, the
  CI providers and a function that gives the place of a reference file
  in a repository of a stack on a provider, or "not used". It replaces
  `referenceTarget`, `gitlabReferences` and both `monorepoComponents`.
  The drift checker, `skill_static_audit_test.go` and
  `hooks_anywhere_test.go` read it. The component IDs and Taskfile names
  that the hooks test needs are in the same manifest.
- **vibe-guard's commands.** `internal/guard` exports its command table:
  each command name, its aliases and its flags, with whether each flag
  takes a value. The usage text is generated from the table. The drift
  checker reads the table and no longer parses help output.
- **The paths the contract reads.** `internal/contract` exports the list
  of repository paths and directories that its rules read. The code that
  reads a path uses the same named value, so that the list and the
  readers cannot drift. The guard's files already have named values in
  `internal/guardrule` (`PolicyFile`, `LocalPolicyFile`). The drift
  checker reads both lists. It no longer reads string literals.
- **Not verified:** the contract reads a set of paths that is small
  enough to list. If a rule builds paths from the configuration (for
  example a component's directory joined to a file name), the list holds
  the file name, and the drift checker matches a bare name as it does
  today.
- `internal/contract`'s exported interface gets this one list and loses
  `ResolveBash`. Spec 0071's intent of a small interface holds.

### Documentation

- `docs/architecture/overview.md`: `internal/testkit`, `writes.go`, the
  placement manifest, and `ResolveBash` in `internal/guardrule`.
- Spec 0069: "Repository rules fail closed" says that an uncompilable
  `args` cannot be built (this spec).
- `GLOSSARY.md`, only if a term changes. No term is expected to change.
- No ADR. `internal/testkit` follows ADR 0028's pattern for a test-only
  package, and no new dependency is added.

## Testing Decisions

- **A good test goes through the interface that callers use and checks
  what they see.** For this refactor, most of the proof is that every
  existing test passes unchanged, together with the golden files, the
  guard corpus and the drift checker. New tests cover only the new
  interfaces.
- **Seams, all existing:**
  1. `internal/guardrule`'s exported functions (`ParsePolicy`, the new
     constructors, `Matches`, `MatchesRefused`, `MatchGlob`,
     `ParseTarget`). Prior art: `match_test.go`, `policy_test.go`,
     `writes_test.go`.
  2. The guard through `guard.Main` with an `Env` (spec 0070) and the
     guard corpus (`TestCorpus`). They show that no decision changes.
  3. The contract through `contracttest` and the golden files of the
     report and `--explain`. They show that no rule text, result or
     finding changes.
  4. `setupskill.Render` and `Install`, and the drift checker. They show
     that the installed skill does not change.
- **New cases:**
  - Each invalid rule and exception that `ParsePolicy` refuses today is
    refused by the constructor with the same message without its key.
    The `ParsePolicy` messages stay the same, key included.
  - The zero `Rule` and the zero `Exception` match no command and no
    path.
  - A match that parses only as PowerShell matches a PowerShell command
    and does not match a Bash command (unchanged; made explicit).
  - A test counts the parses: a policy with N exceptions, checked
    against a command of M simple commands, parses each match once per
    shell, when the policy is read, and not again.
    Its seam is an unexported counter that only an `export_test.go`
    reads.
  - `Render` gives byte-identical output to a golden file of the
    rendered skill for a tagged version and for `latest`. The golden is
    recorded from `main` before the change.
  - The skill holds no `{{…}}` other than the two placeholders.
  - `Dirs` returns a new slice: changing the result does not change the
    next call.
  - The baseline check fails on a fixture with one comment more than
    the ceiling, and passes at the ceiling.
  - The placement manifest gives, for every stack and provider, the
    same places that `referenceTarget` gives on `main`. The table is
    recorded from `main` before the change.
  - The guard's command table gives the same commands and flags that
    the help parsing gives on `main`.
  - `ResolveBash` keeps its existing tests, moved with it.
- **Regression of what must not change:** every existing test passes
  unchanged except where this spec moves a test (`ResolveBash`, spec
  0069's fail-closed test). The guard corpus file, the golden files and
  the rendered skill are unchanged.

## Out of Scope

- Any change to what the guard refuses, to a contract rule or to the
  guard corpus.
- Removing the complexity baselines in files this spec does not touch.
  The ceiling makes sure that the count only goes down.
- A common skip policy for tests that need bash (`VIBE_HOOKS_E2E`,
  `CI`, `VIBE_TEST_REQUIRE_TOOLS`).
- Codex sessions, which have no hooks (ADR 0011).
- #50, #74 and #120 Part B, which stay blocked as #167 says.

## Acceptance Criteria

- [ ] `Rule` and `Exception` have no exported fields. Constructors
      refuse every invalid state, and `ParsePolicy` uses them with
      unchanged messages.
- [ ] A command match is parsed once per shell and a glob is compiled
      once per rule or exception. A test shows the count of parses.
- [ ] `Rule.MatchesCommand` has no compile-at-match branch. The zero
      `Rule` and the zero `Exception` match nothing.
- [ ] `psWriter`'s kind, a `Target`'s kind and a `Rule`'s kind are typed
      values.
- [ ] Shell-write detection is in `writes.go`, and wrapper unwrapping
      stays in `wrappers.go`.
- [ ] A contract rule's check text is derived from its parts, and the
      report and `--explain` golden files are unchanged.
- [ ] `task lint` fails when the count of complexity baselines is
      higher than the recorded ceiling. The ceiling is lower than 64.
- [ ] `{{VIBE_RELEASE_PAGE}}` is a documented placeholder computed by
      the same function as `ReferencePreToolUse`'s link. The rendered
      skill is byte-identical to `main`'s for a tag and for `latest`.
- [ ] `setupskill.FS` builds once, and `Dirs` cannot be changed by a
      caller.
- [ ] One `ResolveBash`, in `internal/guardrule`, is used by the audit,
      `vibe doctor` and every test that needs bash.
- [ ] Every "git in an isolated temporary repository" helper is in
      `internal/testkit`, and depguard keeps it out of non-test files.
- [ ] The drift checker reads the placement manifest, vibe-guard's
      command table and the contract's path list. It reads no string
      literals and parses no help text, and it reports nothing new for
      the skill.
- [ ] The guard corpus file is unchanged, and every existing test
      passes, except for the moved tests.
- [ ] `docs/architecture/overview.md` and spec 0069 describe the
      changes.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.
- [ ] #166 and #167 are closed by the PR.

## Further Notes

- #167 agreed that each #166 item goes "with the work that touches it".
  The guard and contract tracks are now complete, and no planned work
  touches these items, so they are collected in this spec.
- The items are independent. `/implement-spec` can give them out as
  parallel tickets: (a) guardrule rules, exceptions and typed state;
  (b) `writes.go`; (c) check text from parts; (d) the baseline ceiling;
  (e) setupskill; (f) `ResolveBash` and `internal/testkit`; (g) the
  drift checker's data. (b) is best done before (a), because both touch
  `psWriter`. (d) is best done last, so that its ceiling counts the
  baselines that the other tickets remove.
- The review's count of baselines (53) was out of date by 2026-11-26
  (64). The ceiling records the real count when this spec is
  implemented.
