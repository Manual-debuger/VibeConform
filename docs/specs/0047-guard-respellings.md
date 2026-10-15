# Spec 0047: The Agent Guard Denies Other Spellings of Its Commands

Status: accepted and implemented (2026-10-15). Issue #84. Extends spec 0021's policy. Its rule that the
policy is a faithful port of the old bash hooks no longer holds for the
rules named here.
Settled with the user on 2026-10-14 in the grilling of #82, #84 and #85,
on one principle: **same command, different spelling**. A spelling that
the tool itself accepts for a denied command is denied too. A different
command with a similar effect is a new policy decision. This spec leaves
those out and records them as known limitations. Exceptions are deferred
to #80. Follows spec 0046 (#82). One tag after all three are merged.

## Problem

The guard (spec 0021) denies a short list of destructive commands, but
only in the spellings the old bash hooks knew. Probed on
`v0.9.0-alpha.1` with `task -x hook:guard`, and with the real tools on
2026-10-15 (GNU coreutils 8.32 `rm`, Git 2.53.0, PowerShell 7.6.6). Each
of these does what a denied command does, and the guard allows it:

| Allowed today | Denied spelling it equals |
|---|---|
| `rm -r -f x`, `rm -R -f x`, `rm -Rf x`, `rm --recursive --force x`, `rm --rec --for x` | `rm -rf x` |
| `git reset --har` | `git reset --hard` |
| `git -C dir reset --hard` | `git reset --hard` |
| `git clean -f -d`, `git clean --force -d`, `git clean --forc -d` | `git clean -fd` |
| `git push -uf origin main`, `git push origin +main` | `git push --force` |
| `git branch -d -f x`, `git branch -df x`, `git branch --delete --force x`, `git branch --del --forc x` | `git branch -D x` |
| `rm -r -fo x`, `ri -Rec -Fo x`, `del`, `rd`, `rmdir`, `erase` with the same flags | `Remove-Item -Recurse -Force x` |
| `remove-item -recurse -force x` | the same, in another case |
| `Remove-Item "build dir" -Recurse -Force` | the same, with a quoted path |
| `git restore .`, `git checkout -- .` | `git reset --hard` (discards uncommitted work in tracked files) |

Users can also read the guard as a security boundary. It is not one, and
the docs do not say so.

Verified on 2026-10-15, on Windows:
- **Live canary, after the sync.** In this repository's Claude Code
  session, the guard refused `git branch -d -f zz-guard-canary-0047` and
  `git restore --source=HEAD .` (in a scratch repository) with the new
  messages, and let `git restore --staged .` run; the working-tree change
  was kept.
- **`!` skips the guard.** The user ran
  `! git branch -D zz-guard-canary-0047` in a Claude Code session with
  the guard live. Git answered `error: branch 'zz-guard-canary-0047' not
  found`, and there was no `Blocked:` message. So the deny messages name
  `! <command>` (D5).
- **PowerShell 7.6.6.** Each of these deleted a directory that held a
  read-only file: `rm -r -fo`, `rm -re -fo`, `ri -Rec -Fo`,
  `del -r -fo`, `rd T -r -fo`, `rmdir -Recurse -Force`,
  `erase -rec -forc`, `remove-item -recurse -force`,
  `REMOVE-ITEM -RECURSE -FORCE`, `Remove-Item -Force -Recurse:$true`,
  `Remove-Item -LiteralPath T -Recurse -Force`. `rm -r -f` failed:
  `-f` is ambiguous between `-Filter` and `-Force`. Neither parameter has
  an alias.
- **GNU coreutils 8.32.** `rm -r -f`, `-R -f`, `-f -r`,
  `--recursive --force`, `--rec --for`, `--r --f` and `-r --force` each
  deleted the same kind of directory.
- **Git 2.53.0.** Git accepted `reset --har`, `clean --forc -d`,
  `branch --del --forc`, `branch -d -f`, `branch -df`,
  `-C . reset --hard` and `push -uf`. It rejected `push --forc` as
  ambiguous.

## Constraints

- Patterns stay in the subset that Go RE2, Python `re` and JavaScript
  `RegExp` read identically (spec 0021 §3): no lookaround, no
  backreferences, no inline flags. A spelling that cannot be matched in
  that subset is recorded as a known limitation, not matched in one
  runtime only.
- The rules stay in one table, `policy.json`, read by all three guards.
  The guards' code does not change, apart from what the table needs.
- The shared corpus is the contract. Every guard runtime must agree on
  every case (spec 0021).
- A command that the guard allows today and that is not a spelling of a
  denied command stays allowed. In particular: `git push
  --force-with-lease`, `git branch -d`, `git clean -n`, `rm -r` without
  force, `git reset --soft`, `git restore <file>`, `git restore --staged
  .`, `git checkout <branch>`.
- The guard stays a guardrail: only what is listed here is denied. No new
  kinds of command are denied.
- The only escape stays "the agent asks, the user runs it" (#80 designs
  exceptions).
