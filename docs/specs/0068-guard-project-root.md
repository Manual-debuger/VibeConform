# Spec 0068: The guard protects the project it runs in, also below the Git top

Status: approved (2026-11-24). Issue #180, found while reproducing #163
item 3. Part of the design review #167 (guard track). A bug fix with
regression tests. Amends spec 0053 ("Repository root") and the
`vibe unlock` section of `docs/usage.md`. No contract change: the
contract stays 1.2.0 and the guard corpus is unchanged.

## Problem

A person opens Claude Code in `outer/proj`. `outer` is a Git repository.
`outer/proj` has no `.git` of its own, but has its own
`.claude/settings.json`, its hooks and its guard policy. Claude Code sets
`CLAUDE_PROJECT_DIR=outer/proj`.

`vibe-guard` walks up from `CLAUDE_PROJECT_DIR` to the directory that
holds `.git`, and uses that directory, `outer`, as the root for
everything. Reproduced on 2026-11-24 (Windows, a `vibe-guard` built from
`main` at 8f7dcb3), with no unlock marker:

| Tool call (`CLAUDE_PROJECT_DIR=outer/proj`) | Expected | Actual |
|---|---|---|
| Write `outer/proj/.claude/settings.json` | refused, `guardrail-settings` | **allowed** (exit 0) |
| Write `outer/.claude/settings.json` | refused | refused |
| `echo hi`, denied by a rule in `outer/proj/.claude/hooks/policy.json` | refused, `no-x` | **allowed** (exit 0) |

The same project with its own `.git` refuses both. So there are two
fail-opens, both from the one root:

1. **The project's guardrail files are not protected.** The agent can
   change the hooks that run its own guard, with no unlock.
2. **The project's policy is not read.** The guard reads
   `outer/.claude/hooks/policy.json`, or the default when there is none.
   The project's deny and ask rules, and its mode, have no effect. The
   outer repository's exceptions loosen the project's guard.

Relative paths in a tool call, and exception globs, are also read
against `outer`, not against the project.

No `GIT_*` variable is involved. Anyone who opens Claude Code in a
subfolder of a monorepo, where that subfolder carries its own guardrails,
is affected.

## Solution

The guard separates two directories that are the same today:

- **The project root**: the directory the agent session runs in.
  Everything the guard reads or matches "in the repository" is anchored
  here: the policy, exception globs, relative paths and the
  guardrail-settings paths.
- **The Git directories** (Git directory and common Git directory): still
  found by walking up from the project root to the directory that holds
  `.git`. The unlock marker and `vibe-guard.log` stay in the common Git
  directory, where `vibe unlock` already writes the marker.

When the project root is below the Git top (the directory that holds
`.git`), the guard protects the guardrail settings of both: the
project's and the Git top's. The Git top's settings and hooks are what a
session opened at the Git top runs, so a session in a subfolder must not
change them without an unlock either. One unlock lifts both, because
both read the one marker.

A project that has its own `.git`, and every call made without
`CLAUDE_PROJECT_DIR`, decide as before.

## User Stories

1. As a developer who opens Claude Code in a subfolder of a monorepo, I
   want the agent's edits of the subfolder's `.claude/settings.json`
   refused while I have not unlocked, so that the agent cannot turn off
   its own guard.
2. As that developer, I want the agent's edits under the subfolder's
   `.claude/hooks/` refused while locked, so that the agent cannot
   rewrite the hook that runs `vibe-guard`.
3. As that developer, I want a relative path such as
   `.claude/settings.json` in an edit read against my project, so that
   the path I see in the tool call is the path the guard checks.
4. As that developer, I want a shell write such as
   `tee .claude/settings.json` from my project refused while locked, so
   that a shell write is checked like an edit.
5. As that developer, I want the monorepo's top-level
   `.claude/settings.json` and `.claude/hooks/` to stay refused while
   locked, so that an agent in a subfolder cannot change the guardrails
   of a session opened at the top.
6. As that developer, I want my subfolder's
   `.claude/hooks/policy.json` read, so that its deny and ask rules,
   exceptions and mode apply to the session.
7. As that developer, I want the monorepo's top-level policy not to
   apply to my subfolder session, so that the top's exceptions do not
   loosen my project's guard.
8. As that developer, I want an invalid `policy.json` in my subfolder
   to refuse every call under `policy-error`, so that a broken policy
   fails closed as it does at a Git top.
