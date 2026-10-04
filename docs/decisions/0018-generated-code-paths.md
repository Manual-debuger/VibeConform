# ADR 0018: Generated-code paths in `vibe.yaml`

## Status

Accepted. Implemented per `docs/specs/0037-generated-paths.md`. Amends
ADR 0012: a component has an optional fourth field, `generated`.

## Context

Adopters commit code that a generator writes from a schema. The managed
`ruff.toml` and `eslint.config.js` are generated whole, so a repository
could not exclude that code without editing a managed file. The
workaround was post-formatting generator output with the repository's
own style, which couples every generator to this repository's
configuration.

## Decision

1. **The key.** `generated:` is a list of path patterns. It sits on a
   `prod-mono` component (relative to its path), or at the top level of
   `prod-ts` and `prod-py` (relative to the root). A top-level list on
   `prod-mono`, and any list on `prod-go` or a `profile: go` component,
   is an error.
2. **The grammar.** A pattern is relative, clean and slash-separated. It
   has at least two segments, the first a literal directory name. Each
   segment is exactly `**`, or uses only `A-Z a-z 0-9 . _ -`. A single
   `*` is excluded, because the step-0 probe showed that ruff's `*`
   matches `/` while ESLint's and Prettier's do not. `**` means the same
   in all three. The first-literal-segment rule stops a typo from
   excluding a whole component.
3. **Format and lint, not typecheck.** Checks whose findings are fixed by
   editing the file skip generated code (ruff, ESLint, Prettier). Checks
   that judge correctness still run (`pyright`, `tsc`, tests), because a
   type error in a generated contract is a real defect.
4. **Rendering.**
   - Python: `extend-exclude` and `force-exclude = true` in `ruff.toml`.
     `force-exclude` makes explicitly passed files (lefthook,
     `hook:format`) honour it.
   - TypeScript: patterns appended to `eslint.config.js`'s global
     `ignores`, and a managed `generated` section in `.prettierignore`.
     `prod-ts`'s ESLint pre-commit command gains `--no-warn-ignored`.
   - Each is an exact-anchor edit of the embedded template
     (`module.ReplaceOnce`). With no list, every byte is the template's.
5. **Go uses its header.** `// Code generated ... DO NOT EDIT.` is Go's
   own convention, and golangci-lint skips such files by default
   (verified on v2.13.2). `gofmt` still applies: it has no style to
   disagree with, and `gofmt -l` has no exclude option.
6. **A section that follows a declaration.** A recorded `generated`
   section that the plan no longer resolves is removed under the rules
   for a deselected option's section (`module.ConditionalSectioner`). It
   is removed if unmodified and kept as a conflict if modified.

## Consequences

- A `vibe.yaml` without the key resolves byte for byte as before.
- An older `vibe` rejects a `vibe.yaml` that uses the key (strict
  decoding), loudly. `task audit` pins the recorded version.
- When every file passed to ruff is excluded, ruff prints "No Python
  files found" and exits 0.
- This is a declaration about the code, not one of the per-component
  overrides spec 0025 lists as a non-goal. It cannot relax a check for
  hand-written files.

## Alternatives considered

- **Header detection for Python and TypeScript.** The tools have none,
  so `vibe` would have to read file contents at sync time. Resolution
  would then depend on repository contents, not on `vibe.yaml` alone.
  Rejected.
- **Negated CLI globs for Prettier** (spec 0026 §10's mechanism).
  Editors do not see them. `.prettierignore` is read at every entry
  point. Rejected.
- **Excluding generated code from type checking.** Rejected (decision 3).
- **Accepting the key on Go** and rendering `linters.exclusions.paths`.
  This needs a glob-to-regex translation, and it duplicates the header
  convention. Rejected.
- **Single `*` in patterns.** It means different things per tool.
  Rejected after the probe.
