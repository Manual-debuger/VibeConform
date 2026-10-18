# Spec 0054: Release and setup skill (Phase 3)

Status: draft (2026-10-18). Tickets #107, #108, #109, #110 (milestone
"Contract v1 · Phase 3: Release and skill"), spec issue #118. Builds on
spec 0051 (contract v1.0.0), spec 0052 (Phase 1), spec 0053 (Phase 2),
ADR 0025 and ADR 0026. "Qn" refers to the grilling questions the tickets
cite. Those decisions are kept here. Where this spec and spec 0051
disagree, spec 0051 wins, and the disagreement is a defect in this spec.

## Problem Statement

After Phase 2, `vibe audit` decides all of contract 1.0.0, and
`vibe-guard` guards this repository. An adopter still cannot use either
without Go, and has no supported way to give a repository the contract's
properties:

- **No release has `vibe-guard`.** goreleaser builds only `vibe`, and
  every release is a draft that a person publishes by hand. The
  PreToolUse hook tells a developer without `vibe-guard` to run
  `go install …/cmd/vibe-guard@<version>`. No published tag contains
  `cmd/vibe-guard`, so that command fails today, for this repository
  (`@latest`) and for any adopter (`@<version>`).
- **Every CI job that runs `vibe` needs Go.** Each job builds it from
  source or runs `go install`. A Python or TypeScript repository must
  install a Go toolchain only to audit itself.
- **The binaries cannot be traced to a build.** There are checksums but
  no provenance, so nobody can confirm which commit and workflow built a
  binary.
- **There is no setup skill.** The only skill VibeConform writes,
  `vibeconform`, describes the template-sync model: managed files,
  `vibe sync`, seams. Nothing helps an agent take a repository in any
  language to a passing contract audit. The module templates hold the
  knowledge of how to do that, but only the old system can use them.
- **A skill drifts silently.** Nothing checks that the commands, rule IDs
  and files a skill names exist.
- **The contract has not met a real repository.** Every fixture and
  example was written next to the contract. Nobody knows how many manual
  changes a real team repository needs, or whether the files the skill
  writes are readable.
- **Four real-session checks from spec 0053 are still UNVERIFIED**:
  - the Stop hook blocks once and then lets the stop through;
  - an unlock opens and relock closes;
  - whether a `Write|Edit` matcher sees `MultiEdit`;
  - which shell Claude Code runs hooks with on Linux and macOS.

## Solution

Phase 3 makes the contract adoptable:

