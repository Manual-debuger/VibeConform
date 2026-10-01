---
name: spec
description: Write a lightweight spec (what must be true, with acceptance criteria) for a non-trivial change, then propose a plan and stop before implementing. Use it in plan mode, and whenever asked to plan or spec a change.
argument-hint: <feature or issue>
---

This is a planning context for: $ARGUMENTS
(If nothing follows the colon, it is for the change under discussion.)

Do not change code, configuration or tests while using this skill.

1. Read the specs, architecture docs and ADRs that apply (AGENTS.md says
   where they live), and the code involved.
2. List the constraints that apply and the assumptions you have not
   verified. Ask the user about the ones that would change the result.
3. If an approved spec already covers this, reuse it. Otherwise write one
   from the template below, where AGENTS.md says specs live
   (`docs/specs/<feature>.md` if it names no place). In a read-only plan
   mode, put it in the plan instead, and write it once it is approved.
   Keep it to WHAT must be true: no file-level steps, function design or
   sequencing.
4. Propose the implementation plan, the HOW, separately from the spec.
   Do not commit it unless the project asks for that.
5. Stop. Implement only after the user approves the spec and the plan.

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
