# VibeConform Design Principles

Status: living document. These are the tests every spec, contract rule
and setup-skill reference is held to. `docs/architecture/overview.md` describes *how*
VibeConform works; this file describes what it is *for*, and so what a
change must not trade away. When a proposal conflicts with one of these,
the spec says so and says why, rather than quietly bending it.

There are three, in priority order.

## 1. If a rule can be expressed mechanically, do not leave it only in an AI prompt

Agent instructions (`AGENTS.md`, `CLAUDE.md`) are for judgment: what to
read first, how to plan, when to ask. Anything a machine can check belongs
in something a machine runs — `Taskfile.yml`, a linter configuration, a git
hook, a CI job, a test, or `vibe audit` — and prose that restates it is a
pointer to the tool, not a substitute for it.

The reason is that a prompt is advisory and a check is not. An instruction
an agent skips fails silently; a check it trips fails loudly, the same way
for a human, a different agent, or a CI runner that never read the prompt.

How to apply:

- Before writing a rule into `AGENTS.md`, ask whether a task, lint rule,
  hook, test, or audit could enforce it. If so, add that, and let the prose
  name it. The workflow sections the setup skill offers for `AGENTS.md`
  (`reference/optional/workflow.md`) are held to the same test: they
  route to documents and name tasks, and restate nothing a tool
  enforces.
- A guardrail that can be disabled without anything noticing is a gap, not
  a guardrail. Where one exists, document it next to the mechanism (as
  `docs/usage.md` does for the agent guard's known limitations) so the gap
  is a known quantity.
- Fail loudly and fail closed. A removed command returns an error that
  says where to go rather than exiting 0; a missing tool is named rather
  than skipped; a rule the audit cannot decide is "cannot decide", never a
  pass; a missing `vibe-guard` refuses every tool call.

## 2. Easy to use, on Windows and Linux alike

Both platforms are first-class, not "Linux, and Windows if it happens to
work." A repository that conforms on one must conform on the other, byte
for byte, and every entrypoint the setup skill's references write must
run on both without per-OS branching.

How to apply:

- The reference commands go through Task, whose built-in POSIX shell
  (`mvdan.cc/sh`) behaves identically on every platform. Do not write
  `.sh`-only or `.ps1`-only entrypoints for anything a contributor is
  expected to run. Hook commands are POSIX shell, which Claude Code runs
  in Git Bash on Windows.
- Paths are slash-separated everywhere they identify something (component
  paths in `vibe.yaml`, the tracked files the audit reads) and converted
  to host paths only at the I/O boundary.
- `* text=auto eol=lf` in `.gitattributes` keeps checkouts the same on
  both platforms, so it is something git enforces, not an instruction for
  an agent to remember.
- Anything Unix-only (file mode bits, extensionless binaries) is called
  out in `docs/usage.md` together with what a Windows user sees instead.
  Agent hooks are the worked example: they were bash scripts until spec
  0021, then ran through Task, and the guard is now one prebuilt binary,
  `vibe-guard`, on every platform.
- Output and errors say what happened and what to run next. Exit codes are
  part of the contract (`internal/cli/exit.go`), so scripts and CI never
  have to parse prose.

The cross-platform contract, stated once (spec 0028):

- Task is the canonical operational interface on every platform.
- Supported developer platforms are Windows and Linux. CI runs Ubuntu,
  and Windows where a workflow's matrix says so.
- Differences are surfaced, not guessed away. `hook:context` reports the
  platform, the runtime (WSL or a container, from marker files only) and
  linked worktrees. `vibe doctor` reports whether this machine has the
  programs the entrypoints call, the bash the hooks need and
  `vibe-guard`. Neither infers what it can't observe: the shell is
  reported as unknown.
- CI is the independent, clean-environment authority. A local `PASS`
  never replaces it.

## 3. Easy to install and easy to remove

What the setup skill writes into a repository must stand on third-party
tools the ecosystem already uses — `task`, `lefthook`, `go`,
`golangci-lint`, `uv`, `ruff`, `pyright`, `npm`/`pnpm`, `eslint`,
`prettier`, `tsc`, GitHub Actions or GitLab CI — not on `vibe` itself.
VibeConform checks that configuration; it does not become a runtime
dependency of the repository it checks.

The test: delete VibeConform from a repository, and everything the
repository runs keeps working, because it is ordinary project
configuration the repository owns.

How to apply:

- A command that can be expressed with a third-party tool must be. The
  entrypoints, Git hooks and every CI job but the audit's depend on
  native language tooling only, never on `vibe` (spec 0017; contract rule
  R-01 checks it).
- Where `vibe` is genuinely necessary — the audit itself — confine it to
  one CI job, removable on its own. `docs/usage.md`'s "Removing
  VibeConform" section is the contract, and every step it lists is a cost
  a change should try to reduce, never add to.
- Pin what runs `vibe`: the audit job installs an exact version with the
  setup action (checksums checked), never `latest`, so a verdict cannot
  change without a change to the repository.
- `vibe` checks toolchains; it does not provision them. `vibe audit` and
  `vibe doctor` name a missing tool rather than install it.
- Installing `vibe` needs nothing beyond what the ecosystem already offers:
  `go install`, `go run`, or a prebuilt release archive. `vibe-guard` is
  released on its own, so the agent guard keeps working without `vibe`.

## Where these principles already show up

| Principle | Mechanism |
|---|---|
| 1 | The entrypoints declared in `vibe.yaml` are the canonical verification interface; CI and hooks call them instead of duplicating command lists, and rules V-04, H-02, C-01 and A-05 check that they do |
| 1 | `vibe audit` checks behavior by probe: a planted defect `verify` must catch, the guard corpus the agent guard must refuse |
| 1 | Claude Code `PreToolUse` hooks block destructive commands at the tool-call level, not by asking nicely (Codex hooks are suspended: on Windows Codex ignores the deny, spec 0024) |
| 1 | The `Stop` hook runs `task verify:fast` before an agent may end its turn, instead of `AGENTS.md` asking it to (spec 0023). Its known soft spot: a second stop with the checks still failing goes through, so a failure the agent can't fix doesn't loop forever; CI remains the gate |
| 1 | The `examples.yml` workflow audits the three examples, built from the setup skill's references, so references this repository cannot dogfood stay conformant |
| 2 | Task's portable shell, slash-separated paths, and CI's `ubuntu-latest`/`windows-latest` matrix (rule C-05 asks adopters for the same) |
| 3 | `task verify` has no `vibe` dependency (spec 0017), and rule R-01 fails a repository whose entrypoints, hooks or CI (apart from the audit job) call `vibe` |
| 3 | The audit job is the only place `vibe` runs, so removal is deleting files; `vibe-guard` is a separate binary that survives it |
