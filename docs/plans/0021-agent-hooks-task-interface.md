# Plan 0021: Agent hooks behind a Task interface

See `docs/specs/0021-agent-hooks-task-interface.md` for the accepted
scope.

Branch: `feature/agent-hooks-task`, one pull request. **Sequenced after
plan 0020.** Both change `ts-repo-tooling`'s `Taskfile.yml` template, so
this branch merges `main` once PR 0020 has landed, before C3, rather than
resolving the same template twice.

## Resolved open questions

Checked against current documentation on 2026-09-23:

- **Codex's deny protocol.** Codex accepts exit `2` with the reason on
  stderr, the same as Claude Code
  (`learn.chatgpt.com/docs/hooks`). One guard serves both agents with no
  per-agent output.
- **Claude Code on exit 2.** It blocks whatever JSON is printed, and uses
  stderr as the reason (`code.claude.com/docs/en/hooks`). The guard prints
  nothing on stdout.
- **Working directory.** Claude Code runs hooks in the session's *current*
  directory, which can be a subdirectory. Codex uses the session `cwd`.
  `task` searches upward for `Taskfile.yml`, so both work. This confirms
  that the spec's "nearest Taskfile wins" limitation is real.
- **Which shell Claude Code uses on Windows.** Bash, or PowerShell when
  Git Bash is absent (`shell` field default). `task -x hook:guard` parses
  the same in both.

Not yet resolved, measured during C3:

- Task's trailing stderr line after a deny, and how it reads.
- The cost of `go run` (first compile, then cached) and of `uv run
  --no-project python` through Task, and whether `uv run --no-project`
  leaves `.venv` alone.

### New finding: the shipped Codex `hooks.json` doesn't match the schema

The documented schema nests handlers:

```json
{"hooks": {"PreToolUse": [{"matcher": "…", "hooks": [{"type": "command", "command": "…"}]}]}}
```

`agent-config` ships a flat entry instead:

```json
{"hooks": {"PreToolUse": [{"command": "…", "commandWindows": "…"}]}}
```

Codex may never have run the guard at all. C2 writes the documented
shape. The PR calls this out as a fix, not just a refactor, and the plan
records whether the old shape ever fired, if that can be established.

## Order of work

Five commits, each leaving the tree green.

1. **C1: policy table, subset check, corpus.** `internal/module/agents`
   gains the Go policy table and a `policy.json` renderer. It also gets
   the regular-expression subset test and a `testdata` corpus. Nothing is
   wired to agents yet.
2. **C2: `agent-config` switches to Task.** New `settings.json` and
   `hooks.json`, `policy.json` emitted, `.sh` resources dropped, `hookMode`
   removed. The cross-module wiring test goes in `internal/standard`, and
   **fails** until C3. So C2 and C3 are committed together if needed to
   keep the tree green, or C3 goes first.
3. **C3: guards and `hook:guard`.** `guard.go`, `guard.mjs`, and
   `guard.py`, each with its repo-tooling module's task. Rebuild, then
   sync all three roots.
4. **C4: end-to-end corpus in CI.** A hand-authored workflow runs the
   corpus through `task -x hook:guard` on Ubuntu and Windows, for each
   standard.
5. **C5: docs and cleanup.** Delete the stale `.sh` files (by hand, since
   `sync` never deletes). Update `docs/usage.md`, `AGENTS.md` Guardrails,
   `.codex/README.md`, and add a note to ADR-0006.

Given the wiring test, the actual commit order is **C1, C3, C2, C4, C5**:
guards and tasks exist before the agent configs point at them. That also
means no intermediate commit has agent configs calling a task that doesn't
exist, which would make this repository's own guard fail open mid-branch.

## Checklist

### C1: policy table, subset check, corpus

- [x] `internal/module/agents/policy.go`: the `Rule` type (`ID`, `Tool`
      — `command` or `file_path` — `Pattern`, and `Message`) and the
      `Policy` table, ported one-for-one from `block-dangerous.sh` (six
      rules) and `block-secret-files.sh` (one rule).
