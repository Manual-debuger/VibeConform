package contract

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
)

// The static checks of the entrypoints and the repository's own
// documents: V-04, R-02 and R-03.

// checkV04: every required entrypoint is declared and non-empty, and each
// command of every declared entrypoint is one simple command.
func checkV04(env *Env) PartResult {
	names := RequiredEntrypoints
	if _, ok := env.Config.Entrypoints[EntrypointSetup]; ok {
		names = append(names[:len(names):len(names)], EntrypointSetup)
	}
	var problems []Finding
	for _, name := range names {
		problems = append(problems, env.entrypoint(name).problems...)
	}
	if len(problems) > 0 {
		return Fail(problems...)
	}
	return Pass()
}

// checkR02: AGENTS.md contains each verify command as written, and, when
// Claude Code is in use, CLAUDE.md contains the string AGENTS.md. The
// rule is not applicable with no agents; its CLAUDE.md half without
// claude.
func checkR02(env *Env) PartResult {
	halves := []PartResult{agentsMDNamesVerify(env)}
	if slices.Contains(env.Config.Agents, AgentClaude) {
		halves = append(halves, claudeMDPointsToAgentsMD(env))
	}
	return Merge(halves...)
}

func agentsMDNamesVerify(env *Env) PartResult {
	const file = "AGENTS.md"
	if missing := needEntrypoint(env, EntrypointVerify); missing != nil {
		return Fail(*missing)
	}
	verify := env.EntrypointSource(EntrypointVerify)
	data, res, ok := readDocument(env, file, Finding{
		File:    file,
		Message: "is missing, so agents are not told how to verify their work",
		Fix:     "create " + file + " and name the verify commands in it: " + quoteCommands(verify),
	})
	if !ok {
		return res
	}
	var findings []Finding
	for _, cmd := range verify {
		cmd = strings.TrimSpace(cmd)
		if !strings.Contains(string(data), cmd) {
			findings = append(findings, Finding{
				File:    file,
				Message: fmt.Sprintf("does not name the verify command %q", cmd),
				Fix:     fmt.Sprintf("tell agents in %s to run `%s`", file, cmd),
			})
		}
	}
	if len(findings) > 0 {
		return Fail(findings...)
	}
	return Pass()
}

func claudeMDPointsToAgentsMD(env *Env) PartResult {
	const file = "CLAUDE.md"
	data, res, ok := readDocument(env, file, Finding{
		File:    file,
		Message: "is missing, so Claude Code is not pointed to AGENTS.md",
		Fix:     "create " + file + " that references AGENTS.md, such as a line @AGENTS.md",
	})
	if !ok {
		return res
	}
	if !strings.Contains(string(data), "AGENTS.md") {
		return Fail(Finding{
			File:    file,
			Message: "does not reference AGENTS.md",
			Fix:     "add a reference to AGENTS.md, such as a line @AGENTS.md",
		})
	}
	return Pass()
}

// prTemplateHeading is an ATX heading: up to three spaces, one to six #,
// then a space or the end of the line.
var prTemplateHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]|$)`)

// checkR03: every tracked pull request template has an ATX heading
// containing "verif", case-insensitive.
func checkR03(env *Env) PartResult {
	var templates []string
	for _, f := range env.Repo.Files() {
		if isPRTemplate(f) {
			templates = append(templates, f)
		}
	}
	if len(templates) == 0 {
		return Fail(Finding{
			File:    ".github/pull_request_template.md",
			Message: "no pull request template is tracked",
			Fix:     "create .github/pull_request_template.md with a ## Verification heading",
		})
	}
	var findings []Finding
	for _, f := range templates {
		data, err := env.ReadFile(f)
		if err != nil {
			return Undecided(Finding{File: f, Message: fmt.Sprintf("cannot be read: %v", err), Fix: "make the file readable"})
		}
		if !hasVerifHeading(string(data)) {
			findings = append(findings, Finding{
				File:    f,
				Message: "has no heading that asks for verification",
				Fix:     "add a ## Verification heading",
			})
		}
	}
	if len(findings) > 0 {
		return Fail(findings...)
	}
	return Pass()
}

// isPRTemplate recognizes a pull request template: pull_request_template.md
// in the root, .github/ or docs/, or any file under
// .github/PULL_REQUEST_TEMPLATE/, case-insensitive.
func isPRTemplate(file string) bool {
	f := strings.ToLower(file)
	switch f {
	case "pull_request_template.md", ".github/pull_request_template.md", "docs/pull_request_template.md":
		return true
	}
	return strings.HasPrefix(f, ".github/pull_request_template/")
}

func hasVerifHeading(text string) bool {
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\r\n")
		if prTemplateHeading.MatchString(line) && strings.Contains(strings.ToLower(line), "verif") {
			return true
		}
	}
	return false
}

// needEntrypoint returns the V-04 finding for an entrypoint a rule needs,
// or nil when it is usable.
func needEntrypoint(env *Env, name string) *Finding {
	_, missing := env.Entrypoint(name)
	return missing
}

// readDocument reads a tracked file a rule needs. A file that is not
// tracked fails the rule with missing; a file that cannot be read leaves
// it undecided.
func readDocument(env *Env, file string, missing Finding) ([]byte, PartResult, bool) {
	data, err := env.ReadFile(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, Fail(missing), false
	case err != nil:
		return nil, Undecided(Finding{File: file, Message: fmt.Sprintf("cannot be read: %v", err), Fix: "make the file readable"}), false
	}
	return data, Pass(), true
}

func quoteCommands(cmds []string) string {
	quoted := make([]string, len(cmds))
	for i, c := range cmds {
		quoted[i] = "`" + strings.TrimSpace(c) + "`"
	}
	return joinWords(quoted, "and")
}
