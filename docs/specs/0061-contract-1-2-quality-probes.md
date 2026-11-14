# Spec 0061: Contract 1.2.0 probes that quality gates catch defects

Status: draft (2026-11-14). ADR 0027. Spec issue to follow. Adds five
SHOULD rules (contract 1.2.0, MINOR), one guard rule ID, and changes to
the setup skill's references, this repository and the examples.

## Problem Statement

The contract shows that `verify` runs a formatter, a static analyzer and
the tests (V-01). It does not show that a repository has the gates an
agent most often slips past: a type checker, a complexity limit, a
secret scanner, and lint that a blanket `# noqa` or `//nolint` cannot
switch off. An agent can also skip every Git hook with
`git commit --no-verify`, and the guard lets it through.

A person who adopts VibeConform to keep agent-written code in shape
gets no signal when one of these gates is missing. The metrics that
people usually propose (complexity ≤ 10, duplication ≤ 3 %, coverage
≥ 80 %) cannot go into the contract as they are. The contract names
properties, never tools or thresholds (ADR 0025), and the audit has no
base revision to ratchet against.

## Solution

Contract 1.2.0 adds four planted defects and one guard rule, all SHOULD
(ADR 0027):

- **V-05**: `verify` and `verify-fast` catch a type error.
- **V-06**: they catch a function whose cyclomatic and cognitive
  complexity are both at least 40.
- **V-07**: they catch a secret (a PEM private-key block) in the
  working tree.
- **V-08**: they still catch V-01's static-analysis defect when the
  language's common blanket suppressions cover it.
- **A-09**: the agent guard refuses commands that skip Git hooks.

The contract fixes the planted defect, never a threshold or a tool. Any
gate that catches the defect satisfies the rule. The setup skill's
references are updated so that a repository that follows them conforms.
This repository and the three examples conform to 1.2.0 without
waivers. Dependency direction becomes an optional reference, and this
repository enforces its own direction with lint instead of prose.

## User Stories

1. As an adopter, I want the audit to tell me when `verify` has no type checker, so that type errors reach review only when I chose that.
2. As an adopter, I want the audit to tell me when no complexity limit catches an extreme function, so that I know agents can write one unchecked.
3. As an adopter, I want to choose my own complexity thresholds, so that the contract does not dictate my lint rules.
4. As an adopter, I want any complexity metric (cyclomatic or cognitive) to satisfy V-06, so that I can keep the linter I already use.
5. As an adopter, I want the audit to tell me when `verify` does not scan for secrets, so that a committed key fails CI before merge.
6. As an adopter, I want the audit to tell me when a blanket suppression hides a lint finding, so that an agent cannot silence a gate with one comment.
7. As an adopter, I want specific suppressions to stay allowed, so that a function that is complex by nature can carry a named, reasoned exception instead of being split apart.
8. As an adopter, I want each new rule to be a SHOULD with its own ID, so that I can waive one kind, with a reason and an expiry date, and keep the others.
9. As an adopter on contract 1.1.0, I want my verdict to stay conformant after I upgrade `vibe`, so that the new rules inform me without breaking CI.
10. As an adopter, I want the report to name which entrypoint missed which defect, so that I know whether `verify`, `verify-fast` or both need the gate.
11. As an adopter, I want a planted defect to fail only because of its own kind, so that an unrelated lint finding cannot make a missing gate look present.
12. As an adopter with a component that has no tests, I want V-05 to V-08 to say "cannot decide" for it, so that V-01 alone reports the missing tests.
13. As an adopter, I want the guard to refuse `git commit --no-verify`, so that an agent cannot skip my pre-commit hook.
14. As an adopter, I want `git push -n` (a dry run) to stay allowed, so that the guard refuses only real hook bypasses.
15. As an adopter, I want to let one hook-skipping command through with a guard exception, so that I loosen the guard explicitly, as for every other guard rule.
16. As an adopter who writes my own guard, I want A-09 to use its own corpus cases, so that A-02 and A-03 do not change under me in a MINOR release.
17. As an adopter, I want the setup skill's references to satisfy every new rule, so that following them makes my repository conformant.
18. As an adopter migrating an existing repository, I want existing complexity violations to get specific suppressions with one fixed reason, so that new code meets the threshold and the old debt is easy to find.
19. As an adopter, I want the reference to require a written reason on every specific suppression where the linter can enforce it, so that each exception is explained in the diff.
20. As an adopter, I want an optional reference for dependency direction, so that I can enforce my own layers with a tool my ecosystem already has.
21. As an adopter on Windows, I want the secret scanner and the new linters to run on Windows and Linux alike, so that conformance does not depend on the platform.
22. As an adopter, I want my pre-commit hook to scan staged files for secrets, so that a key is caught before it leaves my machine, even though the contract does not check that.
23. As an agent working in an adopting repository, I want the Stop hook's `verify-fast` to report complexity, type, secret and suppression findings, so that I fix them in the same turn.
24. As an agent, I want a specific suppression with a reason to be accepted, so that I am not forced to split a cohesive function to pass a check.
25. As a maintainer of VibeConform, I want `task audit` on this repository to stay conformant under 1.2.0, so that the contract is dogfooded.
26. As a maintainer, I want the examples workflow to audit every example under 1.2.0, so that the references this repository cannot dogfood stay conformant.
27. As a maintainer, I want `vibe`'s source to hold no literal private key, so that secret scanners do not report this repository.
28. As a maintainer, I want the `internal/contract` ↔ `internal/guard` dependency direction enforced by lint, so that AGENTS.md points to a tool instead of restating a rule (principle 1).
29. As a maintainer, I want `vibe explain` to describe each new rule, so that an adopter can read what V-05 to V-08 and A-09 check without the spec.
30. As a reviewer, I want `docs/usage.md` to list what these rules cannot catch (a lowered threshold, a split function, a secret pushed before CI runs), so that the gaps are known.

