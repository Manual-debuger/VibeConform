# Plan 0042: Workflow-neutral defaults

Implements `docs/specs/0042-workflow-neutral-defaults.md`. Tracks issue
#71. The spec and plan were accepted together at review (2026-10-07). The
review chose three things:

- move the root `CLAUDE.md` import to the `claude` integration;
- make every example workflow-neutral;
- keep this repository on `plan-triggered-sdd`.

Branch: `feat/0042-workflow-neutral`, off `main` after PR #69.

## Changes

- `internal/module/agents/claude/workflow.go`: `workflowResources`
  returns the root `importSection(ClaudeMDPath)` without a workflow too.
  The spec skill and the component imports stay behind the workflow. The
  resource order is unchanged with a workflow.
- `internal/module/agents/vibeskill/vibeskill.go`: the `development:`
  bullet says that leaving `workflow` out is recommended, and that every
  value, `direct` too, is a managed workflow.
- Tests:
  - `TestAdapterNeedsAWorkflow` and `TestComponentImports` (root import,
    no spec skill, no component imports without a workflow);
  - `TestResolveReturnsExpectedResourcesInOrder`;
  - `TestWorkflowClaudeAdapter` (the import survives workflow
    deselection);
  - new `TestClaudeWithoutWorkflow`;
  - the vibeskill golden.
- Examples: `python` keeps only `docs_layout`, and `monorepo` drops
  `development:`. All three examples and this repository were synced.
- Docs: `docs/usage.md` ("Development workflow", the non-goals list,
  component instructions, self-hosting, removal), `docs/adopting.md`,
  the README, `docs/architecture/overview.md`, status notes in specs
  0030, 0032 and 0041, ADR 0015, and the new ADR 0023.

## Found during implementation

- **A1, a missing `AGENTS.md`.** The Claude Code memory docs say nothing
  about an `@` import of a missing file. A `claude -p` run in a scratch
  repository showed no error or warning: a `CLAUDE.md` that held only
  the managed import plus one line of text, and no `AGENTS.md`. The
  run still read the rest of `CLAUDE.md`. An interactive session is
  UNVERIFIED.
- **Graphify routing.** The Graphify paragraph lives in the workflow
  section of `AGENTS.md` (spec 0035). A neutral `examples/python`
  therefore no longer has it. Its `graphify` skill and Git hooks remain.
  This is the same gap as the docs layout's: without a workflow, nothing
  in `AGENTS.md` routes agents. Spec 0043 closes it.
- **Clean build for sync.** A `go install` in a working tree with
  untracked files stamps `+dirty`. The binary used for sync was built
  from a clean detached worktree of the commit.
