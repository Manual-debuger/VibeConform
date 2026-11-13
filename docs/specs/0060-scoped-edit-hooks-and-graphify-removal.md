# Spec 0060: Monorepo edit hooks run only the edited component, and the setup skill drops Graphify

Status: draft (2026-10-26, revised 2026-11-13). Issues #81 and #86.
Changes the setup skill's references only: the monorepo reference
Taskfile, and the removal of the optional Graphify content (spec 0054).
No contract change.

## Problem

Two parts of the setup skill's references cost an adopting repository's
agent time, or blur what VibeConform is for.

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

- **Graphify is outside VibeConform's purpose (#86).** VibeConform
  checks that a repository's tooling has the properties the contract
  requires. Graphify is a knowledge-graph tool. No rule checks it, and
  `verify` and CI never use it (spec 0054). The skill still ships
  `reference/optional/graphify.md`, with a `graph:update` task, three
  Git hook jobs and an `AGENTS.md` paragraph. The skill then has to
  keep up with Graphify's installer: #86 reports that
  `graphify claude install` writes a second freshness rule, which
  conflicts with the first, and a PreToolUse hook into
  `.claude/settings.json`, a guardrail file. Fixing that guidance would
  tie the skill more closely to a tool the contract does not cover.
  The maintainer decided (2026-11-13) to remove Graphify from the skill
  instead.

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
- **Windows and Linux alike** (principle 2). The scoped hooks are POSIX
  shell run through Task and Git Bash, with no per-OS branch.
- **History stays.** Specs, plans and ADRs that describe Graphify are
  records. Spec 0059 already marked them superseded. They are not
  edited or deleted.
- **The removal must not break a migrated repository.** A repository
  that used Graphify through the template-sync system keeps its own
  `graph:update` task and hook jobs. The skill does not remove them on
  its own.

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
- Nothing outside `reference/optional/graphify.md` in the embedded skill
  names it or depends on it. A search on 2026-11-13 found Graphify named
  only in that file, in `docs/usage.md`'s reference table, in
  `docs/architecture/overview.md` and in history. The
  `integrations.intelligence` line in `reference/migrate.md` points to
  `reference/optional/` in general.

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

### The setup skill drops Graphify (#86)

8. `internal/setupskill/skill/reference/optional/graphify.md` is
   deleted. The embedded skill names Graphify nowhere.
9. `reference/migrate.md`: `integrations.intelligence` has no
   counterpart. The line says that a migrated repository's
   `graph:update` task, the Git hook jobs that call it and its
   `graphify-out/` ignore line are the repository's own. The person can
   keep them, or remove them together. The skill does not write or
   delete them unless the person asks.
10. `docs/usage.md`'s table of the skill's references no longer lists
    Graphify under `reference/optional/`.
11. `docs/architecture/overview.md`, "Repository intelligence and Skills
    Manager boundaries": the boundary stays (VibeConform does not absorb
    repository-intelligence tools, and they never replace the compiler,
    linter, tests or CI). The sentence saying that the skill offers
    Graphify is removed, and so is Graphify from the list of examples.
12. Spec 0054's status line notes that spec 0060 removed its Graphify
    content. #86 is closed as not planned, with a link to this spec.

### Documentation

13. `docs/usage.md`, "Agent hooks": the PostToolUse table and the
    `hook:check` section say that a monorepo hook runs only the edited
    component, and which edits still run every component.
14. #81's status note is corrected: F-15 was open again after spec 0059,
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
15. As a reader of the setup skill, I want it to cover only what the
    contract checks and the tooling around it, so that its purpose is
    clear.
16. As a maintainer, I want the skill to stop following Graphify's
    installer, so that a change upstream does not become work here.
17. As an adopter who migrated from the template-sync system with
    Graphify, I want to know that my graph tasks are now mine, so that I
    can keep or remove them knowingly.
18. As a reader of `docs/usage.md`, I want the hook tables to say which
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
- **Graphify: delete, don't replace.** No other knowledge-graph or
  code-search integration takes its place in `reference/optional/`.
  `vibe doctor` gets no Graphify check, and `vibe` never mentions
  Graphify.
- **One branch and one PR** for both parts, as the spec covers both
  issues. The Graphify removal is its own commit, so it can be reverted
  alone.

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
- **Seam 2, #86: the existing skill tests.** Deleting a file is covered
  by the tests that walk the embedded skill (`TestSkillLength`, the
  skill drift checker in `internal/cli`, the reference pin tests). They
  must keep passing with the file gone. No test asserts that a word is
  absent.

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
- Editing or deleting the Graphify specs, plans and ADRs (0016, 0020,
  0035, 0039, 0046 and the plans with those numbers).
- The other optional content (VS Code, Zed, language servers, the
  `AGENTS.md` workflow sections, the docs layout).
- The "repository intelligence (GitNexus, if configured)" line in this
  repository's `AGENTS.md`, and the boundary itself in
  `docs/architecture/overview.md`.

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
- [ ] `reference/optional/graphify.md` is gone, and a case-insensitive
      search for "graphify" in `internal/`, `cmd/`, `examples/`,
      `README.md`, `docs/usage.md` and `docs/architecture/` finds
      nothing.
- [ ] `reference/migrate.md` says what happens to a migrated
      repository's Graphify tasks (Desired Behavior 9).
- [ ] Spec 0054's status line and #86 are updated (Desired Behavior 12).
- [ ] `docs/usage.md` and #81's status are updated as in Desired
      Behavior 13–14.
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
- The first draft of this spec (2026-10-26) kept Graphify and gave it
  one freshness rule. The maintainer replaced that part with the
  removal on 2026-11-13.
