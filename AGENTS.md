# Agent Instructions

This repository's own rules. The "Repository workflow" section at the end
routes to `docs/`.
Don't include yourself as contributor in commits.

## Before non-trivial work

1. Read `docs/architecture/overview.md` and any relevant `docs/decisions/*`
   before changing package boundaries, the contract or the guard, and
   `docs/architecture/principles.md` before changing what `vibe` checks
   or what the setup skill's references write.
2. Write the spec with `/to-spec`. Commit it under `docs/specs/` and
   publish it in the issue tracker, which links to the file. Its
   "Implementation Decisions" section serves as the plan, so new work
   has no separate `docs/plans/` file. The user approves the spec once.
   Then implement with `/implement-spec`: one integration branch and one
   PR per spec, with tests, verification and a documentation sync.
3. Preserve the dependency direction documented in
   `docs/architecture/overview.md` (e.g. `internal/contract` and
   `internal/guard` both depend on `internal/guardrule`, never on each
   other).

## Rules

- **`task audit` must pass.** It is the contract audit of this
  repository: it builds `vibe` and `vibe-guard` from the checkout, puts
  them first on `PATH` and runs the full `vibe audit` against
  `vibe.yaml`. It must say `Verdict: conformant`. CI runs the same audit
  as the required check `Contract / audit`. `task audit:static` is the
  quick run, which never reaches a verdict.
- **Guardrail files need a person's unlock.** Do not change
  `.claude/settings.json` or `.claude/hooks/` unless the user has run
  `vibe unlock` in their own terminal; the policy file
  `.claude/hooks/policy.json` is the user's to change in any case (see
  "Guardrails").
- Bug fixes require a regression test.
- Do not introduce a production dependency without recording why (an ADR
  under `docs/decisions/` for anything non-trivial).
- Run `task audit` as well as `task verify` before declaring work done.
  `verify` covers native language tooling only and never runs `vibe`
  (spec 0017), so a change that breaks a contract rule passes `verify`
  locally and fails only in `task audit`, or later in CI's
  `Contract / audit` check. The `Stop` hook runs `task verify:fast` at
  the end of every turn (spec 0023). That is the fast subset, not
  `verify`, and `task audit` is still yours to run.
- Use repository intelligence (GitNexus, if configured) when a change has
  cross-file impact — it augments the compiler/linter/tests, it does not
  replace them.
- Mechanical rules belong in tooling (`Taskfile.yml`, `.golangci.yml`,
  `lefthook.yml`, CI), not just in this file — see
  `docs/architecture/overview.md`'s "canonical verification interface"
  section.

## Guardrails

- This repository runs `vibe-guard`, the reference guard (spec 0053).
  Install it with `go install ./cmd/vibe-guard`; without it on `PATH`
  every tool call is refused with the install line.
  `.claude/settings.json` runs `vibe-guard hook` before each Bash,
  PowerShell, Write, Edit, MultiEdit and NotebookEdit call. It refuses
  destructive commands (`rm -rf`, `git reset --hard`, `git push --force`,
  PowerShell's recursive forced `Remove-Item`, `git restore .`, …) in
  every spelling it can parse, edits to secret-looking files, commands it
  cannot parse, and edits of `.claude/settings.json`, `.claude/hooks/`
  and the guard's policy, by an edit tool or by a shell command that
  writes them (spec 0058). When it refuses a command you need, ask the
  user to run it with `! <command>`; don't look for a spelling it misses.
  `vibe-guard explain '<command>'` says which rule matched. It is not a
  sandbox. Its rules are built into `internal/guard`; this repository's
  own rules and exceptions go in `.claude/hooks/policy.json` (version 2),
  which is the user's to change.
- To change settings or hooks, ask the user to run `vibe unlock` in
  their own terminal. Never run `vibe unlock` or touch the `vibe-unlock`
  marker yourself: the guard refuses it. Your next Stop runs
  `vibe-guard relock` and locks again.
- Known gaps (`docs/usage.md`, "`vibe-guard`, the reference guard"):
  `cmd /c`, `Start-Process`, shell aliases and glob expansion into
  flag-like file names are not read. A shell write to a guardrail file is
  not caught when its path holds an expansion (`> "$f"`), after a `cd`
  in the same command, through an interpreter (`python -c`, `node -e`)
  or a program not in the listed write forms, or by `git checkout` or
  `git restore` of a path.
- Codex hooks are suspended (spec 0024, ADR 0011): `.codex/hooks.json`
  has no hooks, so a Codex session here gets no guard and no `Stop`
  gate. Apply the same rules by judgment.
- The same settings run four more hooks (spec 0023, `docs/usage.md`,
  "Agent hooks"): session context at start, formatting of changed files
  after each edit, the incremental checks in the background, and
  `verify:fast` as a gate when you stop. A stop with failing checks is
  refused once. Their rules are the `hook:*` tasks in `Taskfile.yml`;
  the setup skill's reference Taskfiles under
  `internal/setupskill/skill/reference/taskrunner/` carry the same tasks
  for adopters.
- The guard is live in this repository: a Bash command that runs a denied
  command anywhere, even through `bash -c` or `eval`, is refused, and one
  that does not parse is refused too. Write such text with a file-editing
  tool rather than through the shell. After
  changing the guard, check it with the canary in `docs/usage.md` rather
  than assuming when the running agent picked the change up.

These are guardrails, not a complete enforcement boundary — apply the same
judgment regardless of which agent runtime is in use.

## Repository workflow

This section routes; the documents and tasks it names hold the detail.

Knowledge:

- Specs (what must be true) live in `docs/specs/`, architecture (how it
  works now) in `docs/architecture/`, decisions (ADRs) in
  `docs/decisions/`. Read the relevant ones before a non-trivial change.
- If an approved spec, an ADR and the code disagree, say so. Do not
  pick one silently.

Workflow: plan-triggered lightweight SDD.

- Normal mode: implement, then verify. Respect any spec that applies.
- Planning context (the harness's plan mode, or `/spec`): use the
  `spec` skill. List the constraints that apply and the assumptions
  you have not verified, then write a lightweight spec with acceptance
  criteria. Plan only after that.
- The spec says WHAT must be true; the plan says HOW to change the
  repository. Keep them apart. Reuse an approved spec when one exists.
- In a read-only plan mode, put the spec in the plan. Once it is
  approved, write it to `docs/specs/` first.
- Do not implement until the user approves.

Verification:

- `task verify:fast` while working; `task verify` before declaring
  done. `task verify` runs the tasks CI runs: `task fmt:check`,
  `task typecheck`, `task lint`, `task test`, `task mod:verify`,
  `task security` and `task workflows:lint`. `task fmt` formats. Do not
  weaken a test, lint or type check to make a change pass.
- Finish with a ledger, one line per check: PASS, FAIL or UNVERIFIED.
  Unit tests, CI and a real integration are separate lines.
