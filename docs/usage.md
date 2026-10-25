# VibeConform User Manual

Status: living document, tracks `vibe`'s actual implemented behavior. If
something below and the CLI's own `--help` output disagree, trust
`--help` and file that as a doc bug.

New to VibeConform? [`adopting.md`](adopting.md) walks through adopting it
in a new repository, start to finish. This manual is the reference.

## Installing

Every release has two binaries, `vibe` and `vibe-guard`, for Linux, macOS
and Windows on amd64 and arm64 (spec 0054). Install them from a release
without Go, or build them from source.

### From a release

A `v*` tag push runs `.github/workflows/release.yml`. GoReleaser publishes
the release, with no draft step. A tag with a `-alpha.N`, `-beta.N` or
`-rc.N` suffix is marked "Pre-release", so a preview never becomes "Latest
release" (`docs/decisions/0007-release-tag-naming.md`). The release has
one archive per tool and platform, named after the binary:

- `vibe_<version>_<os>_<arch>.tar.gz`, and `.zip` on Windows;
- `vibe-guard_<version>_<os>_<arch>.tar.gz`, and `.zip` on Windows;
- `checksums.txt`, the SHA-256 of all 12 archives.

`<version>` is the tag without its leading `v`, `<os>` is `linux`,
`darwin` or `windows`, and `<arch>` is `amd64` or `arm64`. `vibe-guard`
has an archive of its own, so you can install the guard without `vibe`.

To install, download the archive and `checksums.txt` from the release
page, check the checksum before you unpack anything, and put the binary
on `PATH`:

```bash
v=v0.12.0-alpha.1
gh release download "$v" --repo Manual-debuger/VibeConform \
  --pattern "vibe-guard_${v#v}_linux_amd64.tar.gz" --pattern checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf "vibe-guard_${v#v}_linux_amd64.tar.gz" vibe-guard
```

On macOS, use `shasum -a 256 --check --ignore-missing checksums.txt`.

### Verifying a release

Every archive, every binary inside it and `checksums.txt` has a GitHub
artifact attestation. It records the commit and the workflow run that
built the file. `gh attestation verify` checks it:

```bash
gh attestation verify "vibe-guard_${v#v}_linux_amd64.tar.gz" \
  --repo Manual-debuger/VibeConform
gh attestation verify vibe-guard --repo Manual-debuger/VibeConform
```

After each release, the release workflow installs both tools through the
setup action on every OS and architecture that GitHub hosts. It checks
that each binary prints the tag, verifies the attestation of every archive
and binary, and runs `go install` for both tools at the tag. A pull
request builds every target as a GoReleaser snapshot
(`.github/workflows/release-snapshot.yml`), so a broken release
configuration fails before a tag exists.

### From source

For teams that do not allow prebuilt binaries, and for a commit that has
no tag. `go install` builds a tag (or any commit) from the module proxy:

```bash
go install github.com/Manual-debuger/VibeConform/cmd/vibe@v0.12.0-alpha.1
go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@v0.12.0-alpha.1
```

The binaries go to `$GOPATH/bin` (usually `~/go/bin`). `@latest` also
works. Go picks the newest release, and when there is none, the newest
pre-release. Pin an exact tag where the version matters.

`go run` fetches and runs without leaving a binary behind:

```bash
go run github.com/Manual-debuger/VibeConform/cmd/vibe@v0.12.0-alpha.1 --help
```

From a checkout:

```bash
git clone https://github.com/Manual-debuger/VibeConform
cd VibeConform
go build -o vibe ./cmd/vibe
go build -o vibe-guard ./cmd/vibe-guard
```

The setup action has the same choice for CI: its `go-install` source
builds both tools from the tagged source.

### The setup action

The setup action is a composite action at this repository's root
(`action.yml`). It installs an exact version of `vibe` and `vibe-guard`
in a GitHub Actions job, and it is meant for the conformance job only.
`verify` must not need `vibe` (contract rule R-01). The action never runs
`vibe audit`. The job's own `run:` step does. Pin the action by commit
SHA (C-02):

```yaml
- name: Install vibe
  uses: Manual-debuger/VibeConform@<commit SHA> # v0.12.0-alpha.1
  with:
    version: v0.12.0-alpha.1
- name: vibe audit
  run: vibe audit --repo-root .
```

Inputs:

- `version` (required): an exact version, such as `v0.12.0-alpha.1`, or a
  Go pseudo-version. The action refuses `latest`, an empty value, build
  metadata and anything that is not semver. Then the job's verdict cannot
  change without a change to the repository (spec 0022).
- `tools`: `vibe vibe-guard` by default. Name one to install only that
  one.
- `source`:
  - `release` downloads the archives for the runner's OS and
    architecture with `checksums.txt`. It checks each archive's SHA-256
    before it unpacks anything. A mismatch or a missing entry fails the
    step.
  - `go-install` sets up Go inside the action (pinned) and runs
    `go install …/cmd/<tool>@<version>`.
  - `workspace` builds both tools from the checked-out VibeConform
    repository, with the Go version its `go.mod` names. This is the
    self-hosting case of ADR 0008.
  - `auto` (the default) chooses `workspace` in a checkout of
    VibeConform itself, `go-install` for a pseudo-version (no release
    exists for one), and `release` otherwise.
  When `auto` chooses `workspace`, the action says so in a notice:
  `version` is then not installed, and the tools are built from the
  checkout.
- `verify-attestation`: `false` by default. With `true` and source
  `release`, the action also runs `gh attestation verify` on each archive
  with the `token` input.
- `checksums`: a `checksums.txt` to check the archives against. It
  replaces the release's own checksum file. Give a path relative to the
  workspace: the action refuses an absolute path and a path with a `..`
  part. This input is mainly for tests and air-gapped mirrors. With it,
  the checksums your repository reviewed decide, not a file on the same
  release.
- `token`: the token `gh attestation verify` uses. The default is the
  job's token (`github.token`).

What the checks prove: the default checksum check proves integrity
only. Each archive matches the release's own `checksums.txt`, so the
download was not corrupted or truncated. It does not prove authenticity:
someone who can change the release can change `checksums.txt` too. Set
`verify-attestation: true` to add provenance. The attestation proves
that this repository's release workflow built the archive from the tag.

Outputs:

- `source`: the source the action used, after `auto` is resolved
  (`release`, `go-install` or `workspace`).
- `bin-dir`: the directory the binaries are in, in the runner's native
  path form. It is a directory under `RUNNER_TEMP`, and the action adds
  it to `PATH`.

`.github/workflows/setup-action.yml` tests the action on Linux, macOS and
Windows in every pull request.

## Commands

`vibe` has four commands: `init`, `audit`, `doctor` and `unlock` (see
"`vibe unlock`" under "Agent hooks"), plus `--help` and `--version`.
`vibe-guard` is a separate binary; see "`vibe-guard`, the reference
guard".

### `vibe init`

Writes a `vibe.yaml` version 2 at the repository root (spec 0059), so
that `vibe audit` has a configuration to read:

```console
$ vibe init
wrote vibe.yaml
component . (go)
next: declare the entrypoints in vibe.yaml; until then vibe audit fails V-04
```

In a Go repository with `go.mod` at the root, the file is:

```yaml
# vibe.yaml version 2, written by vibe init. vibe audit reads it and
# checks the repository against the VibeConform contract.
contract: 1.1.0
platforms: [linux, windows]
agents: [claude]
# Declare the four entrypoints: one command each, or a list of commands.
# Until they are declared, vibe audit fails V-04. The setup skill
# (vibe init --skill) can fill them in. For example:
# entrypoints:
#   verify: task verify
#   verify-fast: task verify:fast
#   fmt: task fmt
#   fmt-check: task fmt:check
```

- **`contract`** is the contract version this `vibe` implements.
  **`platforms`** is `[linux, windows]` and **`agents`** is `[claude]`.
  Edit them to what the repository supports; see "`vibe.yaml` version 2".
- **Components** come from the tracked files (`git ls-files`, so a new
  manifest must be added to Git first). Each directory up to three levels
  below the root with a `go.mod`, a `pyproject.toml`, or a `package.json`
  beside a `tsconfig.json` is a component, of language `go`, `python` or
  `typescript`. When a directory has more than one, that order decides.
  Directories under `node_modules` or `vendor` are skipped, and a
  component inside another one is not a component of its own. A single
  component at the root gets no `components:` key, because the audit
  finds it from the root manifest; several components, or one in a
  subdirectory, are listed:

  ```yaml
  components:
    - path: "apps/web"
      language: typescript
    - path: "services/api"
      language: go
  ```

- **No component found** is an error, and nothing is written:
  `init: found no component in the tracked files: …; commit the
  project's manifest and run vibe init again`.
- **Entrypoints are left as comments.** `vibe init` cannot know which
  commands verify and format the repository. It checks what it writes
  with the audit's own parser, so the file always parses, but the audit
  then fails V-04 (a MUST rule) for the four required entrypoints:
  `verify`, `verify-fast`, `fmt` and `fmt-check`. Every other rule that
  needs an entrypoint fails too, with a finding that says only "this rule
  needs the … entrypoint, and V-04 finds it unusable" and a fix that
  starts "fix V-04 first". Rules that need no entrypoint are checked as
  usual. Declare the entrypoints yourself, or let the setup skill do it.
