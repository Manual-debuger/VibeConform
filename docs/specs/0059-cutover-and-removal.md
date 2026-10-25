# Spec 0059: Cutover to the contract audit, and removal of template sync

Status: accepted (2026-10-25). Issues #111 and #112, milestone "Contract v1
· Phase 4: Cutover and removal". Builds on specs 0051–0058. Carries out
the Consequences of ADR 0025, and depends on spec 0058 landing first.

## Problem

Two systems now live side by side in this repository:

- **The contract model:** `vibe audit` on a `vibe.yaml` version 2,
  `vibe-guard`, and the setup skill.
- **The template-sync model:** `vibe sync`/`diff`, `.vibe/state.yaml`,
  standards, modules, reconciliation, and the old audit.

ADR 0025 accepted this only because #112 would end it. The cost is
visible in every change:

- **Every hook change is made twice.** Spec 0057 changed both the module
  templates and the setup skill's references. The hook tests in
  `internal/module` exercise both.
- **This repository is held to the old model.** The required check is
  still `Conformance / audit`, which is the v1 hash audit. `Contract /
  audit` runs beside it but gates nothing. A hand edit of a generated
  file fails CI, even where the contract has no rule about that file.
  AGENTS.md has to explain `task sync`, `go:embed` staleness and
  `.vibe/state.yaml`.
- **The examples test the templates, not the references.**
  `examples/` comes from `vibe sync`. Nothing runs the setup skill's
  reference files as a whole repository.
- **An example shares this repository's green stamp.** The Stop gate's
  stamp (spec 0057) is `git rev-parse --git-path green-stamp`, in the
  old templates and in the references alike. An example has no `.git`
  of its own, so a session opened in `examples/monorepo/` reads and
  writes this repository's stamp. When this repository's `verify:fast`
  passed on a tree, the example's gate skips its own `verify:fast` on
  that tree, and the reverse. A manual check of spec 0058 found this
  (2026-10-25). CI does not show it, because `examples.yml` audits a
  copy in a fresh Git repository.
- **The CLI still promises the old model.** `vibe` describes itself as
  desired-state reconciliation. `vibe check` is a stub. `vibe init`
  writes only a version 1 `vibe.yaml`. `vibe doctor` understands only
  version 1.
- **The docs describe both models.** README, `docs/usage.md`,
  `docs/architecture/overview.md` and `docs/adopting.md` mostly explain
  sync and drift.

## Constraints

- **The new packages already stand alone.** `internal/contract`,
  `internal/guard`, `internal/guardrule`, `internal/setupskill` and
  `internal/doctor` import no old package (`TestImportsNoOldPackage`,
  `TestImportsOnlyGuard`). Only `internal/cli` and the old packages
  import the old packages.
- **The grilling decisions in #111 and #112 hold** (Q13, Q21, Q24):
  - `vibe` keeps `init`, `audit`, `unlock` and `doctor`.
  - `vibe-guard` stays a separate binary.
  - This repository's generated files are frozen as they are and become
    ordinary repository-owned files.
  - `.vibe/state.yaml` is no longer generated, and this repository's
    copy is deleted.
- **The contract does not change.** This repository stays conformant to
  contract 1.1.0 throughout.
- **R-01** (nothing but the audit calls `vibe`) still holds for this
  repository and for every example.
- **`.claude/settings.json` and `.claude/hooks/` are guardrail files.**
  Changing them needs a person's `vibe unlock`. From this spec on they
  are edited directly; they are no longer synced.
- **Branch protection is the user's setting.** Changing the required
  checks is an outward action that the user confirms or makes.
- **The repository is public.** No private document is linked.

## Assumptions

- Nobody depends on `vibe sync` from a release after this one. Adopters
  still on template sync follow the setup skill's migration guide, or
  pin an older release. The project is pre-1.0 alpha.
- The full contract audit stays fast enough to be a required check. The
  last runs on main took about five minutes.
- An example copied into a fresh Git repository audits the same as an
  adopting repository with the same files.

## Desired Behavior

### This repository: the contract audit gates (#111)

1. `vibe.v2.yaml` becomes `vibe.yaml`, and the version 1 `vibe.yaml` is
   deleted.
