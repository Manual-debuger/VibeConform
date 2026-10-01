# ADR 0015: A Derived Workflow Section in AGENTS.md

## Status

Accepted. To be implemented per `docs/specs/0030-plan-triggered-sdd.md`.
Revises the AGENTS.md/CLAUDE.md non-goal in `docs/usage.md`. Uses ADR
0014's managed sections and option catalog.

## Context

Since M1, `docs/usage.md` has listed `AGENTS.md` and `CLAUDE.md` as
deliberately not managed. Generating them "would produce exactly the
fabricated, ignored-by-everyone instruction file this project argues
against". Principle 1 says the same from the other side: agent
instructions are for judgment, and a rule a tool can check belongs in
the tool.

Issue #42 asks for guidance that has nowhere to go but agent
instructions:

- how normal work differs from work in a planning context;
- keeping the spec (WHAT) apart from the plan (HOW);
- where specs and ADRs live;
- which task is the completion gate;
- how to report verified versus unverified results.

None of this is mechanical. Today every project writes it by hand, or
goes without. That includes this repository, whose AGENTS.md carries a
hand-written version.

Two things changed that make a managed version possible without
becoming the file the non-goal warns about:

- ADR 0014 gave VibeConform managed sections. It can own a delimited
  region of a project's file and leave every other byte alone.
- `vibe.yaml` can now select options with values. So the content can be
  **derived** from what the repository actually chose, rather than
  generic.

## Decision

1. **VibeConform may own one section of AGENTS.md.** It is the
   `workflow` section, with HTML markers, placed at the bottom of the
   file, and only when `development.workflow` is selected. The rest of
   AGENTS.md stays the project's. VibeConform never generates the whole
   file.

2. **The section is derived and routing-only, and it is bounded.**
   - Every line follows from the selection: the workflow value, the docs
     layout, the agent integrations.
   - It names documents and tasks; it does not restate them. A rule that
     a tool enforces appears only as the name of that tool.
   - A test holds it to at most 300 words for every selection. A section
     that cannot meet that budget is a sign that the content belongs in
     `docs/` or in tooling.

3. **"Planning context" is harness-agnostic, and adapters are
   per-runtime.**
   - The section defines planning context as the harness's planning mode
     or an explicit request for a spec.
   - Under ADR 0011, each agent runtime's module adds its own entry
     point. `claude-config` adds a `/spec` command and a `CLAUDE.md`
     section that imports AGENTS.md.
   - No hook detects plan mode.

4. **Workflow and docs layout are options in the catalog**, under a
   `development:` map in `vibe.yaml`. They are not new standards and
   not a `standards:` map. ADR 0014 §1 already composes options and
   prunes their resources. A second composition mechanism would
   duplicate it.

5. **The non-goal is narrowed, not dropped.** `docs/usage.md` now
   reads: AGENTS.md and CLAUDE.md are yours, except the one section each
   that a selected workflow adds.

## Consequences

- Adopters who opt in get a short, maintained workflow contract. Their
  own AGENTS.md prose is untouched, and a hand edit inside the section is
  reported like any other drift.
- The content of the section becomes part of each standard's
  behaviour. Changing a line is a template change that every opted-in
  repository syncs. The word bound keeps that cost small.
- Removability is unchanged. The sections are plain Markdown, and their
  markers are HTML comments that render invisibly. `/spec` is an
  ordinary Claude Code command. "Removing VibeConform" gains no step.
- The workflow is guidance, not a gate. VibeConform does not check that
  a spec preceded a change, because no check could tell a "non-trivial"
  change from a small one. That limit is stated, not hidden.

## Alternatives considered

- **Generate the whole AGENTS.md.** This was rejected for the reason
  the original non-goal gives: a generic file that projects either
  ignore or fight.
- **A separate `AGENTS.vibe.md`, referenced from AGENTS.md.** This was
  rejected because AGENTS.md has no include that every harness honours.
  The reference itself would be partial ownership with an extra hop.
- **Detect plan mode with a hook and inject the workflow then.** This
  was rejected for now. The model already knows when it is in plan
  mode, and a hook would add a moving part on every prompt in order to
  repeat that.
- **Separate `sdd`, `docs-layout` and `agent-instructions` standards**,
  as the issue sketched. This was rejected because they are options of
  one standard. A `standards:` map would be a second selection mechanism
  beside the catalog.
