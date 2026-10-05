# ADR 0022: Shared skill content, written to each harness's skill path

## Status

Accepted. Implemented per `docs/specs/0041-vibeconform-skill.md`.
Narrows the "skills for other harnesses" non-goal of specs 0030 and
0031 (#50) for one skill.

## Context

Until spec 0041, every generated skill was Claude-only:
`.claude/skills/spec/` (spec 0031) and `.claude/skills/graphify/`
(spec 0035). Codex read only AGENTS.md. The `vibeconform` skill is
for every agent that works in an adopting repository, and AGENTS.md
cannot carry it: the generated part of AGENTS.md routes, has a 300-word
cap, and exists only with a development workflow (ADR 0015).

Codex now discovers repository skills in `.agents/skills/<name>/SKILL.md`,
in every directory from the working directory up to the repository root.
It needs the same `name` and `description` front matter as Claude Code,
and it may load a skill when the task matches its description.

## Decision

1. **The skill text lives in one package**,
   `internal/module/agents/vibeskill`. It depends on `module` and
   `manifest`, and no agent module depends on another.
2. **Each agent module writes that text to its harness's skill path**:
   `claude-config` to `.claude/skills/`, `codex-config` to
   `.agents/skills/`. Each module owns its own copy, so deselecting one
   agent removes only that copy, through the ordinary pruning of ADR 0013.
3. **The copies are byte-identical, and each harness decides when to load
   its copy.** No module adds a harness-specific trigger, and no AGENTS.md
   pointer is generated.
4. **The content varies by standard, and by CI provider for prod-mono,
   and by nothing else.** It is derived from the resolved context
   (components, profiles, `ci.provider`). `module.Context` is not
   changed.

## Consequences

- One edit updates every harness, and a test pins the bytes once per
  standard and provider.
- `.agents/` is a new top-level directory in a repository that selects
  `codex`. It is generated and committed, like `.codex/`.
- A harness without a project-skill directory gets nothing. That is
  accepted until such a harness is supported.
- Porting the `spec` and `graphify` skills to Codex can now follow the
  same pattern. That stays out of scope (#50).
