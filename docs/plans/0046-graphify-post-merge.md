# Plan 0046: Graphify rebuilds the graph after a pull

Implements `docs/specs/0046-graphify-post-merge.md`.
Status: approved 2026-10-15 and implemented as planned.

## Repository impact

| Path | Change |
|---|---|
| `internal/module/graphify.go` | `GraphHookJobs` gains `post-merge`; `GraphUpdateTask` comment and `desc`; `AddGraphify` doc comment |
| `internal/module/graphify_test.go` | hooks loop covers `post-merge` (the regression test) |
| `internal/standard/graphify_test.go` | also requires `post-merge:` |
| `internal/standard/testdata/lefthook-0036/*-graphify.yml` | regenerated: gain the `post-merge` job, nothing else |
| `internal/module/intelligence/graphify/graphify.go` | skill text (line 46) and the `RequiredTools` reason (line 112) |
| `internal/module/intelligence/graphify/graphify_test.go` | pins both texts naming all three hooks |
| `examples/python/{lefthook.yml,Taskfile.yml,.claude/skills/graphify/SKILL.md,.vibe/state.yaml}` | synced |
| `docs/usage.md` | the Graphify section, the integrations table, the `lefthook.local.yml` note, D4 |
| `docs/architecture/overview.md` | the Graphify paragraph names `post-merge` |
| `docs/specs/0035-graphify.md` | AC 4 points to spec 0046 |
| `docs/specs/0046-*.md`, this plan | status, D2 result |

The dependency direction is unchanged. No new dependency. This
repository does not select Graphify, so its own sync changes only
`.vibe/state.yaml`, if anything.

## Template

`GraphHookJobs` gets a third hook after `post-checkout`, with the same
job name:

```yaml
post-merge:
  commands:
    graphify-update:
      run: task graph:update
```

The `graph:update` comment says "The post-commit, post-checkout and
post-merge Git hooks run it". Its `desc` becomes "Rebuild the Graphify
graph in graphify-out/ (run by the post-commit, post-checkout and
post-merge hooks)". The doc comment on `GraphHookJobs` says why
`post-merge` is there: a fast-forward pull fires only that hook.

## Golden files

`TestLefthookOnlyGainsExtends` compares each resolved `lefthook.yml`,
minus the spec 0036 `extends` block, to `testdata/lefthook-0036/`. The
four `-graphify` goldens change with this spec. I regenerate them with
`VIBE_WRITE_GOLDEN=1` and check with `git diff` that each one gains only
the four `post-merge` lines. The non-Graphify goldens must not change,
and that is D5.

## Steps (one commit each, in order)

1. Commit this plan (the spec is in `bb95910`).
2. **Red.** Add `post-merge` to the hooks loop in
   `internal/module/graphify_test.go:95`, the `post-merge:` check in
   `internal/standard/graphify_test.go`, and the skill text and reason
   assertions in the intelligence test. Run them and record that they
   fail. Do not commit yet.
3. **Green.** Change `graphify.go` (both packages) and regenerate the
   Graphify goldens. Run `go test ./...`. Commit steps 2 and 3 together,
   as `fix: Graphify rebuilds the graph after a pull (spec 0046, #82)`.
   The red run goes in the PR description.
4. **Sync.** Build `vibe` from a clean worktree at the step 3 commit and
   install it in `~/go/bin`. Run `vibe sync` in `examples/python` and in
   this repository, then `task audit` in each, and in
   `examples/typescript` and `examples/monorepo`, whose files must not
   change. Commit as
   `chore: sync this repository and the examples (spec 0046)`.
5. **D2 by hand.** Copy `examples/python` into a new Git repository in
   the scratch directory (the upstream), and clone it. In the working
   clone, run `lefthook install` and then `task graph:update`. Commit in
   the upstream, then run `git pull --ff-only` in the working clone.
   Expect `built_at_commit` in `graphify-out/graph.json` to equal `HEAD`,
   and `vibe doctor` to show the graph as current. Repeat with
   `git pull --rebase` for a fast-forward. Record the result in the spec.
6. **Docs.** `docs/usage.md`:
   - every hook list names `post-merge`;
   - D4, next to the existing "Graphify's hook jobs are not yours to
     copy" note: delete a `post-merge` workaround from
     `lefthook.local.yml`, because a job under another name rebuilds the
     graph twice;
   - the `post-checkout` latency limitation also names `post-merge`
     (every pull).

   Also `docs/architecture/overview.md`, spec 0035 AC 4 (a pointer), and
   the status of the spec and this plan.
7. `task verify`, `task audit`, push, open the PR, and wait for CI.

## Verification ledger

- PASS: unit tests (`go test ./...`, Windows), after the sync commit.
- PASS: regression test, red before the fix (the hooks loop, the
  `post-merge:` check, the skill text and the reason all failed) and
  green after it.
- PASS: golden diff. Only the four `-graphify` goldens changed, each by
  the `post-merge` job alone.
- PASS: `task audit` in this repository and the three examples. Only
  `examples/python` changed content; the others changed `vibe_version`.
- PASS: D2 by hand, `pull --ff-only` and fast-forward `pull --rebase`.
