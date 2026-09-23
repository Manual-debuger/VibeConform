# VibeConform Design Principles

Status: living document. These are the tests every spec, module, and
template is held to. `docs/architecture/overview.md` describes *how*
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
  name it.
- A guardrail that can be disabled without anything noticing is a gap, not
  a guardrail. Where one exists, document it next to the mechanism (as
  `docs/decisions/0006-resource-file-mode.md` does for unaudited file modes)
  so the gap is a known quantity.
- Fail loudly and fail closed. A scaffolded command returns an error rather
  than exiting 0; a missing tool is named rather than skipped; a stale
  binary gets no verdict rather than a wrong one.

## 2. Easy to use, on Windows and Linux alike

Both platforms are first-class, not "Linux, and Windows if it happens to
work." A repository that conforms on one must conform on the other, byte
for byte, and every generated entry point must run on both without per-OS
branching.

How to apply:

- Generated commands go through Task, whose built-in POSIX shell
  (`mvdan.cc/sh`) behaves identically on every platform. Do not generate
  `.sh`-only or `.ps1`-only entry points for anything a contributor is
  expected to run.
- Resource paths are slash-separated everywhere they identify something
  (resource paths, `.vibe/state.yaml` keys) and converted to host paths
  only at the I/O boundary.
- Content is written exactly as resolved, with no line-ending translation;
  `* text=auto eol=lf` in `.gitattributes` keeps checkouts hashing the same.
- Anything Unix-only (file mode bits, extensionless binaries) is called
  out in `docs/usage.md` together with what a Windows user sees instead.
  Agent hooks are the worked example: they were bash scripts until spec
  0021 and now run through Task on each standard's own runtime.
- Output and errors say what happened and what to run next. Exit codes are
  part of the contract (`internal/cli/exit.go`), so scripts and CI never
  have to parse prose.

## 3. Easy to install and easy to remove

What VibeConform writes into a repository must stand on third-party tools
the ecosystem already uses — `task`, `lefthook`, `go`, `golangci-lint`,
`uv`, `ruff`, `pyright`, `npm`/`pnpm`, `eslint`, `prettier`, `tsc`,
GitHub Actions — not on `vibe` itself. VibeConform writes configuration for
those tools; it does not become a runtime dependency of the repository it
configured.

The test: delete VibeConform from a repository, and everything it generated
keeps working as ordinary project configuration, as if someone had written
it by hand.

How to apply:

- A generated command that can be expressed with a third-party tool must
  be. `task verify` and `task verify-ci` depend on native language tooling
  only, never on `vibe` (spec 0017), and the same applies to git hooks and
  every CI job except `conformance`.
- Where `vibe` is genuinely necessary — the conformance check itself —
  confine it: to as few files, tasks, and jobs as possible, each labeled as
  VibeConform's and removable on its own. `docs/usage.md`'s "Removing
  VibeConform" section is the contract, and every step it lists is a cost
  a change should try to reduce, never add to.
- Where `vibe` is necessary, arrange a fallback for when it is not
  installed: a documented way to obtain the right version without a prior
  install (for example `go run github.com/Manual-debuger/VibeConform/cmd/vibe@<version>`
  or a released binary), and a loud, explainable failure otherwise — never
  a silent pass.
- `vibe` configures toolchains; it does not provision them. The one command
  it runs on a repository's behalf, `lefthook install`, activates a file
  VibeConform just wrote, and a missing binary is a warning, not a failure
  (`docs/usage.md`, "What `prod-go/v1` manages").
- Installing `vibe` needs nothing beyond what the ecosystem already offers:
  `go install`, `go run`, or a prebuilt release archive.

## Where these principles already show up

| Principle | Mechanism |
|---|---|
| 1 | Generated `Taskfile.yml` is the canonical verification interface; CI and hooks call its tasks instead of duplicating command lists |
| 1 | `vibe audit` in CI turns "don't hand-edit managed files" from an `AGENTS.md` sentence into a failing job |
| 1 | Claude/Codex `PreToolUse` hooks block destructive commands at the tool-call level, not by asking nicely |
| 1 | `TestExamplesAreConformant` and the `examples.yml` workflow keep templates this repository cannot dogfood honest |
| 2 | Task's portable shell, slash-separated resource paths, byte-exact writes, and CI's `ubuntu-latest`/`windows-latest` test matrix |
| 3 | `task verify` has no `vibe` dependency (spec 0017); `Taskfile.local.yml` is plain Task `includes`, not a VibeConform mechanism |
| 3 | `sync` warns about missing tools but never installs them (spec 0014) |