## Implementation Decisions

### Contract version

- The contract version this `vibe` implements becomes 1.2.0. A
  configuration that declares 1.0.x or 1.1.x is audited as 1.2.0, as
  today for an older MINOR (spec 0052, "Contract versions"). The new
  rules are SHOULD, so its verdict does not change.

### V-05 to V-08: planted-defect probes

- Each rule has two probe parts: one for `verify`, one for
  `verify-fast`. Each part plants its sample in every component, one
  component at a time, in a reset probe copy, and requires the
  entrypoint to exit non-zero. The precondition is the same as V-02's:
  the entrypoint passes without a planted defect, or the part cannot
  decide. A component without tests leaves the part undecided (V-01
  already fails it).
- The finding names the entrypoint, the component and the defect kind.
- The samples join the planted-defect table: new defect kinds "type
  error", "complexity", "secret" and "blanket suppression". Like the
  existing samples, they are contract content: changing one is a
  contract change.
- **Every sample must be clean apart from its own defect** under the
  setup skill's reference configuration. A sample that trips an
  unrelated rule (unused code, a missing doc comment, too many return
  statements) would let a repository without the gate pass.
- **Go samples for V-05, V-06 and V-07 are non-test files.** The
  existing Go samples are `_test.go` files, and repositories commonly
  relax lint for test files. A Go non-test sample takes the package
  name of the test file beside it, without a `_test` suffix. V-08
  keeps V-01's test file, because it is V-01's sample with
  suppressions added.
- **V-05, type error**: Go, a variable of type `int` initialized with a
  string; Python, a function annotated `-> int` that returns a string;
  TypeScript, an exported `number` constant initialized with a string.
  In Go and TypeScript this overlaps with compilation. Only Python
  needs a separate type checker.
- **V-06, complexity**: one exported (Go: exported, with a doc comment)
  function of one integer parameter. It holds 40 consecutive,
  unnested `if` statements that each add a constant to a running total,
  and returns the total. Cyclomatic complexity 41, cognitive
  complexity 40, one return statement, no nesting, no duplicated
  branches. The contract says: a complexity limit of 40 or less, of
  either metric, catches it.
- **V-07, secret**: a plain text file holding a PEM private-key block:
  a `BEGIN PRIVATE KEY` line, at least 64 characters of base64 body,
  and the matching `END` line. The body is fixed, not random, so the
  sample is deterministic. `vibe` assembles the header and footer at
  run time, so no source file, test or document in this repository
  holds the literal block. The file name is neutral (not `.env`, not
  `*.pem`, not `*.key`), so that ignore files and the guard's
  secret-file rule do not treat it specially. The sample is written to
  the working tree and is not committed, so a scanner must scan files,
  not Git history.
- **V-08, blanket suppressions**: V-01's static-analysis sample with,
  on the defect's line or for its file:
  - Go: `//nolint` with no linter named.
  - Python: `# type: ignore  # noqa`, with no codes.
  - TypeScript: a file-level `eslint-disable` comment with no rule, and
    `// @ts-ignore` on the line before the defect.
  Specific suppressions are not part of any sample.

### A-09: the guard refuses commands that skip Git hooks

- New guard rule ID `git-skip-hooks`, a command rule (its exceptions
  match by word prefix, like `git-push-force`). The shared guard-rule
  package lists it as the first guard rule ID of contract 1.2.0. A
  guard exception may name it.
- The guard corpus marks the cases that belong to A-09. A-02, A-03 and
  R-01 read only the cases they read in 1.1.0, so they do not change.
  A-09 feeds its cases to every PreToolUse hook that matches the
  case's tool, as A-02 does, and honors a guard exception the same way.
