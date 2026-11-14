package contract

import (
	"fmt"
	"strings"
)

// ruleDetails holds what --explain prints after a rule's check, by rule
// ID: material the rule's check reads that the registry's text does not
// hold, such as V-01's planted defects.
var ruleDetails = map[string]func() string{
	"V-01": func() string { return defectSamplesText(v01Kinds) },
	"V-02": func() string { return defectSamplesText(v01Kinds) },
	"V-08": v08Details,
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
		defectSamplesText([]defectKind{defectBlanketSuppression})
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

// defectSamplesText prints the planted defect samples of these kinds
// exactly, in every language that has one, each line indented by four
// spaces.
func defectSamplesText(kinds []defectKind) string {
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
			for _, line := range strings.SplitAfter(s.Content, "\n") {
				switch line {
				case "":
				case "\n":
					b.WriteString("\n")
				default:
					b.WriteString("    " + line)
				}
			}
		}
	}
	return b.String()
}
