package vibeskill

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

// The skill, byte for byte (spec 0041 §2): wantHead, then the standard's
// part, then wantRemoval with the CI files to delete.
const (
	wantHead = "---\n" +
		"name: vibeconform\n" +
		"description: How this repository's VibeConform-managed tooling works and how to change it. Use before editing a file VibeConform manages (Taskfile.yml, lefthook.yml, CI configuration, lint or format configuration, agent settings) or vibe.yaml, when adding tasks or Git hooks, when task audit or the conformance check fails, or when asked how this repository's tooling is set up.\n" +
		"---\n" +
		"\n" +
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
		"- `development:` with `workflow` (`direct`, `plan-triggered-sdd` or\n" +
		"  `always-sdd`) and `docs_layout: standard`.\n" +
		"\n" +
		"VibeConform decodes the file strictly: an unknown key is an error.\n" +
		"\n"

	wantRemovalHead = "## Removing VibeConform\n" +
		"\n" +
		"Delete these. The other generated files keep working as ordinary\n" +
		"configuration.\n" +
		"\n" +
		"- `vibe.yaml` and `.vibe/`\n" +
		"- `Taskfile.vibe.yml`\n"
	wantRemovalTail = "- the `vibeconform` skill in `.claude/skills/` and `.agents/skills/`\n"

	wantGitHubFiles = "- `.github/workflows/conformance.yml`. Also remove `Conformance / audit`\n" +
		"  from the required checks.\n"
	wantGitLabFiles = "- `.gitlab-ci.vibe.yml`\n"

	wantGitHubCI = "- CI: `.github/workflows/ci.yml` ends in the `CI / gate` check, and\n" +
		"  `.github/workflows/conformance.yml` runs `task audit` as\n" +
		"  `Conformance / audit`. Both must be required checks.\n"

	wantGeneratedTop = "- `generated:` lists code that a generator writes, relative to the\n" +
		"  repository root. Format and lint checks skip it. Type checks still\n" +
		"  run.\n"

	wantGo = "## Standard: `prod-go/v1`\n" +
		"\n" +
		"- Tasks: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `test:race`,\n" +
		"  `mod:verify`, `security`, `workflows:lint`, `verify`, `verify:fast`,\n" +
		"  `verify-ci` and `audit`.\n" +
		"  They call `go`, `goimports`, `golangci-lint`, `govulncheck` and\n" +
		"  `actionlint`.\n" +
		"- `generated:` is not accepted. golangci-lint skips a file with a\n" +
		"  `// Code generated ... DO NOT EDIT.` header.\n" +
		wantGitHubCI +
		"\n"

	wantTS = "## Standard: `prod-ts/v1`\n" +
		"\n" +
		"- Tasks: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `verify`,\n" +
		"  `verify:fast`, `verify-ci` and `audit`.\n" +
		"  They run Prettier, ESLint and `tsc` through `pnpm exec`, and the tests\n" +
		"  through `pnpm test`. Install dependencies with `pnpm install` first.\n" +
		wantGeneratedTop +
		wantGitHubCI +
		"\n"

	wantPy = "## Standard: `prod-py/v1`\n" +
		"\n" +
		"- Tasks: `fmt`, `fmt:check`, `lint`, `typecheck`, `test`, `verify`,\n" +
		"  `verify:fast`, `verify-ci` and `audit`.\n" +
		"  They run Ruff, Pyright and pytest through `uv run`.\n" +
		wantGeneratedTop +
		wantGitHubCI +
		"\n"

	wantMonoHead = "## Standard: `prod-mono/v1`\n" +
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
		"  `gitlab` or `none`. This repository uses "

	wantMonoGitHub = wantMonoHead + "`github`.\n" +
		"- CI: `.github/workflows/ci.yml` runs one job per component and ends\n" +
		"  in the `CI / gate` check. `.github/workflows/conformance.yml` runs\n" +
		"  `task audit` as `Conformance / audit`. Both must be required checks.\n" +
		"\n"

	wantMonoGitLab = wantMonoHead + "`gitlab`.\n" +
		"- CI: `.gitlab-ci.yml` runs one job per component, and\n" +
		"  `.gitlab-ci.vibe.yml` runs `task audit` as `conformance:audit`. The\n" +
		"  project setting \"Pipelines must succeed\" must be on.\n" +
		"- The project's own jobs go in `.gitlab-ci.local.yml`, which runs as a\n" +
		"  child pipeline. Runner settings go under `default:` in\n" +
		"  `.gitlab-ci.defaults.yml`, and `default:` must be its only top-level\n" +
		"  key. VibeConform never writes either file.\n" +
		"\n"

	wantMonoNone = wantMonoHead + "`none`.\n" +
		"- CI: none is generated. The project's own CI must run\n" +
		"  `task verify-ci` and `task audit`.\n" +
		"\n"
)

