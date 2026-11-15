package contract

import (
	"fmt"
	"strings"
	"time"
)

// Result is the outcome of a rule or of one part of its check (spec 0052,
// "Rule registry and results"). The values are the --json tokens.
type Result string

// The results.
const (
	ResultPass          Result = "pass"
	ResultFail          Result = "fail"
	ResultNotApplicable Result = "not_applicable"
	ResultWaived        Result = "waived"
	// ResultUndecided is "cannot decide".
	ResultUndecided Result = "undecided"
	// ResultNotRun is a part --static skipped.
	ResultNotRun Result = "not_run"
)

// Results lists every result in summary order.
var Results = []Result{ResultPass, ResultFail, ResultNotApplicable, ResultWaived, ResultUndecided, ResultNotRun}

// Finding explains a result. A failure's finding names the file to change
// or create and the direction of the fix.
type Finding struct {
	// File is slash separated and relative to the repository root, or a
	// directory with a trailing slash, such as .github/workflows/.
	File    string
	Message string
	Fix     string
}

// PartResult is what a CheckFunc decides: pass, fail or cannot decide,
// with the findings that explain it.
type PartResult struct {
	Result   Result
	Findings []Finding
}

// Pass is a part that holds.
func Pass() PartResult { return PartResult{Result: ResultPass} }

// Fail is a part that does not hold. Each finding names a file and a fix.
func Fail(findings ...Finding) PartResult {
	return PartResult{Result: ResultFail, Findings: findings}
}

// Undecided is a part the audit cannot decide, with the reason.
func Undecided(findings ...Finding) PartResult {
	return PartResult{Result: ResultUndecided, Findings: findings}
}

// Merge combines the parts of one check that each decide a half of it,
// such as R-02's AGENTS.md and CLAUDE.md halves: a failure in either
// fails it, otherwise an undecided half leaves it undecided. Findings
// are kept in order.
func Merge(results ...PartResult) PartResult {
	out := Pass()
	for _, r := range results {
		out.Findings = append(out.Findings, r.Findings...)
		switch {
		case r.Result == ResultFail:
			out.Result = ResultFail
		case r.Result == ResultUndecided && out.Result == ResultPass:
			out.Result = ResultUndecided
		}
	}
	return out
}

// ParseFailure is the finding for a file a rule needs that does not
// parse. It fails every rule that needs the file (spec 0052, "What
// static checks read"): a tool that reads it would not get the property
// either.
func ParseFailure(file string, err error) Finding {
	return Finding{
		File:    file,
		Message: fmt.Sprintf("does not parse: %v", err),
		Fix:     "fix the syntax so that the file parses",
	}
}

// PartOutcome is one part of a rule as the audit decided it.
type PartOutcome struct {
	Kind     PartKind
	Result   Result
	Findings []Finding
}

// WaiverStatus is a declared waiver as the audit applied it.
type WaiverStatus struct {
	Reason string
	// Until is the last day the waiver is in force (UTC).
	Until time.Time
	// Expired is true once the until day has ended in UTC: the waiver is
	// ignored and the rule checked ("waiver expired").
	Expired bool
	// Unused is true when the rule is not applicable, so the waiver
	// suspends nothing.
	Unused bool
}

// RuleResult is one rule's result.
type RuleResult struct {
	Rule   Rule
	Result Result
	Parts  []PartOutcome
	// NotApplicable says why the rule does not apply, when it does not.
	NotApplicable string
	// Waiver is the rule's declared waiver, or nil.
	Waiver *WaiverStatus
	// Required is true for a SHOULD rule the configuration requires (spec
	// 0063): it counts in the verdict like a MUST rule.
	Required bool
}

// Binding reports whether the rule's result counts in the verdict: a MUST
// rule or a required SHOULD rule.
func (r RuleResult) Binding() bool {
	return r.Rule.Level == Must || r.Required
}

// Findings returns the findings of every part, in part order.
func (r RuleResult) Findings() []Finding {
	var out []Finding
	for _, p := range r.Parts {
		out = append(out, p.Findings...)
	}
	return out
}

// Verdict is the audit's answer for the whole repository (spec 0051, "The
// verdict"). The values are the --json tokens.
type Verdict string

// The verdicts.
const (
	VerdictConformant    Verdict = "conformant"
	VerdictNotConformant Verdict = "not_conformant"
	// VerdictUndecided is "cannot decide": there is no verdict.
	VerdictUndecided Verdict = "undecided"
)

// GuardException is one exception of the guard policy (A-04), listed in
// the report.
type GuardException struct {
	Rule   string
	Match  string
	Reason string
}