- **It never overwrites `vibe.yaml`.** An existing file, version 1
  included, is an error that says which kind it found:

  ```
  Error: init: vibe.yaml exists; vibe init does not overwrite it. Edit it, or remove it and run vibe init again
  Error: init: vibe.yaml exists and is version 1 (a top-level standard key); vibe init does not overwrite it. to leave the template-sync system, run `vibe init --skill` and follow the migration guide, .claude/skills/vibeconform-setup/reference/migrate.md
  ```

- It needs a Git work tree (it reads the tracked files) and writes no
  other file. It takes no arguments: `vibe init <standard> <version>`
  was removed with the template-sync system.

**Flags:**

| Flag | Default | Meaning |
|---|---|---|
| `--repo-root` | `.` | Repository to write `vibe.yaml` into |
| `--skill` | off | Write the setup skill instead; see below |

### `vibe init --skill`

Writes the setup skill, `vibeconform-setup`, into the repository (spec
0054). See "The setup skill" below for what it does.

```bash
vibe init --skill
vibe init --skill --repo-root ./some/other/repo
```

- It writes the skill to `.claude/skills/vibeconform-setup/` (Claude
  Code) and `.agents/skills/vibeconform-setup/` (Codex). The two copies
  are byte-identical.
- It replaces every file in those two directories with the version
  embedded in this `vibe`, and removes files there that this version
  does not have, with the directories they leave empty. Run it again
  after upgrading `vibe`.
- It writes nothing else: it reads no `vibe.yaml`, so it works with a
  version 1 configuration (which the skill migrates), a version 2 one or
  none.
  `.claude/skills/` is not a guardrail path, so an agent may run it.
- It prints each path it removed (`removed <path>`) and wrote
  (`wrote <path>`).
- The skill's only placeholder is the `vibe` version. `init --skill`
  replaces it with the version the `vibe-guard` install hint uses: this
  `vibe`'s release version, or `latest` for a source build.
- `--skill` takes no arguments.

### `vibe --version`

Prints the `vibe` version and the contract version it implements:

```console
$ vibe --version
vibe version v… (contract 1.1.0)
```

The setup skill's upgrade flow reads the contract version from it.

### `vibe audit`

`vibe audit` reads `vibe.yaml` version 2 (a top-level `contract:` key,
see "`vibe.yaml` version 2" below) and checks the repository against the
rules of the contract version it declares: 25 rules in contract 1.1.0,
in six categories (spec 0051, spec 0052, spec 0055). Each rule ends as
`pass`, `fail`, `not applicable`, `waived`, `cannot decide` or `not
run`. A full run decides every part of every
rule: the static checks, then the execution checks and probes in a
temporary probe copy (spec 0053). `--static` reports the execution and
probe parts as "not run".

```bash
vibe audit                                  # the full audit
vibe audit --config other.yaml              # another configuration file
vibe audit --static                         # the quick run
vibe audit --explain C-02
```

**The full audit** (spec 0053). After the static parts, the audit makes
a *probe copy*: a fresh Git repository in the system's temporary
directory that holds the working-tree content of every tracked file,
committed once. Every execution and probe part runs there, one after
another, from a reset copy: `setup`, A-07, V-03, the baseline runs of
`verify` and `verify-fast`, the *planted defects* of V-01, V-02 and A-05,
the guard corpus of A-02, A-03 and A-08, D-01 and R-01. The copy is
deleted at the end, also after a failure or an interrupt, and the
repository's working tree, index and `.git` never change.

