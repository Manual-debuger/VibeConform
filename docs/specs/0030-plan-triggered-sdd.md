# Spec 0030: Development Workflow, AGENTS.md Section and Docs Layout

Status: accepted and implemented. Tracks issue #42. Per ADR 0015. Builds on spec 0029 and
ADR 0014: the option catalog, managed sections and state schema 4.
Revises the `AGENTS.md`/`CLAUDE.md` non-goal in `docs/usage.md`.
§5's `/spec` command is replaced by a skill in spec 0031. The sections'
bytes gained blank lines, which make them stable under Prettier (plan
0032); the examples below show the text without them.

## Problem

- Agents in an adopting repository get no workflow guidance from
  VibeConform. Each project writes its own AGENTS.md prose about specs,
  plans and verification, or none. The prose is long and generic, and it
  drifts from the tools it describes.
- Strict spec-driven development is too heavy for a small fix. Skipping
  specs on a large or ambiguous change removes the one cheap human
  checkpoint. Harnesses already have a planning mode, such as Claude
  Code's plan mode, which is a natural trigger for writing a spec first.
  Nothing connects that mode to a spec.
- A repository has no agreed place for specs, architecture docs and ADRs,
  so an instruction like "read the relevant spec" points nowhere.
- `docs/usage.md` says that generating AGENTS.md would produce "exactly
  the fabricated, ignored-by-everyone instruction file this project
  argues against". That still holds for a whole generated file. A small
  section derived from what `vibe.yaml` selects is different. ADR 0015
  records the revision.

## Scope

### 1. `development:` in `vibe.yaml`

An optional map with two keys. Each key takes one value:

```yaml
standard: prod-go
version: v1
development:
  workflow: plan-triggered-sdd   # direct | plan-triggered-sdd | always-sdd
  docs_layout: standard
```

- **Absent means off**, as with `policy:`. A manifest without
  `development:` resolves byte for byte as before. `vibe init` writes
  no `development:` map.
- `workflow` accepts `direct`, `plan-triggered-sdd` and `always-sdd`.
  Being a scalar makes the three mutually exclusive. YAML cannot give
  `workflow` two values, so no `Excludes` entry is needed.
- `docs_layout` accepts exactly `standard`.
- Decoding is strict. Errors follow spec 0029's form, with the map's
  own name:
  - `development.workflow (sdd): unknown value (valid: direct, plan-triggered-sdd, always-sdd)`
  - `development.docs_layout (full): unknown value (valid: standard)`
  - an unknown key such as `development.workflows`
- `development:` is not `policy:`. A policy is a property that git or a
  tool enforces. The development map says how people and agents work
  here. Both are scalar groups in the one option catalog (ADR 0014 §1).
  Only the manifest map they appear under differs.

### 2. Catalog entries

All four standards register these entries after `line_endings`. Each is
off by default.

| Group | Name | Resources |
|---|---|---|
| `workflow` | `direct`, `plan-triggered-sdd`, `always-sdd` | the `workflow` section of `AGENTS.md` (§3); with `claude` selected, also `/spec` and a `CLAUDE.md` import (§5) |
| `docs_layout` | `standard` | the `docs` section of `docs/README.md`, and the `specs` section of `docs/specs/README.md` (§4) |

- A scalar group records which manifest map it belongs to (`policy` or
  `development`). `Label()` and error messages use that map's name:
  `development.workflow`, not `policy.workflow`.
- Policy keys stay unique across maps. A test enforces that, so
  `module.Context.Policies` stays a flat map keyed by policy key.
- Switching `workflow` from one value to another updates the section in
  place, three-way, as an ordinary update. It is not a removal and a new
  creation.

### 3. The `workflow` section of AGENTS.md

A `ManagedSection` resource with these properties:

- path `AGENTS.md`
- section ID `workflow`
- HTML-comment markers
- placement at the bottom, so the project's title and introduction come
  first

When VibeConform creates AGENTS.md, it records the file as created.
Codex reads AGENTS.md natively, and Claude Code reaches it through §5.

The section is derived and routing-only:

- **Every line is derived from the selection.** The standard, `workflow`,
  `docs_layout` and whether `claude` is selected decide each line. No
  line is generic advice that a project would need to remove.
- **It points; it does not explain.** It names where knowledge lives and
  which task is the gate. It does not repeat a tool recipe, a formatter
  command, a line-ending rule, CI internals or the architecture. Under
  principle 1, a rule that a tool enforces appears only as the name of
  that tool.
- **Size.** The section must stay within **300 words** (whitespace-
  separated, markers excluded) for every combination of selections, and
  a test enforces that. The design target, which no test checks, is
  about 20 to 35 lines and 150 to 250 words.

The section for `plan-triggered-sdd` with `docs_layout` and `claude`
selected:

