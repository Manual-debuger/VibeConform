<!-- vibeconform:begin workflow -->
## Repository workflow

Managed by VibeConform from `development:` in `vibe.yaml`. This section
routes; the documents and tasks it names hold the detail.

Knowledge:
- Specs (what must be true) live in `docs/specs/`, architecture (how it
  works now) in `docs/architecture/`, decisions (ADRs) in
  `docs/decisions/`. Read the relevant ones before a non-trivial change.
- If an approved spec, an ADR and the code disagree, say so. Do not
  pick one silently.

Workflow: plan-triggered lightweight SDD.
- Normal mode: implement, then verify. Respect any spec that applies.
- Planning context (the harness's plan mode, or `/spec`): list the
  constraints that apply and the assumptions you have not verified,
  then write a lightweight spec with acceptance criteria. Plan only
  after that.
- The spec says WHAT must be true; the plan says HOW to change the
  repository. Keep them apart. Reuse an approved spec when one exists.
- In a read-only plan mode, put the spec in the plan. Once it is
  approved, write it to `docs/specs/` first.
- Do not implement until the user approves.

Verification:
- `task verify:fast` while working; `task verify` before declaring
  done. Do not weaken a test, lint or type check to make a change pass.
- Finish with a ledger, one line per check: PASS, FAIL or UNVERIFIED.
  Unit tests, CI and a real integration are separate lines.
<!-- vibeconform:end workflow -->
