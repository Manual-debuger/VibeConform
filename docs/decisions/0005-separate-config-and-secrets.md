# ADR 0005: Separate Configuration from Secrets

## Status

Accepted (policy only; enforcement not yet implemented).

## Context

Nothing in `docs/architecture/overview.md` or the `production` standard
currently addresses how a repository should organize environment
configuration. A common anti-pattern this project wants to guard against
is a single `.env` (or equivalent) file mixing plain configuration —
ports, hostnames, feature flags, log levels — with secrets — API keys,
database passwords, tokens, credentials.

These two categories have fundamentally different lifecycle and security
requirements:

- Plain config is safe to version-control, safe to print in logs/CI
  output, and has no rotation or access-restriction requirements.
- Secrets must never be committed, must be handled with restricted access
  and rotation, and must never appear in logs or CI output.

Mixing them in one file forces both categories into the stricter handling
(if the whole file is `.gitignore`'d, useful default config is lost from
version control) or the laxer one (if the file is tracked, secrets leak).
It also makes tooling — including VibeConform's own future compliance
checks — unable to distinguish "safe to read/display" from "must never be
displayed" without parsing file *content*, when the split should be
structural.

`internal/module.Module` today can only produce (`Resolve`) resources; it
cannot yet inspect a target repository's existing files and flag
violations — that compliance-checking capability is explicitly deferred
per `docs/specs/0004-vibe-audit-v1.md`'s non-goals. This ADR records the
policy rule now, ahead of the capability that would enforce it, mirroring
how ADR-0003 (resource ownership) preceded the reconciler that acts on it.

## Decision

Configuration and secrets must live in separate files:

- Non-secret configuration belongs in a version-controlled file (e.g. a
  tracked `config.env`, `config.yaml`, or equivalent).
- Secrets belong in a separate, untracked mechanism — a `.gitignore`'d
  file reserved exclusively for secrets, a secrets manager, or values
  injected at deploy time — never mixed into the tracked config file.

This is a policy `production`/`v1` should eventually check, not something
implemented by this ADR. No module exists yet that inspects a target
repository's env files; that is follow-on work, gated on real
compliance-checking capability landing (see Consequences).

## Consequences

- No code changes accompany this ADR. `production`/`v1` does not check
  this today.
- Follow-on work, once module compliance-checking exists (spec 0004's
  deferred follow-on): a module that reads a target repo's env file(s),
  classifies keys as secret-shaped (name matching `KEY`/`SECRET`/`TOKEN`/
  `PASSWORD`/`CREDENTIAL`, or similar) vs. config-shaped, and flags a
  violation when both appear in the same file. That follow-on needs its
  own spec — classification heuristics and false-positive handling are
  real design decisions, not covered here.
- VibeConform's own repository has no `.env` file today, so no immediate
  action is required here; if one is added later it must follow this
  policy itself, consistent with README's framing of this repo as the
  reference example for the standard it will eventually enforce on
  others.
