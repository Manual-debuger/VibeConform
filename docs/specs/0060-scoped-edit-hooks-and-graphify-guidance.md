# Spec 0060: Monorepo edit hooks run only the edited component, and graphify has one freshness rule

Status: draft (2026-10-26). Issues #81 and #86. Changes the setup skill's
references only: the monorepo reference Taskfile and the optional
Graphify content (spec 0054). No contract change.

## Problem

Two kinds of guidance from the setup skill cost an adopting repository's
agent time or mislead it.

- **In a monorepo, every edit runs every component (#81).** The
  reference monorepo has three components: `api` (Go), `web`
  (TypeScript) and `worker` (Python). After each file edit, two
  PostToolUse hooks run:

  - `task fmt` formats every component. Rule A-06 needs a PostToolUse
    hook that calls the `fmt` entrypoint. An edit to one `.tsx` file
    therefore also runs gofmt, goimports and ruff over the other two
    components. The Phase 3 pilot recorded this as F-15
    (`docs/reports/contract-v1-pilot.md`). #81's status note says that
    #129 fixed it. That is no longer true. #129 fixed the generated
    `hook:format`, and spec 0059 deleted the generated files. The
    skill's reference never had `hook:format` or `fmt:changed`.
  - `hook:check` runs the typecheck and the lint of every component, in
    the background (spec 0057). An edit in `apps/web/` runs pyright and
    golangci-lint as well. When one of them fails, `asyncRewake` can
    wake the agent with a failure in a component that it did not touch.

- **Graphify gets two freshness rules, and the louder one is wrong
  (#86).** The skill's Graphify content (`reference/optional/`) tells
  the agent to use the graph only when `built_at_commit` matches HEAD.
  `graphify claude install` (checked with graphify 0.8.18, 2026-10-26)
  writes two more instructions, and neither checks freshness:

  - A `## graphify` section in `CLAUDE.md`: "For codebase questions,
    first run `graphify query` … when graphify-out/graph.json exists",
    and "After modifying code, run `graphify update .`".
  - A PreToolUse hook on `Bash` in `.claude/settings.json`. When a
    command contains `grep`, `rg`, `find` and similar, and
    `graphify-out/graph.json` exists, the hook tells the agent to run
    `graphify query` instead.

  #86 reports that older versions wrote the hook to
  `.claude/settings.local.json` and said "MANDATORY". Version 0.8.18
  writes the hook to `.claude/settings.json`, which is a guardrail file.
  `graphify codex install` writes a similar section to `AGENTS.md`. The
  audit cannot see the conflict: the text is free prose, and no rule
  reads it.

## Constraints

- **No contract change.** A-06's check matches the hook's simple
  command `task fmt` word for word (spec 0051, "called"). The reference
  PostToolUse format command stays
  `cd "${CLAUDE_PROJECT_DIR:-.}" && task fmt 1>&2 || exit 2`. Any
  narrowing happens inside the root `fmt` task.
- **The `fmt` entrypoint keeps its meaning for everyone else.** People
  run `task fmt` in a terminal, and an adopter can call it from CI or a
  Git hook. Without a hook payload, `task fmt` formats every component,
  as it does now.
- **The gates stay whole.** The Stop gate's `verify:fast`, `verify`, the
  Git hooks and CI check every component. They catch what a scoped edit
  hook misses.
- **Hooks hold from any directory** (spec 0058). A scoped hook gives the
  same result from the root, a component, a component's subdirectory and
  a plain subdirectory.
- **`.claude/settings.json` is a guardrail file.** An agent changes it
  only after a person runs `vibe unlock` (A-08, AGENTS.md
  "Guardrails"). `graphify claude install` and `uninstall` write it. The
  guard does not catch a program that writes the file (a known gap), so
  the skill must say this.
- **The skill's length budget.** `TestSkillLength` limits `SKILL.md`.
  The new Graphify guidance goes in `reference/optional/graphify.md`.
- **Graphify stays optional** (spec 0035). Nothing in `verify`, in the
  hooks that the contract reads, or in CI depends on it.

## Assumptions

- Claude Code gives each hook command the event's JSON payload on stdin.
  Task passes stdin to a task's command. `hook:check` already relies on
  both.
- A PostToolUse payload for `Write`, `Edit` and `MultiEdit` has an
  absolute `tool_input.file_path`, and one for `NotebookEdit` has
  `notebook_path`. On Windows the path is in Windows form, and its
  backslashes are JSON-escaped.
- When a person runs `task fmt` in a terminal, stdin is a terminal. In
  CI and in lefthook, stdin is empty or holds no PostToolUse payload. A
  test confirms that `task fmt </dev/null` formats every component.
  During implementation, check that a lefthook job that runs `task fmt`
  does not wait on stdin.
- `graphify claude uninstall` removes only graphify's own hook entry and
  `CLAUDE.md` section, and keeps the reference hooks. This was checked by
  hand with graphify 0.8.18: after install and uninstall, the parsed
  reference settings were unchanged, and `CLAUDE.md` still held
  `@AGENTS.md`. The command deletes `CLAUDE.md` when the file is empty
  after the removal. It can rewrite the file's line endings.

## Desired Behavior

### Edit hooks scoped to the component (#81)

1. In the reference monorepo, a PostToolUse payload whose edited path is
   inside a component's `path` makes `task fmt` run only that
   component's `fmt`. `hook:check` runs only that component's
   `typecheck` and `lint`.
2. When components are nested, the component with the longest matching
   path owns the file.
3. All other cases run every component, as now: no stdin, a terminal, an
   empty stdin, input that is not JSON, a payload without a path, and a
   path that is in the project but in no component (`Taskfile.yml`,
   `docs/`, `.github/`). `hook:check` keeps its skips from spec 0057:
   nothing for a sub-agent's edit, an ignored file or a path outside the
   repository.
4. The edited path is resolved the same way on Windows (Windows-form
   path, escaped backslashes) and on Linux, and in a project whose path
   has spaces. The project can be at the root of its Git repository or
   in a subdirectory of it.
5. The PostToolUse hook commands in the reference settings do not
   change. A-06 still passes, and so does the static audit of every
   reference.
6. The reference monorepo lists the component table once (id and path).
   Both hooks use that table. It agrees with the components in the
   reference `vibe.yaml`. The reference comments tell an adopter to
   update the table when components change.
7. `examples/monorepo` is rebuilt from the changed references, as spec
   0059 rebuilt it.

### One freshness rule for Graphify (#86)

8. `reference/optional/graphify.md` tells the agent not to run
   `graphify claude install` or `graphify codex install`. The guidance
   that they write is replaced by the `AGENTS.md` paragraph that the
   file already defines.
9. The file says how to find the installer's guidance and remove it, if
   the repository already has it:
   - a `## graphify` section in `CLAUDE.md` or `AGENTS.md`;
   - graphify's PreToolUse hook in `.claude/settings.json` or
     `.claude/settings.local.json`.

   `graphify claude uninstall` removes the `CLAUDE.md` section and the
   hook. `graphify codex uninstall` removes the `AGENTS.md` section.
   After the removal, the agent checks that `CLAUDE.md` still exists and
   still imports `AGENTS.md`, and that the reference hooks are still
   there.
10. When the guard is already installed, `.claude/settings.json` is a
    guardrail file. The agent then asks the person to run `vibe unlock`,
    or to run the uninstall with `! graphify claude uninstall`. The agent
    does not run it before that.
11. In the Graphify content, the freshness rule ("use it only when
    `built_at_commit` matches HEAD") appears once. That is in the
    `AGENTS.md` paragraph. No other part of the skill gives a second
    instruction about when to use or rebuild the graph. The rebuild
    stays with the Git hooks (`task graph:update`).

### Documentation

12. `docs/usage.md`, "Agent hooks": the PostToolUse table and the
    `hook:check` section say that a monorepo hook runs only the edited
    component, and which edits still run every component.
13. #81's status note is corrected: F-15 was open again after spec 0059,
    and this spec closes it.

## User Stories

1. As an agent in a monorepo, I want an edit in `apps/web/` to format only
   `web`, so that each edit does not wait for gofmt and ruff over code I
   did not touch.
2. As an agent in a monorepo, I want the background check after an edit
   in `apps/web/` to run only `web:typecheck` and `web:lint`, so that a
   Python lint failure does not wake me in the middle of TypeScript work.
3. As an agent, I want an edit to a root file, such as `Taskfile.yml`, to
   still check every component, so that a change that can affect all of
   them is checked against all of them.
4. As an agent, I want an edit to a file in a nested component to run
   that component's tasks, not its parent's, so that the scope matches
   the owner of the file.
5. As an agent on Windows, I want the same scoping as on Linux, so that
   the hooks do not fall back to every component because of path forms.
6. As an agent working from inside a component's directory, I want the
   scoped hooks to behave as from the root, so that spec 0058 still
   holds.
7. As a person running `task fmt` in a terminal, I want it to format the
   whole repository, as before, so that the entrypoint I know does not
   change meaning.
8. As a CI job or a Git hook that calls `task fmt`, I want it to format
   every component and never wait for input, so that the pipeline does
   not hang.
9. As an adopter, I want `vibe audit` to pass A-06 with the reference
   hooks unchanged, so that scoping costs me no conformance.
10. As an adopter with different components, I want one table to edit
    when I add or move a component, so that the format hook and the check
    hook cannot disagree.
11. As an adopter, I want the reference to agree with the reference
    `vibe.yaml`, so that a stale component table is found by a test, not
    by an agent.
12. As an agent, I want the Stop gate to still check every component, so
    that a defect that the scoped hooks missed still blocks the end of
    the turn.
13. As a sub-agent that edits files, I want my edit formatted in its
    component, so that my output is formatted even though `hook:check`
    skips me.
14. As a maintainer, I want `examples/monorepo` to match the references,
    so that the examples workflow runs what the skill writes.
15. As an agent in a repository with Graphify, I want one rule about when
    the graph can be trusted, so that I do not follow a hook that pushes
    me to a stale graph.
16. As an agent setting up Graphify, I want the skill to tell me not to
    run `graphify claude install`, so that I do not add the conflicting
    guidance myself.
17. As an agent in a repository that already has the installer's
    guidance, I want the exact commands that remove it, so that I do not
    edit graphify's hook by hand.
18. As an agent, I want to check after the uninstall that `CLAUDE.md`
    still imports `AGENTS.md`, so that the uninstall does not delete the
    file that loads the agent instructions.
19. As a person, I want the agent to ask me before graphify's uninstall
    rewrites `.claude/settings.json`, so that a guardrail file changes
    only after my unlock.
20. As an agent using Codex, I want the `AGENTS.md` section from
    `graphify codex install` handled too, so that both agents get the
    same rule.
21. As a maintainer, I want a test that fails when the Graphify content
    states the freshness rule twice or omits the uninstall, so that the
    guidance stays single-sourced.
22. As a reader of `docs/usage.md`, I want the hook tables to say which
    edits run one component and which run all, so that I can predict the
    hooks' cost.

## Implementation Decisions

- **Only the monorepo reference changes for #81.** The Go, TypeScript and
  Python references have one component at the root. Scoping cannot
  change their result.
- **One owner lookup, shared by both hooks.** A task without a `desc` in
  the reference root Taskfile reads a hook payload on stdin. It prints
  the id of the component that owns the edited path, or nothing when
  every component must run. It holds the component table (id and
  path), as `hook:context` already lists the components. `fmt` and
  `hook:check` start it as a child `task` process. #129 used the same
  pattern, and Task then starts it in the root directory.
- **`fmt` becomes one script.** It reads stdin only when stdin is not a
  terminal. It passes the input to the owner lookup. Then it runs that
  component's `fmt`, or every component's `fmt`. Each formatter's exit
  status still fails the task. `fmt:check`, `verify` and `verify:fast`
  stay task lists over every component.
- **`hook:check` keeps its order of decisions.** First the sub-agent
  skip, then the path extraction, then the ignored-path and
  outside-repository skips, then the owner lookup. Last, it runs either
  `<id>:typecheck <id>:lint` or the full list.
- **Path resolution uses the shell, not string rules.** The lookup
  removes the JSON escaping. It changes into the edited file's directory
  and into the project root, and it compares the physical paths that
  both report. A path whose directory does not exist counts as outside
  every component, so every component runs. Git Bash resolves the
  Windows and MSYS forms, so the lookup needs no drive-letter rule.
- **The longest path prefix wins**, and the comparison is on whole path
  segments: `apps/web2/x` is not inside `apps/web`.
- **No new manifest field.** #81's optional `components[].affects`
  (gitignore patterns for shared paths) is not part of this spec. A
  shared path outside every component already runs every component.
- **Graphify: guidance only.** `reference/optional/graphify.md` gets a
  section about the installer's guidance (Desired Behavior 8–11).
  `vibe doctor` gets no warning (#86's proposal 2). `vibe` does not run
  graphify, and does not parse graphify's files.
- **No upstream dependency.** A freshness check in graphify's hook
  (#86's proposal 3) is not a dependency of this spec.

## Testing Decisions

- **Good tests run the reference hooks as Claude Code does.** They feed a
  payload on stdin to the reference settings' commands, run the
  reference Taskfile with Task, and stub each component task so that it
  prints its name. They assert which component tasks ran and the exit
  code. They do not assert how the script is written.
- **Seam 1, #81: the reference-hook harness in `internal/setupskill`.**
  `TestReferenceHooksFromAnyDirectory` and the `hookCheckRepo` fixtures
  already run the PostToolUse commands with an edit payload, from four
  directories. Its `format` and `check` steps now expect only `api:*`
  for an edit to `services/api/main.go`. Table cases add these:
  - an edit in each component;
  - an edit to a root file (every component);
  - a nested component, if the fixture gets one;
  - a sibling with a shared prefix (`apps/web2/`, every component);
  - `NotebookEdit`'s `notebook_path`;
  - a payload with indented JSON, and a Windows-form escaped path;
  - a path whose directory does not exist;
  - `task fmt </dev/null`, `task fmt` with non-JSON input, and an empty
    stdin (every component).

  The existing spec 0057 cases (sub-agent, ignored file, outside the
  repository) keep passing. The harness already runs on the Linux and
  Windows CI runners.
- **The table agrees with the manifest.** A test parses the reference
  root Taskfile's component table and the reference monorepo
  `vibe.yaml`, and compares ids and paths. Prior art:
  `reference_pins_test.go`.
- **A-06 still passes.** The existing static audit of the references
  covers this. No new test.
- **Seam 2, #86: assertions on the embedded skill.** These assertions
  read `reference/optional/graphify.md` from `setupskill.FS()`:
  - it names `graphify claude uninstall` and `graphify codex uninstall`;
  - it says not to run `graphify claude install`;
  - it names `vibe unlock` for `.claude/settings.json`;
  - it states the `built_at_commit` freshness rule exactly once.

  Prior art: `TestReferenceHooksAreTheGuards` and `TestSkillLength` in
  `setupskill_test.go`.
- **Manual check, not in CI:** with graphify installed, run install and
  then uninstall on a repository set up from the references. Then
  confirm that the reference hooks survive and that `CLAUDE.md` still
  imports `AGENTS.md` (Assumption 4). The ledger records it.

## Out of Scope

- Any contract change: A-06's wording, a new `fmt-changed` entrypoint,
  or `components[].affects` in `vibe.yaml`.
- Scoping the Stop gate, `verify:fast`, `verify`, the Git hooks or CI.
- The single-component references, and this repository's own Taskfile
  and settings (one Go component, `hook:format` with `fmt:changed`).
- Formatting only the changed files inside a component (the old
  `fmt:changed`). This spec narrows by component, not by file.
- Skipping `fmt` for edits outside the project. They keep formatting
  every component, as now.
- A `vibe doctor` warning about stale Graphify guidance, and asking
  upstream Graphify for a freshness check.
- `graphify install` (the `/graphify` skill copy) and graphify's Git
  hook installer.

## Acceptance Criteria

- [ ] In a fixture built from the references, an edit in one component
      runs only that component's `fmt`, `typecheck` and `lint`. This is
      the same from the root, a component, a component's subdirectory
      and a plain subdirectory, on Linux and Windows CI.
- [ ] An edit to a root file, a path in no component, a sibling path with
      a shared prefix, or a path that cannot be resolved runs every
      component. So do no payload, an empty stdin and non-JSON input.
- [ ] The spec 0057 skips of `hook:check` still hold.
- [ ] The reference PostToolUse commands are unchanged, and every
      reference passes the static audit (A-06 included).
- [ ] A test fails when the reference component table and the reference
      `vibe.yaml` components differ.
- [ ] `examples/monorepo` matches the references, and the examples
      workflow passes.
- [ ] `reference/optional/graphify.md` says not to run the installers,
      how to remove their guidance, when to ask for `vibe unlock`, and
      what to check after the removal. It states the freshness rule once,
      and a test asserts this.
- [ ] `docs/usage.md` and #81's status are updated as in Desired
      Behavior 12–13.
- [ ] The manual graphify install and uninstall check is recorded in the
      ledger.
- [ ] `task verify`, `task audit` and CI pass.

## Further Notes

- #133 (closed by spec 0058) said to fix the hooks from inside a
  component before this work. That is done. The new tests extend spec
  0058's harness.
- #81's measurements (Go 4.1 s, TypeScript 3.7 s, Python 13.9 s per
  component) included the tests, which spec 0057 removed from
  `hook:check`. Format and lint still scale with the number of
  components. The status note says to measure again. Take one warm
  measurement of `task fmt` and `hook:check` before and after this
  change in `examples/monorepo`, and record it in the PR.
- A narrower edit hook can miss a cross-component break, for example a
  change to a shared type in `api` that `web` consumes. The Stop gate
  runs every component, so the break blocks the end of the turn. It does
  not appear after each edit.