9. As that developer, I want exception globs in my subfolder's policy
   read relative to my subfolder, so that a glob I write against my
   project matches my project's paths.
10. As that developer, I want `vibe unlock`, run in my subfolder, to let
    the agent change the subfolder's guardrail settings, so that the
    unlock works without knowing where the Git top is.
11. As that developer, I want `vibe unlock`, run anywhere in the same
    Git repository, to have the same effect, so that I do not have to
    `cd` first.
12. As that developer, I want the agent's next Stop to end the unlock
    (`vibe-guard relock` with `CLAUDE_PROJECT_DIR` set to my subfolder),
    so that the unlock does not outlast the turn.
13. As that developer, I want `vibe-guard relock` run by hand in my
    subfolder to end the unlock, so that I can lock early.
14. As that developer, I want the agent refused when it writes the
    unlock marker in the monorepo's common Git directory, so that it
    cannot unlock itself from a subfolder.
15. As that developer, I want observe mode's log in the common Git
    directory, as today, so that I find it in one place for the whole
    repository.
16. As that developer, I want `vibe-guard explain` with
    `CLAUDE_PROJECT_DIR` set to my subfolder to decide as the hook does,
    so that I can debug a refusal.
17. As a developer whose project has its own `.git`, I want every
    decision unchanged, so that the fix costs me nothing.
18. As a developer in a linked worktree, I want the unlock marker still
    shared through the common Git directory, so that worktrees behave as
    before.
19. As a developer who runs `vibe-guard` without `CLAUDE_PROJECT_DIR`
    (a person at a terminal, or a hook in another runtime), I want the
    root still found by walking up from the payload's `cwd`, or from the
    working directory, to `.git`, so that those calls decide as before.
20. As a developer whose project has no Git repository above it, I want
    the guard to keep refusing guardrail edits with no possible unlock,
    so that a missing Git directory never looks like an unlock.
21. As a developer with a sibling project `outer/other`, I want an edit
    of `outer/other/.claude/settings.json` from a session in
    `outer/proj` treated as an ordinary file, as a nested project's
    settings are today, so that the guard protects the session's own
    guardrails, not every `.claude` folder in the repository.
22. As a maintainer of VibeConform, I want one place that decides the
    project root and the Git directories for `hook`, `explain` and
    `relock`, so that the three cannot disagree again.
23. As a maintainer, I want regression tests through `vibe-guard`'s
    subprocess seam that fail on `main` today, so that this fail-open
    cannot come back.
24. As a maintainer, I want a test that writes the marker where
    `git rev-parse --git-common-dir` points from the subfolder, so that
    the guard and `vibe unlock` are shown to agree on the marker's place.
25. As an adopter reading `docs/usage.md`, I want it to say that one
    unlock covers every project in the same Git repository, so that I
    know what an unlock lifts.
26. As an implementer of another agent guard, I want spec 0053's
    definition of the root to say which directory is the project root
    and which holds the marker, so that I can match the reference guard.

## Implementation Decisions

### Which directory is which

- **Project root**: `CLAUDE_PROJECT_DIR` as given (made absolute), when
  it is set. Otherwise the payload's `cwd`, else the process's working
  directory, walked up to the directory that holds `.git`, as today.
  Without `.git` above it, the start directory, as today.
- **Git top**: the directory that holds `.git`, found by walking up from
  the project root. It equals the project root when the project has its
  own `.git` and whenever `CLAUDE_PROJECT_DIR` is not set. It is empty
  when no `.git` is found.
- **Git directory and common Git directory**: read from the Git top's
  `.git`, exactly as today (a directory, or a `gitdir:` file and the
  `commondir` file it points to). No `git` is run (A-03).