- **Releases** (#107). goreleaser publishes real releases of `vibe` and
  `vibe-guard` for Linux, macOS and Windows on amd64 and arm64, with
  checksums and GitHub artifact attestations. A job checks every release
  after it is published.
- **A setup action** (#107), at this repository's root. It installs
  `vibe` and `vibe-guard` from a release and checks their checksums. It
  is meant for the conformance job only. For a version that has no
  release, and for teams that do not allow unreviewed binaries, it can
  build from source instead.
- **The examples' CI drops setup-go** (#107). The conformance template
  uses the setup action, and the TypeScript and Python CI templates
  install Task without Go.
- **The setup skill** (#108), `vibeconform-setup`. It is a skill
  directory embedded in `vibe`, and `vibe init --skill` writes it into
  the repository. It detects the stack, keeps the team's task runner,
  writes single-file configurations, installs `vibe-guard`, and runs
  `vibe audit --json` until the audit passes. It migrates a repository
  from the template-sync system. Its upgrade flow asks a person to run
  `vibe unlock` before a contract MAJOR upgrade changes the guardrail
  settings. The module templates become its reference files.
- **A drift test** (#109) fails CI when the skill names a rule ID, guard
  rule ID, command, flag, `vibe.yaml` field or file that does not exist.
- **A pilot** (#110) in one real team repository and two stand-ins built
  from the examples. A pilot report is committed. The contract is
  adjusted from the results, with a semver bump where needed.
- **The open real-session checks** of spec 0053 are run in this phase:
  on Windows in this repository, and on Linux in the pilot repository
  under WSL. Each is recorded as PASS, FAIL or UNVERIFIED.

## User Stories

### Releases and the setup action (#107)

1. As an adopter, I want prebuilt `vibe` and `vibe-guard` binaries for
   Linux, macOS and Windows on amd64 and arm64, so that I do not need Go
   to audit my repository or guard my agent.
2. As an adopter, I want `vibe-guard` released as its own archive, so
   that I can install the guard without `vibe` and keep it after I
   remove VibeConform (ADR 0025, decision 8).
3. As an adopter, I want every archive listed in a checksum file, so
   that I can check a download before I run it.
4. As a security reviewer, I want a GitHub artifact attestation for
   every released archive and binary, so that `gh attestation verify`
   can show me which commit and workflow built it.
5. As the maintainer, I want a tag push to publish the release without a
   manual step, and a pre-release suffix to keep marking it
   "Pre-release" (ADR 0007), so that releases are not stuck as drafts.
6. As the maintainer, I want a job that downloads each new release on
   every platform it can run, runs both binaries and verifies their
   attestations, so that a broken release is found when it is cut.
7. As the maintainer, I want goreleaser to build every target as a
   snapshot on each pull request, so that a broken release configuration
   fails before a tag exists.
8. As an adopter, I want a setup action that installs an exact version
   of `vibe` and `vibe-guard` and checks their checksums, so that my
   conformance job needs no Go and runs the version I chose.
9. As an adopter, I want the setup action to refuse `latest` and any
   version that is not exact, so that my CI verdict cannot change
   without a change to my repository (spec 0022, C-02).
10. As an adopter, I want to opt in to attestation verification in the
    setup action, so that CI checks the provenance as well as the
    checksum.
11. As a team that does not allow unreviewed binaries, I want the setup
    action to build `vibe` and `vibe-guard` from the tagged source with
    `go install`, so that we keep the action and avoid prebuilt
    binaries.
12. As a team that does not allow unreviewed binaries, I want the docs to
    explain how to build both binaries from source, so that we can
    install them on developer machines too.
13. As a developer without `vibe-guard`, I want the hook's install hint
    to work, and to name the release page as well as `go install`, so
    that I can install the guard with or without Go.
14. As the maintainer of the example repositories, I want their CI to
    have no setup-go where no Go component needs it, so that the
    examples show a Python or TypeScript repository that needs no Go.
15. As this repository's maintainer, I want my own conformance job to
    keep auditing with a `vibe` built from my working tree, so that
    template changes are not compared against the previous release
    (ADR 0008).

### The setup skill (#108)

16. As an adopter, I want `vibe init --skill` to write the setup skill
    into `.claude/skills/` and `.agents/skills/`, so that both Claude
    Code and Codex can load it, and the repository owns it (Q22).
17. As an adopter, I want `vibe init --skill` to overwrite the skill on
    upgrade and remove files the new version no longer has, so that the
    skill matches my `vibe` and no stale reference file is left behind.
18. As an adopter, I want `vibe init --skill` to touch nothing outside
    the two skill directories, so that running it is always safe.
19. As an adopter, I want the skill to record the `vibe` version it came
    from, so that I can tell which version I have.
20. As an adopter, I want the skill to detect my components and their
    languages the way the contract does, so that the `vibe.yaml` it
    writes is valid.
21. As an adopter, I want the skill to keep my task runner (Task, make,
    just or package scripts) and use Task only when I have none, so that
    my team does not have to switch tools.
22. As an adopter, I want the skill to keep my hook manager, CI layout
    and lint rules when they already satisfy a rule, so that it changes
    only what the contract needs.
23. As an adopter, I want each configuration in a single file, with no
    `*.local.*` or `*.base.*` seam files, so that my repository has
    fewer files and each is easy to read.
24. As an adopter, I want the skill to write `vibe.yaml` version 2 with
    my entrypoints, platforms and agents, so that the audit can run.
25. As an adopter, I want the skill to run `vibe audit --json` and fix
    what each finding names until the verdict is conformant, so that I
    know the setup is finished.
26. As an adopter, I want the skill to stop and tell me when a finding
    needs a person (installing a tool on my machine, an unlock, a CI
    provider the contract does not support), so that it does not loop
    or work around the problem.
27. As an adopter, I want the skill to stop when two audit rounds in a
    row fix nothing, so that a failing setup ends with a clear report.
28. As an adopter, I want the skill to install `vibe-guard` from a
    release with its checksum checked, or with `go install`, so that the
    guard hook works on my machine (Q12).
29. As an adopter, I want `AGENTS.md` to name my verify entrypoints and
    the rule not to weaken a test, lint or type check to make a change
    pass, so that every agent sees them (R-02, #83).
30. As an adopter, I want the skill to write `CLAUDE.md` so that it
    references `AGENTS.md`, so that Claude Code reads the same rules
    (R-02).
31. As an adopter of the template-sync system, I want the skill to
    migrate my repository: merge the seam files into single files,
    remove `.vibe/` and the `vibe`-only task files, and write
    `vibe.yaml` version 2, so that I can leave `vibe sync` behind.
32. As an adopter, I want optional reference content for integrations
    (graphify, VS Code, Zed, language servers) and for an `AGENTS.md`
    workflow section, so that I can still get what `integrations:` and
    `development:` gave me (Q23).
33. As an adopter upgrading `vibe`, I want an upgrade flow that bumps
    `contract:`, so that my repository follows the version my `vibe`
    implements.
34. As an adopter upgrading across a contract MAJOR version, I want the
    skill to ask me to run `vibe unlock` before it changes
    `.claude/settings.json` or `.claude/hooks/`, so that the agent never
    edits its own guardrails without me (A-08).
35. As an adopter, I want `vibe --version` to name the contract version
    `vibe` implements, so that the skill and I can see which upgrade is
    due.

### The drift test (#109)

36. As the maintainer, I want CI to fail when the skill names a rule ID
    that is not in the contract, so that the skill cannot point at a
    rule that does not exist.
37. As the maintainer, I want CI to fail when the skill names a `vibe` or
    `vibe-guard` command or flag that does not exist, so that the
    instructions stay runnable.
38. As the maintainer, I want CI to fail when the skill names a skill file
    that does not exist, so that the skill's links work.
39. As the maintainer, I want CI to fail when a contract rule has no
    guidance in the skill, so that a new rule cannot ship without a way
    to satisfy it.
40. As the maintainer, I want the skill's reference hook commands to equal
    the reference guard's commands, so that the skill and `vibe-guard`
    cannot disagree.
41. As the maintainer, I want the skill's reference files for each
    language to pass the static audit when assembled into a repository,
    so that the reference implementations satisfy the contract.

### The pilot (#110)

42. As the maintainer, I want the skill run in a real team monorepo
    (Go, Python, TypeScript; GitLab CI; Linux under WSL), so that the
    contract meets a repository it was not written for.
43. As the maintainer, I want the skill run on stand-ins built from the
    examples, one migrated and one set up from scratch, so that both
    entry paths are tried in repositories with GitHub Actions.
44. As the maintainer, I want each pilot to record the audit result, the
    manual changes needed and feedback on the number and readability of
    files, so that the contract is adjusted from evidence.
45. As an adopter, I want any contract change the pilot causes released
    with the right semver bump, so that my declared version keeps its
    meaning.
46. As the owner of the pilot repository, I want the pilot's changes on a
    branch that I decide about, so that nothing lands in my repository
    without me.

### Real-session checks

47. As the maintainer, I want the four open checks of spec 0053 run in
    real Claude Code sessions on Windows and Linux, so that the hook
    behaviour the contract depends on is confirmed, not assumed.

## Implementation Decisions

### Releases (#107)

- **Two builds, one release.** goreleaser gains a second build, `vibe-guard`,
  from `cmd/vibe-guard`, with the same targets (linux, darwin and windows
  × amd64 and arm64), `CGO_ENABLED=0`, and the version stamped the same
  way. Each binary gets its own archive (`vibe_<version>_<os>_<arch>` and
  `vibe-guard_<version>_<os>_<arch>`, tar.gz, zip on Windows). One
  `checksums.txt` lists all 24 archives.
- **Published, not drafted.** `release.draft: true` is removed. A tag push
  publishes the release. `prerelease: auto` stays, so an `-alpha.N` tag
  is still marked "Pre-release" (ADR 0007). ADR 0007's Consequences
  section relies on the draft step. It gets a dated amendment that says
  publishing is now automatic and the pre-release suffix alone keeps
  previews out of "Latest release".
- **Provenance.** The release job gets `id-token: write` and
  `attestations: write`, and attests every archive, every built binary
  and `checksums.txt` with GitHub's build-provenance attestation action,
  pinned to a commit SHA. The goreleaser version is pinned exactly,
  instead of `~> v2`.
- **Release verification job.** It runs after the release job in the
  release workflow. It runs the setup action from the tagged commit on
  Linux, macOS and Windows, on every architecture GitHub hosts for this
  public repository. For each runner it checks that `vibe --version` and
  `vibe-guard version` print the tag. It also downloads all 24 archives
  and runs `gh attestation verify` on each archive and each binary inside.
  It runs `go install …/cmd/vibe-guard@<tag>` and `go install
  …/cmd/vibe@<tag>` once on Linux, which proves that the hook's install
  hint now works.
- **Snapshot on pull requests.** A job in this repository's CI builds
  every target with goreleaser in snapshot mode, without publishing. A
  missing build or a broken configuration fails the pull request.
- **The first release with `vibe-guard`** is cut during this phase as
  `v0.12.0-alpha.1`, from the integration branch's head once #107 is
  implemented. The pilot and the release verification job need it. The
  phase merges with a merge commit, so the tagged commit stays in
  `main`'s history. Pushing a tag publishes a release, so the
  implementer asks the user before each tag push. Later fixes in this
  phase may need `-alpha.2`.

### The setup action (#107)

- **Where.** A composite action at the repository root, used as
  `Manual-debuger/VibeConform@<commit SHA>`. Its `uses:` steps are pinned
  to SHAs (C-02). Its scripts run with `shell: bash`, which every
  GitHub-hosted runner has.
- **Inputs.**
  - `version` (required): an exact release tag such as
    `v0.12.0-alpha.1`. The action refuses `latest`, an empty value and
    any value that is not semver. A Go pseudo-version is accepted (see
    `source`).
  - `tools`: `vibe vibe-guard` by default. Either can be left out.
  - `source`: `auto` (default), `release`, `go-install` or `workspace`.
    - `release` downloads the archives and `checksums.txt` for the
      runner's OS and architecture, and checks each archive's SHA-256
      against `checksums.txt` before it unpacks anything. A mismatch or
      a missing entry fails the step.
    - `go-install` sets up Go inside the action (pinned) and runs `go
      install …/cmd/<tool>@<version>`. This is the "build from source"
      option for teams that do not allow unreviewed binaries.
    - `workspace` sets up Go from the checked-out repository's `go.mod`
      and builds both tools from the checkout. This is ADR 0008's
      self-hosting case.
    - `auto` chooses `workspace` when the checkout's `go.mod` declares
      this module and contains `cmd/vibe`. It chooses `go-install` when
      `version` is a pseudo-version, because no release exists for one.
      Otherwise it chooses `release`.
  - `verify-attestation`: `false` by default. When it is `true` and the
    source is `release`, the action also runs `gh attestation verify` on
    each archive with the job's token.
- **Effect.** The binaries go into a directory under `RUNNER_TEMP`, which
  is added to `PATH`. The action never runs `vibe audit` itself.
  Therefore C-04's "the setup action followed by it" and R-01's single
  exception for the audit job keep their meaning: the audit is the
  job's own `run:` step.
- **Only for the conformance job.** The docs say so. `verify` must not
  need `vibe` (R-01).

### The examples' CI and the conformance template (#107)

Chosen with the user: the existing examples are converted, and no new
example is added.

- **The conformance template** (the old system's `conformance.yml`,
  GitHub variant) changes as follows:
  - setup-go, the Task install and the self-hosting build step are
    removed;
  - one step reads `vibe_version` from `.vibe/state.yaml`;
  - the setup action installs that version (`source: auto`);
  - the job runs `vibe audit --repo-root .` directly.

  The verdict still cannot change without a change to the repository
  (spec 0022), because the recorded version is installed exactly. The
  action is pinned by SHA through the existing pins mechanism (spec
  0044). This repository renders the same file, and `auto` resolves to
  `workspace` here. The GitLab variant is unchanged, because contract
  v1 covers GitHub Actions only.
- **The TypeScript and Python CI templates** install Task with Task's
  official setup action, pinned by SHA, instead of setup-go and `go
  install`. The monorepo example has a Go component, so its CI keeps
  setup-go for that component only.
- **What "no setup-go" means.** The examples' workflow files contain no
  setup-go step. An example synced by a source build records a
  pseudo-version, so its conformance job builds `vibe` through the
  action's `go-install` path, which sets up Go inside the action. An
  example synced by a release downloads binaries and needs no Go. The
  docs say this plainly.
- `vibe sync` regenerates the examples and this repository.
  `.vibe/state.yaml` is committed with them. The template tests and
  goldens are updated.
- This repository's hand-maintained `contract-audit.yml` keeps building
  from source. #111 decides its final shape.

### The install hint

- The PreToolUse reference command (spec 0053, "The reference hook
  commands") keeps its `go install …@<version>` and adds the release
  page, so that a developer without Go can follow it: `…install it: go
  install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@<version>,
  or download it from
  https://github.com/Manual-debuger/VibeConform/releases/tag/<version>`.
  For `latest`, the link is to the releases page. The message stays on
  one line, and the command keeps exiting 2.
- The version rule stays as spec 0053 decided: a release is named as it
  is, and anything `go install` cannot fetch, or that changes per
  commit, becomes `latest`. That rule moves from the old claude module
  into `internal/guard`, beside the reference commands. Then `vibe init
  --skill` can use it without importing an old package. The old module
  calls it from there.
- This repository's settings render `@latest`. Once `v0.12.0-alpha.1`
  exists, `go install …@latest` resolves to it, because Go picks the
  newest pre-release when no release exists. The release verification
  job proves the `@<tag>` form, and the ledger records the `@latest`
  form.

### `vibe --version`

- It prints `vibe version <version> (contract <Implemented>)`, for
  example `vibe version v0.12.0-alpha.1 (contract 1.0.0)`. The skill's
  upgrade flow reads it. `vibe-guard version` is unchanged.

### The setup skill (#108)

- **Name and place.** The skill is `vibeconform-setup`. The old
  `vibeconform` skill describes the template-sync model and is still
  generated for v1 repositories until #112. A different name lets both
  exist during the transition, and #112 removes the old one. The
  glossary's "setup skill" names it.
- **Source.** A skill directory embedded in `vibe` (`go:embed`) in a new
  package, `internal/setupskill`. That package imports no old package.
  It may import `internal/guard` for the version rule and the reference
  commands. CRLF working copies would change the embedded bytes (see
  the CRLF and `go:embed` hazard). Therefore `.gitattributes` forces LF
  for the directory, and a test fails on any carriage return in an
  embedded file.
- **Layout.**
  - `SKILL.md`: the process, at most 1500 words, written for an agent.
  - `reference/`: loaded on demand:
    - `rules.md`: one entry per rule of contract 1.0.0, saying what
      satisfies it and which reference file to start from;
    - per-area reference implementations: task runner (Taskfiles for
      Go, Python, TypeScript and a monorepo), hooks (lefthook), CI (a
      GitHub workflow that runs `verify` on Linux and Windows, the audit
      job with the setup action, Dependabot, a pull request template),
      agents (`.claude/settings.json` with the `vibe-guard` hooks,
      `policy.json` version 2, the `AGENTS.md` verification section,
      `CLAUDE.md`), and lint (golangci-lint, ruff with pyright, ESLint
      with Prettier and `tsconfig`, each as one file);
    - `optional/`: graphify, VS Code, Zed, language servers, and the
      `AGENTS.md` workflow sections (`direct`, `plan-triggered-sdd`,
      `always-sdd`) with the docs layout (Q23);
    - `migrate.md`: leaving the template-sync system;
    - `upgrade.md`: the upgrade flow.
- **"Move the templates into the skill".** The reference files are
  rendered from today's module templates. Each `*.base.*` / `*.local.*`
  pair is merged into one file, and `vibe`-specific content (state,
  `Taskfile.vibe.yml`, `vibe sync`) is dropped. The module templates
  stay where they are, because the old system still renders them. #112
  deletes them, which completes the move.
- **Placeholders.** The only placeholder is the `vibe` version. `vibe
  init --skill` replaces it, using the version rule above, in the
  skill's front matter, the reference hook commands and the `vibe-guard`
  install instructions. Everything else is example text that the agent
  adapts.
- **The process `SKILL.md` describes:**
  1. **Detect.** Components and languages by spec 0051's manifest rule.
     Then the existing configuration: task runner, hook manager, CI
     provider, update bot, lint and format configuration, agent
     settings, and markers of the template-sync system (`.vibe/`,
     `vibe.yaml` without `contract:`). It does not read secret files.
  2. **Migrate** first if the template-sync markers are present
     (`migrate.md`). Merge each seam pair into one file. Fold
     `Taskfile.local.yml` into `Taskfile.yml`. Remove `.vibe/`,
     `Taskfile.vibe.yml`, the old guard scripts and the managed-region
     markers. Replace the old `vibeconform` skill.
  3. **Decide the entrypoints.** Keep the team's runner and use its
     existing targets when they exist. Use Task when there is no runner.
     Write `vibe.yaml` version 2.
  4. **Satisfy the rules** category by category from `rules.md`. Keep
     anything that already satisfies a rule. Each configuration goes in
     one file, with no seam files.
  5. **Install `vibe-guard`** for the developer: download the release
     archive for the machine and check its SHA-256 against
     `checksums.txt`, or run `go install …/cmd/vibe-guard@<version>`
     when Go is present and the person prefers it (Q12).
  6. **Audit loop.** Run `vibe audit --static --json` until no static
     MUST rule fails, then `vibe audit --json`. Fix each finding by its
     `file` and `fix`. Stop when the verdict is conformant. Also stop,
     and report what is left, in these cases:
     - a finding needs a person: a tool missing on this machine, an
       unlock, a CI provider the contract does not support, a waiver;
     - two rounds in a row change no rule's result.
  7. **Report** the verdict, the files written or changed, and the
     SHOULD rules that still fail.
- **The upgrade flow** (`upgrade.md`). A person (or the agent) runs
  `vibe init --skill` with the new `vibe`. The skill compares
  `contract:` with the contract `vibe --version` names.
  - Same MAJOR: it bumps `contract:`, updates `vibe-guard` to the same
    version, and runs the audit loop.
  - New MAJOR: before it changes `.claude/settings.json` or anything
    under `.claude/hooks/`, it asks the person to run `vibe unlock` in
    their own terminal and waits. Then it makes the changes and runs the
    audit loop. The agent's next Stop locks again.
- **`AGENTS.md` and `CLAUDE.md`.** The reference `AGENTS.md` section
  names every `verify` and `verify-fast` command and says: "Do not
  weaken a test, lint or type check to make a change pass." (#83, triage
  in #97). The reference `CLAUDE.md` imports `AGENTS.md`.

### `vibe init --skill` (#108)

Chosen with the user: Phase 3 adds only `--skill`. The `vibe init` that
writes `vibe.yaml` version 2 and detects components stays in #112.

- `vibe init --skill` takes no positional arguments. It writes the skill
  to `.claude/skills/vibeconform-setup/` and
  `.agents/skills/vibeconform-setup/` under `--repo-root`. Both copies
  are byte-identical.
- It replaces every file in those two directories with the embedded
  version and deletes files there that the embedded version does not
  have. It writes nothing else, runs no sync and reads no `vibe.yaml`.
  It works with a v1 configuration, a v2 configuration or none.
- It prints each path it wrote or removed.
- `vibe init <standard> <version>` is unchanged. Passing `--skill`
  together with positional arguments is an error.
- `.claude/skills/` is not a guardrail path (A-08), so an agent may run
  it during the upgrade flow.

### The drift test (#109)

- **A checker over the skill's files.** One function takes the skill as
  a file system and returns every problem it finds. A test runs it on
  the embedded skill, which must have no problems. Fixture skills must
  each produce the problem they plant: an unknown rule ID, an unknown
  command, an unknown flag, a missing reference file, and a rule with
  no guidance. It runs in `go test`, so CI's test job runs it.
- **What it checks.**
  - Every rule ID (the `[VHCADR]-NN` form) exists in the rule registry
    of the contract version `vibe` implements.
  - Every guard rule ID in code spans is one of the ten contract IDs.
  - Every `vibe …` and `vibe-guard …` command in a code span or code
    block names a real subcommand, and every flag after it is a real
    flag of that subcommand. `vibe` is checked against the real command
    tree, and `vibe-guard` against its real command set.
  - Every path into the skill (`reference/…`) exists.
  - Every repository file the skill names is either a file the contract
    reads or a file a reference implementation creates.
  - Every reference `vibe.yaml` loads as a valid version 2 configuration.
  - Every reference `policy.json` loads as a valid version 2 policy.
  - The reference PreToolUse and Stop commands equal the reference
    guard's commands for the placeholder version.
  - `rules.md` has an entry for every rule, MUST and SHOULD.
- **The reference implementations pass the static audit.** For Go,
  Python, TypeScript and a monorepo, a test assembles a fixture
  repository from the reference files around a minimal component with
  one test. The test checks that `vibe audit --static --json` has no
  failing MUST rule. The static audit runs no tool, so no tool gate is
  needed.
- The checker is a new seam. The `vibe` command cannot show what a
  skill's text names, and a checker over a file system is the highest
  point where planted fixtures can make the failure visible.

### The pilot (#110)

Chosen with the user: the pilot is one real repository plus the
examples as stand-ins. The report is committed in the phase PR, and the
PR waits for it.

- **P1, real team repository:** `meeting-summarizer`, under WSL
  (Linux). It is a monorepo with Go, Python and TypeScript components
  and GitLab CI, set up by the template-sync system at v0.9.0
  (`prod-mono`). The skill takes the migration path. The repository
  uses GitLab CI, and contract v1 decides C rules for GitHub Actions
  only (Q19). Its verdict is therefore expected to be "cannot decide"
  for the C rules. That is a pilot result: the report records it with
  every other rule's result, and it is evidence for #74. The work
  happens on a branch in that repository. Nothing is pushed or merged
  there without the user.
- **P2, stand-in, migration:** `examples/typescript`, copied without
  history into a fresh repository outside this one, so that it keeps
  its GitHub Actions configuration. The skill migrates it.
- **P3, stand-in, from scratch:** the source of `examples/python`
  (package, tests and `pyproject.toml`, with no tooling), copied into a
  fresh repository. The skill sets it up from scratch. This is #108's
  "clean repository" done-when.
- **The upgrade run.** After P2 or P3 passes, `vibe` is rebuilt from a
  later commit of the integration branch. `vibe init --skill` and the
  upgrade flow run, and the audit must still pass. This is #108's
  upgrade done-when for the same-MAJOR path. No contract 2.0.0 exists,
  so the MAJOR path is checked only by the drift test, which checks
  that `upgrade.md` names `vibe unlock`. The real MAJOR path is recorded
  as UNVERIFIED.
- **What each pilot records:** stack and platform; starting point;
  verdict and each rule's result; the audit rounds the skill needed;
  the manual changes a person made after the skill stopped, counted and
  described; the files written, changed and removed, with their count;
  the user's feedback on the number of files and their readability; and
  the time taken.
- **The report** is committed as `docs/reports/contract-v1-pilot.md`
  and linked from this spec. Each finding in it is classified as a
  contract change, a skill change or no change.
- **Contract adjustments.** A contract change goes into spec 0051 as a
  new version (MAJOR, MINOR or PATCH by spec 0051's rules), together
  with the matching change to `internal/contract` and its tests, in the
  phase PR. A skill change is made in the skill. A change too large for
  this phase becomes a ticket, and the report names it.

### Real-session checks (carried over from spec 0053)

- **Windows**, in this repository's own Claude Code sessions during the
  implementation:
  - the Stop hook blocks once with a failing `verify:fast`, then lets
    the stop through;
  - after the user runs `vibe unlock` in their own terminal, an edit of
    `.claude/settings.json` is allowed, and after the next Stop it is
    refused again;
  - a `MultiEdit` call is or is not seen by a `Write|Edit` matcher. If
    the Claude Code version in use does not offer `MultiEdit`, that is
    the recorded result.
- **Linux**, in P1 under WSL, after the skill installs the `vibe-guard`
  hooks there: the same three checks. For the hook shell, a temporary
  SessionStart hook in the personal `.claude/settings.local.json` (not
  a guardrail path) writes `$0` and `/proc/$$/exe` to a file outside
  the repository. The hook is removed afterwards.
- **macOS**: no machine is available. The four checks stay UNVERIFIED
  on macOS, and the record says so.
- Each result goes into this spec's verification record and the PR's
  ledger.

### Documentation

- `docs/usage.md`:
  - installing `vibe` and `vibe-guard` from a release (download, check
    the checksum, `gh attestation verify`);
  - building from source (`go install …@<tag>` for both, or `go build`
    from a checkout);
  - the setup action's inputs and the conformance job;
  - `vibe init --skill` and the setup skill;
  - what "no setup-go" means for the examples.
- `GLOSSARY.md`:
  - "Setup skill" names `vibeconform-setup` and `vibe init --skill`;
  - a new term, "setup action": the action that installs `vibe` and
    `vibe-guard` in the conformance job.
- `docs/architecture/overview.md`:
  - `internal/setupskill` and its imports (`internal/guard` only, no
    old package);
  - the setup action;
  - the release with two binaries.
- ADR 0007: the dated amendment above.
- No new ADR. There is no new production Go dependency. The setup
  action's third-party actions and the Task setup action are pinned CI
  dependencies, covered by spec 0044 and C-02.

## Testing Decisions

Good tests check behaviour through the highest seam available and use
real files and processes. No test reads a private helper.

- **Seam 1: the `vibe` command, in-process** (as in Phases 1 and 2).
  `vibe init --skill` is tested against a fixture repository:
  - the files written to both paths, byte-identical;
  - the version placeholder replaced;
  - stale files removed;
  - nothing written outside the two directories (hash of the rest of
    the tree before and after);
  - `--skill` with positional arguments is an error.

  `vibe --version` is tested through the same seam.
- **Seam 2: the drift checker** (above). One test runs it on the
  embedded skill. One fixture per problem kind must fail. The static
  audit test assembles fixture repositories from reference files and
  runs the `vibe audit --static --json` seam on them.
- **Embedded bytes.** A test fails on any carriage return in the
  embedded skill. The prior art is the vibeskill tests, which pin
  content against CRLF.
- **Templates.** The conformance and CI template tests render the
  GitHub variants and assert:
  - no setup-go in the TypeScript and Python CI;
  - no setup-go in any conformance file;
  - the setup action pinned by a full SHA;
  - the GitLab variant unchanged.

  Goldens are updated, and `task audit` passes after `vibe sync`.
- **The install hint and the version rule.** `internal/guard`'s tests
  cover the version rule (moved with `TestVibeGuardInstallVersion`'s
  cases) and the new hint text. A-03 still refuses every deny case
  through the reference command with `vibe-guard` missing.
- **Release configuration.** The PR job's goreleaser snapshot build
  produces all 24 archives and `checksums.txt`. actionlint covers the
  release and action workflows, as `task workflows:lint` does today.
- **The setup action.** A pull-request workflow runs the action on
  Linux, macOS and Windows:
  - against `v0.12.0-alpha.1` (`source: release`);
  - with `verify-attestation: true`;
  - with `source: go-install`;
  - with `source: auto` in this checkout, which resolves to
    `workspace`;
  - with a tampered checksum, which must fail;
  - with `version: latest`, which must fail.

  Before the first release exists, the release cases cannot run. They
  are recorded as UNVERIFIED until the tag is pushed.
- **Real integration** is listed separately in the ledger:
  - the release verification job on `v0.12.0-alpha.1`;
  - `gh attestation verify` for every binary;
  - `go install …@latest` and `…@<tag>`;
  - the setup action workflow;
  - the "Contract / audit" job;
  - the pilots;
  - the real-session checks.
- **The skill itself** is not tested by an automated LLM run. A run is
  not deterministic, and a gate must give the same verdict twice (ADR
  0025, "Alternatives considered"). Its evidence is the pilots, P3 from
  scratch and the upgrade run, recorded in the report.

## Out of Scope

- Making the full audit a required check, renaming `vibe.v2.yaml`, and
  freezing this repository's generated files (#111).
- Removing the template-sync system, the old `vibeconform` skill, the
  module templates and the old guards. Also the `vibe init` that writes
  `vibe.yaml` version 2 and detects components (#112).
- GitLab CI in the C rules (#74) and Codex in the A rules (#50). P1's
  GitLab result is recorded as evidence only.
- The GitLab conformance template's install path.
- The monorepo PostToolUse hook that checks only the edited component
  (#81) and graphify's conflicting guidance (#86). The skill's optional
  content may address them. If it does not, they stay open.
- Signing binaries with a key, as opposed to attestations. Package
  managers (Homebrew, Scoop, apt).
- An automated LLM run of the skill in CI.
- Real-session checks on macOS.

## Further Notes

- **Where this spec departs from the tickets:**
  - #107 says the setup action is "used only by the conformance job".
    This repository's PR workflow and release verification job also run
    it, but only to test it.
  - #107's "the example repository's CI has no setup-go" is met by the
    TypeScript and Python examples. The monorepo keeps setup-go for its
    Go component. For a pseudo-version, the action itself sets up Go
    (see "The examples' CI").
  - #108 says the templates "move" into the skill. Here they are copied
    and merged, and #112's deletion completes the move.
  - #108 says `vibe init` writes the skill. Here only `vibe init
    --skill` does, by the user's choice. #112 adds the v2 `vibe init`.
- **Interpretations beyond the tickets**, approved with this spec:
  - the skill name `vibeconform-setup`;
  - the migration path from the template-sync system ("detect the
    existing configuration");
  - the audit loop's stop conditions;
  - `vibe --version` naming the contract;
  - the hint naming the release page;
  - the release cut from the integration branch;
  - the setup action's `source` and `verify-attestation` inputs;
  - the drift test's static-audit check of the reference
    implementations.
- **Hidden assumptions to verify in implementation:**
  - Task's official setup action exists and can be pinned by SHA. If
    not, Task is installed from its release archive with a checksum
    check.
  - `gh attestation verify` works for this public repository with the
    default job token.
  - GitHub hosts arm64 runners for Linux, macOS and Windows for this
    repository. An architecture with no runner is verified by checksum
    and attestation only.
  - `go install …@latest` picks the newest pre-release when no release
    exists.
  - Claude Code loads reference files from a skill directory under
    `.claude/skills/`, and Codex reads `.agents/skills/` (spec 0041).

  Each is checked, and this spec is corrected where it is wrong.
- **Running time.** The pilot needs the user's time: approving the
  branch in P1, feedback on readability, and `vibe unlock` for the
  session checks. The PR waits for the report.

## Acceptance Criteria

1. A tag push publishes a release with `vibe` and `vibe-guard` archives
   for linux, darwin and windows × amd64 and arm64, and `checksums.txt`
   (#107).
2. `gh attestation verify` verifies every released archive and binary of
   `v0.12.0-alpha.1`, as run by the release verification job (#107).
3. `go install …/cmd/vibe-guard@v0.12.0-alpha.1` succeeds, and the
   reference hook's install hint names the release page (#107, spec 0053
   carry-over).
4. The setup action installs both tools from a release with checksums
   checked, refuses `latest` and a tampered checksum, and supports
   `go-install` and `workspace` (#107).
5. The TypeScript and Python examples' workflows contain no setup-go.
   Every conformance workflow uses the setup action, pinned by SHA
   (#107).
6. `docs/usage.md` describes installing from a release, verifying, and
   building from source (#107).
7. `vibe init --skill` writes `vibeconform-setup` to both skill paths,
   overwrites and prunes it, and writes nothing else (#108).
8. The skill holds the reference implementations, migration and upgrade
   flows, the optional content, and an `AGENTS.md` section that names
   the verify entrypoints and the "do not weaken" rule (#108).
9. P3 (from scratch) ends with a conformant audit. The upgrade run ends
   with a conformant audit (#108).
10. The drift test fails CI for an unknown rule ID, command, flag or skill
    file, and for a rule with no guidance. It passes on the real skill
    (#109).
11. The reference implementations for Go, Python, TypeScript and a
    monorepo pass `vibe audit --static` with no failing MUST rule (#109).
12. `docs/reports/contract-v1-pilot.md` records P1, P2 and P3. Every
    contract change it caused is in spec 0051 with a semver bump (#110).
13. The four real-session checks are recorded for Windows and Linux as
    PASS, FAIL or UNVERIFIED, and macOS is recorded as UNVERIFIED.
14. `GLOSSARY.md`, `docs/usage.md`, `docs/architecture/overview.md` and
    ADR 0007 are updated.
15. `task verify` and `task audit` pass. The "Contract / audit" job
    reports its verdict on the phase PR.
