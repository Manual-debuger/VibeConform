# VibeConform Architecture Overview

Status: living document. This is the authoritative technical description of
VibeConform's design; Notion (or any other project tracker) holds roadmap
and status, never competing architecture.

The design principles every change is held to — mechanical rules over
prompts, Windows/Linux parity, and tooling that stands on third-party
tools rather than `vibe` — are in `docs/architecture/principles.md`.

## Conceptual model

```text
contract (versioned rules: ID, level, check)
    ↓ states
setup skill ──writes──→ adopting repository ←──verifies── vibe audit
(vibeconform-setup)     (vibe.yaml version 2,          (static checks,
                         its own tooling)               execution checks,
                                                        probes)
```

ADR 0025 settled the model, and spec 0059 removed the template-sync system
it replaced. Three roles:

- **The contract** (spec 0051) is a versioned list of rules a repository
  must satisfy. Each rule has a stable ID, a level (MUST or SHOULD) and a
  check: static (reads configuration), execution (runs a command and
  inspects its result) or probe (feeds a known input and checks the
  reaction). A rule names a property, never a tool or a file's content.
- **The setup skill** gives a repository those properties in whatever way
  fits it, and runs the audit until it passes. `vibe init --skill` writes
  it into the repository, which owns it and everything it writes.
- **`vibe audit`** only verifies, deterministically, and never writes a
  file.

`vibe.yaml` version 2 is the repository's declaration: the contract
version it follows, its platforms, agents and components, the commands
of its entrypoints (`verify`, `verify-fast`, `fmt`, `fmt-check`, and
optionally `setup`), and waivers for SHOULD rules. The audit does not
depend on a task runner: any command qualifies as an entrypoint. `vibe
init` writes a first `vibe.yaml`; the repository edits it by hand.

## Contract audit

The audit lives in `internal/contract` (spec 0052 for the static checks,
spec 0053 for execution checks and probes, spec 0055 for GitLab CI):

```text
                 vibe audit (internal/cli)
                          ↓
internal/contract
  vibe.yaml version 2 (a version 1 file is detected and refused)
  the rules of contracts 1.2.0 and 2.1.0
  static checks of tracked files
  execution checks and probes in a probe copy
  report model (plain data)
```

- **Configuration.** `contract.Load` reads `vibe.yaml` once, opens the
  repository and parses (spec 0065); `vibe audit` and `vibe doctor` call
  it. Its read-and-detect step, `contract.ReadConfig`, tells version 2
  (`contract:`) from version 1 (`standard:`) with `contract.DetectSchema`,
  and `vibe init` uses it alone. A missing file (`contract.ErrNoConfig`)
  and a version 1 file (`contract.ErrConfigV1`) are exit 1; the CLI adds
  the pointer (`vibe init`, or the migration guide). `vibe init --skill`
  writes the setup skill and reads no configuration.
- **Language records** (spec 0066). Each component language is one
  record in a source file of its own (`internal/contract/language_*.go`):
  its identifier and display name, its manifest and dependency reader,
  the files detection wants beside the manifest, its lockfiles with
  their check and fix commands, its test file patterns, its planted
  defect samples and complexity layout, and its Dependabot ecosystems
  and Renovate managers. `languageRegistry` lists the records in the
  contract's language order (Go, Python, TypeScript). The valid
  `language` values, the root manifests, D-01's lockfile order and check
  text, and the order of the `--explain` samples are derived from it.
  An internal test fails when a record leaves a fact empty. V-05's
  Python filter is the one language check outside the records, because
  V-05 is a Python rule in the contract.
- **Component detection** (spec 0066). `contract.DetectComponents`
  reads the tracked file list and returns the components, the nested
  manifests and whether the audit's implicit root detection finds the
  same components. The depth limit (`contract.DetectionDepth`), the
  skipped dependency directories, the priority between manifests and the
  manifest phrase (`contract.ManifestPhrase`) are the contract's. The
  audit's root detection and that answer use one function.
- **Dependency direction.** The CLI imports `internal/contract`, never
  the reverse, and renders its report model as text or JSON
  (`internal/cli/contract_*.go`). `internal/contract/imports_test.go`
  keeps the removed template-sync package names out of its imports.
