# Plan 0047: The agent guard denies other spellings of its commands

Implements `docs/specs/0047-guard-respellings.md`.
Status: approved 2026-10-15.

## Repository impact

| Path | Change |
|---|---|
| `internal/module/agents/claude/policy.go` | pattern helpers; rewritten command patterns; new rule `git-discard-worktree`; messages name `! <command>` |
| `internal/module/agents/claude/policy_test.go` | `TestPolicyIsFaithfulPort` replaced (see Tests) |
| `internal/module/agents/claude/testdata/guard_corpus.json` | new deny, allow and known-limitation cases; one case flips |
| `internal/module/agents/claude/guard_corpus_test.go` | a long-command timing test per runtime |
| `internal/module/agents/vibeskill/vibeskill.go`, its test | a short "agent guard" paragraph (D7) |
| `.claude/hooks/policy.json` here and in the three examples, their `.vibe/state.yaml`, the synced skills | synced |
| `docs/usage.md` | "The agent guard": what it denies, known limitations (D6), not a sandbox (D7) |
| `GLOSSARY.md` | the "Agent guard" entry (already written, uncommitted) |
| `AGENTS.md` | the Guardrails list names the new rule |
| `docs/specs/0021-*.md` | a pointer: the faithful-port rule is extended by spec 0047 |
| `docs/specs/0047-*.md`, this plan | status, results |

The guards (`guard.go`, `guard.mjs`, `guard.py`) do not change: they
already read any rule from `policy.json` and search with no flags. The
dependency direction is unchanged. No new dependency.

## Pattern design

Prototyped in a scratch harness and checked with Go RE2, Python 3.14
`re` and Node 24 `RegExp`: 119 cases, every one as intended, and the
three engines gave identical results on all of them.

The patterns are built in Go from a few helpers, so `policy.go` stays
readable and the generated `policy.json` holds the expanded regex:

- `bound` = `(^|[^a-zA-Z0-9_.-])`: a command word starts here, so
  `/bin/rm`, `sudo rm` and `$(rm` match, but `farm` and `rm-old` do not.
- `gap` = `([^ \t;&|\n]+[ \t]+){0,16}`: up to 16 whole words of the same
  simple command. Words never contain `;`, `&`, `|` or a newline, so a
  match never crosses into the next command.
