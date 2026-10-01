# Spec 0029: Line-Ending Policy and Managed Sections

Status: accepted. Tracks issue #45. Per ADR 0014. Revisits the
`.gitattributes` non-goal of spec 0011. Builds on spec 0026 (the
integration catalog, structured patch, pruning) and spec 0028 (`vibe
doctor`).

## Problem

- VibeConform writes and hashes managed files byte for byte. On a Windows
  clone with `core.autocrlf=true` and no `eol` attribute, git checks them
  out as CRLF, so every one of them is drift until someone pins
  `* text=auto eol=lf` by hand. `docs/usage.md` tells adopters to do that,
  and nothing checks whether they did.
- The production standards deliberately leave `.gitattributes` alone
  (spec 0011), so this mechanical property ends up as prose in AGENTS.md
  or in each agent's memory. Formatters, CI and agents then compensate
  for CRLF in their own ways.
- `.gitattributes` belongs to the project. It holds binary declarations,
  linguist settings and per-language exceptions. Whole-file ownership
  would make every such line drift.
- `ManagedSection` has been declared since ADR 0003 and is implemented
  nowhere. #42 needs it for AGENTS.md, so it has to be general and not a
  special case for `.gitattributes`.

## Scope

### 1. `policy:` in `vibe.yaml`

An optional map of repository policies. Each key takes one value:

```yaml
standard: prod-go
version: v1
policy:
  line_endings: lf
```

- **Absent means off.** `policy:` absent, or `line_endings` absent,
  selects no line-ending policy. Opting in is explicit, and a manifest
  without `policy:` resolves byte for byte as before.
- `line_endings` accepts exactly `lf`. Any other value is an error
  naming the valid ones: `policy.line_endings (crlf): unknown value
  (valid: lf)`.
- Decoding is strict. An unknown key such as `policy.line_ending` or
  `policies:` is an error.
- `policy:` is separate from `integrations:`. Integrations are tools
  around the contract (editors, agents, intelligence); a policy is a
  property of the repository itself. `vibe init` writes neither.

### 2. One option catalog behind both keys

The integration catalog of spec 0026 becomes a general option catalog
(ADR 0014 §1). Each entry has:

- a group: an integration category (`editors`, `agents`, `intelligence`)
  or a policy key (`line_endings`);
- a name: `vscode`, `claude`, `lf`, …;
- a module, a default, and `Requires` and `Excludes`.

A group's cardinality says how `vibe.yaml` selects from it:

- An integration category is a list, as in spec 0026.
- A policy key is a scalar, so at most one value is selected.

Selection, resolution order (core modules, then options in catalog
order), validation and pruning (spec 0026 §7) work the same way for
both. All four standards register one policy entry:

| Group | Name | Default | Resource |
|---|---|---|---|
| `line_endings` | `lf` | off | the `line-endings` section of `.gitattributes` (§4) |

The refactor changes no behaviour. Every existing manifest resolves the
same resources in the same order, and the existing `integrations:`
errors keep their exact text.

### 3. Managed sections

A managed-section resource is identified by its path **and** a section
ID, for example `Resource{Path: ".gitattributes", Ownership:
ManagedSection, SectionID: "line-endings", Content: …}`. One file may
hold several sections with different IDs. Each is planned, reported,
recorded and pruned on its own. A section ID matches
`^[a-z][a-z0-9-]*$`.

**Markers.** A section is the lines between a begin marker and an end
marker, each on a line of its own, written in the file's comment syntax.
The resource declares the syntax:

| Syntax | Begin | End |
|---|---|---|
| `#` (gitattributes, YAML, TOML, shell) | `# vibeconform:begin <id>` | `# vibeconform:end <id>` |
| HTML (Markdown) | `<!-- vibeconform:begin <id> -->` | `<!-- vibeconform:end <id> -->` |

Markers match when they are byte-exact after trimming trailing spaces,
tabs and a trailing `\r`. Per ADR 0003, a marker only says *where* the
section is. Whether VibeConform owns it comes from the module that
resolved the resource. A marker in a file that no selected or deselected
option produces is ordinary text.