- **How it runs things.** Entrypoints run without a shell, as a program
  and its arguments found on `PATH`. Hook commands run as Claude Code
  runs them, `bash -c '<command>'` with the JSON payload on stdin (on
  Windows, Git for Windows' bash or `CLAUDE_CODE_GIT_BASH_PATH`). Every
  run has a time limit: the hook's own `timeout` or 600 seconds (Claude
  Code's default) for hooks,
  `--probe-timeout` for entrypoints. A run over its limit is killed with
  its child processes.
- **Tools.** The audit needs the tools the entrypoints call (`task`, the
  linters, the test runners) and bash for the hooks. A missing tool, no
  bash, a timeout or a word with an expansion makes the part `cannot
  decide`, never `fail`: it is the auditing machine's gap. The A-02 and
  A-03 probes run whatever the PreToolUse hook runs, so a repository
  using `vibe-guard` needs it installed.
- **Time.** A full audit runs `verify` and `verify-fast` once each plus
  three times per component, the Stop hook three times, and the whole
  guard corpus through the PreToolUse hook twice. On this repository
  that is several minutes to an hour, mostly `task test`. `--static`
  stays the quick local run.
- **Progress.** In text mode each run prints a line on stderr ("vibe
  audit: verify with the static-analysis defect in ."). `--json` keeps
  stdout to the JSON document.
- `vibe audit --explain V-01` and `V-02` print the defect samples;
  `--explain A-02` and `A-03` print the guard corpus counts by rule ID.

**Flags:**

| Flag | Default | Meaning |
|---|---|---|
| `--repo-root` | `.` | Repository to check. Its `vibe.yaml` is the configuration unless `--config` names another |
| `--config <path>` | `vibe.yaml` at `--repo-root` | Configuration file to read instead. The path is relative to the current directory, not to `--repo-root`. It must be a version 2 file |
| `--static` | off | Run only the static checks. Fast, runs nothing, and never reaches a verdict: execution and probe parts are `not run`, so the exit code is always `1` |
| `--json` | off | Print the report as JSON on stdout, nothing else on stdout |
| `--probe-timeout <duration>` | `30m` | Time limit of each entrypoint run in the probe copy. It must be positive |
| `--keep-probe-copy` | off | Keep the probe copy and print its path on stderr, for debugging |
| `--explain <ID>` | | Explain one rule, such as `C-02`, and read nothing else |

`--explain` reads no repository and no configuration, so it works
anywhere, and combining it with any other audit flag is an error. It
prints what the rule registry holds: ID and rule sentence, level,
category, check kinds, source, when the rule is not applicable, whether
it can be waived, the rationale and the check as spec 0051 states it.
The rationale is spec 0051's source for the rule, plus the category's
introduction where spec 0051 has one (H, C and A). An unknown ID is exit
`1` and lists the valid IDs.

```
$ vibe audit --explain C-02
C-02: Toolchains and actions are pinned

  Level:           MUST
  Category:        C, CI
  Checks:          static
  Source:          spec 0044
  Not applicable:  never; the rule always applies
  Waivable:        no, a MUST rule cannot be waived

Rationale:
  Source: spec 0044. The C rules read GitHub Actions or GitLab CI (GitLab CI
  since 1.1.0). GitHub Actions wins: when the repository has workflows in
  `.github/workflows/`, the C rules read them only. GitLab CI
  (`.gitlab-ci.yml`) is read when there are none. If the repository has CI
  configuration for another provider and neither, the C rules cannot be
  decided. The report names the provider it read.

Check:
  Static, across every workflow (not only generated ones): every `uses:`
  other than a local path is pinned to a full 40-character commit SHA, and
  no `run:` step contains a command word ending in `@latest`
```

**The text report.** A header, one line per category counting its
results (a failed MUST rule counts as `FAIL`, a failed SHOULD rule as
`fail`), then the sections Failed, Waived, Not applicable, Cannot decide,
Not run (`--static` only), Guard exceptions and Local guardrail
adjustments, any notes, and the verdict. An empty section prints
`(none)`. Failures are listed MUST first, then by rule ID, and every
finding names the file to change or create and the direction of the
fix. On a repository whose only failures are two SHOULD rules, for
example (long lines shortened):

```
vibe audit: contract 1.1.0 (vibe …), static checks only

  V  Verification entrypoints              1 pass, 3 not run
  H  Local Git hooks                       2 pass
  C  CI                                    4 pass, 1 fail
  A  Agent guardrails                      3 pass, 5 not run
  D  Dependencies and supply chain         1 pass, 1 fail, 1 not run
  R  Removability and repository hygiene   2 pass, 1 not run

Failed
  C-05  SHOULD  Tests run on Linux and Windows
        .github/workflows/ci.yml: the verify command `task lint` runs on no Windows runner (…)
        Fix: run `task lint` on Windows too, such as with a matrix over os: […]
  D-03  SHOULD  CI scans for known vulnerabilities
        .github/workflows/: no workflow step runs a recognized vulnerability scanner (…)
        Fix: add a CI step that runs the scanner for the component's language, such as govulncheck ./... for Go, …

Waived
  (none)

Not applicable
  (none)

Cannot decide
  (none)

Not run (--static)
  V-01 V-02 V-03 A-02 A-03 A-05 (probe) A-07 (execution) A-08 D-01 R-01 (execution)

Guard exceptions
  (none)

Local guardrail adjustments
  (none)

CI provider read: GitHub Actions
Note: the clone has no refs/remotes/origin/HEAD, so the CI rules assume the default branch is main

Verdict: cannot decide. 10 rules did not run (--static).
```

The verdict line is `Verdict: conformant`, `Verdict: not conformant` or
`Verdict: cannot decide. <reason>`. When the configuration declares a
different version that this vibe audits, such as `1.0.1` or `1.1.3`,
the header says `contract 1.1.0 (declared 1.0.1; vibe …)`. The line
`CI provider read:` names the CI provider the C rules, D-03 and R-01
read (`GitHub Actions` or `GitLab CI`); it is left out when they read
none.

**What is listed.** Not-applicable rules, waivers (in force, expired, or
unused because the rule is not applicable), every exception in
`.claude/hooks/policy.json`, and every exception and extra rule in a
tracked `.claude/hooks/policy.local.json`, so that every loosening shows
in one report. `.claude/settings.local.json` is never read.

**Exit codes:**

| Code | Meaning |
|---|---|
| `0` | conformant: every MUST rule passed or is not applicable |
| `2` | not conformant: a MUST rule failed, and every other MUST rule was decided |
| `1` | cannot decide: a MUST rule could not be decided or did not run (always the case with `--static`). Also no `vibe.yaml`, a version 1 `vibe.yaml`, an invalid configuration, both or neither of `contract` and `standard`, an unsupported contract version, an unknown `--explain` ID, a directory that is not a Git work tree, or any other tool error |

A failed MUST rule beside an undecided one is `1`, not `2`: there is no
verdict until every MUST rule is decided, and the reason says how many
MUST rules already fail. SHOULD results never change the exit code.
After a report, the reason also goes to stderr
(`Error: not conformant: 1 MUST rule fails`,
`Error: cannot decide: …`). A configuration error goes to stderr with no
report:

```
Error: vibe.yaml: waivers[0].until: "2026-13-01" is not a date YYYY-MM-DD
Error: vibe.yaml: contract: cannot decide; upgrade vibe: this vibe implements contract 1.1.0, and the configuration declares 1.2.0
```

A missing `vibe.yaml`, and a version 1 one (a top-level `standard:` key,
written for the template-sync system), are exit `1` too, with a pointer
to what to do. `vibe doctor` prints the same messages:

```
Error: no vibe.yaml in .: run `vibe init` to write one
Error: vibe.yaml is version 1 (a top-level standard key), which this vibe no longer reads; to leave the template-sync system, run `vibe init --skill` and follow the migration guide, .claude/skills/vibeconform-setup/reference/migrate.md
```

**Contract versions.** This vibe implements contract 1.1.0. A declared
version must be `X.Y.Z`. A vibe implements the latest MINOR of its
MAJOR, so every declared `1.0.n` and `1.1.n` (older or newer PATCH) is
audited as 1.1.0 (spec 0051, "Versioning"), and the report shows both
versions when they differ. A repository set up under 1.0.0 or 1.0.1
keeps its `contract:` line. Spec 0051, "Changes", lists what each
version changed:

- 1.0.1: A-04 fails a `.claude/hooks/policy.json` that `vibe-guard`
  cannot load when a PreToolUse hook runs `vibe-guard hook`, and the
  allow cases A-02 and A-08 then see refused name `policy.json`.
- 1.1.0 (spec 0055): the C rules, D-03 and R-01 read GitLab CI when the
  repository has no GitHub Actions workflows (see "GitLab CI" below);
  R-03 recognizes GitLab merge request templates; D-02 recognizes
  Renovate's GitLab locations and asks GitLab CI repositories for its
  `gitlabci` manager; R-01 no longer counts `vibe --version` in a CI
  job that calls no entrypoint. A GitHub Actions repository keeps its
  1.0.1 results, apart from that R-01 relaxation.

Another MAJOR, or a MINOR above 1, is "cannot decide; upgrade vibe"
(exit `1`).

**JSON.** `--json` prints one object. `schema` versions this shape,
separately from the contract; it is `1`.

```json
{
  "schema": 1,
  "vibe": "v0.12.0",
  "contract": {"declared": "1.0.0", "audited": "1.1.0"},
  "static_only": true,
  "verdict": "undecided",
  "reason": "10 rules did not run (--static)",
  "summary": {
    "MUST":   {"pass": 7, "fail": 0, "not_applicable": 0, "waived": 0, "undecided": 0, "not_run": 9},
    "SHOULD": {"pass": 6, "fail": 2, "not_applicable": 0, "waived": 0, "undecided": 0, "not_run": 1}
  },
  "rules": [
    {"id": "C-05", "level": "SHOULD", "category": "C",
     "title": "Tests run on Linux and Windows",
     "result": "fail",
     "parts": [{"check": "static", "result": "fail"}],
     "findings": [{"file": ".github/workflows/ci.yml", "message": "…", "fix": "…"}],
     "waiver": null}
  ],
  "guard_exceptions": [{"rule": "…", "match": "…", "reason": "…"}],
  "guard_exception_uses": [{"rule": "…", "match": "…", "case": "…"}],
  "local_adjustments": [{"file": "…", "kind": "exception", "rule": "…", "pattern": "…", "reason": "…"}],
  "ci_provider": "github_actions"
}
```

- Result tokens, in `result`, `parts[].result` and `summary`: `pass`,
  `fail`, `not_applicable`, `waived`, `undecided`, `not_run`. `summary`
  has every token for both levels.
- Verdict tokens: `conformant`, `not_conformant`, `undecided`.
- `rules` lists all 25 rules in ID order. `parts[].check` is `static`,
  `execution` or `probe`.
- `waiver` is `null`, or `{"reason", "until", "expired"}`. A rule with a
  waiver and result `not_applicable` has an unused waiver.
- `local_adjustments[].kind` is `exception` or `rule` (an extra rule).
- `guard_exception_uses` lists each guard corpus case that an exception
  of `policy.json` let through in A-02's probe (spec 0053). It is a new
  key; `schema` stays `1`. The text report lists the same cases under
  each exception ("let through: …").
- `ci_provider` is the CI provider the C rules, D-03 and R-01 read:
  `github_actions`, `gitlab_ci`, or `null` when they read none (spec
  0055). It is a new key; `schema` stays `1`. The text report's line
  `CI provider read:` says the same.

This repository's `Contract / audit` job
(`.github/workflows/contract-audit.yml`) runs the full audit on every
pull request and passes on exit `0`. It builds `vibe` and `vibe-guard`,
installs the tools the entrypoints call, and sets
`VIBE_TEST_REQUIRE_TOOLS` so that the tests `verify` runs fail rather
than skip without uv, pnpm, Python, Node or bash. It is a required
check (spec 0059). `task audit` runs the same audit locally, with the
`vibe` and `vibe-guard` built from the checkout; `task audit:static`
runs `--static`.

Since `--static` always exits `1`, a CI job that runs it judges the JSON
instead. This repository's `Contract / audit (static)` job
(`.github/workflows/contract-audit.yml`) fails when a MUST rule is
`fail` or `undecided`:

```bash
vibe audit --static --json > contract.json || true
jq -r '.rules[] | select(.level == "MUST" and (.result == "fail" or .result == "undecided")) | .id' contract.json
```

**Behavior to know:**

- Read-only: it never writes a file.
- **Tracked files only.** The static checks read the working-tree
  content of the files `git ls-files` lists. An untracked file counts as
  absent, so a local file cannot make a repository look conformant.
- A file a rule needs that does not parse fails that rule, and the
  finding names the file and the parse error.
- **Not applicable** in contract 1.1.0: C-03 when the C rules read
  GitLab CI; C-05 when `platforms` has no `windows`; every A rule when
  `agents` has no `claude`; R-02 when `agents` is empty, and R-02's
  `CLAUDE.md` half when `agents` has no `claude`.
- Commands are parsed as POSIX shell (ADR 0026). A command is "called"
  when a simple command anywhere in a hook, CI command (a GitHub `run:`
  step, a GitLab `before_script` or `script` line) or entrypoint has
  exactly its words, after quote removal. A string passed to `sh -c` is
  not parsed again.
- **Which CI provider is read.** GitHub Actions wins: when tracked
  `.github/workflows/*.yml` files exist, the C rules, D-03 and R-01's
  CI part read them only, as in 1.0.1. GitLab CI is read when there are
  none and `.gitlab-ci.yml` is tracked. With neither and another
  provider's file tracked (`.circleci/config.yml`, `Jenkinsfile`,
  `azure-pipelines.yml` and others), those rules are "cannot decide";
  with no CI configuration at all, they fail. The report names the
  provider read (`CI provider read:`, `ci_provider`). Without
  `refs/remotes/origin/HEAD`, the CI rules assume `main` or `master` is
  the default branch and say so in a note.
- **R-01 and `vibe --version`** (since 1.1.0, on either provider). A
  command whose words are exactly `vibe --version` is not a call of
  `vibe` in a CI job that calls no command of any declared entrypoint
  (`verify`, `verify-fast`, `fmt`, `fmt-check`, `setup`): such a job
  tests that `vibe` installs. In an entrypoint, a hook command, or a CI
  job that calls an entrypoint, it is still a call, and no other
  spelling is exempt.
- The recognition rules (hook managers, update bots, scanners, runners,
  trigger filters) are in spec 0052, "Recognition rules"; the GitLab
  readings of each rule are in spec 0051, "C: CI", and after the D and
  R tables.

**GitLab CI** (contract 1.1.0, spec 0055). The audit resolves
`.gitlab-ci.yml` statically, the way GitLab does: YAML anchors and merge
keys, `include: local` (with wildcard paths) merged in GitLab's order,
`default:`, `extends:` and `!reference` tags that point at local keys.
Keys that start with `.` are templates, not jobs. A file that does not
parse, an `extends:` or `!reference` that points at nothing, or a cycle
fails every rule that reads the CI. It reads no project setting
(pipelines must succeed, protected branches, job-token scope), as it
reads no GitHub branch protection.

- **Includes.** Only `local:` includes are followed. `project:`,
  `remote:`, `component:` and `template:` includes are not; their jobs
  count as calling nothing. A local include with `rules:` is followed
  only for the pipeline kinds its rules admit.
- **Child pipelines.** A job with `trigger: include:` naming local files
  is followed when it has `strategy: depend` (or `strategy: mirror`), so
  that the parent pipeline mirrors the child's status. The child's jobs
  are read with the trigger job's rules as their outer condition. A
  child without such a strategy, a dynamic child (`include: artifact:`)
  and `trigger: project:` are not followed.
- **Commands.** A job's commands are its `before_script` and `script`
  lines. `after_script` is read by R-01 and D-03 but does not count as
  calling `verify`, since its failure does not fail the job.
  `allow_failure` is ignored.
- **Pipeline kinds.** The audit asks two questions of each job: does it
  run in a merge request pipeline, and does it run on a push to the
  default branch? It evaluates `workflow:rules`, then the job's `rules:`
  (first match wins, `when: never` excludes) or its `only`/`except`. A
  job with no rules and no `only`/`except` runs in both kinds: GitLab
  shows its branch pipeline on the merge request. The conditions it
  reads form a closed list:
  - `$CI_PIPELINE_SOURCE ==` or `!=` a quoted literal
    (`merge_request_event`, `push`, `schedule`, …);
  - `$CI_MERGE_REQUEST_IID`, `$CI_COMMIT_BRANCH` and `$CI_COMMIT_TAG`
    as presence tests;
  - `$CI_COMMIT_BRANCH` or `$CI_COMMIT_REF_NAME` compared with
    `$CI_DEFAULT_BRANCH` or with a quoted branch name;
  - `&&`, `||` and parentheses over these;
  - `only`/`except` with `merge_requests`, `branches`, `pushes`, `tags`,
    `schedules` and branch names.

  A rule with any other condition, or with `changes:` or `exists:`,
  does not admit the job for that pipeline kind. With `when: never` it
  leaves the job out (it might exclude it); without, the audit passes
  over it and a later rule may still admit the job. When none does, the
  finding quotes the first such condition and its line. So a job gated
  by `changes:` does not count for C-01, as a GitHub `paths` filter
  does not. `workflow:rules` and include rules are read the same way.
- **The rules.** C-01 and C-04 ask their questions of the jobs that run
  for each pipeline kind. C-02 requires every `image:` and `services:`
  entry to end in `@sha256:` and a digest, every unfollowed include to
  be pinned (`project:` with a 40-character `ref:`, `component:` with a
  40-character version, `remote:` with `integrity:`), fails every
  `template:` include, and fails any script word ending in `@latest`.
  C-03 is not applicable: GitLab job tokens have no per-file
  permissions. C-05 reads a job's platforms from its `tags:` after
  `parallel: matrix` expansion (a tag containing `windows`, `linux` or
  `macos`, ignoring case); a job with no tags and an `image:` is Linux.
  D-03 also accepts an include of GitLab's Dependency-Scanning template
  or the `dependency-scanning` CI/CD component, which C-02 still checks.
  R-01 reads every job's scripts except those of the job that satisfies
  C-04.
- **D-02 on GitLab CI.** The update bot must be Renovate: Dependabot
  does not run on GitLab and cannot satisfy D-02 there. The Renovate
  configuration covers each component's manager and `gitlabci` instead
  of `github-actions`, and, when it lists `enabledManagers` and the
  pipeline has `project:` includes (the ones Renovate's
  `gitlabci-include` manager updates), `gitlabci-include`.
  `.gitlab/renovate.json` and `.gitlab/renovate.json5` are recognized
  Renovate locations on any repository. The fix text names Renovate on
  GitLab CI and Dependabot on GitHub Actions.
- **R-03.** Every tracked `.md` file directly under
  `.gitlab/merge_request_templates/` is a pull request template, on any
  repository, checked like the GitHub templates.

### `vibe doctor`

Diagnoses whether this machine can run the repository's workflow. Where
`audit` asks "does the repository conform?", `doctor` asks "can I work
here?". It reads `vibe.yaml` version 2 and the tracked
`.claude/settings.json`, and prints one line per check:

```console
$ vibe doctor
PASS        git            git version 2.53.0.windows.1; repository readable
PASS        vibe.yaml      version 2, contract 1.1.0
PASS        task           C:\Users\me\AppData\Local\Microsoft\WinGet\Links\task.exe; runs verify, verify-fast, fmt, fmt-check
PASS        bash           C:\Program Files\Git\bin\bash.exe; runs the hook commands
FAIL        vibe-guard     not on PATH; the PreToolUse hook that runs it fails on every tool call
summary: 4 pass, 0 warn, 1 fail, 0 unverified
Error: doctor: 1 check failed
```

The checks, in order:

- **git:** it runs, and `--repo-root` is a readable Git repository.
- **vibe.yaml:** it parses as `vibe audit` parses it. A missing file and
  a version 1 file fail with the audit's own messages ("run `vibe
  init`", the migration guide), and the checks below, which need the
  configuration, do not run.
- **One line per entrypoint program:** the first word of each command
  of `verify`, `verify-fast`, `fmt`, `fmt-check` and `setup`, with the
  entrypoints that run it. A program named by a path is looked for in
  the repository (or at that absolute path), any other on `PATH`. A
  command that is not one simple command, as V-04 reads it, or whose
  program is an expansion, is `UNVERIFIED`; with no entrypoint declared,
  the line says so. V-04 reports these.
- **bash:** the bash that Claude Code runs the hook commands with, found
  as the audit finds it: Git for Windows' bash or
  `CLAUDE_CODE_GIT_BASH_PATH` on Windows, `bash` on `PATH` elsewhere.
  With no command hook in `.claude/settings.json`, nothing needs it.
- **vibe-guard:** on `PATH`, only when a PreToolUse hook runs
  `vibe-guard hook`. Without it, every tool call is refused.

A `.claude/settings.json` that does not parse gives one `UNVERIFIED`
`hooks` line instead of the last two; the audit's A-01 reports the file.

| Status | Meaning |
|---|---|
| `PASS` | Checked, and healthy. |
| `FAIL` | Something the workflow needs is missing here. |
| `UNVERIFIED` | `doctor` cannot check this reliably, and says so rather than guessing. |

**Exit codes:** `1` if any check is `FAIL`, otherwise `0`. Doctor never
returns `2`, which stays the audit's verdict.

**Behavior to know:**

- Read-only: installs nothing, writes no file, changes no configuration.
- It runs `git` and nothing else: no entrypoint, no hook, no network.
  It looks programs up; it does not check their versions.
- It doesn't check conformance; that is `vibe audit`.
- CI is still the authority. A clean `doctor` on Windows says the
  workflow can run here, not that Linux CI will pass.

**Flags:** `--repo-root` (default `.`), the repository to diagnose.

### Removed commands: `vibe sync`, `vibe diff`, `vibe check`

These belonged to the template-sync system, which spec 0059 removed.
Each prints one line that names the migration guide and exits `1`,
whatever its arguments:

```
Error: vibe sync was removed with the template-sync system; to leave the template-sync system, run `vibe init --skill` and follow the migration guide, .claude/skills/vibeconform-setup/reference/migrate.md
```

They are hidden from `vibe --help`. See "Leaving the template-sync
system" below.

## `vibe.yaml` version 2

A configuration with a top-level `contract:` key is version 2 (spec 0051,
"`vibe.yaml`"; spec 0052). It declares the repository's platforms,
agents, components and entrypoints, and `vibe audit` checks the
repository against the contract version it names. `vibe init` writes a
first one, and the setup skill completes it. The file is the
repository's, edited by hand. This repository's own `vibe.yaml`:

```yaml
contract: 1.1.0
platforms: [linux, windows]
agents: [claude, codex]
# No components: one Go component at the root, found from go.mod.
entrypoints:
  verify:
    - task fmt:check
    - task typecheck
    - task lint
    - task test
    - task mod:verify
    - task security
    - task workflows:lint
  verify-fast: task verify:fast
  fmt: task fmt
  fmt-check: task fmt:check
```

| Field | Required | Meaning |
|---|---|---|
| `contract` | yes | The contract version, `X.Y.Z`, such as `1.1.0` |
| `platforms` | yes | Non-empty list of distinct values from `linux`, `windows`, `macos` |
| `agents` | yes | List of distinct values from `claude`, `codex`; may be empty (`[]`). Contract 1.1.0 has no Codex rules |
| `components` | no | List of `{path, language}`; `language` is `go`, `python` or `typescript` |
| `entrypoints` | yes | `verify`, `verify-fast`, `fmt`, `fmt-check`; `setup` is optional |
| `waivers` | no | List of `{rule, reason, until}` |

**Validation.** Decoding is strict: an unknown key at any level is an
error, so a typo cannot silently turn a rule off. Every invalid
configuration exits `1` with no report, and the error names the file,
the field path and the reason, for the first error in document order:

```
Error: vibe.yaml: entrypoints.verify[2]: must be a command string
Error: vibe.yaml: waivers[0].rule: C-02 is a MUST rule; only SHOULD rules can be waived
Error: vibe.yaml: integrations: the version 2 schema has no integrations field; agents go in agents; editors and repository intelligence moved into the setup skill
```

A file with a top-level `standard:` key and no `contract:` is a version 1
file, and gets the migration pointer instead (see "`vibe audit`"). In a
version 2 file, the other version 1 fields (`version`, `integrations`,
`development`, `policy`, `ci`) are rejected like any unknown key, each
with a pointer to where its content went. With `--config`, errors name the file
as you gave it.

- **Components.** When `components` is omitted, the tracked manifests
  at the root decide: exactly one of `go.mod`, `pyproject.toml`,
  `package.json` gives the language of one component at `.`. None, or
  more than one, is an error that names `components` and the manifests
  found. A declared `path` is a clean, relative, forward-slash directory
  with tracked files (`.` is the root); paths are distinct and do not
  nest.
- **Entrypoints.** Each value is one command or a list of commands. A
  list is "called" only when each of its commands is called, so a CI
  that splits verification across jobs declares `verify` as the list of
  what each job runs. Each command must be exactly one simple command
  (words, optionally after `VAR=value` assignments, with no `&&`, `;`,
  pipes or redirections); write a list instead. A missing, empty or
  compound required entrypoint is not a configuration error: V-04 fails,
  and the rest of the report still runs.
- **Waivers.** `rule` must be a rule ID of contract 1.1.0 and a SHOULD
  rule: a MUST rule cannot be waived. `reason` must be non-empty after
  trimming. `until` is a date, `YYYY-MM-DD`, and the waiver is in force
  through the end of that day in UTC. At most one waiver per rule. An
  expired waiver is ignored: the rule is checked, and the report marks
  it "waiver expired". A waiver for a rule that is not applicable is
  reported as unused.

```yaml
waivers:
  - rule: C-05
    reason: the Windows runners are not set up yet
    until: 2026-12-31
```

## The setup skill

`vibeconform-setup` (spec 0054) takes a repository in any of the
contract's languages to a conformant `vibe audit` under contract 1.1.0.
`vibe init --skill` writes it; the repository owns it from then on.

What the agent does with it, in order:

1. **Detect** the components and their languages (`go.mod`,
   `pyproject.toml`, `package.json`), the existing task runner, hook
   manager, CI, update bot, lint and agent configuration, and whether
   the template-sync system set the repository up. It reads no secret
   file.
2. **Migrate** first when the template-sync system is there
   (`reference/migrate.md`): merge each seam pair into one file, fold
   `Taskfile.local.yml` into `Taskfile.yml`, remove `.vibe/`,
   `Taskfile.vibe.yml`, the managed-region markers and the prose about
   the old system. The old guard keeps working until the new
   `.claude/settings.json` is in place; its task and script go last.
3. **Decide the entrypoints**: keep the team's runner (Task, make, just,
   package scripts), and use Task only when there is none. Write
   `vibe.yaml` version 2. A monorepo whose CI verifies each component in
   its own job declares `verify` as the list of those commands.
4. **Satisfy the rules** one category at a time from
   `reference/rules.md`, keeping whatever already satisfies a rule. Each
   configuration is one file; no `*.local.*` or `*.base.*`. The skill's
   own directories go into the formatter's and linters' ignores. The
   setup action is pinned to a release, never `latest`.
   `.claude/hooks/policy.json` comes before `.claude/settings.json`; a
   guard refuses the agent that file, so the skill hands you a command
   that writes it, and stops until you have run it.
5. **Install `vibe-guard`** from the release archive with its SHA-256
   checked against `checksums.txt`, or with `go install`, and check that
   a non-interactive shell, as hooks use, finds it.
6. **Audit loop**: stage the changes (the audit reads tracked files
   only), `vibe audit --static --json`, then `vibe audit --json`, fixing
   each finding by its `file` and `fix`, until the verdict is
   conformant. It stops and reports when a finding needs a person (a
   missing tool, the guard policy, an unlock, an unsupported CI
   provider, a waiver) or when two rounds change nothing.
7. **Report** the verdict, the files written, changed and removed, and
   the SHOULD rules that still fail.

The skill's files:

| Path | Holds |
|---|---|
| `SKILL.md` | The process above, for the agent |
| `reference/rules.md` | One entry per rule of contract 1.1.0: what satisfies it on GitHub Actions and on GitLab CI, which file to start from |
| `reference/vibe-yaml/` | `vibe.yaml` version 2 for Go, Python, TypeScript and a monorepo |
| `reference/taskrunner/` | Taskfiles per stack, with `verify`, `verify:fast`, `fmt`, `fmt:check` and the agent hook tasks |
| `reference/hooks/` | `lefthook.yml` per stack |
| `reference/ci/` | `ci.yml` (verify on Linux and Windows, a vulnerability scan), `audit.yml` (the audit job with the setup action), `dependabot.yml`, the pull request template; per stack, a `.gitlab-ci.yml` for GitLab CI, and `renovate.json`, the update bot on GitLab (spec 0055) |
| `reference/agents/` | `.claude/settings.json` with the `vibe-guard` hooks, `policy.json` version 2, the `AGENTS.md` Verification section, `CLAUDE.md` |
| `reference/lint/` | golangci-lint; ruff and pyright; ESLint, Prettier (with a `.prettierignore`) and `tsconfig.json`, each one file. The ESLint, Prettier and Ruff files exclude the skill's own directories |
| `reference/optional/` | Graphify, VS Code, Zed, language servers, the `AGENTS.md` workflow sections and a docs layout |
| `reference/migrate.md` | Leaving the template-sync system |
| `reference/upgrade.md` | The upgrade flow |

The reference files live in `internal/setupskill/skill/reference/`, and
the tests beside them (`internal/setupskill/reference_*_test.go`) run
their hook tasks, Stop gate and green stamp, and check their CI pins.
The reference hook commands are `internal/guard`'s, so the skill and
`vibe-guard` cannot disagree. The three `examples/` are assembled from
these files, and the `Examples` workflow audits each one.

**Upgrading.** After installing a newer `vibe`, run `vibe init --skill`,
then ask the agent to upgrade. It compares `contract:` in `vibe.yaml`
with the contract `vibe --version` names. Within a MAJOR it bumps
`contract:`, updates `vibe-guard` and the setup action, and runs the
audit loop. Across a MAJOR it first asks you to run `vibe unlock` in
your own terminal, because it must change `.claude/settings.json` or
`.claude/hooks/`.

## Agent hooks

The setup skill's reference `.claude/settings.json`
(`reference/agents/settings.json`) runs five Claude Code hooks, and its
reference Taskfiles (`reference/taskrunner/`) define the hidden tasks
they call. The contract's A rules check the hooks' behavior, not these
names (spec 0052): a repository may implement them any way that passes.
This section describes the references, which this repository also runs.
See `docs/specs/0021-agent-hooks-task-interface.md` and
`docs/specs/0023-agent-lifecycle-hooks.md`. Codex hooks are suspended;
see "Codex: hooks suspended" below.

| When | Command | What it does | Blocks? |
|---|---|---|---|
| Session start | `task -x hook:context` | prints a few lines of facts into the agent's context | no |
| Before a shell command or file edit | `vibe-guard hook` | refuses destructive commands and secret-file edits; see "The agent guard" | yes |
| After a file edit | `task fmt` | formats the repository (rule A-06) | no (tool already ran) |
| After a file edit, in the background | `task -x hook:check` | runs the typecheck and the lint, no tests; nothing in a sub-agent | no |
| When the agent ends its turn | `task verify:fast`, then `task -x hook:done` | gates the stop on `verify:fast`, unless the tree is unchanged since its last green run | yes, once per stop |

Every hook but the guard starts with `cd "${CLAUDE_PROJECT_DIR:-.}"`, so
it runs from the project root whatever the agent's current directory
(spec 0058). Nothing on these paths calls `vibe`, so all five keep
working after you remove VibeConform. The `hook:*` tasks have no
`desc`, so `task --list` doesn't show them.

**Exit 2 is the only code that reaches the agent.** On exit 2 Claude Code
shows the model the hook's stderr; any other exit is treated as a
non-blocking error. That's why every hook task is called as `task -x …`
(without `-x`, Task reports a failure as 201) and why each hook ends its
failing paths in `exit 2`. The Stop hook's `task verify:fast` needs no
`-x`: only whether it failed matters, and `hook:done` turns that into
exit 2.

### `verify:fast` and the tool caches

The Stop hook and the pre-push hook run the fast, incremental part of
`verify`, which you can also run yourself (`hook:check` runs only its
typecheck and lint):

| Reference Taskfile | `task verify:fast` runs | Only in `task verify` |
|---|---|---|
| Go | `fmt:check`, `typecheck`, `lint`, `test:short` | `security`, `mod:verify`, `workflows:lint`, the tests short mode skips |
| TypeScript | same as `verify` | none |
| Python | same as `verify` | none |

"Affected" comes from each tool's own cache, not from anything
VibeConform computes:

- **Go:** `go test ./...` reuses a package's cached result unless the
  package, or anything it imports, changed. So only affected tests rerun,
  including packages that depend on your change. golangci-lint caches its
  analysis. Don't add `-count=1` to `test`: it turns the cache off.
- **Go's test cache has a cost of its own:** to decide whether a cached
  result still holds, `go test` hashes every file and environment
  variable each package's test touched, which it records in the test
  log. A test that stats hundreds of thousands of paths makes that check
  take minutes, on a cache hit too (spec 0056 found one at 661,000
  entries). The cache stays on (spec 0057); if a package's cached run is
  slow, look at what its tests walk, rather than adding `-count=1`.
- **Go short mode:** `verify:fast` runs `task test:short`,
  which is `go test -short ./...` without its `ok` and `[no test files]`
  lines: it prints the failures, every other line too, and keeps
  `go test`'s exit code. It filters in Task's own shell, so it needs no
  `grep`. A test that checks `testing.Short()` can skip itself there, so
  the slowest tests run only in full mode.
  `task test`, `task verify` and CI run every test; the pre-push hook
  runs `verify:fast`, so its tests are the short ones (spec 0057).
  Full Go runs (`task test` and `task test:race`) have `-timeout 30m`,
  because a slow suite outgrows `go test`'s ten-minute default. In the
  monorepo reference the same holds for each Go component's tasks. In
  this repository, short mode skips three kinds of test (spec 0056): the
  tests that run the contract probes (`internal/contract/probetest`,
  the guard probes in `internal/contract`, the probe tests of
  `internal/cli`), the tests that run the guard corpus through
  subprocesses (`internal/guard`), and the tests that run the reference
  hooks (`internal/setupskill`: the Stop-hook matrix and the hooks run
  from a subdirectory). Each skip message names `task verify`, which
  runs them. Short mode is Go only: pytest and the TypeScript test
  runners have no common equivalent.
- **golangci-lint waits for its lock:** golangci-lint takes one lock in
  the OS temp directory, shared by every checkout on the machine, and a
  second run normally exits 3 with "parallel golangci-lint is running".
  `hook:check` runs in the background after every edit, and agents can
  work in several worktrees at once, so runs overlap. This repository's
  `.golangci.yml` and the setup skill's Go lint reference set
  `run.allow-serial-runners: true` (spec 0056), so
  a run that finds the lock taken waits for it instead. The setting is in
  the config file, so it covers every caller: hooks, lefthook, CI and
  manual runs. Overlapping runs run one after another; nothing debounces
  them.
- **TypeScript:** ESLint `--cache`, Prettier `--cache`, and
  `tsc --incremental` keep their caches under `node_modules/.cache/`,
  which a Node repository already ignores. ESLint's cache doesn't track
  links between files, so a type-aware rule can report a stale result
  locally until the file itself changes. CI starts cold and is unaffected.
- **Python:** Ruff caches by default (in `.ruff_cache/`, which ignores
  itself). `pyright` and `pytest` have no changed-only mode and run whole.
- **`pnpm test` is yours.** The TypeScript reference doesn't choose a
  test runner. If you
  use Vitest or Jest, put its changed-only mode in your own `test` script
  (`vitest related`/`--changed`, `jest --findRelatedTests`/`--changedSince`).

`task verify` remains the full gate, and CI runs it.

### `hook:context` (session start)

Plain text on stdout, which Claude Code adds to the model's context:

```console
$ task hook:context
Repo: VibeConform
Platform: windows/amd64
Runtime: native
Shell: unknown (not reported by the harness)
Branch: feature/agent-lifecycle-hooks
State: dirty
Go: go1.27.0
Task: 3.53.1
Dirty files: 2
 M internal/guard/rules.go
?? docs/notes.md
Affected components:
 100.0% internal/guard/
```

`Platform` comes from Task itself (`{{OS}}/{{ARCH}}`), so it can't fail,
and it is printed even outside git. `Runtime` is `wsl` or `container`
only when a marker file exists (`/proc/sys/fs/binfmt_misc/WSLInterop`,
`/.dockerenv`, `/run/.containerenv`). Otherwise it is `native`, meaning no
marker was found. `Shell` is always `unknown`. The harness doesn't report
which shell the agent's command tool uses, and the OS doesn't settle it,
so the line tells the agent not to assume. A linked worktree adds
`Worktree: linked (main checkout at <path>)`.

`State` also names a rebase, merge, cherry-pick, revert, or bisect in
progress. The runtime lines depend on the reference Taskfile: `Go` for
Go, `Node` and `pnpm` for TypeScript, `uv` and `Python` for Python. A missing
tool shows as `missing`. The list stops after 20 files with `… and N more`.

It only looks. It never installs dependencies, builds, runs tests, starts
services, or runs migrations; those stay explicit tasks. It doesn't repeat
`AGENTS.md`, which the agent loads itself, and it always exits 0.

`git status` is fast even on large repositories (about 85 ms on a
100,000-file checkout with Git for Windows' default settings). If yours is
slower, enable git's own speed-ups, `core.fsmonitor` and
`core.untrackedCache`. VibeConform doesn't set them for you.

### Formatting after each edit

The reference PostToolUse hook runs `task fmt 1>&2 || exit 2` after each
`Write`, `Edit`, `MultiEdit` or `NotebookEdit`: the repository's own
`fmt` entrypoint, which contract rule A-06 asks for. It prints nothing
when it succeeds. A formatter that fails, on a syntax error for
instance, exits 2 and the agent sees the error. After it runs, the
agent's copy of the file is out of date until it reads the file again.

This repository runs `task -x hook:format` instead. It formats only the
files git reports as changed (the `fmt:changed` task: `goimports -w`,
then `gofmt -w`, on the changed `.go` files), so the hook stays fast. The
audit's A-06 looks for a hook that calls `fmt`, so it reports this
repository's hook as a SHOULD failure; that difference is known.
`hook:format` starts `fmt:changed` as a `task` process of its own, in the
root, so it works from any directory (#129): Task's built-in `xargs`,
which Windows uses, starts the formatter in the directory `task` was
started from, where the root-relative file names don't resolve. Run
`fmt:changed` from the root for the same reason.

### `hook:check` (after each edit, in the background)

Runs `typecheck` and `lint` (in `verify`'s order), without
blocking the agent. It runs no tests (spec 0057): an edit in the middle
of a change rarely passes them, and the Stop hook runs them at the end of
the turn. It leaves out `fmt:check` because Claude Code runs an event's
hooks in parallel, so it starts while the format hook may still be
writing the file.

**Nothing runs in a sub-agent.** When the hook payload has a non-empty
`agent_id`, the edit came from a sub-agent, and `hook:check` exits 0 at
once. A failure would wake the parent session, not the sub-agent, and a
test-first sub-agent's red phase fails on purpose. A sub-agent's work is
checked by its parent's Stop hook. There is no `SubagentStop` gate.

Otherwise it reads the edited path from the hook payload on stdin
(`tool_input.file_path`, or `notebook_path` for a notebook edit) and asks
`git check-ignore` about it. It runs nothing and exits 0 when the edit
cannot change the checks' result:

- the path is outside the repository (the agent's memory, its
  scratchpad), or
- git ignores the path.

A tracked file, or an untracked file git doesn't ignore, gets the checks.
So does an edit whose payload has no path it can read (a malformed
payload, a tool with no path), and any edit outside a git repository.
Every reference Taskfile does this. It uses only Task's shell and git,
no `jq`.

Claude Code runs it with `asyncRewake`: on failure (exit 2) it wakes the
session straight away and shows the output as a system reminder. A plain
`async` hook would only deliver JSON `additionalContext` or
`systemMessage` output, and only on the next turn.

Quick successive edits start overlapping runs. The path filter makes
each run cheaper; nothing debounces them (spec 0056).

### `hook:done` (end of turn)

The Stop hook's command is

```sh
vibe-guard relock 2>/dev/null; cd "${CLAUDE_PROJECT_DIR:-.}" && {
  task -x hook:stamp:check >/dev/null 2>&1 </dev/null ||
  { task verify:fast 1>&2 </dev/null && task -x hook:stamp:save >/dev/null 2>&1 </dev/null; } ||
  task -x hook:done; }
```

(on one line in the settings). `vibe-guard relock` first ends any unlock
(see "`vibe unlock`"), before the `cd`, so it runs even when the project
directory cannot be entered. It runs `task verify:fast`, with the
tools' output on stderr, where Claude Code shows it to the model. If that
passes, the stop goes through. If it fails, `hook:done` exits 2 and the
agent can't stop: Claude Code keeps working with the failure as its
reason. The settings call `verify:fast` directly, rather than through
`hook:done`, so `vibe audit` can see that the Stop hook runs it (contract
rule A-05, spec 0052). `</dev/null` keeps every task but `hook:done` away
from the hook's payload, which `hook:done` reads. The command is POSIX
shell: Claude Code runs hook commands in `sh` or `bash` on Linux and in
Git Bash on Windows.

**The green stamp** (spec 0057). After a green `verify:fast`,
`hook:stamp:save` records a hash of the tree: HEAD, the changes against
it (staged or not), and the names and contents of untracked files git
doesn't ignore. The next stop starts with `hook:stamp:check`: when the
tree still has that hash, the stop goes through without running
`verify:fast`. A turn that only read files, planned, or waited for
sub-agents then costs nothing. Any edit, staged file, new untracked file
or commit changes the hash, so the checks run again. A failing run never
records a stamp, so a failure always runs again. The stamp is a file in
the worktree's own Git directory (`git rev-parse --git-path
green-stamp`): it never shows in `git status`, and each linked worktree
keeps its own. A project at the root of its Git repository uses
`green-stamp`. A project in a subdirectory of another repository's
working tree, such as an example in `examples/`, uses
`green-stamp-<prefix>`, where the prefix is `git rev-parse --show-prefix`
in the Taskfile's directory with each `/` turned into `-`
(`green-stamp-examples-python`). So a green run in one project never lets
another one's stop through (spec 0059). Outside a Git repository there is
no stamp, and every stop runs the checks. The pre-push hook doesn't read
the stamp. Delete the file to force a run.

It blocks **once per stop**. If the agent stops again with the checks
still failing, the payload's `stop_hook_active` is `true`, and the stop
goes through with the failure printed. That keeps a failure the agent
can't fix (a flaky test, a missing tool) from looping forever. Claude Code
also has its own cap of eight blocks. It's a deliberate
soft spot: CI is still the authoritative gate, and the contract audit
isn't part of the Stop hook at all.

To try the gate by hand, give the part after the stamp check a payload
on stdin. `hook:done` reads stdin, so with nothing piped it waits:

```console
$ echo '{"stop_hook_active": false}' | sh -c 'task verify:fast 1>&2 </dev/null || task -x hook:done'; echo $?
0
```

### Checking the hooks fire

As for the guard, no automated check can see an agent's own hook dispatch.
In the session you mean to rely on:

- **Session start:** ask the agent what branch it's on without letting it
  run anything. It should know from the context.
- **Format:** ask it to write a badly formatted file; it should come back
  formatted.
- **Check:** break a test in an edit; the failure should reach the agent
  without it running the tests.
- **Stop:** leave the test broken and let it finish; the first stop should
  be refused, and the second let through.

**Known limitations** of these four:

- **A Taskfile that won't load turns them off.** A YAML error in
  `Taskfile.yml` stops Task before the hook task runs, and Claude Code
  treats any exit other than `2` as a non-blocking error. `task verify`
  breaks at the same moment, so it rarely goes unnoticed for long.
- **The Stop gate is only as fast as your cached tests.** Go re-checks
  every file and environment lookup a cached test made before it reuses
  the result, so a suite that looks up many paths is slow even when fully
  cached.
- **Pre-existing failures block the first stop too.** `verify:fast` checks
  the whole repository, not only what the agent changed. The second stop
  goes through.

### The agent guard

Contract rules A-02 and A-03 ask for an agent guard: a PreToolUse hook
that refuses a short list of destructive commands (`rm -rf`,
`git reset --hard`, `git push --force`, `git restore .`, and a few more)
and edits to files that conventionally hold secrets (`.env`, `*.pem`,
`id_rsa`, …), in every spelling the tool itself accepts: `rm -r -f`,
`rm --recursive --force`, `git -C dir reset --har`, `git push origin
+main`, `git branch -d -f`, and PowerShell's `ri -Rec -Fo` all count. The
audit checks this by probe: it runs the guard corpus through the hook
(see "`vibe audit`"). Any guard that passes will do; `vibe-guard`, below,
is the reference, and the setup skill installs it.

**It is not a sandbox.** The guard stops common accidents. It is not a
security boundary: a command it does not recognize runs. For hard
limits, use Claude Code's permission `deny` rules and its sandbox. When
the guard refuses a command you do want, run it yourself: in Claude
Code, `! <command>` runs it in the session without the guard.

The guard is the agent's, not yours: personal, uncommitted settings
(`.claude/settings.local.json`) may loosen it, and the audit never reads
them. Committed loosening goes through exceptions in
`.claude/hooks/policy.json`, which the audit lists one by one.

**Known limitations**, where the guard does *not* run, or allows what it
shouldn't:

- **Claude Code on Windows without Git Bash** runs hooks through
  PowerShell. If that shell cannot start, the hook never runs and the
  command is allowed
  ([anthropics/claude-code#90077](https://github.com/anthropics/claude-code/issues/90077)).
- **Other commands with the same effect are allowed.** The guard refuses
  respellings of its commands, not every way to do the same damage:
  `find . -delete`, `git checkout -f`, `git switch --discard-changes`,
  and deleting a remote branch (`git push origin --delete x`, `git push
  origin :x`) all run. A secret file written through the shell (`echo
  X=1 > .env`) or named without an extension
  (`deploy/secrets/db_password`) is not caught either.
- **Some commands are not read:** `cmd /c`, `Start-Process`, shell
  aliases, and glob expansion into flag-like file names.

**Checking it fires.** No automated check can see an agent's own hook
dispatch. The manual canary: ask the agent to run
`git branch -D some-branch-that-does-not-exist`. It should be refused
with `vibe-guard: refused by rule git-branch-force-delete: …`. Don't
assume when an agent picks up a changed configuration: Claude Code
switched to a new guard mid-session while this was being built, while
other agents or versions may only read it at startup. Run the canary in
the session you actually mean to rely on.

### `vibe-guard`, the reference guard

`vibe-guard` (spec 0053) is the guard contract 1.1.0's guard probes are
written against. It parses commands instead of matching text, so
respellings such as a quoted command name, `eval`, flags after many
operands or a variable in a flag are refused. It is a separate binary
from `vibe`, built from the same module and released in its own
archive, so it keeps working when `vibe` is removed. It opens no network
connection: CI runs its tests in a network namespace with no
interfaces. This repository runs it, and the setup skill's reference
`.claude/settings.json` does too.

**Install** it on `PATH`, from a checkout of this repository or by
version:

```bash
go install ./cmd/vibe-guard
go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@<version>
```

Install it *before* the settings that run it are in place: from then
on, a session without it has every tool call refused. The reference
hooks (`internal/guard`'s reference commands, which the setup skill's
`settings.json` holds):

```text
PreToolUse (Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit):
  command -v vibe-guard >/dev/null 2>&1 || { echo "vibe-guard is not installed; install it: go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@<version>, or download it from https://github.com/Manual-debuger/VibeConform/releases/tag/<version>" >&2; exit 2; }; vibe-guard hook
Stop:
  vibe-guard relock 2>/dev/null; cd "${CLAUDE_PROJECT_DIR:-.}" && { … task verify:fast … || task -x hook:done; }
```

`command -v` is a shell builtin, so a missing `vibe-guard` refuses every
call with the install line, even with only the system's `PATH`. The line
names `go install` and the release page, so a developer without Go can
follow it too (spec 0054). `<version>` is the version of the `vibe`
whose `vibe init --skill` wrote the skill, so the guard installed
matches it; a `vibe` built without a fetchable version (`dev`, one with
`+dirty`, or a pseudo-version) writes `latest`, and the link is then to
the releases page, `https://github.com/Manual-debuger/VibeConform/releases`.
The rule lives in `internal/guard` (`InstallVersion`), beside the
reference commands.

**Commands:**

- `vibe-guard hook` reads Claude Code's PreToolUse payload on stdin.
  Exit `0` allows; exit `2` refuses with one line on stderr,
  `vibe-guard: refused by rule <id>: <why>. Explain: vibe-guard explain
  '<command>'`. A rule at level `ask` exits `0` with Claude Code's
  `permissionDecision: "ask"` JSON on stdout.
- `vibe-guard explain [--tool <tool>] '<command or path>'` prints the
  decision, the rule ID, the simple command and words (or the path) that
  matched, and any exception that applied, and exits as `hook` would. The
  tool defaults to `Bash`, or `Write` for one word that names an existing
  path.

  ```console
  $ vibe-guard explain 'rm -rf /'
  tool: Bash
  command: rm -rf /
  decision: deny
  rule: rm-rf
  why: rm with -r and -f deletes a directory tree without asking; …
    matched simple command (Bash): rm -rf /
    words: [rm] [-rf] [/]
  ```

- `vibe-guard relock` deletes the unlock marker; the Stop hook runs it.
- `vibe-guard version`.

**What it reads.** The repository root is `CLAUDE_PROJECT_DIR`, else the
payload's `cwd`, walked up to `.git`. Bash commands are parsed as bash,
PowerShell commands by a small tokenizer (aliases and parameter prefixes
included). Wrappers (`sudo`, `env`, `xargs`, `find -exec`, `timeout`, …)
are stripped and `bash -c`, `eval` and `pwsh -c` strings parsed again,
eight levels deep. It **fails closed**: a command that does not parse
(`parse-error`), a payload that is not JSON or lacks its field
(`payload-error`), an invalid `policy.json` (`policy-error`) and a command
name built from an expansion (`dynamic-command`, also `curl … | bash`)
are refused. These IDs take no exception.

**`policy.json` version 2.** `vibe-guard` reads only
`.claude/hooks/policy.json`, never `policy.local.json`:

```json
{
  "version": 2,
  "mode": "enforce",
  "rules": [
    {"id": "terraform-destroy", "level": "deny", "reason": "destroys infrastructure",
     "command": "terraform destroy"},
    {"id": "migrations", "level": "ask", "tools": ["Write", "Edit"], "reason": "reviewed migrations",
     "path": "db/migrations/**"}
  ],
  "exceptions": [
    {"rule": "git-clean-fd", "match": "git clean -fdx build/", "reason": "build output"}
  ]
}
```

- `mode`: `enforce` (the default) or `observe`.
- A rule has an `id` that is none of the guard rule IDs, a `level`
  (`deny` or `ask`), optional `tools` (default: every tool of its kind),
  a `reason`, and exactly one of `command` (a word prefix, as exceptions
  use) or `path` (a glob). A command rule may add `args`, a regular
  expression some remaining argument must match.
- An exception names one of the ten contract rule IDs and lifts it for
  one simple command (`match`, compared word by word, never through an
  expansion) or one path glob. Every other rule still applies.
- An unknown key or an invalid value is a policy error, and every call
  is refused with a message naming the key: a typo cannot switch a rule
  off.

**The asymmetry.** To tighten, add `deny` or `ask` rules to
`policy.json`, or another PreToolUse hook to `settings.json`. To loosen,
there is only an exception in `policy.json`, which `vibe audit` lists
with every corpus case it lets through. Personal loosening belongs in
your own `.claude/settings.local.json`, which nobody else gets.

**Observe mode.** With `"mode": "observe"`, every call that would be
refused is allowed, with `vibe-guard (observe): would refuse by rule
<id>: …` on stderr and the same line appended to `vibe-guard.log` in the
common Git directory. It is for trying the guard out: a committed
`observe` fails the audit's A-02. A policy error is refused even then.

**Settings and hooks are locked** (`guardrail-settings`). An agent's edit
of `.claude/settings.json` or of anything under `.claude/hooks/` is
refused unless you unlocked it. `policy.json` stays refused even then.
Always refused: an agent's `vibe unlock`, and any command or edit that
names the `vibe-unlock` marker.

**Shell writes to these files** (spec 0058). `guard-policy-file` and
`guardrail-settings` also check every path a `Bash` or `PowerShell`
command writes, deletes, moves or renames, with the decision and the
exceptions an edit of that path gets: refused, or allowed while you
unlocked the settings and hooks (never `policy.json`). A command writes
a path when the path is:

- the target of an output redirection: `>`, `>>`, `>|`, `&>`, `&>>`,
  `n>`, `n>>`, `>&file`, and PowerShell's `>`, `>>`, `n>`, `*>`. An
  input redirection (`<`) is a read.
- a file operand of `tee`, of `sed -i`/`--in-place` (any suffix), or of
  `truncate`;
- the destination of `cp`, `mv`, `install` or `ln` (the last operand, or
  the directory after `-t`/`--target-directory`), and the source of `mv`.
  A destination may be a directory, so the source's name inside it
  counts too: `cp backup/settings.json .claude/` writes
  `.claude/settings.json`;
- an operand of `rm` or `unlink`, or `dd`'s `of=`;
- in PowerShell, the `-Path`, `-LiteralPath`, `-FilePath` or first
  positional argument of `Set-Content`, `Add-Content`, `Out-File`,
  `Clear-Content`, `Remove-Item`, `Rename-Item` (and the new name),
  `New-Item` (and `-Name`) and `Tee-Object`; the `-Destination` (or
  second positional argument) of `Copy-Item` and `Move-Item`, and
  `Move-Item`'s source. `rm`, `cp`, `mv` and `tee` are read as the
  cmdlet and as the program;
- any of these found through a wrapper or a nested shell (`sudo`, `env`,
  `xargs`, `find -exec`, `bash -c`, `pwsh -c`, …).

A relative path is resolved against the payload's `cwd` (for `explain`,
against the root), and Git Bash's `/d/…` is read as `D:/…`. A read
(`cat`, `jq . <`, `Get-Content`) is allowed, and so is a write to any
other file: `secret-file` stays an edit rule, so `cp .env.example .env`
runs. `explain` shows the rule, the matched path and the simple command
that writes it. **Not caught** (the agent guard is not a security
boundary): a written path that holds an expansion (`> "$f"`), a `cd`
earlier in the same command, writes made by an interpreter (`python -c`,
`node -e`, …) or by any program not listed above, and `git checkout` or
`git restore` of a path.

