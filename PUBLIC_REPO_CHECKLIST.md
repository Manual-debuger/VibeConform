# Public Repository Checklist

This repository is currently **private** on **GitHub Free**. The features
below are unavailable to private repositories on that plan (verified
against current GitHub documentation/pricing at bootstrap time) and are
tracked here so they get enabled the day this repository goes public,
instead of being silently forgotten.

## Blocked today by GitHub Free + private visibility

- [x] **Code scanning / CodeQL** — `.github/workflows/codeql.yml` is now in
      the repository (push/PR to `main` + weekly schedule, Go analysis).
      Still won't run successfully until the repository is public: on
      private repos without GitHub Code Security, the `analyze` step fails
      to upload results. No action needed after visibility flips other than
      watching the first run go green.
- [ ] **Secret scanning + push protection** — same constraint: free on
      public repos, requires GitHub Secret Protection on private repos.
      Enable via repository Settings → Code security once public (it is
      often auto-enabled by default for new public repos).
- [x] **Repository rules / rulesets (branch protection)** — **correction**:
      this was verified live against the actual repository on 2026-09-18
      and it is *not* Team-plan-gated as originally assumed. The classic
      branch protection API
      (`PUT /repos/{owner}/{repo}/branches/{branch}/protection`) works on
      Free-plan private repositories and is now configured on `main`:
      - [x] Require a pull request before merging (0 required approvals —
        solo project, see `docs/plans/0001-bootstrap.md`)
      - [x] Require the `CI / gate` status check to pass, branch must be
        up to date
      - [x] Block force pushes
      - [x] Block branch deletion
      - [x] Require linear history
      - `enforce_admins` is left `false` so the solo maintainer isn't
        locked out; revisit once there's more than one contributor.
      - Not yet verified: the newer *Rulesets* UI/API
        (`repos/{owner}/{repo}/rulesets`) may still be plan-gated for
        private repos — classic protection above already satisfies the
        requirements, so this hasn't been tested.
- [x] **Code owners** — `.github/CODEOWNERS` now exists (`@Manual-debuger`
      as sole owner; no required-review enforcement is configured, so this
      only records ownership).
- [ ] **Draft pull requests** — still gated to public repositories on the
      Free plan; no action needed, it becomes available automatically once
      public.

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
