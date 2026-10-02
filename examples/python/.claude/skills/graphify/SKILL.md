---
name: graphify
description: Query the repository's Graphify knowledge graph (graphify-out/) for architecture and cross-file relationships, after checking that the graph is present and current. Use it before broad searches of an unfamiliar area.
---

This repository keeps a Graphify knowledge graph in `graphify-out/`. It is
derived output, ignored by Git, and rebuilt by `task graph:update`, which
the post-commit and post-checkout Git hooks run.

1. Check that it can be trusted before using it:
   - `graphify --version` works. If not, graphify is not installed: say so.
   - `graphify-out/graph.json` exists. If not, run `task graph:update`.
   - Its top-level `built_at_commit` equals `git rev-parse HEAD`. If not,
     the graph is stale: run `task graph:update`, or say that it is stale.
     Uncommitted changes are never in the graph.
2. Ask it focused questions: `graphify query "<question>"`,
   `graphify path "<A>" "<B>"`, `graphify explain "<concept>"`. Read
   `graphify-out/GRAPH_REPORT.md` only for a broad overview.
3. Fall back to search, the compiler, the language server and the tests
   whenever the graph is missing, stale, or does not answer. An empty
   graph result is not evidence that something does not exist.
4. The graph is static analysis. It never replaces or skips the
   repository's verification: run the checks AGENTS.md requires.
