# Adopting VibeConform in a new repository

A start-to-finish walkthrough: from an empty directory to a repository
whose pull requests are gated by its own verification and by the
contract audit. Each step links to the part of [`usage.md`](usage.md)
that has the full detail.

The example is a Go service. The steps are the same for Python,
TypeScript and a monorepo; only the manifest and the tools differ. For
finished results, see the committed examples, which are assembled from
the setup skill's reference files: `examples/python`,
`examples/typescript`, `examples/monorepo` (Go, TypeScript and Python
components), and this repository itself (Go).

## 1. Prerequisites

You need these on `PATH`, on Windows or Linux:

| Tool | Why |
|---|---|
| `git` | everything; `vibe` reads the tracked files |
| `vibe` and `vibe-guard` | the audit, and the agent guard the setup skill installs (step 2) |
| an AI coding agent | runs the setup skill; Claude Code is the agent the contract's A rules cover |
| the project's own tools | whatever the entrypoints call. With the setup skill's references: [`task`](https://taskfile.dev), [`lefthook`](https://lefthook.dev), and `golangci-lint`, `goimports`, `govulncheck`, `actionlint` for Go; `node` and `pnpm` for TypeScript; [`uv`](https://docs.astral.sh/uv/) for Python |
| bash | Claude Code runs hook commands in it; Git for Windows' bash on Windows |

`vibe doctor` (step 6) checks the ones the repository's configuration
names, so you can install as you go.

## 2. Install `vibe` and `vibe-guard`, pinned

```sh
go install github.com/Manual-debuger/VibeConform/cmd/vibe@<version>
go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@<version>
vibe --version    # vibe version <version> (contract 1.1.0)
```

Without Go, download the release archives and check them against
`checksums.txt`; see [Installing](usage.md#installing).

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
and each has its own `PATH`. If `vibe --version` shows something else,
an older `vibe` comes first on `PATH`; fix that before going on.

## 3. Start the repository

`vibe` checks a repository; it does not create the project. Start the
way you normally would, and add the files to Git: `vibe` reads only
tracked files.

```sh
mkdir my-service && cd my-service
git init -b main
go mod init example.com/my-service
printf 'package main\n\nfunc main() {}\n' > main.go
printf 'bin/\n' > .gitignore
git add -A
```

## 4. Write `vibe.yaml`

```sh
vibe init
```

This writes a `vibe.yaml` version 2: the contract version, `platforms:
[linux, windows]`, `agents: [claude]`, the components found in the
tracked files (none listed here, since one Go component sits at the
root), and the four required entrypoints as comments. See
[`vibe init`](usage.md#vibe-init).

Until the entrypoints are declared, `vibe audit` fails rule V-04, and
every rule that needs an entrypoint fails with "fix V-04 first". The next
step fills them in. Edit `platforms` and `agents` now if they do not fit.

## 5. Let the setup skill set the repository up

```sh
vibe init --skill
```

This writes the setup skill, `vibeconform-setup`, to
`.claude/skills/vibeconform-setup/` and `.agents/skills/vibeconform-setup/`.
Then ask your agent to set up VibeConform. The skill:

- keeps the tools the repository already has, and uses the references
  (Task, lefthook, GitHub Actions, golangci-lint for Go) where it has
  none;
- declares the entrypoints in `vibe.yaml`, such as `verify: task
  verify`;
- writes what each rule needs: `Taskfile.yml`, `lefthook.yml`,
  `.github/workflows/ci.yml` (verify on Linux and Windows, a
  vulnerability scan), `.github/workflows/audit.yml` (the audit job,
  which installs `vibe` with the setup action), `dependabot.yml`, a pull
  request template, `AGENTS.md`, `CLAUDE.md`, and the agent hooks in
  `.claude/settings.json`;
- hands you a command that writes `.claude/hooks/policy.json`, because
  the guard refuses that file to the agent, and waits until you have run
  it;
- checks that `vibe-guard` is found by a non-interactive shell;
- runs `vibe audit` and fixes each finding until the verdict is
  `conformant`, or stops and tells you what needs a person.

See [The setup skill](usage.md#the-setup-skill). Everything it writes is
the repository's own, to edit as you like; the audit only checks the
properties.

## 6. Check the machine

```sh
vibe doctor
```

One line per check: git, `vibe.yaml`, the program of each entrypoint,
the bash that runs the hooks, and `vibe-guard` when a hook runs it. A
`FAIL` means something the workflow needs is missing here. Doctor
installs nothing and writes nothing. See
[`vibe doctor`](usage.md#vibe-doctor).

## 7. Verify, then audit

```sh
lefthook install   # register the Git hooks, once per clone
task verify        # your code against your own tooling; never needs vibe
vibe audit         # does the repository have the contract's properties?
```

`vibe audit` runs the static checks, then the execution checks and
probes in a throwaway copy of the repository: it runs `verify` with a
planted defect, the agent hooks, and the guard corpus through the
guard. It ends with `Verdict: conformant` and exit `0`. `vibe audit
--static` is the quick run, which never reaches a verdict. See
[`vibe audit`](usage.md#vibe-audit).

## 8. Commit

```sh
git add -A
git commit -m "chore: adopt the VibeConform contract"
```

The pre-commit hook runs on this commit. Then push.

## 9. Make the checks required

In the repository's branch protection (GitHub: Settings → Branches, or
a ruleset), require the CI jobs that verify (with the references,
`CI / verify (ubuntu-latest)`, `CI / verify (windows-latest)` and
`CI / security`) and the audit job (`Audit / audit`). Without the audit,
a change that breaks a rule merges unnoticed.

## Day to day

- **Every file is yours.** Change the Taskfile, the hooks, the CI or the
  lint rules directly. The audit fails only when a property is lost, and
  each finding names the file to change and the direction of the fix.
- **Guardrail settings need you.** The agent may not change
  `.claude/settings.json` or `.claude/hooks/` until you run `vibe unlock`
  in your own terminal, and never `.claude/hooks/policy.json`. See
  [`vibe unlock`](usage.md#vibe-unlock).
- **Upgrading VibeConform:** install the newer `vibe` and `vibe-guard`,
  run `vibe init --skill`, and ask the agent to upgrade. Within a contract
  MAJOR it bumps `contract:` in `vibe.yaml`, updates the pinned versions
  and runs the audit loop.

## Adopting an existing repository

The steps are the same. The setup skill keeps what already satisfies a
rule (your task runner, hook manager, CI layout and lint rules) and adds
only what is missing. A repository set up by the old template-sync
system (it has `.vibe/`, or a `vibe.yaml` with `standard:`) is migrated
first; see [Leaving the template-sync
system](usage.md#leaving-the-template-sync-system).

## Leaving

Nothing the repository runs calls `vibe`, so removing VibeConform is
deleting `vibe.yaml`, the audit job and the setup skill; see
[Removing VibeConform](usage.md#removing-vibeconform).
