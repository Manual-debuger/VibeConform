# Dependency direction (optional)

No contract rule checks which part of the code may import which. Use
this reference only when the person asks to enforce layers or a
dependency direction. It turns a rule written in prose ("the domain
never imports the adapters") into a lint finding, so that `verify`, CI
and the agent's Stop hook catch a wrong import.

## Steps

1. **Declare the layers.** Take them from the repository's architecture
   document, or ask the person. Write each rule as "A may import B,
   never the reverse", or "A never imports B". Keep the code's actual
   imports as the starting point: a rule that the code already breaks
   needs the person's decision (fix the import, or change the rule).
2. **Write the rule into the language's tool** (below). The tool's
   configuration is the single source of truth for the direction.
3. **Wire the tool into the `lint` task**, which `verify` and
   `verify-fast` already run. Add no new entrypoint. In a monorepo, add
   it to each component's own `lint` task.
4. **Prove it.** Add a temporary file with one forbidden import, run
   the `lint` task, see it fail on that import, then delete the file.
5. **Point to the lint.** Where `AGENTS.md` or the architecture document
   states the direction, keep one sentence that names the rule and the
   configuration file that enforces it, instead of the full list.

All three tools run the same on Linux, macOS and Windows.

## Go: `depguard`

`depguard` is built into golangci-lint, so there is nothing to install.
Add it to `.golangci.yml`:

```yaml
linters:
  enable:
    - depguard
  settings:
    depguard:
      rules:
        domain:
          files:
            - "**/internal/domain/**"
            - "!$test"
          deny:
            - pkg: example.com/app/internal/adapter
              desc: domain code does not import adapters
```

- Always give `rules`. Enabled with no rules, depguard applies its
  default rule, which allows only the standard library.
- `files` are globs over file paths. `$test` matches `_test.go` files;
  `"!$test"` leaves tests out of a rule, for tests that must cross the
  line.
- A `deny` entry is a prefix: `internal/guard` also denies
  `internal/guardrule`. End the entry with `$` for an exact match.
- An import that goes up a layer is often an import cycle too, which
  the compiler already refuses. depguard is for the directions the
  compiler allows.

## Python: `import-linter`

Add the dev dependency (`uv add --dev import-linter`) and the contracts
to `pyproject.toml`:

```toml
[tool.importlinter]
root_packages = ["app"]

[[tool.importlinter.contracts]]
name = "Layers"
type = "layers"
layers = ["app.api", "app.service", "app.domain"]

[[tool.importlinter.contracts]]
name = "The domain does not import the adapters"
type = "forbidden"
source_modules = ["app.domain"]
forbidden_modules = ["app.adapters"]
```

- In `layers`, a module may import the ones listed below it, never the
  ones above.
- Add `uv run lint-imports` to the `lint` task's commands. It reads
  installed packages, so the package must be importable after
  `uv sync`.

## TypeScript: `dependency-cruiser`

Add the dev dependency (`pnpm add -D dependency-cruiser`) and a
`.dependency-cruiser.cjs`:

```js
/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: "domain-not-to-adapters",
      comment: "Domain code does not import adapters.",
      severity: "error",
      from: { path: "^src/domain/" },
      to: { path: "^src/adapters/" },
    },
  ],
  options: {
    tsConfig: { fileName: "tsconfig.json" },
    doNotFollow: { path: "node_modules" },
  },
};
```

- Add `pnpm exec depcruise src --config .dependency-cruiser.cjs` to the
  `lint` task's commands. It exits non-zero on a rule of severity
  `error`.
- Keep `severity: "error"`: a `warn` rule reports and still passes.
