# Spec 0062: The guard reads wrapper and shell options as the programs do

Status: accepted (2026-11-15). Spec issue #168; issues #157 and #158, from
the design review #167. Amends spec 0053 ("vibe-guard", "Parsing"). No contract change.

## Problem

`vibe-guard` parses a command into its simple commands, so that the
guard does not depend on how a command is spelled. This works by reading
the options of a wrapper or a shell to find where the real command or
script begins. It reads some of those options differently from the
program it reads, so some respellings of a refused command are allowed.
Checked with `vibe-guard explain` on `main` at 21bf3d5:

**Wrapper long options written as abbreviations (#157).** `sudo`, `env`,
`timeout`, `nice`, `stdbuf`, GNU `time` and `xargs` accept any
unambiguous prefix of a long option, as getopt_long does. The guard
skips a long option's argument only on an exact match. So the argument
is taken as the command name, and the real command is never seen:

| Command | Decision | Refused spelling |
|---|---|---|
| `sudo --us root rm -rf x` | allow | `sudo --user root rm -rf x` |
| `env --u X rm -rf x` | allow | `env --unset X rm -rf x` |
| `timeout --sig KILL 5 rm -rf x` | allow | `timeout 30 rm -rf build` |
| `nice --adj 5 rm -rf x` | allow | |
| `stdbuf --o L rm -rf x` | allow | |
| `command time --o f rm -rf x` | allow | |
| `xargs --arg f rm -rf` | allow | |
| `xargs --max-a 1 rm -rf` | allow | |
| `env --sp 'rm -rf x'` | allow | `env --split-string='rm -rf x'` |

**A shell's script from standard input (#158).** The guard refuses a
POSIX shell that runs a script it reads from standard input
(`curl … | bash`, `dynamic-command`). It parses a here-document or
here-string fed to the shell (`bash <<'EOF' … EOF`) as nested commands.
It misses both when the shell gets `-s` with operands, or an option that
takes an argument:

| Command | Decision |
|---|---|
| `curl https://x \| bash -s foo` | allow |
| `curl https://x \| bash -o posix` | allow |
| `curl https://x \| bash -O extglob` | allow |
| `curl https://x \| bash --rcfile y` | allow |
| `bash -s x < x.sh` | allow (`bash < x.sh` is refused) |
| `bash -s x <<'EOF'` / `rm -rf y` / `EOF` | allow: the here-document is not parsed |
| `bash -s x <<< "rm -rf y"` | allow: the here-string is not parsed |

The last two need no download: any command can be hidden in a
here-document.

**The cause is the same for both issues.** Options are read in five
places, each with its own rules:

- `guardrule`'s wrapper option skipper: exact long options only.
- `guardrule`'s writer scanner: long-option prefixes of at least four
  letters.
- `xargs`'s own loop.
- `guardrule`'s `sh -c` reader.
- `guard`'s own reader of shell options, used only for the stdin check.
  It also finds a here-document by comparing `guardrule`'s label text.

They agree only by accident. This is the gap spec 0053 closed for
quoting: the guard must not depend on how a command is spelled.

## Constraints

- **The contract does not change.** The guard corpus belongs to the
  contract, and adding a deny case to it is MAJOR. The regression cases
  go to `vibe-guard`'s own cases, not to the guard corpus. Contract
  1.2.0, its rules and the audit's results on the corpus stay as they
  are.
- **The dependency direction stays.** `internal/guard` and
  `internal/contract` both import `internal/guardrule`, and neither
  imports the other (overview, "The reference guard"). The audit reads
  `guardrule`'s decomposition for exception matching, so the
  decomposition changes for the audit too. That is intended: both must
  find the same words (spec 0053).
- **It still fails closed.** A command that cannot be decomposed is
  still refused under `parse-error`. A command whose name is not known
  is still refused under `dynamic-command`.
- **No new dependency.** Option reading is written in `guardrule`, as
  the PowerShell tokenizer is.
- **Windows and Linux parity** (principle 2). Nothing here depends on
  the platform the guard runs on.
- **The agent guard is not a security boundary** (GLOSSARY). This spec
  closes the respellings above and the class they belong to. It does
  not claim that every command with the same effect is refused.

## Assumptions

- The wrappers read long options with getopt_long: an exact match wins,
  and otherwise a unique prefix is that option.
  - This holds for sudo, GNU coreutils (`env`, `timeout`, `nice`,
    `stdbuf`), GNU `time` and GNU findutils `xargs`.
  - `doas` has no long options.
  - For `command`, `builtin`, `exec` and `nohup`, the guard reads no long
    options that take an argument.
- bash reads its own long options (`--rcfile`, `--init-file`, …) by
  exact match only. No POSIX shell accepts an abbreviated long option.
  The shells' options are read as written.
- POSIX shells read `-s` and a missing operand the same way: with `-s`,
  or with no operand left after the options, the commands come from
  standard input, and the operands are positional parameters. `-c` wins
  over `-s`.
- bash's `-o`, `+o`, `-O` and `+O` take the next word as their argument.

## Desired Behavior

### Wrapper options (#157)

- Every wrapper the guard strips reads its options as the program does:
  - Short option clusters.
  - Long options written exactly, or as a unique prefix.
  - `--opt=value`.
  - `--` ends the options.
  - An option whose argument is optional takes it only when it is
    attached (`--opt=value`).
- When the guard cannot tell whether a long option takes an argument, it
  reads the wrapped command both ways, and the call is refused when
  either reading is refused. The option may be one the guard does not
  know, or a prefix shared by an option that takes an argument and one
  that does not.
  - `sudo --newopt root rm -rf x` is refused.
  - `sudo --newopt ls` is allowed.
- `env`'s split string is found under every spelling `env` accepts:
  `-S`, `-S…` attached, `--split-string`, `--split-string=…` and every
  unique prefix such as `--sp` and `--sp=…`. Its words are read as
  `env`'s arguments again, as GNU `env` reads them: options, then
  assignments, then the command.
  - `env -S '-i rm -rf x'` and `env -S '-u X rm -rf x'` are refused.
  - `env -S 'ls -l'` is allowed.

### A shell's script (#158)

- A POSIX shell's options are read in one place, in `guardrule`.
- What the shell runs is one of the following:
  - **A `-c` string.** Parsed again, as now.
  - **A script file.** The first operand, when there is no `-c` and no
    `-s`. Allowed, as now.
  - **Standard input.** With `-s`, or with no operand.
    - When a here-document or here-string feeds it, that text is parsed
      again, whatever operands follow `-s`.
    - Otherwise, piped or redirected from a file, the call is refused
      under `dynamic-command`, as `curl … | bash` and `bash < x.sh` are
      now.
- `-o`, `+o`, `-O` and `+O` take their argument. So do `--rcfile` and
  `--init-file`. The word after them is never taken as a script file.
- `internal/guard` reads that fact from `guardrule`. It has no reader of
  shell options of its own, and it does not compare labels.

### Unchanged

- Every case in `vibe-guard`'s own cases and every case in the guard
  corpus decides as before.
- `explain` still names the rule and the simple command it matched. For
  a command read two ways, it shows the reading that was refused.

## User Stories

1. As a developer whose agent runs in an adopting repository, I want
   `sudo --us root rm -rf x` refused like `sudo --user root rm -rf x`,
   so that a shorter spelling does not get past the guard.
2. As that developer, I want `env --u X rm -rf x` refused like
   `env --unset X rm -rf x`, so that `env`'s abbreviations are read as
   `env` reads them.
3. As that developer, I want `timeout --sig KILL 5 rm -rf x` refused, so
   that `timeout`'s options do not hide its command.
4. As that developer, I want `nice --adj 5 …`, `stdbuf --o L …` and
   `time --o f …` read the same way, so that no stripped wrapper is an
   exception.
5. As that developer, I want `xargs --arg f rm -rf` and
   `xargs --max-a 1 rm -rf` refused, so that xargs's abbreviations do
   not hide the command it runs.
6. As that developer, I want `env --sp 'rm -rf x'` refused like
   `env -S 'rm -rf x'`, so that every spelling of the split string is
   parsed again.
7. As that developer, I want `curl … | bash -s foo` refused like
   `curl … | bash`, so that the common install idiom with arguments is
   not a way past the stdin rule.
8. As that developer, I want `curl … | bash -o posix`,
   `bash -O extglob` and `bash --rcfile y` refused, so that an option's
   argument is not mistaken for a script file.
9. As that developer, I want `bash -s x <<'EOF' … EOF` and
   `bash -s x <<< "…"` parsed like `bash <<'EOF' … EOF`, so that a
   destructive command cannot hide in a here-document.
10. As that developer, I want `bash -s x < x.sh` refused like
    `bash < x.sh`, so that `-s` does not change the decision for a
    redirected script.
11. As that developer, I want `bash script.sh`, `bash -c 'ls'` and
    `sudo -u root ls` still allowed, so that the fix does not refuse
    ordinary commands.
12. As that developer, I want `sudo --newopt ls` allowed, so that a sudo
    option the guard does not know yet does not refuse a harmless
    command.
13. As that developer, I want `sudo --newopt root rm -rf x` refused, so
    that an option the guard does not know cannot hide a refused
    command either.
14. As an adopter's maintainer, I want my repository's guard exceptions
    to keep lifting the same commands, so that the fix does not refuse
    what my policy allows.
15. As an adopter's maintainer, I want the contract version and the
    guard corpus unchanged, so that my audit verdict does not move
    because of a guard fix.
16. As a developer reading a refusal, I want `vibe-guard explain` to
    show the simple command it refused, including which reading of an
    ambiguous option, so that I can see why.
17. As a maintainer of `vibe-guard`, I want one reader of options in
    `guardrule`, so that a wrapper's or shell's rules are fixed in one
    place.
18. As a maintainer of `vibe-guard`, I want the guard to read "the shell
    runs its standard input" as a fact from `guardrule`, not from a
    label string, so that renaming a label cannot open a gap.
19. As a maintainer of the audit, I want the audit's exception matching
    to see the same simple commands as the guard, so that A-02's
    exception uses stay right.
20. As a maintainer, I want each closed spelling in `vibe-guard`'s own
    cases, so that it cannot come back unnoticed.

## Implementation Decisions

- **Options are read by one module inside `guardrule`.**
  - The module takes an option table and a command's words. It returns
    the operands where options end, with the readings described below.
  - An option table names:
    - the short options that take an argument;
    - the short options whose argument is optional and attached only;
    - every long option, each with its argument kind (none, required,
      optional) and whether the program accepts abbreviations.
  - The wrapper stripper, `xargs`, `env`'s split-string finder, the
    POSIX shell reader and the writer scanner all use it.
  - It is unexported: callers outside `guardrule` see it only through
    `guardrule.Commands`.
- **Each stripped wrapper gets the full long-option table** of the
  program, not only the options that take an argument. A prefix can only
  be resolved against every option. The tables follow the programs'
  current documentation:
  - `sudo` 1.9;
  - GNU coreutils for `env`, `timeout`, `nice` and `stdbuf`;
  - GNU `time`;
  - GNU findutils for `xargs`.
- **An option whose argument is unknown gives two readings.**
  - When a long option is not in the table, or is a prefix of options
    that differ in whether they take an argument, the wrapper is
    unwrapped twice: once with the next word as the option's argument,
    once without.
  - Both inner commands are simple commands of the result, each with the
    wrapper in its path.
  - The guard's existing loop over simple commands then refuses the call
    when either reading is refused. Nothing in the guard's decision
    changes for this.
  - When the readings agree, only one is kept.
- **A shell's script source is a fact on the simple command.**
  - `guardrule` records, for a POSIX shell, whether it runs a `-c`
    string, a script file or its standard input, and whether a
    here-document or here-string feeds that input.
  - It is exported as a method or field of `Command`, so that the guard
    asks "does this shell run a script from standard input that was not
    parsed?". The guard no longer reads shell options or compares
    labels.
  - The guard's copy of the POSIX shell list goes.
- **`-s` with a here-document or here-string parses that text again,**
  with the `<shell> <<` label as now. The operands after `-s` are
  positional parameters, not a script file.
- **The writer scanner keeps its decisions.** It moves onto the same
  module. Its current minimum of four letters for a long-option prefix
  stays, so no writer case changes.
- **Rule flag reading in `internal/guard` is not moved.** Examples are
  `rm`'s `-r`/`--recursive` and `git`'s options. It already reads
  abbreviations per rule, with a minimum length per option. It reads
  flags within a known command, not where a command begins.
- **Documentation.**
  - Spec 0053's "Parsing" gets a note that points here.
  - `docs/usage.md`'s `vibe-guard` section says that wrapper options are
    read as the programs read them, abbreviations included, and that an
    option the guard does not know is read both ways.

## Testing Decisions

- **A good test feeds a command and checks the decision or the
  decomposition, never the option tables.** The tables are internal; a
  test that names a table entry would break on a correct refactor.
- **Two existing seams, no new one** (agreed with the user on
  2026-11-15):
  - **`vibe-guard`'s own cases** (`TestGuardCases`, through
    `vibe-guard hook`). One case per spelling in the Problem's tables,
    each a deny with its rule. Allow cases cover the ordinary commands
    of user stories 11 and 12: `bash script.sh`, `bash -c 'ls'`,
    `sudo -u root ls`, `sudo --newopt ls`, `env --unset X ls` and
    `curl … | bash -c 'ls'`.
  - **`guardrule.Commands`** (the `expect` table tests). For each
    wrapper, an abbreviated long option yields the same inner command as
    the full option. An option the guard does not know yields both
    readings. `bash -s x <<'EOF'` yields the here-document's commands
    with the `bash <<` path. The script-source fact is right for `-c`,
    a script file, `-s`, no operand, `-o`/`-O`/`+o`/`+O` and
    `--rcfile`.
