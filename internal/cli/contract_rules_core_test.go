package cli

import "testing"

// ruleCase is one fixture for one rule: the baseline plus one change,
// and the result and findings that change must give.
type ruleCase struct {
	name     string
	edits    []fixtureEdit
	want     string
	findings []string
}

func runRuleCases(t *testing.T, id string, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep, _ := auditJSON(t, contractFixture(t, tc.edits...), "--static")
			assertRule(t, rep, id, tc.want, tc.findings...)
		})
	}
}

func agentsConfig(list string) fixtureEdit {
	return withFile("vibe.yaml", configWith(section{"agents", "agents: " + list + "\n"}))
}

// R-02: AGENTS.md names every verify command, and CLAUDE.md points to it
// when Claude Code is in use.
func TestRuleR02(t *testing.T) {
	runRuleCases(t, "R-02", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "list verify, every command named", want: "pass", edits: []fixtureEdit{
			entrypoints("  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"),
			withFile("AGENTS.md", "Run `task lint` and `task test`.\n")}},
		{name: "list verify, one command missing", want: "fail", edits: []fixtureEdit{
			entrypoints("  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"),
			withFile("AGENTS.md", "Run `task lint`.\n")},
			findings: []string{"AGENTS.md: ", `"task test"`}},
		{name: "AGENTS.md lacks the command", want: "fail",
			edits:    []fixtureEdit{withFile("AGENTS.md", "Run the checks before you finish.\n")},
			findings: []string{"AGENTS.md: ", `"task verify"`}},
		{name: "AGENTS.md missing", want: "fail",
			edits:    []fixtureEdit{withoutFile("AGENTS.md")},
			findings: []string{"AGENTS.md: ", "create"}},
		{name: "AGENTS.md untracked", want: "fail",
			edits:    []fixtureEdit{withoutFile("AGENTS.md"), withUntracked("AGENTS.md", "Run `task verify`.\n")},
			findings: []string{"AGENTS.md: "}},
		{name: "CLAUDE.md missing", want: "fail",
			edits:    []fixtureEdit{withoutFile("CLAUDE.md")},
			findings: []string{"CLAUDE.md: "}},
		{name: "CLAUDE.md does not reference AGENTS.md", want: "fail",
			edits:    []fixtureEdit{withFile("CLAUDE.md", "Be careful.\n")},
			findings: []string{"CLAUDE.md: ", "AGENTS.md"}},
		{name: "CLAUDE.md half not applicable without claude", want: "pass",
			edits: []fixtureEdit{agentsConfig("[codex]"), withoutFile("CLAUDE.md")}},
		{name: "not applicable without agents", want: "not_applicable",
			edits: []fixtureEdit{agentsConfig("[]"), withoutFile("AGENTS.md"), withoutFile("CLAUDE.md")}},
		{name: "verify missing points at V-04", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")},
			findings: []string{"vibe.yaml: ", "V-04"}},
	})
}

// R-03: every pull request template has a heading containing "verif".
func TestRuleR03(t *testing.T) {
	const tmpl = ".github/pull_request_template.md"
	runRuleCases(t, "R-03", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "root template, upper case", want: "pass",
			edits: []fixtureEdit{withoutFile(tmpl), withFile("PULL_REQUEST_TEMPLATE.md", "# How this was verified\n")}},
		{name: "docs template", want: "pass",
			edits: []fixtureEdit{withoutFile(tmpl), withFile("docs/pull_request_template.md", "### VERIFICATION\n")}},
		{name: "template directory", want: "pass",
			edits: []fixtureEdit{withoutFile(tmpl), withFile(".github/PULL_REQUEST_TEMPLATE/feature.md", "## Verified by\n")}},
		{name: "no template", want: "fail",
			edits:    []fixtureEdit{withoutFile(tmpl)},
			findings: []string{tmpl + ": ", "Verification"}},
		{name: "untracked template", want: "fail",
			edits: []fixtureEdit{withoutFile(tmpl), withUntracked(tmpl, "## Verification\n")}},
		{name: "no heading with verif", want: "fail",
			edits:    []fixtureEdit{withFile(tmpl, "## Summary\n\nVerification: describe it.\n")},
			findings: []string{tmpl + ": "}},
		{name: "a setext heading is not an ATX heading", want: "fail",
			edits: []fixtureEdit{withFile(tmpl, "Verification\n------------\n")}},
		{name: "a hash without a space is not a heading", want: "fail",
			edits: []fixtureEdit{withFile(tmpl, "#Verification\n")}},
		{name: "one of two templates lacks it", want: "fail",
			edits:    []fixtureEdit{withFile(".github/PULL_REQUEST_TEMPLATE/bug.md", "## Steps\n")},
			findings: []string{".github/PULL_REQUEST_TEMPLATE/bug.md: "}},
	})
}

func entrypoints(body string) fixtureEdit {
	return withFile("vibe.yaml", configWith(section{"entrypoints", "entrypoints:\n" + body}))
}

// V-04: the entrypoints are declared, non-empty, and each command is one
// simple command (spec 0052, "Entrypoints").
func TestRuleV04(t *testing.T) {
	const others = "  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"
	runRuleCases(t, "V-04", []ruleCase{
		{name: "baseline", want: "pass"},
		{name: "list entrypoint", want: "pass",
			edits: []fixtureEdit{entrypoints("  verify: [task lint, task test]\n" + others)}},
		{name: "leading assignment", want: "pass",
			edits: []fixtureEdit{entrypoints("  verify: CGO_ENABLED=0 go test ./...\n" + others)}},
		{name: "setup is optional and checked", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task verify\n  setup: pnpm i && pnpm build\n" + others)},
			findings: []string{"vibe.yaml: entrypoints.setup", "&&", "split"}},
		{name: "missing", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n")},
			findings: []string{"vibe.yaml: entrypoints.fmt is not declared"}},
		{name: "no entrypoints at all", want: "fail",
			edits:    []fixtureEdit{withFile("vibe.yaml", configWith(section{"entrypoints", ""}))},
			findings: []string{"entrypoints.verify is not declared", "entrypoints.fmt-check is not declared"}},
		{name: "null", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify:\n" + others)},
			findings: []string{"entrypoints.verify is empty"}},
		{name: "empty list", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: []\n" + others)},
			findings: []string{"entrypoints.verify is empty"}},
		{name: "empty string", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: \"\"\n" + others)},
			findings: []string{"entrypoints.verify", "is empty"}},
		{name: "list item empty", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: [task lint, \" \"]\n" + others)},
			findings: []string{"entrypoints.verify[1]", "is empty"}},
		{name: "a list joined by &&", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task lint && task test\n" + others)},
			findings: []string{"entrypoints.verify", "not one simple command", "split"}},
		{name: "a pipeline", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task test | tee log\n" + others)},
			findings: []string{"pipeline"}},
		{name: "a redirection", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task test > log\n" + others)},
			findings: []string{"redirection"}},
		{name: "does not parse", want: "fail",
			edits:    []fixtureEdit{entrypoints("  verify: task 'test\n" + others)},
			findings: []string{"does not parse as POSIX shell"}},
	})
}
