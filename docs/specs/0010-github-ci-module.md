# Spec 0010: GitHub CI module (`internal/module/ci/github`)

Status: accepted and implemented.

## Problem

`production/v1` composes one module resolving one resource at a repository
root. Every mechanic M1 needs beyond that is therefore untested: multiple
resources from one module, resources in subdirectories, and a stable
ordering for the reports `audit`/`diff`/`sync` print.

This repository's `.github/` directory is also still hand-maintained, which
`docs/specs/0001-v0-control-plane.md` names as exactly what M1 must migrate
under VibeConform.

## Scope

A new package, `internal/module/ci/github`, providing one module:

- `Module.Name()` returns `"github-ci"`.
- `Module.Resolve` returns three `resource.Generated` resources, each with
  content embedded via `go:embed`:
  - `.github/workflows/ci.yml`
  - `.github/dependabot.yml`
  - `.github/pull_request_template.md`
- Every template is seeded **verbatim** from this repository's current file.

`production/v1` registers the module alongside `go-tooling`:

```go
Modules: []module.Module{gotooling.New(), github.New()},
```

## Behavior

- Resolution order is fixed and documented: modules in the order
  `standard.Standard.Modules` lists them, resources in the order `Resolve`
  returns them. `audit`, `diff`, and `sync` report in that order, so their
  output is stable across runs and across platforms. Nothing iterates a map
  to produce output.
- `.github/workflows/ci.yml` is the first resource whose path has a parent
  directory. `sync` already calls `os.MkdirAll` (spec 0008); this spec is
  what exercises it, and brings the test that proves it.
- Like `go-tooling`, `Resolve` ignores `mctx` — v1 content is fixed, not
  computed from the target repository.

## Explicit non-goals

- **No `.github/workflows/release.yml`, and no `.goreleaser.yaml`.** The
  release workflow presumes GoReleaser, tag-driven releases, and a project
  that ships binaries. That is a repository policy choice, not a baseline
  guardrail every conformant repository should have. Both files stay
  project-owned and hand-maintained; `docs/specs/0013-dogfood-self-management.md`
  says so explicitly rather than leaving the gap implicit.
- **No conditionality.** `production/v1` cannot yet say "this resource only
  if the repository releases binaries" — there are no manifest overrides and
  no component graph (M2). A standard is all-or-nothing in v1, which is the
  real reason `release.yml` is excluded rather than made optional.
- No parameterization: Go version, action SHAs, and job names are fixed in
  the template, not derived from the target repository. A repository on a
  different Go version cannot conform to `production/v1` yet.
- No validation that the embedded workflow is valid GitHub Actions YAML
  beyond `actionlint` running over this repository's own copy — the module
  does not lint its own template at runtime.
- No change to `module.Module`, `module.Context`, or `resource.Resource`.
- No ownership mode other than `Generated`.

## Design notes

- Package path `internal/module/ci/github` matches the layout
  `docs/architecture/overview.md` projects (`ci/github/` under a `ci/`
  grouping), leaving room for a future `ci/gitlab` without a rename.
- Templates live as real `.yml` / `.md` files in the package and are
  embedded, not Go string literals — same reasoning as spec 0005: they stay
  reviewable and diffable in their native format, and `actionlint` can be
  pointed at them later.
- Embedding a copy rather than reading this repository's live `.github/`
  files at runtime keeps the module's desired state from silently tracking
  VibeConform's own tooling drift. The duplication is real and temporary:
  `docs/specs/0013-dogfood-self-management.md` closes it by making the live
  files outputs of the module.
- Resource paths are written slash-separated in the module
  (`.github/workflows/ci.yml`), never with `filepath.Join`, since they are
  repository-relative identifiers and `.vibe/state.yaml` keys — not host
  paths. `internal/cli` converts at the I/O boundary (spec 0008).

## Follow-on work

- `internal/module/repotooling` (spec 0011) and the agent-config module
  (spec 0012) complete the module set M1 needs.
- Parameterized templates (Go version, action pins) once `vibe.yaml`
  supports overrides — M2.
- `release.yml` / `.goreleaser.yaml`, if and when standards can express
  optional components.
