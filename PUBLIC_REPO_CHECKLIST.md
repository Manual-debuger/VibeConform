# Public Repository Checklist

This repository is currently **private** on **GitHub Free**. The features
below are unavailable to private repositories on that plan (verified
against current GitHub documentation/pricing at bootstrap time) and are
tracked here so they get enabled the day this repository goes public,
instead of being silently forgotten.

## Blocked today by GitHub Free + private visibility

- [ ] **Code scanning / CodeQL** — free on public repositories; on private
      repositories it requires purchasing GitHub Code Security (Team/
      Enterprise). Enable a `codeql-analysis` workflow once public.
- [ ] **Secret scanning + push protection** — same constraint: free on
      public repos, requires GitHub Secret Protection on private repos.
      Enable via repository Settings → Code security once public (it is
      often auto-enabled by default for new public repos).
- [ ] **Repository rules / rulesets (branch protection)** — the Free plan's
      rule enforcement applies to public repositories only; private repos
      need at least the Team plan. Once public (or upgraded), configure a
      ruleset on `main`:
      - [ ] Require a pull request before merging
      - [ ] Require the `CI / gate` status check to pass
      - [ ] Require branches to be up to date before merging
      - [ ] Block force pushes
      - [ ] Block branch deletion
      - [ ] Require linear history
      - Do **not** require additional human approvals beyond 0 while this
        remains a solo project (see `docs/plans/0001-bootstrap.md`).
- [ ] **Draft pull requests / code owners** — also gated to public
      repositories on the Free plan; revisit `CODEOWNERS` once public.

## Already available today (no action needed later)

- Dependabot alerts, dependency graph, and version updates
  (`.github/dependabot.yml`) — included on all plans regardless of
  visibility.
- GitHub Actions CI/CD — available on private repos, subject to the Free
  plan's monthly Actions-minutes quota (private repos consume minutes;
  public repos do not).

## How to action this later

When the repository visibility changes to public (Settings → General →
Danger Zone → Change visibility), work through the unchecked boxes above
in order, then delete this file (its purpose is to survive the interval
between "documented" and "enabled," not to be permanent repository
scaffolding).
