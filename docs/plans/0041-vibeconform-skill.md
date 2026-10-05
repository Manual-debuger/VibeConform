# Plan 0041: A `vibeconform` skill for adopter agents

Implements `docs/specs/0041-vibeconform-skill.md`. ADR:
`docs/decisions/0022-agent-skill-paths.md`.
Status: approved 2026-10-05 and implemented.

Branch: `feat/0041-vibeconform-skill`. One pull request.

## Design

- **The content lives in `internal/module/agents/vibeskill`.** It is Go
  constants: the front matter, a generic body with a `{standard}` slot,
  one part per standard, and the CI files each provider adds to the
  removal list. `Content(Kind)` assembles them.
- **The kind comes from the resolved context, so `module.Context` is
  not changed.** `KindOf` reads it:
  - non-empty `Components` means `prod-mono`, and `module.CIProvider`
    gives `github`, `gitlab` or `none`;
  - otherwise the single entry of `Profiles` gives the standard.

  A context that cannot tell, such as a module resolved in isolation,
  gets no skill.
- **`Resources(skillsDir, mctx)` returns the one generated resource.**
  `claude-config` calls it with `.claude/skills`, after the workflow
  resources and before the `.gitignore` section. `codex-config` calls it
  with `.agents/skills`, after `.codex/hooks.json`.

## Tests

- `vibeskill_test.go` pins every kind byte for byte. Its expected text
  is written out separately from the implementation. It also checks the
  900-word cap, `KindOf` and `Resources`.
- `internal/standard/vibeskill_test.go` covers every standard and agent
  selection:
  - each selected agent gets its copy, with no `development:`;
  - the two copies are byte-identical;
  - the content is that standard's.
- Tests whose expectations change:
  - `TestComponentImports`: claude resolves four resources without a
    workflow, not three.
  - `TestDeselectRemovesUnchangedFiles`: deselecting codex also removes
    `.agents/skills/vibeconform/SKILL.md`, and claude's copy stays.
  - `TestLocalLefthookIsNotManaged`: exempts the skill's prose file by
    exact path. The skill names `lefthook.local.yml` to tell agents the
    file is the project's. No Go code reads or writes it.
- `TestGraphifyFollowsSelection` holds unchanged: the skill describes
  `intelligence` without naming the integration.

## Steps

1. Spec, plan and ADR.
2. The package, the two emissions, and the tests.
3. `docs/usage.md`: the module tables, a "The `vibeconform` skill"
   section, and "Removing VibeConform".
4. Commit. Then rebuild `vibe` from the clean tree and sync this
   repository and `examples/*`, so `vibe_version` records a clean
   pseudo-version. Commit the generated files with `.vibe/state.yaml`.
5. `task verify`, `task audit`, PR.
