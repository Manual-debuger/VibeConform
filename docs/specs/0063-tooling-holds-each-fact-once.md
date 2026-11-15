# Spec 0063: The tooling holds each fact once

Status: draft (2026-11-15). Issue #164, from the design review #167.
Amends spec 0052 ("vibe.yaml", "The verdict") and spec 0061 (acceptance
criterion 7, how this repository checks its SHOULD rules). No contract
change: the contract stays 1.2.0.

## Problem

This repository's tooling keeps several facts in more than one place,
and one copy reads text that was written for people:

1. **The SHOULD-rule gate exists twice.** Spec 0061 requires this
   repository to meet its SHOULD rules (V-06, V-07, V-08 and A-09 pass,
   V-05 is not applicable). The verdict counts only MUST rules, so two
   scripts check them:
   - `task audit` matches the text report, including its column
     padding. A cosmetic change to the text renderer breaks
     `task audit`.
   - CI's `Contract / audit` job reads the JSON report with `jq` and
     keeps its own list of rule IDs.

   CI cannot call `task audit` instead: C-04 requires a CI step that
   runs `vibe audit` itself. And `task audit` cannot read JSON with
   `jq`, which a Windows machine with Git Bash does not have.
2. **Tool installation is copy-pasted.** The gitleaks download and
   SHA-256 check appears seven times in four workflows, and the
   `goimports`, `govulncheck` and `actionlint` versions are pinned
   inline in several workflows each.
3. **Work runs twice.** `task lint` runs `go vet` and then golangci-lint,
   which runs `govet` too, locally, in CI and in every probe-copy
   `verify`. `Contract / audit (static)` runs `vibe audit --static`
   twice, once for text and once for JSON.
4. **The green-stamp hash script is written twice per Taskfile**, in
   `hook:stamp:check` and `hook:stamp:save`: in this repository, in the
   four reference Taskfiles of the setup skill and in the three examples.
5. **A leftover alias.** `task verify-ci` has no caller.
6. **The docs and the tooling disagree.** AGENTS.md says a new package
   boundary gets a `depguard` rule, but the `doctor` and `setupskill`
   boundaries are enforced by import tests.

## Constraints

- C-04 needs a CI job, triggered on pull requests, with a step whose
  simple command starts with `vibe audit` and no step that calls a
  verify command. The `Contract / audit` job must keep one.
- `task audit` runs on Windows with Git Bash and Task's built-in shell.
  It can rely on `go`, `git` and Task, not on `jq`.
- The verdict's meaning for a repository that does not use the new
  configuration must not change: same verdict, same exit code, same
  text and JSON output.
- JSON schema 1 may gain keys, never lose or rename them (spec 0052).
- `vibe.yaml` decoding stays strict: an unknown key is an error.
- A planted defect that a contract probe expects `verify` to catch
  (V-06 to V-08) must still be caught after `go vet` leaves `task lint`.
- The setup skill's references are what adopters copy. A change to a
  reference Taskfile keeps every task name and exit code the Stop hook
  and the contract read (A-05, spec 0057).
- No `.claude/settings.json` or `.claude/hooks/` change is needed. If
  one turns out to be needed, it waits for the user's `vibe unlock`.

## Assumptions

Not verified yet; each is checked before the step that depends on it.

1. golangci-lint v2's `govet`, with this repository's configuration,
   runs every analyzer `go vet ./...` runs by default. If it does not,
   the missing analyzers are enabled in `.golangci.yml` before `go vet`
   is dropped.
2. C-02 accepts a local composite action (`uses: ./.github/actions/…`):
   it skips `uses:` values that start with `./` today.
3. Moving tool installs into a composite action changes no C rule, D-03
   or R-01 result: those read the commands of a job's `run:` steps, and
   installing a tool runs no entrypoint and no scanner.
4. Every workflow that installs tools checks out this repository at the
   workspace root before it installs them, so the local action path
   resolves.
