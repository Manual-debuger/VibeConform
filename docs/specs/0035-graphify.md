# Spec 0035: Optional Graphify Integration

Status: accepted. Tracks issue #52. Builds on spec 0026 (optional
integrations), spec 0028 (`vibe doctor`), spec 0029 (managed sections)
and spec 0023 (agent hooks).

## Context

Graphify (PyPI `graphifyy`, MIT) builds a persistent knowledge graph of
a repository under `graphify-out/`. Agents can query it with
`graphify query/path/explain`. Some users already run it. Issue #52 asks
for Graphify as an optional, removable integration that is off by
default. It would be the first entry in the `intelligence` category,
which spec 0026 created and left empty.

Upstream's own `graphify install --project --platform <p>` merges hooks
into `.claude/settings.json` and `.codex/hooks.json`. VibeConform owns
both of those files whole, and the generated content depends on the
installed graphify version. If `vibe sync` ran that installer, the result
would be drift that the next `vibe audit` fails. Instead, VibeConform
generates the project-level agent surface itself. Git hooks keep the
graph fresh, so the integration is fully autonomous and needs no install
step.

## Constraints that apply

- Principle 3: generated files depend on third-party tools only (here
  `graphify`, `lefthook`, `task`), never on `vibe`. `task verify`,
  `verify-ci` and CI must not touch graphify.
- Spec 0026: selection is a pure function of `vibe.yaml`. An absent
  category keeps today's defaults. Deselection prunes only recorded,
  unmodified resources.
- Spec 0024 / ADR 0011: Codex hooks stay suspended.
- Spec 0028 §2.1: a missing optional capability is `WARN`, not `FAIL`.
- `sync` installs nothing and changes no global agent or git config.

## Scope

### 1. Selection

```yaml
integrations:
  intelligence: [graphify]
```

- Registered in the shared catalog for all four standards, under
  `intelligence`. Its default is **off**. It declares no `Requires` and
  no `Excludes`, so it can coexist with a future GitNexus provider.
- With it unselected, every standard resolves byte-for-byte as before.
- The "no code-intelligence providers are available yet" error goes
  away. An unknown intelligence name lists `graphify` as the valid one.

### 2. Resources when selected

| Resource | Ownership | Present when |
|---|---|---|
| Root `.gitignore`: section `graphify`, containing `graphify-out/` | managed section (hash-comment markers); the rest of the file stays the user's | always (file created if absent) |
| `.claude/skills/graphify/SKILL.md` | generated | `claude` selected |
| `hook:context` gains a `Graphify:` line | part of the generated `Taskfile.yml` | `claude` selected |
| `Taskfile.yml` gains a `graph:update` task | part of the generated `Taskfile.yml` | always |
| `lefthook.yml` gains `post-commit` and `post-checkout` `graphify-update` jobs running `task graph:update` | part of the generated `lefthook.yml` | always |
| AGENTS.md "Repository workflow" section gains a repository-intelligence bullet | part of the existing managed section | a workflow is selected |

- **All of `graphify-out/` is ignored**, cache, manifest and report
  included. It is derived, machine-local output, and committing it would
  invite stale results that look current.
- **The skill** is short and written by VibeConform, not vendored from
  upstream. It says:
  - when to use `graphify query`, `graphify path` and `graphify explain`;
  - check freshness first: `graph.json`'s `built_at_commit` against
    `git rev-parse HEAD`, and `graphify update .` to rebuild;
  - if `graphify` or the graph is missing, or the graph is stale, say so
    and fall back to grep, the compiler, the LSP and tests;
  - an empty graph result is never evidence of absence;
  - graph output never replaces or skips required verification.
- **The `hook:context` line** reads `Graphify: graph present (check
  freshness: built_at_commit vs HEAD)` or `Graphify: no graph
  (graphify-out/graph.json absent; run graphify update .)`. It uses only
  the shell's built-in `test -f`, so the allowlist doesn't grow, and the
  task still exits 0.
- **The lefthook jobs** run `task graph:update`, and that task runs
  `graphify update .` (AST-only: no LLM, no network). The logic lives in
  Task's portable shell, not in `lefthook.yml`: on Windows, lefthook
  2.1.14 does not keep shell quoting in a `run:` line intact (it was
  probed during spec review), and Task is the canonical interface on
  every platform (principle 2).
  - If `graphify` isn't on PATH, the task prints one line saying the graph
    was not updated, and exits 0.
  - If the update itself fails, the job prints a loud "graph may be
    stale" line and exits 0. A post-commit or post-checkout failure
    cannot undo the operation anyway, and the graph must never block
    work.
  - Neither job is part of `pre-commit`, `pre-push`, `verify` or CI.
- **Codex** gets the AGENTS.md bullet only. It reads AGENTS.md, and its
  hooks are suspended.

### 3. Missing-tool and graph diagnostics

