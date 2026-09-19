# Plan 0013: Dogfood — VibeConform manages VibeConform

See `docs/specs/0013-dogfood-self-management.md` for the accepted scope, and
`docs/plans/0007-m1-milestone.md` for where this sits in M1. This is the
closing increment: when it merges, M1's four exit criteria are met.

## Correction: adopt before changing templates

This plan originally said "templates, then sync, then docs", reasoning that
syncing before the template edits would produce a no-op and hide whether the
templates were right. Implementation showed that order does not work, for a
reason worth recording:

on first adoption there is no `.vibe/state.yaml`, so `previous` is absent for
every resource. Any resource whose live file differs from its template then
decides `Conflict` (truth-table row 3), not `Overwrite` — and `sync` refuses
to write it. Editing the templates first therefore produces two conflicts the
tool will not resolve, and the only honest fixes are deleting the live files
or hand-editing them, both of which destroy the evidence that the templates
reproduce the repository.

The working order is the reverse:

1. `vibe init`, then `vibe sync` with templates exactly as seeded. Every
   resource reports `unchanged`, state is recorded, and *that* is the
   acceptance test for 0010–0012.
2. Then edit the templates and `vibe sync` again. With state on record the
   same edits decide `Overwrite`, and sync applies them cleanly.

Observed on the real repository: step 1 gave `0 created, 0 updated, 11
unchanged, 0 conflicts`; step 2 gave `0 created, 2 updated, 9 unchanged, 0
conflicts`.

Also worth knowing: `go:embed` resolves at build time, so the binary must be
rebuilt after every template change. A stale binary silently reconciles
against the templates it was compiled with.

## Checklist

### Adopt first

- [x] `vibe.yaml` at the repository root: `standard: production`,
      `version: v1`. Write it with `vibe init production v1`, not by hand.
- [x] Run `vibe sync` with templates exactly as seeded. Every resource must
      report `unchanged`. **This is the acceptance test for increments
      0010–0012** — templates written from hand-maintained files reproducing
      those files exactly. Observed: `0 created, 0 updated, 11 unchanged, 0
      conflicts`.
- [x] Confirm `.vibe/state.yaml` is actually tracked (`git check-ignore`
      must not match) — reconciliation silently degrades to two-way if it is
      not committed.

### Then change templates

- [x] `internal/module/repotooling/templates/Taskfile.yml`: add the `audit`
      target and add `task: audit` to `verify`'s step list, after `test` and
      before `mod:verify`.
- [x] `internal/module/ci/github/templates/ci.yml`: add a `conformance` job
      (checkout, setup-go, install task, `task audit`) with
      `continue-on-error: true`, **not** added to `gate`'s `needs`.
- [x] Rebuild the binary, then `vibe sync` again: the two edited resources
      decide `Overwrite` and are applied. Observed: `0 created, 2 updated, 9
      unchanged, 0 conflicts`.
- [x] Run `vibe audit`: every resource `ok`, exit 0.
- [x] `task verify` clean — now including `task audit`.

### Documentation

- [x] `AGENTS.md`: a rule under "Rules" — files managed by VibeConform
      (listed, or pointed at `vibe audit`'s output) must be changed via
      their module template followed by `vibe sync`, never edited directly.
      Include the reason: a direct edit is non-conformant the moment it
      lands, and CI will say so.
- [x] `README.md`: status banner — VibeConform manages its own guardrails
      and audits itself in CI; M1 complete.
- [x] `docs/usage.md`: a short "VibeConform manages itself" section pointing
      at `vibe.yaml` and `.vibe/state.yaml` as the worked example, and
      naming `release.yml` / `.goreleaser.yaml` as the remaining
      hand-maintained files under `.github/`.
- [x] `docs/architecture/overview.md`: the "Future internal package layout"
      section is now partly present tense — mark which packages exist.
- [x] `docs/plans/0001-bootstrap.md`: sequencing steps 3 and 4 are done; say
      so.
- [x] `docs/plans/0007-m1-milestone.md`: record M1's exit criteria as met.
- [x] Specs 0008–0013 and plans 0008–0013: `Status: accepted and
      implemented`, checklists ticked.

### Step B — make the gate binding

- [x] `internal/module/ci/github/templates/ci.yml`: remove
      `continue-on-error` from `conformance`; add `conformance` to `gate`'s
      `needs` **and to the result loop inside `gate`'s step**. Adding it to
      `needs` alone makes `gate` wait for the job without ever checking its
      outcome — a gate that looks wired up and enforces nothing.
- [x] Rebuild, `vibe sync`, commit the workflow and the state change.
      Observed: `0 created, 1 updated, 10 unchanged, 0 conflicts`.
- [ ] **Open:** confirm on a real PR that `CI / gate` fails when a managed
      file is edited directly — drift one file, watch it fail, revert. This
      cannot be verified locally; it needs a run on real runners. Locally,
      `vibe audit` exits 2 on a hand-edited managed file, so the input to
      the gate is proven; what remains unproven is the workflow wiring.
      An untested gate is an assumed gate.

## Notes

- Order matters, in the direction the correction note above establishes:
  adopt first, then change templates.
- The branch-protection required check stays `CI / gate` throughout; only
  what `gate` waits for changes. No repository settings need touching.
- Expect this PR's diff to be large and mostly mechanical: the managed files
  move from hand-maintained to generated without their content changing. The
  interesting parts are `vibe.yaml`, `.vibe/state.yaml`, the two template
  edits, and the `AGENTS.md` rule.

## Explicitly still deferred (M2)

Per `docs/specs/0013-dogfood-self-management.md`: no `release.yml` /
`.goreleaser.yaml` management, no branch-protection or repository-settings
management, no `vibe sync` in CI. Carried forward from
`docs/plans/0012-agent-config-module.md`: no `AGENTS.md`/`CLAUDE.md`
generation, no mode in reconciliation or state, no `StructuredPatch`
merging, no `.gitattributes`/`.gitignore` management, no toolchain
provisioning, no parameterization or conditionality in standards, no
`--force`/`--dry-run`/`--json`, no orphan pruning or detection, no
`.vibe/lock.yaml`, no affected-component graph, no `check`/`doctor`, no
dynamic standard loading, no `vibe.yaml` overrides, no second dogfood
repository.
