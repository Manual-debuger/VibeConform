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
  the 25 rules of contract 1.1.0
  static checks of tracked files
  execution checks and probes in a probe copy
  report model (plain data)
```

- **Configuration.** `vibe audit` reads `vibe.yaml` once.
  `contract.DetectSchema` tells version 2 (`contract:`) from version 1
  (`standard:`); a version 1 file, and a missing one, are exit 1 with a
  pointer (the migration guide, or `vibe init`). `vibe init --skill`
  writes the setup skill and reads no configuration.
- **Dependency direction.** The CLI imports `internal/contract`, never
  the reverse, and renders its report model as text or JSON
  (`internal/cli/contract_*.go`). `internal/contract/imports_test.go`
  keeps the removed template-sync package names out of its imports.
- **Shell parsing.** "Called" means a simple command in the shell parse
  tree, so `internal/contract` depends on `mvdan.cc/sh/v3/syntax` (ADR
  0026). `internal/contract/shell.go` is the only file that imports it;
  every check sees the package's own `Command` and `Word`. Its other
  third-party imports are ones `vibe` already had: `gopkg.in/yaml.v3`,
  and `github.com/tailscale/hujson` for Renovate's JSON5 files.
- **Read-only, tracked files only.** Static checks read the working-tree
  content of files `git ls-files` lists and never write. Since spec 0053,
  execution checks and probes run in a *probe copy*: a fresh Git
  repository in the system's temporary directory, holding the same
  files, reset between runs and deleted at the end. Entrypoints run
  without a shell, hooks through `bash -c`, each with a time limit.
  `vibe audit` always lists the tracked files with git (`OpenRepo`); the
  CLI's tests hand a fixture's known file list in through a repository
  seam (`contract.NewRepo`), so a rule-table case starts no git (spec
  0056).
- **One CI model, two readers** (spec 0055). The C rules, D-03 and
  R-01 read the repository's CI through one provider-neutral model
  (`ci_model.go`): the CI provider read, its jobs with their commands
  and scripts, the includes not followed, and for each *pipeline kind*
  (merge request, push to the default branch) the jobs that run and
  why the others do not. `Env.CI()` picks the provider once per audit:
  GitHub Actions when tracked workflows exist, otherwise GitLab CI when
  `.gitlab-ci.yml` is tracked. The GitHub reader wraps the existing
  workflow parsing, and the GitHub checks keep reading the workflows
  directly so their 1.0.1 results and wording do not change. The GitLab
  reader (`gitlab_ci.go`) resolves `.gitlab-ci.yml` statically: anchors,
  local includes, `default:`, `extends:`, `!reference` and child
  pipelines the parent waits for. `gitlab_rules.go` evaluates
  `workflow:rules`, `rules:` and `only`/`except` over a closed list of
  conditions, and `gitlab_checks.go` holds the C, D-03 and R-01 checks
  on that model. Both readers live in `internal/contract` and add no
  import: they use `gopkg.in/yaml.v3`, the package's shell parser and
  `internal/guardrule`, which the package already imported, so the
  dependency direction above is unchanged. The report model carries the provider
  read, which the CLI prints as `CI provider read:` and `ci_provider`.

### The reference guard

Spec 0053 adds the guard the contract's guard probes are written
against, and the rule IDs both share:

```text
cmd/vibe     → internal/cli → internal/contract ─┐
                           └─────────────────────┤ (vibe unlock: the marker)
cmd/vibe-guard ──────────────→ internal/guard ───┤
                                                 ↓
                                        internal/guardrule
                                 (rule IDs, policy.json v2, the unlock
                                  marker and policy file names, command
                                  parsing: mvdan.cc/sh for Bash, a
                                  PowerShell tokenizer, wrappers)
