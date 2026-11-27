# Spec 0074: Contract 2.1.0: the guard rules become SHOULD, and C-05 follows `platforms`

Status: draft (2026-11-27). Issue #250. A MINOR contract release under
spec 0051, "Versioning". It amends ADR 0025 (decisions 9 to 11) through
a new ADR.

## Problem Statement

Adopters report three things that keep them out of conformance or make
conformance expensive.

1. **C-05 assumes Linux and Windows.** C-05 ("tests run on Linux and
   Windows") checks for a Linux runner and a Windows runner whatever the
   repository declares in `platforms`. A self-hosted GitLab adopter
   would need a Windows runner that runs Docker. The rule is already
   SHOULD and not applicable without `windows` in `platforms`, but the
   check does not follow the declaration in the other direction:
   - `platforms: [windows]` still asks for a Linux runner.
   - `platforms: [linux, macos]` makes C-05 not applicable, so the
     declared macOS support is never tested in CI.
2. **A-05 seems to put a container build at the end of every agent
   turn.** An adopter whose tests need Docker and Postgres reads A-05
   ("the agent's turn ends with `verify-fast`") together with V-02
   (`verify-fast` catches the planted defects) as "every Claude turn
   runs the database tests". The contract does not require that: the
   planted test has no dependencies, and V-02 already says to leave
   slow and network checks out of `verify-fast`. But nothing tells the
   adopter how to split the tests, and one common pytest layout (an
   autouse fixture in `conftest.py` that starts the database) makes
   even the planted test start the database.
3. **The guard rules make one binary a MUST.** A-01 to A-04 and A-08
   are MUST rules. In practice only `vibe-guard` passes the guard
   corpus, so every conformant Claude Code repository runs one
   maintainer's binary before every Bash, PowerShell, Write and Edit
   call, and lets it lock `.claude/settings.json`. Some adopters will
   not run an outside binary in that position. Today their only way out
   is `agents: []`, which also drops A-05 to A-07, the hook rules they
   do want. ADR 0025 forbids waiving a MUST rule, so a waiver is not a
   way out either.

## Solution

Contract 2.1.0, a MINOR release:

- **The guard rules become SHOULD.** A-01, A-02, A-03, A-04 and A-08
  move from MUST to SHOULD under contract 2.1.0. They form the *guard
  set*. A repository that wants a guarded agent lists the guard set in
  `require`, and the verdict then counts those rules exactly as before.
  The core MUST rules stay MUST: V-01 to V-04, C-01 to C-04, A-05, D-01
  and R-01. "Conformant" still always means that the agent's work is
  verified, at the end of each turn and in CI, with the repository's
  own tools.
- **Guarded is the default.** The setup skill's reference `vibe.yaml`
  files and `vibe init` write `require: [A-01, A-02, A-03, A-04, A-08]`.
  An adopter that does not want the guard deletes that line, which
  makes the choice visible in its own file. This repository keeps the
  guard set in `require`.
- **The guard set is required together or not at all.** `require`
  that lists some but not all of the guard set is a configuration
  error. A guard that is hooked up but not tested proves nothing, and a
  tested guard that is not hooked up does nothing. `require` with A-09
  needs the whole guard set too.
- **No guard means the rules that test it are not applicable.** When
  the guard set is not required and `.claude/settings.json` has no
  PreToolUse hook for `Bash`, the rules that test a guard (A-02, A-03,
  A-04, A-08 and A-09) report "not applicable" instead of failing. A-01
  still reports its result, so the report shows that the agent is not
  guarded.
- **C-05 follows `platforms`.** Under contract 2.1.0, C-05 checks that
  the steps that call `verify` run on a runner for each platform in
  `platforms`, and it always applies. `platforms: [linux]` is satisfied
  by one Linux job. The rule stays SHOULD and waivable. Its title
  becomes "tests run on every declared platform".
- **Guidance for tests that need services.** `rules.md` (under V-02)
  and `migrate.md` gain a section on tests that need Docker or a
  database: keep them out of `verify-fast` and in `verify` and CI, with
  the pytest-marker and Go build-tag patterns, and with the warning
  about an autouse fixture in the folder of the first test file.
- **Contract 1.x audits do not change.** A repository that declares
  1.x is still audited as 1.2.0, with the guard rules MUST and the old
  C-05. A repository that declares 2.0.x is audited as 2.1.0, as spec
  0051 says for an older MINOR of the same MAJOR.

## User Stories

1. As an adopter whose product runs only on Linux, I want to declare
   `platforms: [linux]` and have C-05 pass with my Linux CI job, so
   that I do not need a Windows runner I have no use for.
2. As an adopter on self-hosted GitLab without a Windows runner that
   runs Docker, I want C-05 to ask only for the platforms I declare, so
   that my CI setup does not decide my audit result.
3. As an adopter whose product runs only on Windows, I want C-05 to
   ask for a Windows runner and not a Linux one, so that the rule
   matches what I ship.
4. As an adopter that declares `macos`, I want C-05 to check for a
   macOS runner, so that my declared macOS support is tested in CI and
   not silently skipped.
5. As an adopter that declares a platform it cannot test in CI yet, I
   want to waive C-05 with a reason and an expiry, so that I can still
   reach a verdict while I set the runner up.
6. As an adopter reading a C-05 failure, I want the message to name
   each declared platform that has no runner, so that I know which job
   to add.
7. As an adopter whose CI uses runners of unknown platform, I want C-05
   to list those runners as it does today, so that I can tag or label
   them.
8. As an adopter on contract 1.x, I want C-05 to behave exactly as
   before, so that my verdict does not change until I move to 2.x.
9. As an adopter whose tests need Docker and Postgres, I want the
   contract's reference to tell me how to keep those tests out of
   `verify-fast`, so that an agent turn does not end with a container
   build.
10. As an adopter using pytest, I want an example of a marker that
    excludes the database tests from `verify-fast`, so that I can copy
    it.
11. As an adopter using Go, I want an example of a build tag that keeps
    integration tests out of `go test ./...`, so that `verify-fast`
    runs only the unit tests.
12. As an adopter with an autouse database fixture in `conftest.py`, I
    want to be warned that the planted test will start the database
    too, so that I move the database tests into their own folder or
    marker.
13. As an adopter, I want to know that the Stop hook already skips
    `verify-fast` when nothing changed since the last passing run, so
    that I understand the real cost of A-05.
14. As an adopter who will not run an outside binary before every tool
    call, I want to leave the guard rules out of `require` and still be
    conformant, so that I keep A-05 to A-07 without `vibe-guard`.
15. As an adopter who wants a guarded agent, I want the setup skill to
    put the guard set in `require` by default, so that I am guarded
    unless I choose otherwise.
16. As an adopter who drops the guard, I want that choice to show in my
    `vibe.yaml`, so that a reviewer sees it.
17. As an adopter without a guard, I want A-02, A-03, A-04, A-08 and
    A-09 to report "not applicable" rather than fail, so that the
    report shows one fact (no guard) and not five failures.
18. As an adopter without a guard, I want A-01 to still report its
    result, so that the report states that the agent is not guarded.
19. As an adopter who lists only part of the guard set in `require`, I
    want a configuration error that names the missing rules, so that I
    do not believe I am guarded when I am not.
20. As an adopter who requires A-09 without the guard set, I want a
    configuration error, so that a skip-hooks check does not stand in
    for a guard.
21. As an adopter who requires the guard set, I want the verdict to
    count A-01 to A-04 and A-08 exactly as under 2.0.0, so that
    requiring them gives me the same guarantee as before.
22. As an adopter who requires the guard set but has no guard hook, I
    want A-01 to fail and the other guard rules to fail too, so that a
    required guard cannot pass by being absent.
23. As an adopter declaring `contract: 2.0.0`, I want to be audited as
    2.1.0 with no change to my verdict when my guard is `vibe-guard`,
    so that I do not have to edit anything.
24. As an adopter, I want `vibe audit --explain A-01` to print the
    rule's level for the contract I am audited against, so that I am
    not told it is MUST when it is SHOULD for me.
25. As an adopter, I want the text and JSON reports to mark the guard
    rules `(required)` when I require them, as they do for other
    required SHOULD rules, so that the report reads the same way for
    every rule.
26. As an adopter who wants to suspend a guard rule for a while, I want
    to waive it like any SHOULD rule, so that the guard rules follow
    the same mechanism as the others.
27. As an adopter, I want `rules.md` to say that a person can always
    edit `.claude/settings.json` in their own editor and that `vibe
    unlock` only lets the agent edit it, so that I understand what A-08
    locks.
28. As a maintainer of this repository, I want this repository's
    `vibe.yaml` to require the guard set, so that our own audit still
    holds us to the guard.
29. As a maintainer, I want ADR 0025's decisions on the guard (9 to 11)
    amended by a new ADR, so that the record says the guard is the
    adopter's choice and why.
30. As a maintainer, I want `vibe --version`, `vibe init` and the setup
    skill to name contract 2.1.0, so that new repositories declare the
    current contract.
31. As a maintainer, I want the contract's changes log in spec 0051 to
    list 2.1.0 with each change and why it is MINOR, so that adopters
    can see what changed.
32. As a setup-skill user, I want the skill to ask the person whether
    they want the guard before installing `vibe-guard`, so that the
    binary is never installed without the person's consent.
33. As a setup-skill user whose person declines the guard, I want the
    skill to skip the `vibe-guard` install, the guard hooks and the
    guard policy, and to remove the guard set from `require`, so that
    the repository has no half-installed guard.

## Implementation Decisions

- **A rule's level depends on the audited contract.** The rule table
  keeps one entry per rule. A rule whose level changed records the
  contract version that changed it, in the way `Since` records the
  version that added a rule. The engine, the report, `--explain`,
  waiver validation and `require` validation all read the level for
  the audited contract. Under 1.2.0 the guard set is MUST; under 2.1.0
  it is SHOULD.
- **Contract versions.** The 2.x contract this vibe implements becomes
  2.1.0, and it is the latest. A declared 2.0.x and 2.1.x are audited
  as 2.1.0; a declared 1.x as 1.2.0. The behaviours that were gated on
  "audits contract 2" (the narrowed exception reading, the exception
  re-run, `vibe` by path) stay on for 2.1.0.
- **The guard set** is a named, ordered list in the contract package:
  A-01, A-02, A-03, A-04, A-08. Configuration validation uses it:
  under 2.1.0, `require` lists all of the guard set or none of it, and
  `require` with A-09 needs the whole guard set. The error names the
  missing IDs. Under 1.x the guard set is MUST, so `require` already
  rejects it.
- **Applicability of the guard-testing rules.** Under 2.1.0, A-02,
  A-03, A-04, A-08 and A-09 are not applicable when both of these
  hold: the configuration does not require the guard set, and
  `.claude/settings.json` has no PreToolUse hook whose matcher covers
  `Bash` (the same reading A-01 uses). The reason names A-01. They keep
  their current `agents`-includes-`claude` condition too. A-01 has no
  new condition: it reports pass or fail. A-05 to A-07 are unchanged.
- **C-05 under 2.1.0.** It checks each command of `verify` for a runner
  of each platform in `platforms`, in the order `platforms` lists them,
  through the same CI model and runner readers as today (GitHub
  `runs-on` labels and matrices, GitLab `tags:` and `image:`). It has
  no applicability condition beyond the existing one that a CI the
  audit reads exists. Its title and check text change; the finding
  names the missing platforms and the runners of unknown platform as
  today. Under 1.2.0 it keeps the fixed Linux and Windows check and the
  `windows` applicability.
- **Why 2.1.0 is MINOR.** Lowering a MUST rule to SHOULD is a relaxed
  rule. C-05 is SHOULD, so its new check changes no verdict unless the
  repository requires C-05. One case can change: a repository that
  requires C-05, declares `macos` and no `windows`, and has no macOS
  runner. C-05 was not applicable there and now fails. We accept this
  as MINOR because only an explicit `require` of a rule that was not
  applicable reaches it, and the changes log names it.
- **Setup skill.** The reference `vibe.yaml` files name
  `contract: 2.1.0` and carry the guard set in `require`. `SKILL.md`
  asks the person, before step 5 installs `vibe-guard`, whether they
  want the agent guard; on "no" it skips the guard install, the guard
  PreToolUse hooks, `policy.json` and the `vibe-guard relock` part of
  the Stop command, and removes the guard set from `require`. Its rule
  that the skill does not add `require` on its own changes to except
  the guard set. `rules.md` marks the guard rules SHOULD, explains the
  guard set and its all-or-none rule, states what A-08 does and does
  not lock, rewrites C-05, and gains the "tests that need services"
  paragraph under V-02. `migrate.md` gains the same paragraph.
- **`vibe init`** writes the guard set in `require` of the first
  `vibe.yaml`, with a comment that removing it drops the guard rules.
- **This repository.** `vibe.yaml` moves to `contract: 2.1.0` and adds
  the guard set to `require`.
- **Documentation.** Spec 0051 gets the 2.1.0 entry in "Changes", its
  "Current version" line, the new levels in its rule table and the new
  C-05 text. `docs/usage.md` covers the guard set, the new `require`
  error and C-05. `GLOSSARY.md` gains "guard set". A new ADR (0029)
  records that the guard is the adopter's choice and amends ADR 0025's
  decisions 9 to 11: they hold whenever the repository requires the
  guard set.

## Testing Decisions

- **Seams.** Rule behaviour is tested through `contract.Audit` (ADR
  0028), with fixture repositories, asserting each rule's result,
  reason and findings. Configuration errors are tested through the
  configuration loader. The report wording is tested through the
  existing report golden files in the CLI. No new seam is added.
- **C-05:** for 2.1.0, cases for `[linux]`, `[windows]`,
  `[linux, windows]` and `[linux, macos]`, on GitHub Actions and on
  GitLab CI, each passing with the right runners and failing with the
  missing platform named. For 1.2.0, the existing cases stay and pass
  unchanged. Prior art: the C-05 cases in the CI and GitLab rule tests.
- **Guard set levels:** under 2.1.0, a repository with no guard and no
  `require` is conformant when the core rules pass; A-01 fails as a
  SHOULD rule and A-02, A-03, A-04, A-08 and A-09 are not applicable.
  The same repository with the guard set in `require` is not
  conformant. A repository with `vibe-guard` and the guard set
  required keeps the 2.0.0 result. Under 1.2.0, a repository with no
  guard is not conformant. Prior art: the agent rule tests and the
  probe fixtures.
- **Configuration:** `require` with part of the guard set, and with
  A-09 alone, is an error naming the missing IDs; the whole set is
  accepted under 2.x and rejected under 1.x; a waiver of a guard rule
  is accepted under 2.x and rejected under 1.x. Prior art: the
  `require` and waiver cases in the configuration tests.
- **Version mapping:** declared 2.0.0 and 2.1.x are audited as 2.1.0;
  2.2.0 is "upgrade vibe". Prior art: the version tests.
- **Report and `--explain`:** golden files show the guard rules as
  SHOULD under 2.1.0, `(required)` when required, and the new C-05
  title. `--explain A-01` prints SHOULD for a 2.x configuration and
  MUST for a 1.x one.
- **Setup skill drift:** the existing skill drift and static-audit
  tests cover the reference `vibe.yaml` files, so they must pass with
  `contract: 2.1.0` and the guard set in `require`.
- **Our own audit:** `task audit` says `Verdict: conformant` with the
  guard set required.
- A good test asserts what the audit reports for a repository, not
  which function decided it.

## Out of Scope

- Letting an adopter lower any other MUST rule. The core stays MUST.
- Codex in the A rules (#50).
- Accepting Claude Code's built-in `permissions.deny` as a guard. A
  repository's own guard can still satisfy the guard set, as today.
- Lowering the trust a guarded repository puts in `vibe-guard` (a
  pinned, checksum-checked install in the setup skill, and a test that
  the guard makes no network calls). That is a separate change.
- Changing A-05 or V-02. Only their guidance changes.

## Further Notes

- `require` already exists (spec 0063); this spec only widens which
  rules it can name under 2.1.0 and adds the guard-set check.
- The audit reads the guard hook in `.claude/settings.json` for
  applicability, which is repository content, as C-03's GitLab
  applicability already is.
