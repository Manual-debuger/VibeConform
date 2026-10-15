# VibeConform

VibeConform generates and audits a repository's tooling (tasks, hooks, CI,
lint and agent configuration) from versioned standards, so the repository
stays conformant over time.

## Language

**Standard**:
A named, versioned bundle of modules that defines a repository's tooling,
such as `prod-go/v1` or `prod-mono/v1`.
_Avoid_: profile, preset, template set

**Adopting repository**:
A repository that VibeConform manages under a standard. "Adopter" is an
acceptable short form.
_Avoid_: adapter, target repository, downstream repository

**Component**:
One single-language project inside a `prod-mono` repository, named by
its id and laid out as the matching single-language standard.
_Avoid_: package, workspace, sub-project

**Agent guard**:
The check that runs before an agent's shell command or file edit and
refuses a short list of destructive commands and edits to secret files.
It stops common accidents. It is not a security boundary, and it does not
refuse every command with the same effect.
_Avoid_: sandbox, firewall, policy engine

**Floor**:
The part of a repository's tooling that decides what the gate runs and
whether its result counts. No seam can change it.
_Avoid_: lock, locked rule, contract

**Guard exception**:
An adopting repository's statement that one agent guard rule lets one
named command through. Every other rule still applies to it.
_Avoid_: allow rule, whitelist, override

**Base config**:
The managed half of a lint or compiler configuration, which the
repository's own configuration under the tool's usual name extends.
_Avoid_: managed config (alone), vendor config, default config

## CI

**CI floor**:
The floor's part in a generated CI job: what the job runs and whether
its result counts. VibeConform owns it; an adopting repository cannot
change it.
_Avoid_: hardening, contract (alone)

**Seam**:
A documented place where an adopting repository changes generated
behaviour without editing a managed file, such as a local extension file
or a CI/CD variable.
_Avoid_: hook, override, escape hatch

**Required check**:
A CI job whose name VibeConform keeps stable so that branch protection can
require it: the gate and the conformance audit. Other job names may change.
_Avoid_: status check (alone), gate job (for conformance)