func want(part, ciFiles string) string {
	return wantHead + part + wantRemovalHead + ciFiles + wantRemovalTail
}

// TestContent pins every kind byte for byte, and the word cap.
func TestContent(t *testing.T) {
	cases := map[Kind]string{
		Go:         want(wantGo, wantGitHubFiles),
		TS:         want(wantTS, wantGitHubFiles),
		Py:         want(wantPy, wantGitHubFiles),
		MonoGitHub: want(wantMonoGitHub, wantGitHubFiles),
		MonoGitLab: want(wantMonoGitLab, wantGitLabFiles),
		MonoNone:   want(wantMonoNone, ""),
	}
	if len(cases) != len(Kinds) {
		t.Fatalf("%d cases for %d kinds", len(cases), len(Kinds))
	}
	for _, k := range Kinds {
		got := Content(k)
		if got != cases[k] {
			t.Errorf("%s: content differs\n got: %q\nwant: %q", k, got, cases[k])
		}
		if n := len(strings.Fields(got)); n > MaxWords {
			t.Errorf("%s: %d words, cap %d", k, n, MaxWords)
		}
	}
}

func TestKindOf(t *testing.T) {
	comps := []manifest.Component{{ID: "api", Path: "services/api", Profile: manifest.ProfileGo}}
	cases := []struct {
		name string
		mctx *module.Context
		want Kind
		ok   bool
	}{
		{"nil", nil, "", false},
		{"no profile", &module.Context{}, "", false},
		{"go", &module.Context{Profiles: []manifest.Profile{manifest.ProfileGo}}, Go, true},
		{"ts", &module.Context{Profiles: []manifest.Profile{manifest.ProfileTS}}, TS, true},
		{"py", &module.Context{Profiles: []manifest.Profile{manifest.ProfilePy}}, Py, true},
		{"mono default", &module.Context{Components: comps, Profiles: []manifest.Profile{manifest.ProfileGo}}, MonoGitHub, true},
		{"mono github", &module.Context{Components: comps, Policies: map[string]string{manifest.CIProvider: module.CIGitHub}}, MonoGitHub, true},
		{"mono gitlab", &module.Context{Components: comps, Policies: map[string]string{manifest.CIProvider: module.CIGitLab}}, MonoGitLab, true},
		{"mono none", &module.Context{Components: comps, Policies: map[string]string{manifest.CIProvider: module.CINone}}, MonoNone, true},
	}
	for _, c := range cases {
		if got, ok := KindOf(c.mctx); got != c.want || ok != c.ok {
			t.Errorf("%s: KindOf = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestResources: the skill sits in the harness's skills directory, and
// is absent when the context cannot tell the standard.
func TestResources(t *testing.T) {
	rs := Resources(".claude/skills", &module.Context{Profiles: []manifest.Profile{manifest.ProfileTS}})
	if len(rs) != 1 || rs[0].Path != ".claude/skills/vibeconform/SKILL.md" || string(rs[0].Content) != Content(TS) {
		t.Errorf("Resources = %+v", rs)
	}
	if rs := Resources(".claude/skills", nil); len(rs) != 0 {
		t.Errorf("nil context: %+v", rs)
	}
}