### `vibe unlock`

To let an agent change the guardrail settings for a while, run this in
**your own terminal**, not through the agent:

```console
$ vibe unlock --minutes 15
unlocked until 2026-10-17T14:05:00Z (/path/to/repo/.git/vibe-unlock)
…
```

- `--minutes N`, 1 to 240 (default 15); `--repo-root` (default `.`).
  It writes the expiry, RFC 3339 UTC, into `vibe-unlock` in the common
  Git directory, so every linked worktree shares it. Running it again
  replaces the expiry.
- The agent's next Stop locks again (`vibe-guard relock` in the Stop
  hook). To lock earlier, run `vibe-guard relock` yourself.
- An agent's `Bash` or `PowerShell` call of `vibe unlock` is refused.
  Whether Claude Code's `! <command>` passes through the PreToolUse hook
  is not verified, so use a terminal of your own.

**The canary with `vibe-guard`.** Ask the agent to run `git branch -D
some-branch-that-does-not-exist`: it should be refused with
`vibe-guard: refused by rule git-branch-force-delete: …`. Ask it to run
`vibe unlock`: refused under `guardrail-settings`. After installing or
upgrading `vibe-guard`, run the canary in the session you rely on.

### Codex: hooks suspended

Contract 1.1.0 has no Codex rules (ADR 0025), and the setup skill writes
no Codex hooks. This repository keeps `.codex/config.toml` (approval
policy and sandbox mode) and a `.codex/hooks.json` with no hooks (spec
0024):