2. `Contract / audit` (the full audit) is a required check, next to
   `CI / gate`. `Conformance / audit` and `conformance.yml` are gone.
   `Contract / audit (static)` stays, but is not required.
3. `task audit` builds `vibe` and `vibe-guard` from the checkout, puts
   them first on `PATH`, and runs the full contract audit. `task
   audit:static` runs `--static`. `Taskfile.vibe.yml` is gone, and so
   is its optional include.
4. `.vibe/` is deleted. Every file it listed is an ordinary repository
   file from now on, with its content unchanged at the cutover.
5. The managed-section markers (`vibeconform:begin`/`end`) are removed
   from `AGENTS.md`, `CLAUDE.md`, `docs/README.md`,
   `docs/specs/README.md`, `.gitignore` and `.gitattributes`. The text
   between them stays.
6. The `vibeconform` skill (spec 0041), which teaches how this
   repository's synced tooling works, is deleted from `.claude/skills/`
   and `.agents/skills/`. The `spec` skill stays as an ordinary file.
7. `Taskfile.local.yml` loses `sync`. `hook-guard.yml` loses the jobs
   that sync a prod-go repository and run the `internal/module` guard
   tests. Its `vibe-guard` jobs stay. `setup-action.yml` stops reading
   `.vibe/state.yaml`.

### The CLI (#112)

8. `vibe` has the commands `init`, `audit`, `unlock` and `doctor`, plus
   help and version. `sync`, `diff` and `check` are gone. Running one of
   them prints one line that names the setup skill's migration guide,
   and exits 1. The root help describes the contract model.
9. `vibe audit` reads only `vibe.yaml` version 2:
   - A version 1 file (`standard:`) gets exit 1 and a pointer to the
     migration guide.
   - A missing file gets exit 1 and "run `vibe init`".
   - Flags, report format and exit codes are otherwise those of spec
     0052.
10. `vibe init` with no arguments writes a `vibe.yaml` version 2:
    - `contract:` the latest version this `vibe` implements;
    - `platforms: [linux, windows]`;
    - `agents: [claude]`;
    - `components` detected from `go.mod`, `pyproject.toml` and
      `package.json`, or none for a single component at the root;
    - the four entrypoints as comments to fill in.
    It refuses to overwrite an existing `vibe.yaml`, version 1 included,
    and says why. `vibe init --skill` is unchanged. `vibe init
    <standard> <version>` is gone.
11. `vibe doctor` reads `vibe.yaml` version 2 and reports, one line per
    check:
    - git;
    - the program of each entrypoint command, on `PATH`;
    - `bash` for the hook commands (Git Bash on Windows, found as the
      audit finds it);
    - `vibe-guard` on `PATH` when a PreToolUse hook calls it.
    It stays read-only. A version 1 file or a missing file gets the same
    messages as `vibe audit`.

### The code (#112)

12. These are deleted, with their tests:
    - `internal/module` and all its subpackages, the old Go, Python and
      Node guard templates included;
    - `internal/standard`, `internal/reconcile`, `internal/state`,
      `internal/resource` and `internal/manifest`;
    - `internal/textregion` and `internal/jsonarray`, if nothing else
      uses them;
    - the `internal/cli` files for sync, diff, plan, patch, prune, seed,
      sections, gitignore, tools, pin and the v1 audit.
13. Tests in `internal/module` that exercise the setup skill's
    references move next to the setup skill. They keep their behavior,
    with the module-template cases dropped:
    - `hookCheckSources` users;
    - `TestStopGateEverySource`;
    - the Go test-mode tests;
    - `TestSkillCIMatchesPins`.
    Tests that read deleted files are deleted: `TestGuardProbesOldGuards`
    and `TestRuntimeMarkersMatchTemplates`. So are tests that cover only
    the templates.

### The examples (#112)

14. `examples/monorepo`, `examples/python` and `examples/typescript` are
    rebuilt from the setup skill's reference files: Taskfiles, hooks,
    agents, CI, lint and a `vibe.yaml` version 2. Their source code and
    tests stay. They have no `.vibe/` and no `Taskfile.vibe.yml`.