- [x] Port every POSIX bracket class (`[[:space:]]`) to `\s`: Python `re`
      and JavaScript `RegExp` have no POSIX classes.
- [x] `renderPolicy()` produces deterministic JSON (fixed field order,
      two-space indent, trailing newline, LF), under a top-level
      `"version": 1`.
- [x] `policy_test.go`:
  - [x] `TestPolicyPatternsInPortableSubset` rejects lookaround,
        backreferences, possessive or atomic groups, named groups, POSIX
        classes, `\A`/`\z`, and inline flags other than `(?i)`. Each
        pattern must also compile with Go's `regexp`.
  - [x] `TestRenderPolicyDeterministic`.
  - [x] `TestPolicyIsFaithfulPort`: every `deny()` message from the old
        scripts appears in exactly one rule. This stops the port from
        silently dropping a rule.
- [x] `internal/module/agents/testdata/guard_corpus.json`: cases of
      `{name, payload, want: "deny"|"allow"}`. It covers:
  - [x] each rule, positive and near-miss;
  - [x] the known false positive (a command that only *quotes* a
        dangerous one inside a string argument), expected **allow**
        because the match runs on the parsed command. Note that a heredoc
        body is still part of `command` and still matches: recorded as
        **deny**, with a comment pointing at the spec's limitation;
  - [x] malformed JSON containing a dangerous string, expected **deny**
        through the raw fallback;
  - [x] an unknown `tool_name`, expected raw matching;
  - [x] a Codex-shaped payload (`tool_name: "Bash"`) and Claude-shaped
        `Bash`, `PowerShell`, `Write`, and `Edit` payloads.

### C3: guards and `hook:guard` (committed before C2)

Guard contract, identical in all three runtimes:

1. Read all of stdin.
2. Parse it as JSON. If that fails, match every rule against the raw text.
3. Otherwise, map `tool_name`: `Bash` and `PowerShell` mean `command`,
   checked against `tool_input.command`; `Write` and `Edit` mean
   `file_path`, checked against `tool_input.file_path`. If the tool is
   unknown or the field is missing, match every rule against the raw text.
4. On the first match, write the rule's message to stderr and exit `2`.
5. Otherwise exit `0`, with no output.

`policy.json` is loaded relative to the guard file's own directory, not
the working directory.

- [x] `internal/module/repotooling/templates/guard.go`: `package main`,
      standard library only (`encoding/json`, `regexp`, `os`,
      `path/filepath`, `runtime`). Emitted to `.claude/hooks/guard.go`.
      Because the directory is dot-prefixed, `go build ./...` and
      golangci-lint skip it. Confirm this with `task verify`.
- [x] `internal/module/tsrepotooling/templates/guard.mjs`: Node standard
      library only (`node:fs`, `node:path`, `node:url`).