- An option starts at its dash. `end1` = `(:[^ \t;&|\n]*)?[ \t]+` closes
  an option that another follows (`:` covers PowerShell's `-Recurse:$true`).
  `end2` = `([^a-zA-Z0-9_-]|$)` closes the last one.
- `both(cmd, a, b)`: `cmd gap a end1 gap b end2`, or `b` before `a`.
- `prefixes("--recursive", 3)`: `--r(e(c(u(…)?)?)?)?`, every prefix
  from the shortest one the tool accepts. `anyCase("Remove-Item")`:
  `[Rr][Ee]…`, because the engines share no portable case-insensitive
  flag.
- `gitCommand(sub)`: `bound git`, then up to 8 global options (`-C <dir>`,
  `-c <k=v>`, `--git-dir <path>` and the like, or any `-…` word), then
  `sub` and blanks.

Rules (the IDs stay, so messages and docs keep their names):

| Rule | Matches |
|---|---|
| `rm-rf` | `rm` with a recursion option (`-…r…`, `-…R…`, `--r…`) and a force option (`-…f…`, `--f…`) in either order, or one cluster holding both |
| `remove-item-recurse-force` | `Remove-Item`, `ri`, `del`, `erase`, `rd`, `rmdir`, `rm` in any case, with `-r…` (prefix of `-Recurse`) and `-fo…` (prefix of `-Force`) in either order |
| `git-reset-hard` | `gitCommand(reset)`, then `--ha`, `--har` or `--hard` |
| `git-clean-fd` | `gitCommand(clean)` with force (`-…f…`, `--f…`) and `-…d…`, separate in either order or in one cluster |
| `git-push-force` | `gitCommand(push)`, then `-…f…`, exactly `--force`, or a refspec starting with `+` |
| `git-branch-force-delete` | `gitCommand(branch)` with `-…D…`, or delete (`-…d…`, `--d…`) with force (`-…f…`, `--forc`, `--force`) |
| `git-discard-worktree` (new) | `gitCommand(checkout)` with pathspec `.`, `./` or `:/`; `gitCommand(restore)` with that pathspec and either no `--staged`/`-S…` word before it, or `--worktree`/`-…W…` anywhere |

Every prefix length matches what the tool accepts. `--forc` is the
shortest for `git branch`, because `--for` is ambiguous with
`--format`. `git push` takes `--force` only, because Git rejects
`--forc` as ambiguous. `-fo` is the shortest for PowerShell, because
`-f` is ambiguous.

**Why the 16-word cap.** The first prototype allowed any text between
options. A 34,000-character `git commit -m` holding a thousand
`rm -r …` fragments then took 67 s in Python and 10 s in Node. Each
backtracking engine tried every split. Whole-word gaps with a cap bring
every adversarial input I tried under 50 ms. A denied option more than 16
words after its command is a recorded limitation (D6).

**Messages.** Each one keeps its first sentence and ends in
`Ask the user to run it themselves (in Claude Code: ! <command>).` The
new rule says `Blocked: git restore . (or git checkout -- .) discards
uncommitted changes to tracked files.`

**Raw fallback.** For command rules, `Raw` stays equal to `Pattern`. On a
payload that cannot be parsed, matching the JSON text can only match
more, never less (spec 0021).

## Tests

- **Corpus** (`guard_corpus.json`). The deny cases are every "Allowed
  today" row of the spec, plus the respellings the prototype covered:
  `-Rf`, flags after operands, `--rec --for`, `-C`/`-c`/`--git-dir`,
  `+HEAD:main`, the PowerShell aliases, case and `:$true`, and
  `./`, `:/` and `"."`. The allow cases are the spec's Constraints list,
  plus near misses: `rm -f a && grep -r`, `git clean --dry-run -d`,
  `git branch -d feature-fix`, `git restore --staged -- .`,
  `git restore ./src`, `Remove-Item -Recurse -Filter`. The known
  limitations are each D6 item, plus "more than 16 words" and
  "a separator inside quotes", each named "known limitation".
  The existing case "Remove-Item with a quoted path before the flags
  (gap carried over from the bash hook)" flips to deny and is renamed.
  The existing Go, Node, Python and end-to-end corpus tests run them
  unchanged.
- **`TestPolicyIsFaithfulPort`** pinned the seven messages of the old
  bash hooks. Spec 0047 changes them on purpose, so it is replaced by
  `TestPolicyRules`, which pins the eight rule IDs and checks that every
  message names the `! <command>` escape.
- **`TestGuardLongCommand`** (new, `guard_corpus_test.go`): feeds a
  34,000-character adversarial command to each available runtime's guard
  and fails if one takes more than 2 s. It is skipped when the runtime
  is missing, as the corpus tests are.
- The portable-subset test runs on the new patterns unchanged.

## Steps (one commit each, in order)

1. Commit this plan (the spec is in `e79b14c`).
2. **Red.** Add the corpus cases and `TestGuardLongCommand`. Run the
   corpus tests and record which deny cases fail. Do not commit yet.
3. **Green.** Add the helpers and rules in `policy.go`, and replace
   `TestPolicyIsFaithfulPort`. Run `go test ./internal/module/agents/...`
   with Node, Python and `uv` on `PATH`, so that no runtime is skipped.
   Commit steps 2 and 3 together, as
   `fix: the agent guard denies other spellings of its commands (spec 0047, #84)`.
4. **Skill.** Add the "agent guard" paragraph to the `vibeconform` skill
   and pin it in its test. Commit.
5. **Sync.** Build `vibe` from a clean worktree and install it in
   `~/go/bin`. Run `vibe sync` here and in the three examples, then
   `task audit` in each. Commit as
   `chore: sync this repository and the examples (spec 0047)`.
6. **Live canary.** In this session, ask the guard to refuse spellings
   that it allowed before, using commands that are harmless if they run:
   `git branch -d -f zz-guard-canary-0047` on a branch that doesn't
   exist, and `git restore --source=HEAD .` in a scratch repository.
   Check that `git restore --staged .` is still allowed. Record the
   results in the spec.
7. **Docs.** `docs/usage.md`, `GLOSSARY.md`, `AGENTS.md`, the spec 0021
   pointer, and the status of the spec and this plan.
8. `task verify`, `task audit`, push, open the PR, and wait for CI. CI
   runs the corpus in Go, Node and Python on Linux and Windows.

## Verification ledger (filled in as the steps run)

- corpus tests: red before the fix, green after it, in Go, Node, Python
  and end to end
- long-command timing per runtime
- `task verify` (Windows)
- `task audit` here and in the three examples
- live canary in this session
- CI on the PR
