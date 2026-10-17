# ADR 0026: `vibe` Parses Shell with `mvdan.cc/sh`

## Status

Accepted (2026-10-16). Issue #99, spec 0052 ("Shell parsing"). Extends
ADR 0025 decision 8, which makes `mvdan.cc/sh` a dependency of
`vibe-guard` only.

## Context

Contract 1.0.0 decides most static rules by asking whether a command is
"called": a hook command, CI `run:` step or entrypoint "that, when
parsed as shell, contains the declared command as a whole simple
command" (spec 0051, "Rules"). Entrypoints are "parsed as POSIX shell"
(spec 0051, "vibe.yaml"), and V-04 needs to know whether each one is
exactly one simple command.

Answering that means following quoting, escapes, lists, pipelines,
conditionals, loops, subshells, functions and command substitutions. A
`run:` step such as

```sh
if task "fmt:check"; then (cd web && pnpm test) | tee log; fi
```

calls `task fmt:check` and `pnpm test`; a regular expression over the
text cannot say so without re-implementing a shell lexer. Spec 0047
showed what that costs in the guard: every quoting spelling a regex
missed was a way past it.

`go.mod` had no shell parser. `mvdan.cc/sh/v3` is the parser Task
itself uses (Task v3.53.1 pins it), so every repository this project
generates already runs it.

## Decision

1. **`vibe` depends on `mvdan.cc/sh/v3`**, package `syntax` only, at a
   tagged release (v3.14.1). The interpreter and the formatter are not
   used.
2. **POSIX mode.** Commands are parsed with `syntax.LangPOSIX`, as spec
   0051 says. A Bash-only construct does not parse, and the check that
   needs it treats the command conservatively (spec 0052): an
   entrypoint fails V-04; a `run:` step calls nothing, and C-02 splits
   it on whitespace instead.
3. **One wrapper.** `internal/contract/shell.go` is the only code that
   imports `syntax`. It turns a parse tree into simple commands whose
   words have had quote removal, drops leading assignments and
   redirections, and marks any word holding an expansion as matching
   nothing. The checks see `Command` and `Word`, not the parser's
   types.

## Consequences

- The audit decides "called" the way a shell would run the command,
  including inside `if`, `&&`, pipelines, subshells and functions. A
  string passed to `sh -c` stays one word and is not parsed again
  (spec 0052).
- `go.sum` gains `mvdan.cc/sh/v3` and the test-only modules its own
  `go.mod` lists. The `vibe` binary links only `syntax` and its sibling
  `fileutil`, which import nothing outside the standard library.
- When `vibe-guard` (#105) lands, both binaries share one parser.
- A newer `mvdan.cc/sh` that parses differently could change a verdict.
  It is upgraded like any dependency, by a reviewed update-bot pull
  request, and the fixture tests of every "called" rule run on it.

## Alternatives considered

- **Split on whitespace and `&&`/`;`.** It cannot follow quotes,
  escapes, subshells or line continuations, so `task "verify"` would
  not call `task verify`, and `echo "a; task verify"` would.
- **Write a small POSIX lexer.** It would be several hundred lines that
  must agree with real shells on every quoting rule; the guard's regex
  history (spec 0047) shows how that drifts.
- **Run the command under `sh -n` or `bash -n`.** That checks syntax
  but returns no tree, needs a shell on the auditing machine (Windows
  has none by default), and is no longer a static check that runs
  nothing.