15. `examples.yml` copies each example into a fresh Git repository and
    runs the full contract audit there, on Linux and on Windows. Each
    example must be conformant. The GitLab job audits the monorepo
    example with the reference `.gitlab-ci.yml` in place of the GitHub
    workflows. `examples.yml` keeps its gate job.
16. The reference Taskfiles key the green stamp on the project as well
    as the Git directory. A project at the root of its Git repository
    keeps the stamp it has now, so a single-project repository and an
    adopter's monorepo see no change. A project in a subdirectory of
    another repository's working tree, such as an example here, gets a
    stamp of its own. The stamp still lives in the Git directory, and
    the gate behaves as in spec 0057 otherwise.

### Documentation (#112)

17. README, `docs/usage.md`, `docs/architecture/overview.md` and
    `docs/adopting.md` describe only the contract model. Sync, drift,
    standards, managed files and `vibe check` are gone from them,
    except in a short note on how to leave the old system (pointing at
    the migration guide). `docs/usage.md` has a section on `vibe init`
    and `vibe doctor` as they now behave.
18. AGENTS.md's "Rules" drop the managed-files / `task sync` /
    `go:embed` rule. They say instead that `task audit` (the contract
    audit) must pass, and that guardrail files need a person's unlock.
19. GLOSSARY drops its "Retired terms" section.
20. Each ADR and spec whose subject is the template-sync model gets the
    status "superseded by ADR 0025 (removed in spec 0059)". The files
    are kept.

## User Stories

1. As the maintainer, I want one system in the repository, so that every
   hook or CI change is made and tested once.
2. As the maintainer, I want the contract audit to be the required
   check, so that this repository is held to what it asks adopters to
   meet.
3. As the maintainer, I want to edit Taskfile.yml, lefthook.yml and CI
   directly, so that a change needs no template, sync or rebuild.
4. As an agent in this repository, I want AGENTS.md to say what is
   true, so that I do not run `task sync` or look for templates that
   no longer exist.
5. As an agent, I want `task audit` to judge the checkout with the
   checkout's own `vibe` and `vibe-guard`, so that a stale binary on
   PATH never gives me a verdict.
6. As a contributor, I want `task audit:static` for a quick local run,
   so that I can check the static rules in seconds.
7. As an adopter still on template sync, I want `vibe sync` to tell me
   where the migration guide is, so that I am not left with "unknown
   command".
8. As an adopter with a version 1 `vibe.yaml`, I want `vibe audit` to
   tell me to migrate, so that I do not get a confusing verdict.
9. As a new adopter, I want `vibe init` to write a version 2 `vibe.yaml`
   with my components found, so that I start from a valid skeleton.
10. As a new adopter, I want `vibe init` to refuse to overwrite my
    `vibe.yaml`, so that I never lose my declarations.
11. As an adopter, I want `vibe doctor` to tell me which entrypoint
    programs are missing on my machine, so that I fix my machine before
    I blame the audit.
12. As an adopter on Windows, I want `vibe doctor` to tell me whether
    the bash it needs for hooks is there, so that hook probes do not
    fail without a reason.
13. As an adopter, I want `vibe doctor` to check `vibe-guard` when my
    hooks call it, so that I know the guard fails closed only because
    it is missing.
14. As an adopter reading the README, I want only the contract model
    described, so that I learn one way to use VibeConform.
15. As the maintainer, I want the examples built from the setup skill's
    references, so that the references are tested as whole
    repositories.
16. As the maintainer, I want each example audited on Linux and on
    Windows, so that a reference that breaks on one platform fails CI.
17. As the maintainer, I want a GitLab variant of the monorepo example
    audited, so that the GitLab reference CI stays conformant.
18. As the maintainer, I want the hook tests that cover the references
    kept, so that removing the templates removes no coverage of what
    adopters get.
19. As a reader of the decision history, I want old ADRs and specs
    marked superseded but kept, so that I can still follow why things
    are the way they are.
20. As a reader of the glossary, I want the retired terms gone, so that
    the glossary names only what exists.
21. As the maintainer, I want CI green and branch protection pointing
    at `Contract / audit`, so that the cutover is complete when this
    merges.
