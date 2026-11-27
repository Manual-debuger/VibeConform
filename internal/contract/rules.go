package contract

import (
	"slices"
	"strings"
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
	categoryVerification Category = "V"
	categoryHooks        Category = "H"
	categoryCI           Category = "C"
	categoryAgents       Category = "A"
	categoryDependencies Category = "D"
	categoryRemovability Category = "R"
)

// categoryInfo is a category's long name, after spec 0051's headings, and
// the introduction spec 0051 gives the category, where it has one.
var categoryInfo = []struct {
	category Category
	name     string
	intro    string
}{
	{categoryVerification, "Verification entrypoints", ""},
	{categoryHooks, "Local Git hooks", "Recognized hook managers: lefthook (`lefthook.yml`, `.lefthook.yml`), " +
		"pre-commit (`.pre-commit-config.yaml`) and husky (`.husky/`). Adding one is MINOR. " +
		"A repository that uses another manager gets \"not recognized\" on both rules."},
	{categoryCI, "CI", "The C rules read GitHub Actions or GitLab CI (GitLab CI since 1.1.0). " +
		"GitHub Actions wins: when the repository has workflows in `.github/workflows/`, the C rules read them only. " +
		"GitLab CI (`.gitlab-ci.yml`) is read when there are none. If the repository has CI configuration " +
		"for another provider and neither, the C rules cannot be decided. The report names the provider it read."},
	{categoryAgents, "Agent guardrails", "The A rules cover Claude Code only. Codex is exempt; bringing it in is MAJOR. " +
		"Personal, uncommitted settings (`.claude/settings.local.json`) may override any guardrail, " +
		"and the audit does not read them."},
	{categoryDependencies, "Dependencies and supply chain", ""},
	{categoryRemovability, "Removability and repository hygiene", ""},
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

// intro is spec 0051's introduction to the category, or "" when it has
// none.
func (c Category) intro() string {
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
// env and returns what it decided (spec 0064).
type CheckFunc func(env *Env) PartResult

// LastFunc decides the last part: it reads env and the results of every
// execution and probe part that ran before it (spec 0071).
type LastFunc func(env *Env, earlier Earlier) PartResult

// Part is one part of a rule's check. It sets exactly one of Run and
// RunLast.
type Part struct {
	Kind PartKind
	// Lead is the text spec 0051 writes before the part's check in the
	// rule's check, such as "Static: ", "Execution and probe: " or
	// "Static, ". An empty lead continues the text of the part before it.
	Lead string
	// Check is the part's check as spec 0051 states it, without its
	// lead. --explain prints it as the part's check.
	Check string
	// Run decides every part but the last part.
	Run CheckFunc
	// RunLast decides the last part: the one execution or probe part
	// that runs after every other execution and probe part (spec 0071).
	RunLast LastFunc
}

// LevelChange is a contract version that changed a rule's level, and the
// level the rule has from that version on (spec 0074, "A rule's level
// depends on the audited contract").
type LevelChange struct {
	// Since is the contract version that changed the level, such as
	// "2.1.0".
	Since string
	Level Level
}

// Revision is a contract version that changed what a rule says and when
// it applies, with the rule's text from that version on (spec 0074). It
// replaces the rule's title, the check text of each part and its
// applicability, all together. The parts' check functions do not change:
// a check that differs between contract versions reads the audited
// version itself.
type Revision struct {
	// Since is the contract version that made the change, such as "2.1.0".
	Since string
	Title string
	// Checks are the parts' check texts, without their leads: one for each
	// part, in part order.
	Checks            []string
	NotApplicableWhen string
	// applies is the applicability from Since on; nil means the rule
	// always applies.
	applies func(env *Env) (bool, string)
}

// Rule is one rule of contract 1.0.0 (spec 0051, "Rules"). The report
// and --explain read only the registry.
type Rule struct {
	ID string
	// Level is the level the rule was added with, or, on a rule At
	// returned, its level under that contract. A registry rule's level
	// under a contract is LevelAt's, and a RuleResult carries the level
	// under the audited contract.
	Level Level
	// LevelChanges are the contract versions that changed the rule's
	// level, oldest first, or none when the level never changed.
	LevelChanges []LevelChange
	Category     Category
	// Revisions are the contract versions that changed the rule's title,
	// check text or applicability, oldest first; At applies them. Title,
	// the parts' Check, NotApplicableWhen and applies hold the rule's text
	// before the first of them. Read a rule through At.
	Revisions []Revision
	// Title is the rule sentence of spec 0051.
	Title string
	// Source names the specs, ADRs and principles the rule comes from.
	Source string
	// Since is the contract version that added the rule, such as "1.2.0",
	// or "" for a rule of spec 0051's table (contract 1.0.0).
	Since string
	// NotApplicableWhen says when the rule is not applicable, or "" when
	// it always applies.
	NotApplicableWhen string
	Parts             []Part
	// applies decides applicability; nil means the rule always applies.
	// It returns why the rule is not applicable. It sees the repository,
	// not only the configuration: C-03 is not applicable on GitLab CI.
	applies func(env *Env) (bool, string)
}

// Check is the rule's check as spec 0051 states it, every part: each
// part's lead and check, in order, joined by a space (spec 0073).
func (r Rule) Check() string {
	texts := make([]string, len(r.Parts))
	for i, p := range r.Parts {
		texts[i] = p.Lead + p.Check
	}
	return strings.Join(texts, " ")
}

// At is the rule as it reads under the contract version: its level is
// LevelAt's, and its title, the check text of its parts and its
// applicability are those of its last revision that the version is not
// older than. The rule it returns has no level changes and no revisions
// left, so At of another version on it returns it unchanged.
func (r Rule) At(contract string) Rule {
	out := r
	out.Level, out.LevelChanges, out.Revisions = r.LevelAt(contract), nil, nil
	out.Parts = slices.Clone(r.Parts)
	for _, v := range r.Revisions {
		if compareVersions(contract, v.Since) < 0 {
			continue
		}
		out.Title, out.NotApplicableWhen, out.applies = v.Title, v.NotApplicableWhen, v.applies
		for i := range out.Parts {
			out.Parts[i].Check = v.Checks[i]
		}
	}
	return out
}

// LevelAt is the rule's level under the contract version: the level of
// its last change that the version is not older than, or the level it was
// added with.
func (r Rule) LevelAt(contract string) Level {
	level := r.Level
	for _, c := range r.LevelChanges {
		if compareVersions(contract, c.Since) >= 0 {
			level = c.Level
		}
	}
	return level
}

// WaivableAt reports whether a waiver may suspend the rule under the
// contract version: only SHOULD rules can be waived (spec 0051,
// "vibe.yaml").
func (r Rule) WaivableAt(contract string) bool { return r.LevelAt(contract) == Should }

// Rationale is why the rule exists: its sources and, where spec 0051
// introduces the rule's category, that introduction.
func (r Rule) Rationale() string {
	text := "Source: " + r.Source + "."
	if intro := r.Category.intro(); intro != "" {
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

// appliesTo reports whether the rule applies to the audited repository and
// its configuration, and why not when it does not.
func (r Rule) appliesTo(env *Env) (bool, string) {
	if r.applies == nil {
		return true, ""
	}
	return r.applies(env)
}

// onConfig is an applicability that reads only the configuration.
func onConfig(f func(cfg *Config) (bool, string)) func(env *Env) (bool, string) {
	return func(env *Env) (bool, string) { return f(env.config) }
}

// notOnGitLab is C-03's applicability: GitLab CI has no workflow
// permissions to read (spec 0055, Q6).
func notOnGitLab(env *Env) (bool, string) {
	if _, gitlab := env.gitLabCI(); gitlab {
		return false, "the C rules read GitLab CI: GitLab job tokens have no per-file permissions, " +
			"and job-token push access is a project setting the audit does not read"
	}
	return true, ""
}

// The leads the part constructors give, one for each kind. A part whose
// lead differs in spec 0051 sets it with withLead.
const (
	leadStatic    = "Static: "
	leadExecution = "Execution: "
	leadProbe     = "Probe: "
)

// withLead is the part with the lead spec 0051 writes before it.
func (p Part) withLead(lead string) Part {
	p.Lead = lead
	return p
}

func static(check string, run CheckFunc) Part {
	return Part{Kind: Static, Lead: leadStatic, Check: check, Run: run}
}

// executionRun is an execution part this vibe implements.
func executionRun(check string, run CheckFunc) Part {
	return Part{Kind: Execution, Lead: leadExecution, Check: check, Run: run}
}

// executionLast is the last part: an execution part that runs after
// every other execution and probe part and gets their results (spec
// 0071).
func executionLast(check string, run LastFunc) Part {
	return Part{Kind: Execution, Lead: leadExecution, Check: check, RunLast: run}
}

// probeRun is a probe part this vibe implements.
func probeRun(check string, run CheckFunc) Part {
	return Part{Kind: Probe, Lead: leadProbe, Check: check, Run: run}
}

func needsWindows(cfg *Config) (bool, string) {
	if slices.Contains(cfg.platforms, platformWindows) {
		return true, ""
	}
	return false, "platforms does not include windows"
}

func needsClaude(cfg *Config) (bool, string) {
	if slices.Contains(cfg.agents, agentClaude) {
		return true, ""
	}
	return false, "agents does not include claude"
}

func needsAgents(cfg *Config) (bool, string) {
	if len(cfg.agents) > 0 {
		return true, ""
	}
	return false, "agents is empty"
}

const notApplicableWithoutClaude = "when `agents` does not include `claude`"

// testsTheGuard is r, a rule that tests the guard (A-02, A-03, A-04,
// A-08 and A-09), with its 2.1.0 revision: the same title and checks,
// and not applicable when there is no guard to test (spec 0074,
// "Applicability of the guard-testing rules").
func testsTheGuard(r Rule) Rule {
	checks := make([]string, len(r.Parts))
	for i, p := range r.Parts {
		checks[i] = p.Check
	}
	r.Revisions = append(r.Revisions, Revision{
		Since: contract21, Title: r.Title, Checks: checks,
		NotApplicableWhen: notApplicableWithoutClaude + ", or when `require` does not list the guard set and no " +
			"PreToolUse hook in `.claude/settings.json` matches `Bash` (A-01 reports the missing guard)",
		applies: guardToTest,
	})
	return r
}

// guardToTest is the applicability of the rules that test the guard from
// 2.1.0: with `claude` in agents they apply unless require does not list
// the guard set and no PreToolUse hook matches Bash, read as A-01 reads
// it. Then there is no guard to test, and A-01's failure is the one
// finding (spec 0074).
func guardToTest(env *Env) (bool, string) {
	if ok, why := needsClaude(env.config); !ok {
		return false, why
	}
	if env.guardSetBinds() || env.hooksBash() {
		return true, ""
	}
	return false, "no PreToolUse hook in " + claudeSettingsFile + " matches Bash, so there is no guard to test; " +
		"A-01 reports the missing guard"
}

// rules is the rule table, in report order: contract 1.0.0's 16 MUST and
// 9 SHOULD rules, whose sentences, checks and sources are spec 0051's
// verbatim, and the SHOULD rules contract 1.2.0 adds (spec 0061).
var rules = []Rule{
	{
		ID: "V-01", Level: Must, Category: categoryVerification,
		Title:  "`verify` covers formatting, static analysis and tests",
		Source: "spec 0011, principle 2",
		Parts: []Part{
			executionRun("in the probe copy, `verify` exits 0.", checkV01Execution).withLead("Execution and probe: "),
			// The probe part continues the sentence the execution part's
			// lead opens.
			probeRun("With each planted defect in each component, it exits non-zero. The report names each defect "+
				"kind (format, static analysis, test) that did not make it fail.", checkV01Probe).withLead(""),
		},
	},
	{
		ID: "V-02", Level: Must, Category: categoryVerification,
		Title:  "`verify-fast` catches the same defects",
		Source: "spec 0023",
		Parts: []Part{
			probeRun("with each planted defect in each component, `verify-fast` exits non-zero. The contract does not measure speed.", checkV02Probe),
		},
	},
	{
		ID: "V-03", Level: Must, Category: categoryVerification,
		Title:  "`fmt-check` writes no file",
		Source: "spec 0011",
		Parts: []Part{
			executionRun("in the probe copy with the unformatted-file defect planted, after `fmt-check` runs, "+
				"`git status --porcelain` lists only the planted file, with its content unchanged", checkV03),
		},
	},
	{
		ID: "V-04", Level: Must, Category: categoryVerification,
		Title:  "The entrypoints are declared and used",
		Source: "ADR 0025 (replaces ADR 0009's protected task names)",
		Parts: []Part{
			static("`vibe.yaml` declares every required entrypoint, each non-empty. The other rules check that CI and hooks call them.", checkV04),
		},
	},
	{
		ID: "V-05", Level: Should, Category: categoryVerification, Since: "1.2.0",
		Title:             "`verify` catches a type error",
		Source:            "spec 0061, ADR 0027",
		NotApplicableWhen: "when no component is Python",
		Parts: []Part{
			probeRun("with the type-error defect planted in each Python component, `verify` exits non-zero. "+
				"Components in other languages are skipped: there a type error is a compile error, which V-01 shows.", checkV05Probe),
		},
		applies: onConfig(hasPythonComponent),
	},
	{
		ID: "V-06", Level: Should, Category: categoryVerification, Since: "1.2.0",
		Title:  "`verify` catches an over-complex function",
		Source: "spec 0061, ADR 0027",
		Parts: []Part{
			probeRun("with the complexity defect ("+complexityFunction+") "+
				"in each component, `verify` exits non-zero. "+complexityCatches, checkV06Probe),
		},
	},
	{
		ID: "V-07", Level: Should, Category: categoryVerification, Since: "1.2.0",
		Title:  "`verify` catches a secret in the working tree",
		Source: "spec 0061, ADR 0027",
		Parts: []Part{
			probeRun("with the secret defect (a PEM private-key block in a plain text file) planted once, in the "+
				"first component in configuration order that has tests, `verify` exits non-zero.", checkV07Probe),
		},
	},
	{
		ID: "V-08", Level: Should, Category: categoryVerification, Since: "1.2.0",
		Title:  "Blanket suppressions do not hide a defect",
		Source: "spec 0061, ADR 0027",
		Parts: []Part{
			probeRun("with V-01's static-analysis defect under the language's blanket suppressions planted in each "+
				"component, `verify` exits non-zero. A component without tests leaves it undecided.", checkV08Probe),
		},
	},
	{
		ID: "H-01", Level: Should, Category: categoryHooks,
		Title:  "Hook configuration is committed and installs with one command",
		Source: "spec 0011",
		Parts:  []Part{static("a recognized hook manager's configuration is committed", checkH01)},
	},
	{
		ID: "H-02", Level: Should, Category: categoryHooks,
		Title:  "pre-commit checks formatting; pre-push runs verification",
		Source: "spec 0011, ADR 0017",
		Parts: []Part{
			static("the pre-commit hook calls `fmt-check` or `fmt`; the pre-push hook calls `verify-fast` or `verify`", checkH02),
		},
	},
	{
		ID: "C-01", Level: Must, Category: categoryCI,
		Title:  "CI runs `verify` on pull requests and on pushes to the default branch",
		Source: "spec 0010",
		Parts: []Part{
			static("workflows triggered by `pull_request` and by `push` to the default branch call `verify`", checkC01),
		},
	},
	{
		ID: "C-02", Level: Must, Category: categoryCI,
		Title:  "Toolchains and actions are pinned",
		Source: "spec 0044",
		Parts: []Part{
			static("across every workflow (not only generated ones): every `uses:` other than a local path is "+
				"pinned to a full 40-character commit SHA, and no `run:` step contains a command word ending in `@latest`", checkC02).
				withLead("Static, "),
		},
	},
	{
		ID: "C-03", Level: Must, Category: categoryCI,
		Title:             "Workflow permissions default to read",
		Source:            "spec 0010",
		NotApplicableWhen: "when the C rules read GitLab CI",
		Parts: []Part{
			static("every workflow has top-level `permissions` that grant no write scope (`contents: read`, "+
				"`read-all` or `{}`). Jobs may widen them.", checkC03),
		},
		applies: notOnGitLab,
	},
	{
		ID: "C-04", Level: Must, Category: categoryCI,
		Title:  "The audit runs in its own job",
		Source: "spec 0022",
		Parts: []Part{
			static("a job triggered on pull requests runs `vibe audit` (or the setup action followed by it) and does not call `verify`", checkC04),
		},
	},
	{
		ID: "C-05", Level: Should, Category: categoryCI,
		Title:             "Tests run on Linux and Windows",
		Source:            "principle 2",
		NotApplicableWhen: "when `platforms` does not include `windows`",
		Parts: []Part{
			static("the steps that call `verify` run on a Linux runner and on a Windows runner, after matrix "+
				"expansion. Not applicable when `platforms` does not include `windows`.", checkC05),
		},
		applies: onConfig(needsWindows),
		// Under 2.1.0 the rule follows platforms (spec 0074); checkC05
		// reads the audited version.
		Revisions: []Revision{{
			Since: contract21,
			Title: "Tests run on every declared platform",
			Checks: []string{"the steps that call `verify` run on a runner of each platform in `platforms`, " +
				"after matrix expansion."},
		}},
	},
	{
		ID: "A-01", Level: Must, LevelChanges: guardSetLevels, Category: categoryAgents,
		Title:             "Shell commands and file edits pass through the guard",
		Source:            "spec 0021",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("in `.claude/settings.json`, PreToolUse hooks match each of `Bash`, `PowerShell`, `Write`, `Edit` and `MultiEdit`", checkA01),
		},
		applies: onConfig(needsClaude),
	},
	testsTheGuard(Rule{
		ID: "A-02", Level: Must, LevelChanges: guardSetLevels, Category: categoryAgents,
		Title:             "The guard refuses the baseline unless an exception says otherwise",
		Source:            "specs 0012, 0047",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			probeRun("every corpus case is fed to every PreToolUse hook command that matches its tool. A deny case "+
				"is satisfied as described under \"Guard policy and exceptions\". Every allow case must be allowed. "+
				"A hook \"refuses\" by exiting 2.", checkA02),
		},
		applies: onConfig(needsClaude),
	}),
	testsTheGuard(Rule{
		ID: "A-03", Level: Must, LevelChanges: guardSetLevels, Category: categoryAgents,
		Title:             "The guard fails closed",
		Source:            "spec 0021",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			probeRun("every deny case is still refused in two situations: with `PATH` reduced to the operating "+
				"system's directories, and with `.claude/hooks/policy.json` replaced by invalid JSON", checkA03),
		},
		applies: onConfig(needsClaude),
	}),
	testsTheGuard(Rule{
		ID: "A-04", Level: Must, LevelChanges: guardSetLevels, Category: categoryAgents,
		Title:             "The guard is loosened only through listed exceptions",
		Source:            "spec 0050, ADR 0024",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("`.claude/hooks/policy.json` is valid version 2; every exception names a guard rule ID and "+
				"has a non-empty reason; when a PreToolUse hook runs `vibe-guard hook`, vibe-guard can load it; "+
				"the audit lists them. Tightening is not restricted.", checkA04),
		},
		applies: onConfig(needsClaude),
	}),
	{
		ID: "A-05", Level: Must, Category: categoryAgents,
		Title:             "The agent's turn ends with `verify-fast`",
		Source:            "spec 0023",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("a Stop hook calls `verify-fast`.", checkA05),
			probeRun("with a planted defect, the Stop hook exits 2 for a payload with `stop_hook_active: false`, "+
				"and exits 0 for one with `stop_hook_active: true`", checkA05Probe),
		},
		applies: onConfig(needsClaude),
	},
	{
		ID: "A-06", Level: Should, Category: categoryAgents,
		Title:             "Edited files are formatted",
		Source:            "spec 0023",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts:             []Part{static("a PostToolUse hook that matches `Write` and `Edit` calls `fmt`", checkA06)},
		applies:           onConfig(needsClaude),
	},
	{
		ID: "A-07", Level: Should, Category: categoryAgents,
		Title:             "Session start injects facts and changes nothing",
		Source:            "spec 0023",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			static("a SessionStart hook exists.", checkA07),
			// Spec 0052, "Further Notes": A-07's execution part belongs
			// with #102's execution checks.
			executionRun("running it in the probe copy exits 0, and `git status --porcelain` stays empty.", checkA07Execution),
		},
		applies: onConfig(needsClaude),
	},
	testsTheGuard(Rule{
		ID: "A-08", Level: Must, LevelChanges: guardSetLevels, Category: categoryAgents,
		Title:             "An agent changes guardrail settings only after a person unlocks them",
		Source:            "ADR 0025",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			probeRun("a `Write` to `.claude/settings.json` and an `Edit` under `.claude/hooks/` are refused with no "+
				"marker, allowed with a valid marker, and refused with an expired marker. A `Bash` call that runs "+
				"`vibe unlock`, and any `Write` or `Bash` that creates the marker, are refused. After the Stop hook "+
				"runs, the marker is gone.", checkA08),
		},
		applies: onConfig(needsClaude),
	}),
	testsTheGuard(Rule{
		ID: "A-09", Level: Should, Category: categoryAgents, Since: "1.2.0",
		Title:             "The guard refuses commands that skip Git hooks",
		Source:            "spec 0061, ADR 0027",
		NotApplicableWhen: notApplicableWithoutClaude,
		Parts: []Part{
			probeRun("every A-09 corpus case is fed to every PreToolUse hook command that matches its tool. A deny "+
				"case is satisfied as described under \"Guard policy and exceptions\". Every allow case must be allowed.",
				checkA09),
		},
		applies: onConfig(needsClaude),
	}),
	{
		ID: "D-01", Level: Must, Category: categoryDependencies,
		Title:  "Lockfiles are committed and match the declared dependencies",
		Source: "specs 0010, 0020",
		Parts: []Part{
			executionRun(d01CheckText, checkD01).withLead("Execution, "),
		},
	},
	{
		ID: "D-02", Level: Should, Category: categoryDependencies,
		Title:  "An update bot covers dependencies and actions, without automerge",
		Source: "spec 0010",
		Parts: []Part{
			static("`.github/dependabot.yml` or a Renovate configuration covers each component's ecosystem "+
				"and `github-actions`, and enables no automerge", checkD02),
		},
	},
	{
		ID: "D-03", Level: Should, Category: categoryDependencies,
		Title:  "CI scans for known vulnerabilities",
		Source: "spec 0010",
		Parts: []Part{
			static("a workflow runs a recognized scanner: `govulncheck`, `pip-audit`, `pnpm audit`, "+
				"`npm audit`, `osv-scanner` or `trivy`. Adding one is MINOR.", checkD03),
		},
	},
	{
		ID: "R-01", Level: Must, Category: categoryRemovability,
		Title:  "Everything but the audit works without vibe",
		Source: "principle 3, spec 0017",
		Parts: []Part{
			static("no entrypoint, hook command or CI step calls `vibe`, except the audit job of C-04 "+
				"and `vibe --version` in a CI job that calls no entrypoint.", checkR01),
			// R-01 reads A-02's deny verdict and the vibe calls of every
			// other part, so it is the last part.
			executionLast("with `vibe` not on `PATH`, `verify` passes in the probe copy and the A-02 deny cases "+
				"are still refused.", checkR01Execution),
		},
	},
	{
		ID: "R-02", Level: Should, Category: categoryRemovability,
		Title:             "Agents are told the verification entrypoints",
		Source:            "spec 0043",
		NotApplicableWhen: "when `agents` is empty; its `CLAUDE.md` half when `agents` does not include `claude`",
		Parts: []Part{
			static("`AGENTS.md` exists and contains every `verify` command, and `CLAUDE.md` exists and references `AGENTS.md`", checkR02),
		},
		applies: onConfig(needsAgents),
	},
	{
		ID: "R-03", Level: Should, Category: categoryRemovability,
		Title:  "Pull requests report verification",
		Source: "spec 0010",
		Parts: []Part{
			static("a pull request template exists and has a heading containing \"verif\" (case-insensitive)", checkR03),
		},
	},
}

