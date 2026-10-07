<!-- vibeconform:begin intelligence -->

## Repository intelligence

Managed by VibeConform from `integrations.intelligence` in `vibe.yaml`.

- Graphify keeps a knowledge graph in `graphify-out/`. Use it only when
  `built_at_commit` in `graph.json` matches HEAD (`task graph:update`
  rebuilds it); otherwise, or when it is absent, use search, the
  compiler and tests. It never replaces verification.

<!-- vibeconform:end intelligence -->

<!-- vibeconform:begin knowledge -->

## Repository documents

Managed by VibeConform from `development.docs_layout` in `vibe.yaml`.

- Specs (what must be true) live in `docs/specs/`, architecture (how it
  works now) in `docs/architecture/`, decisions (ADRs) in
  `docs/decisions/`. Read the relevant ones before a non-trivial change.
- If an approved spec, an ADR and the code disagree, say so. Do not
  pick one silently.

<!-- vibeconform:end knowledge -->
