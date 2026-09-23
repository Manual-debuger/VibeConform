# Spec 0009: `vibe audit` (v2 — strict conformance gate)

Status: accepted and implemented. **Partly superseded by
`docs/specs/0019-drift-classification.md`**, which splits the `Overwrite`
decision below into `LocalDrift` and `OutOfDate`, gives them distinct
messages, adds exit code `3` for out-of-date-only, and makes a `vibe` older
than the one that last synced the repository decline to give a verdict.

The output examples and the decision table in this document are therefore
the pre-0019 contract. They are left as written: this is a record of what
was decided then, not a description of current behaviour. See
`docs/usage.md` for the latter.

## Problem

`vibe audit` still prints `<N> modules configured, nothing to check` — the
placeholder report spec 0004 shipped before any module or reconciliation
engine existed. Both exist now, and spec 0004 explicitly deferred "audit
becomes strict" until they did. Spec 0008 finished the last prerequisite:
users can now fix what `audit` reports, so `audit` can start failing.

Without this, nothing in M1 can gate. `docs/plans/0007-m1-milestone.md`'s
closing increment wires `audit` into CI as this repository's own conformance
check, which requires a non-zero exit on drift and — critically — an exit
code CI can tell apart from "the tool broke".

## Scope

- `vibe audit [--repo-root]` reuses `buildPlan` (spec 0008) instead of
  re-reading the manifest and standard itself, so `audit`, `diff`, and
  `sync` all decide identically.
- Report: the `standard: <name>/<version>` header, one line per resource
  naming its state, then a verdict line.
- Conformance rule: a repository is conformant when every supported
  resource decides `NoChange`. `Create`, `Overwrite`, and `Conflict` are all
  non-conformance — a missing managed file is exactly as non-conformant as a
  drifted one.
- Resources with an ownership mode no command handles are reported and
  excluded from the verdict. Nothing can act on them, so they cannot be
  drift.
- Exit codes:
  - `0` — conformant.
  - `1` — the tool could not answer: missing/unreadable `vibe.yaml`,
    unregistered `(standard, version)`, malformed `.vibe/state.yaml`, or an
    I/O failure.
  - `2` — the tool answered, and the repository is not conformant.
- `internal/cli` exports `ExitCode(err error) int` so `cmd/vibe/main.go` can
  map a non-conformance error to `2` and everything else to `1`.

Example, conformant:

```
$ vibe audit
standard: production/v1
.golangci.yml: ok
1 resource checked, 0 drifted, 0 conflicts
conformant
```

Non-conformant:

```
$ vibe audit
standard: production/v1
.golangci.yml: drifted (run vibe sync)
1 resource checked, 1 drifted, 0 conflicts
not conformant
$ echo $?
2
```

## Behavior

### Decision → report line and verdict

| Decision | Line | Conformant? |
|---|---|---|
| `NoChange` | `ok` | yes |
| `Create` | `missing (run vibe sync)` | no |
| `Overwrite` | `drifted (run vibe sync)` | no |
| `Conflict` | `conflict: manual changes detected` | no |
| unsupported ownership | `not yet checked (unsupported ownership)` | not counted |

`Conflict` is counted separately from `drifted` in the summary line because
the remedy differs: drift is fixed by running `sync`, a conflict is not.

### Why a distinct exit code

CI treats any non-zero exit as failure, which is correct for gating but
useless for diagnosis: a repository that has drifted and a repository whose
`vibe.yaml` is missing are different problems, and only one of them is
fixed by running `sync`. Separating them now means the dogfood increment's
CI job can report which happened without parsing output text.

## Explicit non-goals

- **No `--fix` / no writing.** `audit` stays read-only; `sync` is the writer.
  A flag that quietly turns the gate into a mutation is exactly the kind of
  thing that should require typing a different command.
- **No `--strict` flag.** Strict *is* the behavior now. A flag to make the
  gate optional invites repositories to disable it and call themselves
  conformant.
- No per-module grouping in the report — resources are listed flat, in plan
  order. Grouping earns its place when a standard has enough modules for the
  flat list to be hard to read.
- No machine-readable output (`--json`). Nothing consumes it yet; the exit
  code carries what CI needs.
- No change to `diff`, `sync`, or `reconcile.Decide`.
- No orphan detection — resources recorded in state but no longer resolved
  by any module are ignored, matching spec 0008's non-goal.

## Design notes

- The non-conformance error is an unexported type in `internal/cli` with an
  `errors.As`-compatible accessor, rather than a sentinel value, so the
  count travels with it and `ExitCode` stays a pure mapping.
- `main.go` becomes `os.Exit(cli.ExitCode(err))`. This is the only exported
  surface `internal/cli` gains; keeping the mapping inside the package means
  future commands can reuse it without `main` learning about each one.
- The `<N> modules configured, nothing to check` string disappears, which
  also retires the pluralization nit spec 0005 left open.
- `audit_test.go`'s existing module-count assertion is rewritten rather than
  extended — the old output format no longer exists.

## Follow-on work

- Wiring `audit` into `task verify` and CI against this repository is the
  dogfood increment (plan 0007's 0013), not this spec.
- `--json` output, once something automated consumes `audit`'s report rather
  than its exit code.
