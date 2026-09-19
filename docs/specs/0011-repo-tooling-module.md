# Spec 0011: Repo-tooling module (`internal/module/repotooling`)

Status: accepted and implemented.

## Problem

`docs/architecture/overview.md` names `Taskfile.yml` as VibeConform's
canonical verification interface — the single entry point CI, Git hooks, and
docs all call rather than duplicating command lists — and `lefthook.yml` as
the fast local first line of defense. Both are still hand-maintained here,
and neither is expressible as desired state, so a repository claiming
conformance to `production/v1` today gets a lint config and CI workflows but
no way to run them.

## Scope

A new package, `internal/module/repotooling`, providing one module:

- `Module.Name()` returns `"repo-tooling"`.
- `Module.Resolve` returns two `resource.Generated` resources, embedded via
  `go:embed`, seeded verbatim from this repository:
  - `Taskfile.yml`
  - `lefthook.yml`

Registered into `production/v1` after `github-ci`.

## Behavior

No new mechanics: same shape as spec 0010's module, fixed content, ignores
`mctx`, documented resolution order. This increment is deliberately small —
it exists to complete the verification surface, not to introduce anything.

## Explicit non-goals

- **No `.gitattributes`.** `* text=auto eol=lf` matters enormously to
  reconciliation (spec 0008's line-ending note), which is an argument for
  managing it — but it also governs how git materializes every file in the
  repository, including files VibeConform writes. Managing the file that
  determines how your own outputs are hashed is a loop worth entering
  deliberately, with its own spec, not as a rider here.
- No `.gitignore` — genuinely project-specific; a standard cannot know what
  a repository builds.
- No installation or version management for `task`, `lefthook`,
  `golangci-lint`, or any other tool the templates invoke. VibeConform
  writes configuration; it does not provision toolchains. A repository that
  syncs `Taskfile.yml` without `task` installed gets a file it cannot run,
  and that is the correct division of responsibility.
- No `lefthook install` — writing `lefthook.yml` does not register git
  hooks. Running a command in the user's repository as a side effect of
  `sync` is a materially larger claim on the machine than writing a file.
  `docs/usage.md` documents the manual step.
- No parameterization (Go-specific task names are fixed in v1), no
  conditionality, no ownership mode beyond `Generated` — as spec 0010.

## Design notes

- Package name `repotooling` rather than `tooling` keeps it clearly distinct
  from `internal/module/gotooling`: one owns language-specific lint
  configuration, the other owns the repository's verification entry points.
  They are separate modules because a non-Go repository would want the
  second without the first.
- `Taskfile.yml` is the highest-blast-radius resource in the standard: a
  broken template breaks `task verify` locally *and* every CI job, since
  every job calls a task target. The template-vs-live equality test
  (spec 0010's pattern) matters here more than anywhere.
- The `verify` target in the template gains a `vibe audit` step in the
  dogfood increment (spec 0013), not here — changing the template and
  adopting it are separate reviews.

## Follow-on work

- Agent-config module (spec 0012) is the last module M1 needs.
- `.gitattributes` management, with its own spec, if the reconciliation
  interaction is worth the loop.
- Non-Go standards, once `production/v1` has a sibling that omits
  `go-tooling`.