- A-09's deny cases (Bash, and one PowerShell case):
  `git commit --no-verify`, `git commit -n`, `git commit -nm <msg>`,
  `git push --no-verify`, `git -c core.hooksPath=<path> commit`,
  `git -c core.hookspath=<path> push` (Git config keys ignore case).
- A-09's allow cases: `git commit -m <msg>`, `git commit -m "-n"`,
  `git commit -m "--no-verify notes"`, `git push -n` (a dry run),
  `git push --dry-run`, `git -c user.name=<x> commit`.
- `vibe-guard` reads short-flag clusters of `git commit` up to the
  first letter that takes a value (`m`, `F`, `c`, `C`, `t`), so `-nm x`
  is refused and `-mn` (message "n") is not.
- `vibe-guard` also refuses, under the same ID but outside the corpus,
  the hook-manager switches `LEFTHOOK=0` and `HUSKY=0` as an
  environment prefix of a `git commit` or `git push`. They are specific
  to one tool, so the contract does not require them.

### `vibe explain` and the report

- Each new rule has its explain text: what is planted, which
  entrypoints, and what satisfies the rule. The V-06 text states the
  threshold the sample implies (40). The V-08 text lists the
  suppressions per language. The A-09 text lists its cases.

### Setup skill references

- **Go lint**: enable `gocyclo` (limit 15), `gocognit` (limit 20) and
  `nolintlint` (a specific linter and an explanation required). Keep
  `go vet` as a separate command in the lint task.
- **Python lint**: add `C901` (limit 15), `PGH003` and `PGH004` to
  Ruff. Pyright stays in `standard` mode. Ruff cannot require a reason
  on a suppression, so the reference's comment asks for one.
- **TypeScript lint**: enable ESLint's `complexity` (limit 15),
  `sonarjs/cognitive-complexity` (limit 20), and the `eslint-comments`
  rules `no-unlimited-disable`, `require-description` and
  `no-unused-disable`. New dev dependencies: `eslint-plugin-sonarjs`
  and `@eslint-community/eslint-plugin-eslint-comments`.
- **Secret scanning**: each reference Taskfile gains a `secrets` task
  that runs gitleaks over the repository's files (not its history),
  with redaction. `verify` and `verify-fast` include it. The lefthook
  references run gitleaks on staged changes in pre-commit. A reference
  gitleaks configuration excludes dependency and build directories and
  the setup skill's own directories. The CI references install a
  pinned gitleaks release and check its checksum, on Linux and Windows
  runners.
- **Migration and upgrade**: the skill's migration and upgrade
  documents gain the 1.1 → 1.2 steps. Existing complexity violations
  get a specific suppression with the reason
  `baseline: pre-existing complexity`. Existing secret findings are
  rotated first. A finding that is not a secret (a test fixture) goes
  into the gitleaks allowlist with a reason.
- **The skill's rule reference** lists V-05 to V-08 and A-09 with what
  satisfies each.
- **Dependency direction**: a new optional reference explains how to
  declare layers and wire the check into `lint`: Go with `depguard`,
  Python with `import-linter`, TypeScript with `dependency-cruiser`.
  It is not a contract rule.
- The reference `vibe.yaml` declares contract 1.2.0.

### This repository

- `vibe.yaml` declares contract 1.2.0. Its `verify` list gains the
  `secrets` task, CI runs it, and `verify:fast` and pre-commit include
  it.
- `.golangci.yml` gains the Go reference's linters and `depguard`
  rules for the dependency direction in `docs/architecture/overview.md`
  (`internal/contract` and `internal/guard` both depend on
  `internal/guardrule` and never on each other). Existing complexity
  violations get the baseline suppression. AGENTS.md's dependency rule
  then points to the lint.
- A gitleaks configuration allowlists this repository's known
  non-secrets (guard test fixtures with secret-looking names), each
  with a reason.

### Examples

- The three examples follow the updated references and declare 1.2.0.
  `examples.yml` installs gitleaks and the new ESLint plugins.

## Testing Decisions

- A good test drives the audit or the guard from outside, as an
  adopter's CI would, and asserts the verdict and the findings. It does
  not assert which function computed them.
- **Seam 1: the contract audit on probetest fixtures, with real
  processes.** Prior art: the planted-defect tests (V-01, V-02, A-05
  probe part) in the contract's probe tests. The fixture's small
  check program gains a check per new kind, so that a fixture's
  `verify` or `verify-fast` can catch or miss each kind on purpose.
  Cases: each rule passes when both entrypoints catch the defect; fails
  and names the entrypoint when one misses it; is undecided when the
  baseline fails or a component has no tests. One test asserts that a
  configuration declaring 1.1.0 keeps a conformant verdict when only
  the new rules fail.
