# Plan 0030: Development workflow, AGENTS.md section and docs layout

Implements `docs/specs/0030-plan-triggered-sdd.md` and ADR 0015. Tracks
issue #42. Spec accepted at review (2026-10-01, `a54730c`); plan accepted at review (2026-10-01).

Branch: `feat/plan-triggered-sdd`, off `main` after PR #47. One pull
request, the last of three: #44 (spec 0028), #45 (spec 0029), then this.

## Resolved design questions

**How does a scalar group know its manifest map?** `Group` gains `Map`:

```go
type Group struct {
    Key    string // "editors", "line_endings", "workflow"
    Scalar bool
    Map    string // "policy" or "development"; empty for integrations
}
```

- `Label()` returns `Map + "." + Key` for a scalar group. The line-ending
  entry sets `Map: "policy"`, so its label and every #45 message stay
  byte for byte the same.
- A test asserts two things over every standard's catalog: each scalar
  group has a known `Map`, and scalar keys are unique across maps. That
  keeps `Selection.Policies` and `module.Context.Policies` flat maps keyed
  by policy key, so neither changes shape.

**How does the manifest grow?** `internal/manifest` gains a parallel
`Development` struct:

```go
type Development struct {
    Workflow   *string `yaml:"workflow,omitempty"`
    DocsLayout *string `yaml:"docs_layout,omitempty"`
}
```

- `PolicyKeys` and `(*Policy).Get` generalize into one ordered list of
  scalar keys, `ScalarKeys []ScalarKey{Map, Key}`, and
  `(*Manifest).Scalar(map, key) *string`.
- `Standard.Select`'s policy loop iterates `ScalarKeys`. Its error for an
  unknown value already has the spec's form once `where` uses
  `Map + "." + key`.
- The "offers no …" error names its noun by map: `policy` for the policy
  map, `setting` for the development map. No shipped standard can reach
  it, and a test does.
- Strict decoding covers `development:` with no extra code.

**Which module owns what?** There is a new package,
`internal/module/workflow`:

| Constructor | Module name | Resources |
|---|---|---|
| `New(mode)` | `development-workflow` | the `workflow` section of `AGENTS.md` |
| `NewDocsLayout()` | `docs-layout` | the `docs` section of `docs/README.md`, the `specs` section of `docs/specs/README.md` |

- The catalog registers `New("direct")`, `New("plan-triggered-sdd")` and
  `New("always-sdd")` under one group, then `NewDocsLayout()`. All are
  after `lf` and all off by default.
- The three workflow modules resolve the same (path, section ID). So
  switching mode is an ordinary three-way update. Pruning finds the
  section once, because `planPrunes` already de-duplicates section keys.
- `workflow.SpecTemplate` is an exported constant. Both `docs-layout` and
  `claude-config` use it, so `/spec` and `docs/specs/README.md` carry
  the same text. `claude` importing `workflow` follows existing
  module-to-module imports such as `tsrepotooling` and `githubmono`.

**How is the AGENTS.md section composed?** It is composed from Go
constants, not embedded templates, for the same CRLF reason as
`lineendings` (memory: CRLF / go:embed hazard). The order is:

1. header: `## Repository workflow` and the "Managed by VibeConform" lines;
2. knowledge: `docsLayout` or `noDocsLayout`;
3. workflow: one of `direct`, `planTriggered` or `alwaysSDD`;
4. verification.

- Within a workflow block, the planning-context phrase is a function of
  whether `/spec` exists: "the harness's plan mode, or `/spec`" or "…, or
  a request for a spec".
- "`/spec` exists" means that `claude` is in `Context.Integrations`. A nil
  `Integrations` (a module resolved in isolation) counts as selected,
  like `WantsAgentHooks`.
- `docs_layout` is read from `Context.Policies["docs_layout"]`.

**Which bytes are pinned, and how?**
- Each block's variants are inline expected strings in
  `workflow_test.go`: one header, two knowledge, three workflows times
  two phrasings, one verification. A composition test checks the order.
  Together that pins every variant without twelve copies.
- The 300-word bound is a separate test over all twelve combinations,
  using `strings.Fields` on the content, which excludes the markers. It
  also logs each combination's line and word counts, so a change that
  overshoots the 150–250 target shows up in `-v`.
- There is no `-update` flag; this repository has none.

**What does the `CLAUDE.md` check accept as a duplicate import?** A line
outside the section whose trimmed text is `@AGENTS.md` or `@./AGENTS.md`.
Fenced code is not special-cased. A false warning is cheap, and the check
never writes. `claude-config` implements `module.SectionChecker`, and its
`CheckSection` returns nothing for any path but `CLAUDE.md`.

**The fixed text of the spec template's prompts:**

| Heading | Prompt |
|---|---|
| Problem | What is wrong or missing, and for whom. |
| Constraints | Architecture, compatibility and product rules this must not break. |
| Assumptions | What this takes as true without evidence from the repository or the user. |
| Desired Behavior | What must be true when this is done, observable from outside. |
| Non-goals | What this deliberately leaves out. |
| Acceptance Criteria | Checkable statements; each is verified before the work is called done. |

## Repository impact

