<!-- vibeconform:begin specs -->
# Specs

A spec says what must be true; it does not say how to change the code.
Write one file per feature, `docs/specs/<feature>.md` (a numeric prefix
is fine), starting with a Status line: draft, accepted, implemented or
superseded. A spec is a living document: update it when the behavior it
describes changes.

Template:

```markdown
# Feature: <name>

Status: draft

## Problem

What is wrong or missing, and for whom.

## Constraints

Architecture, compatibility and product rules this must not break.

## Assumptions

What this takes as true without evidence from the repository or the user.

## Desired Behavior

What must be true when this is done, observable from outside.

## Non-goals

What this deliberately leaves out.

## Acceptance Criteria

Checkable statements; each is verified before the work is called done.

- [ ] ...
```
<!-- vibeconform:end specs -->
