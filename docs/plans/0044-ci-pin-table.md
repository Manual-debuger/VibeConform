# Plan 0044: One pin table for generated CI, and a GitLab schema check

Implements `docs/specs/0044-ci-pin-table.md`.
Status: draft, awaiting approval.

Branch: `feat/0044-ci-pin-table`. One pull request.

## Design

### The table: `internal/module/ci/pins`

The package depends only on the standard library. It holds:

```go
// Table is every toolchain version a generated CI file names (spec 0044).
//
// Bump practice: when Go is bumped, bump the tool pins (GolangciLint,
// Goimports, Govulncheck, Actionlint) in the same change, to versions that
// build with that Go. Task and TaskSHA256 change together. Stale-pin
// detection: issue #75.
type Table struct {
	Go, Task, TaskSHA256, GolangciLint string
	Goimports, Govulncheck, Actionlint string
	Node, Python                       string
}

// Current is the table the generated CI uses. Tests may swap it.
var Current = Table{...}

// Parse parses a CI template with [[ ]] delimiters, which leave GitHub's
// ${{ }} and GitLab's ${VAR} untouched.
func Parse(name, src string) *template.Template

// Render executes t with data and wraps errors with the template name.
func Render(t *template.Template, data any) ([]byte, error)
```

- `Goimports` holds the `golang.org/x/tools` module version, since
  goimports is installed from that module.
- **Values are read when `Resolve` runs, never at package init.** This
  lets a test swap `pins.Current` and see every file change (acceptance
  item "Rendered agreement").
- **Dependency direction.** The `ci/*` modules and `conformance` import
  `ci/pins`. Nothing under `internal/module` imports a CI module except
  `githubmono` and `gitlabmono` (they import `github`, for the PR
  template), so this adds no cycle. `ci/pins` does not import `module`
  or `resource`.

### Templates

All of these are rendered with `pins.Parse`, and get the table as
`.Pins`:

| Module | Template change |
|---|---|
| `ci/github`, `ci/githubpy`, `ci/githubts` | `templates/ci.yml` → `ci.yml.tmpl`. `dependabot.yml` and the PR template stay static: they hold no pins. |
| `conformance` | `conformance.yml` → `.tmpl` and `gitlab-ci.vibe.yml` → `.tmpl`. `golang:1.27.0` becomes `golang:[[.Pins.Go]]` and renders the same bytes. `Taskfile.vibe.yml` stays static. |
| `ci/githubmono`, `ci/gitlabmono` | The existing data structs gain `Pins pins.Table`. Their local `[[ ]]` parse and render helpers are replaced by `pins.Parse` and `pins.Render`. |

- A literal value becomes `"[[.Pins.Go]]"` and similar. The surrounding
  YAML, comments and `${{ env.GO_VERSION }}` references stay as they
  are, so the output bytes stay the same.
- `@latest` becomes:
  - `golang.org/x/tools/cmd/goimports@[[.Pins.Goimports]]`
  - `golang.org/x/vuln/cmd/govulncheck@[[.Pins.Govulncheck]]`
  - `github.com/rhysd/actionlint/cmd/actionlint@[[.Pins.Actionlint]]`

  These are written literally into the `run:` / `script:` line, with no
  new `env:` variable, so the only lines that change are the `@latest`
  ones.
- **Checked before converting:** none of these five templates contains
  `[[` (grep, 2026-10-08), so the delimiter never collides with literal
  text.
- **Static modules render in `Resolve`.** For example, `github.Resolve`
  returns the rendered `ci.yml`. It already returns an error, so a render
  failure goes through it rather than panicking.

### Choosing the three new pins

At implementation time, for each of `golang.org/x/tools`,
`golang.org/x/vuln` and `github.com/rhysd/actionlint`:
1. Take the latest release from `go list -m <module>@latest`.
2. Run `go install …@<version>` with Go `1.27.0` to confirm it builds
   with the pinned Go.
3. Record the versions in the PR.

### Tests

