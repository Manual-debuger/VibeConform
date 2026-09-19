# Plan 0013: Dogfood — VibeConform manages VibeConform

See `docs/specs/0013-dogfood-self-management.md` for the accepted scope, and
`docs/plans/0007-m1-milestone.md` for where this sits in M1. This is the
closing increment: when it merges, M1's four exit criteria are met.

## Checklist

### Template changes first

- [ ] `internal/module/repotooling/templates/Taskfile.yml`: add the `audit`
      target and add `task: audit` to `verify`'s step list, after `test` and
      before `mod:verify`.
- [ ] `internal/module/ci/github/templates/ci.yml`: add a `conformance` job
      (checkout, setup-go, install task, `task audit`) with
      `continue-on-error: true`, **not** added to `gate`'s `needs`.
- [ ] Both `templates_test.go` drift alarms will now fail — they compare
      templates against live files that have not been synced yet. That
      failure is expected and is resolved by the sync step below, not by
      weakening the tests.

### Adopt

- [ ] `vibe.yaml` at the repository root: `standard: production`,
      `version: v1`. Write it with `vibe init production v1`, not by hand.
- [ ] Run `vibe sync` and commit both the changed managed files and the
      generated `.vibe/state.yaml`.
- [ ] Confirm `.vibe/state.yaml` is actually tracked (`git status` must not
      show it as ignored) — reconciliation silently degrades to two-way if
      it is not committed.
- [ ] Run `vibe audit`: every resource `ok`, exit 0. **This is the
      acceptance test for increments 0010–0012.** Any `drifted` or
      `conflict` result is a finding to resolve explicitly in this PR, not
      to paper over by re-seeding the template from the live file.
- [ ] `task verify` clean — now including `task audit`.

### Documentation

- [ ] `AGENTS.md`: a rule under "Rules" — files managed by VibeConform
      (listed, or pointed at `vibe audit`'s output) must be changed via
      their module template followed by `vibe sync`, never edited directly.
      Include the reason: a direct edit is non-conformant the moment it
      lands, and CI will say so.
- [ ] `README.md`: status banner — VibeConform manages its own guardrails
      and audits itself in CI; M1 complete.
- [ ] `docs/usage.md`: a short "VibeConform manages itself" section pointing
      at `vibe.yaml` and `.vibe/state.yaml` as the worked example, and
      naming `release.yml` / `.goreleaser.yaml` as the remaining
      hand-maintained files under `.github/`.
- [ ] `docs/architecture/overview.md`: the "Future internal package layout"
      section is now partly present tense — mark which packages exist.
- [ ] `docs/plans/0001-bootstrap.md`: sequencing steps 3 and 4 are done; say
      so.
- [ ] `docs/plans/0007-m1-milestone.md`: record M1's exit criteria as met.
- [ ] Specs 0008–0013 and plans 0008–0013: `Status: accepted and
      implemented`, checklists ticked.

### Step B — make the gate binding (separate PR)

- [ ] `internal/module/ci/github/templates/ci.yml`: remove
      `continue-on-error` from `conformance`; add `conformance` to `gate`'s
      `needs`.
- [ ] `vibe sync`; commit the workflow and the state change.
- [ ] Confirm on a real PR that `CI / gate` fails when a managed file is
      edited directly — deliberately drift one file, watch it fail, revert.
      An untested gate is an assumed gate.

## Notes

- Order matters: templates, then sync, then docs. Syncing before the
  template edits produces a no-op and hides whether the templates were
  right.
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