- Every bug fix needs a regression test (AGENTS.md).

## Assumptions

- Over-matching is the safer error. A rare safe command that the new
  patterns also deny, such as an abbreviation of `--staged` that the
  pattern doesn't recognize, is acceptable when the corpus records it.

## Desired Behavior

The guard matches only within one simple command: flags after a `;`,
`&`, `|` or newline belong to the next command.

- **D1. `rm -rf`.** `rm` is denied when its options include both
  recursion (`-r`, `-R`, `--recursive`, or a unique prefix of it) and
  force (`-f`, `--force`, or a unique prefix), combined in one cluster or
  in separate words, in any order, before or after the operands.
- **D2. Git commands.** For `git reset --hard`, `git clean -fd`,
  `git push --force` and `git branch -D`:
  - Git's global options between `git` and the subcommand (`-C <dir>`,
    `-c <key>=<value>`, `--no-pager`, `--git-dir=…`, `--work-tree=…`)
    don't change the result.
  - A long option may be spelled as any prefix that Git accepts for it:
    `--har`, `--forc`, `--del`.
  - Short options count in a cluster or separately, in any order:
    `-f -d`, `-df`, `-uf`.
  - `git push` with a refspec that starts with `+` is denied.
  - `git branch` with delete (`-d`, `--delete`) and force (`-f`,
    `--force`) together is denied, as `-D` is.
  - `--force-with-lease` and `--force-if-includes` are not `--force`.
- **D3. PowerShell `Remove-Item`.** It is denied with both `-Recurse` and
  `-Force`. The rule covers:
  - the cmdlet's aliases (`rm`, `ri`, `del`, `erase`, `rd`, `rmdir`);
  - parameter prefixes that PowerShell accepts (`-r`, `-Rec`, `-fo`,
    `-Forc`);
  - any letter case, as PowerShell ignores it;
  - a `:$true` suffix;
  - a quoted path anywhere in the command.
- **D4. Whole-tree discard.** `git restore` and `git checkout` are denied
  when they would overwrite the whole working tree from the index or a
  commit:
  - the pathspec is `.`, `./` or `:/`, with or without a `--` before it;
  - for `git restore`, unless only the index is restored (`--staged` or
    `-S` without `--worktree` or `-W`).

  `git restore <file>`, `git checkout -- <file>` and
  `git checkout <branch>` stay allowed.
- **D5. Messages.** Each deny message names the command it blocked and
  how to run it anyway, for example: `Blocked: rm -rf is a destructive
  operation. Ask the user to run it themselves (in Claude Code:
  ! <command>).` A command run with `!` does not go through the guard
  (verified above).
- **D6. Known limitations, recorded and tested.** These stay allowed.
  Each one has an `allow` case in the corpus that names it as a known
  limitation, and `docs/usage.md` lists them:
  - other commands with a similar effect: `find . -delete`,
    `git checkout -f`, `git switch --discard-changes`,
    `cmd /c rd /s /q x`;
  - a secret file written through the shell (`echo X=1 > .env`), or
    named without an extension (`deploy/secrets/db_password`);
  - remote branch deletion (`git push origin --delete x`,
    `git push origin :x`);
  - spellings that textual matching can't see: a command built from
    variables, `eval`, quotes inside a word (`r''m -rf`), aliases, and
    any form the portable regex subset cannot express.
- **D7. Not a sandbox.** `docs/usage.md` ("The agent guard") and the
  `vibeconform` skill say that the guard stops common accidents and is
  not a security boundary. For hard limits they point to Claude Code's
  permission `deny` rules and its sandbox. `GLOSSARY.md` defines
  "Agent guard" the same way.

## Non-goals

- New kinds of denied command, including those listed in D6.
- Exceptions or per-repository rule changes (#80).
- Changes to the `secret-file` rule.
- A shell parser in the guards.
- Codex hooks (still suspended, spec 0024).

## Acceptance Criteria

- [x] Every "Allowed today" row of the Problem table is a `deny` case in
      the corpus. The corpus test fails before the change and passes
      after it in all three runtimes (Go, Node, Python), and end to end
      through `task -x hook:guard`.
- [x] Each allowed command listed under Constraints is an `allow` case
      that passes before and after.
- [x] Each D6 limitation is an `allow` case whose name says
      "known limitation".
- [x] The PowerShell spellings in D3 were checked on real `pwsh` to
      delete a read-only file in a directory. The result is recorded
      here. A spelling that PowerShell rejects (`-f`, which is
      ambiguous) needs no rule of its own.
- [x] The portable-subset test passes for every pattern.
- [x] The `!` canary has been run and its result is recorded here.
- [x] The deny messages name `! <command>` (D5).
- [x] `docs/usage.md` and the `vibeconform` skill carry D6 and D7, and
      `GLOSSARY.md` has the "Agent guard" entry.
- [x] This repository and the three examples are synced and conformant.
      The guard canary in `docs/usage.md` still fires in a live session.
- [x] `task verify` and `task audit` pass.
