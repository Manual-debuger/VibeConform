# ADR 0009: Extending a Generated File Without Owning It

## Status

Accepted. Implemented by the `includes:` block in all three repo-tooling
`Taskfile.yml` templates, per
`docs/specs/0018-prod-go-audit-portability.md`.

## Context

`docs/decisions/0003-resource-ownership.md` defines `generated` as
"VibeConform owns the whole file." `Taskfile.yml` is `generated`, and it is
also the canonical verification interface
(`docs/architecture/overview.md`) — every repository invokes its tooling
through it. The two facts together leave an adopting repository with
nowhere to put a task of its own: hand-adding one makes the repository
non-conformant the moment it lands, and CI says so.

`prod-go/v1` papered over this by shipping two tasks of its own:

```yaml
  build:
    desc: Build the vibe binary into ./bin.
    cmds:
      - go build -o bin/vibe ./cmd/vibe

  run:
    desc: 'Run the CLI, e.g. task run -- --help.'
    cmds:
      - go run ./cmd/vibe {{.CLI_ARGS}}
```

Every external `prod-go/v1` adopter received a `task build` that builds
*VibeConform's* CLI. `docs/usage.md` had already noticed this, recording
that TS/PY omit the pair because "those build the `vibe` binary this
repository ships, which doesn't generalize to an adopting repository" — but
filed the conclusion as a TS/PY omission rather than a Go defect.

The generalization failure is not about `vibe` specifically. How a
repository builds and runs its own artifacts is repository-specific even
within one language: two Go repositories on `prod-go/v1` may build a CLI, a
library, several binaries, or nothing. A standard has no basis for
asserting a `build` command. But deleting `build`/`run` without providing
an alternative would say instead that a repository may have no tasks of its
own, which is worse.

## Decision

A generated `Taskfile.yml` declares an optional, flattened include of a
project-owned `Taskfile.local.yml`:

```yaml
includes:
  local:
    taskfile: ./Taskfile.local.yml
    optional: true
    flatten: true
```

VibeConform keeps owning the generated file completely. `Taskfile.local.yml`
is outside the managed set entirely: `vibe` does not create it, write it,
read it, validate it, or audit it, and the string does not appear anywhere
in `internal/`. Ownership is unchanged — `Taskfile.yml` stays
`resource.Generated`. This is a seam *beside* a wholly-generated file, not
a move to `structured-patch` or `managed-section`.

The standard defines the verification interface (`fmt`, `fmt:check`,
`lint`, `typecheck`, `test`, `audit`, `verify`, `verify-ci`, plus Go's
`test:race`, `mod:verify`, `security`, `workflows:lint`). Everything else a
repository needs is its own business and belongs in the local file. All
three repo-tooling templates declare the include identically; giving it to
one language would recreate the asymmetry this decision removes.

This repository is its own first adopter: `build` and `run` moved out of
the Go template into a committed `Taskfile.local.yml` here.

## Consequences

The mechanism is Task's own `includes`, not a VibeConform feature. `vibe`
gains no code, and the properties below are Task's behaviour. Verified
empirically against Task v3.39.0 and the pinned v3.53.1:

- **Absent is a no-op.** `optional: true` means a repository with no
  `Taskfile.local.yml` — the adopter default — sees no warning and no
  error. `examples/typescript` and `examples/python` exercise this on every
  CI run.
- **Present is flat.** `flatten: true` merges tasks at top level, so they
  are invoked bare (`task build`, not `task local:build`), appear in
  `task --list`, and receive `{{.CLI_ARGS}}` normally.
- **Collision is a hard error, not an override.** A local file defining a
  task the generated file already defines makes Task exit **203** with
  `Found multiple tasks (verify) included by "local"`, before running
  anything. A repository can *add* tasks; it cannot redefine `verify`,
  `audit`, `lint`, or `test`. This is what keeps the seam from being a hole
  in the standard: an adopter cannot dodge the `conformance` gate by
  shadowing `audit`, because Task refuses to run at all.

That last property is load-bearing and is **not** enforced by VibeConform.
If Task ever changed collision handling to last-wins, the standard would
silently acquire exactly the hole this design avoids, with no test failing.
Re-confirm it when bumping `TASK_VERSION`.

- **`includes.flatten` requires Task v3.39.0 or newer.** v3.38.0 and below
  ignore `flatten` and namespace the tasks instead, so `task build` fails
  with `Task "build" does not exist` (exit 200) while the printed task list
  shows `local:build`. That is a confusing failure rather than a clear
  "unsupported" one — non-zero and visible, but it names the wrong problem.
  `TASK_VERSION` is pinned to v3.53.1 in all three CI templates, well above
  the floor, so CI is unaffected; a contributor running an older Task
  locally is the exposed case.
- Adding a task to the local file cannot change what `verify` does. Spec
  0017's `TestVerifyNeverInvokesVibe` reads the *templates*, and `verify`'s
  closure only reaches tasks it names — so a local task is unreachable from
  `verify` unless `verify` itself is redefined, which is the collision
  error above.
- `Taskfile.local.yml` is committed, not gitignored. It is project
  configuration that belongs to the repository, not developer-local
  scratch; the name refers to "local to this repository," not "local to
  this machine."