```

- **`internal/guardrule`** holds what the audit and the guard must agree
  on: the ten contract guard rule IDs and the guard-internal ones,
  `policy.json` version 2 parsing, exception matching, the policy file
  paths, the unlock marker's name and format (`vibe unlock` writes it,
  `vibe-guard` reads it, the audit plants it), and the decomposition of a
  Bash or PowerShell command into simple commands. It imports no
  `internal/` package.
- **`internal/guard`** is `vibe-guard`'s logic: the baseline rules, the
  decision, observe mode, the unlock marker, `hook`, `explain`, `relock`,
  and the reference hook commands. It imports `internal/guardrule` and
  nothing from `internal/contract`. `cmd/vibe-guard`
  is a thin `main` over it, a separate binary that opens no network
  connection.
- **`internal/contract`** imports `internal/guardrule`, never
  `internal/guard`: the audit checks a guard as a black box through the
  hooks in `.claude/settings.json`. Its guard corpus is embedded in
  `vibe`; `internal/guard`'s tests run the same corpus through
  `vibe-guard`.
- The dependency is one way: `internal/contract` → `internal/guardrule`
  ← `internal/guard`, and tests in each package enforce it.
  `internal/setupskill` imports `internal/guard` for the reference
  commands and the version rule (`InstallVersion`: the version the
  install hint names).

### The setup skill

Spec 0054 adds `internal/setupskill`: the `vibeconform-setup` skill
directory, embedded with `go:embed`, and `Install`, which `vibe init
--skill` calls to write it to `.claude/skills/` and `.agents/skills/`.

```text
cmd/vibe → internal/cli ──→ internal/setupskill ──→ internal/guard
           (vibe init --skill)   (embedded skill/)    (InstallVersion,
                                                       reference commands)
```

- **Imports.** Among this repository's packages, `internal/setupskill`
  imports only `internal/guard`, for the version rule that fills the
  skill's one placeholder (the `vibe` version); its tests enforce that.
- **Embedded bytes.** `.gitattributes` forces LF for
  `internal/setupskill/skill/`, and a test fails on any carriage return
  in an embedded file, so a CRLF working copy cannot change what a build
  ships.
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
- **The drift test** (#109) is test code in `internal/cli`
  (`skill_drift_checker_test.go`), not a package: it checks the
  skill's `vibe` commands and flags against the real command tree, which
  only `internal/cli` builds, and loads the reference `vibe.yaml` files
  through `contract.Parse` in a Git work tree. One function reads a
  skill as a file system and returns every problem: rule IDs, guard rule
  IDs, `vibe` and `vibe-guard` commands and flags (`vibe-guard`'s from its
  help output), paths into the skill, repository files named, the
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

- **Repository intelligence** (GitNexus, Graphify): index, search,
  dependency, caller, impact and trace queries. It must never replace the
  compiler, LSP, linter, tests, or CI; it augments them. The setup skill
  offers Graphify as optional content (`reference/optional/graphify.md`);
  no rule checks it, and `verify` and CI never consult a graph.
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
  guardrule/                # guard rule IDs, policy.json v2, command parsing shared by the audit and vibe-guard; imports nothing from internal/ (spec 0053)
  guard/                    # vibe-guard: baseline rules, hook/explain/relock, unlock marker, reference commands, version rule; imports only guardrule (spec 0053, 0054)
  setupskill/               # the embedded vibeconform-setup skill, vibe init --skill, and the tests that run its reference hooks; imports only guard (spec 0054)
  doctor/                   # vibe doctor's checks; imports only contract, for the configuration, hook reader and bash resolution (spec 0059)
```

- **`vibe init`** (spec 0059) detects components from the tracked files
  with a small function in `internal/cli`, and checks the file it writes
  with `contract.Parse`, so it never writes a configuration `vibe audit`
  cannot read.
- **`vibe doctor`** reuses the audit's readers (`contract.Parse`, the
  entrypoint splitter `ParseSimpleCommand`, `ReadClaudeHooks` and
  `ResolveBash`), so it finds programs and bash exactly as the audit
  would run them.
- **The removed commands.** `vibe sync`, `vibe diff` and `vibe check` are
  hidden commands that print the migration guide's path and exit 1.
  `contract.DetectSchema` stays, so that a version 1 `vibe.yaml` gets the
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

`task verify` and `task verify:fast` depend only on native language
tooling, never on `vibe` (spec 0017; contract rule R-01 for adopters).
`task audit` is deliberately not one of `verify`'s steps: it builds `vibe`
and `vibe-guard` from the checkout into the Git directory, puts them first
on `PATH`, and runs the full contract audit (spec 0059). CI runs the same
audit as the required check `Contract / audit`
(`.github/workflows/contract-audit.yml`), next to `CI / gate`.