| Area | Change |
|---|---|
| `internal/manifest` | `Development`, `ScalarKeys`, `Scalar()`; `Policy.Get` folded in |
| `internal/standard` | `Group.Map`, `Label()`, `Select` loop over `ScalarKeys`, catalog entries |
| `internal/module/workflow` (new) | workflow and docs-layout modules, `SpecTemplate` |
| `internal/module/agents/claude` | `/spec`, the `CLAUDE.md` section, `CheckSection` |
| `internal/cli` | no production change expected. Sections, pruning and checkers are spec 0029's. New end-to-end tests. |
| `internal/doctor` | none |
| Examples | `examples/python` selects both options and gets five new managed resources |
| This repository | `vibe.yaml` gains `development:`; `AGENTS.md`, `CLAUDE.md` and `docs/README.md` gain sections; new `docs/specs/README.md` and `.claude/commands/spec.md` |
| Docs | usage, overview, principles, README status |

No new dependency. No state schema change, since schema 4 already
records sections.

## Order of work

**C3: `development.workflow` and the AGENTS.md section.**
- Manifest `Development`, `ScalarKeys`; `Group.Map`; `Label()`; `Select`.
- `workflow.New(mode)` and its content constants; three catalog entries.
- Tests:
  - manifest: decode, an unknown key, an absent map;
  - standard: unknown values with exact text, the noun by map, labels,
    unique keys, `lf` messages unchanged;
  - workflow: the pinned blocks, the composition, the word bound over
    all twelve combinations, the nil-Integrations default;
  - cli `workflow_test.go`:
    - an AGENTS.md is created and recorded as created;
    - project prose is kept and the section is appended after it;
    - a hand edit inside the section is a conflict;
    - switching `direct` to `always-sdd` is an update and no prune;
    - removing `development:` removes only the section, and deletes a
      file VibeConform created that is left empty;
    - no `development:` resolves byte for byte as before. Today's
      resources for each example manifest are compared with and without
      the code.

**C4: `development.docs_layout`.**
- `NewDocsLayout()`, the `SpecTemplate` constant, and the catalog entry.
- The AGENTS.md knowledge block reads `docs_layout`. Its test already
  exists from C3; C4 makes the docs-layout variant reachable end to end.
- Tests:
  - module: the exact bytes of both sections, and their placement;
  - cli: both files are created on a bare repository; an existing
    `docs/README.md` keeps its prose above the section; deselection
    prunes both sections; the AGENTS.md section switches its knowledge
    bullet when `docs_layout` is toggled.

**C5: the Claude adapter.**
- `claude-config` resolves `.claude/commands/spec.md` and the
  `CLAUDE.md` `agents` section when `Context.Policies["workflow"]` is
  set. It implements `CheckSection`.
- Tests:
  - module: neither resource appears without a workflow; the exact
    bytes of the command, including front matter and the template from
    `SpecTemplate`; the duplicate-import warning, and no warning for
    the section's own line;
  - cli:
    - deselecting `claude` prunes both resources and drops "`/spec`"
      from AGENTS.md;
    - deselecting `workflow` prunes all three;
    - a project CLAUDE.md that only says `@AGENTS.md` syncs with the
      warning.

**C6: docs, examples, self-hosting.**
- `docs/usage.md`, `overview.md`, `principles.md` and README, as listed
  in the spec's Documentation section.
- `examples/python/vibe.yaml` gets `development:`. Then
  `go install ./cmd/vibe` and `vibe sync`, committing the new files and
  the state.
- This repository:
  - `vibe.yaml` gets `development: {workflow: always-sdd, docs_layout: standard}`;
  - `vibe sync`;
  - `AGENTS.md`: move the overlapping prose into the section by
    deleting it outside, and keep the repository-specific rules;
  - `CLAUDE.md`: delete the hand-written `@AGENTS.md` line, which
    leaves the section alone, after the warning;
  - write a project paragraph about `docs/plans/` above the section in
    `docs/README.md`.

**Then:** `task verify`, `task audit`, the PR, CI green, and a CI-green
record in this plan.

## Checklist

- [ ] C3 workflow option, AGENTS.md section, tests
- [ ] C4 docs layout, tests
- [ ] C5 Claude `/spec` and CLAUDE.md import, tests
- [ ] C6 docs, `examples/python`, self-hosting
- [ ] `task verify` and `task audit` green locally
- [ ] Scratch-repo verification (below)
- [ ] PR open, CI green, recorded here

## Verification

Beyond the automated tests, these are checked in a scratch git
repository with a locally installed `vibe`:

1. An AGENTS.md with a title and a project paragraph. After selecting
   `plan-triggered-sdd`, `sync` appends the section. `audit` passes, and
   the project text is unchanged byte for byte.
2. An edit inside the section is reported by `audit` (exit 2) and
   refused by `sync` (exit 1).
3. Switching to `always-sdd` gives an update. Removing `development:`
   gives the section, `/spec` and the CLAUDE.md section removed, and the
   project text kept.
4. The section's line and word counts for every combination are
   reported, from the C3 test log.
5. `/spec` in a real Claude Code session, in the scratch repository.
   Interactively if possible; otherwise `claude -p "/spec add a --json flag"`.
   It writes only a spec under `docs/specs/`, proposes a plan, and stops
   without editing code. Whatever cannot be observed is recorded as
   UNVERIFIED, not assumed.

## Explicitly still deferred

From the spec's non-goals:
- component-level AGENTS.md for `prod-mono`;
- `docs/development/` and `docs/operations/`;
- adopting an existing docs layout;
- plan-mode hooks;
- adapters for harnesses other than Claude Code.
