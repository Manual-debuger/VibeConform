# Plan 0035: Optional Graphify integration

Implements `docs/specs/0035-graphify.md`. Tracks issue #52. The spec was
accepted on 2026-10-02 and amended in review: the lefthook jobs run
`task graph:update`, because lefthook on Windows mangles an inline shell
`run:`.

Branch: `feat/52-graphify`, off `main`. One pull request, merged with a
merge commit.

## Resolved design questions

**Where does the graphify module live?** In a new package,
`internal/module/intelligence/graphify`. The module emits the
`.gitignore` section, and also the Claude skill, but only when `claude`
is in `mctx.Integrations`. That way the claude module stays unaware of
graphify, and deselection prunes the skill through the existing
`planPrunes` trial selection. The module declares `graphify` through
`ToolRequirer` with `Optional: true`.

**How do core files learn about the selection?** A new helper,
`module.Selected(mctx, name)`, returns false for a nil context or nil
integrations. This is the opposite polarity to `WantsAgentHooks`: an
unknown selection must mean *off*, so `TestAgentHooksFollowClaude`'s nil
baseline keeps holding.

**How do the three byte-template repo-tooling modules add content?** A
helper in package `module`, `AddGraphify(taskfile, lefthook []byte,
hooks bool)`, does exact-anchor insertion. It uses the same rule as
`tsrepotooling`'s `replaceOnce`: the anchor must occur exactly once.

- The helper adds the `graph:update` task (before `hook:context`, or at
  the end of the tasks).
- It adds the `Graphify:` line after the `Task:` echo in `hook:context`,
  when `hooks` is true.
- It appends the `post-commit` and `post-checkout` jobs to `lefthook`.

`prod-ts` applies it after the Prettier rewrites. Monorepo uses a
`[[- if .Graphify]]` block in both of its templates instead.

**Doctor seam.** These fields are added:

- `module.Tool.Optional` and `doctor.Tool.Optional`: a missing optional
  tool is `WARN` with an install hint.
- `doctor.Env.Open`: streams `graph.json`.

A new `doctor.Graphify(ctx, env, root) []Result` uses `env.Run("git",
…)` for HEAD, the dirty state, and `check-ignore`. The freshness probe
walks `json.Decoder.Token()` at depth 1 until it finds
`built_at_commit`, skipping nested values without buffering them.

**AGENTS.md bullet.** `workflow.LayoutContent` takes the selection
through a small options struct rather than a fourth positional bool.
Callers in the tests are updated. The bullet is a short "Repository
intelligence" paragraph placed before Verification, and the 300-word
cap must still hold.

## Steps

1. **Catalog and selection.**
   - Add `graphify` under `intelligence` in `standard.go` `catalog()`,
     after codex.
   - Remove the dead "no code-intelligence providers" branch from
     `options.go`.
   - Update `options_test.go`'s rejected-name case to `unknown
     intelligence integration (valid: graphify)`.
2. **Module package.** `intelligence/graphify`:
   - the `.gitignore` section (`HashComment`, `Bottom`, id `graphify`,
     content a one-line comment plus `graphify-out/`);
   - the skill as Go constants (for CRLF safety, like the spec skill),
     emitted only when claude is selected;
   - `Tools()` with `Optional`.
3. **`module.Selected` and `module.AddGraphify`.**
   - Wire them into repotooling, tsrepotooling and pyrepotooling.
   - In monorepotooling, add the template fields and `[[if]]` blocks.
   - The `graph:update` task script, in Task's shell: the `command -v`
     skip branch, then `graphify update .`, or a "graph may be stale"
     line on failure. It always exits 0. Its `desc:` mentions the
     post-commit hook.
4. **Workflow section.** Add the repository-intelligence bullet when
   `Selected(mctx, "graphify")`.
5. **Optional tools and doctor.**
   - In `tools.go`, word the warning for optional tools.
   - Add `doctor.Tool.Optional` and the `Env.Open` seam. Update
     `System()` and both fakes.
   - Add `doctor.Graphify`, called from `runDoctor` when it is selected.
6. **ADR 0016.** Managed sections in `.gitignore`, for integration-owned
   ignore entries only. This narrows spec 0026's non-goal.
7. **Example.**
   - Add `intelligence: [graphify]` to `examples/python/vibe.yaml`.
   - Build, then run `./bin/vibe sync --repo-root examples/python` and
     `./bin/vibe audit`.
   - Commit the files and the state.
8. **Docs.**
   - `docs/usage.md`: the catalog row, a Graphify subsection (setup on
     Windows and Linux, the verified version, what is generated,
     troubleshooting, limitations, removal, license), the doctor
     section, "What `vibe.yaml` means today", "Removing VibeConform",
     and the line-979 invalid example.
   - `docs/architecture/overview.md`: intelligence.
   - Spec 0026's §1 error text gets an "as of spec 0035" note.

## Tests (by acceptance criterion)

1. **Default resolution.** Every standard with `intelligence` absent
   resolves byte-identically to a nil-graphify baseline. The existing
   default tests stay green (`TestPlanDefaultsKeepAgentsAndHooks`,
   wiring).
2. **Resource set.** Graphify-selected resolution is checked under
   `agents` set to `[claude, codex]`, `[codex]` and `[]`: the skill is
   present or absent, the `Graphify:` line is present or absent, and
   `graph:update` and the lefthook jobs are always present.
3. **`.gitignore`.**
   - A section test keeps user lines and creates the file when it is
     absent.
   - A real git work tree test: after sync, `git check-ignore
     graphify-out/graph.json` matches.
4. **Lefthook jobs and `graph:update`.**
   - The jobs appear only under `post-commit` and `post-checkout`.
   - A runtime test runs `task graph:update` with PATH stripped of
     graphify. It exits 0 and prints the skip line.
   - The `hook:context` allowlist test is extended to resolved,
     graphify-selected Taskfiles.
5. **Doctor.** Each status in the spec's table, through fake `Env`.
   Graphify-only WARN results exit 0. `TestDoctorWritesNothing` gains a
   graphify-selected variant.
6. **Sync warning.** A missing optional tool warns, and sync exits 0.
7. **Deselection.**
   - It removes the section and the skill.
   - A modified skill is kept as `RemoveConflict`.
   - An untracked `graphify-out/` file survives.
   - A created `.gitignore` is deleted when only the section was in it.
8. **Verification independence.** A new test asserts that resolved,
   graphify-selected output keeps `graphify` out of:
   - the `verify`, `verify-ci` and `verify:fast` closures;
   - lefthook `pre-commit` and `pre-push`;
   - the CI workflow templates.
9. **Example.** `TestExamplesAreConformant` with `examples/python`
   synced.
10. The workflow section stays under its word cap with graphify.

## Verification

- `task verify:fast` while working.
- `task verify` and `task audit` before declaring done.
- After a rebuild, run `vibe audit` for the repository root and every
  example.
- Manual integration on Windows, in a scratch clone of
  `examples/python`:
  - `lefthook install`, then a commit: `graphify-out/graph.json`
    appears and its `built_at_commit` equals HEAD;
  - `vibe doctor` shows `PASS` for graphify;
  - after another commit with the hook skipped: `WARN stale`;
  - with graphify off PATH: the skip line.
- CI: the Ubuntu and Windows test matrix and `examples.yml`. Linux
  lefthook behavior is covered there only through the `task graph:update`
  runtime test.

The ledger at the end lists unit tests, CI, and the manual Windows
integration as separate lines.
