# ADR 0020: claude-config ignores `.claude/settings.local.json`

## Status

Accepted. Implemented per `docs/specs/0039-doctor-paths-graphify-coexistence.md`
(issue #56). Amends ADR 0016.

## Context

ADR 0016 lets an integration own a managed section of `.gitignore`, for
"output that it alone causes". Graphify's `graphify-out/` is that kind
of output.

`.claude/settings.local.json` is not output of `claude-config`. Claude
Code creates it, per developer. But spec 0039's coexistence layout puts
graphify's hook-guard hooks in that file, because the alternative,
`.claude/settings.json`, is managed. A layout that sends configuration
into a file that may be committed by accident is incomplete without the
ignore line. Today every adopter writes that line by hand, as this
repository did.

## Decision

1. ADR 0016's rule widens. An integration may also own a `.gitignore`
   section for a per-developer file of its runtime, when VibeConform's
   documented layout routes configuration into that file.
2. `claude-config` owns the section `claude`, at the bottom, with `#`
   markers, holding `.claude/settings.local.json`.
3. Every other ADR 0016 rule holds: one section per integration, the
   rest of `.gitignore` is the project's, and deselecting removes the
   section under spec 0026's rules.

## Consequences

- Every repository that selects `claude`, the default, gets the section
  on its next sync. Until then `vibe audit` reports it as missing,
  which is spec 0019's usual one-time cost.
- A project's own identical line is harmless: git applies both. This
  repository removed its own line.
- Codex has no equivalent file today, so `codex-config` gets no section.

## Alternatives considered

- **Document the line only.** That leaves every adopter to rediscover
  it. Rejected.
- **Put it in graphify's section.** The file belongs to Claude Code, not
  to graphify, and it matters without graphify too. Rejected.
