package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// The text report (spec 0052, "Report (#100)"): a header, one summary
// line per category, then the sections Failed, Waived, Not applicable,
// Cannot decide, Not run (--static only), Guard exceptions and Local
// guardrail adjustments, any notes, and the verdict line. A section with
// nothing in it prints "(none)".

// writeContractText prints the report as text.
func writeContractText(w io.Writer, rep *contract.Report, vibeVersion string) error {
	var b strings.Builder
	writeTextHeader(&b, rep, vibeVersion)
	writeCategorySummary(&b, rep)

	failed := rulesWith(rep, contract.ResultFail)
	slices.SortStableFunc(failed, func(x, y contract.RuleResult) int {
		return levelRank(x.Rule.Level) - levelRank(y.Rule.Level)
	})
	writeSection(&b, "Failed", failed, func(rr contract.RuleResult) {
		writeRuleBlock(&b, rr, contract.ResultFail)
	})
	writeSection(&b, "Waived", rulesWithWaiver(rep), func(rr contract.RuleResult) {
		fmt.Fprintf(&b, "%s%s\n", ruleLead(rr.Rule), waiverText(rr))
	})
	writeSection(&b, "Not applicable", rulesWith(rep, contract.ResultNotApplicable), func(rr contract.RuleResult) {
		fmt.Fprintf(&b, "%s%s\n", ruleLead(rr.Rule), rr.NotApplicable)
	})
	writeSection(&b, "Cannot decide", rulesWith(rep, contract.ResultUndecided), func(rr contract.RuleResult) {
		writeRuleBlock(&b, rr, contract.ResultUndecided)
	})
	if rep.Static {
		writeNotRun(&b, rulesWith(rep, contract.ResultNotRun))
	}
	writeSection(&b, "Guard exceptions", rep.GuardExceptions, func(g contract.GuardException) {
		fmt.Fprintf(&b, "  %s \"%s\"%s\n", g.Rule, g.Match, reasonSuffix(g.Reason))
	})
	writeSection(&b, "Local guardrail adjustments", rep.LocalAdjustments, func(l contract.LocalAdjustment) {
		kind := l.Kind
		if kind == "rule" {
			kind = "extra rule"
		}
		fmt.Fprintf(&b, "  %s: %s %s \"%s\"%s\n", l.File, kind, l.Rule, l.Pattern, reasonSuffix(l.Reason))
	})

	if len(rep.Notes) > 0 {
		b.WriteString("\n")
		for _, n := range rep.Notes {
			fmt.Fprintf(&b, "Note: %s\n", n)
		}
	}
	b.WriteString("\n")
	switch rep.Verdict {
	case contract.VerdictConformant:
		b.WriteString("Verdict: conformant\n")
	case contract.VerdictNotConformant:
		b.WriteString("Verdict: not conformant\n")
	default:
		fmt.Fprintf(&b, "Verdict: cannot decide. %s.\n", rep.Reason)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeTextHeader names the contract audited, the version declared when
// it is a newer PATCH audited as this vibe's, the running vibe, and
// --static.
func writeTextHeader(b *strings.Builder, rep *contract.Report, vibeVersion string) {
	declared := ""
	if rep.Contract.Declared != rep.Contract.Audited {
		declared = "declared " + rep.Contract.Declared + "; "
	}
	scope := ""
	if rep.Static {
		scope = ", static checks only"
	}
	fmt.Fprintf(b, "vibe audit: contract %s (%svibe %s)%s\n\n", rep.Contract.Audited, declared, vibeVersion, scope)
}

// writeCategorySummary prints one line per category counting its
// results. A failed MUST rule counts as FAIL, a failed SHOULD rule as
// fail.
func writeCategorySummary(b *strings.Builder, rep *contract.Report) {
	width := 0
	for _, c := range contract.Categories() {
		width = max(width, len(c.Name()))
	}
	for _, c := range contract.Categories() {
		counts := map[string]int{}
		for _, rr := range rep.Rules {
			if rr.Rule.Category != c {
				continue
			}
			word := resultText(rr.Result)
			if rr.Result == contract.ResultFail && rr.Rule.Level == contract.Must {
				word = "FAIL"
			}
			counts[word]++
		}
		var parts []string
		for _, word := range []string{"pass", "FAIL", "fail", "not applicable", "waived", "cannot decide", "not run"} {
			if counts[word] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", counts[word], word))
			}
		}
		fmt.Fprintf(b, "  %s  %-*s   %s\n", c, width, c.Name(), strings.Join(parts, ", "))
	}
}

// writeSection prints a titled section with one entry per item, or
// "(none)".
func writeSection[T any](b *strings.Builder, title string, items []T, entry func(T)) {
	fmt.Fprintf(b, "\n%s\n", title)
	if len(items) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, item := range items {
		entry(item)
	}
}

// writeRuleBlock prints a rule with the findings of its parts that have
// result: each finding's file and message, then its fix.
func writeRuleBlock(b *strings.Builder, rr contract.RuleResult, result contract.Result) {
	fmt.Fprintf(b, "%s%s\n", ruleLead(rr.Rule), rr.Rule.Title)
	for _, p := range rr.Parts {
		if p.Result != result {
			continue
		}
		for _, f := range p.Findings {
			line := f.Message
			if f.File != "" {
				line = f.File + ": " + line
			}
			fmt.Fprintf(b, "        %s\n", line)
			if f.Fix != "" {
				fmt.Fprintf(b, "        Fix: %s\n", f.Fix)
			}
		}
	}
}

// writeNotRun lists the rules --static left without a result on one
// line, naming the parts that did not run when others did.
func writeNotRun(b *strings.Builder, rules []contract.RuleResult) {
	b.WriteString("\nNot run (--static)\n")
	if len(rules) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	items := make([]string, 0, len(rules))
	for _, rr := range rules {
		var kinds []string
		for _, p := range rr.Parts {
			if p.Result == contract.ResultNotRun {
				kinds = append(kinds, string(p.Kind))
			}
		}
		item := rr.Rule.ID
		if len(kinds) < len(rr.Parts) {
			item += " (" + strings.Join(kinds, ", ") + ")"
		}
		items = append(items, item)
	}
	fmt.Fprintf(b, "  %s\n", strings.Join(items, " "))
}

// ruleLead is the start of a rule's line: ID and level in columns.
func ruleLead(rule contract.Rule) string {
	return fmt.Sprintf("  %s  %-6s  ", rule.ID, rule.Level)
}

// waiverText says how a declared waiver applied: in force, expired (the
// rule was checked), or unused (the rule is not applicable).
func waiverText(rr contract.RuleResult) string {
	w := rr.Waiver
	until := w.Until.Format(time.DateOnly)
	switch {
	case w.Unused:
		return fmt.Sprintf("waiver unused (not applicable) until %s: %s", until, w.Reason)
	case w.Expired:
		return fmt.Sprintf("waiver expired %s: %s (checked: %s)", until, w.Reason, resultText(rr.Result))
	default:
		return fmt.Sprintf("until %s: %s", until, w.Reason)
	}
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

func rulesWith(rep *contract.Report, result contract.Result) []contract.RuleResult {
	var out []contract.RuleResult
	for _, rr := range rep.Rules {
		if rr.Result == result {
			out = append(out, rr)
		}
	}
	return out
}

func rulesWithWaiver(rep *contract.Report) []contract.RuleResult {
	var out []contract.RuleResult
	for _, rr := range rep.Rules {
		if rr.Waiver != nil {
			out = append(out, rr)
		}
	}
	return out
}

// levelRank orders failures: MUST first.
func levelRank(l contract.Level) int {
	if l == contract.Must {
		return 0
	}
	return 1
}

// resultText is a result as the text report words it.
func resultText(r contract.Result) string {
	switch r {
	case contract.ResultNotApplicable:
		return "not applicable"
	case contract.ResultUndecided:
		return "cannot decide"
	case contract.ResultNotRun:
		return "not run"
	default:
		return string(r)
	}
}
