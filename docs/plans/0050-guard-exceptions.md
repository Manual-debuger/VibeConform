# Plan 0050: Guard exceptions and extra guard rules

Implements `docs/specs/0050-guard-exceptions.md` under ADR 0024.
Status: approved 2026-10-15 and implemented. Deviations from the plan:
- **More corpus cases than listed**: also no version, not an object,
  `rules: null`, a non-object exception, a rule on an unknown field, an
  extra rule whose pattern does not compile, and a backup file next to the
  policy (allowed). 31 new cases, 162 in all, run by each runtime.
- **`AGENTS.md`** gained one sentence in its hand-written guardrails: an
  exception is the user's to grant, and the agent does not write the file
  through the shell either.
- **Live check** done as planned: `git clean -fdn no-such-dir/` ran, the
  same command followed by a second `git clean -fdn` was refused by
  `git-clean-fd`, an Edit of the local file was refused by
  `guard-policy-file`, and after `rm` of the file audit printed what it did
  before.

## Spec amendment (approved with this plan)

**The guard denies agent edits to `.claude/hooks/policy.local.json`.**
Without this, an agent the guard refuses can write an exception for
itself with one Edit call, and the next attempt goes through. Audit
would list the exception, but only after the fact. A new managed file
rule, `guard-policy-file`, denies Edit, MultiEdit and Write of
`.claude/hooks/policy.json` and `.claude/hooks/policy.local.json`, with
the message "ask the user to edit it". An exception is then the user's
decision, as the escape hatch for a denied command already is (`!
<command>`). A shell write (`echo … >`) is not covered: the guard is not
a sandbox. Spec D-list gains D12 for it; the corpus gains its cases.

## Repository impact

