# Spec 0053: Contract audit, probes and the reference guard (Phase 2)

Status: accepted (2026-10-17). Tickets #102, #103, #104, #105, #106
(milestone "Contract v1 · Phase 2: Probes and guard"), spec issue #116. Builds on spec
0051 (contract v1.0.0), spec 0052 (Phase 1, static checks), ADR 0025 and
ADR 0026. "Qn" refers to the grilling questions the tickets cite. Where
this spec and spec 0051 disagree, spec 0051 wins, and the disagreement
is a defect in this spec.

## Problem Statement

Phase 1 made `vibe audit` decide every static part of contract 1.0.0.
Every execution and probe part still reports "cannot decide: not
implemented in this vibe". Without `--static`, the audit therefore never
reaches a verdict. Half of the contract is about behaviour: `verify`
must catch real defects, `fmt-check` must not write, lockfiles must
match, the guard must refuse what the corpus lists, fail closed, and
keep agents away from their own guardrails. None of that is checked.

The guard has a second problem. Three implementations (Go, Python,
Node) each match commands with one large regular expression. A regex
cannot follow shell quoting, so spellings slip through (spec 0047). The
guards do not read contract exceptions, so a `policy.json` with a
non-empty `exceptions` makes them refuse every call. Nothing stops an
agent from editing `.claude/settings.json` to switch the guard off.

## Solution

Phase 2 implements every remaining part of contract 1.0.0, and the
guard that satisfies them:

- **The probe copy**: a temporary Git repository holding the working
  tree's tracked files, in which every execution check and probe runs.
  This is the phase's shared foundation.
