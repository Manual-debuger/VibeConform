# ADR 0025: The Contract Model Replaces Desired State

## Status

Accepted (2026-10-16). Issue #95. Supersedes ADR 0002, ADR 0003 and
ADR 0009. Generalizes ADR 0024. The rules themselves are in
`docs/specs/0051-contract-v1.md` (issue #96). The work is tracked as
issues #95–#112, milestones "Contract v1 · Phase 0" to "Phase 4".
Settled with the user on 2026-10-16 in a grilling session; "Qn" below
refers to its questions, which the issues also cite.

## Context

VibeConform generates an adopting repository's tooling from a standard
and records a hash of each managed file in `.vibe/state.yaml` (ADR 0002,
ADR 0003). `vibe audit` compares the two. That has three costs:

- Any customization is drift. Each seam we added to stop that (ADR 0009,
  ADR 0017, ADR 0021, ADR 0024) only covers one file or one tool.
- An adopter cannot choose its own tools. A standard fixes the linter,
  the hook manager and the CI layout, so a team that already uses
  pre-commit or make has to switch or fall out of conformance.
- Hash checks only see managed files. For example, an unpinned
  `goimports@latest` in this repository's `hook-guard.yml` passes the
  audit because that workflow is not generated.

ADR 0024 already split the two kinds of content. The floor is fixed;
rule content is the repository's to adjust. This ADR applies the same
split to the whole repository.

## Decision

1. **The contract defines properties, not files.** The contract is a
   versioned list of rules a repository must satisfy. Each rule has a
   stable ID, a level (MUST or SHOULD) and a check (static, execution
   or probe). It never names a tool or a file's content. The floor of
   ADR 0024 becomes the contract; everything else is the repository's.
2. **Three roles.** A setup skill gives a repository those properties,
   in any reasonable way. The contract states them. `vibe audit` only
   verifies them, deterministically, and never writes a file.
3. **Behaviour is checked by probes.** Where a property is about what a
   tool does, the audit feeds a known input and checks the reaction.
   It does not read configuration text. Examples: the guard corpus for
   the agent guard, and planted defects for `verify`. This is what lets
   the implementation be free.
4. **The audit does not depend on a task runner.** A repository declares
   its entrypoints (verify, verify-fast, fmt, fmt-check) as commands in
   `vibe.yaml`. Task, make, just and npm scripts all qualify. Hook task
   names are not part of the contract (Q5).
5. **The contract uses semver.** `vibe.yaml` declares the version a
   repository follows. A change that makes a conformant repository
   non-conformant is MAJOR. A new SHOULD rule or a relaxed rule is
   MINOR. A wording fix is PATCH. A vibe release implements the latest
   MINOR of each MAJOR it supports (Q15).
6. **Not applicable and waivers.** A repository declares its platforms.
   A rule that does not apply, such as Windows CI for a Linux-only
   repository, is reported as not applicable. Only a SHOULD rule can be
   waived, with a reason and an expiry date (Q9).
7. **Codex is exempt for now.** The agent rules cover Claude Code only.
   Bringing Codex in later is a MAJOR change.
8. **The guard parses shell.** The reference guard parses commands with
   `mvdan.cc/sh` instead of one large regex, and refuses a command it
   cannot parse. It is a single prebuilt Go binary, `vibe-guard`,
   released separately from `vibe` so it works when `vibe` is removed.
   `mvdan.cc/sh` becomes a production dependency of `vibe-guard`. Task
   already depends on it, and a regex cannot follow quoting, so
   spellings slip through (spec 0047).
9. **A missing guard fails closed** (Q12). The hook command checks that
   `vibe-guard` is installed. If it is not, it prints how to install it
   and exits 2. A shell's "command not found" (127) would otherwise
   read as allow to Claude Code.
10. **Loosening the guard is explicit.** It is only possible through
    exceptions in the guard policy, which the audit lists one by one.
    Tightening is free: add deny rules or another PreToolUse hook.
    Personal, uncommitted settings may override any guardrail. The
    audit does not check them, because the guardrails exist to stop
    agent mistakes, not human decisions.
11. **Agent guardrail settings need an unlock** (rule A-08). An agent
    cannot change `.claude/settings.json` or `.claude/hooks/` unless a
    person ran `vibe unlock`. The unlock expires, and an agent cannot
    grant it to itself.
12. **One fixed defect per language** (Q16). Mutation probes plant one
    static-analysis defect that typecheck or lint always catches:
    for Go, a `go vet` printf mismatch; for Python and TypeScript, an
    undefined name. Lint rule content is the repository's choice, so
    any other lint defect might be legitimately allowed. This is the
    one lint floor the contract implies.
13. **Probes plant next to existing tests** (Q17). A component with no
    test file fails, because nothing shows that `verify` runs tests.
14. **No delegation to OpenSSF Scorecard** (Q20). The rules on pinning,
    workflow permissions and update bots are implemented in vibe.
    Scorecard needs the network and the GitHub API, and its criteria
    are not versioned with our contract, so the result would stop
    being deterministic.
15. **The setup skill lives in the repository** (Q22). `vibe init`
    writes it into the repository's skill path, and from then on the
    repository owns it. This needs no plugin mechanism and works for
    any harness's skill path.
16. **Prebuilt binaries.** `vibe` and `vibe-guard` are released for
    Linux, macOS and Windows on amd64 and arm64, with checksums and
    provenance attestations. An adopter's CI does not need Go.
    Building from source stays possible.

## Consequences

- `.vibe/state.yaml`, three-way reconciliation, `vibe sync` and
  `vibe diff` are no longer needed, and #112 removes them. The module
  templates become the setup skill's reference implementations.
- This repository's generated files become ordinary repository-owned
  files when the new audit is required (#111). AGENTS.md drops its
  "managed files / `vibe sync`" rules (#112).
- Integrations (graphify, editors, LSP) and the AGENTS.md workflow
  section stop being generated, and become optional content the
  setup skill can write (Q23).
- The audit runs every verify entrypoint several times. The
  conformance job takes longer; `--static` gives a quick local run
  that cannot reach a verdict (Q18).
- Contract v1 covers GitHub Actions only. Other CI providers get
  "cannot decide" until a MINOR release supports them (Q19).
- An adopter that removes VibeConform keeps working tooling, because
  nothing the repository runs calls `vibe` (rule R-01).

## Alternatives considered

- **Keep desired state and add seams.** Each seam fixes one file. The
  cost grows with every tool, and an adopter still cannot choose
  tools.
- **A new repository for the contract model.** It would mean losing the
  guard corpus, the probes' prior art and the issue history. Keeping
  both systems side by side in one repository is only acceptable
  because #112 removes the old one.
- **LLM-judged readiness checks** (as some readiness scorers do). Their
  scores drift between runs; a gate must give the same verdict twice.
- **Keep three guard implementations (Go, Python, Node)** so each stack
  can run the guard without another runtime. A prebuilt binary removes
  that need, and three implementations of one parser would not stay in
  step.
