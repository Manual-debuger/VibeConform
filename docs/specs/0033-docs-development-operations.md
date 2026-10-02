# Spec 0033: Optional `docs/development/` and `docs/operations/`

Status: accepted and implemented. Tracks issue #49 ("`docs/development/`
and `docs/operations/`"). Extends spec 0030 §4 (the docs layout).

## Problem

- The standard docs layout of spec 0030 names three directories: specs,
  architecture and decisions. It has no place for contributor guides
  (how to build, test and contribute locally) or for operations
  (deploying, running, incidents).
- Projects invent their own place for these, and the agents are not told
  where to look.
- Not every repository needs both. A library has nothing to operate.

## Scope

### 1. Two keys under `development:`

```yaml
development:
  docs_layout: standard
  docs_development: on   # adds docs/development/
  docs_operations: on    # adds docs/operations/
```

- Each key takes one value, `on`. An absent key means off, and any other
  value is rejected with the usual `(valid: on)` error.
- The two are independent of each other.
- Each **requires `docs_layout`**. Selecting either one without it is an
  error naming both keys, for example
  `development.docs_operations: on requires development.docs_layout: standard, which is not selected`.

### 2. What they change

- The `docs` section of `docs/README.md` lists each selected directory
  after the standard three:
  - `development/`: building, testing and contributing locally;
  - `operations/`: deploying, running and handling incidents.
- The knowledge rule of the `AGENTS.md` `workflow` section names each
  selected directory, so agents know where to look. The 300-word cap of
  spec 0030 §3 still holds for the largest selection.
- No file is created in either directory, as with `architecture/` and
  `decisions/` (spec 0030 §4). They appear with the project's first
  document there.

### 3. Deselection

Turning a key off updates both sections back to the text without that
directory. That is an update, not a removal.

## Explicit non-goals

- Templates or starter documents for either directory.
- Deciding whether a project is deployable.
- Other directories, or renaming these two (spec 0034 covers using
  existing locations).

## Acceptance criteria

- [ ] Either key without `docs_layout` is an error that names both keys.
- [ ] A value other than `on` is rejected with `(valid: on)`.
- [ ] Golden tests cover the docs index and the `AGENTS.md` knowledge rule for neither, development, operations and both. The word cap holds for the largest selection.
- [ ] Turning a key off is reported as an update of both sections, and restores the text without that directory.