- **Sample tests**: the Go V-06 sample builds and passes `go vet`;
  the V-07 sample matches gitleaks's private-key pattern shape (header,
  body length, footer). No test file holds the literal PEM header.
- **Seam 2: the guard corpus.** Prior art: the guard's corpus test,
  which feeds `vibe-guard` every corpus case, and the contract's guard
  probe test. `vibe-guard` gives every A-09 case its expected result. A
  guard that refuses only the 1.1.0 corpus fails A-09 and still passes
  A-02 and A-03. An exception naming `git-skip-hooks` lets its one
  command through.
- **Seam 3: real toolchains.** `task audit` on this repository and
  `examples.yml` on the three examples reach "conformant" with every
  new rule passing.
- **Negative control** (in `examples.yml`): for each new sample and
  language, a job that switches off only the targeted gate (the
  complexity linters, the type checker, gitleaks, the suppression
  rules) and shows that the corresponding rule now fails. This proves
  each sample fails only because of its own kind.
- Bug fixes found on the way get a regression test (AGENTS.md).

## Out of Scope

- Threshold rules of any kind in the contract (complexity number,
  duplication, docstring coverage, file length, coverage percentage).
- Mutation testing, and ratchets that need a base revision: suppression
  counts, test counts, skip markers.
- Locking lint or test configuration behind an unlock, and CODEOWNERS
  rules (ADR 0027, Q4).
- Dependency direction as a contract rule.
- Checking pre-commit hooks by execution. Secret scanning in pre-commit
  is in the references only.
- `--no-verify` on `git merge`, `git rebase`, `git am` and other
  commands; Git configuration through environment variables
  (`GIT_CONFIG_*`); hook-manager switches other than `LEFTHOOK=0` and
  `HUSKY=0`.
- Promoting any new rule to MUST (a contract 2.0 decision).
- A release of `vibe` itself; the existing release process applies.

## Constraints

- ADR 0025: rules name properties, not tools or content; behaviour is
  checked by probes; the audit never writes into the adopter's tree.
- ADR 0027: SHOULD only; one rule per defect kind; specific
  suppressions are the repository's choice.
- Adding a deny case to A-02's corpus is MAJOR, so A-09 has its own
  cases.
- Principle 2: everything runs the same on Windows and Linux.
- Principle 3: the references stand on third-party tools; nothing they
  write calls `vibe`.

## Assumptions (not yet verified)

1. Ruff reports `PGH004` on a blanket `# noqa` even though that `noqa`
   covers its own line. If it does not, the Python reference needs
   another way to fail V-08.
2. The `eslint-comments` rule `no-unlimited-disable` reports a
   file-level `eslint-disable` that would otherwise disable it. If it
   does not, the TypeScript reference needs another way.
3. `gitleaks dir` honours an exclusion configuration well enough to
   skip `node_modules`, `.venv` and build output in reasonable time,
   and it reports the V-07 sample with the default rules.
4. Pyright in `standard` mode reports a string returned from a function
   annotated `-> int`.
5. This repository has few enough functions over the new Go
   thresholds that baseline suppressions are practical.

Each assumption is checked first during implementation. If one fails,
the spec is revised before the reference is changed.

## Acceptance Criteria

1. `vibe audit` reports V-05, V-06, V-07, V-08 and A-09 as SHOULD rules
   under contract 1.2.0, and `vibe explain` describes each.
2. For each of V-05 to V-08, a fixture whose `verify` misses the defect
   fails the rule, and the finding names `verify`; the same for
   `verify-fast`.
3. A configuration declaring 1.1.0 whose only failures are the new rules
   is conformant.
4. `vibe-guard` refuses every A-09 deny case and allows every A-09
   allow case; an exception naming `git-skip-hooks` lets its command
   through.
5. A guard that refuses only the 1.1.0 corpus passes A-02 and A-03 and
   fails A-09.
6. No file in this repository contains a literal PEM private-key
   header.
7. `task audit` on this repository says `Verdict: conformant` with
   every new rule passing and no waiver.
8. `examples.yml` audits the three examples as conformant with every
   new rule passing, and each negative-control job shows the targeted
   rule failing.
9. The dependency direction between `internal/contract` and
   `internal/guard` is enforced by `task lint`.
10. `task verify` passes.

## Further Notes

- The audit runs `verify` and `verify-fast` eight more times per
  component. The conformance job gets slower; `--static` is
  unaffected.
- Goodhart's law still applies: an adopter's linter can be tuned to
  catch exactly the planted samples, and an agent can split a function
  into meaningless pieces. ADR 0027 records both as accepted limits.
