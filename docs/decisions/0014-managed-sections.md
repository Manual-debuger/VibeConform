# ADR 0014: Managed sections, repository policies, and structured state

## Status

Accepted. Implemented per `docs/specs/0029-text-policy.md`.

## Context

ADR 0003 declared four ownership kinds. ADR 0013 implemented the second,
`StructuredPatch`, for JSON arrays. Two open issues now need the third,
`ManagedSection`:

- **#45, line endings.** It needs VibeConform's own rules in
  `.gitattributes`, a file that is otherwise the project's.
- **#42, a thin AGENTS.md contract.** It needs one section of a
  human-written Markdown file.

If either built its own mechanism, we would get two marker formats, two
state shapes and two prune paths.

Both issues also add things a repository selects that are not tools: a
line-ending policy and a development workflow. ADR 0013's catalog
already selects, validates and prunes optional modules, but its manifest
surface, `integrations:`, means "editors, agents, intelligence". Putting
a policy there would make `vibe.yaml` say something untrue.

Finally, `.vibe/state.yaml` is a map keyed by path. One file holding
two independently owned sections cannot be recorded without encoding two
identities into one key, which schema 3 already does once for elements
(`tasks/task fmt`).

## Decision

1. **One option catalog, several manifest keys.** The integration
   catalog generalizes into an option catalog. Each entry has a group, a
   name, a module, a default, and `Requires`/`Excludes`. A group is
   either an integration category (selected as a list under
   `integrations:`, unchanged) or a repository policy key (a scalar under
   its own top-level map, e.g. `policy: {line_endings: lf}`). Selection,
   resolution order, validation and pruning are one code path for both.
   Spec 0030's `development:` is the next group.

2. **`ManagedSection` is a general primitive keyed by (path, section
   ID).** A resource declares the following:
   - its path and section ID;
   - the comment syntax of its markers (`#` or HTML);
   - where a new section is placed: the top of the file or the bottom.

   The markers are `vibeconform:begin <id>` and `vibeconform:end <id>`.
   Only the bytes between them are hashed and reconciled three-way, with
   the existing decision table. Bytes outside them are never written.
   Malformed markers are a conflict and are never repaired by guessing.
   As ADR 0003 requires, markers only scope a section. Ownership comes
   from the module that resolved it.

3. **A module may veto a section with a check over the whole file.** A
   section can be intact and still be defeated by text around it (`*
   eol=crlf` after the line-ending rules). The module that owns the
   section may report such contradictions as a conflict. The check reads
   the file and never edits it. Text-region parsing stays generic, and
   the knowledge of what counts as a contradiction stays in the module.

4. **State schema 4 is a list of structured records.** Each entry has
   `path`, optional `section_id`, `ownership`, and the fields its
   ownership needs: `sha256`, `created`, `elements`. Entries are sorted
   by (`path`, `section_id`), and that pair is unique. Schemas 1–3 load
   and convert. The next sync writes schema 4.

## Consequences

- #42 adds an AGENTS.md section and a `development:` group with no new
  ownership code and no new state shape.
- Every adopter's `.vibe/state.yaml` is rewritten once, as a list, on
  its first sync after upgrading. The diff is large and mechanical. An
  older `vibe` fails to load schema 4 with exit 1 rather than misreading
  it. `task audit` already pins the recorded `vibe_version`.
- `.gitattributes` and AGENTS.md become partly managed. Spec 0011 and
  `docs/usage.md`'s "not managed" list are revised, and both files stay
  project-owned outside their markers.
- Structured-patch element keys keep the `<array>/<id>` form inside
  `elements`. Making them structured too is possible later, and is not
  needed by any current consumer.
- A managed section whose markers a user deletes and rewrites by hand is
  indistinguishable from an untouched one. That is the accepted cost of
  text markers, and it is bounded: the content between them is still
  hashed.

## Alternatives considered

- **A special case for `.gitattributes`.** That would mean owning the
  lines matching the rule. It was rejected because pattern lines repeat,
  order is meaningful, and #42 would need a second mechanism anyway.
- **`policy` as an integration category.** That is
  `integrations: {text: [lf]}`. It was rejected because it is a list for
  a single-valued property, and because it calls a repository property a
  tool.
- **A schema-4 map keyed `path#section`.** It was rejected because it
  repeats the encoded-identity problem, and a record leaves room for
  later fields.
