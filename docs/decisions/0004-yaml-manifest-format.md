# ADR 0004: YAML for `vibe.yaml`

## Status

Accepted.

## Context

`vibe.yaml` currently holds two flat fields (`standard`, `version`), but
`docs/architecture/overview.md` already anticipates it growing to describe
a component graph:

```yaml
components:
  - id: web
    path: apps/web
    profile: typescript
    depends_on: [contracts]

  - id: api
    path: services/api
    profile: go
    depends_on: [contracts]
```

TOML was the main alternative considered.

## Decision

Use YAML (`gopkg.in/yaml.v3`), not TOML.

- Nested, list-heavy structures like the component graph above are
  expressed naturally in YAML. The equivalent in TOML requires
  array-of-tables syntax (`[[components]]`) that gets noisier as nesting
  grows, and TOML has no compact inline form for a list of maps like
  `depends_on: [contracts]` embedded in a table.
- Every other piece of repository configuration this project already
  produces or consumes is YAML: GitHub Actions workflows,
  `.github/dependabot.yml`, `.golangci.yml`, `lefthook.yml`,
  `.goreleaser.yaml`. Using YAML for `vibe.yaml` avoids introducing a
  second config format for repository maintainers to context-switch
  between.

## Consequences

- YAML's well-known footguns (the "Norway problem" — bareword `yes`/`no`/
  `on`/`off` parsed as booleans; whitespace-significant indentation) apply.
  `internal/manifest.Parse` validates required fields explicitly rather
  than relying on YAML's implicit typing, which mitigates but does not
  eliminate this.
- TOML's advantages (unambiguous scalar types, a simpler non-indentation-
  sensitive grammar) were judged not to outweigh the nesting cost once the
  component graph lands — revisit only if `vibe.yaml` ends up staying flat
  long-term, which the architecture doc suggests it will not.