- **Execution checks** (#102): V-03, D-01, R-01's execution part and
  A-07's execution part.
- **Guard probes** (#103): A-02 and A-03 feed the contract's guard corpus
  to whatever PreToolUse hooks the repository declares.
- **Mutation probes** (#104): V-01, V-02 and A-05's probe part plant
  fixed defects and check that `verify`, `verify-fast` and the Stop hook
  catch them.
- **`vibe-guard`** (#105): one Go binary that parses commands as shell,
  holds the baseline rules, reads `policy.json` version 2 (the
  repository's own rules and exceptions), names a rule ID in every
  refusal, explains its decisions, has an observe mode, and makes no
  network calls.
- **Unlock** (#106): `vibe unlock --minutes N` lets a person open the
  agent guardrail settings for a while. The guard refuses an agent that
  tries to unlock by itself, and the Stop hook locks them again (A-08).
- **This repository** switches to `vibe-guard`, built from source,
  through an opt-in in the old template system. The defaults that
  adopters and the example repositories get stay unchanged until #112.

After this phase, `vibe audit` without `--static` reaches a verdict.

## User Stories

1. As an adopter, I want `vibe audit` without `--static` to reach a
   verdict, so that "conformant" means every MUST rule was checked.
2. As an adopter, I want the audit to run my entrypoints in a temporary
   copy, so that it never changes my working tree, index or Git
   history.
3. As an adopter, I want the copy to hold my tracked files with their
   uncommitted changes, so that the audit checks what I am about to
   commit.
4. As an adopter, I want untracked and ignored files kept out of the
   copy, so that a local build output or `.env` cannot make my
   repository pass or leak into a probe.
5. As an adopter, I want my declared `setup` entrypoint run once in the
   copy before anything else, so that my dependencies are installed the
   way my CI installs them.
6. As an adopter whose `setup` fails, I want the audit to say so and
   show the output, so that I fix the setup before reading other
   results.
7. As an adopter, I want the audit to tell me when a tool my entrypoint
   names is not installed on this machine, so that a missing tool is
   "cannot decide", not a defect of my repository.
8. As an adopter, I want each run to have a time limit, so that a hung
   test cannot hang the audit.
9. As an adopter, I want progress lines on stderr while the probes run,
   so that I know a run of several minutes is still working.
10. As an adopter, I want each failed run to show the end of its
    output, so that I can see why it failed without running it again.
11. As an adopter, I want V-01 to tell me which defect kind (format,
    static analysis, test) `verify` did not catch, and in which
    component, so that I know what to add to `verify`.
12. As an adopter with a component that has no test file, I want V-01
    to fail with "component <path> has no tests", so that I know
    `verify` cannot be shown to run tests.
13. As an adopter, I want to see the exact defect samples with
    `vibe audit --explain V-01`, so that I can reproduce a probe by
    hand.
14. As an adopter, I want V-02 to check `verify-fast` with the same
    defects, so that the fast gate my agent relies on is not a no-op.
15. As an adopter, I want V-03 to show any file `fmt-check` wrote, so
    that a check that silently formats is caught.
16. As an adopter, I want D-01 to check each component's lockfile with
    the package manager's own check, so that a lockfile that does not
    match the manifest fails.
17. As an adopter with dependencies and no lockfile, I want D-01 to
    fail, so that unpinned dependencies are not silently accepted.
18. As an adopter, I want R-01 to run `verify` and the guard with `vibe`
    unavailable, so that I know removing VibeConform leaves working
    tooling.
19. As an adopter, I want R-01 to name every place that tried to run
    `vibe`, so that I can find a hidden dependency.
20. As an adopter, I want A-07 to run my SessionStart hook in the copy,
    so that I know it changes no file.
21. As an adopter using any guard, I want the audit to feed the guard
    corpus to the PreToolUse hook commands in my committed
    `.claude/settings.json`, so that my own guard implementation is
    judged by its behaviour.
22. As an adopter, I want A-02 to name every corpus case my guard gets
    wrong, so that I can fix the guard or add an exception.
23. As an adopter, I want each corpus case that a guard exception lets
    through listed in the report, so that every loosening stays
    visible.
24. As an adopter, I want A-03 to check that my guard still refuses with
    its runtime missing and with its policy file broken, so that a
    broken guard cannot silently allow everything.
25. As an adopter, I want A-05 to send a real Stop payload to my Stop
    hook with a defect planted, so that I know the agent cannot end a
    turn with failing checks, and is not stuck in a loop either.
26. As an adopter, I want A-08 probed with no unlock, a valid unlock and
    an expired unlock, so that I know my agent cannot change its own
    guardrails.
27. As a developer, I want `vibe unlock --minutes N` to open the agent
    guardrail settings for N minutes, so that I can let my agent edit
    them while I watch.
28. As a developer, I want the unlock to end when the agent's turn ends,
    so that an unlock I forgot does not stay open.
29. As a developer, I want the guard to refuse an agent that runs
    `vibe unlock` or writes the unlock marker, so that only a person can
    unlock.
30. As a developer, I want `vibe-guard` to parse commands as shell, so
    that quoting, `rm -r -f`, `sh -c '…'` and command substitutions
    cannot get a dangerous command past it.
31. As a developer, I want `vibe-guard` to refuse a command it cannot
    parse, so that an unusual spelling fails closed.
32. As a developer on Windows, I want PowerShell commands tokenized as
    PowerShell, so that `ri -Rec -Fo build` is refused and an ordinary
    PowerShell command is not refused as a shell syntax error.
33. As a developer, I want every refusal to name its rule ID, so that I
    know which rule refused and which exception would allow it.
34. As a developer, I want `vibe-guard explain '<command>'` to show
    which rule matched, which words matched and which exception
    applied, so that I can understand a refusal without reading code.
35. As a team trying the guard, I want an observe mode that only
    reports what it would refuse, so that we can adopt it without
    blocking work on the first day.
36. As a team, I want to add our own deny or ask rules to `policy.json`
    by command prefix, argument pattern, path glob and tool, so that
    most customization needs no code.
37. As a team, I want the baseline rules built into the guard, so that
    `policy.json` can only add rules or list exceptions, never delete a
    baseline rule.
38. As a security reviewer, I want `vibe-guard` to make no network
    calls, proven by a CI run with the network disabled, so that the
    guard cannot leak commands or depend on a service.
39. As a developer without `vibe-guard` installed, I want the hook to
    refuse every call and tell me how to install it, so that a missing
    guard fails closed instead of reading as "allow".
40. As the maintainer, I want this repository to run `vibe-guard` built
    from source, so that the reference guard is what guards its own
    development.
41. As the maintainer, I want the template defaults and the example
    repositories unchanged, so that adopters of the old system are not
    moved before #112.
42. As the maintainer, I want the Go, Python and Node guards to keep
    passing their own corpus, and the audit to name exactly the new
    cases they miss, so that the old guards stay usable and their gaps
    are recorded.
43. As the maintainer, I want a non-required CI job that runs the full
    audit on this repository, so that I see the verdict on every pull
    request before #111 makes it required.
44. As a contributor, I want each execution and probe part to have a
    passing and a failing fixture test, so that a change to a probe
    cannot pass unnoticed.
45. As a contributor without `uv` or `pnpm` installed, I want the
    Python and TypeScript probe tests to skip locally, so that I can
    run the suite, while CI refuses to skip them.
46. As the setup skill (#108), I want the JSON report to carry each
    probe finding with its file and fix, so that I can fix a repository
    and audit again.

## Implementation Decisions

### The probe copy (foundation)

- **One copy per audit.** Without `--static`, after the static parts,
  the audit builds one probe copy in the operating system's temporary
  directory, outside the repository. It holds every file `git ls-files`
  lists that exists in the working tree, with its working-tree content:
  the same set Phase 1's `Repo` reads (spec 0051, decision 6). A
  tracked symbolic link is copied as a link. Where the operating system
  refuses to create one, every execution and probe part is `cannot
  decide`, naming the file. Submodules (gitlinks) are not copied, and
  the report notes them.
- **The copy is a fresh Git repository.** The audit runs `git init` in
  the copy (initial branch `main`), adds every file and commits once
  with a fixed author. "`git status --porcelain`" and "`git diff`" in
  the checks are therefore relative to what the audit copied. The copy
  has no remote. The real repository's `.git` is never read by a probe
  and never written.
- **Reset.** Between probes, the audit restores the copy's tracked
  files to that commit and deletes untracked files that are not
  ignored. Ignored files, such as the dependencies `setup` installed,
  stay. Planted defect files are untracked, so a reset removes them.
- **Deletion.** The copy is deleted when the audit ends, also after a
  failure, a timeout or an interrupt. `--keep-probe-copy` keeps it and
  prints its path on stderr, for debugging.
- **Environment.** Runs inherit the audit's environment with these
  changes: every `GIT_*` variable that points Git at a repository
  (`GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`,
  `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`) is
  removed, so that a run cannot reach the real repository.
  `CLAUDE_PROJECT_DIR` is the copy. `PATH` starts with the R-01 shim
  directory (below), except where A-03 replaces it.
- **Order.** The parts run one after another in a fixed order, and
  each starts from a reset copy: `setup`; A-07; V-03; the baseline runs
  of `verify` and `verify-fast`; the planted defects for V-01, V-02 and
  A-05; A-02; A-03; A-08; D-01. A part whose precondition did not hold
  is `cannot decide` and names the precondition.
- **`setup`** (spec 0051, decision 2) runs once, first. If it fails,
  every execution and probe part is `cannot decide`, and the finding
  shows the end of its output.

### How the audit runs commands

- **Entrypoints run without a shell.** Each command of an entrypoint is
  already one simple command (V-04, spec 0052). The audit runs its
  words directly as a program and its arguments, with leading
  assignments added to the environment. The program is found on `PATH`
  (with `PATHEXT` on Windows). A list runs in order and stops at the
  first failure (spec 0051). This needs no shell on any platform and
  gives the same result everywhere.
  - A word that holds a parameter expansion or a command substitution
    cannot be run without a shell: the part is `cannot decide`, and the
    finding says so. Accepting it later is MINOR.
  - A program not found on `PATH` makes the part `cannot decide`
    ("`task` is not installed on this machine"). That is the auditing
    machine's gap, not the repository's. The one exception is `vibe`
    itself (R-01).
  - Working directory: the copy's root. Entrypoints are repository-wide
    (spec 0051).
- **Hook commands run the way Claude Code runs them**: `bash -c
  '<command>'` with the hook's JSON payload on stdin, the copy as the
  working directory, and `CLAUDE_PROJECT_DIR` set. On Windows, bash is
  the one Claude Code uses: `CLAUDE_CODE_GIT_BASH_PATH` when it names
  an existing `bash.exe`, `sh.exe`, `bash` or `sh`, otherwise Git for
  Windows' `bash.exe` (found next to `git.exe`, never
  the WSL launcher in `System32`). With no bash, every hook part is
  `cannot decide`. Bash is resolved once, before any change to `PATH`.
  - Only `type: command` hooks run. Hook entries are read as Phase 1
    reads them (spec 0052, "Claude Code settings"), including the
    anchored matcher reading.
  - A hook "refuses" or "blocks" only by exiting 2 (spec 0051, A-02).
    A JSON `permissionDecision` on stdout is not read. Reading it would
    be a MINOR relaxation.
  - Each payload carries the fields Claude Code sends: `session_id`
    (`vibe-audit`), `transcript_path` (empty), `cwd` (the copy),
    `hook_event_name`, and the event's own fields (`tool_name` and
    `tool_input` for PreToolUse, `stop_hook_active` for Stop, `source:
    startup` for SessionStart).
- **Time limit.** Every run has a limit: a hook's own `timeout` field
  when it has one, otherwise 600 seconds for hooks (Claude Code's
  default; corrected from 60 on 2026-10-18, see "Hidden assumptions"),
  and
  `--probe-timeout` (default 30 minutes) for entrypoints. A run that
  exceeds it is killed together with its child processes (a process
  group on Unix, a job object on Windows), and its part is `cannot
  decide` ("timed out after …").
- **Output.** The output of each run is captured. A finding about a
  failed run shows its last 20 lines. In text mode, a progress line
  goes to stderr before each run ("vibe audit: verify with the
  static-analysis defect in ."). `--json` keeps stdout to the JSON
  document only.

### Planted defects (#104)

- **Samples.** One fixed sample per language and defect kind, as spec
  0051's table says. The samples are part of contract 1.0.0. Changing
  one could change a verdict, so it is versioned like a rule. Their
  exact content is fixed during implementation, shown by `vibe audit
  --explain V-01` and `--explain V-02`, and written into this spec
  before it is merged. Each is the smallest file that the language's
  usual tools treat as the defect:
  - Go: the unformatted file and the vet defect use the package clause
    of the existing `_test.go` file next to them, so the package still
    builds. The failing test is a `Test…` function that calls
    `t.Fatal`.
  - Python: the failing test is a `test_*.py` with one test that
    asserts false.
  - TypeScript: the failing test is a `*.test.ts` that throws when it
    is loaded. Every common runner (Vitest, Jest, `node --test`)
    reports that as a failed file.
  - Every sample names itself as planted by `vibe audit` in a comment,
    and its file name starts with `vibe_probe`.
- **The samples of contract 1.0.0** (fixed in #104; `vibe audit
  --explain V-01` prints the same text). In the Go samples,
  `{{package}}` is the name in the package clause of the `_test.go`
  file the placement rule found. The Go samples are `_test.go` files, so
  that an external test package (`package x_test`) still builds.

  | Language | Kind | File |
  |---|---|---|
  | Go | format | `vibe_probe_format_test.go` |
  | Go | static analysis | `vibe_probe_static_analysis_test.go` |
  | Go | test | `vibe_probe_test.go` |
  | Python | format | `vibe_probe_format.py` |
  | Python | static analysis | `vibe_probe_static_analysis.py` |
  | Python | test | `vibe_probe_test.py` |
  | TypeScript | format | `vibe_probe_format.ts` |
  | TypeScript | static analysis | `vibe_probe_static_analysis.ts` |
  | TypeScript | test | `vibe_probe.test.ts` |

  `vibe_probe_format_test.go`:

  ```go
  // Planted by vibe audit: an unformatted file (the format defect).

  package {{package}}

  func  vibeProbeFormat()  {}
  ```

  `vibe_probe_static_analysis_test.go`:

  ```go
  // Planted by vibe audit: a Printf call whose arguments do not match its format
  // (the static-analysis defect).

  package {{package}}

  import "fmt"

  func vibeProbeStaticAnalysis() { fmt.Printf("%d\n", "vibe audit") }
  ```

  `vibe_probe_test.go`:

  ```go
  // Planted by vibe audit: a test that fails (the test defect).

  package {{package}}

  import "testing"

  func TestVibeProbe(t *testing.T) { t.Fatal("planted by vibe audit: this test fails") }
  ```

  `vibe_probe_format.py`:

  ```python
  # Planted by vibe audit: an unformatted file (the format defect).
  vibe_probe = [1,2 ,3]
  ```

  `vibe_probe_static_analysis.py`:

  ```python
  # Planted by vibe audit: a reference to an undefined name (the static-analysis defect).
  def vibe_probe() -> int:
      return vibe_probe_undefined_name
  ```

  `vibe_probe_test.py` (pytest collects `*_test.py` by default, and the
  name starts with `vibe_probe`):

  ```python
  # Planted by vibe audit: a test that fails (the test defect).
  def test_vibe_probe() -> None:
      assert False, "planted by vibe audit: this test fails"
  ```

  `vibe_probe_format.ts`:

  ```ts
  // Planted by vibe audit: an unformatted file (the format defect).
  export const vibeProbe = [1,2 ,3]
  ```

  `vibe_probe_static_analysis.ts`:

  ```ts
  // Planted by vibe audit: a reference to an undefined name (the static-analysis defect).
  export const vibeProbe: number = vibeProbeUndefinedName;
  ```

  `vibe_probe.test.ts`:

  ```ts
  // Planted by vibe audit: a test file that throws when it is loaded (the test defect).
  export {};

  throw new Error("planted by vibe audit: this test fails");
  ```

  Each sample carries only its own defect: the static-analysis and test
  samples are formatted, and the format sample type-checks. `go test`
  runs part of `go vet` by itself, `printf` included, so a Go `verify`
  whose `go test` keeps that default also catches the static-analysis
  sample. The Go probe fixture's `test` step runs `go test -vet=off`, so
  that the variant without `typecheck` misses it.
- **Placement** (spec 0051, Q17). Each defect file goes into the first
  directory, in sorted path order, inside the component that qualifies
  (Go: holds a `_test.go` file; Python: holds a `test_*.py` or
  `*_test.py`; TypeScript: holds a `*.test.ts`, `*.spec.ts`,
  `*.test.tsx` or `*.spec.tsx`). Directories under `vendor/` and
  `node_modules/` do not qualify. When no directory qualifies, the
  component fails V-01 with "component <path> has no tests". V-02 and
  A-05 are then `cannot decide` for that component: there is nowhere to
  plant. If the sample's file name already exists, the part is `cannot
  decide`.
- **One defect at a time** (spec 0051), with a reset between defects.

### The rules

- **V-01** (#104). Execution: in the reset copy, `verify` exits 0.
  Probe: for each component and each defect kind, with that defect
  planted, `verify` exits non-zero. A failure names the component and
  each defect kind that did not make `verify` fail. When the baseline
  `verify` fails, the probe part is `cannot decide`: a defect that
  "made it fail" would prove nothing. A component with no tests fails
  the probe part whatever the baseline does, since that finding needs
  no run.
- **V-02** (#104). Probe: for each component and each defect kind,
  `verify-fast` exits non-zero. Precondition: `verify-fast` exits 0 in
  the reset copy. If it does not, V-02 is `cannot decide`, with the
  baseline's output. This precondition goes beyond spec 0051's wording
  and is approved with this spec. Without it, a `verify-fast` that
  always fails would pass V-02.
- **V-03** (#102). For each component: plant the unformatted-file
  defect, run `fmt-check`, then `git status --porcelain` in the copy
  must list exactly the planted file as untracked, with its content
  unchanged byte for byte. Any other path listed, or a changed planted
  file, fails V-03 and is named. `fmt-check`'s exit code is not read
  here (V-01 reads `verify`).
- **A-05, probe part** (#104). The failing-test defect of the first
  component, by path, is planted. Every Stop hook command gets a Stop
  payload with `stop_hook_active: false`. At least one must exit 2.
  Then each gets the same payload with `stop_hook_active: true`, and
  every one must exit 0. With no component that has tests, the part is
  `cannot decide`. With no Stop hook, it fails, as the static part does:
  no hook exits 2. Each run starts from a reset copy with the defect
  planted again.
- **A-07, execution part** (#102; spec 0052 noted it had no ticket).
  Every SessionStart hook whose matcher matches `startup` runs once in
  the reset copy. Each must exit 0, and `git status --porcelain` must
  then be empty.
- **D-01** (#102). For each component, in the component's directory,
  for each recognized lockfile found there:
  - `go.sum`: `go mod tidy`, then `git diff --exit-code go.mod go.sum`
  - `uv.lock`: `uv lock --check`
  - `poetry.lock`: `poetry check --lock`
  - `pnpm-lock.yaml`: `pnpm install --frozen-lockfile --lockfile-only`
  - `package-lock.json`: `npm ci --dry-run`

  The check runs as the program it names; a missing program is `cannot
  decide`. A non-zero exit fails D-01, with the output. **"Has
  dependencies"** decides the no-lockfile case. Go: `go.mod` has a
  `require` directive. Python: `pyproject.toml` declares a non-empty
  `[project].dependencies`, `[project].optional-dependencies`,
  `[dependency-groups]` or `[tool.poetry]` dependency table, not
  counting `python` itself. TypeScript: `package.json` has a non-empty
  `dependencies`, `devDependencies`, `optionalDependencies` or
  `peerDependencies`. A component with dependencies and no recognized
  lockfile fails without running anything. `go mod tidy` may need the
  module cache or the network. A failure for that reason is a D-01
  failure, and its output says why. Spec 0051's commands win over #102's
  shorter list (`--lockfile-only` for pnpm, poetry and npm added).
- **R-01, execution part** (#102). "`vibe` not on `PATH`" is done with
  a **shim**, not by removing directories: removing the directory that
  holds `vibe` would also remove every other tool installed beside it,
  such as `~/go/bin`. The shim directory is first on `PATH` for every
  run. It holds a copy of the running `vibe` executable, named `vibe`
  (`vibe.exe` on Windows). Started as the shim, the copy writes its
  arguments to a log in the audit's temporary directory and exits 127
  with "vibe is not available during the R-01 probe". R-01's execution
  part passes when the baseline `verify` exited 0 and every A-02 deny
  case was satisfied, both with the shim in place. Its findings list
  every recorded shim call, with the run it came from. A shim call made
  by another part (for example `fmt-check`) is listed as well. It fails
  only through the run it broke.
- **A-02** (#103). Every case of the contract's guard corpus is fed to
  every PreToolUse hook command whose matcher matches the case's tool,
  in the reset copy, with the unlock state the case names (below).
  - A deny case is satisfied when some hook exits 2, or when every hook
    allows and a guard exception for the case's rule ID matches it
    (spec 0051, "Guard policy and exceptions").
  - An allow case is satisfied when no hook exits 2.
  - A case whose tool no hook matches is not refused. A-01 already
    fails for that tool.
  - A failure names each case: its name, tool, command or path, the
    expected result and every hook's exit code.
  - A tracked `.claude/hooks/policy.local.json` is in the copy. If it
    lets a deny case through, A-02 fails, because only `policy.json`
    exceptions count. An untracked one is not copied (personal
    settings, spec 0051).
  - The report's "Guard exceptions" section lists, under each
    exception, the corpus cases it let through. The JSON gains
    `guard_exception_uses: [{rule, match, case}]` (schema stays 1: a
    new key).
- **A-03** (#103). Every deny case is fed again in two situations, and
  every one must be refused (exit 2). Exceptions do not count, because
  in both situations the guard cannot apply them.
  1. `PATH` holds only the operating system's directories: `/usr/bin`,
     `/bin`, `/usr/sbin` and `/sbin` on Linux and macOS;
     `%SystemRoot%\System32`, `%SystemRoot%`,
     `%SystemRoot%\System32\Wbem`,
     `%SystemRoot%\System32\WindowsPowerShell\v1.0` and
     `%SystemRoot%\System32\OpenSSH` on Windows. Bash is started by the
     absolute path resolved before.
  2. `.claude/hooks/policy.json` in the copy is replaced by the text
     `{` (invalid JSON). It is restored afterwards.

  A failure names the situation and each case that was not refused.
- **How A-02 and A-03 run the corpus** (decided during implementation
  of #103, 2026-10-17). One hook run is a bash and a guard process,
  about 0.2 seconds on Windows, so one case after another A-02 and
  A-03 would take minutes. Within each part, the cases with the same
  unlock state run concurrently, at most eight at a time (approved by
  the user on 2026-10-18); the unlock
  states run one after another, because the marker is one file, and
  the parts still run one after another. The R-01 shim calls made
  during these runs are listed under "the PreToolUse hooks during
  A-02" (or A-03), not under one case. The deny cases whose tool no
  hook matches are reported in one finding per tool, which names every
  such case; A-03 reports one finding per situation, which names every
  case it did not refuse with every hook's exit code.
- **A-08** (#106). The corpus's A-08 cases, all with guard rule ID
  `guardrail-settings`, run in their unlock states, and the Stop hook
  is checked:
  - With no marker, with an expired marker (one minute in the past)
    and with a valid marker (ten minutes ahead), a `Write` to
    `.claude/settings.json` and an `Edit` of a file under
    `.claude/hooks/` are refused, refused and allowed.
  - In every state, these are refused: a `Bash` and a `PowerShell` call
    that runs `vibe unlock`, a `Write` to the marker, and a `Bash` or
    `PowerShell` command that creates or changes the marker.
  - The marker is written by the audit into the copy's common Git
    directory, as spec 0051 says (one RFC 3339 line).
  - Then, with a valid marker in place, every Stop hook gets a Stop
    payload with `stop_hook_active: true`. Afterwards the marker must
    not exist. The copy is clean at that point, so `verify-fast`
    passes and the run is not a second A-05 probe.
  - Guard exceptions do not satisfy A-08. Spec 0051's A-08 check says
    "refused", and A-08 is a MUST rule. An exception for
    `guardrail-settings` therefore passes A-02 and fails A-08, and the
    finding says so.
  - How it runs (decided during implementation of #106, 2026-10-18):
    A-08's cases are the corpus cases with guard rule ID
    `guardrail-settings` and the allow cases named "A-08 (#106): …".
    They go through every PreToolUse hook that matches their tool, the
    same way as in A-02. Each case that fails gets its own finding,
    which names its unlock state. With no Stop hook, A-08 fails ("no
    Stop hook removes the unlock marker"). The Stop hooks run one after
    another in a reset copy, and their exit codes do not count. If a
    Stop hook did not end by itself and the marker is still there, the
    part is undecided.
- **Parts no longer unimplemented.** Every part of every rule now has a
  check function. The "not implemented in this vibe; #N adds it" path
  and the parts' issue numbers are removed. `--static` keeps its
  meaning: every execution and probe part is `not run`.

### The contract's guard corpus (#103, #105, #106)

- **It belongs to the contract** (spec 0051, Q5), not to a guard. It is
  data embedded in `vibe` and lives in `internal/contract`. Each case
  has `name`, `want` (`deny` or `allow`), a Claude Code `payload`
  (`tool_name`, `tool_input`), `rule` for deny cases (one of the ten
  guard rule IDs), and `unlock` (`none`, `valid` or `expired`; default
  `none`).
- **#103 derives it** from the existing corpus, which belongs to the old
  guards. It keeps the cases that:
  - carry a payload for one of the five A-01 tools (cases with raw
    `stdin`, or a tool such as `SomeNewTool`, test one guard's input
    handling, not the contract);
  - have no `local`, `local_text` or `policy` override (those test the
    old guards' spec 0050 and policy-version handling);
  - are not marked "known limitation". Those allow cases record what
    the old regex missed, such as `cmd /c rd /s /q` or a secret file
    without an extension. Keeping them would forbid a guard from
    refusing them.

  Each kept deny case is given its guard rule ID. Spec 0051 counts 162
  cases (101 deny, 61 allow); the old corpus has 172 since Phase 1
  added policy-version cases. The derived corpus's counts are recorded
  here before merge.

  **Derived counts.** The corpus is `internal/contract/guard_corpus.json`.
  Derived from the old corpus's 172 cases, it has 117 cases: 75 deny
  and 42 allow. Dropped: 33 with a `local`, `local_text` or `policy`
  override, 13 marked "known limitation" (12 allow and 1 deny), 6 with
  raw `stdin`, 2 for `SomeNewTool`, and 1 more (see below). #105 then
  added 108 cases, named "variant (#105): …" and "near miss (#105): …":
  89 deny and 19 allow. #106 added 38 cases, named "A-08 (#106): …",
  "unlock (#106): …" and "near miss (#106): …": 31 deny and 7 allow.
  The review fix of `guardrail-settings` (2026-10-18, below) turned its
  two absolute-path deny cases into relative ones and added two allow
  cases outside the repository. The corpus now has 265 cases: 195 deny
  and 70 allow. By guard rule ID (a test keeps this table equal to the
  corpus):

  | Guard rule ID | Deny cases |
  |---|---|
  | `rm-rf` | 52 |
  | `remove-item-recurse-force` | 29 |
  | `git-reset-hard` | 14 |
  | `git-clean-fd` | 13 |
  | `git-push-force` | 12 |
  | `git-branch-force-delete` | 13 |
  | `git-discard-worktree` | 22 |
  | `secret-file` | 6 |
  | `guard-policy-file` | 5 |
  | `guardrail-settings` | 29 |

  Decisions made in the derivation:
  - The one deny case marked "known limitation" (`gh issue create
    --body 'never run git reset --hard here'`) is dropped with the
    others. It recorded that the old regex refuses a pattern quoted
    inside an argument. Keeping it would force a shell-parsing guard to
    refuse a harmless command.
  - "known tool missing its field falls back to raw matching" (a `Bash`
    payload with `script` instead of `command`) is dropped. It tests
    one guard's input handling, as the `stdin` cases do: `vibe-guard`
    refuses it under `payload-error`, which is not a contract ID, so no
    contract rule ID fits it.
  - A payload keeps only `tool_name` and `tool_input`; the audit adds
    the other PreToolUse fields. The "Codex-shaped" cases therefore
    lose their Codex fields but keep their names, as every kept case
    does, so that they can be traced to the old corpus.
  - "guard policy: a backup next to it is allowed" (a `Write` of
    `.claude/hooks/policy.local.json.bak`) is a path under
    `.claude/hooks/`, which `guardrail-settings` refuses without an
    unlock. #106 gives it `unlock: valid`, so it still tests that
    `guard-policy-file` does not refuse the backup.
  - `guardrail-settings` is a path rule for exceptions, but its deny
    cases may be `Bash` or `PowerShell` calls: A-08's self-unlock cases
    (`vibe unlock`, a command that writes the marker) are refused under
    it. The corpus accepts that rule on every tool.
- **#105 adds** variant spellings the shell parser makes refusable. At
  least: separate and reordered flags (`rm -r -f`, `rm -f -r`,
  `rm --recursive --force`, `git push origin main -f`), quoting
  (`"rm" -rf`, `r\m -rf`), wrappers (`sudo`, `env`, `command`, `nohup`,
  `time`, `xargs`, `find -exec`), nested shells (`bash -c '…'`, `sh -c`,
  `eval`, `pwsh -c` and back), command substitution and subshells, and
  `guardrail-settings` cases with no unlock. Allow cases are added
  beside them for near misses (`rm -r` alone, `--force-with-lease`,
  `git clean -n`).
- **#106 adds** the A-08 cases above.
- **Old guards.** The old corpus file and its tests stay unchanged for
  the Go, Python and Node guards until #112 (Q13). The A-02 fixture
  tests run each old guard through the audit. Each old guard's test
  asserts the exact list of corpus cases added by #105 and #106 that it
  does not refuse: the audit fails A-02 on them and names them, as it
  should. The cases taken over by #103 must all pass, which is #103's
  done-when.

### Guard rule IDs and exceptions, shared

- A new package, `internal/guardrule`, holds the ten guard rule IDs of
  spec 0051, the `policy.json` version 2 exception format, and exception
  matching. `internal/contract` and the guard both import it, so the
  audit and `vibe-guard` cannot disagree about what an exception
  matches. It imports no old package.
- **What `match` means is decided by the rule ID.** For the path rules
  (`secret-file`, `guard-policy-file`, `guardrail-settings`), `match` is
  a path glob (`*`, `?`, `**`). It is matched against the file path,
  made relative to the repository root and slash separated when it is
  inside the root, and as given otherwise. For the command rules,
  `match` is parsed into words like a command, and it matches when its
  words are a prefix of the words of a simple command the rule refused.
  The tool decides how those words are found: POSIX/Bash parsing for
  `Bash`, PowerShell tokenizing for `PowerShell`. Spec 0051 says
  "a command prefix or a path glob" without saying which applies when;
  this is that decision.
- An exception lifts only its own rule. A command that another rule
  also refuses stays refused.
- **The command decomposition lives in `internal/guardrule` too**, not
  in `internal/guard` as "Parsing" below implies (decided during
  implementation, 2026-10-17). Exception matching needs the simple
  commands and their words, and the audit and `vibe-guard` must find
  the same ones, so the Bash parsing (`syntax.LangBash`), the
  PowerShell tokenizer, wrapper stripping, nested parsing to depth 8
  and the unknown-word reading are in `guardrule`, which therefore
  imports `mvdan.cc/sh/v3/syntax` (ADR 0025, decision 8; already a
  dependency). The baseline rules themselves stay in `internal/guard`.
  How an exception matches a command, in detail: the match is parsed
  in the command's language as one simple command with no expansion;
  its first word compares with the command name as a program name (last
  path element, lower-cased, without `.exe`) and, in PowerShell, also
  with aliases resolved; the other words compare after quote removal,
  in PowerShell without case; a word of the command that holds an
  expansion never matches. For the audit, an exception matches a corpus
  case when it matches some simple command of the case's command.

### `vibe-guard` (#105)

- **A separate binary** from the same module, `cmd/vibe-guard`, with
  its logic in a new package, `internal/guard`, which imports
  `internal/guardrule` and `mvdan.cc/sh/v3/syntax` (ADR 0025, decision
  8). It imports nothing from `internal/contract` or the old packages.
  It is built from source here. Release binaries are #107.
- **Commands.**
  - `vibe-guard hook`: the PreToolUse hook. It reads the payload on
    stdin and decides it. Exit 0 allows, exit 2 refuses with one line on
    stderr: `vibe-guard: refused by rule <id>: <why>. Explain: vibe-guard
    explain '<command>'`. An `ask` rule exits 0 with Claude Code's
    `permissionDecision: "ask"` JSON on stdout.
  - `vibe-guard explain [--tool <tool>] '<command or path>'`: prints the
    decision, the rule ID, the simple command and words (or the path)
    that matched, and any exception that applied. It reads the policy of
    the repository it is run in, and exits with the code `hook` would.
    The default tool is `Bash`, or `Write` when the argument has no
    spaces and names an existing path.
  - `vibe-guard relock`: deletes the unlock marker. The Stop hook runs
    it.
  - `vibe-guard version`.
- **Repository root**: `CLAUDE_PROJECT_DIR`, otherwise the payload's
  `cwd`, walked up to the directory holding `.git`. The common Git
  directory is found by reading `.git` itself (a directory, or a
  `gitdir:` file and the `commondir` file it points to), not by running
  `git`. The guard therefore works with only the operating system's
  `PATH` (A-03), and a missing Git cannot make an unreadable marker look
  valid.
- **Parsing.**
  - `Bash` commands are parsed with `syntax.LangBash`, because the Bash
    tool runs bash. The audit's parsing stays POSIX (ADR 0026).
  - `PowerShell` commands go through a small PowerShell tokenizer
    written here, not a dependency. It handles single and double quotes,
    backtick escapes, `;`, `|`, `&&`, `||`, script blocks, `&` and `.`
    invocations, and `$(…)` subexpressions. Command names and
    parameters are matched case-insensitively, with PowerShell's
    aliases (`rm`, `ri`, `del`, `erase`, `rd`, `rmdir`) and parameter
    prefixes (`-Rec`, `-Fo`, `-Recurse:$true`).
  - Simple commands are found everywhere: lists, pipelines,
    conditionals, loops, subshells, functions, command and process
    substitutions. Wrappers are stripped: `sudo`, `env` with its
    assignments, `command`, `builtin`, `exec`, `nohup`, `time`, `nice`,
    `xargs` and `find -exec`/`-execdir`/`-ok`. Strings passed to
    `bash -c`, `sh -c`, `zsh -c`, `eval`, `pwsh -c`/`-Command` and
    `powershell -Command` are parsed again with the matching language,
    up to a depth of 8. This is stricter than the audit's "called",
    which does not parse `sh -c` strings again: the guard must not miss
    a command, and the audit must not invent a call.
  - **Fails closed.** A command that does not parse, nesting deeper
    than 8, a payload that is not JSON or lacks its tool's input, an
    empty stdin, and an unreadable or invalid `policy.json` are all
    refused. The refusal names a guard-internal rule ID: `parse-error`,
    `payload-error` or `policy-error`. These IDs are not contract IDs
    and cannot have exceptions (A-04 accepts only the ten).
  - **Unknown words.** An unquoted expansion can become any number of
    words, including flags. When a rule reads a word that holds one, the
    word counts as matching, unless the rule's known words already rule
    the match out. A quoted expansion is one word and is treated as an
    operand. An expansion in the command name refuses under
    `dynamic-command`, a guard-internal ID.
  - **Decided during implementation (#105, 2026-10-17).** A quoted
    expansion is never a flag, but where a rule reads an operand's
    value (a `+refspec` for `git-push-force`, a whole-tree pathspec for
    `git-discard-worktree`) it counts as matching unless its known text
    rules the match out: `git push origin "$ref"` is refused,
    `git push origin "feature-$n"` is not. A POSIX shell given no `-c`
    string, no script file and no here-document (`curl … | bash`) runs
    a script the guard cannot see, and is refused under
    `dynamic-command`; `bash script.sh` is allowed, as before.
    `timeout` and `stdbuf` are stripped as wrappers too. (These two
    decisions, `curl … | bash` refused under `dynamic-command` and
    `timeout`/`stdbuf` stripped, were approved by the user on
    2026-10-18.) Still not
    read: `cmd /c`, `Start-Process`, `watch`, shell aliases, and glob
    expansion into flag-like file names. A missing `policy.json` is
    the default policy (enforce, no rules, no exceptions; approved by
    the user on 2026-10-18); an
    unreadable or invalid one refuses every call that reaches the
    guard, other tools included. Observe mode applies to payload and
    parse errors too; a policy error is always refused, since its mode
    cannot be read. `ask` rules still ask in observe mode. The refusal
    line adds `--tool <tool>` to the explain command for every tool
    other than `Bash`.
- **Tools.** `Bash` and `PowerShell` are commands. `Write`, `Edit`,
  `MultiEdit` and `NotebookEdit` (`notebook_path`) are file edits. Any
  other tool is allowed, because the settings' matcher decides what
  reaches the guard.
- **Baseline rules** are built in, one per guard rule ID of spec 0051,
  plus `guardrail-settings` as below. `policy.json` cannot remove them.
- **`guardrail-settings`.** A file edit of the repository's
  `.claude/settings.json`, or of any path under its `.claude/hooks/`,
  is refused unless the marker is valid. Spec 0051's `guard-policy-file` still refuses
  `.claude/hooks/policy.json` and `policy.local.json` even then: an
  unlock opens settings and hooks, not the guard's own policy. These
  are always refused, whatever the marker says:
  - a simple command whose words start with `vibe unlock`;
  - a file edit of a path named `vibe-unlock` inside a `.git` directory
    or the common Git directory;
  - a command that names `vibe-unlock` as a word or inside a word, such
    as a redirection target, `touch`, `cp`, `mv`, `tee`, `Set-Content`
    or `Out-File`.

  Decided while building it (#106):
  - These three always-refused cases are refused under
    `guardrail-settings`, and no exception lifts them: an exception
    for `guardrail-settings` lifts only edits of settings and hooks.
    (The audit's A-08 counts no exception at all.)
  - A `vibe` whose first word after its flags holds an expansion
    (`vibe $cmd`) may be `vibe unlock`, so it is refused too. `go run
    …/cmd/vibe unlock` is not recognised.
  - The marker's name and the `.git` element are compared without
    case. A path is the marker when its last element is `vibe-unlock`
    and an element above it is `.git`, or when its directory is the
    Git directory or the common Git directory (a bare common directory
    such as `repo.git`).
  - The guard's settings and hooks match is case-insensitive, as
    `guard-policy-file` is: `.claude/settings.json` and any path with
    an element after `.claude/hooks/`. Corrected in review (2026-10-18,
    a bug): spec 0051 names the repository's files, so only paths
    inside the repository root match, read as an exception glob reads
    them (relative to the root, slash separated). The person's own
    `~/.claude/settings.json`, or a `.claude/` directory below the
    root, is not refused. Two corpus cases with absolute paths outside
    any repository (`/repo/…`, `C:\repo\…`) became relative paths; the
    corpus gained two allow cases for files outside the repository, and
    `internal/guard` a regression test.
  - A PowerShell expression redirected to a file (`'x' > f`) is a
    redirect-only command in `internal/guardrule`, as `> f` is in Bash.
    Before, the PowerShell tokenizer skipped it.
- **`policy.json` version 2.** The contract reads only `version` and
  `exceptions` (spec 0051). `vibe-guard` also reads:
  - `mode`: `enforce` (the default) or `observe`;
  - `rules`: the repository's own rules. Each has an `id` (not one of
    the ten contract IDs and not a guard-internal ID), a `level`
    (`deny` or `ask`), optional `tools` (default: all), a `reason`, and
    exactly one of `command` (a word prefix, as exceptions use) or
    `path` (a glob). A command rule may add `args`, a regular
    expression that some remaining argument must match.

  Unknown keys and invalid values are a policy error, so everything is
  refused, with a message that names the key. A typo cannot silently
  turn a rule off. `vibe-guard` does not read `policy.local.json`.
  Personal loosening goes through `.claude/settings.local.json`, which
  spec 0051 leaves to the person.
- **Observe mode** logs instead of refusing. Every call that would be
  refused exits 0, with "vibe-guard (observe): would refuse by rule
  <id>: …" on stderr and the same line appended to `vibe-guard.log` in
  the common Git directory. Committing `mode: observe` fails A-02, which
  is the intended price of trying the guard out. The documentation says
  so.
- **No network.** `vibe-guard` opens no network connection. The
  documentation promises it, and CI runs the guard's corpus tests on
  Linux in a network namespace with no interfaces (`unshare -rn`). The
  test binary is built first, then run inside the namespace.
- **The asymmetry is documented** in `docs/usage.md`. To tighten, add
  deny rules to `policy.json`, or add another PreToolUse hook in
  `settings.json`. To loosen, only exceptions in `policy.json`.

### The reference hook commands

- **PreToolUse** (Q12, ADR 0025 decision 9): `command -v vibe-guard
  >/dev/null 2>&1 || { echo "vibe-guard is not installed; install it:
  go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@<version>"
  >&2; exit 2; }; vibe-guard hook`. `command -v` is a shell builtin, so
  the check works with only the operating system's `PATH` (A-03). The
  matcher is `Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit`.
  `<version>` is the running `vibe`'s version, or `latest` for a build
  `go install` cannot fetch ("This repository" below).
- **Stop**: `vibe-guard relock 2>/dev/null; task verify:fast 1>&2
  </dev/null || task -x hook:done`. The relock comes first, so it runs
  whatever `verify-fast` does. It keeps the simple command Phase 1's
  A-05 static part reads.
- `vibe-guard` is not `vibe`, so R-01's static part does not flag these
  commands.

### `vibe unlock` (#106)

- `vibe unlock --minutes N`, with N from 1 to 240 (default 15). It writes
  `vibe-unlock` into `git rev-parse --git-common-dir` of the repository
  at `--repo-root`. The file holds one line: the expiry, in RFC 3339
  UTC (spec 0051). The command prints the expiry and that the agent's
  next Stop will lock again. Running it again replaces the expiry. To
  lock early, run `vibe-guard relock`. An N out of range, or a
  `--repo-root` where `git rev-parse` fails, is an error and writes
  nothing. The expiry is written to the second.
- The marker lives in the common Git directory, so every linked
  worktree of the repository shares one unlock.
- `vibe unlock` is meant for a person's own terminal. A `Bash` or
  `PowerShell` tool call that runs it is refused (A-08). Whether
  Claude Code's `!` prefix passes through PreToolUse hooks is not
  verified. The documentation tells people to unlock from their own
  terminal.

### This repository

- **Opt-in through the old templates** (decided with the user on
  2026-10-17). The v1 `vibe.yaml` gains `policy.agent_guard`. Its values
  are the standard's runtime guard (the default, unchanged output) and
  `vibe-guard`. With `vibe-guard`, the claude module renders:
  - the PreToolUse and Stop commands above;
  - `policy.json` as version 2 with `mode`, `rules: []` and
    `exceptions: []`;
  - no `.claude/hooks/guard.go` and no `hook:guard` task.

  The other standards, every default and the example repositories
  render byte-for-byte as before. The existing module tests prove that.
  This is a small change to code #112 deletes. It is needed because
  this repository's `.claude/` and `Taskfile.yml` stay managed until
  #111. The opt-in becomes the frozen, repository-owned file at #111.
  Decided during implementation (2026-10-18):
  - The values are `runtime` and `vibe-guard`; an absent key is
    `runtime`. Only `prod-go` offers the key, since only this repository
    uses it; the other standards reject it as a policy they do not offer.
  - The install hint in the PreToolUse command names `@<version>`, the
    version of the `vibe` that renders it, as "The reference hook
    commands" says. (An implementation that named a fixed `@latest` was
    reverted on 2026-10-18, by the user's decision.) A version `go
    install` cannot fetch names `@latest` instead: `dev` from an
    unstamped build, `(devel)`, an empty version, or one with build
    metadata such as `+dirty`. A pseudo-version names `@latest` too: `go
    build` stamps it from the commit, so a repository that builds `vibe`
    from source (this one's Conformance / audit job does) would render a
    different hint on every commit and fail its own audit. Naming
    pseudo-versions did exactly that on this spec's pull request, and
    was corrected on 2026-10-18. Only a release is named as it is, so
    the settings change with each release that syncs them.
  - The generated `Taskfile.yml` keeps a four-line comment where
    `hook:guard` was. The vibeconform skill's "agent guard" paragraph
    names `vibe-guard` and `vibe unlock` instead of `hook:guard`.
    `policy.local.json` is not checked with `vibe-guard`, which does not
    read it, and `vibe doctor` names `vibe-guard` instead of `go` as the
    hook runtime.
  - The claude module imports `internal/guard` for the reference
    commands, so the settings and the guard cannot disagree. That is an
    old-to-new import; #112 deletes the module.
- This repository's `vibe.yaml` sets `policy.agent_guard: vibe-guard`.
  `vibe sync` regenerates the files, and `.vibe/state.yaml` is
  committed with them. Developers install `vibe-guard` with `go install
  ./cmd/vibe-guard`. The hook's message says how, and
  `docs/usage.md` does too.
- **The guard canary** in `docs/usage.md` is updated for `vibe-guard`,
  and AGENTS.md's "Guardrails" section names `vibe-guard`, its policy
  and `vibe unlock`.
- **CI.**
  - `contract-audit.yml` gains a second job, "Contract / audit". It
    builds `vibe` and `vibe-guard` from source, installs the tools the
    entrypoints need (as `ci.yml` does), and runs `vibe audit --config
    vibe.v2.yaml`. It passes on exit 0. It is not a required check
    (#111). The static job stays.
  - `hook-guard.yml` gains a `vibe-guard` job on Linux and Windows. It
    runs the contract corpus through the exact PreToolUse command of
    this repository's settings. On Linux it also runs the corpus with
    the network disabled.
  - Decided during implementation (2026-10-18): the "Contract / audit"
    job sets `VIBE_TEST_REQUIRE_TOOLS=bash,uv,pnpm,python,node`, since
    the `verify` it audits runs the tests with the uv, pnpm and old-guard
    fixtures; `ci.yml`'s test job does not, because `ci.yml` is a
    generated file shipped to adopters (this departs from "Testing
    Decisions", which names the test job too; approved by the user on
    2026-10-18). The "Contract / audit" job's `timeout-minutes` is 60
    (review, 2026-10-18; it was 300): the audit runs one full `verify`
    in the probe copy, after which Go's test cache makes the
    planted-defect reruns short, so it is expected to take well under
    30 minutes. Its real time on CI is not yet measured. The
    network-disabled job lifts
    Ubuntu's AppArmor restriction on unprivileged user namespaces
    before `unshare -rn`. `hook-guard.yml`'s `prod-go` leg now runs the
    old corpus in a repository synced to `prod-go`'s defaults, since
    this repository has no `hook:guard` any more.
- **Carried over from Phase 1.** The Stop hook command and the anchored
  matcher reading are checked in a real Claude Code session on Linux
  and on Windows: a refusal shows its rule ID, the Stop hook blocks
  once and then lets the stop through, an unlock opens and relocks, and
  a `MultiEdit` is or is not seen by a `Write|Edit` matcher. Each result
  is recorded here and in the pull request's ledger. A check that
  cannot be run stays UNVERIFIED and says so.

### Report and CLI

- New flags on the contract engine: `--probe-timeout <duration>`
  (default `30m`) and `--keep-probe-copy`. With a v1 configuration they
  are an error, as `--static` is (spec 0052).
- `--explain` for V-01 and V-02 prints the defect samples. For A-02 and
  A-03 it prints the corpus counts by rule ID.
- The verdict now reaches "conformant" or "not conformant" without
  `--static`. Exit codes are unchanged (spec 0052).
- Failing probe findings name a file to change, as spec 0052 requires:
  `vibe.yaml` for an entrypoint that misses a defect,
  `.claude/settings.json` for a hook, the lockfile or manifest for
  D-01, and the policy for an exception.

### Documentation

- `GLOSSARY.md`: "probe copy" and "planted defect"; "agent guard" names
  `vibe-guard` as the reference guard; "unlock" names `vibe unlock` and
  `vibe-guard relock`.
- `docs/usage.md`: running the full audit, its flags, time and tools;
  `vibe-guard` (install, `hook`, `explain`, `relock`, observe mode,
  `policy.json` v2 rules, the asymmetry, no network); `vibe unlock`.
- `docs/architecture/overview.md`: `internal/guardrule` and
  `internal/guard`, `cmd/vibe-guard`, and the dependency direction:
  `internal/contract` → `internal/guardrule` ← `internal/guard`, and
  neither of the new packages imports the old ones.
- No new ADR. `mvdan.cc/sh` for `vibe-guard` is ADR 0025 decision 8,
  and the PowerShell tokenizer is code, not a dependency. If
  implementation needs a new production dependency (for example, for
  job objects on Windows), it gets an ADR first.

## Testing Decisions

Good tests here check behaviour through the two seams below, with real
tools and real processes. No test calls a check function, the probe
runner or the guard's matcher directly.

- **Seam 1: the `vibe audit` command, in-process** (as Phase 1). Tests
  build the root command, point `--repo-root` and `--config` at a
  fixture Git repository, and assert the exit code, the rule and part
  results in `--json`, and, for whole reports, text against goldens.
  `vibe unlock` uses the same seam. Prior art: Phase 1's fixture helper
  (testdata tree, `git init`, per-test edits) and the CLI's `TestMain`,
  which clears `GIT_DIR` and its relatives.
- **Seam 2: the `vibe-guard` command, as a subprocess.** Tests build the
  binary once, feed payloads on stdin, and assert the exit code, the
  rule ID on stderr, `ask` JSON on stdout, `explain` output, observe
  mode and its log, and `relock`. Prior art: the old corpus test, which
  runs each guard as a subprocess per case.
- **Fixture projects that need only Go.** The Go probe fixture is a tiny
  module whose entrypoints are `go run ./ci <step>`. The `ci` program
  is part of the fixture: `fmt-check` fails when `gofmt -l` lists a
  file, `typecheck` runs `go vet`, and `test` runs `go test`. Variants
  flip one thing:
  - `verify` without the test step: V-01 fails and names "test", which
    is #104's done-when.
  - Without vet: names "static analysis".
  - Without fmt-check: names "format".
  - A `fmt-check` that writes (`gofmt -w`): V-03 fails.
  - A `verify-fast` that misses a defect: V-02 fails.
  - No `_test.go`: "component . has no tests".
  - Hook commands are small `go run` programs, guarded with
    `command -v go || exit 2`, so that A-03 behaves.
- **Python and TypeScript fixtures** (uv with ruff and pytest; pnpm with
  tsc, prettier and Vitest) run when `uv` or `pnpm` is on `PATH` and
  skip otherwise. `VIBE_TEST_REQUIRE_TOOLS=uv,pnpm` turns a skip into a
  failure. The full-audit CI job sets it; `ci.yml`'s test job does not
  ("This repository", CI; approved by the user on 2026-10-18).
- **D-01 without the network.** Go fixtures use a `replace` to a module
  inside the fixture, so `go mod tidy` needs no download. Passing: tidy
  changes nothing. Failing: an unused `require` that tidy removes. No
  lockfile with dependencies fails without any tool. Python and
  TypeScript lockfile cases run with the tool gate above.
- **Guard probe fixtures.** A fixture's `.claude/settings.json` calls the
  `vibe-guard` the tests built (on `PATH` for the test). The cases are:
  - It passes A-02, A-03 and A-08.
  - A deliberately broken guard, a fixture program that allows
    `rm -r -f`, fails A-02, and the report names that case (#103's
    done-when).
  - A guard with `mode: observe` fails A-02.
  - An exception lets its case through and is listed; an exception for
    `guardrail-settings` passes A-02 and fails A-08.
  - A tracked `policy.local.json` that loosens fails A-02.
  - A hook without the install check fails A-03.
  - A Stop hook without `vibe-guard relock` fails A-08.

  The old Go guard runs through `go run`. The Python and Node guards run
  when their runtimes are present, with the same tool gate. Each
  asserts its exact list of missed new cases, as decided above.

  Seam deviation (decided during implementation of #103, 2026-10-17):
  each of these fixtures runs hundreds of hook processes, and with them
  `internal/cli`'s tests passed `go test`'s ten-minute default. They
  therefore live in `internal/contract` (`guardprobe_test.go`, package
  `contract_test`), which has a budget of its own, and call
  `contract.Audit`, the engine `vibe audit` runs, with only the rules
  each needs. What the command adds, the findings in `--json`,
  `guard_exception_uses` and the text report's listing under each
  exception, is tested through the `vibe audit` seam in `internal/cli`
  with fixtures that run no hook.

  Decided later (2026-10-18), for the same budget: the fixtures that run
  real tools for A-07, V-03, D-01 (`go mod tidy`), R-01, `setup` and the
  planted defects of V-01, V-02 and A-05 moved from `internal/cli` to a
  test-only package, `internal/contract/probetest`, which calls
  `contract.Audit` with only the rules each test asserts (a test of R-01
  with Claude Code also runs A-02, whose verdict R-01 reads). It reuses
  `internal/cli`'s probe fixtures in testdata. In `internal/cli` the
  configuration tests run `--static`, and the rule and configuration
  case tables run their cases in parallel. What stays there is what the
  command adds: `TestContractAuditNeverWrites` (the whole registry),
  `--keep-probe-copy`, progress lines, the A-02 and A-08 report
  listings, and the D-01 lockfile cases that need no tool. Raising
  `-test.timeout` from a package's `TestMain` does not help: `go test`
  kills the test binary one minute after its own `-timeout`.
- **R-01.** A fixture whose `verify` calls `vibe` fails R-01's execution
  part and lists the shim call. The same fixture without that call
  passes.
- **Every execution and probe part** has a passing and a failing
  fixture. `cannot decide` has a case for each cause: setup failed, tool
  not found, no bash, timeout (with a tiny `--probe-timeout`), a word
  with an expansion, nowhere to plant, and a baseline that fails.
- **The audit never writes.** A test hashes the fixture's working tree
  and `.git` before and after a full audit, and asserts that nothing
  changed and that the probe copy is gone.
- **`vibe-guard` corpus.** The whole contract corpus runs through seam
  2, on Linux and Windows. Every case's refusal names the case's rule
  ID, and `explain` gives that rule for every deny case (#105's
  done-when). Guard-only cases are in the guard's own testdata: parse
  errors, payload and policy errors, each `policy.json` v2 field and
  typo, `ask`, observe mode, and nesting depth.
- **The network-disabled run** (`unshare -rn` on Linux CI) and the
  missing-`vibe-guard` run (through the reference hook command with
  `PATH` reduced, so that every case is refused) are separate tests,
  and CI runs both.
- **Template opt-in.** The claude module's tests render both values of
  `policy.agent_guard`. The existing goldens and corpus tests for the
  default stay unchanged and pass, and `task audit` passes after
  `vibe sync`.
- **Goldens are LF**, as in Phase 1.
- **Real integration** is separate in the ledger: the "Contract /
  audit" job on this spec's pull request, the `vibe-guard` jobs in
  `hook-guard.yml`, and the manual Claude Code session checks above.

## Out of Scope

- Releasing `vibe` and `vibe-guard` as prebuilt binaries, and the setup
  action (#107).
- Making the full audit a required check, renaming `vibe.v2.yaml`, and
  freezing the generated files (#111).
- Removing the Go, Python and Node guards, their corpus and templates,
  and the old system (#112). Their defaults and the example
  repositories are unchanged here.
- The setup skill writing `vibe-guard` settings for adopters (#108).
- Reading a hook's JSON `permissionDecision` as a refusal, running
  entrypoints through a shell, and recognizing more lockfiles. Each is a
  later MINOR release.
- Codex hooks (spec 0051: Codex is exempt from the A rules).
- Parallel probe parts. The parts run one after another. Within A-02
  and A-03, the corpus cases of one unlock state run concurrently, up
  to eight at a time ("How A-02 and A-03 run the corpus"; approved by
  the user on 2026-10-18).
- Measuring `verify-fast`'s speed (spec 0051).

## Further Notes

- **Where the tickets differ from spec 0051, spec 0051 wins:**
  - #102 says V-03's `git status --porcelain` "must be empty"; spec 0051
    says it lists only the planted file.
  - #102 lists `pnpm install --frozen-lockfile`; spec 0051 adds
    `--lockfile-only` and the poetry and npm checks.
  - #102 says "vibe removed from PATH"; this spec does that with a shim,
    which has the same effect without removing other tools.
  - #103 counts 162 corpus cases; the old file now has 172, and the
    contract corpus is derived by the rule above.
- **A-07's execution part** is added to #102, as spec 0052 asked.
- **Interpretations beyond spec 0051**, approved with this spec:
  - the probe copy is a fresh Git repository;
  - entrypoints run without a shell, and hooks run through bash;
  - a missing tool is `cannot decide`;
  - baseline preconditions for V-01's probe part and for V-02;
  - A-05 uses the failing-test defect of the first component;
  - the R-01 shim;
  - "has dependencies" for D-01;
  - exception `match` semantics by rule ID;
  - guard exceptions do not satisfy A-08;
  - the contract corpus drops the old guards' "known limitation" allow
    cases.
- **`ask` is not refusal.** A repository's own `ask` rule never counts
  as refusing a deny case. The baseline rules are all `deny`.
- **Running time.** A full audit runs `verify` 1 + 3 × components times
  and `verify-fast` 1 + 3 × components times, the Stop hook three times,
  and D-01's checks. On this repository that takes several minutes,
  which ADR 0025 expects. `--static` stays the quick local run.
- **Hidden assumptions to verify in implementation:** that Claude Code
  sends the payload fields listed above; that it runs hooks on Windows
  in Git Bash; that `CLAUDE_CODE_GIT_BASH_PATH` is the variable it
  reads; and that its default hook timeout is 60 seconds. Each is
  checked against Claude Code's documentation, and this spec is
  corrected where it is wrong.

  **Checked on 2026-10-18** against
  https://code.claude.com/docs/en/hooks and
  https://code.claude.com/docs/en/env-vars:
  - Payload fields: confirmed. Every hook gets `session_id`,
    `transcript_path`, `cwd` and `hook_event_name` (and more the audit
    does not send and no check reads, such as `permission_mode`);
    PreToolUse adds `tool_name`, `tool_input` and `tool_use_id`; Stop
    adds `stop_hook_active` ("true when Claude Code is already
    continuing as a result of a stop hook"); SessionStart's `source`
    is `startup` for a new session, which is also its matcher value.
    Exit 2 blocks a PreToolUse call and keeps a Stop from stopping.
  - Git Bash on Windows: confirmed. A command hook's shell form runs in
    Git Bash on Windows, and in PowerShell only when Git Bash is not
    installed. On Linux and macOS the page says both that the `shell`
    field defaults to `bash` and that the shell form is passed to
    `sh -c`; the audit keeps `bash -c`, and which one Claude Code runs
    there stays UNVERIFIED (the reference commands are POSIX, so both
    read them the same).
  - `CLAUDE_CODE_GIT_BASH_PATH`: confirmed, with a correction. It is
    the path to `bash.exe`, Windows only. When the path does not exist
    or its file is not named `bash.exe`, `sh.exe`, `bash` or `sh`,
    Claude Code ignores the variable and finds Git Bash as if it were
    unset. The audit made every hook part `cannot decide` instead; it
    now ignores such a value too (regression test
    `TestProbeIgnoresAnInvalidGitBashPath`).
  - Default timeout: wrong. Command hooks default to 600 seconds on
    most events, PreToolUse, Stop and SessionStart included (30 on
    `UserPromptSubmit`, which the audit does not run). The audit's
    default is now 600 seconds; "How the audit runs commands" is
    corrected.

## Acceptance Criteria

1. `vibe audit` without `--static` runs every execution and probe part
   of contract 1.0.0 in a probe copy. No part reports "not implemented".
2. The audit leaves the repository's working tree, index and `.git`
   unchanged and deletes the probe copy (a test proves both).
3. V-01, V-02, V-03, D-01, A-02, A-03, A-05 (probe), A-07 (execution),
   A-08 and R-01 (execution) each have a passing and a failing fixture
   test (#102, #104 and #106 done-when). Amended, approved by the user
   on 2026-10-18: the fixtures that run real tools or guards may call
   `contract.Audit`, the engine `vibe audit` runs
   (`internal/contract/probetest`, `internal/contract/guardprobe_test.go`),
   and each part also has at least one light test through the `vibe
   audit` seam (`internal/cli`: `TestProbePartsLight`, `TestRuleD01`,
   `TestRuleA02Reporting`, `TestRuleA08Reporting`).
4. With the tests removed from `verify`, V-01 fails and names "test"
   (#104).
5. The Go, Python and Node guards pass every corpus case taken over by
   #103. A deliberately broken guard fails A-02 and the report names
   the case (#103).
6. The contract corpus is embedded in `vibe`; every deny case has a guard
   rule ID; its counts are recorded in this spec.
7. `vibe-guard` passes the whole contract corpus on Linux and Windows,
   including the variant spellings and A-08 cases. Every refusal names
   its rule ID, and `explain` gives the correct rule for every case
   (#105).
8. The corpus passes with the network disabled. With `vibe-guard`
   missing from `PATH`, the reference hook command refuses every corpus
   case (#105).
9. `vibe unlock` probes cover no marker, a valid marker and an expired
   marker. Every self-unlock case is refused. The Stop hook removes the
   marker (#106).
10. `internal/guardrule` and `internal/guard` import no old package, and
    `internal/contract` still imports none.
11. This repository runs `vibe-guard` through `policy.agent_guard`, and
    every other standard and example renders unchanged. `task audit`
    passes.
12. The "Contract / audit" job runs the full audit on this spec's pull
    request and reports its verdict. It is not a required check. Any
    MUST rule it fails is recorded in this spec with the reason.
13. The real Claude Code session checks (Linux and Windows) are recorded
    as PASS, FAIL or UNVERIFIED.
14. `GLOSSARY.md`, `docs/usage.md`, `docs/architecture/overview.md` and
    AGENTS.md's "Guardrails" section are updated.
15. `task verify` and `task audit` pass.