- **Test seam** (ADR 0028, spec 0065). `internal/contract/contracttest`
  builds fixture repositories from the trees in
  `internal/contract/testdata`, opens them without git when every file
  is tracked, and runs `contract.Load` and `contract.Audit` with a check
  on every report: each failed part has a finding, and each finding
  names a file and a fix. The tests of `internal/contract` and
  `internal/cli` import it. Among this module's packages it imports only
  `internal/contract` and `internal/testkit`, and only test files import
  it; two `depguard` rules enforce both. The report goldens stay in `internal/cli/testdata`.
- **Test Git repositories** (spec 0073). `internal/testkit` is the one
  helper for a test's isolated temporary Git repository: git runs with
  the person's global and system configuration ignored, none of the
  person's `GIT_` variables and no repository above the temporary
  directory, and the helper inits a repository with one commit and
  names its common Git directory. Every test that runs git in a
  repository of its own uses it, and so does `contracttest`'s fixture
  builder for its Git fixtures. Only test files and `contracttest`
  (itself imported by test files only) import `internal/testkit`, and it
  imports no package of this module; two `depguard` rules enforce both,
  as for `contracttest`.
- **Shell parsing.** "Called" means a simple command in the shell parse
  tree, so `internal/contract` depends on `mvdan.cc/sh/v3/syntax` (ADR
  0026). `internal/contract/shell.go` is the only file that imports it;
  every check sees the package's own `shellCommand` and `shellWord`. Its
  other third-party imports are ones `vibe` already had:
  `gopkg.in/yaml.v3`, and `github.com/tailscale/hujson` for Renovate's
  JSON5 files.
- **Read-only, tracked files only.** Static checks read the working-tree
  content of files `git ls-files` lists and never write. Since spec 0053,
  execution checks and probes run in a *probe copy*: a fresh Git
  repository in the system's temporary directory, holding the same
  files, reset between runs and deleted at the end. Entrypoints run
  without a shell, hooks through `bash -c`, each with a time limit.
  Under contract 2.0.0, A-02 and A-09 rewrite `.claude/hooks/policy.json`
  in the probe copy without the exceptions a deny case matches, run the
  case again, and write the original back (spec 0072, "The rerun").
  `vibe audit` always lists the tracked files with git (`OpenRepo`); the
  CLI's tests hand a fixture's known file list in through a repository
  seam (`contract.NewRepo`), so a rule-table case starts no git (spec
  0056).
- **Check text from parts** (spec 0073). A rule's check text,
  `Rule.Check()`, is derived: each part's `Lead` ("Static: ",
  "Probe: ", "Static, " and so on, as spec 0051 writes it) and its
  `Check`, joined in order; an empty lead continues the part before it.
  `--explain` prints a part's `Check` without its lead. `registry_test.go`
  compares each rule's derived text with spec 0051's row and spec
  0061's text.