5. Task cannot run an internal task from the command line, so the shared
   hash script is not `internal: true` if the other tasks capture its
   output by running `task`.

## Desired Behavior

### A repository can require SHOULD rules

- `vibe.yaml` gains an optional top-level key, `require`: a list of
  SHOULD rule IDs that the repository holds itself to.
- A required rule counts in the verdict like a MUST rule:
  - It fails: the verdict is not conformant (exit 2) once every MUST
    and required rule is decided.
  - It cannot be decided or does not run: there is no verdict (exit 1).
  - It is not applicable: that is fine, as for a MUST rule.
- The reason names required rules apart from MUST rules, such as
  `not conformant: 1 required SHOULD rule fails` or
  `not conformant: 1 MUST rule fails; 2 required SHOULD rules fail`.
- The text report marks a required rule where it lists it, and the JSON
  report gives every rule a boolean `required` key. Schema stays 1.
- Configuration errors, each exit 1 with no report, naming the field
  path:
  - an ID that is not a rule of the contract this vibe implements;
  - a MUST rule ("C-02 is a MUST rule; it is always required");
  - the same ID twice;
  - an ID that also has a waiver, expired or not ("V-06 is both
    required and waived").
- With no `require` key, nothing changes.

### This repository

- `vibe.yaml` requires V-05, V-06, V-07, V-08 and A-09.
- `task audit` runs `vibe audit` and nothing more: no text matching.
- `Contract / audit` runs `vibe audit` and nothing more: no `jq`, no
  rule list. It still satisfies C-04.
- `Contract / audit (static)` runs `vibe audit --static --json` once,
  prints a summary from that JSON, and fails when a MUST or required
  rule fails or cannot be decided.
- A local composite action holds every tool pin and the gitleaks block.
  Each workflow that installs these tools uses it.
- `task lint` runs golangci-lint only. The Go reference Taskfile and the
  Go example Taskfile drop `go vet` too, under the same condition
  (assumption 1).
- Each Taskfile with the green stamp computes the hash in one place.
  `hook:stamp:check` and `hook:stamp:save` keep their names, their exit
  codes and the stamp's location.
- `task verify-ci` is gone. Historical documents that mention it stay
  as they are.
- `depguard` rules enforce the `doctor` and `setupskill` boundaries, and
  their import tests are deleted. AGENTS.md's sentence is then true as
  written.
- AGENTS.md says why CI runs `vibe audit` directly rather than
  `task audit` (C-04), and that both read the same `vibe.yaml`.

## User Stories

1. As a maintainer, I want the list of SHOULD rules this repository
   meets to live in one file, so that adding one is a one-line change.
2. As a maintainer, I want `task audit` to fail when a required SHOULD
   rule fails, so that I learn it locally rather than in CI.
3. As a maintainer, I want to change the text report's layout without
   breaking `task audit`, so that the renderer is free to change.
4. As a maintainer, I want CI's audit and `task audit` to reach the same
   result from the same configuration, so that "the same audit" in
   AGENTS.md is true.
5. As an adopter, I want to hold my repository to SHOULD rules it
   already meets, so that a regression fails my audit.
6. As an adopter, I want a required rule that is not applicable to my
   repository to be accepted, so that listing V-05 in a Go-only
   repository is not a failure.
7. As an adopter, I want a typo in `require` to be a configuration
   error, so that a misspelled ID does not silently require nothing.
8. As an adopter, I want listing a MUST rule in `require` to be an
   error, so that the file does not suggest MUST rules are optional.
9. As an adopter, I want a rule that is both waived and required to be
   an error, so that the configuration cannot contradict itself.
10. As an adopter who does not use `require`, I want the same verdict,
    exit code and output as before, so that upgrading vibe is safe.
11. As a reader of the report, I want required rules marked, so that I
    know why a SHOULD failure made the repository not conformant.
12. As a reader of the verdict reason, I want MUST and required failures
    counted separately, so that I know which kind failed.
13. As a consumer of the JSON report, I want a `required` key on every
    rule, so that my script does not keep its own list.
14. As a maintainer, I want one place that pins each CI tool's version,
    so that an upgrade is one edit.
15. As a maintainer, I want gitleaks downloaded and checked against its
    SHA-256 by one block, so that a new release is one edit.
16. As a maintainer, I want `task lint` to run `govet` once, so that
    lint and every probe-copy `verify` are faster.
17. As a maintainer, I want the static audit job to run the audit once,
    so that it does not do the same work twice.
18. As a maintainer, I want the green-stamp hash written once per
    Taskfile, so that `check` and `save` cannot drift apart.
19. As an adopter, I want the reference Taskfiles to compute the stamp
    the same way, so that my Stop hook keeps working after I copy them.
20. As a maintainer, I want unused tasks removed, so that the Taskfile
    lists only what something runs.
21. As a maintainer, I want every package boundary enforced by
    `depguard`, so that AGENTS.md describes the tooling as it is.
22. As an agent, I want AGENTS.md to say why CI runs `vibe audit`
    directly, so that I do not "fix" it into `task audit` and break C-04.

## Implementation Decisions

### `require` in vibe.yaml

- `internal/contract`'s config parser gains `require`, parsed into the
  `Config` as a set of rule IDs, after `waivers` so that the conflict
  check sees both. Errors follow the waiver errors' wording and path
  form (`require[1]: …`).
- The verdict is decided in `internal/contract`, where `decide` counts
  MUST rules today. It counts a rule as binding when its level is MUST
  or its ID is required. The report gains a count of failing required
  rules beside `FailingMust`, and each `RuleResult` says whether it was
  required.
- The CLI renders the mark and the reasons. The JSON rule object gains
  `"required": true|false`. Schema stays 1.
- No contract version change. The contract's rules and checks do not
  change; `require` is configuration of the vibe that reads it, like
  `waivers`. A vibe without this change rejects the key as unknown,
  which fails closed. `docs/usage.md` says which vibe release first
  reads it.
- The setup skill's reference `vibe.yaml` files do not add `require`.
  `docs/usage.md` documents it with the other fields, and the setup
  skill's SKILL.md mentions it as optional.

### This repository's audit

- `vibe.yaml`: `require: [V-05, V-06, V-07, V-08, A-09]`.
- `task audit`'s full branch keeps building `vibe` and `vibe-guard` into
  the Git directory and prepending it to `PATH`, then runs `vibe audit`
  and exits with its code. The text-matching block is deleted.
- `Contract / audit`'s audit step runs `vibe audit`. `REQUIRED_SHOULD`
  and the `jq` script are deleted.
- `Contract / audit (static)`: one step runs `vibe audit --static --json`
  into a file, prints one line per rule and the verdict from it, and
  applies the existing gate with `.level == "MUST" or .required`.

### CI tool installation

- A composite action under `.github/actions/` installs the tools a job
  asks for, by input: Task, goimports, golangci-lint, govulncheck,
  actionlint, gitleaks. It holds every version pin and gitleaks'
  SHA-256 sums, for Linux and Windows runners. Its run steps set
  `shell: bash`.
- `ci.yml`, `contract-audit.yml`, `examples.yml` and
  `negative-control.yml` use it in place of their inline installs.
  Workflow-level `env` pins that only fed those installs are removed.
  Go, Node, pnpm and uv stay on their setup actions.
- The setup skill's reference CI files are not changed: adopters cannot
  use this repository's local action.

### Lint, stamp, alias

- `task lint`: `golangci-lint run` only, and its `desc` says so. The Go
  reference Taskfile and `examples/monorepo/services/api` do the same,
  each after checking assumption 1 against its own golangci
  configuration.
- Green stamp: one task per Taskfile prints the tree's hash, or nothing
  when it cannot compute one. `hook:stamp:check` compares it with the
  stamp; `hook:stamp:save` writes it. The hash's inputs and the stamp's
  path are unchanged. The same edit goes to this repository's Taskfile,
  the four reference Taskfiles and the three example Taskfiles.
- `verify-ci` is deleted from `Taskfile.yml`.

### Boundaries and AGENTS.md

- `.golangci.yml` gains two `depguard` rules for non-test files:
  `internal/doctor` may import no package of this module except
  `internal/contract`; `internal/setupskill` none except `internal/guard`.
  `internal/doctor/imports_test.go` and `TestImportsOnlyGuard` are
  deleted.
- AGENTS.md's "Rules" says `task audit` and `Contract / audit` both run
  `vibe audit` on `vibe.yaml`, and that CI runs it directly because C-04
  requires a step that does. `docs/architecture/overview.md` is synced
  if it describes either.

## Testing Decisions

- A good test drives `vibe audit` through its existing seam and asserts
  on the verdict, the exit code and the report, never on internal
  counters. The seam stays the CLI, in process (spec 0052). #165 moves
  it later; these cases move with it.
- `require` parsing: new cases in the config error table, beside the
  waiver cases. One per error: unknown ID, MUST rule, duplicate, waived
  and required. One valid case.
- Verdict: new engine cases with stubbed rule results. A required rule
  that fails gives not conformant and exit 2. One that is undecided gives
  exit 1. One that is not applicable gives conformant. A failing SHOULD
  rule that is not required leaves a conformant verdict, as today.
- Output: the golden text and JSON reports gain one fixture with a
  required rule. The existing goldens, which have no `require`, must not
  change except for the new `"required": false` key in JSON.
- The green stamp: the existing reference Stop-hook and hooks-anywhere
  tests run the reference Taskfiles. They must pass unchanged, on Linux
  and Windows.
- `depguard`: checked once by hand. Add a forbidden import to each
  package, see `task lint` fail, revert. The PR records the output.
- Workflows: `task workflows:lint` (actionlint) and a green CI run on the
  PR, including `Contract / audit`, both static and full.

## Non-goals

- Changing the contract, its rules or its version.
- Changing the setup skill's reference CI files, or adding `require` to
  the reference `vibe.yaml` files.
- A `--require` flag on `vibe audit`.
- The other findings of #167: #159 to #163, #165, #166.
- Rewriting historical documents that mention `verify-ci`.

## Acceptance Criteria

- [ ] `vibe.yaml` accepts `require` as described, and each invalid form
      is a configuration error naming its path.
- [ ] A failing required rule makes the verdict not conformant (exit 2);
      an undecided one makes it undecided (exit 1); a not-applicable one
      leaves it conformant.
- [ ] Without `require`, the text report is byte-identical to before, and
      the JSON report differs only by `"required": false` on each rule.
- [ ] This repository's `vibe.yaml` requires V-05, V-06, V-07, V-08 and
      A-09, and `task audit` says `Verdict: conformant`.
- [ ] `Taskfile.yml` and `contract-audit.yml` contain no rule IDs and no
      text matching of the report.
- [ ] `Contract / audit` still satisfies C-04 (the audit passes) and is
      green on the PR.
- [ ] `Contract / audit (static)` runs `vibe audit` once.
- [ ] Each tool version and the gitleaks SHA-256 sums appear once under
      `.github/`.
- [ ] `task lint` does not run `go vet`, and V-06 to V-08's planted
      defects are still caught (the full audit passes).
- [ ] Each Taskfile with a green stamp has one copy of the hash script,
      and the reference Stop-hook tests pass on Linux and Windows.
- [ ] `task verify-ci` no longer exists.
- [ ] `depguard` enforces the `doctor` and `setupskill` boundaries, and
      their import tests are gone.
- [ ] AGENTS.md describes how CI and `task audit` run the audit, and
      why.
- [ ] `task verify` and `task audit` pass.

## Further Notes

- Spec 0061's acceptance criterion 7 stays true; only how it is checked
  changes, from two scripts to `require`.
- `require` is the opposite of a waiver: a waiver lets a SHOULD rule
  fail, `require` makes it count. A rule cannot be both.
