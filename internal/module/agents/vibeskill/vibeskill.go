// Package vibeskill provides the vibeconform skill of spec 0041: what an
// adopter's coding agent needs to know to work in a repository
// VibeConform manages, without reading VibeConform's own repository.
// Every agent module that has a skill path writes the same bytes there;
// see docs/specs/0041-vibeconform-skill.md.
package vibeskill

import (
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

const (
	// Name is the skill's name, and the directory each harness keeps it in.
	Name = "vibeconform"
	// MaxWords bounds the skill for every standard and provider (spec
	// 0041 §2).
	MaxWords = 900
)

// Kind is what the skill's standard-specific part depends on: the
// standard, and for prod-mono the CI provider.
type Kind string

// The kinds, one per standard, and per CI provider for prod-mono.
const (
	Go         Kind = "prod-go"
	TS         Kind = "prod-ts"
	Py         Kind = "prod-py"
	MonoGitHub Kind = "prod-mono/github"
	MonoGitLab Kind = "prod-mono/gitlab"
	MonoNone   Kind = "prod-mono/none"
)

// Kinds lists every kind, in a fixed order.
var Kinds = []Kind{Go, TS, Py, MonoGitHub, MonoGitLab, MonoNone}

// KindOf derives the kind from what the context resolves: a standard that
// takes components is prod-mono, and the others have exactly one profile.
// It reports false when the context cannot tell, as for a module resolved
// in isolation.
func KindOf(mctx *module.Context) (Kind, bool) {
	if mctx == nil {
		return "", false
	}
	if len(mctx.Components) > 0 {
		switch module.CIProvider(mctx) {
		case module.CIGitLab:
			return MonoGitLab, true
		case module.CINone:
			return MonoNone, true
		default:
			return MonoGitHub, true
		}
	}
	if len(mctx.Profiles) != 1 {
		return "", false
	}
	switch mctx.Profiles[0] {
	case manifest.ProfileGo:
		return Go, true
	case manifest.ProfileTS:
		return TS, true
	case manifest.ProfilePy:
		return Py, true
	}
	return "", false
}

// The skill is composed from constants rather than embedded templates, so
// a CRLF working copy of a template file cannot change the bytes a build
// ships. {standard} and {ciFiles} are filled in by Content.
const (
	frontMatter = "---\n" +
		"name: " + Name + "\n" +
		"description: How this repository's VibeConform-managed tooling works and how to change it. Use before editing a file VibeConform manages (Taskfile.yml, lefthook.yml, CI configuration, lint or format configuration, agent settings) or vibe.yaml, when adding tasks or Git hooks, when task audit or the conformance check fails, or when asked how this repository's tooling is set up.\n" +
		"---\n"

	generic = "\n" +
		"# VibeConform\n" +
		"\n" +
		"This repository declares a VibeConform standard in `vibe.yaml`.\n" +
		"`vibe sync` generates part of its tooling from that standard and\n" +
		"records what it wrote in `.vibe/state.yaml`. CI checks that the files\n" +
		"still match.\n" +
		"\n" +
		"## Managed files\n" +
		"\n" +
		"- A managed file is generated whole. A managed section is a block\n" +
		"  between VibeConform markers in a file that is otherwise the\n" +
		"  project's, such as `.gitignore`, `AGENTS.md` or an editor's task file.\n" +
		"- `vibe audit` lists every managed file and section. Do not guess the\n" +
		"  list. Run the command.\n" +
		"- Do not edit managed content by hand. The edit is drift: `vibe audit`\n" +
		"  and CI's conformance check fail, and the next `vibe sync` restores\n" +
		"  the file or reports a conflict.\n" +
		"\n" +
		"## Changing what is managed\n" +
		"\n" +
		"1. Edit `vibe.yaml`. Editing it by hand is expected.\n" +
		"2. Run `vibe diff` to preview the change. It writes nothing.\n" +
		"3. Run `vibe sync`. It writes the files and `.vibe/state.yaml`.\n" +
		"4. Commit the changed files and `.vibe/state.yaml` together.\n" +
		"\n" +
		"If the standard cannot express the change, tell the user. Do not edit\n" +
		"a managed file to work around it.\n" +
		"\n" +
		"Changes that belong to the project go where VibeConform manages\n" +
		"nothing:\n" +
		"\n" +
		"- `Taskfile.local.yml`: the project's own tasks, such as build, run or\n" +
		"  deploy. `Taskfile.yml` includes it. A task with the name of a\n" +
		"  generated task is an error, so it cannot redefine `verify`, `lint`,\n" +
		"  `test` or `audit`.\n" +
		"- `lefthook.local.yml`: the project's own Git hooks. `lefthook.yml`\n" +
		"  extends it.\n" +
		"- Anything outside the markers of a managed section.\n" +
		"\n" +
		"## Commands\n" +
		"\n" +
		"- `vibe audit`: a read-only check. Exit 0: conformant. Exit 3: only out\n" +
		"  of date (the standard moved and nothing was edited). Exit 2: not\n" +
		"  conformant (drifted, missing or a conflict). Exit 1: it could not\n" +
		"  answer, for example because this `vibe` is older than the one that\n" +
		"  last synced the repository.\n" +
		"- `vibe diff`: a preview of `vibe sync`. It exits 0 whatever it finds.\n" +
		"- `vibe sync`: the only command that writes. It never overwrites a file\n" +
		"  that was edited by hand. It reports a conflict and exits non-zero.\n" +
		"  Move the edit to a place the project owns, restore the file with\n" +
		"  `git checkout <file>`, and sync again.\n" +
		"- `vibe doctor`: checks whether this machine can run the workflow: the\n" +
		"  tools on `PATH` and the agent hooks. It does not check conformance.\n" +
		"- `task audit`: runs `vibe audit`. Without `vibe` on `PATH`, it\n" +
		"  installs the `vibe_version` that `.vibe/state.yaml` records, so CI\n" +
		"  uses the `vibe` that last synced. To upgrade, sync with a newer\n" +
		"  `vibe` and commit the new `vibe_version`.\n" +
		"\n" +
		"Use `task verify:fast` while you work and `task verify` before you say\n" +
		"the work is done. Both use the language tools only and never need\n" +
		"`vibe`. The `hook:*` tasks are for the agent hooks. Do not copy them.\n" +
		"\n" +
		"## `vibe.yaml`\n" +
		"\n" +
		"Every standard accepts these keys:\n" +
		"\n" +
		"- `integrations:` with `editors` (`vscode`, `zed`), `agents`\n" +
		"  (`claude`, `codex`, both on by default) and `intelligence` (code\n" +
		"  intelligence, off by default). A category that is not given takes\n" +
		"  its defaults. `[]` means none.\n" +
		"- `policy: {line_endings: lf}`: a managed section of `.gitattributes`.\n" +
		"- `development:` with `docs_layout: standard` and `workflow`\n" +
		"  (`direct`, `plan-triggered-sdd` or `always-sdd`). Without\n" +
		"  `workflow`, VibeConform chooses no development process, and that is\n" +
		"  the recommended setting. Each value, `direct` too, is a workflow\n" +
		"  that VibeConform manages.\n" +
		"\n" +
		"VibeConform decodes the file strictly: an unknown key is an error.\n" +
		"\n" +
		"{standard}" +
		"## Removing VibeConform\n" +
		"\n" +
		"Delete these. The other generated files keep working as ordinary\n" +
		"configuration.\n" +
		"\n" +
		"- `vibe.yaml` and `.vibe/`\n" +
		"- `Taskfile.vibe.yml`\n" +
		"{ciFiles}" +
		"- the `vibeconform` skill in `.claude/skills/` and `.agents/skills/`\n"

	goPart = "## Standard: `prod-go/v1`\n" +
		"\n" +
		"- Tasks: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `test:race`,\n" +
		"  `mod:verify`, `security`, `workflows:lint`, `verify`, `verify:fast`,\n" +
		"  `verify-ci` and `audit`.\n" +
		"  They call `go`, `goimports`, `golangci-lint`, `govulncheck` and\n" +
		"  `actionlint`.\n" +
		"- `generated:` is not accepted. golangci-lint skips a file with a\n" +
		"  `// Code generated ... DO NOT EDIT.` header.\n" +
		githubCI +
		"\n"

	tsPart = "## Standard: `prod-ts/v1`\n" +
		"\n" +
		"- Tasks: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `verify`,\n" +
		"  `verify:fast`, `verify-ci` and `audit`.\n" +
		"  They run Prettier, ESLint and `tsc` through `pnpm exec`, and the tests\n" +
		"  through `pnpm test`. Install dependencies with `pnpm install` first.\n" +
		generatedTop +
		githubCI +
		"\n"

	pyPart = "## Standard: `prod-py/v1`\n" +
		"\n" +
		"- Tasks: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `verify`,\n" +
		"  `verify:fast`, `verify-ci` and `audit`.\n" +
		"  They run Ruff, Pyright and pytest through `uv run`.\n" +
		generatedTop +
		githubCI +
		"\n"

	generatedTop = "- `generated:` lists code that a generator writes, relative to the\n" +
		"  repository root. Format and lint checks skip it. Type checks still\n" +
		"  run.\n"

	githubCI = "- CI: `.github/workflows/ci.yml` ends in the `CI / gate` check, and\n" +
		"  `.github/workflows/conformance.yml` runs `task audit` as\n" +
		"  `Conformance / audit`. Both must be required checks.\n"

	monoPart = "## Standard: `prod-mono/v1`\n" +
		"\n" +
		"- `components:` lists each project with an `id`, a `path` and a\n" +
		"  `profile` (`go`, `ts` or `py`). Each component is a complete\n" +
		"  single-language project at its path, with its own `Taskfile.yml`.\n" +
		"- In a component's directory, `task verify` and the other tasks work as\n" +
		"  in a single-language repository. From the root, `task <id>:verify`\n" +
		"  runs one component, and `task verify`, `task verify:fast` and\n" +
		"  `task fmt` run every component.\n" +
		"- A component's `generated:` list names code that a generator writes,\n" +
		"  relative to the component. Format and lint checks skip it. Type\n" +
		"  checks still run. A `go` component does not accept it.\n" +
		"- `ci: {provider: ...}` selects the CI system: `github` (the default),\n" +
		"  `gitlab` or `none`. This repository uses `{provider}`.\n" +
		"{monoCI}" +
		"\n"

	monoGitHub = "- CI: `.github/workflows/ci.yml` runs one job per component and ends\n" +
		"  in the `CI / gate` check. `.github/workflows/conformance.yml` runs\n" +
		"  `task audit` as `Conformance / audit`. Both must be required checks.\n"

	monoGitLab = "- CI: `.gitlab-ci.yml` runs one job per component, and\n" +
		"  `.gitlab-ci.vibe.yml` runs `task audit` as `conformance:audit`. The\n" +
		"  project setting \"Pipelines must succeed\" must be on.\n" +
		"- The project's own jobs go in `.gitlab-ci.local.yml`, which runs as a\n" +
		"  child pipeline. Runner settings go under `default:` in\n" +
		"  `.gitlab-ci.defaults.yml`, and `default:` must be its only top-level\n" +
		"  key. VibeConform never writes either file.\n"

	monoNone = "- CI: none is generated. The project's own CI must run\n" +
		"  `task verify-ci` and `task audit`.\n"
)

// Content returns the skill for kind: the front matter, then the part
// every standard shares, with kind's part before the removal steps.
func Content(kind Kind) string {
	var part, ciFiles string
	switch kind {
	case Go:
		part, ciFiles = goPart, githubFiles
	case TS:
		part, ciFiles = tsPart, githubFiles
	case Py:
		part, ciFiles = pyPart, githubFiles
	case MonoGitHub:
		part, ciFiles = mono(module.CIGitHub, monoGitHub), githubFiles
	case MonoGitLab:
		part, ciFiles = mono(module.CIGitLab, monoGitLab), "- `.gitlab-ci.vibe.yml`\n"
	case MonoNone:
		part = mono(module.CINone, monoNone)
	}
	return frontMatter + strings.NewReplacer("{standard}", part, "{ciFiles}", ciFiles).Replace(generic)
}

// githubFiles names the conformance workflow among the files to delete.
// The required check it runs has to go from branch protection too.
const githubFiles = "- `.github/workflows/conformance.yml`. Also remove `Conformance / audit`\n" +
	"  from the required checks.\n"

// Resources returns the skill at skillsDir/vibeconform/SKILL.md, where
// skillsDir is the directory a harness reads project skills from. It
// returns none when the context cannot tell the standard.
func Resources(skillsDir string, mctx *module.Context) []resource.Resource {
	kind, ok := KindOf(mctx)
	if !ok {
		return nil
	}
	return []resource.Resource{{
		Path:      skillsDir + "/" + Name + "/SKILL.md",
		Ownership: resource.Generated,
		Content:   []byte(Content(kind)),
	}}
}

func mono(provider, ci string) string {
	return strings.NewReplacer("{provider}", provider, "{monoCI}", ci).Replace(monoPart)
}