22. As a contributor in a session opened in an example, I want the
    example's Stop gate to keep its own green stamp, so that a green run
    of this repository never makes it skip the example's checks.

## Implementation Decisions

- **One spec, one integration branch, one PR.** The order inside the
  branch:
  1. The cutover of #111, while the old code still exists: rename the
     config, add the new `task audit`, remove `conformance.yml`, remove
     the markers and `.vibe/`.
  2. Move the reference hook tests out of `internal/module`.
  3. Delete the old packages and CLI commands.
  4. Rewrite `vibe init` and `vibe doctor`.
  5. Rebuild the examples.
  6. Update the docs.
  Branch protection changes after the PR's `Contract / audit` is green,
  and before the merge.
- **The CLI's removed commands are small stubs.** Each is one hidden
  command that prints the migration pointer. `vibe audit`'s v1/v2
  dispatch collapses to v2 only, and `contract.DetectSchema` is kept,
  so that a version 1 file gets its own message.
- **`vibe init`'s detection is a small function in `internal/cli`.** It
  walks the tracked tree up to a fixed depth. It treats each directory
  with `go.mod`, `pyproject.toml`, or `package.json` plus `tsconfig.json`
  as a component, and skips `node_modules`, `vendor` and ignored paths.
  A single component found at the root means no `components:` key. The
  output is checked against `contract`'s config parser, so `init` never
  writes a file `audit` cannot read. Entrypoints are left as comments,
  so V-04 fails until the adopter (or the setup skill) declares them.
- **`vibe doctor` reuses the audit's readers.** It uses the contract
  config parser, the entrypoint splitter (simple commands, as V-04
  reads them), the hook reader, and the audit's bash resolution. It
  imports only new packages. Its old checks of standards, modules,
  graphify and line endings are dropped.
- **This repository's `task audit`** builds both binaries with
  `go build` into the Git directory, as spec 0057's self-build does,
  and puts that directory first on `PATH`. The probes then use the
  checkout's `vibe-guard` (A-02 runs the one on `PATH`).
- **Frozen files stay byte for byte** at the cutover, except for the
  markers and the edits listed in Desired Behavior 3–7. Later changes
  are ordinary edits.
- **The moved hook tests** go into a test package next to
  `internal/setupskill`, which imports only `setupskill`, `guard` and
  `contract`. Spec 0058's new hook test already lives there.
- **Examples are assembled by hand from the references**, not by a
  generator. A test (Seam 3) keeps them conformant. A drift test
  between an example and its references is not added: the audit is the
  check.
- **The stamp's name carries the project's path.** `hook:stamp:check`
  and `hook:stamp:save` read `git rev-parse --show-prefix` in the
  Taskfile's directory. An empty prefix keeps `green-stamp`. Any other
  prefix gives `green-stamp-<prefix>`, with `/` turned into `-`. The
  hash and the rest of spec 0057's design stay. Spec 0057's
  constraint that the stamp never leaves the Git directory still holds.
  This repository's own Taskfile is at the root, so it keeps its stamp
  and needs no edit.
- **No contract change, no new dependency.** No new ADR, because ADR
  0025 already decided this.

## Testing Decisions

A good test here runs the `vibe` command, or the hook, the way a person
or CI does, and checks the exit code, the output and the files. It does
not check which packages exist.

- **Seam 1, the `vibe` CLI in-process** on fixture Git repositories.
  This is the existing harness of the `contract_*` tests in
  `internal/cli`:
  - `audit` on version 2 (unchanged suites);
  - `audit` and `doctor` on a version 1 file and on no file;
  - `init` on fixtures: Go at the root, Python, TypeScript, a
    three-language monorepo, an existing `vibe.yaml`. The written file
    must parse, and must get exactly the V-04 finding for the missing
    entrypoints;
  - `doctor` with a missing entrypoint program and with a missing
    `vibe-guard`;
  - `sync`, `diff` and `check` print the pointer and exit 1.
  Prior art: `contract_config_test.go`, `contract_dispatch_test.go`,
  `init_skill_test.go`, `doctor_test.go`.