// LocalAdjustment is one exception or extra rule of a tracked
// .claude/hooks/policy.local.json, listed in the report for review.
type LocalAdjustment struct {
	File string
	// Kind is "exception" or "rule".
	Kind string
	// Rule is the guard rule ID an exception loosens, or an extra rule's
	// ID.
	Rule    string
	Pattern string
	Reason  string
}

// Report is the contract audit's result as plain data. The CLI renders it
// as text or JSON.
type Report struct {
	Contract Versions
	// Static is true for --static: execution and probe parts did not run.
	Static  bool
	Verdict Verdict
	// Reason explains a VerdictUndecided, such as "10 rules did not run
	// (--static); 1 MUST rule already fails".
	Reason string
	// FailingMust counts the MUST rules that fail.
	FailingMust int
	// FailingRequired counts the required SHOULD rules that fail.
	FailingRequired int
	// Rules holds every rule in report order.
	Rules           []RuleResult
	GuardExceptions []GuardException
	// GuardExceptionUses are the corpus cases each exception let through
	// (A-02), in corpus order.
	GuardExceptionUses []GuardExceptionUse
	LocalAdjustments   []LocalAdjustment
	// Notes are facts the audit assumed, such as the default branch.
	Notes []string
	// CIProvider is the CI provider the C rules, D-03 and R-01 read, or ""
	// when they read none (spec 0055, Q1).
	CIProvider CIProvider
}

// Count returns how many rules of a level have a result.
func (r *Report) Count(level Level, result Result) int {
	n := 0
	for _, rr := range r.Rules {
		if rr.Rule.Level == level && rr.Result == result {
			n++
		}
	}
	return n
}

// combine decides a rule from its parts: it fails if any part that ran
// failed; otherwise it cannot be decided if any part could not be;
// otherwise it did not run if any part did not run; otherwise it passes.
func combine(parts []PartOutcome) Result {
	has := map[Result]bool{}
	for _, p := range parts {
		has[p.Result] = true
	}
	switch {
	case has[ResultFail]:
		return ResultFail
	case has[ResultUndecided]:
		return ResultUndecided
	case has[ResultNotRun]:
		return ResultNotRun
	default:
		return ResultPass
	}
}

// decide sets the report's verdict from its binding rules: the MUST rules
// and the required SHOULD rules. Other SHOULD results never change it.
func (r *Report) decide() {
	var undecided, notRun, bindingOpen int
	for _, rr := range r.Rules {
		switch rr.Result {
		case ResultUndecided:
			undecided++
		case ResultNotRun:
			notRun++
		}
		if !rr.Binding() {
			continue
		}
		switch rr.Result {
		case ResultFail:
			if rr.Rule.Level == Must {
				r.FailingMust++
			} else {
				r.FailingRequired++
			}
		case ResultUndecided, ResultNotRun:
			bindingOpen++
		}
	}
	switch {
	case bindingOpen > 0:
		r.Verdict = VerdictUndecided
		r.Reason = r.undecidedReason(undecided, notRun)
	case r.FailingMust > 0 || r.FailingRequired > 0:
		r.Verdict = VerdictNotConformant
	default:
		r.Verdict = VerdictConformant
	}
}

// undecidedReason says why there is no verdict: how many rules cannot be
// decided or did not run, then how many binding rules already fail.
func (r *Report) undecidedReason(undecided, notRun int) string {
	var why []string
	if undecided > 0 {
		why = append(why, fmt.Sprintf("%d %s", undecided, plural(undecided, "rule cannot be decided", "rules cannot be decided")))
	}
	if notRun > 0 {
		why = append(why, fmt.Sprintf("%d %s (--static)", notRun, plural(notRun, "rule did not run", "rules did not run")))
	}
	why = append(why, r.failing("already ")...)
	return strings.Join(why, "; ")
}

// FailingReason says which binding rules fail, such as "1 MUST rule fails;
// 2 required SHOULD rules fail". It is empty when none fails.
func (r *Report) FailingReason() string {
	return strings.Join(r.failing(""), "; ")
}

// failing counts the failing MUST rules, then the failing required SHOULD
// rules, each with its verb after adverb, such as "1 MUST rule already
// fails".
func (r *Report) failing(adverb string) []string {
	var why []string
	for _, k := range []struct {
		n    int
		kind string
	}{{r.FailingMust, "MUST rule"}, {r.FailingRequired, "required SHOULD rule"}} {
		if k.n > 0 {
			why = append(why, fmt.Sprintf("%d %s %s%s", k.n, plural(k.n, k.kind, k.kind+"s"), adverb, plural(k.n, "fails", "fail")))
		}
	}
	return why
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