```markdown
<!-- vibeconform:begin workflow -->
## Repository workflow

Managed by VibeConform from `development:` in `vibe.yaml`. This section
routes; the documents and tasks it names hold the detail.

Knowledge:
- Specs (what must be true) live in `docs/specs/`, architecture (how it
  works now) in `docs/architecture/`, decisions (ADRs) in
  `docs/decisions/`. Read the relevant ones before a non-trivial change.
- If an approved spec, an ADR and the code disagree, say so. Do not
  pick one silently.

Workflow: plan-triggered lightweight SDD.
- Normal mode: implement, then verify. Respect any spec that applies.
- Planning context (the harness's plan mode, or `/spec`): list the
  constraints that apply and the assumptions you have not verified, then
  write a lightweight spec with acceptance criteria. Plan only after that.
- The spec says WHAT must be true; the plan says HOW to change the
  repository. Keep them apart. Reuse an approved spec when one exists.
- In a read-only plan mode, put the spec in the plan. Once it is
  approved, write it to `docs/specs/` first.
- Do not implement until the user approves.

Verification:
- `task verify:fast` while working; `task verify` before declaring
  done. Do not weaken a test, lint or type check to make a change pass.
- Finish with a ledger, one line per check: PASS, FAIL or UNVERIFIED.
  Unit tests, CI and a real integration are separate lines.
<!-- vibeconform:end workflow -->
```

How each selection changes the section:

| Selection | Change |
|---|---|
| `docs_layout` not selected | The first Knowledge bullet becomes "Read the specs, architecture docs and ADRs that apply before a non-trivial change." Specs are written where the project keeps them. |
| `claude` not selected | "or `/spec`" becomes "or a request for a spec". |
| `direct` | The Workflow block becomes "Workflow: direct. Implement, then verify. Respect any spec that applies; a planning context (…) writes one when asked." The rest of it is dropped. |
| `always-sdd` | The heading line becomes "Workflow: spec-driven." The normal-mode bullet becomes "A non-trivial behavioural change needs an approved spec with acceptance criteria before it is planned, in any mode. A small fix may go straight to implement and verify." |

The exact bytes of each variant are pinned by golden tests. The plan
fixes how the variants are composed.

### 4. The docs layout

`docs_layout: standard` names three canonical directories and seeds the
two files that a directory needs in order to exist in git:

- **`docs/README.md`**, section `docs`, HTML markers, placed at the
  bottom. It is a short "## Layout" list linking `specs/`,
  `architecture/` and `decisions/`, one line each saying what belongs
  there.
- **`docs/specs/README.md`**, section `specs`, HTML markers, placed at
  the top. It covers three things:
  - naming: one file per feature, `docs/specs/<feature>.md`, and a
    numeric prefix is fine;
  - a Status line: draft, accepted, implemented or superseded;
  - the lightweight spec template (§5's template, the same text from one
    Go constant).

No other files are created. `docs/architecture/` and `docs/decisions/`
appear when the project writes its first document there. There are no
`active/` and `completed/` subdirectories. A spec is a living document
whose Status line records where it stands.

`development/` and `operations/` sections are deferred. So is adopting a
project's existing docs layout instead of this one.

### 5. Claude Code adapter

With `workflow` selected **and** `claude` selected, `claude-config`
resolves two more resources:

- **`.claude/commands/spec.md`**, a `Generated` file. Claude Code
  documents `.claude/commands/` as the home of project slash commands,
  invoked as `/spec <feature>`. It has a `description` and an
  `argument-hint` in its front matter. The body:
  1. says that this is a planning context, and that it changes no code,
     configuration or tests;
  2. reads the relevant specs, architecture docs and ADRs, which AGENTS.md
     routes to;
  3. lists constraints and unverified assumptions, and asks about the
     material ones;
  4. reuses an approved spec, or writes one from the template under the
     project's spec location (`docs/specs/` when AGENTS.md names none);
  5. proposes an implementation plan separately, in the conversation;
  6. stops and waits for approval.

  The template:

  ```markdown
  # Feature: <name>

  Status: draft

  ## Problem
  ## Constraints
  ## Assumptions
  ## Desired Behavior
  ## Non-goals
  ## Acceptance Criteria
  - [ ] ...
  ```

  Each heading has a one-line prompt under it. The prompts are fixed in
  the plan.
- **`CLAUDE.md`**, section `agents`, HTML markers, placed at the top,
  containing `@AGENTS.md`. Claude Code loads CLAUDE.md, not AGENTS.md.
  This import is the documented way to make it read AGENTS.md. A
  section, not the whole file, so a project's own CLAUDE.md content
  stays its own. If `@AGENTS.md` already appears outside the section,
  the module's check (ADR 0014 §3) reports a **warning**: the import is
  duplicated, so remove your own line. It is not a conflict.

The command is generated for all three workflow values. Under `direct`,
`/spec` is how one asks for a spec.

**No hooks.** Claude Code's plan mode is a permission mode, and the
model is told when it is in it. Hook input carries the mode too, so a
hook could detect it. This spec adds no such hook: it would only repeat
what the model already knows. The AGENTS.md section defines "planning
context" as "the harness's plan mode, or `/spec`". An agent in plan mode
recognizes it from its own context, and `/spec` is the explicit
fallback. Other harnesses get the AGENTS.md section and no adapter.

### 6. Deselection and removal