| Test | Where | What it checks |
|---|---|---|
| `TestTemplatesHoldNoPins` | `ci/pins` external test package | Reads the template sources from disk: every `templates/*` under `internal/module/ci/*` and `internal/module/conformance`. Fails if a source contains any non-empty `pins.Current` value, or `@latest`. |
| `TestRenderedPins` | same | Resolves the CI and conformance modules for each case: `github`, `githubpy` and `githubts` standalone; `githubmono` and `gitlabmono` with the `examples/monorepo` components; `conformance` under `github` and under `gitlab`. Fails if any output contains `@latest` or lacks the table's values. Then swaps `pins.Current` to sentinel values (with `t.Cleanup`, not parallel) and fails if any old value remains or a sentinel is missing. |
| GitLab golden | `ci/gitlabmono` | `.gitlab-ci.yml` and `.gitlab-ci.vibe.yml` for the `examples/monorepo` components, committed under `testdata/` with an `-update` flag. No committed copy of the GitLab output exists anywhere else, so this is what proves its shape is unchanged. Specs 2, 3 and 5 will review their output changes through this golden's diff. |
| Removed | `ci/gitlabmono` | `TestVersionsMatchGitHub`, replaced by `TestRenderedPins`. |
| Updated | `ci/github`, `conformance` (and the py/ts equivalents, if any) | `TestTemplatesAreLF` checks template sources. `TestTemplatesMatchLiveFiles` compares the rendered output, not the embedded bytes, with this repository's live files. |

**Proof that the shape is unchanged:**
- The GitLab golden is committed **first**, from the current code. Its
  diff in the PR then shows only the `@latest` lines changing.
- For GitHub output: this repository's `.github/workflows/ci.yml` and
  `conformance.yml`, and the `examples/*` files, are synced copies. Their
  PR diff shows the same, and `TestTemplatesMatchLiveFiles` together
  with `task audit` keep them in step.

### CI: schema check

`.github/workflows/examples.yml` is project-owned (not in
`.vibe/state.yaml`) and is edited directly. In the `monorepo-gitlab`
job, after the "Copy the example and switch it to GitLab" step, a new
step runs:

```yaml
- name: GitLab CI schema check
  working-directory: ${{ env.COPY }}
  # Keywords and value shapes only. It does not check include/extends/rules
  # resolution or a real run (issue #74).
  run: uvx check-jsonschema@0.38.2 --builtin-schema vendor.gitlab-ci --data-transform gitlab-ci .gitlab-ci.yml .gitlab-ci.vibe.yml
```

The job already installs `uv` (`astral-sh/setup-uv`).

## Commits

1. **Baseline GitLab golden.**
   - golden test and `testdata/` from the current templates
   - the `examples.yml` schema-check step
   - Commit `GLOSSARY.md` here too.
2. **Pin table.**
   - `ci/pins`, the template conversions and the new tests; remove
     `TestVersionsMatchGitHub`
   - No output change yet: `@latest` stays, with `TestRenderedPins`'s
     `@latest` assertion added in commit 3.
3. **Pin the three tools.**
   - table entries, template lines, the `@latest` assertion
   - golden update (diff: three lines × two templates)
4. **Sync.**
   - Rebuild vibe from a clean worktree of the branch (memory: a stale
     binary syncs stale templates).
   - `vibe sync` this repository and `examples/python`,
     `examples/typescript` and `examples/monorepo`.
   - Commit the files and `.vibe/state.yaml`.
5. **Docs.**
   - the `docs/usage.md` "CI provider" note
   - spec status set to implemented, plan status set to implemented

## Repository impact

| Area | Files |
|---|---|
| New | `internal/module/ci/pins/pins.go`, `pins_test.go`; `internal/module/ci/gitlabmono/testdata/*` |
| Renamed and templated | `ci/github/templates/ci.yml`, `ci/githubpy/templates/ci.yml`, `ci/githubts/templates/ci.yml`, `conformance/templates/conformance.yml`, `conformance/templates/gitlab-ci.vibe.yml` |
| Changed Go | the five modules' `.go` files and their template tests |
| Synced output (only the `@latest` lines change) | this repository's `.github/workflows/ci.yml`; `examples/monorepo/.github/workflows/ci.yml`; `.vibe/state.yaml` hashes |
| Unchanged output (Go-only tools) | `examples/python` and `examples/typescript` CI. Their sync should be a no-op, and that is checked. |
| Hand-edited | `.github/workflows/examples.yml` (one step), `docs/usage.md`, `GLOSSARY.md` (new) |
| Not touched | Taskfiles, `doctor`, `standard`, `manifest`, CLI |

**Risk.** Adopting repositories get the pinned lines on their next
`vibe sync`, as a moved standard (spec 0019). No toolchain changes. The
one behaviour change: a newer goimports, govulncheck or actionlint
release no longer reaches their CI until a VibeConform release.

## Verification ledger (to fill in)

- `task verify:fast` while working; `task verify`
- `task audit` (repository, after sync)
- unit tests: `TestTemplatesHoldNoPins`, `TestRenderedPins`, GitLab
  golden, live-file tests
- negative schema check by hand (misspelled keyword fails), recorded in
  the PR
- CI on the PR: `CI`, `Conformance / audit`, `examples` (including
  `monorepo-gitlab`'s schema step)
- real GitLab run: UNVERIFIED, issue #74