- [x] `internal/module/pyrepotooling/templates/guard.py`: standard library
      only (`json`, `re`, `sys`, `pathlib`). Requires Python 3.12 (the
      standard's version) and no third-party packages, so `uv run
      --no-project` needs nothing installed.
- [x] Each repo-tooling `Taskfile.yml` template gains the task below. It
      has no `desc`, so it stays out of `task --list`, and `silent: true`.
      - Go: `go run .claude/hooks/guard.go`
      - TS: `node .claude/hooks/guard.mjs`
      - Python: `uv run --no-project python .claude/hooks/guard.py`
- [x] Each repo-tooling module's `Resolve` adds the guard resource, placed
      after `lefthook.yml`, using the default mode.
- [x] Module tests: resource order, LF check, `hook:guard` present with no
      `desc`, and the guard's path matching what the task runs.
- [x] Go test `TestGuardGoCorpus` (in `repotooling`): `go build` the
      embedded `guard.go` into `t.TempDir()`, write `policy.json` beside
      it, and run every corpus case. Skipped only when `go` is unavailable,
      which can't happen in CI.
- [x] Measure the cost through `task -x hook:guard` in each root: first
      `go run`, cached `go run`, `node`, and `uv run --no-project`. Record
      the numbers under Verification. If first-run `go run` is over ~5 s,
      stop and raise it before continuing, rather than silently accepting
      it.

### C2: `agent-config` calls Task

- [x] `templates/claude/settings.json`: both matchers run `task -x
      hook:guard`.
- [x] `templates/codex/hooks.json`: the documented nested schema, with
      matcher `Bash`, `type: command`, and `command: "task -x hook:guard"`.
      No `commandWindows`, since the command is identical.
- [x] `agents.go`:
  - [x] drop both `.sh` resources and `hookMode`;
  - [x] add `.claude/hooks/policy.json` with content from
        `renderPolicy()`. This is the first resource whose content is
        computed rather than embedded; `Resolve` stays deterministic.
- [x] `agents_test.go`:
  - [x] `TestAgentConfigsCallTaskWithExitCode`: every agent config's
        command is exactly `task -x hook:guard`. This is the `-x`
        regression test.
  - [x] `TestCodexHooksMatchDocumentedSchema`: unmarshal and assert the
        nested shape.
- [x] `internal/standard/standard_test.go`,
      `TestAgentConfigWiring`: for every registered standard that
      composes `agent-config`, the resolved `Taskfile.yml` defines
      `hook:guard` (parsed as YAML, not substring-matched). The file its
      command names is also a resolved resource.
- [x] Rebuild, then sync the root and both examples, then audit all three.
      The old `.sh` files still exist but are no longer referenced.
- [x] **This repository's own guard is now the new one.** Smoke-test it in
      this session: ask the agent to run a denied command and confirm the
      denial, and confirm an ordinary command still runs. (Done without
      asking; see "Live smoke test" under Verification.)

### C4: end-to-end corpus in CI

- [x] `internal/module/agents/guard_e2e_test.go`,
      `TestGuardCorpusEndToEnd`: when `VIBE_GUARD_E2E_DIR` is set, run
      every corpus case through `task -x hook:guard` in that directory,
      the exact string the agent configs contain. Assert exit codes: `2`
      for deny, `0` for allow. Skipped when the variable is unset, so
      `task verify` stays fast and needs no Node or uv.
- [x] New hand-authored workflow, `.github/workflows/hook-guard.yml`. It
      isn't a module resource, for the same reason `examples.yml` isn't.
      - Matrix: `root` (`prod-go`), `examples/typescript`, and
        `examples/python`, crossed with `ubuntu-latest` and
        `windows-latest`.
      - Each leg installs Go and Task, plus pnpm/Node or uv as needed,
        then runs `go test ./internal/module/agents -run
        TestGuardCorpusEndToEnd` with `VIBE_GUARD_E2E_DIR` set to the
        root.
      - Action pins match the existing workflows.

### C5: docs and cleanup

- [x] Delete `.claude/hooks/block-dangerous.sh` and
      `block-secret-files.sh` in the root and both examples.
- [x] `docs/usage.md`:
  - [x] Replace the file-mode paragraph: hooks are no longer mode-bit
        dependent.
  - [x] Document `hook:guard` as a reserved task name.
  - [x] List the known limitations from the spec.
  - [x] Add adopter migration steps: delete the old `.sh` files.
  - [x] Update "What `prod-go/v1` manages" (resource table).
- [x] `docs/decisions/0006-resource-file-mode.md`: dated note that no
      shipped resource depends on mode any more.
- [x] `AGENTS.md` Guardrails: rewrite for the new mechanism and its
      limitations. This also fixes issue #26, item 11.
- [x] `.codex/README.md`:
  - [x] the new `hooks.json` shape;
  - [x] the schema fix;
  - [x] the Windows `PreToolUse` gap (codex#24453);
  - [x] the existing missing file-edit guard.
- [x] `docs/architecture/overview.md` package list: `agents/` described as
      "Claude/Codex config + guard policy".
- [x] Spec 0021 status: "accepted and implemented". Tick this checklist
      and record verification.

## Verification

Required before the pull request:

- `task verify` and `task audit` at the root, and `vibe audit` for both
  examples.
- `TestGuardCorpusEndToEnd` locally on Windows for all three roots, with
  Task, pnpm/Node, and uv installed here.
- The measured costs from C3.
- The live smoke test from C2, in a Claude Code session on this machine.
- CI: `CI / gate`, `examples.yml`, and all six `hook-guard.yml` legs
  green.

### Observed locally (Windows 11, 2026-09-23)

- **Corpus in-process** (`go test ./internal/module/agents`): all 41 cases
  pass against each guard, with nothing skipped.
  - Go: built from the template.
  - Node: `node guard.mjs`.
  - Python: `uv run --no-project python guard.py`.

  A deliberately broken Node guard (never returning a match) failed 19
  cases, then passed again once restored. (The C3 commit message says 42
  cases; the count is 41.)
- **Corpus end to end** (`TestGuardCorpusEndToEnd` through `task -x
  hook:guard`): all 41 pass in the root, `examples/typescript`, and
  `examples/python`. The first run failed every deny case in the root; see
  Deviations.
- **Mutation check on `-x`:** removing `-x` from `.codex/hooks.json` fails
  `TestAgentConfigsCallTaskWithExitCode`.
- **Wiring test:** `TestAgentConfigWiring` passes for `prod-go/v1`,
  `prod-ts/v1`, and `prod-py/v1`.
- **Verify, lint, audit:**
  - `task lint` at the root: 0 issues.
  - `go test ./...`: all pass.
  - `task verify` in both examples: exit 0. Their own eslint, Prettier,
    ruff, and pyright now cover the guard files.
  - `vibe audit` in all three roots: conformant.
- **Cost per tool call** (payload `ls`, through `task -x hook:guard`):

  | Runtime | Time |
  |---|---|
  | `go run`, cold (empty `GOCACHE`) | 3822 ms |
  | `go run`, warm | 213–244 ms |
  | `node` | 135–180 ms |
  | `uv run --no-project python` | 297–319 ms |

  The cold Go run is under the 5 s stop threshold, and paid once per
  change to `guard.go`.
- **What a deny looks like on stderr:** the guard's message first, then
  `exit status 2` (from `go run`) and Task's
  `task: Failed to run task "hook:guard": exit status 2`. The reason the
  agent shows is the first line.
- **No stray files:** running hooks in the three roots left no `.task/`
  directory and no new `.venv`. `examples/python` already had its `.venv`
  from `uv sync`, so a fresh uv project was not tested for `.venv`
  creation.
- **Live smoke test: done by accident, in the implementing session.** I
  had expected Claude Code to keep the hooks it loaded at session start.
  It didn't: once C2 changed `.claude/settings.json`, the session ran the
  new guard.
  - **Deny path:** after the PR was opened, a Bash heredoc writing a
    memory note merely *mentioned* `git branch -D` as example text. It was
    refused with `[task -x hook:guard]: Blocked: git branch -D
    force-deletes a branch…`, then `exit status 2` and Task's line. That
    is a real deny through the real path, and also the documented "quoted
    content still matches" limitation happening live.
  - **Allow path:** every Bash, Write, and Edit call between C2 and the PR
    (builds, syncs, test runs, commits, the push) went through the guard
    and was allowed.
  - `docs/usage.md` and `AGENTS.md` previously claimed agents load hooks
    only at startup. They were corrected to say "check, don't assume".

### Deviations from the checklist

- **`go run` swallows the guard's exit code. Every `hook:guard` command
  now ends in `|| exit 2`.** The end-to-end test showed that `go run`
  reports its program's exit 2 as 1, so `prod-go`'s guard would have
  *allowed* everything it was meant to deny, through the very path
  agents use. The in-process tests couldn't see this because they run a
  built binary.

  The suffix fixes it for Go. Applied to all three standards, it also
  makes any failure after Task starts the task a deny: a compile error, a
  missing `node` or `uv`, an unreadable policy. That is stricter than the
  spec, which accepted failing open when a runtime is missing, and it is
  recorded there. `TestGoRunReportsGuardDenyAsOne` pins `go run`'s
  behavior.
- **Each rule has a `raw` pattern as well as a `pattern`.** The fallback
  matches the payload text with the old scripts' own patterns, so it is
  exactly as strict as they were. They differ only for the secret-file
  rule, whose old pattern matches the JSON around the path.
- **The guard takes the policy path as its argument** instead of finding
  it next to its own source, which `go run` compiles away.
- **`policy.json` is rendered in Prettier's style**, with short string
  arrays on one line. `prod-ts`'s `fmt:check` runs Prettier over
  `**/*.json`, `.claude/` included, and failed on `json.Encoder`'s layout.
- **The examples' linters check the guards.** eslint required
  `{ cause }` on a rethrown error; ruff reformatted the Python guard. The
  templates were formatted to each standard's own rules, which is also a
  standing check that adopters' tooling accepts them.
- **Claude Code gets one matcher, `Bash|PowerShell|Write|Edit`,** rather
  than two entries: one guard handles both kinds of call. `MultiEdit` is
  in the policy's file-edit tools, since the `Edit` matcher (a regular
  expression) routes it to the guard.
- **The corpus contradicted the spec's claim about quoted commands.** A
  pattern quoted inside a command argument still matches, because it is
  part of `tool_input.command`. The corpus records that as *deny*, and the
  spec is corrected. What parsing does fix is a pattern in a *different*
  field, such as `description`, which the old hook denied.
- **`TestGuardCorpusGo` lives in `internal/module/agents`,** next to the
  other runtimes and `renderPolicy`, not in `repotooling`.
- **The end-to-end test fails rather than skips** when
  `VIBE_GUARD_E2E_DIR` is set but `task` is missing, so a CI leg cannot
  pass having run nothing.
- **The first CI run on the PR failed twice, for reasons Windows couldn't
  show.**
  - `TestSyncCmdAppliesResourceModes` still expected
    `block-dangerous.sh` at `0755`. It skips on Windows, so it passed
    locally. It now checks the guard at the default `0644`, and
    `TestWriteResourceAppliesDeclaredMode` keeps the non-default path
    covered.
  - Every `hook-guard.yml` leg failed its stdout check. Under GitHub
    Actions, Task prints `::error title=Task 'hook:guard' failed::…` to
    stdout whenever a task fails, and a deny is a failure. Reproduced
    locally with `GITHUB_ACTIONS=true`. The check now applies to allows
    only: agents parse stdout on exit 0, while on exit 2 they block
    regardless and take the reason from stderr. The same annotation
    appears if an agent itself runs under GitHub Actions, and is harmless
    there for the same reason.
- **This repository's own `.sh` hooks were deleted last**, just before
  pushing. The reason was to keep the implementing session guarded if it
  still ran the bash hooks it had loaded at startup. It turned out not to
  be (see "Live smoke test"), so the ordering was harmless but
  unnecessary.

## Pull request

Title `feat: agent hooks run through task -x hook:guard`. The body links
the spec and plan, and calls out:

- the Codex schema fix;
- the reserved `hook:guard` name;
- the adopter clean-up of the old `.sh` files;
- the known fail-open conditions.

Merged with a merge commit after an explicit go-ahead.

## Explicitly still deferred

- Shell-aware command matching (heredoc bodies still match).
- `vibe doctor` checking that `hook:guard` launches.
- A single top-level Taskfile for multi-language repositories.
- Codex on Windows, until codex#24453 is resolved.
