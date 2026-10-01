# Plan 0029: Line-ending policy and managed sections

Implements `docs/specs/0029-text-policy.md` and ADR 0014. Tracks issue
#45. Spec accepted at review (2026-10-01, `c114e1c`); plan accepted (`cb14b4c`).

Branch: `feat/text-policy`, off `main` after PR #46. It is one pull
request, merged with a merge commit, and the second of three: #44 (spec
0028, merged), then this, then #42 (spec 0030).

## Resolved design questions

**What does the option catalog look like?** In `internal/standard`,
`Integration` becomes `Option`:

```go
type Option struct {
    Group    Group         // an integration category or a policy key
    Name     string        // "vscode", "claude", "lf"
    Module   module.Module
    Default  bool
    Requires []string
    Excludes []string
}

type Group struct {
    Key    string // "editors", "line_endings"
    Scalar bool   // a policy key: at most one value
}
```

- `Standard.Integrations` becomes `Standard.Options`.
- `Select` takes the whole manifest and returns a `Selection`: integration
  names in catalog order, plus policy values by key.
- `ModulesFor(Selection)` and `planPrunes` iterate options, not
  integrations.
- `module.Context.Integrations` keeps its meaning (integration names
  only), so `WantsAgentHooks` and every module that reads it are
  untouched. `Context` gains `Policies map[string]string`.
- Prune messages name the option by its manifest spelling. An
  integration stays `codex`, so today's text is unchanged byte for byte.
  A policy reads `policy.line_endings`.

**Does `.vibe/state.yaml`'s in-memory form change everywhere?** No.
`State.Resources map[string]ResourceState` stays for whole-file and
structured-patch entries, keyed by path, so `sync`, `plan` and their
tests keep their shape. State gains:

```go
Sections map[SectionKey]SectionState // SectionKey{Path, ID}
```

The list layout of schema 4 is produced by custom
`MarshalYAML`/`UnmarshalYAML` on `State`:

- **Writing:** one sorted list. `ownership` is derived when writing: an
  entry with `elements` is `structured-patch`, any other `Resources`
  entry is `generated`, and every `Sections` entry is `managed-section`.
- **Reading schema 4:** each record is dispatched by its `ownership`. An
  unknown ownership, a duplicate (`path`, `section_id`), or a
  `section_id` on a non-section entry is a load error.
- **Reading schema 1–3:** a `resources:` mapping is read through the old
  path unchanged.

The two shapes are told apart by the YAML node kind (mapping or
sequence), not only by the `schema:` number.

**How do several sections in one file apply without overwriting each
other?** Sections are planned per file, not per resource:

- After resolve and prune discovery, `buildPlan` groups every
  managed-section resource and every section prune candidate by path into
  one `sectionFile`. It parses the file once, decides each section, splices
  all edits into one result, and runs the owning modules' checks on it.
- Each `resourcePlan` and `prunePlan` points at its shared `sectionFile`
  and its own `sectionPlan`. This is exactly how `patchPlan` keeps
  elements apart.
- A conflict in any section holds the whole file, as in spec 0026 §5.
- `sync` writes a file once, the first time it reaches one of the
  file's sections.

**Where does the textual work live?** In a new `internal/textregion`
package, which imports nothing from `internal/` (like `jsonarray`):

- `Parse(data, syntax, ids)` returns, per ID, the span (begin line, inner
  bytes, end line) or why the markers are malformed.
- `Insert(top|bottom)`, `Replace` and `Remove` return new bytes.
  Everything outside the touched block is copied verbatim, and the
  file's CRLF lines stay CRLF.
- Marker matching follows spec §3: exact text after trimming trailing
  ` \t\r`.

**How does a module veto a section (ADR 0014 §3)?** Through a new
optional interface in `internal/module`:

```go
type SectionChecker interface {
    // CheckSection sees the file as it would be after sync: the bytes
    // before and after r's section. It returns conflicts (which hold
    // the file) and warnings (stderr only).
    CheckSection(r resource.Resource, before, after []byte) (conflicts, warnings []string)
}
```

The planner already knows which module produced each resource, so it
calls the module's own check. `textregion` stays generic.