- **One last part** (spec 0071, which replaced spec 0064's needs). The
  runner decides the static parts first. Then, in the probe copy, it
  runs the execution and probe parts that have `Run`, in registry order
  and in part order inside a rule. Then it runs the one *last part*, the
  part that has `RunLast`. The last part gets every earlier execution
  and probe result, and the `vibe` calls of setup, as an argument
  (`Earlier`). R-01's execution part is the last part: it reads A-02's
  deny verdict and every `vibe` call from `Earlier`, so each call is
  reported once. Before it builds the probe copy, also with `--static`,
  the runner checks the rule list. It panics, and names the part, when
  two parts are last parts, when a static part is the last part, or when
  a part sets both or neither of `Run` and `RunLast`; a registry test
  keeps `contract.Rules()` valid. Results come back through return
  values (spec 0064): a part returns all of its output in its
  `PartResult`, and the runner keeps the results of one audit in run
  order, each with the `vibe` calls the shim recorded while that part
  ran, where `Env` cannot reach them. The report's guard exception uses
  come from A-02's and A-09's results, in run order. A check reads `Env`
  and adds no record to it except through `note`.
- **One CI model, two readers** (spec 0055). The C rules, D-03 and R-01
  read the repository's CI through one provider-neutral model
  (`ci_model.go`): the CI provider read, its jobs with their commands
  and scripts, the includes not followed, and for each *pipeline kind*
  (merge request, push to the default branch) the jobs that run and why
  the others do not. `env.pickCI()` picks the provider once per audit:
  GitHub Actions when tracked workflows exist, otherwise GitLab CI when
  `.gitlab-ci.yml` is tracked. Each reader fills the whole model when it
  reads the CI (spec 0067): the runs of both pipeline kinds, each job's
  runner platforms, and the references C-02 checks (GitHub `uses:`,
  GitLab images and includes). The GitHub reader (`githubCI` in
  `ci_model.go`) wraps the existing workflow parsing and decides its
  runs with the trigger walk (`workflow_triggers.go`). The GitLab reader
  (`gitlab_ci.go`) resolves `.gitlab-ci.yml` statically: anchors, local
  includes, `default:`, `extends:`, `!reference` and child pipelines the
  parent waits for. `gitlab_rules.go` evaluates `workflow:rules`,
  `rules:` and `only`/`except` over a closed list of conditions. The
  model's `Run` is a lookup for both providers. C-01, C-02, C-04, C-05
  and D-03 each have one implementation (`checks_ci.go`) that reads only
  the model, and no C check compares the provider. The words of a
  finding that differ by provider ("pull requests" or "merge request
  pipelines", the trigger hint, some messages and fixes) come from the
  provider's wording (`ci_wording.go`), which the reader sets, so each
  provider keeps its text. C-03 reads the GitHub workflows (the model's
  `Workflows`), which only the GitHub reader fills; the registry makes
  it not applicable on GitLab CI. A new provider needs a reader and a
  wording; those five rules need no change. Both readers live in
  `internal/contract` and add no import: they use `gopkg.in/yaml.v3`,
  the package's shell parser and `internal/guardrule`, which the package
  already imported, so the dependency direction above is unchanged. The
  report model carries the provider read, which the CLI prints as
  `CI provider read:` and `ci_provider`.

### The reference guard

Spec 0053 adds the guard the contract's guard probes are written
against, and the rule IDs both share:

```text
cmd/vibe     → internal/cli → internal/contract ─┐
                           └─────────────────────┤ (vibe unlock: the marker
                                                 │  and where it goes)
cmd/vibe-guard ──────────────→ internal/guard ───┤
                                                 ↓
                                        internal/guardrule
                                 (rule IDs, policy.json v2, the unlock
                                  marker and policy file names, command
                                  parsing: mvdan.cc/sh for Bash, a
                                  PowerShell tokenizer, wrappers)
```

- **`internal/guardrule`** holds what the audit and the guard must agree
  on: the contract guard rule IDs and the guard-internal ones,
  `policy.json` version 2 parsing, exception matching (a `Rule` and an
  `Exception` have unexported fields and are built only by `NewRule` and
  `NewException`, which refuse an invalid one and parse a command match
  once per shell and compile a glob once; `ParsePolicy` builds through
  them, spec 0073), the kinds as typed values (a rule's `RuleKind`,
  `CommandRule` or `PathRule`, with one pattern; a `Target`'s
  `TargetKind`, built by `NewCommandTarget` and `NewPathTarget`; a
  PowerShell writer's kind; spec 0073), the policy file
  paths, the unlock marker's name and format (`vibe unlock` writes it,
  `vibe-guard` reads it, the audit plants it), the finder of the Git top
  and the common Git directory that holds the marker (`FindRepo`, which
  reads `.git` files and runs no `git`; `vibe-guard` and `vibe unlock`
  both call it, spec 0069), and the decomposition of a Bash or
  PowerShell command into simple commands: `wrappers.go` unwraps the
  commands a wrapper runs (spec 0053), and `writes.go` reads the files a
  command writes (spec 0058). It imports no `internal/` package.
- **`internal/guard`** is `vibe-guard`'s logic: the baseline rules, the
  decision, observe mode, the unlock marker, `hook`, `explain`, `relock`,
  and the reference hook commands. Its `Guard` keeps its state
  unexported: `New(repo, Options{Now, Dir})` loads the policy, and one
  `Decide(Call)` decides a target or a payload error, applies a policy
  error and observe mode in one place, and returns a `Decision` whose
  `Refuses` gives `hook` and `explain` their exit code (spec 0070).
  It imports `internal/guardrule` and nothing from `internal/contract`.
  `cmd/vibe-guard`
  is a thin `main` over it, a separate binary that opens no network
  connection.
- **`internal/contract`** imports `internal/guardrule`, never
  `internal/guard`: the audit checks a guard as a black box through the
  hooks in `.claude/settings.json`. Its guard corpus is embedded in
  `vibe`; `internal/guard`'s tests run the same corpus through
  `vibe-guard`.
- **`internal/guard`'s tests run `guard.Main` in-process** with an
  `Env` (stdin, captured output, the variables the test sets, the
  fixture's directory, a fixed clock), the corpus included, under
  `go test -short` too. Only the reference-hook tests and the binary set
  (one deny, allow and `ask` payload, one `explain`, `version`) run the
  built `vibe-guard` as a subprocess (spec 0070).
- The dependency is one way: `internal/contract` → `internal/guardrule`
  ← `internal/guard`. `task lint` enforces it for non-test code with
  `depguard` (`.golangci.yml`, spec 0061), and tests in each package
  check their own imports.
  `internal/setupskill` imports `internal/guard` for the reference
  commands and the version rule (`InstallVersion`: the version the
  install hint names). The `internal/setupskill` and `internal/doctor`
  boundaries have `depguard` rules too, and no import tests (spec 0063).
- **One way to find bash** (spec 0073). `guardrule.ResolveBash` finds
  the bash Claude Code runs hooks with: Git Bash on Windows
  (`CLAUDE_CODE_GIT_BASH_PATH`, or the `bash.exe` next to `git.exe`,
  never WSL's), bash on `PATH` elsewhere. The audit, `vibe doctor` and
  the tests that run hooks (`hookShell` in `internal/setupskill`,
  `findBash` in `internal/guard`) all call it; each test keeps its own
  skip policy. `guardrule.LookPath`, which it uses, is also how the
  audit finds the programs it runs.

### The setup skill

Spec 0054 adds `internal/setupskill`: the `vibeconform-setup` skill
directory, embedded with `go:embed`, and `Install`, which `vibe init
--skill` calls to write it to `.claude/skills/` and `.agents/skills/`.

```text
cmd/vibe → internal/cli ──→ internal/setupskill ──→ internal/guard
           (vibe init --skill)   (embedded skill/)    (InstallVersion,
                                                       ReleasePage,
                                                       reference commands)
```

- **Imports.** Among this repository's packages, `internal/setupskill`
  imports only `internal/guard`, for the version rule and the release
  page that fill the skill's two placeholders: `{{VIBE_VERSION}}`, the
  version the install hint names (`InstallVersion`), and
  `{{VIBE_RELEASE_PAGE}}`, its release page (`ReleasePage`, which the
  reference PreToolUse command uses too; spec 0073). A `depguard` rule
  in `.golangci.yml` enforces that for non-test code (spec 0063). The
  drift checker fails on any other `{{VIBE_…}}` in the skill.
- **Embedded bytes.** `.gitattributes` forces LF for
  `internal/setupskill/skill/`, and a test fails on any carriage return
  in an embedded file, so a CRLF working copy cannot change what a build
  ships. `FS()` builds the skill's file system once, and `Dirs()`
  returns a new slice on each call, so no caller can change the
  package's state (spec 0073).
- **Reference files.** The skill's reference implementations
  (`skill/reference/`) are single files per stack, maintained here; they
  began as copies of the template-sync system's templates, which spec
  0059 deleted. The reference hook commands in its `settings.json` equal
  `internal/guard`'s.
- **Reference hook tests** (spec 0059). The tests that run the reference
  hooks sit beside the skill, in `internal/setupskill/*_test.go`, and
  import only `setupskill`, `guard` and `contract`: `hook:check`'s path
  filter and its checks, the Stop gate and the green stamp (including a
  project in a subdirectory), the Go test modes, the CI pins, and the
  hooks run from any directory (spec 0058).
- **Placement manifest** (spec 0073). `placement.go` says where a
  repository built from the references places each reference file:
  `Stacks`, `Providers`, `Place(stack, provider, ref)` and the
  reference monorepo's `MonorepoComponents` (directory, language and
  Taskfile include ID). The drift checker, the static-audit test and
  the hooks test read it; none keeps its own copy.
- **The drift test** (#109) is test code in `internal/cli`
  (`skill_drift_checker_test.go`), not a package: it checks the
  skill's `vibe` commands and flags against the real command tree, which
  only `internal/cli` builds, and loads the reference `vibe.yaml` files
  through `contract.Parse` in a Git work tree. One function reads a
  skill as a file system and returns every problem: rule IDs, guard rule
  IDs, `vibe` and `vibe-guard` commands and flags (`vibe-guard`'s from
  the command table `guard.Commands`, which also generates its usage),
  paths into the skill, repository files named (against
  `contract.RepoPaths`, the paths the rules read, guardrule's policy
  files and the placement manifest; spec 0073), the
  reference `vibe.yaml`, `policy.json` and hook commands, a `rules.md`
  entry per rule, and `vibe unlock` in `upgrade.md`. A fenced block
  marked `text` is output, not commands. A second test assembles a
  repository per stack from the reference files and requires
  `vibe audit --static --json` to fail no MUST rule.

## Affected-component graph

Adopted from [Nx](https://nx.dev/), and not built:

```text
changed files
    ↓
owning components
    ↓
reverse dependency closure
    ↓
affected components
    ↓
applicable validation tasks
```

`vibe.yaml` version 2 names components (`path`, `language`), and the
audit plants its defects per component, but there is no dependency graph
between them and nothing that narrows verification to the affected ones.
Each tool's own cache (`go test`, golangci-lint, ESLint) is what makes
`verify:fast` incremental today (`docs/usage.md`, "`verify:fast` and the
tool caches"). If affected-component validation is built, it must fall
back to the full `verify` whenever it cannot establish the narrower scope
safely: it may never report success having silently skipped checks.

## Repository intelligence and Skills Manager boundaries

VibeConform may orchestrate, but not absorb, two adjacent systems:

- **Repository intelligence** (such as GitNexus): index, search,
  dependency, caller, impact and trace queries. It must never replace the
  compiler, LSP, linter, tests, or CI; it augments them. The setup skill
  offers no repository-intelligence content (spec 0060); no rule checks
  it, and `verify` and CI never consult a graph.
- **Skills Manager**: canonical skill storage, deployment, and per-project
  activation. It must never become a CI dependency.

## Internal package layout

```text
cmd/
  vibe/                     # the CLI's main
  vibe-guard/               # the reference agent guard's main (spec 0053)
internal/
  cli/                      # command tree: init, audit, doctor, unlock; hidden stubs for the removed sync, diff and check
  contract/                 # contract audit: vibe.yaml v2, rule registry, static checks, the CI model with its GitHub Actions and GitLab CI readers, probe copy and probes (spec 0052, 0053, 0055)
    contracttest/           # the audit's test seam: fixture builder, listed-fixture opener, audit helper with the report check; imported by test files only (ADR 0028)
    probetest/              # the probe tests that run real tools, in a package of their own for go test's time budget (spec 0053)
  guardrule/                # guard rule IDs, policy.json v2, command parsing shared by the audit and vibe-guard, ResolveBash; imports nothing from internal/ (spec 0053, 0073)
  guard/                    # vibe-guard: baseline rules, hook/explain/relock, unlock marker, reference commands, version rule; imports only guardrule (spec 0053, 0054)
  setupskill/               # the embedded vibeconform-setup skill, vibe init --skill, and the tests that run its reference hooks; imports only guard (spec 0054)
  doctor/                   # vibe doctor's checks; imports only contract, for the configuration and hook reader, and guardrule, for bash resolution (spec 0059, 0073)
  testkit/                  # the tests' isolated temporary Git repository; imported by test files only, imports nothing from this module (spec 0073)
  baselines/                # the complexity baseline ceiling task lint runs, a main package; imports nothing from this module (spec 0073)
```

- **`vibe init`** (spec 0059) gets the components from
  `contract.DetectComponents`, which reads the tracked file list and
  shares its root rule with the audit's implicit root detection (spec
  0066). The contract owns the language records and component
  detection; init holds no language knowledge and no detection code.
  Init only renders what it gets, with the contract's manifest phrase
  in its help text and its "found no component" error, and checks the
  file it writes with `contract.Parse`, so it never writes a
  configuration `vibe audit` cannot read.
- **`vibe doctor`** loads the configuration with `contract.Load` and
  hands the outcome, a load error with the CLI's text included, to
  `doctor.Run`, which chooses the checks and their order (git,
  `vibe.yaml`, the entrypoints in `contract.EntrypointOrder`, then what
  the hooks need) and returns the report; the CLI prints it and sets
  the exit code (spec 0065). Doctor reuses the audit's readers (the
  entrypoint splitter `ParseSimpleCommand` and `ReadClaudeHooks`) and
  its bash resolution (`guardrule.ResolveBash`, spec 0073), so it finds
  programs and bash exactly as the audit would run them. Among this
  repository's packages it imports only `internal/contract` and
  `internal/guardrule`; a `depguard` rule in `.golangci.yml` enforces
  that (spec 0063).
- **The removed commands.** `vibe sync`, `vibe diff` and `vibe check` are
  hidden commands that print the migration guide's path and exit 1.
  `contract.ReadConfig` stays, so that a version 1 `vibe.yaml` gets the
  same pointer from `audit`, `doctor` and `init`.

**The release and the setup action** (spec 0054). One release holds both
binaries: `.goreleaser.yaml` builds `vibe` and `vibe-guard` for linux,
darwin and windows on amd64 and arm64, each in its own archive, with one
`checksums.txt`. A tag push publishes it (`.github/workflows/release.yml`)
and attests every archive and binary. The jobs after it install the
release on every hosted platform and verify the attestations. The root
`action.yml` is the setup action: it installs an exact version of either
tool from a release (checksums checked) or builds it from source. Its
consumer outside this repository is the audit job of the setup skill's
reference CI (`reference/ci/*/audit.yml`).

**The examples** (spec 0059). `examples/typescript`, `examples/python`
and `examples/monorepo` are assembled by hand from the setup skill's
reference files, each with a `vibe.yaml` version 2. They are the live
counterpart of references this repository cannot dogfood: a Go
repository never runs the TypeScript or Python references.
`.github/workflows/examples.yml` copies each into a fresh Git repository
and runs the full audit there on Linux and Windows, plus a GitLab variant
of the monorepo. The audit is the check; there is no drift test between
an example and its references.

Packages are created when there is real code to put in them, not in
advance.

## Canonical verification interface

Taskfile (`Taskfile.yml`) is this repository's single cross-platform entry
point for local and CI verification (`task fmt`, `task lint`, `task test`,
`task security`, `task verify`, `task audit`). CI, Git hooks, and docs
invoke these tasks rather than duplicating command lists, and `vibe.yaml`
declares them as the entrypoints. An adopting repository may use any
runner; the contract only asks that its entrypoints exist and behave.

`task lint` runs `golangci-lint`, then the complexity baseline ceiling
(`internal/baselines`, spec 0073): it counts the `//nolint:… //
baseline: pre-existing complexity` comments in the repository's Go
files and fails when the count is higher than the ceiling recorded in
`internal/baselines/main.go`, so that a new function over the
`gocyclo` or `gocognit` limit is simplified, not given a baseline. A
change that removes a baseline lowers the ceiling in the same commit;
below the ceiling the check passes and names the new one.

`task verify` and `task verify:fast` depend only on native language
tooling, never on `vibe` (spec 0017; contract rule R-01 for adopters).
`task audit` is deliberately not one of `verify`'s steps: it builds `vibe`
and `vibe-guard` from the checkout into the Git directory, puts them first
on `PATH`, and runs the full contract audit (spec 0059). CI runs the same
audit as the required check `Contract / audit`
(`.github/workflows/contract-audit.yml`), next to `CI / gate`. Its
audit step runs `vibe audit` itself, not `task audit`, because contract
rule C-04 requires a CI step that does. Both
runs read the same `vibe.yaml`, whose `require` key lists the SHOULD
rules this repository holds itself to, so they reach the same verdict
(spec 0063).
