# Spec 0046: Graphify Rebuilds the Graph After a Pull

Status: accepted and implemented (2026-10-15). Issue #82. Replaces acceptance criterion 4 of spec 0035
("They exist only under `post-commit` and `post-checkout`"). The rest of
spec 0035 stays as it is.
Settled with the user on 2026-10-14 in the grilling of #82, #84 and #85.
Follows spec 0045 (#87) and comes before the fix for #84. One tag after
all three are merged.

## Problem

With `intelligence: [graphify]`, the generated `lefthook.yml` runs
`task graph:update` on `post-commit` and `post-checkout` only (spec
0035). A fast-forward pull fires neither hook, so the graph goes stale
after every pull of merged work. An adopting repository that merges
through merge requests saw `vibe doctor` report the graph ten commits
behind, after two `git pull --ff-only`.

Probed on 2026-10-15 with Git 2.53.0 (Windows), with each hook logging
its name in a scratch clone:

| Command | Hooks fired |
|---|---|
| `git pull --ff-only` | `post-merge` |
| `git pull --rebase`, fast-forward | `post-merge` |
| `git pull --rebase`, diverged | `post-checkout`, `post-commit`, `post-rewrite` |

So a fast-forward pull fires `post-merge` whether it is configured to
merge or to rebase. A rebase over local commits already fires
`post-checkout`.

Verified by hand on 2026-10-15, on Windows (Git 2.53.0, graphify
0.8.18): a scratch repository built from the committed `examples/python`
and a clone of it, with `lefthook install` run in the clone.
- After `git pull --ff-only` of a new upstream commit, `post-merge` ran
  `graphify-update`, `built_at_commit` equalled `HEAD`, and `vibe doctor`
  reported `graphify graph  current: built at HEAD`.
- The same after a fast-forward `git pull --rebase`.

## Constraints

- Principle 3: the generated hooks depend on `task`, `lefthook` and
  `graphify` only, never on `vibe`. `task verify` and CI never run
  graphify (spec 0035, AC 8).
- The graph never gates work: no Graphify job under `pre-commit` or
  `pre-push`. `task graph:update` always exits 0 (spec 0035).
- The logic stays in the `graph:update` task. A lefthook job runs only
  `task graph:update` (lefthook on Windows mangles shell quoting in
  `run:`).
- `lefthook.local.yml` is the adopter's file (ADR 0017). VibeConform
  never reads or writes it.
- Without `graphify` selected, every generated file stays byte-identical
  (spec 0035, AC 1).
- Every bug fix needs a regression test (AGENTS.md).

## Assumptions

- `vibe sync` runs `lefthook install`, so an adopter's existing clone
  registers the new Git hook on the next sync. Nothing else is needed.
- `post-merge` also fires on a local `git merge` and on a non-fast-forward
  `git pull` that creates a merge commit. A rebuild there is wanted too,
  because `HEAD` moved.
- A non-fast-forward pull with a merge commit does not fire
  `post-commit` as well, so the graph is not rebuilt twice. (Not probed;
  a double rebuild would cost time only.)

## Desired Behavior

- **D1.** With `graphify` selected, the generated `lefthook.yml` has a
  `post-merge` hook with a `graphify-update` job that runs
  `task graph:update`, beside the existing `post-commit` and
  `post-checkout` jobs of the same name.
- **D2.** After a fast-forward `git pull`, with or without `--rebase`,
  the graph is rebuilt, and `vibe doctor` does not report it stale.
- **D3.** Every place that lists the Git hooks that run
  `task graph:update` names all three: the task's `desc` and comment in
  the generated `Taskfile.yml`, the generated Graphify skill, the reason
  `vibe` gives for the Graphify resources, `docs/usage.md` and
  `docs/architecture/overview.md`.
- **D4.** `docs/usage.md` tells adopters who added the issue's
  workaround to `lefthook.local.yml` to delete it. A workaround job named
  `graphify-update` merges into the managed job and is harmless. A job
  with another name runs a second rebuild after every pull.
- **D5.** Without `graphify` selected, nothing changes.

## Non-goals

- A `post-rewrite` job. A rebase already fires `post-checkout`, and
  `git commit --amend` already fires `post-commit`.
- Rebuilding the graph from `vibe sync`, `verify` or CI.
- Detecting or rewriting an adopter's `lefthook.local.yml`.
- Making the rebuild cheaper or asynchronous.

## Acceptance Criteria

- [x] A regression test fails before the fix and passes after it: with
      `graphify` selected, the generated `lefthook.yml` of every
      single-language standard has `post-merge`, `post-commit` and
      `post-checkout` `graphify-update` jobs that run
      `task graph:update`, and no Graphify job under `pre-commit` or
      `pre-push`.
- [x] The test for a manifest without `intelligence` passes unchanged
      (D5).
- [x] Spec 0035's AC 4 points to this spec.
- [x] D3: the texts named there list all three hooks. A test pins the
      skill text and the resource reason.
- [x] D4 is in `docs/usage.md`.
- [x] `examples/python` is synced and conformant.
- [x] D2 is checked once by hand in a scratch clone of
      `examples/python` with graphify installed: after
      `git pull --ff-only` of a new commit, `graph.json`'s
      `built_at_commit` equals `HEAD`. The result is recorded here.
- [x] `task verify` and `task audit` pass.
