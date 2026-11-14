# ADR 0027: Contract 1.2 Probes for Code-Quality Gates

## Status

Accepted (2026-11-14). Supersedes decision 12 of ADR 0025 ("one fixed
defect per language", the single lint floor). The rules themselves go in
a new spec written with `/to-spec`. Settled with the user on 2026-11-14
in a grilling session; "Qn" below refers to its questions.

## Context

The user asked for deterministic assurance of code quality in the
contract: code that is easy to read, easy to extend and secure. A
proposal listed metric thresholds (cyclomatic complexity ≤ 10, duplication
≤ 3 %, docstring coverage ≥ 80 %, mutation score ≥ 60 %, and so on),
ratchets on suppression comments and test counts, and protection of the
threshold files from agents.

Most of that conflicts with ADR 0025. The contract states properties,
never tools or configuration content (decision 1), and lint rule content
is the repository's choice (decision 12). The audit reads one checkout,
so it has no base revision to ratchet against. Deterministic metrics can
only show that code is not bad. They cannot show that it is good, and an
agent optimizes a proxy metric that becomes a target (Goodhart's law).

## Decision

1. **The contract checks that quality gates exist and catch defects. It
   does not check how good the code is** (Q1). New rules are probes with
   planted defects, as V-01 already is. The contract fixes the planted
   defect and never a threshold or a tool. The repository chooses its
   thresholds; any threshold that catches the planted defect satisfies
   the rule.
2. **Four new planted defects, one SHOULD rule each, in contract 1.2.0**
   (Q2, Q11, Q18). V-05 is a type error. V-06 is a function whose
   cyclomatic and cognitive complexity are both at least 40 (Q14). V-07
   is a secret: a PEM private-key block (Q16). V-08 is the V-01
   static-analysis defect hidden by every blanket suppression the
   language commonly uses (Q7). Per-kind rules let an adopter waive one
   kind.
3. **The new rules cost as few entrypoint runs as possible** (Q12,
   revised the same day for cost). Each rule probes `verify` only:
   CI runs `verify`, so it is the gate, and the references still put
   the checks in `verify-fast` for the agent's Stop hook. V-05 applies
   to Python components only, since in Go and TypeScript a type error
   is a compile error that V-01 already shows. V-07 is planted once
   per repository, since a secret does not depend on the language.
   This cuts the added runs from 8 to 3 for one Go component, and from
   24 to 8 for a three-language monorepo. The negative control that
   checks the samples themselves runs only when they or the references
   change, weekly, and on demand.
4. **This reverses ADR 0025 decision 12 for complexity.** That decision
   kept one lint floor because "any other lint defect might be
   legitimately allowed". No repository legitimately allows a function
   at complexity 40, and a blanket suppression only exists to switch off
   a gate. The contract still says nothing about lint rules below that.
5. **Specific suppressions stay the repository's choice** (Q13). An
   agent may silence one finding at one place by naming the rule. The
   setup skill's references require a written reason where the linter
   can enforce one (Go `nolintlint`, ESLint `eslint-comments`). Code
   that already exceeds the reference thresholds when a repository
   adopts them gets a specific suppression with the fixed reason
   `baseline: pre-existing complexity` (Q15), not a raised threshold.
6. **The guard refuses commands that skip Git hooks** (Q3, Q18). A new
   SHOULD rule, A-09, uses its own guard-corpus subset:
   `git commit --no-verify`, `git commit -n`, `git push --no-verify`,
   and `git -c core.hooksPath=…` with `commit` or `push`. Bypasses
   specific to one hook manager (`LEFTHOOK=0`, `HUSKY=0`) are
   `vibe-guard` built-in rules, not corpus cases. Adding deny cases to
   A-02's corpus would be MAJOR, so A-02 is unchanged.
7. **Out of the contract** (Q4, Q5): locking lint or test configuration
   behind an unlock, CODEOWNERS, duplication, docstring coverage, file
   length, coverage percentage, mutation score, and every ratchet that
   needs a base revision. Dependency direction differs too much between
   projects; it becomes an optional setup-skill reference (Q6).

## Consequences

- An agent can still lower a threshold in the repository's lint
  configuration, as long as the planted defect is still caught. PR
  review and CI catch that, not the contract. `docs/usage.md` lists it
  as a known gap (principle 1).
- An agent can still split a complex function into meaningless pieces.
  No deterministic check prevents it.
- V-07 runs in `verify`, so a secret is caught before merge, not before
  push. The setup skill's references also run the scanner in pre-commit,
  which the contract does not check.
- The planted private key is assembled at run time, so `vibe`'s own
  source holds no literal key for secret scanners to report.
- This repository and the examples satisfy every new rule in the
  release that adds them, without waivers (Q19). This repository also
  enforces its own dependency direction (`internal/contract` and
  `internal/guard` depend on `internal/guardrule`, never on each other)
  with `depguard` instead of prose in `AGENTS.md`.

## Alternatives considered

- **Threshold rules in the contract** (for example "cyclomatic
  complexity ≤ 10"). These dictate lint content and name tools.
- **Static recognition of linter configuration.** This reads
  configuration text, which ADR 0025 decision 3 replaced with probes.
- **Ratchets on suppression counts and test counts.** These need a base
  revision the audit does not have, and the baseline file would be one
  more file an agent can edit.
- **Mutation testing.** It is slow, and its result depends on what the
  repository's tests cover, so the same input does not give the same
  verdict across repositories.
- **Locking quality configuration files behind an unlock.**
  `pyproject.toml` and `package.json` also hold dependencies, so every
  dependency change would need a person.