`CLAUDE_PROJECT_DIR` is not walked up, because Claude Code sets it to
the project root (spec 0058's assumption). Walking up is what put the
root at `outer`.

### The repository value

- The guard's repository value keeps its root field and its meaning
  changes to "project root". It gains a field for the Git top. Every
  current reader of the root (policy file, exception globs, edit-path
  matching, shell-write path resolution, the Git Bash drive form, the
  marker's relative-path check) keeps reading the root, and so reads the
  project root.
- One function in `internal/guard` builds the repository value from the
  environment and the payload's `cwd`. `hook`, `explain` and `relock`
  call it. It replaces today's start-directory step followed by the
  walk, which is spread over the three entry points. The walk itself
  stays where it is; moving it into `internal/guardrule` and sharing it
  with `vibe unlock` is #163's work.

### The guardrail-settings rule

- A path is a guardrail setting when, relative to the project root, it is
  `.claude/settings.json` or under `.claude/hooks/`, **or**, when the Git
  top is not the project root, the same relative to the Git top.
  Matching is case-insensitive as today.
- The unlock is the same for both: a valid marker in the common Git
  directory. Nothing else in the rule changes: the marker check, the
  shell-write rules and the order of path rules stay.
- `guard-policy-file` already matches any `.claude/hooks/policy.json`
  and is unchanged.

### The policy

- `policy.json` is read from the project root only. The Git top's
  policy, when the two differ, is not read and not merged.

### `vibe unlock` and `vibe-guard relock`

- `vibe unlock` does not change. Its `git rev-parse --git-common-dir`,
  run in the project root or anywhere below the Git top, names the same
  common Git directory the guard reads.
- `vibe-guard relock` deletes the marker in the common Git directory
  found from the project root, through the shared function.
- The marker is per Git repository, not per project: one unlock lifts
  the guardrail settings of every project in that repository. This is
  how linked worktrees already share it, and it is documented, not
  changed.

### Documentation

- Spec 0053, "Repository root": the definition above, with the project
  root and the Git directories named apart.
- `docs/usage.md`: "What it reads" names the project root; the
  `vibe unlock` section says that one unlock covers every project in the
  Git repository and that both the project's and the Git top's guardrail
  settings are protected when they differ.
- `docs/architecture/` only if it describes the root (check during the
  documentation sync).
- No ADR: no new dependency and no reversal of an earlier decision.

## Testing Decisions

- **A good test runs `vibe-guard` as a person or Claude Code does and
  checks the exit code and the rule named.** It does not read the
  repository value or call internal functions.
- **One seam, the existing one** (agreed with the user on 2026-11-24):
  `vibe-guard`'s subprocess harness (`runGuard`), next to
  `TestGuardrailSettingsOnlyInTheRepository`, `TestUnlockInLinkedWorktree`
  and `TestRelock`. No in-process test of the walk; that seam is #163's.
- **Fixture.** A fixture repository whose `.git` directory is at the
  top, with a project in a subdirectory, and `CLAUDE_PROJECT_DIR` set to
  the subdirectory. The harness already unsets `CLAUDE_PROJECT_DIR` and
  every `GIT_*` variable; each case sets `CLAUDE_PROJECT_DIR` itself.
- **Cases** (each fails on `main` unless marked "unchanged"):
  - Write, Edit and MultiEdit of the project's `.claude/settings.json`
    and a file under its `.claude/hooks/`, absolute and relative, with a
    payload `cwd` in the project: refused under `guardrail-settings`.
    The relative `.claude/settings.json` is unchanged (see "Code-review
    notes").
  - The same for the Git top's settings: refused (unchanged).
  - A sibling project's `.claude/settings.json`: allowed (unchanged).
  - `tee .claude/settings.json` from the project: refused.
  - With a valid marker in the top's common Git directory, the project's
    and the top's settings edits are allowed; with an expired marker,
    refused.
  - A Write of `vibe-unlock` in the top's `.git`: refused (unchanged:
    any path with a `.git` part was already refused).
  - A deny rule in the project's policy refuses its command; a deny rule
    only in the top's policy does not; an exception only in the top's
    policy does not lift a rule; an invalid project policy refuses under
    `policy-error`.
  - `vibe-guard relock` with `CLAUDE_PROJECT_DIR` set to the project
    deletes the marker in the top's `.git` (unchanged: the old walk
    already reached the top's `.git`).
  - `vibe-guard explain --tool Write <project settings>` with the same
    environment exits as the hook does and names `guardrail-settings`,
    for the absolute path, the relative `.claude/settings.json` and a
    shell write (`tee .claude/settings.json`).
  - Observe mode in the project writes `vibe-guard.log` in the top's
    common Git directory (story 15).
  - **Agreement with `vibe unlock`**: with a real `git init` of the top
    (skipped when `git` is not on `PATH`), the marker is written where
    `git rev-parse --git-common-dir`, run in the project, points; the
    project's settings edit is then allowed.
- **Regression of what must not change.** Every existing guard test, the
  guard corpus (`TestCorpus`, unchanged file) and the audit's A-02, A-03,
  A-08, A-09 and R-01 tests pass unchanged. Together they cover a
  project with its own `.git`, linked worktrees and calls without
  `CLAUDE_PROJECT_DIR`.
- **Prior art:** `TestGuardrailSettingsOnlyInTheRepository` (paths in
  and out of the root), `TestUnlockInLinkedWorktree` (marker in the
  common directory), `TestRelock` (relock from a subdirectory without
  `CLAUDE_PROJECT_DIR`), `TestObserveModeLogsInTheCommonGitDir`.
- **Code-review notes.** The review of the integration branch found
  three cases above that also pass on `main`; they are marked
  "unchanged" and kept as regression tests:
  - relock with `CLAUDE_PROJECT_DIR` set: the old walk already reached
    the top's `.git`;
  - a Write of `vibe-unlock` in the top's `.git`: any path with a `.git`
    part was already refused;
  - a relative `.claude/settings.json`: it was already refused,
    resolved against the top.

  The review also added the `explain` cases for a relative path and a
  shell write, and the observe-mode log case, which hold the acceptance
  criterion "by `vibe-guard hook` and by `vibe-guard explain`" and story
  15.

## Out of Scope

- **#163.** Making `Guard` deeper, one path normaliser, moving the walk
  into `internal/guardrule` and having `vibe unlock` use it, and
  `GIT_DIR`/`GIT_COMMON_DIR` (fail-closed, tracked there).
- **A per-project unlock marker.** One marker per Git repository is
  kept.
- **Finding the project root without `CLAUDE_PROJECT_DIR`**, for example
  by looking for the nearest `.claude/settings.json`. A person who runs
  `vibe-guard explain` for a project in a subfolder sets
  `CLAUDE_PROJECT_DIR`.
- **Protecting sibling projects' guardrail settings.**
- **The audit's own repository root** (`vibe audit`, `vibe doctor`).
- **Codex sessions**, which have no hooks (ADR 0011).
- **Git Bash's `/d/…` form of `CLAUDE_PROJECT_DIR`.** Read as today.

## Acceptance Criteria

- [ ] With `CLAUDE_PROJECT_DIR` set to a subdirectory without `.git`, and
      no valid marker, an edit or shell write of the project's
      `.claude/settings.json` or of a file under its `.claude/hooks/` is
      refused under `guardrail-settings`, by `vibe-guard hook` and by
      `vibe-guard explain`.
- [ ] In the same setup, the Git top's `.claude/settings.json` and
      `.claude/hooks/` stay refused, and a sibling project's settings are
      allowed.
- [ ] In the same setup, the project's `policy.json` is the one read:
      its rules, exceptions and mode apply, the top's do not, and an
      invalid project policy refuses under `policy-error`.
- [ ] A valid marker written where `git rev-parse --git-common-dir`
      points from the project allows those edits; `vibe-guard relock`
      with `CLAUDE_PROJECT_DIR` set to the project removes it.
- [ ] `hook`, `explain` and `relock` get the project root and the Git
      directories from one function in `internal/guard`.
- [ ] Each case in "Testing Decisions" is a test through `runGuard`, and
      the new cases fail on `main` at 8f7dcb3.
- [ ] Every existing guard test and audit test passes unchanged; the
      guard corpus file is unchanged.
- [ ] Spec 0053's "Repository root" and `docs/usage.md` ("What it
      reads", "`vibe unlock`") describe the project root and the shared
      marker.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.
- [ ] The canary in `docs/usage.md` still refuses in a live session after
      `go install ./cmd/vibe-guard`.

## Further Notes

- The choice between "protect the project's and the Git top's paths" and
  "treat the project as the only root" was left open in #180. The user
  chose both on 2026-11-24: it fails closed, and one unlock still lifts
  both.
- The policy fail-open (the project's `policy.json` not read) was found
  while reproducing for this spec. It has the same cause and the same
  fix, so it is in scope.
- **Assumption, not verified:** Claude Code sets `CLAUDE_PROJECT_DIR` to
  the directory the session was opened in, also when that is below the
  Git top. #180's steps and spec 0058 assume this; the fix relies on it.
- **Assumption, not verified:** whether Claude Code also loads the Git
  top's `.claude/settings.json` for a session opened in a subfolder.
  Protecting the top's settings is right in either case.
- Fix this before #163's refactor (#167's guard track), so the refactor
  starts from a guard that has these tests.