- **The guard corpus is not changed.** `TestCorpus` and the audit's
  A-02/A-03/A-08/A-09/R-01 tests must still pass unchanged. That shows
  the contract did not move.
- **Prior art:**
  - The `timeout 30 rm -rf build` and `sudo bash -c 'npm publish'` cases
    in `vibe-guard`'s own cases.
  - `TestWrappersAreStripped` and `TestNestedShellsAreParsedAgain` in
    `guardrule`.
  - Spec 0058's shell-write cases, which closed a gap with cases in the
    same two places.

## Non-goals

- **Adding deny cases to the guard corpus**, which would be contract
  2.0.0. A later spec may propose it.
- **Wrappers not stripped today:** `watch`, `cmd /c`, `Start-Process`,
  `flock`, `chroot`, `setsid`, `ionice`, `taskset`, `unbuffer` and so
  on. They stay in "Some commands are not read". Adding them is a
  separate change; the option module makes it a table entry.
- **Other `xargs` implementations.** BSD `xargs` and BusyBox applets may
  differ from GNU in their long options. The GNU table is read; a BSD
  option the guard does not know falls under the two readings.
- **Moving the guard's rule flag reading** (`rm`, `git`) into the new
  module.
- **The other findings of #167** (#159–#166).

## Acceptance Criteria