**Hashing.** The section's content is the bytes strictly between the
two marker lines. The marker lines are excluded, and the bytes are taken
exactly as on disk (§6). That content is decided three-way by
`reconcile.Decide`, with the recorded hash (P), the current content (C)
and the resolved content (T), using the existing decision table. Bytes
outside the section are never read for the decision and never written.

**Malformed markers are a conflict for that section.** That means:

- one marker of the pair without the other;
- an end marker before its begin marker;
- the same ID appearing twice;
- two sections that overlap or nest.

A file with no marker for an ID simply has no such section.

| File and section | Decision | `sync` writes |
|---|---|---|
| file absent | `Create` | the file, containing only the section; recorded `created` |
| file present, no section | `Create` | the section inserted at the resource's placement (below) |
| section present, C == T | `NoChange` | nothing |
| C != T, C == P | `OutOfDate` | the section's content, in place |
| C != T, T == P | `LocalDrift` | the section's content, in place |
| C != T, P absent | `Conflict` | nothing; the section exists but was never recorded, and its content differs |
| otherwise, or markers malformed | `Conflict` | nothing |

**Placement.** The resource says where a new section goes in an
existing file: at the top (the default) or at the bottom. The block is
the begin marker, the content and the end marker. Inserting at the top
writes the block, then one empty line, then the file's existing bytes
unchanged. Inserting at the bottom adds a newline if the file doesn't
end with one, then an empty line, then the block. Once a section exists
VibeConform never moves it. A user may move it, and that is not drift.

**Removal.** Pruning a recorded section (deselection, §5) removes the
block and the one empty line inserted with it, if that line is still
there. If removal leaves a `created` file empty, the file is deleted.

**Diff and audit.** Lines name the path and the section, for example
`.gitattributes (section line-endings): would add`. Every exit code
follows spec 0019, unchanged.

### 4. The line-ending policy

`policy: {line_endings: lf}` resolves a single managed section:

```gitattributes
# vibeconform:begin line-endings
# Managed by VibeConform: policy.line_endings in vibe.yaml.
* text=auto eol=lf
# vibeconform:end line-endings
```

- Syntax `#`, placement top. In the file git reads, every rule the user
  writes below the section comes later, and for each attribute git
  applies the last line that matches the path. So a narrower user
  exception wins for the paths it names, and the policy covers
  everything else.
- The section's path is `.gitattributes` at the repository root, for all
  four standards.

**Conflicting rules.** Besides the section itself, the module checks
the user's lines in the same file. A rule outside the section whose
pattern is global (`*` or `**`) and that changes `text`, `eol` or
legacy `crlf` contradicts the policy for every path. Examples:
`* eol=crlf`, `* -text`, `* binary`, `* text` (no longer `auto`) and
`* -eol`.

| User rule | Where | Result |
|---|---|---|
| `*.bat eol=crlf`, `*.png binary`, `docs/** -text` | below the section | legitimate; reported as nothing |
| `* eol=crlf`, `* -text`, `* binary` | below the section | `Conflict`, naming the line number and the rule |
| `* text=auto eol=lf` (same values) | anywhere | not a conflict |
| any rule setting `text`/`eol`/`crlf` | above the section | warning: the policy overrides it for the paths it names |

A conflict is reported by `diff`, `audit` (exit 2) and `sync`.
`sync` rewrites neither the user's rule nor the section, and exits
non-zero. Macro attributes (`[attr]`) a user defines and applies to `*`
are not expanded. That limit is documented and not detected.

The check reads `.gitattributes` only. Nested `.gitattributes` files,
`.git/info/attributes` and `core.attributesFile` can also override the
policy. `vibe doctor` sees their effect (§7), and the file check does
not.

### 5. Deselection

Removing `line_endings` from `vibe.yaml` prunes the section through the
same rules as spec 0026 §7. A section is a prune candidate only when
state records it.

