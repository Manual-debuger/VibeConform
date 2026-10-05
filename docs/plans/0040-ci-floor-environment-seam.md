# Plan 0040: The CI floor, and an environment seam for GitLab jobs

Implements `docs/specs/0040-ci-floor-environment-seam.md` (issue #65).
Status: approved 2026-10-05 and implemented.

Branch: `feat/65-ci-environment-seam`. One pull request.

## Design

**The shape check is a module interface, modelled on
`module.SectionChecker`.** Modules stay pure: `buildPlan` keeps all
file I/O, as it does today.

```go
// ProjectFileGuard is implemented by a module that opens a seam for a
// project-owned file and limits what that file may hold. VibeConform
// never writes the file; it only checks it (docs/specs/0040-…).
type ProjectFileGuard interface {
	GuardedFiles(mctx *Context) []GuardedFile
}

type GuardedFile struct {
	Path  string                        // slash-separated, repo-relative
	Check func(content []byte) []string // problems; none means conformant
}
```

- `gitlab-ci-mono` implements it. Under `ci.provider: gitlab` it returns
  `.gitlab-ci.defaults.yml`. Under any other provider it returns nothing,
  so `github` and `none` never read the file.
- `Check` decodes with `yaml.v3`. A problem is reported when:
  - the file isn't valid YAML;
  - it holds more than one YAML document;
  - the first document isn't a mapping (an empty file counts);
  - it has a top-level key other than `default`. Each such key is named,
    in file order.
- `buildPlan` adds `Guards []guardPlan{Path, Problems}` to `repoPlan`.
  An absent file adds no entry, so the output doesn't change for any
  repository without the file.
- **audit** prints `path: ok`, or one
  `path: conflict: <problem>` line per problem. The file counts as
  checked, and each failing file as one conflict, so audit exits 2.
- **diff and sync** print the same lines. Sync counts a failing file as
  a conflict (non-zero exit) but still applies every other resource, as
  it does for a structured-patch conflict (`sync.go`
  `applyPatchResource`). Sync never writes the file.

## Steps

1. **Docs first.** Write the spec (done), this plan, and ADR 0021. The
   ADR covers the floor/environment split, the `default:`-only include
   with the audit shape check, and the documented gaps. It amends ADR
   0019 decision 4. Commit them.
2. **Template** (`internal/module/ci/gitlabmono/templates/gitlab-ci.yml.tmpl`):
   - top-level `VIBE_CACHE_POLICY: "pull-push"`;
   - the opt-in `workflow:rules` entry before the plain merge request
     rule;
   - `policy: $VIBE_CACHE_POLICY` on each component job's `cache:`;
   - the `.gitlab-ci.defaults.yml` include with `rules: exists`, after
     the vibe include;
   - `inherit: default: false` on `local`;
   - the header comment, which names the new seam and the floor.
   `conformance:audit` needs no change: it inherits project defaults
   (for example runner tags), and all its contract keys are already
   declared.
3. **Interface and check.** Add `ProjectFileGuard` and `GuardedFile` to
   `internal/module/module.go`. Implement them in `gitlabmono.go`, with
   a `DefaultsPath` constant next to `LocalPath`.
4. **CLI wiring.** `guardPlan` in `plan.go`, filled in `buildPlan` from
   each module that implements the interface. Then the output lines and
   counting in `audit.go`, `diff.go` and `sync.go`.
5. **Tests.**
   - `gitlabmono_test.go`:
     - update `TestPipelineStructure` (two includes now, `inherit` on
       `local`);
     - add a contract-key test over every component job and
       `conformance:audit`;
     - add the variable default and per-job policy;
     - add the workflow rule order, checking the other rules are
       unchanged;
     - add table tests of `Check`: default only, an extra job,
       `variables`, `include`, a non-mapping, empty, invalid YAML, two
       documents;
     - check `GuardedFiles` is empty under `github` and `none`.
   - `internal/cli` (beside `ciprovider_test.go`), end to end in a temp
     `prod-mono` + `gitlab` repository:
     - absent and `default:`-only files are conformant;
     - an extra key gives exit 2, with the key named in the output;
     - sync reports the conflict, leaves the file byte for byte
       unchanged, and still writes the other resources;
     - under `github`, a bad file is ignored.
6. **Docs.**
   - `docs/usage.md`, "CI provider": the floor, the defaults file with a
     `tags:` example, the two variables, and the protected-cache caveat
     (including the Maintainer/Developer difference). Add the file to
     the GitLab file table as project-owned.
   - Spec 0038 §4: one sentence pointing to spec 0040 for the seam.
   - `docs/architecture/overview.md`: the new interface next to
     `SectionChecker`, and the seam next to ADR 0009/0019.
7. **Sync and release hygiene.** Rebuild and sync the root and the
   examples. No generated file changes, since this repository and the
   examples use GitHub. Commit only if a managed file changed. If one
   did, use the pinned-version two-commit flow from #64.

## Impact

| Area | Change |
|---|---|
| `internal/module` | one optional interface, no change to existing modules |
| `internal/module/ci/gitlabmono` | template, `GuardedFiles`, the check |
| `internal/cli` | plan, audit, diff, sync: one new kind of finding |
| Generated output | `gitlab` repositories only: `.gitlab-ci.yml` changes; the next sync updates it, and audit reports it out of date until then |
| `github` / `none` | byte-identical; the file is never read |
| Dependencies | none (`yaml.v3` is already used) |

## Verification

- New tests fail before the change and pass after it.
- `task verify:fast` while working; `task verify` and `task audit`
  before declaring done.
- `vibe audit` on the root and each example.
- A scratch `examples/monorepo` copy with `ci.provider: gitlab`:
  - sync, then audit: conformant;
  - add a bad `.gitlab-ci.defaults.yml`: audit exits 2;
  - add a good one: conformant again.
- Real GitLab pipeline (spec acceptance "Real integration"): UNVERIFIED
  unless a GitLab project and runner are available. It is a separate
  ledger line, as in spec 0038.