**Which fields does a section resource carry?** `resource.Resource`
gains:

- `SectionID string`
- `Markers resource.MarkerSyntax`: `HashComment` or `HTMLComment`
- `Placement resource.Placement`: `Top` (the zero value) or `Bottom`

`planResource` rejects a `ManagedSection` without a valid `SectionID`, a
`SectionID` on any other ownership, and two resources with the same
(path, ID).

**How does the line-endings check parse `.gitattributes`?**

- Line by line. Blank lines, `#` comments and `[attr]` macro definitions
  are skipped.
- The pattern is the first field. A quoted pattern is unquoted per
  gitattributes(5).
- A global pattern is `*` or `**`, with or without a leading `/`.
- An attribute is contradicting if any of these holds:
  - `text` is set to anything except `auto`: `text`, `-text` or `!text`;
  - `eol` is anything except `lf`, including `-eol` and `!eol`;
  - `crlf` appears in any form;
  - the `binary` macro is used.
- Lines after the section produce conflicts, each naming its line
  number. Lines before it produce warnings.

**Where does the policy module live?** In
`internal/module/policy/lineendings`, named `line-endings-policy`. It
produces one resource (spec §4) and implements `SectionChecker`. It has
no template file: the four content lines are a Go constant, so
`go:embed` and CRLF working copies cannot affect them.

**What does doctor need?** `doctor.LineEndings` gains a `policy bool`
argument and one probe: `git ls-files --eol`, counting `i/crlf` entries.
The CLI passes whether `line_endings` is selected. The rows follow spec
§7. With the policy off, the output keeps spec 0028's lines and adds the
hint to the detail.

## Repository impact

| Area | Change |
|---|---|
| `internal/standard/{integrations,standard}.go` | `Option`, `Group`, `Selection`; `catalog()` returns options; the policy entry (C6) |
| `internal/standard/*_test.go` | renamed types; selection unchanged for every existing manifest; scalar-group validation |
| `internal/manifest/manifest.go` | `Policy *Policy` with `LineEndings *string`; strict decoding; value validated against the catalog (C6) |
| `internal/module/module.go` | `Context.Policies`; `SectionChecker` |
| `internal/resource/resource.go` | `SectionID`, `MarkerSyntax`, `Placement` |
| `internal/textregion/` | new: parse, insert, replace, remove; tests |
| `internal/state/state.go` | schema 4, `Sections`, custom YAML, migration; tests |
| `internal/cli/plan.go`, new `section.go` | per-file section planning, section prunes |
| `internal/cli/{sync,diff,audit,prune}.go` | section lines, apply once per file, state entries |
| `internal/cli/*_test.go` | sections through a fake module; two sections in one file; prune; schema 4 round trip |
| `internal/module/policy/lineendings/` | new module and tests |
| `internal/doctor/environment.go`, `internal/cli/doctor.go` | policy health |
| `vibe.yaml`, `.gitattributes`, `.vibe/state.yaml` | self-hosting (C7) |
| `examples/typescript/{vibe.yaml,.gitattributes,.vibe/state.yaml}` | the policy with one user rule (C7) |
| `examples/{python,monorepo}/.vibe/state.yaml` | schema 4 only (C4) |
| Docs | `docs/usage.md`, `docs/architecture/overview.md`, `README.md`, spec 0011 pointer, spec 0029 status |

## Order of work

C1, the spec and ADR, is committed. Every later commit leaves
`task verify` and `task audit` green. The spec and ADR described one
managed-section commit. Here it is two (C4 state, C5 sections), so the
schema migration can be reviewed and bisected on its own. The policy and
docs move to C6 and C7.

1. **C2 `docs: plan 0029`**: this file. Gate: user approval.
2. **C3 `refactor: generalize the integration catalog into an option
   catalog (spec 0029 C3)`**:
   - Add `Option`, `Group` and `Selection`, and rename call sites.
   - No policy group is registered yet.
   - Proof of no behaviour change: the existing suites pass unmodified
     apart from type names, including `wiring_test.go`, the integration
     and prune tests, and `TestExamplesAreConformant`. `vibe diff` in the
     root and in the three examples reports nothing.
