# Spec 0058: The guard and the hooks hold for shell writes and from any directory

Status: accepted (2026-10-25). Issues #123 and #133. Amends spec 0053
("guardrail-settings", "guard-policy-file") and the setup skill's
reference settings (spec 0054). No contract change.

## Problem

Two gaps let an agent get past the agent guardrails. Each one depends on
how the agent spells a call or where it runs it:

- **Shell writes to the guardrail files are allowed (#123).** After
  `vibe-guard relock`, `Edit` and `Write` on `.claude/settings.json` are
  refused (`guardrail-settings`), and so are `Edit` and `Write` on
  `.claude/hooks/policy.json` (`guard-policy-file`). A `Bash` call that
  writes the same files is allowed: `sed -i … .claude/settings.json`,
  `echo x > .claude/settings.json` and `tee .claude/hooks/policy.json`
  all get `decision: allow, rule: none`. A real `sed -i` changed the
  settings in a Claude Code session (pilot P1, v0.12.0-alpha.2). The
  problem is still there on main at a1283f3 (v0.13.0-alpha.1). The parser already reads
  redirection targets (`Command.Redirects`). Only the self-unlock rule
  uses them; the path rules read only the `Edit`/`Write` path.
- **The Task-based hooks break inside a component (#133).** Claude Code
  runs a hook command in the agent's current directory. Task looks
  upward for a Taskfile, so from `services/api/` it finds the
  component's Taskfile first. That file has no `hook:*` tasks. Claude
  Code blocks only on exit 2, and every hook exits 200 there:
  - With the old generated prod-mono settings, `task -x hook:guard`
    exits 200, so the guard is off.
  - With the setup skill's reference settings, the guard is called
    directly (`vibe-guard hook`) and still holds. But `hook:context`,
    `hook:check`, the green stamp tasks and `hook:done` exit 200. The
    Stop gate runs the component's own `verify:fast`, so the other
    components are not checked, and the gate never exits 2.
  - The same happens in a single-language repository with a nested
    Taskfile. This repository is one: `examples/monorepo/` has its own
    Taskfile.

## Constraints

- **No contract change.** The guard corpus belongs to the contract.
  Adding shell-write cases to it would make conformant repositories
  non-conformant, which is MAJOR (ADR 0025, decision 5). This spec
  changes `vibe-guard` and the reference files only.
- **A-05's static part** matches the literal simple command
  `task verify:fast` in the Stop hook command (spec 0057, constraint 1).
  A prefix on that same simple command, such as `task -d …`, stops it
  from matching. A separate simple command before it does not.
- **The agent guard is not a security boundary** (GLOSSARY). It refuses
  the commonest spellings. It does not refuse every command with the
  same effect, and its documented gaps stay documented.
- **Fail closed where the guard already does.** A command the guard
  cannot parse is still refused. This spec adds no new reason to allow
  a call.
- **The old templates go away in spec 0059** (#112). This spec patches
  only what survives: `vibe-guard`, the setup skill's references, and
  the one settings variant this repository uses (the vibe-guard variant
  of the Claude module, until spec 0059 makes the file repository-owned).
- **`.claude/settings.json` is a guardrail file here.** Changing this
  repository's copy needs a person's `vibe unlock` (A-08).

## Assumptions

- Claude Code sets `CLAUDE_PROJECT_DIR` to the project root for every
  hook command, and runs the command with bash (Git Bash on Windows),
  as spec 0053 already assumes for the audit's hook runs.
- In Git Bash, `cd "D:\path"` with a Windows-form `CLAUDE_PROJECT_DIR`
  works. A test on the Windows CI runner confirms this.
- The contract audit splits a hook command into simple commands, so
  `cd "…" && task verify:fast` still counts as a call to `verify-fast`
  for A-05 and R-01. A test confirms this before any reference changes.

## Desired Behavior

### Shell writes to the guardrail files (#123)

1. `vibe-guard` applies the `guard-policy-file` and
   `guardrail-settings` path rules to the files that a `Bash` or
   `PowerShell` command writes. It also applies them to the files that
   the command deletes, moves or renames. An `Edit` or `Write` of the
   same path gets the same decision, and the same exception lifts it.
2. A shell command **writes** a path when the path is:
   - the target of an output redirection: bash `>`, `>>`, `>|`, `&>`,
     `&>>`, `n>`, `n>>`, and PowerShell `>`, `>>`, `n>`, `*>`. An input
     redirection (`<`) is not a write.
   - a file operand of `tee`, of `sed` with `-i` or `--in-place` (any
     suffix form), or of `truncate`.
   - the destination of `cp`, `mv`, `install` or `ln` (the last
     operand, or the directory after `-t`/`--target-directory`), and the
     source of `mv`.
   - an operand of `rm`, or of `dd`'s `of=`.
   - in PowerShell: the `-Path`, `-LiteralPath` or first positional
     argument of `Set-Content`, `Add-Content`, `Out-File`,
     `Clear-Content`, `Remove-Item`, `Rename-Item`, `New-Item` and
     `Tee-Object` (also `-FilePath`). For `Copy-Item` and `Move-Item`,
     it is the `-Destination` argument, and `Move-Item`'s source too.
   - any of these, found through a wrapper or a nested shell the parser
     already follows (`sudo`, `env`, `bash -c`, `pwsh -c`, `xargs`, …).
3. A relative path is resolved against the payload's `cwd`, or against
   the repository root when the payload has no `cwd`. The rules then
   match the result as they match an edit's path.
4. A path that holds an expansion (`> "$f"`) is not matched. This is a
   documented gap, like `cd` earlier in the same command (the guard does
   not follow it) and writes made by interpreters (`python -c`,
   `node -e`) or by `git checkout`/`git restore` of a path.
5. With a valid unlock, a shell write to `.claude/settings.json` or
   `.claude/hooks/` is allowed, as an edit is. `policy.json` stays
   refused, because no unlock opens it.
6. `secret-file` stays an edit rule. `cp .env.example .env` is still
   allowed.
7. `vibe-guard explain` shows the rule and the matched path for a shell
   write, in the same form it uses for an edit.

### Hooks from any directory (#133)

8. In the setup skill's reference settings, every hook command that
   calls Task starts with `cd "${CLAUDE_PROJECT_DIR:-.}" &&`. Each hook
   then resolves the root Taskfile and runs the root tasks, whatever
   the agent's current directory. The guard command does not need the
   prefix, because `vibe-guard` finds the root itself, but it may have
   it.
9. The Stop command keeps `task verify:fast 1>&2 </dev/null` as one
   simple command, so A-05's static part still finds it. The block-once
   behavior and the green stamp of spec 0057 do not change.
10. From a component directory, a component subdirectory, a plain
    subdirectory and the root of a monorepo built from the references,
    every hook gives the same exit code and runs the same root tasks.
    The Stop gate checks every component, and blocks once with exit 2
    when one fails.
11. This repository's `.claude/settings.json` gets the same prefix, through
    the vibe-guard variant of the Claude module's settings, then
    `task sync`. A person runs `vibe unlock` first.
12. The setup skill tells the agent why the prefix is there, so an
    adopter's own hooks keep it.

### Documentation

13. `docs/usage.md` ("`vibe-guard`, the reference guard") lists the
    write forms of item 2 and the gaps of item 4. AGENTS.md's "Known
    gaps" line names them too.
14. Spec 0053's sections on `guardrail-settings` and `guard-policy-file`
    say that shell writes are matched. The setup skill's hook reference
    says that hooks run from the project root.

## User Stories

1. As a person who relocked the guardrails, I want a `sed -i` on
   `.claude/settings.json` refused, so that the agent cannot change its
   own hooks in a shell command.
2. As a person, I want `echo … > .claude/hooks/policy.json` refused, so
   that the agent cannot loosen the guard through the shell.
3. As a person, I want `tee`, `cp`, `mv` and `rm` on these files
   refused, so that common spellings of the same write are covered.
4. As a person on Windows, I want `Set-Content` and `Out-File` on these
   files refused, so that PowerShell is covered as Bash is.
5. As a person, I want a write through `bash -c` or `sudo` refused,
   so that a wrapper does not hide it.
6. As a person who ran `vibe unlock`, I want the agent's shell write to
   `.claude/settings.json` allowed, so that an unlocked change works in
   any tool.
7. As a person, I want `policy.json` refused even while unlocked, so
   that only I change the guard's exceptions.
8. As an agent, I want reads such as `cat .claude/settings.json` and
   `jq . < .claude/settings.json` allowed, so that I can still inspect
   the settings.
9. As an agent, I want `cp .env.example .env` allowed, so that the usual
   project setup still works.
10. As an adopter with a guard exception for `guardrail-settings`, I
    want it to lift shell writes too, so that one exception means one
    thing.
11. As a person reading `vibe-guard explain`, I want to see which path
    and which rule refused a shell write, so that I understand the
    refusal.
12. As a person, I want the guard's known gaps listed, so that I know
    what it does not catch.
13. As an agent working in `services/api/`, I want the Stop gate to run
    the root `verify:fast`, so that I cannot end a turn with another
    component broken.
14. As an agent in a component, I want a failing gate to block me with
    exit 2, so that I fix the failure before I stop.
15. As an agent in a component, I want `hook:check` to typecheck and
    lint after my edit, so that I get the same feedback as at the root.
16. As an agent that starts a session in a subdirectory, I want
    `hook:context` to run, so that I get the session facts.
17. As an agent in a component, I want the green stamp to work, so that
    a stop with an unchanged tree costs nothing there too.
18. As an adopter with a single-language repository and a nested
    Taskfile, I want the hooks to use the root Taskfile, so that a
    nested one never hides them.
19. As an adopter, I want my audit to stay conformant after I take the
    new reference settings, so that the fix costs me no contract
    finding.
20. As a Windows adopter, I want the prefix to work in Git Bash with a
    Windows path, so that the fix holds on every platform I declare.
21. As a VibeConform contributor in `examples/monorepo/`, I want this
    repository's hooks to use the root Taskfile, so that my gate checks
    VibeConform and not the example.

## Implementation Decisions

- **Write targets come from the parser.** `guardrule` tells a write
  redirection from a read redirection, and gives each simple command a
  list of written paths. That list holds the write redirections and the
  operands of the commands in Desired Behavior 2. The command tables
  are a closed list, kept beside the existing wrapper tables, and each
  entry says which operands it writes. `vibe-guard` and the contract
  audit share `guardrule`. The audit does not read the new list, so its
  verdicts do not change.
- **One decision path for paths.** The guard checks each written path
  of a shell command with the same `pathRules` and exception lookup as
  `decidePath`. The first refusal decides. A command rule that matches
  first (for example `rm -rf`) still decides the outcome, as it does
  now. Only `guard-policy-file` and `guardrail-settings` run on shell
  writes. `secret-file` and the unlock marker rule do not. The marker
  is already covered by the self-unlock command rule.
- **Path resolution.** A written path is made absolute against the
  payload's `cwd` (the guard already reads it), else the root. It is
  then made repository-relative with the existing helper. A path
  outside the repository never matches `guardrail-settings`.
  `guard-policy-file` keeps its suffix match.
- **A word with an expansion is not a written path.** The command rules
  treat an unknown word as matching, but write targets do not, for
  example `> "$tmp"`. Refusing every such write would refuse common
  agent scripts. This is the documented gap of Desired Behavior 4.
- **The hook prefix is in the references, not in Task.** Two ways were
  rejected. `task -d "$CLAUDE_PROJECT_DIR" …` breaks A-05's literal
  match. A component Taskfile that forwards `hook:*` to the root means
  one forwarder per component, and a component would need to know where
  the root is. `cd "${CLAUDE_PROJECT_DIR:-.}" &&` works for every hook,
  every task runner and every layout. When the variable is not set (a
  person runs the command by hand), it falls back to the current
  directory.
- **What changes:** the reference `settings.json` (one file for every
  stack); the setup skill's hook guidance; the vibe-guard variant of
  `internal/module/agents/claude`'s settings; and through `task sync`,
  this repository's `.claude/settings.json`. The runtime-guard variants
  and the prod-mono templates are not patched, because spec 0059 deletes
  them.
- **No new dependency, no ADR.** It is a fix inside existing decisions.

## Testing Decisions

A good test here drives the guard or the hook the way Claude Code does:
a JSON payload on stdin, then a check of the exit code and of what ran.
It does not read the guard's tables or a file's text, except where that
text is the contract (A-05's literal call).

- **Seam 1, the `vibe-guard` subprocess** (#123). This is the existing
  harness in `internal/guard`, which runs `vibe-guard hook` and
  `vibe-guard explain` in fixture repositories. It gets a table of
  shell writes and reads, in Bash and in PowerShell, with no marker,
  with a valid marker and with an expired marker:
  - each write form of Desired Behavior 2, for both files, is refused
    and names its rule;
  - reads (`cat`, `<`, `Get-Content`) are allowed;
  - writes to other files are allowed, `.env` included;
  - writes through `bash -c` and `sudo` are refused;
  - a relative path from a subdirectory `cwd` resolves correctly;
  - an exception lifts the refusal.
  Prior art: `harness_test.go`, `unlock_test.go`, `guard_test.go`'s
  `cwd` cases. `guardrule`'s own tests cover the new written-path list
  through `Commands`, as the command and match tests already do.
- **Seam 2, the reference hooks in a fixture monorepo** (#133). The
  test builds a fixture monorepo from the reference Taskfiles and the
  reference `settings.json`. It runs each hook command with `bash -c`,
  `CLAUDE_PROJECT_DIR` set, and the working directory set to the root,
  a component, a component's subdirectory and a plain subdirectory. It
  asserts the same exit code and the same root task from each:
  - the Stop gate blocks once with a failing component that is not
    the current one;
  - the stamp skips an unchanged tree;
  - `hook:check` and `hook:context` run.
  It runs on Linux and on Windows CI (Desired Behavior 20). Prior art:
  `TestStopGateEverySource`, `TestHookCheckSkipsEditsThatCannotMatter`,
  `TestHookTasksShape`. These tests live in `internal/module` today and
  move in spec 0059, so the new test goes where spec 0059 moves them,
  next to the setup skill, from the start.
- **Static audit of the references:**
  `TestReferenceImplementationsPassTheStaticAudit` keeps every reference
  conformant, A-05 and R-01 included, with the prefix in place. A new
  case shows that `cd "…" && task verify:fast` counts as a call and that
  `task -d … verify:fast` does not.
- **This repository**: `task audit` (v1) and
  `vibe audit --config vibe.v2.yaml` stay as they are. A manual check
  in a real session runs from `examples/monorepo/` and sees this
  repository's Stop gate run (UNVERIFIED until a person does it).

## Non-goals

- Changing the contract or the guard corpus. Shell-write cases in the
  corpus are a MAJOR change. They are a candidate to ship with #50.
- An audit probe that runs the hooks from a subdirectory. That is a
  tighter check, so it is MAJOR too.
- `secret-file` on shell writes, and reading of secret files.
- Following `cd`, interpreters, `git checkout`/`git restore` of a path,
  or expansions in a written path (Desired Behavior 4).
- Patching the old runtime-guard templates and the prod-mono templates.
  Adopters still on template sync keep #133's guard gap until they
  migrate. The release notes say so.
- Scoping `hook:check` to the edited component (#81).

## Acceptance Criteria

- [ ] With no unlock, every write form of Desired Behavior 2 on
      `.claude/settings.json` and `.claude/hooks/policy.json` is refused
      in Bash and in PowerShell, directly and through `bash -c` and
      `sudo`. `vibe-guard explain` names the rule and the path.
- [ ] Reads of those files, and writes to other files `.env` included,
      are allowed.
- [ ] With a valid unlock, shell writes to `.claude/settings.json` and
      `.claude/hooks/` (but not `policy.json`) are allowed. With an
      expired unlock, they are refused.
- [ ] A guard exception for either rule lifts a shell write as it lifts
      an edit.
- [ ] In a fixture monorepo built from the references, every hook
      command gives the same result from the root, a component, a
      component's subdirectory and a plain subdirectory. The Stop gate
      blocks once with exit 2 when a different component fails. This
      runs on Linux and on Windows in CI.
- [ ] Every reference still passes the static audit. A test shows that
      `cd … && task verify:fast` counts for A-05.
- [ ] This repository's `.claude/settings.json` has the prefix (template
      plus `task sync`, after a person's `vibe unlock`). `task audit`
      and `Contract / audit` pass.
- [ ] `docs/usage.md`, AGENTS.md's "Known gaps", spec 0053 and the
      setup skill's hook guidance are updated as in Desired Behavior
      13–14.
- [ ] `task verify`, `task audit` and CI pass.

## Further Notes

- Order: this spec lands before spec 0059. Spec 0059 moves the hook
  tests out of `internal/module`. Placing the new hook test next to the
  setup skill now avoids moving it twice.
- #123's original report was on Linux/WSL2, and the check on main was
  on Windows. Both shells are in Seam 1's table.