- **Seam 2, the reference hooks** (moved). These are the reference cases
  of `TestHookCheckSkipsEditsThatCannotMatter`,
  `TestHookCheckRunsTypecheckAndLintOnly`, `TestStopGateEverySource`,
  the Go test-mode tests and `TestSkillCIMatchesPins`, unchanged in
  behavior, in their new package. Spec 0058's hooks-from-any-directory
  test sits beside them. A new case runs the stamp tasks of a project in
  a subdirectory of a fixture repository and of a project at its root:
  a green save in one does not make the other's check pass, and the
  root project's stamp keeps its name.
- **Seam 3, the examples in CI.** `examples.yml` audits each example on
  Linux and Windows, plus the GitLab variant. This is CI only, a real
  integration of the references; it is a separate line in the ledger.
- **This repository:** `task audit` is conformant, nothing is undecided,
  and the SHOULD findings are no worse than at the branch point.
  `Contract / audit` is green on the PR.
- **Removed tests:** tests of deleted packages go with them. Each
  deleted test that covered the setup skill's references is accounted
  for by a moved test, and the PR lists them.

## Non-goals

- Changing the contract, or any rule's check.
- #86 (graphify guidance in the setup skill). It becomes simpler once
  this lands, but it is separate.
- Trimming what `vibe init --skill` writes (pilot finding F-17). It is
  a candidate follow-up.
- A generator or drift test that keeps the examples equal to the
  references.
- Merging `Taskfile.local.yml` into `Taskfile.yml`.
- Releasing and tagging. The release after this merge is a separate
  step, and its notes say that `sync` is gone.

## Acceptance Criteria

- [ ] `vibe.yaml` is version 2 (the former `vibe.v2.yaml`). `.vibe/`,
      `Taskfile.vibe.yml` and `conformance.yml` are gone. No
      `vibeconform:begin` marker remains outside `docs/specs`,
      `docs/decisions` and the setup skill's text.
- [ ] Branch protection on `main` requires `CI / gate` and `Contract /
      audit`, and no longer requires `Conformance / audit`. The user
      confirms or makes the change.
- [ ] `task audit` builds the checkout's `vibe` and `vibe-guard` and is
      conformant, with nothing undecided. `task audit:static` runs the
      static audit.
- [ ] `vibe --help` lists `init`, `audit`, `unlock` and `doctor`. `vibe
      sync`, `vibe diff` and `vibe check` print the migration pointer
      and exit 1.
- [ ] `vibe audit` and `vibe doctor` give exit 1 and a pointer for a
      version 1 file, and for no file.
- [ ] `vibe init` writes a version 2 `vibe.yaml` with the detected
      components for each fixture, refuses an existing file, and the
      audit then reports only the missing entrypoints (V-04).
- [ ] `vibe doctor` reports a missing entrypoint program, missing bash
      and a missing `vibe-guard`, and changes no file.
- [ ] The old packages and CLI files of Desired Behavior 12 are gone,
      and `go build ./...` and `go vet ./...` pass.
- [ ] Every test that covered the setup skill's references runs in its
      new place and passes. The PR lists the deleted tests.
- [ ] The three examples come from the references, with version 2
      configs. `examples.yml` audits each one conformant on Linux and
      Windows, and the GitLab variant conformant too.
- [ ] A project in a subdirectory of another repository's working tree
      gets its own green stamp. A project at the root keeps
      `green-stamp`. A test shows both.
- [ ] README, `docs/usage.md`, `docs/architecture/overview.md`,
      `docs/adopting.md`, AGENTS.md and GLOSSARY are updated as in
      Desired Behavior 17–19. Superseded ADRs and specs are marked.
- [ ] `task verify` and `task audit` pass, and CI is green with the new
      required checks.

## Further Notes

- **Spec 0058 lands first.** Its hook test is placed where step 2 above
  moves the others. It patches only the templates that survive until
  this spec.
- **The `vibeconform` skill** in this repository and its spec 0041 copy
  in adopters' repositories differ. Adopters' copies are theirs since
  the contract model. Only this repository's copy is deleted.
- **#81 and #133** change the monorepo hooks. After this spec, they
  touch only the references and the monorepo example.
- **Releases.** v0.13.x is the last release with `sync`. The migration
  guide (`reference/migrate.md`) already covers leaving template sync.
