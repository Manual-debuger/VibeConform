# Spec 0069: The guard reads every path one way, and `vibe unlock` finds the marker where the guard does

Status: draft (2026-11-25). Issue #163, the bug-fix part. Part of the
design review #167 (guard track) and #166 (the fail-open
`Rule.MatchesCommand`). A bug fix with regression tests. The rest of
#163 (a deeper `Guard` interface and in-process tests) is spec 0070.
Amends spec 0053 ("Repository root", "`vibe unlock`") and spec 0058
("Path resolution"). No contract change: the contract stays 1.2.0 and
the guard corpus is unchanged.

## Problem

The guard has three bugs and one latent fault. They all come from the
same thing: more than one place decides what a path or a Git directory
is.

**1. An edit path that leaves the project root with `..` and comes back
is not refused.** Reproduced on 2026-11-25 with a `vibe-guard` built
from `main` at 8021031, `CLAUDE_PROJECT_DIR=<abs>/proj`, a `.git` in
`proj` and no unlock marker:

| `vibe-guard explain --tool Write …` | Expected | Actual |
|---|---|---|
| `.claude/settings.json` | refused, `guardrail-settings` | refused |
| `../proj/.claude/settings.json` | refused, `guardrail-settings` | **allowed**, rule `none` |
| `<abs>/proj/../proj/.claude/settings.json` | refused, `guardrail-settings` | **allowed**, rule `none` |
| `.claude/hooks/x.sh` | refused, `guardrail-settings` | refused |
| `../proj/.claude/hooks/x.sh` | refused, `guardrail-settings` | **allowed**, rule `none` |

The agent can change the hooks that run its own guard without an
unlock. The cause is in the shared path reading: a path is cleaned only
after it is compared with the project root, so `..` survives the
comparison. Shell writes are not affected, because the guard joins and
cleans a written path first.

The same reading is used for a repository's own path rules in
`policy.json`. A deny rule on `secrets/**` should refuse
`../proj/secrets/a` in the same way. This is found by reading the code
and is not reproduced. Building the fixture needs a `policy.json`, which
is the user's to write under this repository's guard.

**2. Edit paths and shell-write paths are read differently.**
- A path that a command writes is made absolute against the payload's
  `cwd` (spec 0058), with Git Bash's `/d/…` form read as `D:/…`.
- An edit tool's path goes to the path rules as given. A relative edit
  path is read against the project root, not the `cwd`, and the Git Bash
  form is not read.
- Two functions named `resolve` do different things.

Claude Code normally sends absolute edit paths, so the different reading
of a relative path is a low risk on its own. But it is why bug 1 exists
for edits and not for shell writes.

