# Plan 0017: Decouple `task verify` from `vibe audit`

See `docs/specs/0017-verify-audit-decoupling.md` for the accepted scope.
Sequenced after spec 0016 (TS/PY tooling parity); no new modules or
standards, so no renumbering assumptions beyond spec 0016's.

## Checklist

### Increment 1 — remove `audit` from `verify`'s dependency chain

- [x] `internal/module/repotooling/templates/Taskfile.yml` (Go): removed
      `- task: audit` from `verify`'s `cmds`. `audit` (`go run ./cmd/vibe
      audit --repo-root .`) is unchanged and still independently runnable.
- [x] `internal/module/tsrepotooling/templates/Taskfile.yml` (TS): same
      removal. `audit` (`vibe audit --repo-root .`) unchanged.
- [x] `internal/module/pyrepotooling/templates/Taskfile.yml` (PY): same
      removal. `audit` unchanged.
- [x] Rebuilt `vibe` (`go build -o bin/vibe ./cmd/vibe`) and ran
      `vibe sync --repo-root .`, `vibe sync --repo-root examples/typescript`,
      `vibe sync --repo-root examples/python` to propagate the template
      edits into the committed `Taskfile.yml` and `.vibe/state.yaml` in all
      three locations (`vibe sync` also picked up increment 2's CI-template
      comment changes in the same pass, since both edits landed before the
      rebuild).

### Increment 2 — make the CI conformance job an explicit, separable integration

- [x] Added an in-template comment to the `conformance:` job in
      `internal/module/ci/github/templates/ci.yml`,
      `internal/module/ci/githubts/templates/ci.yml`, and
      `internal/module/ci/githubpy/templates/ci.yml` identifying it as
      VibeConform's own check, independent of `verify`/`verify-ci`, and
      safe to delete (with its `gate.needs` entry) if a repository stops
      using VibeConform.
- [x] Added a "Removing VibeConform" section to `docs/usage.md` (delete
      `.vibe/`/`vibe.yaml`; remove the `conformance` job and its
      `gate.needs` entry; `task audit` becomes inert and can be deleted or
      left as dead code) and a one-line pointer to it from `README.md`'s
      "Using `vibe`" section.
- [x] Updated `.github/workflows/examples.yml`: reordered each example
      job so `task verify` runs first, with no `vibe` on `PATH` at that
      point, and `task audit` runs afterward as its own explicitly-named
      step against a freshly source-built `vibe` — demonstrating the split
      in this repository's own dogfood CI rather than only in prose.

### Increment 3 — document the `vibe check` safe-fallback contract

- [x] Added a "`vibe check`'s safe-fallback contract" subsection to
      `docs/architecture/overview.md`'s "Canonical verification interface"
      section (decided against a separate ADR — it reads as a direct
      extension of the paragraph already there, not a standalone decision
      record). States the must-run-full-`verify` conditions: `vibe`
      unresolved, affected scope unclear, or a global-config change.
      Documentation only — `internal/affected`/`internal/validation` remain
      unbuilt, consistent with every milestone spec since M0.

### Cross-cutting

- [x] `vibe audit --repo-root .` on this repository: conformant, 0 drifted,
      0 conflicts, after the increment-1 sync.
- [x] `task verify` in `examples/typescript`, with `vibe` removed from
      `PATH`: passes on the clean fixture (fmt:check, lint, typecheck,
      test all ran; `audit` did not run).
- [x] `task verify-ci` in `examples/python`, with `vibe` removed from
      `PATH`: passes on the clean fixture, same shape.
- [x] `task audit` in `examples/python`, with `vibe` removed from `PATH`:
      fails clearly (`"vibe": executable file not found in $PATH`, exit
      127/201) — confirmed no regression from today's behavior.
- [x] Deliberately broke lint in `examples/typescript/src/index.ts`
      (unused variable): `task verify` failed on the `lint` step as
      expected; change reverted (`git checkout --`) before committing,
      confirmed no diff left behind.
- [x] This repository's own `task verify` (root `Taskfile.yml`): full run
      green, including `fmt:check`, `typecheck`, `lint`, `test`,
      `mod:verify`, `security`, `workflows:lint` — no `audit`/`cmd/vibe`
      step, and no test regressions across `internal/module/{repotooling,
      tsrepotooling,pyrepotooling,ci/*}` (all still resolve their template
      byte-for-byte against the synced fixtures).
- [x] Filed issue #20 for the pre-existing, out-of-scope Go
      `go run ./cmd/vibe`-based `audit` bug, cross-referenced from spec
      0017's non-goals.

## Explicitly still deferred

- `vibe check`/`vibe doctor` implementation, `internal/affected`,
  `internal/validation` — no milestone has built these yet; increment 3
  only fixes the contract they'll need to satisfy once they exist.
- Fixing Go's `repotooling` `audit` task to use a `PATH`-resolved `vibe`
  instead of `go run ./cmd/vibe` for external `prod-go/v1` adopters —
  tracked as issue #20, not part of this spec.
- No change to spec 0013's "`vibe sync` never runs in CI" rule.
- No new standards, no module renames.