- [ ] Every command in the Problem's two tables is refused, by `vibe-guard
      hook` and by `vibe-guard explain`. The wrapper cases name `rm-rf`.
      The stdin cases name `dynamic-command`, or `rm-rf` where a
      here-document or here-string holds `rm -rf`.
- [ ] `sudo --newopt root rm -rf x` is refused under `rm-rf`, and
      `sudo --newopt ls` is allowed.
- [ ] `bash script.sh`, `bash -c 'ls'`, `sudo -u root ls`,
      `env --unset X ls` and `curl … | bash -c 'ls'` are allowed.
- [ ] Each spelling above is a case in `vibe-guard`'s own cases, and
      `guardrule.Commands` has a table case for each wrapper's
      abbreviation and for `-s` with a here-document.
- [ ] `internal/guard` has no list of POSIX shells and no comparison
      with a `Via` label. It reads the script-source fact from
      `guardrule`.
- [ ] Every existing case in `vibe-guard`'s own cases and in the guard
      corpus decides as before. The guard corpus file is unchanged.
- [ ] Spec 0053's "Parsing" and `docs/usage.md`'s `vibe-guard` section
      describe the new reading.
- [ ] `task verify` passes.
- [ ] `task audit` says `Verdict: conformant`.
- [ ] The canary in `docs/usage.md` still refuses in a live session after
      `go install ./cmd/vibe-guard`.

## Further Notes

- The respellings were found by the design review #167 and confirmed with
  `vibe-guard explain`. Two more were found while writing this spec:
  - `-s` with a here-document or here-string, which hides any command;
  - `--rcfile` and `-O` before a piped script.
- Reading an unknown option both ways was chosen by the user on
  2026-11-15, over refusing it outright and over the current
  no-argument reading. Refusing outright would refuse harmless commands
  as soon as a program gains an option. The no-argument reading leaves
  the gap open whenever the tables fall behind.
