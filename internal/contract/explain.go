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
	"V-07": secretSampleText,
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
			writeIndented(&b, s.Content)
		}
	}
	return b.String()
}
