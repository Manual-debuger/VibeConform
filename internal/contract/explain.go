package contract

import (
	"fmt"
	"strings"
)

// ruleDetails holds what --explain prints after a rule's check, by rule
// ID: material the rule's check reads that the registry's text does not
// hold, such as V-01's planted defects.
var ruleDetails = map[string]func() string{
	"V-01": defectSamplesText,
	"V-02": defectSamplesText,
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

// defectSamplesText prints every planted defect sample exactly, each
// line indented by four spaces.
func defectSamplesText() string {
	var b strings.Builder
	b.WriteString("Planted defects:\n")
	b.WriteString("  One at a time, each into the first directory, in sorted path order,\n" +
		"  inside a component that holds a test file of its language; the copy is\n" +
		"  reset between defects. In Go samples, {{package}} is the package clause\n" +
		"  of the test file beside it.\n")
	for _, lang := range defectLanguages {
		for _, kind := range defectKinds {
			s := defectSamples[lang.id][kind]
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
