# Plan 0039: Tool paths in `vibe doctor`, Graphify coexistence

Implements `docs/specs/0039-doctor-paths-graphify-coexistence.md`
(issue #56). Status: approved and implemented. Approved together with
the session plan for specs 0036–0038 on 2026-10-02.

Branch: `feat/56-doctor-graphify`. One pull request.

## Steps

1. Write the spec, this plan, and ADR 0020 (which amends ADR 0016).
2. **Doctor.**
   - In `internal/doctor/doctor.go`, `tool` keeps the path beside the
     version, and `Git` reports the path of `git`.
   - In `internal/doctor/environment.go`, `AgentHooks` names each
     binary's path.
   - Update the exact-detail tests in `doctor_test.go` and
     `environment_test.go`. Check the substring assertions in
     `internal/cli`.
3. **The `.gitignore` section.**
   - `claude-config` (`internal/module/agents/claude/claude.go`)
     resolves a managed section `claude` of `.gitignore`. It mirrors
     `graphify`'s section: `#` markers, `Bottom` placement, and a
     `Managed by VibeConform` comment line.
   - Tests cover the claude module, the standard wiring (only `claude`
     adds it), and CLI sections: user lines kept, file created, section
     removed on deselect.
4. **Sync.**
   - Rebuild from a clean commit and sync the root and the examples.
   - Remove this repository's hand-written ignore line in the same
     commit as its sync.
5. **Docs.**
   - `docs/usage.md`: the doctor sample and wording, the Graphify
     coexistence layout, and the `.gitignore` line in "What `prod-go/v1`
     manages".
   - Spec 0028's sample note.
   - Spec 0035: a pointer to the layout.

## Impact

| Area | Change |
|---|---|
| `internal/doctor` | detail strings only |
| `internal/module/agents/claude` | one new resource (a section) |
| Generated output | every repository selecting `claude` gets a new `.gitignore` section; `audit` reports it missing until the next sync |
| Dependencies | none |

## Verification

- `task verify` and `task audit`.
- `vibe audit` for the root and each example.
- `vibe doctor` in this repository, with the output inspected by hand.
