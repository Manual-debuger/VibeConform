package contract

import (
	"slices"
)

// Level is how binding a rule is (spec 0051, "The verdict"): a failed
// MUST rule makes a repository not conformant; a failed SHOULD rule is
// reported and never changes the verdict.
type Level string

// The levels of contract 1.0.0.
const (
	Must   Level = "MUST"
	Should Level = "SHOULD"
)

// Category groups rules by the property they are about. It is the rule
// ID's letter.
type Category string

// The categories of contract 1.0.0, in report order.
const (
	CategoryVerification Category = "V"
	CategoryHooks        Category = "H"
	CategoryCI           Category = "C"
	CategoryAgents       Category = "A"
	CategoryDependencies Category = "D"
	CategoryRemovability Category = "R"
)

// categoryInfo is a category's long name, after spec 0051's headings, and
// the introduction spec 0051 gives the category, where it has one.
var categoryInfo = []struct {
	category Category
	name     string
	intro    string
}{
	{CategoryVerification, "Verification entrypoints", ""},
	{CategoryHooks, "Local Git hooks", "Recognized hook managers: lefthook (`lefthook.yml`, `.lefthook.yml`), " +
		"pre-commit (`.pre-commit-config.yaml`) and husky (`.husky/`). Adding one is MINOR. " +
		"A repository that uses another manager gets \"not recognized\" on both rules."},
	{CategoryCI, "CI", "Contract v1 supports GitHub Actions only (Q19). If the repository has CI configuration " +
		"for another provider and no `.github/workflows/`, the C rules cannot be decided."},
	{CategoryAgents, "Agent guardrails", "The A rules cover Claude Code only. Codex is exempt; bringing it in is MAJOR. " +
		"Personal, uncommitted settings (`.claude/settings.local.json`) may override any guardrail, " +
		"and the audit does not read them."},
	{CategoryDependencies, "Dependencies and supply chain", ""},
	{CategoryRemovability, "Removability and repository hygiene", ""},
}

// Categories returns the categories in report order.
func Categories() []Category {
	out := make([]Category, len(categoryInfo))
	for i, c := range categoryInfo {
		out[i] = c.category
	}
	return out
}

// Name is the category's long name, such as "Agent guardrails".
func (c Category) Name() string {
	for _, info := range categoryInfo {
		if info.category == c {
			return info.name
		}
	}
	return string(c)
}

// Intro is spec 0051's introduction to the category, or "" when it has
// none.
func (c Category) Intro() string {
	for _, info := range categoryInfo {
		if info.category == c {
			return info.intro
		}
	}
	return ""
}

// PartKind is the kind of one part of a rule's check (spec 0051,
// "Checks").
type PartKind string

// The kinds of check.
const (
	// Static reads committed configuration and runs nothing.
	Static PartKind = "static"
	// Execution runs entrypoints or other commands.
	Execution PartKind = "execution"
	// Probe feeds a known input and checks the reaction.
	Probe PartKind = "probe"
)

// CheckFunc decides one part of a rule against a repository. It reads
// only what env offers and never writes.
type CheckFunc func(env *Env) PartResult

// Part is one part of a rule's check.
type Part struct {
	Kind PartKind
	// Check is the part's check as spec 0051 states it.
	Check string
	// Run decides the part. Nil means this vibe does not implement it
	// yet: the part cannot be decided, and Issue names the ticket that
	// adds it.
	Run CheckFunc
	// Issue is the GitHub issue that implements a part Run is nil for.
	Issue int
}

// Rule is one rule of contract 1.0.0 (spec 0051, "Rules"). The report
// and --explain read only the registry.
type Rule struct {
	ID       string
	Level    Level
	Category Category
	// Title is the rule sentence of spec 0051.
	Title string
	// Check is the rule's check as spec 0051 states it, every part.
	Check string
	// Source names the specs, ADRs and principles the rule comes from.
	Source string
	// NotApplicableWhen says when the rule is not applicable, or "" when
	// it always applies.
	NotApplicableWhen string
	Parts             []Part
	// applies decides applicability; nil means the rule always applies.
	// It returns why the rule is not applicable.
	applies func(cfg *Config) (bool, string)
}

// Waivable reports whether a waiver may suspend the rule: only SHOULD
// rules can be waived (spec 0051, "vibe.yaml").
func (r Rule) Waivable() bool { return r.Level == Should }