```json
{
  "hooks": {}
}
```

A Codex session therefore gets no guard, no session context, no
formatting, and no `Stop` gate.
`AGENTS.md` still applies, and CI is still the gate.

Why: a live test of Codex 0.156.1 found that on Windows it ignores exit
code 2, so neither the guard's deny nor the `Stop` gate's block takes
effect, and that on every platform a background hook's stderr never
reaches the model. Hooks written for Claude Code's contract don't carry
over. See `docs/decisions/0011-one-module-per-agent-runtime.md` for the
results and what resuming would take.

`.codex/config.toml` doesn't set `hooks = true`, and deliberately
doesn't set `hooks = false` either, so hooks you configure in your own
`~/.codex/` keep running.

## Git hooks

Contract rules H-01 and H-02 (both SHOULD) ask for committed Git hooks:
pre-commit checks formatting, pre-push runs verification. The setup skill's reference `lefthook.yml`
files (`reference/hooks/`) have two hooks, and each calls an entrypoint
rather than a tool (spec 0057), so the checks live in the Taskfile
alone:

| Hook | Runs | When |
|---|---|---|
| pre-commit | `task fmt:check` | a staged file has an extension `task fmt` covers |
| pre-push | `unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR; task verify:fast` | every push |

pre-push runs the fast verification, not the full suite: in Go the
tests run in short mode. CI's test jobs are the full gate, and
`task test` runs every test by hand. pre-push doesn't read the agents'
green stamp; it always runs.

