# Plan 0031: `/spec` as a Claude Code skill

Implements `docs/specs/0031-spec-skill.md`. Tracks issue #49. Spec and
plan accepted at review (2026-10-01), together with specs 0032–0034, as
one plan for #49.

Branch: `feat/49-sdd-follow-ups`, off `main` after PR #48. One pull
request for specs 0031–0034.

## Changes

- `internal/module/agents/claude/workflow.go`:
  - `SpecSkill(mode)` builds `.claude/skills/spec/SKILL.md` from
    constants: a model-invoked front matter for the SDD workflows, a
    user-invoked one (`disable-model-invocation: true`) for `direct`,
    and one shared body around `workflow.SpecTemplate`;
  - `Retired()` names `.claude/commands/spec.md`.
- `internal/module/module.go`: the optional `Retirer` interface.
- `internal/cli/plan.go`, `prune.go`: `planRetired` adds a prune for every
  recorded, unresolved retired path of any module of the standard.
  `prunePlan.cause()` gives each message its reason: "X deselected", or
  the retirement reason.
- `internal/module/workflow/workflow.go`: the planning rules
  (`{planningRule}`, `{directRule}`) name the `spec` skill when claude is
  selected. Without claude they are byte-identical to spec 0030.
- Tests: the skill byte for byte per workflow, `Retired()`, the AGENTS.md
  golden and variant tests, and `TestSpecCommandRetired` for the
  unchanged, modified and unrecorded command.
- Docs: `docs/usage.md` (Claude Code adapter, removal), the architecture
  overview's removal rules, README, and spec 0030's status line.
- Self-hosting: this repository and `examples/python` synced with vibe
  `fcfc0dd`. Both reported
  `.claude/commands/spec.md: removed (replaced by .claude/skills/spec/SKILL.md)`.

## Found during implementation

- **Upgrading needed retirement.** The approved plan assumed the existing
  prune would remove the old command. It does not: ADR 0013's prune
  covers only paths that a deselected option would still produce. Spec
  0031 §3 adds `module.Retirer` for whole generated files only.
- **Pushing from a linked worktree corrupted the repository.** Git
  exports `GIT_DIR` to hooks there. The pre-push `task test` passed it on
  to the suites that run real git in temp directories. They then
  committed to this branch, set `core.bare=true`, and wrote a test
  identity into `.git/config`. Those were repaired by hand, and the
  junk commits were dropped. `TestMain` in `internal/cli`,
  `internal/module`, `internal/module/conformance` and
  `internal/module/tsrepotooling` now unsets `GIT_DIR`, `GIT_WORK_TREE`,
  `GIT_COMMON_DIR` and `GIT_INDEX_FILE`. A full `go test ./...` with
  `GIT_DIR` pointed at a canary repository left the canary untouched.

## Verification

Scratch git repository with `prod-go`, `always-sdd` and `docs_layout`,
synced with vibe `fcfc0dd`, Claude Code 2.1.286:

| Check | Result |
|---|---|
| Unit tests, lint, `task verify`, `task audit` | PASS |
| Upgrade of this repository and `examples/python` | PASS |
| A. `claude -p --permission-mode plan`, change prompt without `/spec`, 5 runs | PASS: 5 of 5 invoked the `spec` skill as their first tool call |
| B. `claude -p '/spec …'` (`acceptEdits`) | PASS, with notes below |
| C. Interactive plan mode, and interactive `/spec` | UNVERIFIED (needs a person) |
| CI on the pull request | see the PR |

Notes:
- **A.** Every run read the code, listed assumptions and asked about the
  ones that change the result, and wrote the spec and the plan to the
  plan file. None changed the repository. With no ExitPlanMode in `-p`
  mode, each asked for approval in its reply.
- **B.** The skill was listed as both a skill and a slash command. The
  run followed it: it wrote a spec from the template, raised its
  questions, and stopped without changing code. A permission prompt
  blocked its write to `docs/specs/`, so it returned the draft instead.
- As in plan 0030, a `hello.exe` build artifact was left untracked. No
  tracked file changed.
- No plan-mode reminder hook is needed: A shows that the skill is
  invoked by itself in plan mode.
