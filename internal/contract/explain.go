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
	"V-06": func() string {
		return fmt.Sprintf("Threshold:\n  The sample's cyclomatic complexity is %d and its cognitive complexity %d,\n"+
			"  so a limit of %d or less, of either metric, catches it. A specific\n"+
			"  suppression (one rule at one function, with a reason) is the\n"+
			"  repository's choice; the sample carries none.\n\n",
			complexityBranches+1, complexityBranches, complexityBranches) +
			defectSamplesText([]defectKind{defectComplexity})
	},
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