| Path | Change |
|---|---|
| `internal/module/repotooling/templates/guard.go` | read and check the local file; extra rules; exceptions |
| `internal/module/tsrepotooling/templates/guard.mjs` | the same |
| `internal/module/pyrepotooling/templates/guard.py` | the same |
| `internal/module/agents/claude/policy.go` | `LocalPolicy`, `checkLocal` (audit's view, same rules as the guards); `guard-policy-file` rule |
| `internal/module/agents/claude/claude.go` | implements `module.ProjectFileGuard` for the local file |
| `internal/module/module.go`, `internal/cli/guard.go` | `GuardedFile.Notes`: non-failing lines; `printGuard` prints them |
| `internal/module/agents/claude/testdata/guard_corpus.json`, `guard_corpus_test.go` | cases with a local file; the runner writes it per case |
| tests in `internal/module/agents/claude` and `internal/cli` | audit's conflicts and notes |
| `.claude/hooks/*` here and in the examples, `.vibe/state.yaml` | synced guards and policy |
| `docs/usage.md` | D11 |
| `docs/specs/0050-*.md`, this plan | status, D12, deviations |

No Taskfile change: each guard derives the local path from its policy
argument (`policy.local.json` in the same directory), so `hook:guard`
stays `… .claude/hooks/policy.json` in all three standards. Dependency
direction unchanged.

## Guard logic (all three runtimes alike)

1. Load `policy.json` as today (fail closed).
2. Look for `policy.local.json` next to it. Absent: as today.
3. Present: parse and validate. Broken when any of:
   - unreadable, not JSON, or not an object;
   - `version` is not 1;
   - a top-level key other than `version`, `rules`, `exceptions` (a typo
     such as `rulse` would otherwise drop rules silently);
   - `rules` or `exceptions` present but not a list;
   - a rule without a non-empty string `id`, `field`, `pattern` or
     `message`, or a `field` that `policy.json`'s `tools` does not name;
   - an exception without a non-empty string `rule` or `pattern`.

   Broken denies every call: `guard: .claude/hooks/policy.local.json:
   <reason>; every tool call is blocked until it is fixed. Ask the user to
   fix it (in Claude Code: ! <command>, or their editor).`
4. Rules = managed rules, then the local rules. A local rule without
   `raw` uses its `pattern` as `raw`.
5. Parsed call: for each rule on the call's field whose pattern matches
   (a non-compiling pattern matches, as today), skip it if an exception
   for that rule id full-matches the value; otherwise deny with its
   message. Full match: Go `regexp.Compile("^(?:" + p + ")$")`, JS
   `new RegExp('^(?:' + p + ')$')`, Python `re.fullmatch(p, s)`. An
   exception that does not compile matches nothing.
6. Raw fallback: as today, over all rules, no exceptions (D5).

Duplicate ids and unknown exception rule ids do not change the guard's
decision (an unknown id lifts nothing); audit reports them (D8).

## Audit (vibe side)

`claude` implements `ProjectFileGuard` with one `GuardedFile` for
`.claude/hooks/policy.local.json`:

- `Check` returns D8's conflicts: everything step 3 rejects, with the same
  wording; an exception naming a rule id neither file defines; a pattern
  (rule `pattern`/`raw`, exception `pattern`) that Go's `regexp` does not
  compile; an extra rule repeating an id.
- New optional `Notes func(content []byte) []string` on `GuardedFile`:
  one line per exception, `exception <rule>: <pattern>` (plus ` (<reason>)`
  when given), and per extra rule, `extra rule <id>: <pattern>`.
  `printGuard` prints `path: <note> (project-owned)` for each note before
  the `ok`/conflict lines; notes count for nothing.

`checkLocal` lives in `policy.go` next to the managed policy, so audit and
the guards are tested against the same corpus inputs: a Go test runs
`checkLocal` on every corpus case's local file and requires "broken"
exactly where the case expects every call denied.

## Corpus

Each case may carry `local` (a JSON object written as
`policy.local.json`) or `local_text` (raw text, for a broken file). The
runner gives each case its own directory with `policy.json` and that
file. The end-to-end run writes it to the synced repository's
`.claude/hooks/` for the case and removes it afterwards, failing if one
was already there. New cases, at least:

- exception full-matching `git clean -fdx build/` → allow;
- the same plus `; rm -rf ~` → deny (rm-rf); plus `&& git clean -fdx .` →
  deny (the exception does not full-match);
- excepted `git clean -fdx build/ && git reset --hard` → deny;
- `secret-file` exception for `\.env\.example` → Write of `.env.example`
  allowed, `.env` still denied;
- exception with `(` as pattern → deny;
- exception for an unknown rule id → deny as before;
- extra rule `terraform destroy` → deny with its message; `terraform plan`
  → allow;
- broken: not JSON, version 2, unknown key, rule without message,
  exception without pattern → `ls` denied;
- raw fallback (`stdin` text) with an exception that would match → deny;
- `guard-policy-file`: Write/Edit of `.claude/hooks/policy.local.json`
  and `policy.json` → deny; Write of `docs/policy.local.json.md` → allow.

## Tests besides the corpus

- `checkLocal` unit tests for every D8 conflict and the notes.
- CLI: audit with no file (output unchanged), with a good file (notes,
  conformant), with each kind of problem (conflict, exit 2); sync never
  writes the file.

## Live check (acceptance 4)

In this repository, in this session, all with harmless commands:

1. Before syncing the new guard here, I write a temporary
   `.claude/hooks/policy.local.json` (the old guard has no
   `guard-policy-file` rule yet) with one exception: `git-clean-fd` for
   `git clean -fdn no-such-dir/`, a dry run (`-n`) of a directory that
   does not exist.
2. Sync the new guard into this repository.
3. `git clean -fdn no-such-dir/` runs. The same command followed by
   `; git clean -fdn other-dir/` is denied. An Edit of the local file is
   denied by `guard-policy-file`.
4. I remove the file with `rm .claude/hooks/policy.local.json` (a plain
   `rm`, which the guard allows) and check that `vibe audit` is back to
   its old output.

Recorded in the ledger.

## Commits

1. `docs: plan 0050` (with the D12 amendment to the spec).
2. `feat: guards read policy.local.json: extra rules and exceptions` —
   three runtimes, `guard-policy-file`, corpus and runner.
3. `feat: audit checks policy.local.json and lists its exceptions` —
   `checkLocal`, `Notes`, CLI tests.
4. `chore: sync this repository and the examples (spec 0050)`.
5. `docs: guard exceptions in usage; spec 0050 status`.

Then `task verify`, `task audit` here and in the examples, the live
check, push, and a PR closing #80.