- **`vibe sync`**: `graphify` is declared as an *optional* tool. When it
  is not on PATH, sync prints the existing missing-tool warning ("used by
  the post-commit graph update"). It never installs it.
- **`vibe doctor`** adds one `graphify` block when the integration is
  selected, in fixed order:

| Check | Status |
|---|---|
| `graphify` not on PATH | `WARN` (optional capability), names `pip install graphifyy` / `uv tool install graphifyy` |
| `graphify --version` fails or times out | `WARN` |
| `graphify-out/graph.json` absent | `WARN`: "no graph; run `graphify update .`" |
| graph unreadable or not JSON | `WARN`: "graph unreadable; rebuild" |
| no top-level `built_at_commit` | `UNVERIFIED`: "freshness unknown" |
| `built_at_commit` ≠ HEAD | `WARN`: "stale: built at `abc1234`, HEAD `def5678`" |
| `built_at_commit` = HEAD | `PASS` ("working-tree changes are not reflected" when the tree is dirty) |
| `graphify-out/` not ignored by git | `WARN` |

  The graph check reads `graph.json` as a stream, so it never loads a
  large graph into memory. The doctor exit code contract is unchanged:
  graphify alone never makes doctor exit 1. Doctor writes nothing.
- With graphify unselected, doctor output is unchanged.

### 4. Deselection and removal

- Removing `graphify` from `vibe.yaml`:
  - prunes the `.gitignore` section and the skill (per spec 0026 §7, so
    a modified one is kept and reported as a conflict);
  - updates `Taskfile.yml`, `lefthook.yml` and AGENTS.md back to their
    unselected content.
- `graphify-out/` is never deleted by `vibe`, because VibeConform never
  recorded it. The docs say to delete it, or run
  `graphify uninstall --purge`.
- User-run upstream installs (for example a global
  `~/.claude/skills/graphify`) are never touched.

### 5. Example

`examples/python` selects `intelligence: [graphify]`, proving the
integration stays conformant under `TestExamplesAreConformant`. This
repository does not self-host it.

### 6. Documentation

- `docs/usage.md`, "Selecting integrations":
  - the catalog row;
  - install on Windows and Linux (`uv tool install graphifyy` or
    `pip install graphifyy`; `graphify --version`);
  - the supported upstream version;
  - what is generated;
  - troubleshooting: the graph isn't updating, a stale graph, the hook
    skipped, the Windows PATH for Python user scripts;
  - limitations: AST-only updates don't refresh semantic or doc
    extraction; `post-checkout` adds latency;
  - removal and licensing (MIT; nothing upstream is vendored).
- `docs/usage.md`: the doctor section, "What `vibe.yaml` means today",
  and "Removing VibeConform".
- `docs/architecture/overview.md`: the repository-intelligence section
  and the "not built yet" list.
- An ADR amending spec 0026's "no `.gitignore` editing" non-goal:
  managed sections in `.gitignore`, for integration-owned ignores only.

## Behavior

- Unselected (the default): nothing changes in any standard, example or
  this repository, apart from the recorded `vibe_version`.
- `task verify` and CI pass with neither graphify nor vibe installed.
- Selected, `vibe sync` followed by `vibe audit` is clean, and a second
  `sync` reports nothing.
- Selection, deselection and re-selection all round-trip through the
  existing Create, Remove and Forget decisions.

## Acceptance criteria

1. A manifest without `intelligence` resolves byte-identically to today
   in all four standards. A test covers this.
2. `intelligence: [graphify]` resolves exactly the §2 resources. With
   `agents: []` there is no skill and no `hook:context` line. With
   `agents: [codex]` there is no `.claude` file.
3. The `.gitignore` section preserves user lines byte-for-byte, and
   `graphify-out/graph.json` is reported ignored by `git check-ignore`.
4. The lefthook jobs exit 0 when graphify is absent and print the skip
   line. They exist only under `post-commit` and `post-checkout`.
5. Doctor returns each §3 status through test seams. Graphify-only
   problems keep exit code 0. Doctor writes nothing.
6. `sync` warns about a missing `graphify` and still exits 0.
7. Deselecting removes only the unmodified owned section and skill. A
   modified skill is kept as a conflict. `graphify-out/` is untouched.
8. `task verify` and CI never reference graphify. The
   verify-independence tests cover it.
9. `examples/python` is conformant with graphify selected.
10. The docs cover setup, limitations, maintenance and removal.

## Explicit non-goals

- Running `graphify` from `vibe sync`, or wrapping upstream's
  `graphify install`.
- A Graphify MCP server config (`.mcp.json` isn't managed yet).
- A PreToolUse hook. Session context and the skill cover discovery
  without a per-call cost.
- Codex skills or hooks.
- Semantic (LLM) extraction, `--watch`, or a global graph.
- Owned object keys (spec 0027).

## Assumptions not yet verified

- The upstream home: the issue links `Graphify-Labs/graphify`, but the
  installed 0.8.18 metadata says `safishamsi/graphify`. The docs name the
  PyPI package `graphifyy` and record whichever repository is current.
- `graphify update .` and the top-level `built_at_commit` in
  `graph.json` are stable across 0.8.x. This was verified locally on
  0.8.18 only. The docs state "verified with 0.8.18".
- `post-checkout` latency is acceptable on real repositories. It was
  measured only on a toy repository (about 1 s on Windows).

Verified during review, on Windows with lefthook 2.1.14, Task 3.53.1
and graphify 0.8.18:

- An inline compound `run:` in `lefthook.yml` is mangled. A
  `run: task graph:update` job works, and the skip branch exits 0.
- `graphify update .` writes `graph.json` whose last top-level key,
  `built_at_commit`, equals `git rev-parse HEAD`.