**Why `unset`.** A hook that Git starts from a linked worktree gets
`GIT_DIR` exported to it. A test that runs `git` in a temporary fixture
would then act on your real repository. pre-push clears Git's repository
variables before `verify:fast`, so your tests don't each have to.
lefthook runs `run:` with `sh` on every platform, Windows included.

In the monorepo reference both hooks call the root Taskfile, which runs
each component's `fmt:check` or `verify:fast`; pre-commit fires for any
component's file extensions. Run `lefthook install` once per clone to
register the hooks.

## This repository

VibeConform is checked by its own contract audit. Its `vibe.yaml` is the
version 2 file shown under "`vibe.yaml` version 2", and every file in the
repository is an ordinary file, edited directly.

- `task audit` builds `vibe` and `vibe-guard` from the checkout into the
  worktree's Git directory, puts that directory first on `PATH`, and runs
  the full `vibe audit`. The probes therefore use this checkout's
  `vibe-guard`, and a stale `vibe` installed elsewhere never gives the
  verdict. `task audit:static` runs `vibe audit --static` with the
  checkout's `vibe`; it exits `0` when no MUST rule fails, since a static
  audit never reaches a verdict.
- `.github/workflows/contract-audit.yml` runs the same full audit as the
  required check `Contract / audit`, next to `CI / gate`. Its
  `Contract / audit (static)` job is not required.