// Rationale is why the rule exists: its sources and, where spec 0051
// introduces the rule's category, that introduction.
func (r Rule) Rationale() string {
	text := "Source: " + r.Source + "."
	if intro := r.Category.Intro(); intro != "" {
		text += " " + intro
	}
	return text
}

// Kinds lists the rule's check kinds in order, without repeats.
func (r Rule) Kinds() []PartKind {
	var kinds []PartKind
	for _, p := range r.Parts {
		if !slices.Contains(kinds, p.Kind) {
			kinds = append(kinds, p.Kind)
		}
	}
	return kinds
}

// Applies reports whether the rule applies to a configuration, and why
// not when it does not.
func (r Rule) Applies(cfg *Config) (bool, string) {
	if r.applies == nil {
		return true, ""
	}
	return r.applies(cfg)
}

func static(check string, run CheckFunc) Part {
	return Part{Kind: Static, Check: check, Run: run}
}

func execution(check string, issue int) Part {
	return Part{Kind: Execution, Check: check, Issue: issue}
}

// executionRun is an execution part this vibe implements.
func executionRun(check string, run CheckFunc) Part {
	return Part{Kind: Execution, Check: check, Run: run}
}

func probe(check string, issue int) Part {
	return Part{Kind: Probe, Check: check, Issue: issue}
}

func needsWindows(cfg *Config) (bool, string) {
	if slices.Contains(cfg.Platforms, PlatformWindows) {
		return true, ""
	}
	return false, "platforms does not include windows"
}

func needsClaude(cfg *Config) (bool, string) {
	if slices.Contains(cfg.Agents, AgentClaude) {
		return true, ""
	}
	return false, "agents does not include claude"
}

func needsAgents(cfg *Config) (bool, string) {
	if len(cfg.Agents) > 0 {
		return true, ""
	}
	return false, "agents is empty"
}

const notApplicableWithoutClaude = "when `agents` does not include `claude`"