| Section on disk | Decision | `sync` |
|---|---|---|
| absent (both markers gone) | `Forget` | drops the state entry |
| content equals recorded | `Remove` | removes the block (§3), deleting a `created` file left empty |
| content differs, or markers malformed | `RemoveConflict` | keeps it; non-zero exit |

User rules are never touched. A repository that deselects the policy
keeps a working `.gitattributes`, or none if VibeConform created it and
nothing else was ever added.

### 6. Line endings of managed content

- Content is written as resolved, with LF and no translation. This is
  unchanged from spec 0008. A section inserted into a file whose other
  lines use CRLF still writes LF, and the rest of the file keeps its
  bytes.
- Hashes are taken over raw bytes, as for generated files. The policy is
  the mechanism that makes those bytes the same on every checkout, so
  the hash does not normalize them.
- Under the policy, a checkout on Windows (any `core.autocrlf`) and one
  on Linux produce byte-identical managed files and identical
  `.vibe/state.yaml` hashes. A golden-hash test of a synced fixture runs
  on both CI legs.
- Converting files already committed with CRLF is the adopter's step and
  is not automatic: `git add --renormalize .` after the first sync.
  `docs/usage.md` documents it, and doctor reports whether it's still
  needed (§7).

### 7. Doctor reports policy health

The `line endings` check of spec 0028 gains the policy:

| Situation | Result |
|---|---|
| policy selected, `git check-attr eol` of a managed file is `lf`, no tracked text file stored as CRLF | `PASS policy line_endings: lf` |
| policy selected, N tracked text files stored as CRLF in the index (`git ls-files --eol`, `i/crlf`) | `WARN`, naming N and `git add --renormalize .` |
| policy selected, but the effective `eol` of a managed file isn't `lf` (an override elsewhere, §4) | `FAIL`, naming the effective value |
| policy not selected | unchanged from spec 0028, plus the hint `policy.line_endings is not selected` |

Doctor stays read-only.

### 8. State schema 4

Every entry becomes a structured record. Its identity is `path` plus,
for a section, `section_id`, never a combined string:

```yaml
schema: 4
resources:
  - path: .gitattributes
    section_id: line-endings
    ownership: managed-section
    created: true
    sha256: 3b7a...
  - path: .vscode/tasks.json
    ownership: structured-patch
    created: true
    elements:
      tasks/task fmt:
        sha256: 91ab...
  - path: Taskfile.yml
    ownership: generated
    sha256: 5c1f...
```

- Entries are sorted by `path`, then `section_id`, so the file is
  deterministic.
- `ownership` is recorded for every entry. `section_id` is present only
  for managed sections. The (`path`, `section_id`) pair is unique.
- **Migration.** `state.Load` still reads schemas 1 to 3, which use the
  `resources:` map keyed by path. It converts each entry: one with
  `elements` becomes `structured-patch`, and every other entry becomes
  `generated`. The next `sync` writes schema 4. A repository with no
  policy changes only the state file's layout, plus the recorded
  `vibe_version` as on every sync.
- A `vibe` older than this spec fails loudly on a schema 4 file, at
  `load state` and with exit 1. It never misreads it. `task audit`
  already installs the recorded `vibe_version` (spec 0022).

### 9. Examples and self-hosting

| Repository | `policy:` | Shows |
|---|---|---|
| this repository | `line_endings: lf` | the hand-written `* text=auto eol=lf` becomes the managed section |
| `examples/typescript` | `line_endings: lf` | a nested `.gitattributes` with one user rule (`*.png binary`) below the section, proving user lines survive |
| `examples/python`, `examples/monorepo` | absent | opt-in: nothing changes |

### Documentation

- `docs/usage.md`:
  - a new "Line-ending policy" section;
  - the line-ending note at :412 points to it;
  - the "not managed" entry for `.gitattributes` (:567) and the
    hand-maintained list (:1408) are updated;
  - "`.vibe/state.yaml`" describes schema 4;
  - "Removing VibeConform" notes that deleting the markers leaves the
    rules working.