- Deselecting `workflow` removes the AGENTS.md section, `/spec` and the
  CLAUDE.md section. Deselecting `claude` removes the last two. Each
  goes through spec 0029's pruning, unchanged:
  - text outside the markers is kept;
  - a file VibeConform created is deleted once its section is gone and
    nothing else is left in it;
  - a section edited since the last sync is a conflict and is kept.
- Everything generated is plain Markdown that needs no `vibe`. After
  VibeConform is removed, the sections read as ordinary prose, and the
  markers are HTML comments that render invisibly. `/spec` keeps working
  as an ordinary Claude Code command. "Removing VibeConform" gains no
  new step. Its note about markers names these files too.

### 7. Examples and self-hosting

- **`examples/python`** selects `workflow: plan-triggered-sdd` and
  `docs_layout: standard`. Its claude integration stays at the default,
  so `TestExamplesAreConformant` covers the AGENTS.md, CLAUDE.md, `/spec`
  and docs resources.
- **This repository** selects `workflow: always-sdd` and
  `docs_layout: standard`. Its AGENTS.md keeps the project's own rules
  outside the markers:
  - managed files and `vibe audit`;
  - recording dependencies in an ADR;
  - the guard;
  - commit attribution.

  The prose that now overlaps with the section moves into it: "routing
  surface", read the docs before non-trivial work, spec before plan, the
  `task verify` gate, and not weakening tests. CLAUDE.md becomes the
  managed section alone. `docs/README.md` is created with a project
  paragraph about `docs/plans/` above the section, which this repository
  commits and the standard does not require.

### Documentation

- **`docs/usage.md`:**
  - a "Development workflow" section after "Line-ending policy";
  - the `AGENTS.md and CLAUDE.md` not-managed bullet is rewritten: both
    stay yours, apart from the sections a selected `development.workflow`
    adds;
  - "What vibe.yaml means today" mentions `development:`;
  - the hand-maintained list and the removal section's marker note are
    updated.
- **`docs/architecture/overview.md`:** the new catalog groups, and the
  new module's place in the layout.
- **`docs/architecture/principles.md`:** principle 1 gains one sentence.
  A managed AGENTS.md section is held to the same test, routing and
  naming tools rather than restating them.
- **README:** the status block.

## Behavior

| Situation | Result |
|---|---|
| No `development:` | Resources and output byte for byte as before |
| `workflow: plan-triggered-sdd`, no AGENTS.md | AGENTS.md is created, holding the section; recorded `created: true` |
| AGENTS.md with project prose | Section appended after it; prose untouched |
| Section edited by hand | `audit` reports drifted (exit 2); `sync` restores it, as for any managed resource |
| Section edited by hand, and the standard's text changed too | `audit` reports a conflict (exit 2); `sync` refuses (exit 1) and keeps the edit |
| `workflow` changed from `direct` to `always-sdd` | Section updated in place |
| `development:` removed | Section, `/spec` and the CLAUDE.md section pruned; created files that are now empty are deleted |
| `claude` deselected, `workflow` kept | `/spec` and the CLAUDE.md section are pruned; the AGENTS.md section drops its `/spec` mention |
| CLAUDE.md already contains `@AGENTS.md` | Warning; the section is still written |
| `development.workflow: sdd` | Exit 1 with the §1 error |

## Explicit non-goals

- **Generating a whole AGENTS.md or CLAUDE.md.** Both stay the
  project's, outside one section each.
- **Detecting plan mode** through hooks or any undocumented signal.
- **Enforcing the workflow.** Nothing checks that a spec exists before
  code changes. The section is guidance for judgment, and principle 1's
  test applies: there is no mechanical check to point at.
- **Component-level AGENTS.md** for `prod-mono` components, and how it
  would inherit from the root.
- **`docs/development/`, `docs/operations/`**, and adopting or migrating
  an existing docs layout.
- **Adapters for other harnesses** (Codex, OpenCode).
- **Committing implementation plans.** `docs/plans/` stays this
  repository's own practice.
- **A `standards:` map with `sdd`, `docs-layout` and
  `agent-instructions` entries**, as the issue sketched. The catalog
  already composes options; ADR 0015 explains this.

## Design notes

- **Why a section and not `AGENTS.vibe.md`.** AGENTS.md has no include
  syntax that every harness honours. A separate file would need prose in
  AGENTS.md pointing at it, which is the same partial ownership with an
  extra hop.
- **Why the section names `task verify:fast` and `task verify`.** Every
  standard generates both. They are the canonical interface, so naming
  them is pointing at a tool rather than restating it.
- **Why the ledger is prose.** It is an output convention for the agent.
  The gates are mechanical and already exist. The ledger stops an agent
  from reporting "tests pass" as "it works".
- **Why `/spec` lives in `claude-config`.** ADR 0011 gives each agent
  runtime its own module. The adapter is Claude-specific, while the
  workflow module stays harness-agnostic.

## Follow-on work

- Component-level AGENTS.md sections for `prod-mono`.
- The `development/` and `operations/` docs sections, and adopting an
  existing docs layout.
- A Codex adapter, when Codex documents project commands or a planning
  mode.