// rules is contract 1.0.0's rule table, in report order: 16 MUST and 9
// SHOULD rules. Rule sentences, checks and sources are spec 0051's,
// verbatim.
var rules = []Rule{
	{
		ID: "V-01", Level: Must, Category: CategoryVerification,
		Title: "`verify` covers formatting, static analysis and tests",
		Check: "Execution and probe: in the probe copy, `verify` exits 0. With each planted defect in each " +
			"component, it exits non-zero. The report names each defect kind (format, static analysis, test) " +
			"that did not make it fail.",
		Source: "spec 0011, principle 2",
		Parts: []Part{
			execution("in the probe copy, `verify` exits 0.", 104),
			probe("With each planted defect in each component, it exits non-zero. The report names each defect "+
				"kind (format, static analysis, test) that did not make it fail.", 104),
		},
	},
	{
		ID: "V-02", Level: Must, Category: CategoryVerification,
		Title:  "`verify-fast` catches the same defects",
		Check:  "Probe: with each planted defect in each component, `verify-fast` exits non-zero. The contract does not measure speed.",
		Source: "spec 0023",
		Parts: []Part{
			probe("with each planted defect in each component, `verify-fast` exits non-zero. The contract does not measure speed.", 104),
		},
	},
	{
		ID: "V-03", Level: Must, Category: CategoryVerification,
		Title: "`fmt-check` writes no file",
		Check: "Execution: in the probe copy with the unformatted-file defect planted, after `fmt-check` runs, " +
			"`git status --porcelain` lists only the planted file, with its content unchanged",
		Source: "spec 0011",
		Parts: []Part{
			executionRun("in the probe copy with the unformatted-file defect planted, after `fmt-check` runs, "+
				"`git status --porcelain` lists only the planted file, with its content unchanged", checkV03),
		},
	},
	{
		ID: "V-04", Level: Must, Category: CategoryVerification,
		Title:  "The entrypoints are declared and used",
		Check:  "Static: `vibe.yaml` declares every required entrypoint, each non-empty. The other rules check that CI and hooks call them.",
		Source: "ADR 0025 (replaces ADR 0009's protected task names)",
		Parts: []Part{
			static("`vibe.yaml` declares every required entrypoint, each non-empty. The other rules check that CI and hooks call them.", checkV04),
		},
	},
	{
		ID: "H-01", Level: Should, Category: CategoryHooks,
		Title:  "Hook configuration is committed and installs with one command",
		Check:  "Static: a recognized hook manager's configuration is committed",
		Source: "spec 0011",
		Parts:  []Part{static("a recognized hook manager's configuration is committed", checkH01)},
	},
	{
		ID: "H-02", Level: Should, Category: CategoryHooks,
		Title:  "pre-commit checks formatting; pre-push runs verification",
		Check:  "Static: the pre-commit hook calls `fmt-check` or `fmt`; the pre-push hook calls `verify-fast` or `verify`",
		Source: "spec 0011, ADR 0017",
		Parts: []Part{
			static("the pre-commit hook calls `fmt-check` or `fmt`; the pre-push hook calls `verify-fast` or `verify`", checkH02),
		},
	},
	{
		ID: "C-01", Level: Must, Category: CategoryCI,
		Title:  "CI runs `verify` on pull requests and on pushes to the default branch",
		Check:  "Static: workflows triggered by `pull_request` and by `push` to the default branch call `verify`",
		Source: "spec 0010",
		Parts: []Part{
			static("workflows triggered by `pull_request` and by `push` to the default branch call `verify`", checkC01),
		},
	},
	{
		ID: "C-02", Level: Must, Category: CategoryCI,
		Title: "Toolchains and actions are pinned",
		Check: "Static, across every workflow (not only generated ones): every `uses:` other than a local path is " +
			"pinned to a full 40-character commit SHA, and no `run:` step contains a command word ending in `@latest`",
		Source: "spec 0044",
		Parts: []Part{
			static("across every workflow (not only generated ones): every `uses:` other than a local path is "+
				"pinned to a full 40-character commit SHA, and no `run:` step contains a command word ending in `@latest`", checkC02),
		},
	},
	{
		ID: "C-03", Level: Must, Category: CategoryCI,
		Title: "Workflow permissions default to read",
		Check: "Static: every workflow has top-level `permissions` that grant no write scope (`contents: read`, " +
			"`read-all` or `{}`). Jobs may widen them.",
		Source: "spec 0010",
		Parts: []Part{
			static("every workflow has top-level `permissions` that grant no write scope (`contents: read`, "+
				"`read-all` or `{}`). Jobs may widen them.", checkC03),
		},
	},
	{
		ID: "C-04", Level: Must, Category: CategoryCI,
		Title:  "The audit runs in its own job",
		Check:  "Static: a job triggered on pull requests runs `vibe audit` (or the setup action followed by it) and does not call `verify`",
		Source: "spec 0022",
		Parts: []Part{
			static("a job triggered on pull requests runs `vibe audit` (or the setup action followed by it) and does not call `verify`", checkC04),
		},
	},
	{
		ID: "C-05", Level: Should, Category: CategoryCI,
		Title: "Tests run on Linux and Windows",
		Check: "Static: the steps that call `verify` run on a Linux runner and on a Windows runner, after matrix " +
			"expansion. Not applicable when `platforms` does not include `windows`.",
		Source:            "principle 2",
		NotApplicableWhen: "when `platforms` does not include `windows`",
		Parts: []Part{
			static("the steps that call `verify` run on a Linux runner and on a Windows runner, after matrix "+
				"expansion. Not applicable when `platforms` does not include `windows`.", checkC05),
		},
		applies: needsWindows,
	},
	{
		ID: "A-01", Level: Must, Category: CategoryAgents,
		Title:             "Shell commands and file edits pass through the guard",
		Check:             "Static: in `.claude/settings.json`, PreToolUse hooks match each of `Bash`, `PowerShell`, `Write`, `Edit` and `MultiEdit`",
		Source:            "spec 0021",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("in `.claude/settings.json`, PreToolUse hooks match each of `Bash`, `PowerShell`, `Write`, `Edit` and `MultiEdit`", checkA01),
		},
		applies: needsClaude,
	},
	{
		ID: "A-02", Level: Must, Category: CategoryAgents,
		Title: "The guard refuses the baseline unless an exception says otherwise",
		Check: "Probe: every corpus case is fed to every PreToolUse hook command that matches its tool. A deny case " +
			"is satisfied as described under \"Guard policy and exceptions\". Every allow case must be allowed. " +
			"A hook \"refuses\" by exiting 2.",
		Source:            "specs 0012, 0047",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			probe("every corpus case is fed to every PreToolUse hook command that matches its tool. A deny case "+
				"is satisfied as described under \"Guard policy and exceptions\". Every allow case must be allowed. "+
				"A hook \"refuses\" by exiting 2.", 103),
		},
		applies: needsClaude,
	},
	{
		ID: "A-03", Level: Must, Category: CategoryAgents,
		Title: "The guard fails closed",
		Check: "Probe: every deny case is still refused in two situations: with `PATH` reduced to the operating " +
			"system's directories, and with `.claude/hooks/policy.json` replaced by invalid JSON",
		Source:            "spec 0021",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			probe("every deny case is still refused in two situations: with `PATH` reduced to the operating "+
				"system's directories, and with `.claude/hooks/policy.json` replaced by invalid JSON", 103),
		},
		applies: needsClaude,
	},
	{
		ID: "A-04", Level: Must, Category: CategoryAgents,
		Title: "The guard is loosened only through listed exceptions",
		Check: "Static: `.claude/hooks/policy.json` is valid version 2; every exception names a guard rule ID and " +
			"has a non-empty reason; the audit lists them. Tightening is not restricted.",
		Source:            "spec 0050, ADR 0024",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("`.claude/hooks/policy.json` is valid version 2; every exception names a guard rule ID and "+
				"has a non-empty reason; the audit lists them. Tightening is not restricted.", checkA04),
		},
		applies: needsClaude,
	},
	{
		ID: "A-05", Level: Must, Category: CategoryAgents,
		Title: "The agent's turn ends with `verify-fast`",
		Check: "Static: a Stop hook calls `verify-fast`. Probe: with a planted defect, the Stop hook exits 2 for a " +
			"payload with `stop_hook_active: false`, and exits 0 for one with `stop_hook_active: true`",
		Source:            "spec 0023",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("a Stop hook calls `verify-fast`.", checkA05),
			probe("with a planted defect, the Stop hook exits 2 for a payload with `stop_hook_active: false`, "+
				"and exits 0 for one with `stop_hook_active: true`", 104),
		},
		applies: needsClaude,
	},
	{
		ID: "A-06", Level: Should, Category: CategoryAgents,
		Title:             "Edited files are formatted",
		Check:             "Static: a PostToolUse hook that matches `Write` and `Edit` calls `fmt`",
		Source:            "spec 0023",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts:             []Part{static("a PostToolUse hook that matches `Write` and `Edit` calls `fmt`", checkA06)},
		applies:           needsClaude,
	},
	{
		ID: "A-07", Level: Should, Category: CategoryAgents,
		Title:             "Session start injects facts and changes nothing",
		Check:             "Static: a SessionStart hook exists. Execution: running it in the probe copy exits 0, and `git status --porcelain` stays empty.",
		Source:            "spec 0023",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("a SessionStart hook exists.", checkA07),
			// Spec 0052, "Further Notes": A-07's execution part belongs
			// with #102's execution checks.
			executionRun("running it in the probe copy exits 0, and `git status --porcelain` stays empty.", checkA07Execution),
		},
		applies: needsClaude,
	},
	{
		ID: "A-08", Level: Must, Category: CategoryAgents,
		Title: "An agent changes guardrail settings only after a person unlocks them",
		Check: "Probe: a `Write` to `.claude/settings.json` and an `Edit` under `.claude/hooks/` are refused with no " +
			"marker, allowed with a valid marker, and refused with an expired marker. A `Bash` call that runs " +
			"`vibe unlock`, and any `Write` or `Bash` that creates the marker, are refused. After the Stop hook " +
			"runs, the marker is gone.",
		Source:            "ADR 0025",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			probe("a `Write` to `.claude/settings.json` and an `Edit` under `.claude/hooks/` are refused with no "+
				"marker, allowed with a valid marker, and refused with an expired marker. A `Bash` call that runs "+
				"`vibe unlock`, and any `Write` or `Bash` that creates the marker, are refused. After the Stop hook "+
				"runs, the marker is gone.", 106),
		},
		applies: needsClaude,
	},
	{
		ID: "D-01", Level: Must, Category: CategoryDependencies,
		Title: "Lockfiles are committed and match the declared dependencies",
		Check: "Execution, per component, by the lockfile found. For `go.sum`: run `go mod tidy`, then " +
			"`git diff --exit-code go.mod go.sum`. For `uv.lock`: `uv lock --check`. For `poetry.lock`: " +
			"`poetry check --lock`. For `pnpm-lock.yaml`: `pnpm install --frozen-lockfile --lockfile-only`. " +
			"For `package-lock.json`: `npm ci --dry-run`. A component with dependencies and no recognized " +
			"lockfile fails.",
		Source: "specs 0010, 0020",
		Parts: []Part{
			executionRun("per component, by the lockfile found. For `go.sum`: run `go mod tidy`, then "+
				"`git diff --exit-code go.mod go.sum`. For `uv.lock`: `uv lock --check`. For `poetry.lock`: "+
				"`poetry check --lock`. For `pnpm-lock.yaml`: `pnpm install --frozen-lockfile --lockfile-only`. "+
				"For `package-lock.json`: `npm ci --dry-run`. A component with dependencies and no recognized "+
				"lockfile fails.", checkD01),
		},
	},
	{
		ID: "D-02", Level: Should, Category: CategoryDependencies,
		Title: "An update bot covers dependencies and actions, without automerge",
		Check: "Static: `.github/dependabot.yml` or a Renovate configuration covers each component's ecosystem " +
			"and `github-actions`, and enables no automerge",
		Source: "spec 0010",
		Parts: []Part{
			static("`.github/dependabot.yml` or a Renovate configuration covers each component's ecosystem "+
				"and `github-actions`, and enables no automerge", checkD02),
		},
	},
	{
		ID: "D-03", Level: Should, Category: CategoryDependencies,
		Title: "CI scans for known vulnerabilities",
		Check: "Static: a workflow runs a recognized scanner: `govulncheck`, `pip-audit`, `pnpm audit`, " +
			"`npm audit`, `osv-scanner` or `trivy`. Adding one is MINOR.",
		Source: "spec 0010",
		Parts: []Part{
			static("a workflow runs a recognized scanner: `govulncheck`, `pip-audit`, `pnpm audit`, "+
				"`npm audit`, `osv-scanner` or `trivy`. Adding one is MINOR.", checkD03),
		},
	},
	{
		ID: "R-01", Level: Must, Category: CategoryRemovability,
		Title: "Everything but the audit works without vibe",
		Check: "Static: no entrypoint, hook command or CI step calls `vibe`, except the audit job of C-04. " +
			"Execution: with `vibe` not on `PATH`, `verify` passes in the probe copy and the A-02 deny cases " +
			"are still refused.",
		Source: "principle 3, spec 0017",
		Parts: []Part{
			static("no entrypoint, hook command or CI step calls `vibe`, except the audit job of C-04.", checkR01),
			executionRun("with `vibe` not on `PATH`, `verify` passes in the probe copy and the A-02 deny cases "+
				"are still refused.", checkR01Execution),
		},
	},
	{
		ID: "R-02", Level: Should, Category: CategoryRemovability,
		Title:             "Agents are told the verification entrypoints",
		Check:             "Static: `AGENTS.md` exists and contains every `verify` command, and `CLAUDE.md` exists and references `AGENTS.md`",
		Source:            "spec 0043",
		NotApplicableWhen: "when `agents` is empty; its `CLAUDE.md` half when `agents` does not include `claude`",
		Parts: []Part{
			static("`AGENTS.md` exists and contains every `verify` command, and `CLAUDE.md` exists and references `AGENTS.md`", checkR02),
		},
		applies: needsAgents,
	},
	{
		ID: "R-03", Level: Should, Category: CategoryRemovability,
		Title:  "Pull requests report verification",
		Check:  "Static: a pull request template exists and has a heading containing \"verif\" (case-insensitive)",
		Source: "spec 0010",
		Parts: []Part{
			static("a pull request template exists and has a heading containing \"verif\" (case-insensitive)", checkR03),
		},
	},
}

// Rules returns every rule of contract 1.0.0 in report order. Each rule
// has its own copy of its parts.
func Rules() []Rule {
	out := slices.Clone(rules)
	for i := range out {
		out[i].Parts = slices.Clone(out[i].Parts)
	}
	return out
}

// LookupRule returns the rule with the given ID.
func LookupRule(id string) (Rule, bool) {
	i := slices.IndexFunc(rules, func(r Rule) bool { return r.ID == id })
	if i < 0 {
		return Rule{}, false
	}
	r := rules[i]
	r.Parts = slices.Clone(r.Parts)
	return r, true
}

// RuleIDs returns every rule ID in report order.
func RuleIDs() []string {
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	return ids
}
