package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// explainWidth is the column --explain wraps its long text at.
const explainWidth = 76

// writeExplain prints one rule as the registry holds it (spec 0052,
// "Package and command"): the facts first, then the rationale and the
// check as spec 0051 states it. The rule is the one under the contract
// version: its title, level, check, applicability and whether it is
// waivable (spec 0074). source says where the version comes from.
func writeExplain(w io.Writer, rule contract.Rule, version, source string) error {
	rule = rule.At(version)
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n\n", rule.ID, rule.Title)

	kinds := make([]string, 0, len(rule.Parts))
	for _, k := range rule.Kinds() {
		kinds = append(kinds, string(k))
	}
	notApplicable := rule.NotApplicableWhen
	if notApplicable == "" {
		notApplicable = "never; the rule always applies"
	}
	waivable := "no, a MUST rule cannot be waived"
	if rule.Level == contract.Should {
		waivable = "yes, through a waiver in vibe.yaml with a reason and an until date"
	}
	for _, field := range [][2]string{
		{"Contract", source},
		{"Level", string(rule.Level)},
		{"Category", string(rule.Category) + ", " + rule.Category.Name()},
		{"Checks", strings.Join(kinds, ", ")},
		{"Source", rule.Source},
		{"Not applicable", notApplicable},
		{"Waivable", waivable},
	} {
		label := field[0] + ":"
		writeWrapped(&b, fmt.Sprintf("  %-17s", label), strings.Repeat(" ", 19), field[1])
	}

	b.WriteString("\nRationale:\n")
	writeWrapped(&b, "  ", "  ", rule.Rationale())
	b.WriteString("\nCheck:\n")
	writeWrapped(&b, "  ", "  ", rule.Check())
	if details := contract.RuleDetails(rule.ID); details != "" {
		b.WriteString("\n" + details)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// writeWrapped writes text word-wrapped at explainWidth: the first line
// after first, the others after rest. A word longer than the line stays
// whole.
func writeWrapped(b *strings.Builder, first, rest, text string) {
	line := first
	empty := true
	for _, word := range strings.Fields(text) {
		if !empty && len(line)+1+len(word) > explainWidth {
			b.WriteString(line + "\n")
			line, empty = rest, true
		}
		if !empty {
			line += " "
		}
		line += word
		empty = false
	}
	b.WriteString(line + "\n")
}