- `task verify` never runs `vibe` (spec 0017), so run `task audit` too
  before calling a change done.
- `.github/workflows/examples.yml` copies each of `examples/typescript`,
  `examples/python` and `examples/monorepo` into a fresh Git repository
  and runs the full audit there, on Linux and Windows; each must be
  conformant. Its GitLab job audits the monorepo example with the
  reference `.gitlab-ci.yml` in place of the GitHub workflows (the real
  GitLab pipeline is not run, #74).
- `Taskfile.local.yml` holds this repository's own tasks (`task build`
  and others), included optionally by `Taskfile.yml`.

## Removing VibeConform

Nothing a conformant repository runs calls `vibe` (rule R-01), so its
tasks, Git hooks and CI keep working without it. To remove VibeConform,
delete:

- `vibe.yaml`;
- the audit job: `.github/workflows/audit.yml` in the reference layout,
  or the audit job of `.gitlab-ci.yml` on GitLab CI. Also drop its check
  from your branch protection's required checks, or pull requests will
  wait for a check that no longer runs;
- the setup skill: `.claude/skills/vibeconform-setup/` and
  `.agents/skills/vibeconform-setup/`.

`vibe-guard` is a separate binary and does not need `vibe`, so the agent
guard can stay as it is. To drop it too, remove the PreToolUse hook that
runs it and the `vibe-guard relock` at the start of the Stop hook from
`.claude/settings.json`, and `.claude/hooks/policy.json`.

## Leaving the template-sync system

Before spec 0059, `vibe sync` generated a repository's tooling from a
standard (`prod-go`, `prod-ts`, `prod-py` or `prod-mono`) and recorded
it in `.vibe/state.yaml`. v0.13.x is the last release with that system.
This `vibe` no longer reads a version 1 `vibe.yaml` (one with a
top-level `standard:` key): `vibe audit` and `vibe doctor` exit `1` with
a pointer, `vibe init` refuses to overwrite it, and `vibe sync`,
`vibe diff` and `vibe check` only print the pointer.

To move such a repository to the contract, run `vibe init --skill` and
ask the agent to set the repository up. The setup skill detects the old
system and follows its migration guide first,
`.claude/skills/vibeconform-setup/reference/migrate.md`: it merges each
generated base file with its local extension, folds
`Taskfile.local.yml` into `Taskfile.yml`, removes `.vibe/`,
`Taskfile.vibe.yml`, the conformance workflow and the managed-section
markers, writes a `vibe.yaml` version 2, and then runs the audit loop.
The files the old system wrote become the repository's own.

## Getting help

```bash
vibe --help
vibe audit --help
vibe audit --explain <rule ID>
vibe-guard explain '<command>'
```