3. **C4 `feat: state schema 4, structured resource records (spec 0029
   C4)`**:
   - Schema 4 for `State`, with `Sections` (empty in this commit), custom
     YAML, and migration.
   - Tests:
     - round trips of schema 4;
     - loading each of schema 1, 2 and 3 from fixtures;
     - deterministic ordering;
     - rejection of duplicates and of unknown ownership.
   - Rebuild, then sync the root and the three examples. Only the state
     layout and `vibe_version` change. Commit each state file.
4. **C5 `feat: managed-section ownership keyed by path and section id
   (spec 0029 C5)`**:
   - `internal/textregion` with table tests:
     - absent, well-formed, half-pair, end before begin, duplicate,
       overlapping and nested markers;
     - CRLF lines around and inside a section;
     - insert top and bottom, with and without a trailing newline;
     - remove, including the inserted blank line;
     - both marker syntaxes.
   - Resource fields, and the `SectionChecker` interface.
   - CLI wiring through a fake module in tests. No shipped module emits a
     section yet. The CLI tests cover:
     - every row of spec §3's decision table, for diff, audit and sync;
     - two sections in one file, one in conflict holding both;
     - a checker conflict and a checker warning;
     - prune remove, forget, and conflict, including deleting a
       `created` file that becomes empty.
5. **C6 `feat: opt-in policy.line_endings owns its .gitattributes rules
   (spec 0029 C6)`**:
   - The `policy:` manifest key, the `line_endings` group in all four
     standards, and the `lineendings` module and its check.
   - The doctor rows.
   - Tests:
     - manifest strictness and the unknown-value message;
     - the global-versus-narrow table in spec §4, line numbers included;
     - quoted and `/*` patterns;
     - deselection keeping user rules;
     - a golden test that syncs a fixture repository with the policy and
       asserts the exact `.gitattributes` bytes and the state `sha256`
       as constants. It runs on both CI legs, which is what proves
       Windows and Linux agree.
6. **C7 `docs: line-ending policy in usage; self-host it (spec 0029
   C7)`**:
   - Add `policy: {line_endings: lf}` to this repository's `vibe.yaml`
     and to `examples/typescript`'s, and sync both.
   - In the root, delete the now-duplicate hand-written
     `* text=auto eol=lf` line below the section.
   - In `examples/typescript`, add `*.png binary` below the section
     before syncing.
   - Commit each file together with its state file.
   - Write the docs listed in spec §Documentation, set the spec status
     to implemented, and fill in this plan's checklist and verification
     record.

**Self-hosting hazards.**

- `.gitattributes` governs how git materializes the files VibeConform
  hashes, and that includes itself. Before C7 is committed, run
  `git add --renormalize .`, then `git status`, and expect no changes:
  the root is already LF everywhere (`git ls-files --eol`). The rule
  does not change between the hand-written line and the managed section,
  so nothing is re-checked-out.
- `task audit` runs the `vibe` on PATH. Run `go install ./cmd/vibe` after
  C4, C5 and C6, before any sync or audit.
- Schema 4 written in C4 cannot be read by the `vibe` on `main`. CI's
  conformance job installs the recorded `vibe_version` (spec 0022), so it
  is unaffected, but anyone running an older `vibe` locally against this
  branch gets the load error from spec §8.

## Checklist

- [x] C2 plan approved and committed
- [x] C3 every existing standard, manifest and example resolves the same resources in the same order; error texts unchanged
- [x] C4 schema 1, 2 and 3 fixtures load; the next save writes schema 4, sorted
- [x] C4 a schema 4 file round-trips byte for byte; duplicate (`path`, `section_id`) and unknown `ownership` are load errors
- [x] C4 root and examples re-synced; only the state layout and `vibe_version` change
- [x] C5 `textregion`: every malformed-marker case is reported, never repaired
- [x] C5 bytes outside a section are unchanged after insert, replace and remove, CRLF included
- [x] C5 every row of the decision table, for diff, audit (exit codes) and sync
- [x] C5 two sections in one file are planned and written together; a conflict in one holds both
- [x] C5 prune: remove, forget, conflict; a `created` file left empty is deleted, and one with user lines is kept
- [x] C6 `policy:` strict decoding; `policy.line_endings (crlf): unknown value (valid: lf)`; a scalar group's `Requires`/`Excludes` checked like an integration's (fake catalog)
- [x] C6 a narrow rule below the section is not reported; a global contradiction below it is a conflict (exit 2 in audit, non-zero in sync, nothing written); a rule above it is a warning
- [x] C6 deselecting the policy removes the section and keeps `*.png binary`
- [x] C6 the golden-hash test passes on Ubuntu and Windows CI
- [x] C6 doctor: PASS, WARN (`i/crlf` count), FAIL (overridden `eol`), and the hint when the policy is off
- [x] C7 root and `examples/typescript` self-host the policy; `vibe audit` is conformant in all four repositories
- [x] C7 docs updated; spec status set to implemented

