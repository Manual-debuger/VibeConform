# Upgrading to a newer `vibe` or contract

## 1. Refresh the skill

The person installs the new `vibe` and runs `vibe init --skill`. It
rewrites `.claude/skills/vibeconform-setup/` and
`.agents/skills/vibeconform-setup/` and changes nothing else. Reload this
skill afterwards: the files you read before may have changed.

## 2. Compare the contract versions

- The repository's version is `contract:` in `vibe.yaml`.
- The new version is in the output of `vibe --version`:

  ```text
  vibe version <version> (contract <X.Y.Z>)
  ```

Compare the MAJOR parts (the X).

## 3a. Same MAJOR

1. Set `contract:` in `vibe.yaml` to the new version.
2. Update `vibe-guard` to the version of the new `vibe` (`SKILL.md`,
   step 5).
3. Update the setup action in the audit workflow: its `version` input,
   and its pinned SHA to the commit of the new release tag. Never use
   `latest`: the action refuses it. When the new `vibe` is not a release
   (`vibe --version` shows a pseudo-version or `dev`, and this skill
   names `latest`), there is no tag for it: keep the action on the
   newest release tag that
   `git ls-remote --tags https://github.com/Manual-debuger/VibeConform`
   lists, and install `vibe-guard` from the same commit as `vibe`.
4. Run the audit loop (`SKILL.md`, step 6). A new MINOR may add SHOULD
   rules; see `reference/rules.md`. From 1.1 to 1.2, also do
   section 4 below.

## 3b. New MAJOR

A new MAJOR may tighten a rule, add a MUST rule or add a guard corpus
case, and it may need new guardrail settings.

1. Read `reference/rules.md` for what changed, and tell the person.
2. Before you change `.claude/settings.json` or anything under
   `.claude/hooks/`, ask the person to run `vibe unlock` in their own
   terminal, and wait until they say it is done. Never run it yourself.
   The guard refuses those edits until then.
3. Update `vibe-guard`, then make the guardrail changes.
4. Set `contract:` in `vibe.yaml` to the new version, and update the
   setup action as in 3a.
5. Run the audit loop.

Your next Stop locks the guardrails again (`vibe-guard relock`).

## 4. From contract 1.1 to 1.2

1.2.0 adds SHOULD rules only, so the verdict does not change. To pass
them, as `reference/rules.md` describes, bring the repository's
configuration in line with the references. Each rule probes `verify`
only, but put each new check in `verify-fast` too, so that the agent's
Stop hook reports it.

- **V-05, type error (Python components only).** Run a type checker in
  `verify`: the `typecheck` task of `reference/taskrunner/python/`, with
  `reference/lint/python/pyrightconfig.json` (`standard` mode). Fix the
  errors it reports, or give each one a specific suppression with a
  reason (`# pyright: ignore[reportReturnType]  # reason`). A repository
  without a Python component skips this step.
- **V-06, complexity.** Add the complexity limits of the lint reference
  for each language (`reference/rules.md`, V-06). Give each function
  that already exceeds them a specific suppression with the reason
  `baseline: pre-existing complexity`, for example
  `//nolint:gocognit // baseline: pre-existing complexity`,
  `# noqa: C901  # baseline: pre-existing complexity` or
  `// eslint-disable-next-line complexity -- baseline: pre-existing complexity`.
  Do not raise a threshold to make old code pass. New code meets the
  limits.
- **V-07, secrets.** Add the `secrets` and `secrets:staged` tasks from
  `reference/taskrunner/<stack>/`, `secrets` to `verify` and
  `verify:fast` (and to `vibe.yaml`'s `verify` when it lists commands),
  the hook reference's pre-commit `secrets` command,
  `reference/secrets/.gitleaks.toml` at the root, and the gitleaks
  install step from `reference/ci/<stack>/` to every CI job that runs
  `verify` or `vibe audit`. Then run `task secrets`. A finding that is a
  real secret is rotated first, then removed; tell the person, since
  the old value stays in the Git history. A finding that is not a secret
  (a test fixture) goes into the configuration's allowlist with a
  reason.
- **V-08, blanket suppressions.** Add the suppression rules of the lint
  reference for each language (`reference/rules.md`, V-08): Go
  `nolintlint` with `require-specific` and `require-explanation`; Ruff
  `PGH003` and `PGH004`; ESLint the `eslint-comments` rules and
  `linterOptions.reportUnusedDisableDirectives: 'error'`. Then run
  `task lint`, and replace each blanket suppression it reports with a
  specific one that names the rule and gives a reason. Remove a
  suppression that hides nothing.
- **A-09, hook bypass.** Update `vibe-guard` (3a, step 2): from contract
  1.2.0 it refuses `git commit --no-verify`, `git commit -n`,
  `git push --no-verify` and `git -c core.hooksPath=…` with `commit` or
  `push`. A guard of the repository's own must refuse the same commands
  and allow `git push -n`. Add an exception for `git-skip-hooks` only
  when the person asks for one.