**3. `vibe unlock` and the guard can disagree on where the unlock marker
is.** `vibe unlock` asks `git rev-parse --git-common-dir`, which honours
`GIT_DIR` and `GIT_COMMON_DIR`. The guard reads `.git` from files and
does not (A-03). Reproduced on 2026-11-16 (#163): with a bare Git
directory plus a work tree, or with `GIT_DIR` or `GIT_COMMON_DIR` set to
another repository, `vibe unlock` writes the marker where the guard
never reads it. The settings edit stays refused, and in the second and
third case the marker unlocks another repository instead. This fails
closed. The person sees "still refused" and gets no explanation.

**4. A repository command rule whose `args` is not a valid regular
expression never matches.** `Rule.MatchesCommand` compiles `args` when
`ParsePolicy` has not, and returns "no match" if compiling fails. Today
only `ParsePolicy` builds rules, and it rejects a bad `args`, so this
cannot happen yet. But a deny rule built any other way fails open
without a sign.

**5. A non-string `cwd` is dropped without a sign.** The hook reads the
payload's `cwd` and ignores a decoding error. A `cwd` that is not a
string falls back to the project root, while every other payload defect
is refused under `payload-error`. Once edit paths are read against the
`cwd` (2), this matters for edits too.

## Solution

- Every path the guard checks, from an edit tool or from a command that
  writes it, is read one way: separators made forward slashes, the Git
  Bash drive form read on Windows, a relative path joined to the
  payload's `cwd` (else the project root), and the result cleaned. Only
  then is it compared with the project root and the Git top. A `..` that
  comes back into the project is the project's path.
- The shared reading in `internal/guardrule` that turns a path into a
  project-relative one cleans before it compares. The audit and the
  guard keep one reading.
- The function that finds the project root's Git top, Git directory and
  common Git directory moves to `internal/guardrule`. `vibe-guard` and
  `vibe unlock` both call it. `vibe unlock` no longer runs `git`. It
  writes the marker where the guard reads it, or fails with a message
  that says why.
- A repository rule whose `args` cannot be compiled matches: the guard
  fails closed.
- A `cwd` that is present and is not a string is a payload error.

## User Stories

1. As a person who uses Claude Code with `vibe-guard`, I want an edit of
   `../proj/.claude/settings.json` from inside `proj` refused, so that
   the agent cannot change its own guard by spelling the path another
   way.
2. As that person, I want `<abs>/proj/../proj/.claude/settings.json`
   refused, so that an absolute path with `..` is no way around the
   guard.
3. As that person, I want a file under `.claude/hooks/` reached through
   `..` refused, so that the hook scripts are protected like the
   settings file.
4. As that person, I want `.claude/hooks/policy.json` reached through
   `..` refused under `guard-policy-file`, so that the policy stays mine
   to change.
5. As that person, I want a secret-looking file reached through `..`
   refused under `secret-file`, as it is today, so that the fix does not
   weaken that rule.
6. As that person, I want the Git top's guardrail settings reached
   through `..` from a project below the Git top refused, so that spec
   0068's protection holds for every spelling.
7. As a repository owner, I want my `policy.json` path rules to match a
   path that leaves the project with `..` and comes back, so that my deny
   and ask rules cannot be bypassed.
8. As a repository owner, I want my exception globs to match a path that
   leaves the project with `..` and comes back, so that an exception
   works for every spelling of the path it names.
9. As a repository owner, I want a path that really leaves the project
   (`../other/.claude/settings.json`) still allowed, so that a sibling
   project is not treated as this one.
10. As a person, I want a valid unlock to allow the `..` spellings too,
    so that the fix does not make an unlock weaker.
11. As a person, I want an edit tool's relative path read against the
    session's working directory, as a shell write already is, so that
    the same path names the same file in both.
12. As a person on Windows with Git Bash, I want an edit path in the
    `/d/…` form read as `D:/…`, as a shell write already is, so that the
    form does not hide a protected file.
13. As a person, I want `vibe-guard explain` to decide every one of these
    paths as `vibe-guard hook` does, so that I can check a refusal
    myself.
14. As a person who ran `vibe unlock`, I want the guard to read the
    marker where `vibe unlock` wrote it, in every repository layout, so
    that an unlock always has an effect.
15. As a person with `GIT_DIR` or `GIT_COMMON_DIR` exported in my
    terminal, for example for a bare-repository dotfiles setup, I want
    `vibe unlock` to unlock the repository I named with `--repo-root`,
    not the one the variable points at, so that I do not unlock another
    repository by accident.
16. As a person whose work tree has no `.git` (a bare Git directory used
    through `GIT_DIR`), I want `vibe unlock` to fail with a message that
    says the guard cannot find a Git directory there, so that I know why
    nothing was unlocked.
17. As a person, I want `vibe unlock` to work in a linked worktree, a
    `--separate-git-dir` repository, a worktree with relative paths and
    a project below the Git top, as it does today, so that the change
    breaks no layout that works.
18. As a person, I want `vibe unlock` to work without `git` on `PATH`,
    so that it needs no more than the guard needs.
19. As a person, I want `vibe unlock` outside any Git repository to fail
    and write nothing, as it does today.
20. As a repository owner, I want a command rule whose `args` cannot be
    compiled to refuse (deny) or ask (ask), not to be skipped, so that a
    broken rule never loosens the guard.
21. As a guard maintainer, I want an invalid payload `cwd` refused under
    `payload-error`, so that every payload defect is handled one way.
22. As a guard maintainer, I want one function that normalises a path,
    used for edits and shell writes, so that the two cannot drift again.
23. As a guard maintainer, I want one function that finds the Git
    directories, used by the guard and by `vibe unlock`, so that the
    marker location is computed once.
24. As an auditor running `vibe audit`, I want A-02's reading of an
    exception unchanged for every path without `..`, so that no verdict
    changes for a conformant repository.
25. As a maintainer of a third-party guard, I want the guard corpus
    unchanged, so that this fix does not change what the contract asks
    of my guard.

## Implementation Decisions

### The path reading in `internal/guardrule`

- The function that makes a path relative to the project root (today's
  `RelPath`) cleans before it compares:
  - an absolute path is cleaned, then compared with the cleaned root;
  - a relative path is joined to the root, cleaned, and then compared.
- Inside the root, the result is the slash-separated relative path, as
  today. Outside the root, the result is the cleaned path as given
  (relative stays relative, absolute stays absolute), as today.
- Drive letters are still compared without case. Backslashes still count
  as separators.
- This is the reading the audit's A-02 and the guard share. A path
  without `..` gives the same result as today.

### One path normaliser in `internal/guard`

- One function turns a path from a tool call into the absolute, cleaned
  path the path rules check. It takes the project root, the payload's
  `cwd` and the path:
  - backslashes become forward slashes;
  - on a root with a drive letter, Git Bash's `/d` or `/d/…` becomes
    `D:` or `D:/…`;
  - a relative path is joined to the `cwd`, else to the project root;
  - the result is cleaned.
- The edit tools (Write, Edit, MultiEdit, NotebookEdit) and the shell
  writes both call it before any path rule, exception glob or repository
  path rule. Today's `resolve` in the guard is replaced by it.
- The rules then see only absolute, clean paths. The relative-path
  branches in `guardrail-settings` (the Git top case) and in the
  unlock-marker check go away.
- A finding still shows the path as the tool call gave it for an edit,
  and as the project-relative path for a shell write, as today.

### One Git-directory finder in `internal/guardrule`

- The walk from the project root to the Git top, and the reading of
  `.git` (a directory, or a `gitdir:` file), `commondir` and the common
  Git directory, move from `internal/guard` to `internal/guardrule`
  unchanged. The repository value (project root, Git top, Git directory,
  common Git directory) moves with them.
- The file-reading helper keeps a name of its own, so the package has
  one function called `resolve` at most.
- `internal/guard`'s session function (spec 0068) calls the shared
  finder. Nothing it decides changes.
- `vibe unlock` calls the shared finder with `--repo-root` made absolute
  and walks up from it, as `git rev-parse` did. It no longer runs `git`
  and does not read `GIT_DIR` or `GIT_COMMON_DIR`.
- When the finder finds no common Git directory (no `.git` above, an
  unreadable `.git` file, an empty `commondir`), `vibe unlock` fails and
  writes nothing. The message names the directory, says that no Git
  directory was found by reading `.git` as `vibe-guard` does, and says
  that `GIT_DIR` and `GIT_COMMON_DIR` are not read.
- The depguard rules already let `internal/cli` and `internal/guard`
  import `internal/guardrule`. No new package boundary.

### Repository rules fail closed

- When a command rule has `args` and no compiled expression, matching
  compiles it. If that fails, the rule matches: a deny rule refuses and
  an ask rule asks. Rules from `ParsePolicy` are unchanged, since
  `ParsePolicy` rejects a bad `args`.
- Exceptions already fail closed (a match that does not parse lifts
  nothing) and do not change.

### The payload's `cwd`

- A `cwd` key whose value is not a JSON string is a payload error,
  refused under `payload-error` like a missing `tool_name`. A missing
  `cwd`, or an empty string, still falls back to the project root.

### Documentation

- Spec 0058, "Path resolution": edit paths are resolved in the same way
  as written paths.
- Spec 0053, "Repository root" and "`vibe unlock`": `vibe unlock` reads
  `.git` as the guard does, and does not honour `GIT_DIR` or
  `GIT_COMMON_DIR`.
- `docs/usage.md`, "`vibe unlock`": the same, and the message for a
  bare Git directory.
- `docs/architecture/` only if it says where the walk lives (check
  during the documentation sync).
- No ADR: no new dependency and no reversal of an earlier decision. The
  move follows #163's agreed direction.

## Testing Decisions

- **A good test drives a binary or command as a person or Claude Code
  does and checks the exit code, the rule named and the files written.**
  It does not read the repository value or call unexported functions.
- **Three existing seams, no new one:**
  1. `vibe-guard`'s subprocess harness (`runGuard`, `hook`, `explain`).
     It covers the path bugs, the `cwd` payload error and the guard side
     of the marker. Prior art: `TestGuardrailSettingsOnlyInTheRepository`,
     `TestShellWritePathsResolveAgainstCwd`, `TestUnlockInLinkedWorktree`
     and the `TestProjectBelowGitTop*` tests.
  2. `vibe unlock` run in-process through the CLI's root command
     (`runVibeUnlock`). It covers where the marker is written and the
     error cases. Prior art: `TestUnlockLinkedWorktreeSharesCommonDir`
     and `TestUnlockOutsideGit`.
  3. `internal/guardrule`'s exported functions. They cover the path
     reading the audit shares, the shared finder's agreement with `git`
     and the fail-closed rule. Prior art: `match_test.go`, and
     `TestProjectBelowGitTopAgreesWithGit` for comparing with
     `git rev-parse`.
  The in-process guard seam (`guard.Main` with an `Env`) is spec 0070's.
- **Cases for the guard** (each fails on `main` at 8021031 unless marked
  "unchanged"). Each runs through `hook` with an absolute and a relative
  payload path where it applies, and through `explain`:
  - Write, Edit and MultiEdit of `../proj/.claude/settings.json`,
    `<abs>/proj/../proj/.claude/settings.json` and
    `../proj/.claude/hooks/x.sh`: refused under `guardrail-settings`.
  - The same paths with a valid marker: allowed.
  - `../proj/.claude/hooks/policy.json`: refused under
    `guard-policy-file` (unchanged: that rule matches a suffix).
  - `../proj/.env`: refused under `secret-file` (unchanged).
  - From a project `top/proj` below the Git top:
    `../../top/.claude/settings.json`, which leaves the Git top and comes
    back to its settings: refused. `../.claude/settings.json` is refused
    today and stays refused (unchanged).
  - `../other/.claude/settings.json`, a sibling project: allowed
    (unchanged).
  - A repository deny path rule on `secrets/**` refuses
    `../proj/secrets/a`, and an exception glob lifts a built-in rule for
    a `..` spelling of the path it names. The test writes its own
    `policy.json` inside a temporary fixture, as the harness already
    does.
  - A relative edit path with a payload `cwd` in a subdirectory is read
    against the `cwd`: `Write ../.claude/settings.json` with
    `cwd=<proj>/sub` is refused.
  - An edit path in the Git Bash form (`/c/…/proj/.claude/settings.json`)
    on Windows: refused. Skipped on other systems.
  - A payload whose `cwd` is a number: refused under `payload-error`.
    A payload without `cwd`: decided as today (unchanged).
- **Cases for `vibe unlock`:**
  - In a repository, a linked worktree, a `--separate-git-dir`
    repository and a project below the Git top: the marker is written in
    the common Git directory (unchanged; the first two exist today).
  - With `GIT_DIR` set to another repository's `.git`: the marker is
    written in `--repo-root`'s own common Git directory, and nothing in
    the other repository. With `GIT_COMMON_DIR` set: the same.
  - A work tree without `.git`, used with a bare Git directory through
    `GIT_DIR`: `vibe unlock` fails, writes nothing and the message names
    `GIT_DIR`.
  - Outside Git: fails and writes nothing (unchanged).
  - These tests set `GIT_DIR` and `GIT_COMMON_DIR` with `t.Setenv`, so
    they do not run in parallel.
- **Cases for `internal/guardrule`:**
  - The project-relative reading: a table of absolute and relative
    paths with `..`, inside and outside the root, with drive letters in
    either case and backslashes. Every existing `RelPath` case passes
    unchanged.
  - The shared finder agrees with `git rev-parse --git-common-dir` for a
    plain repository, a linked worktree, `--separate-git-dir`,
    `git worktree add --relative-paths` and a project below the Git top
    (skipped without `git`).
  - A `Rule` built with a command and an `args` that does not compile
    matches a command with that prefix.
- **Regression of what must not change.** Every existing guard,
  `guardrule`, `vibe unlock` and audit test passes unchanged, and so do
  the guard corpus (`TestCorpus`, unchanged file) and the audit's A-02,
  A-03, A-08 and A-09 tests.

## Out of Scope

- **Spec 0070**, the rest of #163:
  - a deeper `Guard` (unexported state, `New` with options, one decide
    entry point for policy errors and observe mode,
    `Decision.Refuses()`/`ExitCode()`);
  - one way to read an exception for the audit and the guard;
  - the 278-case corpus run in-process;
  - `explain`'s `os.Stat` against the process's directory;
  - `vibe unlock` with a clock passed in.
- **#166's other items**: parsing rule and exception matches once at
  load time, typed fields, splitting `wrappers.go`, the setupskill and
  test-helper items.
- **Honouring `GIT_DIR` or `GIT_COMMON_DIR`** in the guard. The guard
  reads files only (A-03), and the hook's environment is not the
  person's terminal.
- **New guard corpus cases** for `..` paths. The corpus is part of the
  contract; adding cases is a contract change of its own.
- **Symbolic links.** A path through a symlink into `.claude/` is not
  resolved, as today.
- **Codex sessions**, which have no hooks (ADR 0011).

## Acceptance Criteria

- [ ] An edit of the project's `.claude/settings.json`, or of a file
      under its `.claude/hooks/`, spelled with `..` that leaves and comes
      back into the project, absolute or relative, is refused under
      `guardrail-settings` by `vibe-guard hook` and by
      `vibe-guard explain`, and is allowed with a valid marker.
- [ ] The same holds for the Git top's guardrail settings from a project
      below the Git top.
- [ ] A repository path rule and an exception glob match the `..`
      spelling of a path they match today. A path that really leaves the
      project is decided as today.
- [ ] Edit paths and shell-write paths go through one normaliser in
      `internal/guard`. A relative edit path is read against the
      payload's `cwd`, and an edit path in the Git Bash drive form is
      read as on a shell write.
- [ ] A payload `cwd` that is not a string is refused under
      `payload-error`.
- [ ] `vibe-guard` and `vibe unlock` find the Git directories through one
      function in `internal/guardrule`. `vibe unlock` runs no `git`.
- [ ] With `GIT_DIR` or `GIT_COMMON_DIR` set, `vibe unlock` writes the
      marker in `--repo-root`'s own common Git directory. Without a
      `.git` above `--repo-root`, it fails, writes nothing and says why.
- [ ] A repository command rule whose `args` cannot be compiled matches.
- [ ] Each case in "Testing Decisions" is a test through one of the three
      seams, and the cases not marked "unchanged" fail on `main` at
      8021031.
- [ ] Every existing guard, `guardrule`, `vibe unlock` and audit test
      passes unchanged. The guard corpus file is unchanged.
- [ ] Specs 0053 and 0058 and `docs/usage.md` describe the one path
      reading and `vibe unlock`'s marker location.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.
- [ ] The canary in `docs/usage.md` still refuses in a live session after
      `go install ./cmd/vibe-guard`.

## Further Notes

- The split of #163 into this spec (bugs) and spec 0070 (refactor) was
  proposed on 2026-11-25, so that the `..` fail-open ships without
  waiting for the refactor. Spec 0070 starts from a guard that has these
  tests.
- `vibe unlock` stops honouring `GIT_DIR` and `GIT_COMMON_DIR`. The
  other way, the guard honouring them, was rejected: the guard reads
  files only (A-03), and whether Claude Code passes the person's
  variables to hooks is not something the guard can rely on.
- A third-party guard may have the same `..` gap. Adding `..` cases to
  the guard corpus is a candidate for the next contract MINOR release,
  not part of this fix.
- **Assumption, not verified:** Claude Code sends absolute paths for its
  edit tools. If it ever sends relative ones, they are now read against
  the payload's `cwd`, which is where Claude Code runs the tool.
