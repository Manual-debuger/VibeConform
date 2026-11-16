# ADR 0028: Rule Tests Go Through `contract.Audit`, Not the CLI

## Status

Accepted (2026-11-16). Supersedes the first bullet of spec 0052's
"Testing Decisions" ("One seam: the `vibe audit` command, in-process").
Decided in design review #167 (issue #165) and carried out by spec 0065
together with issue #162.

## Context

Spec 0052 chose one test seam for the contract audit: the `vibe audit`
command, run in-process through cobra. Tests point `--repo-root` at a
fixture and assert the exit code, the rule results in `--json` and, for
whole reports, golden text. No test calls a check function directly.

That seam kept the tests honest, but by 2026-11 it costs more than it
gives:

- `internal/contract` has about 11,600 production lines and 1,300 test
  lines of its own. About 4,850 lines of rule tests live in
  `internal/cli` (`contract_*_test.go`).
- The CLI carries seams that exist only for those tests: the rule list,
  a hook that sees every report, the probe environment and the
  repository opener. The hook checks a contract invariant (every
  failure names a file and a fix) from the CLI.
- The JSON report is mirrored three times in test code. A change to the
  JSON shape ripples into rule tests that do not care about JSON.
- Every rule case pays for cobra and for a JSON round trip.

Since spec 0064, `contract.Audit` with `Options.Rules` is already the
seam of the runner tests, the guard probe tests and
`internal/contract/probetest`. It is a public interface of the package,
not a check function.

## Decision

1. **Rule behaviour is tested through `contract.Load` and
   `contract.Audit`**, in package `contract_test`, and the tests assert
   on `*contract.Report`. This covers rule results, findings, fixes,
   waivers, verdicts, configuration errors and the probes. Still, no
   test calls a check function directly.
2. **The CLI keeps a thin suite**: flags and their conflicts, exit
   codes, the text the CLI adds to `contract.Load`'s errors, and the
   golden text and JSON reports. The JSON report is mirrored once, in
   that suite.
3. **The report invariant moves with the rule tests.** The helper that
   every rule test runs the audit through checks each report: every
   failed part has a finding, and every finding names a file and a fix.
4. **Fixtures are shared through one helper package** that both suites
   import, so the fixture builder exists once.
5. **The CLI's seams shrink** to what its own suite needs. The rule
   list, the report hook and the probe environment leave the CLI.

## Consequences

- A rule test reads the report model, so a change to the JSON shape
  touches only the CLI suite and its goldens.
- Rule tests no longer prove that the CLI wires a rule through to the
  output. The golden reports and the JSON golden in the CLI suite cover
  that wiring for the whole report at once.
- `contract.Report` becomes the interface the tests depend on. Renaming
  a report field is now a test change in `internal/contract`, where
  the field lives.
- Spec 0052's "Testing Decisions" is amended to point here.