## Found during implementation

- **Doctor got a second function, not a flag (C6).** The plan gave
  `doctor.LineEndings` a `policy bool` argument. Instead, a separate
  `doctor.LineEndingPolicy` shares an `attributes` helper with it, so the
  spec 0028 rows and their tests are unchanged. The CLI appends
  `; policy.line_endings is not selected` to the old row.
- **`Selection.Has` and `Standard.With` (C3).** Pruning needs a trial
  selection that adds one option of either kind. `With` builds it in
  catalog order, and `Has` replaces `slices.Contains` over names, which
  could not tell a policy value from an integration name.
- **Held sections are reported, not counted (C5).** When one section of
  a file conflicts, the file's other sections are reported as follows:
  - `diff` adds `(held: another section of this file conflicts)`;
  - `sync` prints `not written: …`.

  They are not counted as conflicts themselves. The conflicting section
  already makes `sync` exit non-zero.
- **A policy names itself in relation errors (C6).** A policy's
  `Requires` or `Excludes` error reads
  `policy.line_endings: lf requires …`. An integration's text is
  unchanged.

## Verification

Observed locally on 2026-10-01, on Windows 11 (windows/amd64) with Go
1.27.0, Task 3.53.1, Git 2.53.0.windows.1, and `core.autocrlf=true`:

- `task verify` passed at C3, C4, C5, C6 and C7. `task audit` was
  conformant at each. `vibe audit` was conformant in all three examples.
- **C3:** `vibe diff` in the root and the three examples reported
  nothing.
- **C4:** re-syncing the root and the three examples changed only the
  state layout and `vibe_version`. The sorted set of `sha256` values in
  each state file was identical before and after.
- **Scratch repository** (`git init`, prod-go, policy on, an existing
  `.gitattributes` with `*.png binary` and `*.sh text eol=lf`):
  - `diff` showed `would add`.
  - `sync` inserted the section above the user's rules, and `audit` was
    conformant.
  - `git check-attr` resolved `eol: lf`, `text: auto` for `Taskfile.yml`,
    and `text: unset` for `a.png`.
  - Appending `* eol=crlf` made `audit` exit 2, naming line 8. `sync`
    exited 1 and left the line in place.
  - Narrowing the rule to `*.bat eol=crlf` made `audit` conformant again.
  - `vibe doctor` printed
    `PASS line endings policy line_endings: lf; core.autocrlf=true; …`.
  - Deselecting the policy removed the section and kept all three user
    rules. Doctor then fell back to the spec 0028 `WARN`, plus the
    not-selected hint.
- **Second scratch repository** (`core.autocrlf=false`, one file
  committed with CRLF): after syncing the policy, doctor printed
  `WARN … 1 tracked file is stored with CRLF; run git add --renormalize .`.
  After `git add --renormalize .` it printed `PASS`.
- **Self-hosting:**
  - The root's `.gitattributes` is now the managed section alone; the
    hand-written duplicate line was removed.
  - `examples/typescript` has `*.png binary` below the section.
  - `git add --renormalize .` staged no file beyond those edited.
  - `vibe doctor` in the root printed `PASS` for line endings.

Pending: CI on the PR, where the golden-hash test runs on both Ubuntu and
Windows.

## Explicitly still deferred

As spec 0029's non-goals. In addition:

- making structured-patch element keys structured in state (ADR 0014,
  Consequences);
- a `vibe` command that converts an existing hand-written rule into the
  managed section.
