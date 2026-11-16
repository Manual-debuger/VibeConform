package contract_test

import (
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// R-02: AGENTS.md names every verify command, and CLAUDE.md points to it
// when Claude Code is in use.
func TestRuleR02(t *testing.T) {
	t.Parallel()
	contracttest.RunRuleCases(t, "R-02", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "list verify, every command named", Want: "pass", Edits: []contracttest.Edit{
			contracttest.Entrypoints("  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"),
			contracttest.WithFile("AGENTS.md", "Run `task lint` and `task test`.\n")}},
		{Name: "list verify, one command missing", Want: "fail", Edits: []contracttest.Edit{
			contracttest.Entrypoints("  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"),
			contracttest.WithFile("AGENTS.md", "Run `task lint`.\n")},
			Findings: []string{"AGENTS.md: ", `"task test"`}},
		{Name: "AGENTS.md lacks the command", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("AGENTS.md", "Run the checks before you finish.\n")},
			Findings: []string{"AGENTS.md: ", `"task verify"`}},
		{Name: "AGENTS.md missing", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile("AGENTS.md")},
			Findings: []string{"AGENTS.md: ", "create"}},
		{Name: "AGENTS.md untracked", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile("AGENTS.md"), contracttest.WithUntracked("AGENTS.md", "Run `task verify`.\n")},
			Findings: []string{"AGENTS.md: "}},
		{Name: "CLAUDE.md missing", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile("CLAUDE.md")},
			Findings: []string{"CLAUDE.md: "}},
		{Name: "CLAUDE.md does not reference AGENTS.md", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("CLAUDE.md", "Be careful.\n")},
			Findings: []string{"CLAUDE.md: ", "AGENTS.md"}},
		{Name: "CLAUDE.md half not applicable without claude", Want: "pass",
			Edits: []contracttest.Edit{contracttest.AgentsConfig("[codex]"), contracttest.WithoutFile("CLAUDE.md")}},
		{Name: "not applicable without agents", Want: "not_applicable",
			Edits: []contracttest.Edit{contracttest.AgentsConfig("[]"), contracttest.WithoutFile("AGENTS.md"), contracttest.WithoutFile("CLAUDE.md")}},
		{Name: "verify missing points at V-04", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")},
			Findings: []string{"vibe.yaml: ", "V-04"}},
	})
}

// R-03: every pull request template has a heading containing "verif".
func TestRuleR03(t *testing.T) {
	t.Parallel()
	const tmpl = ".github/pull_request_template.md"
	contracttest.RunRuleCases(t, "R-03", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "root template, upper case", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithoutFile(tmpl), contracttest.WithFile("PULL_REQUEST_TEMPLATE.md", "# How this was verified\n")}},
		{Name: "docs template", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithoutFile(tmpl), contracttest.WithFile("docs/pull_request_template.md", "### VERIFICATION\n")}},
		{Name: "template directory", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithoutFile(tmpl), contracttest.WithFile(".github/PULL_REQUEST_TEMPLATE/feature.md", "## Verified by\n")}},
		// Regression for pilot finding F-13: a GitLab merge request
		// template was not recognized (spec 0055, R-03 (Q11)).
		{Name: "GitLab merge request template only", Want: "pass",
			Edits: []contracttest.Edit{contracttest.WithoutFile(tmpl), contracttest.WithFile(".gitlab/merge_request_templates/Default.md", "## Verification\n")}},
		{Name: "GitLab merge request template without the heading", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".gitlab/merge_request_templates/Default.md", "## Summary\n")},
			Findings: []string{".gitlab/merge_request_templates/Default.md: "}},
		{Name: "a .md in a subdirectory of the GitLab templates is not a template", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile(tmpl), contracttest.WithFile(".gitlab/merge_request_templates/old/Default.md", "## Verification\n")},
			Findings: []string{tmpl + ": ", "no pull request template is tracked"}},
		{Name: "no template", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithoutFile(tmpl)},
			Findings: []string{tmpl + ": ", "Verification"}},
		{Name: "untracked template", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithoutFile(tmpl), contracttest.WithUntracked(tmpl, "## Verification\n")}},
		{Name: "no heading with verif", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(tmpl, "## Summary\n\nVerification: describe it.\n")},
			Findings: []string{tmpl + ": "}},
		{Name: "a setext heading is not an ATX heading", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithFile(tmpl, "Verification\n------------\n")}},
		{Name: "a hash without a space is not a heading", Want: "fail",
			Edits: []contracttest.Edit{contracttest.WithFile(tmpl, "#Verification\n")}},
		{Name: "one of two templates lacks it", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile(".github/PULL_REQUEST_TEMPLATE/bug.md", "## Steps\n")},
			Findings: []string{".github/PULL_REQUEST_TEMPLATE/bug.md: "}},
	})
}

// V-04: the entrypoints are declared, non-empty, and each command is one
// simple command (spec 0052, "Entrypoints").
func TestRuleV04(t *testing.T) {
	t.Parallel()
	const others = "  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"
	contracttest.RunRuleCases(t, "V-04", []contracttest.RuleCase{
		{Name: "baseline", Want: "pass"},
		{Name: "list entrypoint", Want: "pass",
			Edits: []contracttest.Edit{contracttest.Entrypoints("  verify: [task lint, task test]\n" + others)}},
		{Name: "leading assignment", Want: "pass",
			Edits: []contracttest.Edit{contracttest.Entrypoints("  verify: CGO_ENABLED=0 go test ./...\n" + others)}},
		{Name: "setup is optional and checked", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  setup: pnpm i && pnpm build\n" + others)},
			Findings: []string{"vibe.yaml: entrypoints.setup", "&&", "split"}},
		{Name: "missing", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n")},
			Findings: []string{"vibe.yaml: entrypoints.fmt is not declared"}},
		{Name: "no entrypoints at all", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: ""}))},
			Findings: []string{"entrypoints.verify is not declared", "entrypoints.fmt-check is not declared"}},
		{Name: "null", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify:\n" + others)},
			Findings: []string{"entrypoints.verify is empty"}},
		{Name: "empty list", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: []\n" + others)},
			Findings: []string{"entrypoints.verify is empty"}},
		{Name: "empty string", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: \"\"\n" + others)},
			Findings: []string{"entrypoints.verify", "is empty"}},
		{Name: "list item empty", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: [task lint, \" \"]\n" + others)},
			Findings: []string{"entrypoints.verify[1]", "is empty"}},
		{Name: "a list joined by &&", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task lint && task test\n" + others)},
			Findings: []string{"entrypoints.verify", "not one simple command", "split"}},
		{Name: "a pipeline", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task test | tee log\n" + others)},
			Findings: []string{"pipeline"}},
		{Name: "a redirection", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task test > log\n" + others)},
			Findings: []string{"redirection"}},
		{Name: "does not parse", Want: "fail",
			Edits:    []contracttest.Edit{contracttest.Entrypoints("  verify: task 'test\n" + others)},
			Findings: []string{"does not parse as POSIX shell"}},
	})
}
