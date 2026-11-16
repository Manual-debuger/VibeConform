package contract

import (
	"fmt"
	"strings"
)

// ruleDetails holds what --explain prints after a rule's check, by rule
// ID: material the rule's check reads that the registry's text does not
// hold, such as V-01's planted defects.
var ruleDetails = map[string]func() string{
	"V-01": func() string { return defectSamplesText("V-01") },
	"V-02": func() string { return defectSamplesText("V-02") },
	"V-05": func() string {
		return defectSamplesText("V-05") +
			"\n  Planted in Python components only, and run with verify only. Any type\n" +
			"  checker that verify runs satisfies the rule, such as Pyright in its\n" +
			"  standard mode or mypy.\n"
	},
	"V-06": func() string {
		return fmt.Sprintf("Threshold:\n  The sample's cyclomatic complexity is %d and its cognitive complexity %d.\n"+
			"  A linter reports a value above its limit, so a limit of %d or less, of\n"+
			"  either metric, catches it. A specific suppression (one rule at one\n"+
			"  function, with a reason) is the repository's choice; the sample carries\n"+
			"  none.\n\n",
			complexityBranches+1, complexityBranches, complexityLimit) +
			defectSamplesText("V-06")
	},
	"V-07": secretSampleText,
	"V-08": v08Details,
	"A-02": guardCorpusText,
	"A-03": guardCorpusText,
	"A-09": a09CasesText,
}

// guardCorpusText prints the guard corpus's counts by guard rule ID, for
// A-02 and A-03 (spec 0053, "Report and CLI").
func guardCorpusText() string {
	byRule, deny, allow := guardCorpusCounts()
	var b strings.Builder
	fmt.Fprintf(&b, "Guard corpus:\n  %d cases: %d deny and %d allow. Deny cases by guard rule ID:\n", deny+allow, deny, allow)
	for _, c := range byRule {
		fmt.Fprintf(&b, "  %-26s %3d\n", c.Rule, c.Deny)
	}
	return b.String()
}

// a09CasesText lists A-09's corpus cases: decision, tool and command.
func a09CasesText() string {
	var b strings.Builder
	b.WriteString("A-09 corpus cases:\n" +
		"  Its own cases, which A-02, A-03 and R-01 do not read. A deny case names\n" +
		"  the guard rule ID git-skip-hooks, which an exception may name.\n")
	for _, c := range corpusFor(ProbeA09) {
		fmt.Fprintf(&b, "  %-5s  %-10s  %s\n", c.Want, c.Payload.ToolName, c.Target().Command)
	}
	b.WriteString("  vibe-guard also refuses LEFTHOOK=0 and HUSKY=0 before git commit or git\n" +
		"  push; they are specific to one hook manager, so A-09 does not require it.\n")
	return b.String()
}

// v08Details lists V-08's suppressions per language before its samples.
func v08Details() string {
	return "Blanket suppressions:\n" +
		"  V-01's static-analysis defect, with the language's common blanket\n" +
		"  suppressions on the defect's line or for its file. Specific\n" +
		"  suppressions, which name a rule, are not part of any sample.\n" +
		"    Go: //nolint, naming no linter.\n" +
		"    Python: # type: ignore  # noqa, with no codes.\n" +
		"    TypeScript: a file-level eslint-disable comment naming no rule, and\n" +
		"    // @ts-ignore on the line before the defect.\n" +
		"  Any gate in verify that still reports the defect satisfies the rule:\n" +
		"  golangci-lint's nolintlint (require-specific), Ruff's PGH004, ESLint's\n" +
		"  eslint-comments/no-unlimited-disable, or a checker that ignores the\n" +
		"  comments (go vet ignores //nolint).\n\n" +
		defectSamplesText("V-08")
}

// secretSampleText is V-07's planted defect: one sample for every
// language, planted once per repository.
func secretSampleText() string {
	var b strings.Builder
	b.WriteString("Planted defect:\n")
	b.WriteString("  Once per repository, in the first component in configuration order that\n" +
		"  has tests: into its first directory, in sorted path order, that holds a\n" +
		"  test file of its language. The file is written to the probe copy's\n" +
		"  working tree and is not committed, so the scanner must read the working\n" +
		"  tree's files, not the Git history. Any secret scanner that reports it\n" +
		"  satisfies the rule.\n")
	fmt.Fprintf(&b, "\n  Every language, %s: %s\n", defectSecret, secretSample.Name)
	writeIndented(&b, secretSample.Content)
	return b.String()
}

// writeIndented writes content with each line indented by four spaces,
// and blank lines kept blank.
func writeIndented(b *strings.Builder, content string) {
	for _, line := range strings.SplitAfter(content, "\n") {
		switch line {
		case "":
		case "\n":
			b.WriteString("\n")
		default:
			b.WriteString("    " + line)
		}
	}
}

// RuleDetails is what --explain prints after the check of the rule with
// this ID, or "" when there is nothing more.
func RuleDetails(id string) string {
	if f, ok := ruleDetails[id]; ok {
		return f()
	}
	return ""
}

// defectLanguages is the order --explain lists the samples in.
var defectLanguages = []struct{ id, name string }{
	{LanguageGo, "Go"},
	{LanguagePython, "Python"},
	{LanguageTypeScript, "TypeScript"},
}

// defectSamplesText prints the planted defect samples of a rule's kinds
// (ruleDefectKinds) exactly, in every language that has one, each line
// indented by four spaces.
func defectSamplesText(rule string) string {
	kinds := ruleDefectKinds[rule]
	var b strings.Builder
	b.WriteString("Planted defects:\n")
	b.WriteString("  One at a time, each into the first directory, in sorted path order,\n" +
		"  inside a component that holds a test file of its language; the copy is\n" +
		"  reset between defects. In Go samples, {{package}} is the package clause\n" +
		"  of the test file beside it, without a _test suffix in a file that is not\n" +
		"  a _test.go file.\n")
	for _, lang := range defectLanguages {
		for _, kind := range kinds {
			s, ok := defectSamples[lang.id][kind]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "\n  %s, %s: %s\n", lang.name, kind, s.Name)
			writeIndented(&b, s.Content)
		}
	}
	return b.String()
}
