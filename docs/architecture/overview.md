# VibeConform Architecture Overview

Status: living document. This is the authoritative technical description of
VibeConform's design; Notion (or any other project tracker) holds roadmap
and status, never competing architecture.

The design principles every change is held to — mechanical rules over
prompts, Windows/Linux parity, and generated output that stands on
third-party tools rather than `vibe` — are in
`docs/architecture/principles.md`.

## Conceptual model

```text
vibe.yaml
    +
versioned standard
    ↓
VibeConform resolver/reconciler
    ↓
repo integrations
├── language tooling
├── CI
├── Git hooks
├── Codex
├── Claude
├── Skills Manager
└── repository intelligence
```

`vibe.yaml` is human-owned desired state: which standard and version a
repository conforms to, plus repository-specific overrides. Machine-owned
state (`.vibe/lock.yaml`, `.vibe/state.yaml`) records what was last resolved
and applied, enabling three-way reconciliation. `.vibe/state.yaml` exists
today: `vibe sync` writes it and `vibe audit`/`vibe diff` read it.
`.vibe/lock.yaml` does not exist yet (see "Internal package layout" below).

## Module/component composition

Adapted from [projen](https://github.com/projen/projen): a standard is a
composition of modules, and each module resolves into concrete resources.

```text
standard
    ↓
modules/components
    ↓
resolved resources/tasks/adapters
```

```go
type Module interface {
    Name() string
    Resolve(ctx context.Context, mctx *Context) ([]resource.Resource, error)
}
```

See `internal/module` for the current (intentionally minimal) interface and
`internal/resource` for the `Resource` type it produces. Do not treat this
signature as frozen — it will grow as the resolver is implemented.

## Resource ownership

Adopted to avoid relying on fragile text markers as the primary ownership
mechanism. See `docs/decisions/0003-resource-ownership.md` and
`internal/resource` for the `Ownership` enum:

- `generated` — VibeConform owns the whole file.
- `structured-patch` — VibeConform owns specific structured fields.
- `managed-section` — VibeConform owns a delimited section of a
  project-owned file.
- `project-owned` — read-only context; never written.

`generated` owning the whole file does not mean a repository has no
recourse: a generated file may delegate to an unmanaged sibling by a
documented extension point, as `Taskfile.yml` does to `Taskfile.local.yml`.
That is a seam beside a wholly-generated file, not a weaker ownership mode
— the ownership of `Taskfile.yml` itself is unchanged. See
`docs/decisions/0009-managed-file-local-extension.md`.

## Three-way reconciliation

Adopted from [Copier](https://github.com/copier-org/copier):

```text
previous resolved state
        +
current repository
        +
new desired state
        ↓
reconciliation
```

- `current == target` → no change.
- `current == previous`, `target != previous` → **out of date**: the
  repository is exactly as VibeConform last wrote it and the standard has
  moved on. Safe replacement.
- `current != previous`, `target == previous` → **local drift**: the
  managed file was edited after it was written. Safe replacement too, but a
  different thing to tell the user.
- `current != previous` and `target != previous` → conflict, surfaced to the
  user rather than silently overwritten.
- `project-owned` → never overwritten.

The two middle cases both mean "write the target", and spec 0006 therefore
collapsed them into one decision. Spec 0019 separates them because they
differ in whose doing it is: reporting a moved standard as drift told
adopters who had changed nothing that files they never opened had drifted.
No extra recorded state was needed — the two conditions are mutually
exclusive wherever both are reachable, so the distinction was already
implied by the three hashes.

What hashes cannot express is *direction*: a target that moved because the
binary is newer looks identical to one that moved because the binary is
older. `.vibe/state.yaml` schema 2 records which `vibe` last wrote it, so
`sync` can refuse to run backwards instead of reverting managed files and
recording the result as correct. Provenance is absent from pre-0019 state
files, and absent means unknown — never old.

## Audit / diff / sync UX

Adopted from [Cruft](https://github.com/cruft/cruft):

- `vibe audit` — read-only compliance/drift check; non-zero exit on strict
  non-compliance. Safe to run in CI.
- `vibe diff` — human-readable reconciliation preview.
- `vibe sync` — performs the reconciliation.

## Affected-component graph

Adopted from [Nx](https://nx.dev/):

```text
changed files
    ↓
owning components
    ↓
reverse dependency closure
    ↓
affected components
    ↓
applicable validation tasks
```

Mixed-language repositories are represented as an explicit component graph,
not a flat `languages: [...]` list:

```yaml
components:
  - id: web
    path: apps/web
    profile: typescript
    depends_on: [contracts]

  - id: api
    path: services/api
    profile: go
    depends_on: [contracts]
```

The graph engine is not built at bootstrap time (see
`docs/plans/0001-bootstrap.md`).

## Repository intelligence and Skills Manager boundaries

VibeConform will eventually orchestrate, but not absorb, two adjacent
systems through provider abstractions:

- **GitNexus** (repository intelligence): index/search/dependency/
  caller/impact/trace. It must never replace the compiler, LSP, linter,
  tests, or CI — it augments them.
- **Skills Manager**: canonical skill storage, deployment, and per-project
  activation. It must never become a CI dependency.

## Internal package layout

Present today:

```text
internal/
  manifest/                 # vibe.yaml parsing
  standard/                 # versioned standard definitions
  module/                   # module composition interface + optional ToolRequirer
    gotooling/              # .golangci.yml
    ci/github/              # GitHub Actions workflow, dependabot, PR template (prod-go)
    ci/githubts/            # the same, for prod-ts
    ci/githubpy/            # the same, for prod-py
    repotooling/            # Taskfile.yml, lefthook.yml, Go guard (prod-go)
    tsrepotooling/          # Taskfile.yml, lefthook.yml, Node guard (prod-ts)
    pyrepotooling/          # Taskfile.yml, lefthook.yml, Python guard (prod-py)
    agents/                 # Claude/Codex config + guard policy (policy.json)
    tstooling/              # eslint, prettier, tsconfig base
    pythontooling/          # ruff, pyright
  resource/                 # resource + ownership + file mode model
  state/                    # .vibe/state.yaml read/write
  reconcile/                # three-way decision engine
  atomicfile/               # temp-file + rename writes
  cli/                      # command tree; audit/diff/sync share one plan walk
```

Since M2, `examples/typescript` and `examples/python` hold real repositories
declaring the language standards, synced and committed. They are the drift
alarm for modules this repository cannot dogfood: a Go repository never
resolves `tstooling` or `pythontooling`, so without them those templates
would have no live counterpart, which is the role `.golangci.yml` plays for
`gotooling`. `internal/cli` enforces it — deliberately not `Taskfile.yml` or
the CI workflow, since both are resources shipped to every adopting
repository and must not name paths that exist only here.

Not built yet:

```text
internal/
  affected/       # changed-files -> affected-components graph
  validation/     # task execution for affected components
  skills/         # Skills Manager provider
  intelligence/   # GitNexus provider
```

`.vibe/lock.yaml` does not exist either: state alone closes the
reconciliation loop, and the lock file earns its place when standards
resolve dynamically rather than being registered in Go.

Packages are created when there is real code to put in them, not in
advance. Audit/diff/sync orchestration lives in `internal/cli` rather than a
separate `audit/` package, because all three are thin reporters over one
shared plan walk — a package boundary there would separate nothing.

## Canonical verification interface

Taskfile (`Taskfile.yml`) is the single cross-platform entry point for local
and CI verification (`task fmt`, `task lint`, `task test`, `task security`,
`task audit`, `task verify`). CI, Git hooks, and docs invoke these tasks
rather than duplicating command lists. Since M1, `Taskfile.yml` is itself a
managed resource, and `task audit` checks this repository against the
standard it declares.

Since spec 0017, `task verify`/`task verify-ci` depend only on native
language tooling — `task audit` is deliberately not one of `verify`'s
steps. A generated repository's language verification must not require a
`vibe` binary (or, for Go, a local `cmd/vibe`) to succeed; `task audit` and
its CI `conformance` job stay independently invocable, VibeConform-specific
checks layered on top, not folded into what `verify` means by "the code is
correct."

Since spec 0018, all three standards resolve `vibe` the same way in
`audit`: a `PATH` lookup (`vibe audit --repo-root .`). Go previously used
`go run ./cmd/vibe`, which resolved only in this repository and left every
external `prod-go/v1` adopter with a `task audit` — and so a CI
`conformance` job — that could not run. The Go `conformance` job's
`Install vibe` step is conditional as a result; see
`docs/decisions/0008-self-hosting-probe-in-shipped-templates.md` for why a
published release is the wrong binary for this repository to audit itself
with.

The managed target set is fixed and extended, never overridden. A
repository's own tasks go in a project-owned `Taskfile.local.yml`, which
the generated `Taskfile.yml` includes optionally; Task treats a name
collision as a hard error, so the verification interface stays what the
standard says it is. See
`docs/decisions/0009-managed-file-local-extension.md`.

### `vibe check`'s safe-fallback contract

`vibe check` (not yet implemented; see "Not built yet" under "Internal
package layout" above) will
eventually let validation skip components unaffected by a change set, once
the affected-graph and validation subsystems exist. Whenever it can't
establish that narrower scope safely, it must run the full equivalent of
`task verify` instead of skipping anything — it may never report success
having silently skipped checks. A full run is required whenever: `vibe`
itself can't be resolved from the calling context; the affected-component
scope can't be determined with confidence (no prior state to diff against,
or a graph-resolution error); or a change touches something with unbounded
blast radius that isn't representable as a single component (`Taskfile.yml`
itself, lint/tooling configuration, dependency manifests, or the
`vibe.yaml` standard declaration). This extends two precedents already in
the codebase: `vibe check`/`vibe doctor`'s existing "not implemented yet"
placeholder already fails loudly rather than no-oping (`docs/usage.md`),
and `vibe sync`'s `ToolRequirer` warning already surfaces a missing tool
rather than hiding it (spec 0014) — `vibe check`'s fallback is stricter
than that warning-only pattern, since it must still run full verification,
not just warn.