// guardSet is the guard set: the rules that hold the agent's guard (spec
// 0074). Under contract 1.x they are MUST; from 2.1.0 they are SHOULD,
// and require lists all of them or none.
var guardSet = []string{"A-01", "A-02", "A-03", "A-04", "A-08"}

// guardSetLevels is the level history of each rule of the guard set.
var guardSetLevels = []LevelChange{{Since: contract21, Level: Should}}

// guardSetLevel is the level of the guard set's rules under the contract
// version: MUST under 1.x, SHOULD from 2.1.0. It reads their level
// history, not the registry, so that a check can call it.
func guardSetLevel(contract string) Level {
	return Rule{Level: Must, LevelChanges: guardSetLevels}.LevelAt(contract)
}

// GuardSet returns the guard set, A-01, A-02, A-03, A-04 and A-08, in
// that order (spec 0074). Under contract 2.1.0 a configuration requires
// all of it or none of it.
func GuardSet() []string { return slices.Clone(guardSet) }

// Rules returns every rule of contract 1.0.0 in report order. Each rule
// has its own copy of its parts, level changes and revisions.
func Rules() []Rule {
	out := make([]Rule, len(rules))
	for i, r := range rules {
		out[i] = copyRule(r)
	}
	return out
}

// LookupRule returns the rule with the given ID.
func LookupRule(id string) (Rule, bool) {
	i := slices.IndexFunc(rules, func(r Rule) bool { return r.ID == id })
	if i < 0 {
		return Rule{}, false
	}
	return copyRule(rules[i]), true
}

// copyRule is r with its own copy of its parts, level changes and
// revisions.
func copyRule(r Rule) Rule {
	r.Parts = slices.Clone(r.Parts)
	r.LevelChanges = slices.Clone(r.LevelChanges)
	r.Revisions = slices.Clone(r.Revisions)
	for i := range r.Revisions {
		r.Revisions[i].Checks = slices.Clone(r.Revisions[i].Checks)
	}
	return r
}

// RuleIDs returns every rule ID in report order.
func RuleIDs() []string {
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	return ids
}
