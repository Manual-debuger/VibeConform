# Adopting VibeConform in a new repository

A start-to-finish walkthrough: from an empty directory to a repository
whose pull requests are gated by its own verification and by a
conformance check. Each step links to the part of
[`usage.md`](usage.md) that has the full detail.

The example uses `prod-go`. The [differences for other
standards](#other-standards) are at the end. For finished results,
see the committed examples: `examples/python` (`prod-py`),
`examples/typescript` (`prod-ts`), `examples/monorepo` (`prod-mono`),
and this repository itself (`prod-go`).

## 1. Prerequisites

You need these on `PATH`, on Windows or Linux:

| Tool | Why | Standards |
|---|---|---|
| `git` | everything | all |
| `go` | installing `vibe`; also `task audit`'s fallback (spec 0022) | all |
| [`task`](https://taskfile.dev) | every generated entry point (`task verify`, …) | all |
| [`lefthook`](https://lefthook.dev) | the Git hooks `vibe sync` registers | all |
| `golangci-lint`, `goimports`, `govulncheck`, `actionlint` | lint, format, security, workflow lint | `prod-go` |
| `node`, `pnpm` (10 or below) | every TypeScript task | `prod-ts` |
| [`uv`](https://docs.astral.sh/uv/) | every Python task | `prod-py` |

`vibe sync` warns about anything missing and `vibe doctor` checks it
(step 6), so you can install as you go.

## 2. Install `vibe`, pinned

```sh
go install github.com/Manual-debuger/VibeConform/cmd/vibe@v0.5.0-alpha.1
vibe --version    # vibe version v0.5.0-alpha.1
```

`go install` prints nothing when it succeeds. It puts the binary in
`$(go env GOBIN)`, or in `$(go env GOPATH)/bin` (usually `~/go/bin`)
when `GOBIN` is unset. If the shell then says `vibe: command not found`,
the install worked but that directory is not on `PATH`:

- **Linux and WSL** (bash; use `~/.zshrc` for zsh):

  ```sh
  echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bashrc
  source ~/.bashrc
  ```

- **Windows** (PowerShell). The Go installer usually adds
  `%USERPROFILE%\go\bin` already. If it did not:

  ```powershell
  $user = [Environment]::GetEnvironmentVariable('Path', 'User')
  [Environment]::SetEnvironmentVariable('Path', "$user;$(go env GOPATH)\bin", 'User')
  ```

  Then open a new terminal.

WSL and Windows are separate machines here. Each needs its own install,
and each has its own `PATH`.

If `vibe --version` shows something else, an older `vibe` comes first
on `PATH`. Fix that before going on: `task audit` uses whichever `vibe`
is on `PATH`, and an older one may not know options the newer one wrote.

Use a **released tag**, not a local build. `vibe sync` records the
version that wrote the files in `.vibe/state.yaml`, and CI's conformance
job installs exactly that version (`task audit`, see
[usage.md](usage.md#what-prod-gov1-manages)). A `dev` or `+dirty` build
records a version no module proxy serves, so `sync` warns:

```text
warning: vibe … is not a released or pseudo-version, so task audit
cannot pin it; CI's conformance check will fail unless vibe is on PATH.
```

Other ways to install are in [Installing](usage.md#installing).

## 3. Start the repository

`vibe` writes configuration around your code; it does not create the
project itself. Start the way you normally would:

```sh
mkdir my-service && cd my-service
git init -b main
go mod init example.com/my-service
printf 'package main\n\nfunc main() {}\n' > main.go
printf 'bin/\n' > .gitignore
```

Do this first, so that the `lefthook install` at the end of `vibe sync`
has a Git repository to register hooks in.

## 4. Declare the standard

```sh
vibe init prod-go v1
```

This writes a two-line `vibe.yaml`. Before syncing, decide which opt-ins
you want. Each one is optional, and each can be added or removed later
with one edit and one `vibe sync`:

```yaml
standard: prod-go
version: v1

# Optional integrations (usage.md, "Selecting integrations").
# Agents default to [claude, codex]; editors and intelligence to none.
integrations:
  agents: [claude]          # drop Codex's files
  editors: [vscode]         # owned tasks and recommendations in .vscode/
  intelligence: [graphify]  # needs graphify on PATH to be useful

# Optional repository policy (usage.md, "Line-ending policy"):
# pin LF everywhere. Recommended when anyone works on Windows.
policy:
  line_endings: lf

# Optional docs layout (usage.md, "Development workflow"): a short
# managed section in docs/README.md and docs/specs/README.md.
development:
  docs_layout: standard
```

There is no `workflow` key in this sample, and that is the recommended
setting: VibeConform chooses no development process, so you can use
your own. If you want VibeConform's bundled lightweight-SDD workflow,
add `workflow: plan-triggered-sdd` (or `always-sdd`, or `direct`). Each
value adds a managed section to `AGENTS.md`, so none of them is neutral.
See usage.md, "Development workflow".

`vibe.yaml` is decoded strictly: a misspelled key is an error, not
ignored. See [What `vibe.yaml` means today](usage.md#what-vibeyaml-means-today).

## 5. Preview, then sync

```sh
vibe diff     # what would be created; writes nothing
vibe sync
```

On a fresh repository every resource is `created` and the run ends with
lefthook registering the hooks:

```text
standard: prod-go/v1
.golangci.yml: created
.github/workflows/ci.yml: created
.github/workflows/conformance.yml: created
Taskfile.yml: created
lefthook.yml: created
.claude/settings.json: created
…
AGENTS.md (section workflow): created
18 created, 0 updated, 0 unchanged, 0 conflicts
lefthook: git hooks registered
```

What you now have is described in [What `prod-go/v1`
manages](usage.md#what-prod-gov1-manages). In short:

- `Taskfile.yml`: the canonical interface (`task verify`,
  `task verify:fast`, `task fmt`, …).
- `lefthook.yml`: pre-commit format and vet checks, and pre-push lint
  and test.
- CI: `ci.yml`, whose required check is `CI / gate`, and
  `conformance.yml`, whose check is `Conformance / audit`.
- Agent guardrails in `.claude/` (if `claude` is selected).
- `.vibe/state.yaml`: what `sync` wrote and with which version.

Files that stay yours, never generated: `go.mod`, your code, `.gitignore`
(apart from an integration's managed section), `AGENTS.md`/`CLAUDE.md`
(apart from one short managed section each), `Taskfile.local.yml` for
[your own tasks](usage.md#adding-your-own-tasks-taskfilelocalyml), and
`lefthook.local.yml` for
[your own Git hooks](usage.md#adding-your-own-git-hooks-lefthooklocalyml).

## 6. Check the machine

```sh
vibe doctor
```

One line per check, `PASS`/`WARN`/`FAIL`/`UNVERIFIED`. A `FAIL` means a
required local workflow cannot run here, usually a missing tool from step
1. Doctor installs nothing and writes nothing. See
[`vibe doctor`](usage.md#vibe-doctor).

## 7. Verify, then audit

```sh
task verify   # your code against the standard's tooling; never needs vibe
task audit    # are the managed files exactly what the standard says?
```

`task verify` must pass before the first commit, since the pre-push hook
and CI run the same checks. `task audit` prints `conformant` and exits 0.
If it fails with an error about an option in `vibe.yaml` (for example
`no code-intelligence providers are available yet`), it ran an older
`vibe` from `PATH`; see step 2.

## 8. Commit

Commit everything `sync` wrote, **including `.vibe/state.yaml`**. It is
what lets CI pin the `vibe` version and lets later syncs tell your edits
from template upgrades.

```sh
git add -A
git commit -m "chore: adopt VibeConform prod-go/v1"
```

The pre-commit hook runs on this commit. Then push.

## 9. Make the checks required

In the repository's branch protection (GitHub: Settings → Branches, or a
ruleset), require both:

- **`CI / gate`**: your verification, on every pull request.
- **`Conformance / audit`**: the managed files still match the standard.

Without the second one, a hand-edited managed file merges unnoticed.

## Day to day

- **Never hand-edit a managed file.** `vibe audit` lists them. A direct
  edit is drift: `audit` exits 2, and the next `sync` puts the file back.
  Put your own tasks in `Taskfile.local.yml`, your own Git hooks in
  `lefthook.local.yml`, and change behavior through `vibe.yaml`.
- **Changing what you opted into** is one edit to `vibe.yaml`, then
  `vibe diff`, then `vibe sync`, then a commit. Deselecting removes only
  what `sync` wrote and you left unmodified
  ([Deselecting](usage.md#deselecting)).
- **Upgrading VibeConform:** install the newer tag, then `vibe sync`.
  Review the diff: template changes, plus the new `vibe_version` in
  `.vibe/state.yaml`. CI keeps using the recorded version until you
  commit that.
- **A conflict** means a managed file changed in a way `sync` cannot
  account for. `sync` leaves it untouched and exits non-zero. Reconcile
  the file, or delete it and sync again.

## Adopting an existing repository

The steps are the same. The difference is that files you already have,
such as `Taskfile.yml` or `.github/workflows/ci.yml`, are not yet
recorded in state. When their content differs from the standard's,
`sync` reports them as **conflicts** and does not overwrite them. Move
anything you want to keep into `Taskfile.local.yml`, `lefthook.local.yml`
or your own workflow file, delete the conflicting file, and sync again. An existing docs tree
can be adopted in place with the `development.*_dir` keys (spec 0034,
[Development workflow](usage.md#development-workflow)).

## Other standards

- **`prod-ts`:** `vibe init prod-ts v1`. You need a `package.json` with a
  `packageManager: "pnpm@10.x"` field, eslint, prettier, and typescript
  as devDependencies, a committed `pnpm-lock.yaml`, and a `tsconfig.json`
  that extends the generated `tsconfig.base.json`. See [`prod-ts` uses
  pnpm](usage.md#prod-ts-uses-pnpm) and `examples/typescript`.
- **`prod-py`:** `vibe init prod-py v1`. You need a `pyproject.toml`
  whose dev dependency group has ruff, pyright, and pytest, and a
  committed `uv.lock`. See `examples/python`.
- **`prod-mono`:** add a `components:` list (`id`, `path`, `profile`)
  to `vibe.yaml`, which `init` does not write. See [What `prod-mono/v1`
  manages](usage.md#what-prod-monov1-manages) and `examples/monorepo`.

## Leaving

Everything generated keeps working without `vibe`. Removal is deleting
four things; see [Removing VibeConform](usage.md#removing-vibeconform).
