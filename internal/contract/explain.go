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
	"V-05": func() string {
		return defectSamplesText([]defectKind{defectTypeError}) +
			"\n  Planted in Python components only, and run with verify only. Any type\n" +
			"  checker that verify runs satisfies the rule, such as Pyright in its\n" +
			"  standard mode or mypy.\n"
	},
	"A-09": a09CasesText,
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