- The new section covers:
  - **`core.autocrlf`:** the `eol` attribute overrides `core.autocrlf`
    and `core.eol` for every path it covers, so no per-machine git
    setting is needed or changed.
  - **Formatters:** `prod-ts`'s Prettier already sets `endOfLine: lf`,
    gofmt writes LF, and ruff keeps a file's line endings.
  - **Editors:** an editor that writes CRLF is normalized by git on
    commit, and `.editorconfig` remains the project's own.
  - **Existing files:** the `--renormalize` step.
- `docs/architecture/overview.md`: the option catalog, managed sections,
  and state schema 4.
- `README.md`: one line.
- Spec 0011's non-goal gets a pointer to this spec.

## Behavior

- Without `policy:`, every resource resolves and syncs byte for byte as
  before. Only the state file's layout changes (§8).
- `task verify`, lefthook and every workflow stay `vibe`-free. The
  section is plain `.gitattributes` text that git reads with VibeConform
  removed (principle 3).
- No AGENTS.md text is needed to keep line endings LF. Git enforces it,
  and `audit` and doctor report it (principle 1).
- Section planning, splicing and removal use slash paths and byte-exact
  writes. They behave the same on Windows and Linux.

## Explicit non-goals

- **No `crlf` or `native` policy value.** One value, `lf`. The key is a
  scalar, so adding a value later is not a schema change.
- **No user or global git configuration.** `core.autocrlf`, `core.eol`
  and `core.attributesFile` are reported, never changed.
- **No automatic renormalization** of committed files, and no conversion
  of binary or mixed-EOL history.
- **No `.editorconfig`** and no editor settings. Formatters are not
  replaced.
- **No nested `.gitattributes` analysis** beyond the root file. Doctor
  observes the effective attributes.
- **No section adoption.** An existing hand-written `* text=auto eol=lf`
  is left as user content next to the new section. The user removes the
  duplicate, as plan 0029 does for this repository.
- **No AGENTS.md section.** That is spec 0030 (#42), on this primitive.

## Design notes

- **Why a section rather than a structured patch over lines.**
  `.gitattributes` has no stable identity per line that a user would
  recognize. Patterns repeat, and order is semantic. A marked block is
  what a human reading the file sees as "VibeConform's part", and the
  same primitive works for Markdown (#42).
- **Why the top of the file.** Git applies the last matching line. At
  the top, the policy is the floor and every user rule is an exception
  layered over it, with no need to edit anything the user wrote.
- **Why only global contradictions conflict.** A narrower rule is how a
  repository says "except these files", which is exactly what the policy
  should allow. A global rule after the section silently disables the
  policy for every path, which is the failure the policy exists to
  surface.
- **Why rules above the section only warn.** They are overridden, not
  contradicting. The policy holds, but the user's intent probably does
  not, and a warning says so without failing an otherwise conformant
  repository.
- **Why a malformed marker is a conflict, not a re-create.** Guessing
  where a half-deleted section ends could delete user lines. Stopping is
  the only safe answer (principle 1: fail loudly).
- **Why raw-byte hashing.** It is the same rule as generated files. The
  policy fixes the bytes at checkout. Normalizing in the hash would hide
  exactly the CRLF checkouts this spec makes visible.
- **Why schema 4 is a list.** A section's identity is two fields. A map
  keyed by `path` cannot hold two sections of one file without encoding
  both fields into one key, and a structured record leaves room for
  later fields such as the marker syntax or a section schema version.
- **Why `policy:` is not an integration category.** It is a property of
  the repository, not a tool someone chose. Reusing the catalog
  internally keeps one selection, validation and prune path, without
  making the manifest say something untrue.

## Follow-on work

- Spec 0030 (#42): `development.workflow` and the managed AGENTS.md
  section, as a second user of the option catalog and of managed
  sections.
- Reading nested `.gitattributes` and `core.attributesFile` in the
  conflict check, if doctor's effective-attribute report proves too
  late.
- A `crlf` value or a per-path policy, if a standard needs one.
