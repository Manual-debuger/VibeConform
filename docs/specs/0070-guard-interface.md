# Spec 0070: A deeper `Guard`, one way to decide a call, and the guard's tests in-process

Status: draft. Issue #163, the refactor part (items 1, 4 and 6). Part of
the design review #167 (guard track). Spec 0069 fixed the bugs of #163
(items 2, 3 and 5). Mostly a refactor, with two small behaviour fixes in
`vibe-guard explain`. Amends spec 0053 ("Testing Decisions", "Guard rule
IDs and exceptions, shared"). No contract change: the contract stays
1.2.0 and the guard corpus is unchanged.

## Problem

`vibe-guard` works, but its main type is hard to use correctly, the two
readings of a guard exception can drift apart without a test noticing,
and its tests are slow enough that the quick runs skip the corpus.

**1. `Guard` is a shallow interface.**

- Its state is exported and mutable: the repository, the policy, the
  policy error, the clock and the payload's directory. A caller must
  know the rules that tie them together: the policy is the default when
  there is a policy error, and the directory is set after `New`.
- `hook` sets the directory after `New`. `explain` never sets it, so a
  relative path in `explain` is read against the project root, while
  its default-tool test (`os.Stat`) reads the same path against the
  process's working directory.
- There are two decide methods, `Decide` and `DecideError`, and both
  repeat the policy-error short-circuit and the observe flag.
- `hook` and `explain` each work out from the outcome and the observe
  flag whether the call is refused and which exit code follows.

**4. The audit and the guard read an exception in two ways.**

- `vibe-guard` lifts a rule for the simple command that rule refused,
  or for the path it refused. It never lifts a fixed rule (the unlock
  marker and the self-unlock command). Since spec 0058, a path exception
  also lifts a shell write.
- The audit (A-02, A-09) reads an exception through
  `Exception.Matches`: it counts when its words prefix *some* simple
  command of the case, and a path exception counts for edit tools only.
  Spec 0053 chose this on purpose: the audit checks a guard as a black
  box and cannot know which simple command a rule refused.
- On today's corpus, every lift that `vibe-guard` makes is also one the
  audit accepts. Checked by reading on 2026-11-25: the corpus has no
  shell-write deny case for `guard-policy-file`, and its shell-write
  cases for `guardrail-settings` all write the unlock marker or run
  `vibe unlock`, which are fixed rules. No test keeps it so. A new
  corpus case for a shell write of `.claude/settings.json` would make
  the audit fail a repository whose `vibe-guard` lifts it by a valid
  exception.
- The audit is wider than the guard in the other direction: an
  exception that matches another simple command of a compound case
  (`{rm-rf, "cd /tmp"}` on `cd /tmp && rm -rf`), or names a fixed rule,
  counts for A-02. That makes the audit accept a third-party guard that
  lets the case through. Closing it tightens A-02, which is a MAJOR
  change under spec 0051's versioning rules, and is out of scope here
  (see "Further Notes").

**6. The guard's tests run out of process only.**

- Every guard test runs `vibe-guard` as a subprocess. The 278-case
  corpus test and the reference-hook tests are skipped under `-short`,
  so `task verify:fast` and the `Stop` hook never run the corpus.
- `Env` already injects stdio, `Getenv`, the working directory and the
  clock, but two reads bypass it: `explain` stats its argument against
  the process's working directory, and a relative `CLAUDE_PROJECT_DIR`
  is made absolute against the process's working directory.
- `vibe unlock` reads the wall clock directly, so its tests cannot
  check the exact expiry it writes.

## Solution

- `Guard` keeps its state unexported. `New` takes the repository and
  options (the clock and the payload's directory) and loads the policy.
  The only way in is one decide method, which takes a call: a target, or
  the reason the payload could not be read. It applies the policy error
  and the observe mode in one place.
- A `Decision` says whether it refuses the call (`Refuses`): a deny that
  observe mode does not soften. `hook` and `explain` use it instead of
  their own reading.
- `explain` reads a relative path, for its default tool and for the
  decision, against its own working directory. The hook reads it
  against the payload's `cwd`, as today.
- Every read of the process goes through `Env`. The guard's tests call
  `guard.Main` in-process with an `Env`, the corpus included, so the
  corpus runs under `-short`. A few subprocess smoke tests keep the
  binary, stdin and the exit code covered.
- A test runs the corpus through `vibe-guard` with exceptions built from
  each deny case, and checks that every case the guard lets through by
  an exception is one the audit's reading accepts. The audit's reading
  does not change.
- `vibe unlock` takes its clock from the CLI's seams.

## User Stories

1. As a maintainer of `vibe-guard`, I want `Guard`'s state unexported,
   so that no caller can build a guard whose policy and policy error
   disagree.
2. As a maintainer, I want `New` to take the clock and the payload's
   directory as options, so that a guard is complete when `New`
   returns.
3. As a maintainer, I want one decide method for a call that was read
   and a call that was not, so that the policy-error rule and observe
   mode are written once.
4. As a maintainer, I want a policy error to win over a payload error
   in that one method, as today, so that an unloadable policy is always
   what the person sees first.
5. As a maintainer, I want `Decision.Refuses`, so that `hook` and
   `explain` cannot disagree about what observe mode does to a deny.
6. As a person who uses `vibe-guard`, I want `hook` and `explain` to
   print and exit exactly as today for every call, so that the refactor
   changes nothing I see.
7. As a person who runs `vibe-guard explain` from a subdirectory, I
   want a relative path read against that directory, so that the
   default tool and the decision talk about the same file.
8. As a person who runs `vibe-guard explain .env` from the project
   root, I want the same answer as today, so that the common case does
   not change.
9. As a maintainer, I want a relative `CLAUDE_PROJECT_DIR` made
   absolute against `Env`'s working directory, so that in-process tests
   do not depend on the test process's directory.
10. As a maintainer, I want the corpus test to call `guard.Main`
    in-process, so that it runs in seconds and under `-short`.
11. As a person who works in this repository, I want `task verify:fast`
    and the `Stop` hook to run the guard corpus, so that a change that
    breaks the guard is caught before the full `task verify`.
12. As a maintainer, I want the guard's other tests (paths, shell
    writes, project root, unlock, observe mode, relock, explain) to run
    in-process through the same helpers, so that one seam covers the
    guard's behaviour.
13. As a maintainer, I want a few subprocess smoke tests, so that the
    built binary, real stdin, the exit codes and `version` stay covered.
14. As a maintainer, I want the reference-hook tests (the PreToolUse
    command run by `bash`, with and without `vibe-guard` on `PATH`) to
    stay subprocess tests, so that the install line and the `PATH`
    lookup are tested for real.
15. As CI's network-disabled job, I want a prebuilt `vibe-guard` named
    by `VIBE_GUARD_BIN` to keep working for the subprocess tests that
    remain, so that a prebuilt test binary still needs no toolchain.
16. As a maintainer, I want in-process tests isolated from the process
    environment, so that a `GIT_DIR` or `CLAUDE_PROJECT_DIR` from a hook
    cannot leak into them.
17. As a maintainer, I want in-process tests to pass a fixed clock, so
    that unlock-marker tests do not depend on the wall clock.
18. As a maintainer of the audit, I want a test that fails when
    `vibe-guard` lifts a corpus case by an exception that the audit's
    reading would not accept, so that the two readings cannot drift
    apart unseen.
19. As a maintainer, I want that test to build its exceptions from
    each deny case (a word prefix of each simple command, the path the
    case edits or writes, and a catch-all glob), so that it needs no
    hand-kept list.
20. As a maintainer, I want that test to cover shell writes with path
    exceptions, so that a future corpus case for a shell write of
    `.claude/settings.json` fails the test until the audit's reading is
    fixed.
21. As an adopter, I want the audit's verdict on my repository
    unchanged, so that this refactor needs nothing from me.
22. As a maintainer, I want spec 0053 to say why the audit's reading is
    wider than the guard's and which test keeps it at least as wide, so
    that the difference is a decision and not an accident.
23. As a maintainer of `vibe unlock`, I want its clock passed in, so
    that a test can check the exact expiry written to the marker.
24. As a person who runs `vibe unlock`, I want the same marker and
    message as today, so that nothing changes for me.
25. As a maintainer, I want the corpus file, the guard rule IDs and the
    contract version unchanged, so that this is a refactor and not a
    contract change.

## Implementation Decisions

### The `Guard` type

- `Guard`'s fields become unexported: the repository, the policy, the
  policy error, the clock and the payload's directory.
- `New` takes the repository and an options value with the clock (a
  time) and the directory that relative paths are read against. It
  loads the policy as today: no file is the default policy (enforce, no
  rules, no exceptions), a file that cannot be read or parsed is a
  policy error.
- One exported decide method takes a call. A call is either a target
  (from `guardrule`'s target parsing, or built by `explain`) or a
  payload error with its rule ID and reason. Shape:

  ```go
  type Call struct {
      Target gr.Target
      // Err is set when the payload could not be read; Target is then unset.
      Err *CallError // {Rule, Why string}
  }
  func (g *Guard) Decide(c Call) Decision
  ```

  The method applies, in order: a policy error refuses under
  `policy-error`; a call error refuses under its rule; a target is
  decided by the command or path rules, as today. The observe flag is
  set from the policy's mode on every decision that is not a policy
  error, as today.
- `Decide`'s rule order, findings, lifted rules and reasons are
  unchanged. The command and path checks (`decideCommand`,
  `decidePath`, `checkPath`, `normalize`, `exception`) are not
  restructured beyond reading unexported fields.
- `Decision` gains `Refuses()`: true for a deny when observe is off.
  An ask is not a refusal. The exit code stays a CLI concern in
  `hook` and `explain`, both of which use `Refuses`.
- The repository the session acts in is still found by the session
  function of spec 0068 and passed to `New`. The guard's public surface
  outside `Main` is `New`, `Options`, `Call`, `Decide`, `Decision`,
  `Outcome`, `Finding` and the reference-hook helpers. No package
  outside `internal/guard` calls `New` today, so no caller changes.

### `vibe-guard explain`

- `explain` builds the guard with the directory set to `Env`'s working
  directory. A relative path is then read against the directory
  `explain` runs in, as `hook` reads it against the payload's `cwd`.
- The default-tool test (one word that names an existing path means
  `Write`) stats the path joined to `Env`'s working directory when it is
  relative.
- From the project root, both give today's answer. From a
  subdirectory, a relative path now names the file in that
  subdirectory, for the default tool and for the decision alike.

### Every process read through `Env`

- A relative `CLAUDE_PROJECT_DIR` is joined to `Env`'s working
  directory, not made absolute against the process's.
- After this, `guard.Main` reads the process only through `Env` (stdio,
  `Getenv`, working directory, clock) and the file system. A test in
  the guard's package checks this by running `Main` from a test whose
  process directory differs from `Env`'s.

### The guard's tests

- The test harness keeps its helpers (`newRepo`, `hook`, `explain`,
  `runGuard`) but runs `guard.Main` in-process with an `Env`: stdin from
  the test's text, captured stdout and stderr, `Getenv` from a map that
  holds only what the test sets, the fixture's directory, and a fixed
  clock. The exit code is `Main`'s return value.
- The corpus test and the guard's behaviour tests move to the
  in-process harness and stop skipping under `-short`.
- Subprocess tests that remain: the reference-hook tests (`bash` runs
  the reference PreToolUse command), and a small smoke set that runs
  the built binary for one deny, one allow and one ask payload on real
  stdin, one `explain`, and `version`. The binary is built once, or
  taken from `VIBE_GUARD_BIN`, as today.
- The contract's guard-probe tests keep running the real `vibe-guard`
  binary. They check the audit, not the guard.

### The two readings of an exception

- The audit keeps `Exception.Matches` as its reading (spec 0053). The
  guard keeps lifting the simple command or path its rule refused, and
  never a fixed rule.
- A new test in the guard's package keeps the guard's reading inside the
  audit's. For each deny case of the corpus and each exception built
  for it, it runs the case through `guard.Main` with a policy that holds
  that one exception. When the guard lets the case through, the test
  asserts that `Exception.Matches` accepts the exception for the case.
  The exceptions built for a case are:
  - for a command rule, each word prefix of each simple command of the
    case that holds no expansion;
  - for a path rule, the path the case edits, made project-relative,
    and the path of each literal shell write of the case, made
    project-relative;
  - for any path rule, `**`.
- If that test fails on a corpus case, the fix belongs in
  `Exception.Matches` (for example, reading a shell write's path), and
  it is a contract PATCH under spec 0051 ("a fix that makes a check
  match its stated purpose"). Today it is expected to pass.

### `vibe unlock`'s clock

- `vibe unlock` reads the current time from the CLI's seams, the
  value that already carries the audit's injected dependencies. The
  real command passes the wall clock. Its output and marker format do
  not change.

### Documentation

- Spec 0053, "Testing Decisions": the guard's seam is `guard.Main` with
  an `Env`, in-process; the subprocess seam covers the reference hook
  and a smoke set.
- Spec 0053, "Guard rule IDs and exceptions, shared": the audit's
  reading is at least as wide as `vibe-guard`'s, on purpose, and the
  test of "The two readings of an exception" keeps it so. Narrowing the
  audit's reading is the MAJOR item named in "Further Notes".
- `docs/usage.md`, `vibe-guard explain`: a relative path is read
  against the directory `explain` runs in.
- `docs/architecture/overview.md`, "The reference guard": `Guard`'s
  interface in one sentence, and that the guard's tests run in-process.

## Testing Decisions

- **A good test drives `vibe-guard` as Claude Code or a person does and
  checks the exit code, the output and the files written.** It does not
  read `Guard`'s state or call unexported functions. `New`, `Decide` and
  `Decision.Refuses` are tested through `guard.Main`, not directly.
- **One main seam: `guard.Main(args, Env{…})`, in-process.** It covers
  every guard behaviour test and the corpus. Prior art: the existing
  helpers in the guard's test harness, which keep their names and
  signatures, and `doctor.Env`, which injects the process the same way.
- **Two existing seams keep their role:**
  1. The subprocess harness, for the reference-hook tests and the smoke
     set. Prior art: `TestReferenceHookWithVibeGuard`,
     `TestReferenceHookWithoutVibeGuard`.
  2. `vibe unlock` run in-process through the CLI's root command, for
     the clock. Prior art: `TestUnlockLinkedWorktreeSharesCommonDir`.
- **Cases:**
  - The whole corpus through `hook` and `explain`, in-process, with
    today's assertions; under `-short` too.
  - Observe mode: a deny is allowed with the observe line and the log
    entry; an ask is unchanged; a policy error is refused even in
    observe mode (unchanged behaviour, now through the one decide
    method).
  - A payload error with an unloadable policy is refused under
    `policy-error` (unchanged).
  - `explain` from a subdirectory: `explain x.env` where `sub/x.env`
    exists and the root has none picks `Write` and names
    `secret-file`. As a subprocess this passes today; in-process it
    needs the default-tool test to read `Env`'s directory.
  - `explain --tool Write .claude/settings.json` from `sub`, the
    project root being its parent: allowed with rule `none`, as `hook`
    decides that path with `cwd` `sub`. Today it is refused under
    `guardrail-settings`, because `explain` reads it against the root.
  - A relative `CLAUDE_PROJECT_DIR` is read against `Env`'s directory,
    not the test process's directory. Fails today in-process.
  - The two-readings test over the corpus, as described above.
  - `vibe unlock` with a fixed clock writes the exact expiry.
  - The smoke set through the built binary.
- **What is not tested directly:** the option values of `New`, the
  `Call` shape and `Refuses` in isolation. They are internal to the
  guard's behaviour and covered through `Main`.

## Out of Scope

- **Narrowing the audit's reading of an exception** (compound commands,
  fixed rules) or making the audit re-run the guard without the
  exception. It tightens A-02, a MAJOR change. It is a new issue, with
  #50 and #120.
- **Moving the baseline rules into `internal/guardrule`.** Spec 0053
  keeps them in `internal/guard`; this spec does not reopen that.
- **#166's other items**: parsing rule and exception matches once at
  load time, typed fields, splitting `wrappers.go`, the setupskill and
  test-helper items.
- **New corpus cases**, a new guard rule ID or any contract change.
- **Restructuring the rule checks** inside `Decide` (`decideCommand`,
  `decidePath`); they keep their logic and order.
- **Codex sessions**, which have no hooks (ADR 0011).

## Acceptance Criteria

- [ ] `Guard` has no exported fields. `New` takes the repository and an
      options value with the clock and the directory.
- [ ] One exported decide method handles a target, a payload error and
      a policy error. `DecideError` is gone.
- [ ] `Decision.Refuses` exists, and `hook` and `explain` decide their
      exit code from it.
- [ ] `vibe-guard hook` and `vibe-guard explain` give the same exit
      code, stdout and stderr as on `main` at 48b741d for every corpus
      case and every existing guard test.
- [ ] `explain` reads a relative path, for the default tool and for the
      decision, against its working directory.
- [ ] A relative `CLAUDE_PROJECT_DIR` is joined to `Env`'s working
      directory.
- [ ] The guard's tests run `guard.Main` in-process, except the
      reference-hook tests and a smoke set through the built binary.
- [ ] `go test -short ./internal/guard/...` runs the whole corpus.
- [ ] The two-readings test runs every deny case of the corpus with
      the exceptions built for it, and passes.
- [ ] `vibe unlock` takes its clock from the CLI's seams, and a test
      checks the exact expiry it writes.
- [ ] The guard corpus file, the guard rule IDs and the contract
      version are unchanged.
- [ ] Spec 0053, `docs/usage.md` and `docs/architecture/overview.md`
      describe the new seam, `explain`'s path reading and why the
      audit's reading of an exception is the wider one.
- [ ] A new issue records the MAJOR item: A-02 counts an exception only
      when the guard lets the case through because of it.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.
- [ ] The canary in `docs/usage.md` still refuses in a live session
      after `go install ./cmd/vibe-guard`.

## Further Notes

- **Option chosen for item 4 (2026-11-25).** Three options were
  weighed: re-run the guard in the audit without the case rule's
  exceptions; move the baseline rules into `internal/guardrule` and
  share one lifting function; keep both readings and test that the
  guard's stays inside the audit's. Re-running the guard alone would
  loosen A-02: it trusts the guard's own reading, so a guard that
  ignores `match` would pass. Re-running it as well as matching would
  tighten A-02: a guard that lets a deny case through whatever the
  exception says would become non-conformant. Under spec 0051 that is
  MAJOR. So this spec keeps both readings, adds the test, and moves the
  tightening to the MAJOR batch with #50 and #120.
- The two-readings test proves only that the guard's reading is inside
  the audit's on the corpus. It does not prove the audit is tight; that
  is the MAJOR item.
- **Assumption, not verified:** no test outside the guard's package
  depends on the guard's tests being subprocess tests. CI's
  network-disabled job runs a prebuilt test binary with
  `VIBE_GUARD_BIN`; the smoke set and the reference-hook tests keep it
  needed.
- **Assumption, not verified:** the in-process corpus with `t.Parallel`
  has no shared state in `guard.Main`. The package's globals are
  compiled regular expressions and rule tables, which are read-only.
