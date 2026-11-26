# VibeConform

VibeConform checks that a repository's tooling (tasks, hooks, CI and agent
guardrails) has the properties a versioned contract requires. A setup
skill gives the repository those properties; the audit only verifies them.

## Language

**Contract**:
The versioned list of rules an adopting repository must satisfy. It names
properties a repository must have, never the files or tools that provide
them.
_Avoid_: floor, standard, policy

**Rule**:
One property in the contract, with a stable ID such as `A-02`, a level and
a check. An ID is never reused once published.
_Avoid_: requirement, criterion, standard

**Level**:
How much a rule counts: a failed MUST rule makes the repository not
conformant; a failed SHOULD rule is only reported.
_Avoid_: severity, tier, grade

**Check**:
How the audit decides a rule: static (reads configuration), execution
(runs a command and inspects its result) or probe.
_Avoid_: test (alone), validation

**Part**:
One kind's share of a rule's check: its static, execution or probe
part. A part is named by its rule ID and kind, such as A-02's probe.
_Avoid_: half, step, sub-check

**Last part**:
The one execution or probe part that runs after every other execution
and probe part and gets their results. R-01's execution part is the last
part.
_Avoid_: need, final step

**Probe**:
A check that feeds a known input, such as a guard corpus case or a planted
defect, and verifies how the repository reacts.
_Avoid_: mutation test, smoke test, canary

**Probe copy**:
The throwaway Git repository in which the audit runs every execution
check and probe. It holds the working-tree content of every tracked file,
committed once. The audit resets it between runs and deletes it at the
end, so the adopter's own working tree and `.git` never change.
_Avoid_: sandbox, scratch repo, clone

**Planted defect**:
A small file the audit writes into the probe copy, in one component, that
one kind of check must catch: a format error, a static analysis error, a
failing test, a type error, an over-complex function, a secret, or a
static analysis error hidden by blanket suppressions. An entrypoint that
still passes with it planted has missed that kind.
_Avoid_: mutant, injected bug, fault

**Suppression**:
A comment in source code that silences a lint or type-check finding at
one place. A specific suppression names the rules it silences; a blanket
suppression names none. A planted defect must still fail `verify` under
blanket suppressions. Specific suppressions are the repository's choice.
_Avoid_: exception, waiver, ignore

**Guard corpus**:
The fixed set of tool calls that any agent guard must refuse or allow.
It belongs to the contract and is versioned with it.
_Avoid_: test cases, fixtures

**Entrypoint**:
A named command that an adopting repository declares (verify, verify-fast,
fmt, fmt-check) and that CI, hooks and probes call.
_Avoid_: task, target, script

**Conformant**:
The verdict for an adopting repository with no failed MUST rule under the
contract version it declares.
_Avoid_: compliant, passing, green

**Waiver**:
An adopting repository's statement, with a reason and an expiry date, that
it knowingly does not satisfy one SHOULD rule.
_Avoid_: exemption, suppression, ignore

**Not applicable**:
The state of a rule that does not apply to an adopting repository because
of what the repository declares, such as a Linux-only platform, or
because of the CI provider it uses. It is reported, not checked.
_Avoid_: skipped, waived

**Adopting repository**:
A repository that declares a contract version and is checked against it.
"Adopter" is an acceptable short form.
_Avoid_: adapter, target repository, downstream repository

**Component**:
One single-language project inside an adopting repository, named by its
path and language. A single-language repository is one component at its
root.
_Avoid_: package, workspace, sub-project

**Language record**:
Everything `vibe` knows about one component language, in one place: its
identifier, manifest, lockfiles, test files, planted defects and
update-bot names. Each language the contract supports has exactly one.
_Avoid_: language table, language config, language plugin

**Setup skill**:
The agent skill that gives an adopting repository the properties the
contract requires, and runs the audit until it passes. It is
`vibeconform-setup`, embedded in `vibe`; `vibe init --skill` writes it
into the repository's skill paths, where the repository owns it.
_Avoid_: installer, generator, template

## Agent guardrails

**Agent guard**:
The check that runs before an agent's shell command or file edit and
refuses a short list of destructive commands and edits to secret files.
It stops common accidents. It is not a security boundary, and it does not
refuse every command with the same effect. `vibe-guard` is VibeConform's
reference agent guard; any guard that passes the guard probes will do.
_Avoid_: sandbox, firewall, policy engine

**Guard exception**:
An adopting repository's statement that one agent guard rule lets one
named command through. Every other rule still applies to it. It is the
only way to loosen the guard.
_Avoid_: allow rule, whitelist, override

**Unlock**:
A person's time-limited permission for an agent to change the agent
guardrail settings. An agent cannot grant it to itself. The person runs
`vibe unlock` in their own terminal; the agent's next Stop, or
`vibe-guard relock`, ends it.
_Avoid_: override, bypass, escape

**Project root**:
The directory an agent session runs in: `CLAUDE_PROJECT_DIR` when it is
set. Otherwise it is the Git top found from the working directory, or
that directory when no `.git` is above it. The guard reads the policy,
relative paths and the agent guardrail settings against it. A nested
project's root is below the Git top.
_Avoid_: repository root, sub-project

**Git top**:
The directory that holds `.git`, found by walking up from the project
root. It is the project root unless the project is below it. The unlock
marker and `vibe-guard.log` are in its common Git directory.
_Avoid_: repository root, Git root

## CI

**CI provider**:
The CI service whose configuration the audit reads for the CI rules:
GitHub Actions or GitLab CI. A repository has one CI provider as the
audit sees it; when both are configured, GitHub Actions is the one read.
_Avoid_: CI platform, CI system, forge, host

**Pipeline kind**:
One of the two situations the CI rules ask a job about: a merge request
(a pull request on GitHub) and a push to the default branch. A job either
runs for a pipeline kind or does not.
_Avoid_: event, trigger, pipeline source, pipeline type

**Required check**:
A CI job whose name stays stable so that branch protection can require
it: the gate and the conformance audit. Other job names may change.
_Avoid_: status check (alone), gate job (for conformance)

**Setup action**:
The GitHub Action at this repository's root that installs an exact
version of `vibe` and `vibe-guard` in the conformance job: from a release
with its checksums checked, or built from source. It never runs the audit
itself, and `verify` must not use it.
_Avoid_: installer, setup-vibe, bootstrap step
