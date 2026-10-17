package contract

import (
	"cmp"
	"context"
	"slices"
	"time"
)

// Options shape one audit run.
type Options struct {
	// Static runs only the static parts: every execution and probe part
	// is not run, and the audit reaches no verdict (spec 0052, Q18).
	Static bool
	// Now decides whether waivers have expired. The zero value means the
	// current time.
	Now time.Time
	// Rules replaces the registry. Nil means Rules(). Tests use it to
	// stand in for parts.
	Rules []Rule
	// Probe shapes the runs of the execution and probe parts.
	Probe ProbeOptions
}

// Audit checks a repository against the configuration's contract and
// returns the report. It never writes to the repository: the execution
// and probe parts run in a probe copy, which is deleted before Audit
// returns (spec 0053).
func Audit(ctx context.Context, cfg *Config, repo *Repo, opts Options) *Report {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	list := opts.Rules
	if list == nil {
		list = Rules()
	}

	env := newEnv(ctx, cfg, repo)
	waivers := map[string]Waiver{}
	for _, w := range cfg.Waivers {
		waivers[w.Rule] = w
	}

	rep := &Report{Contract: cfg.Contract, Static: opts.Static}
	var pending []pendingPart
	for i, rule := range list {
		rr, parts := auditRule(env, rule, waivers, now, opts.Static)
		rep.Rules = append(rep.Rules, rr)
		for _, j := range parts {
			pending = append(pending, pendingPart{rule: i, part: j, order: partOrder(rule.ID, rule.Parts[j].Kind)})
		}
	}
	runPending(env, rep, list, pending, opts.Probe)
	for i := range rep.Rules {
		if rep.Rules[i].Result == "" {
			rep.Rules[i].Result = combine(rep.Rules[i].Parts)
		}
	}
	rep.GuardExceptions, rep.LocalAdjustments = guardListings(env)
	rep.GuardExceptionUses = env.guardUses
	rep.Notes = env.notes
	rep.decide()
	return rep
}

// auditRule decides one rule: not applicable, then waived, stop the
// check; otherwise every static part is decided. It returns the indexes
// of the execution and probe parts left to run in the probe copy; the
// rule's result is set once they ran.
func auditRule(env *Env, rule Rule, waivers map[string]Waiver, now time.Time, staticOnly bool) (RuleResult, []int) {
	rr := RuleResult{Rule: rule}
	if w, ok := waivers[rule.ID]; ok {
		rr.Waiver = &WaiverStatus{Reason: w.Reason, Until: w.Until, Expired: !now.Before(waiverEnd(w))}
	}

	if ok, why := rule.Applies(env.Config); !ok {
		rr.NotApplicable = why
		if rr.Waiver != nil {
			rr.Waiver.Unused = true
		}
		return settle(rr, ResultNotApplicable, Finding{File: env.Config.File, Message: "not applicable: " + why}), nil
	}
	if rr.Waiver != nil && !rr.Waiver.Expired {
		return settle(rr, ResultWaived), nil
	}

	var pending []int
	for j, part := range rule.Parts {
		po := PartOutcome{Kind: part.Kind}
		switch {
		case staticOnly && part.Kind != Static:
			po.Result = ResultNotRun
		case part.Kind == Static:
			po.Result, po.Findings = decided(part.Run(env))
		default:
			pending = append(pending, j)
		}
		rr.Parts = append(rr.Parts, po)
	}
	return rr, pending
}

// pendingPart is an execution or probe part waiting for the probe copy.
type pendingPart struct {
	rule, part, order int
}

// executionOrder is the fixed order of the execution and probe parts
// (spec 0053, "Order"), after setup. R-01's execution part comes last: it
// reads the baseline verify, A-02's deny cases and every shim call the
// other parts recorded.
var executionOrder = []string{
	"A-07/execution",
	"V-03/execution",
	"V-01/execution", "V-02/probe",
	"V-01/probe", "A-05/probe",
	"A-02/probe",
	"A-03/probe",
	"A-08/probe",
	"D-01/execution",
	"R-01/execution",
}

// partOrder is a part's place in executionOrder. A part the order does
// not name, such as a test's stand-in, runs before R-01.
func partOrder(id string, kind PartKind) int {
	if i := slices.Index(executionOrder, id+"/"+string(kind)); i >= 0 {
		return i * 10
	}
	return (len(executionOrder)-1)*10 - 5
}

// runPending runs the execution and probe parts in their fixed order, in
// one probe copy built for them, each from a reset copy. The copy is
// deleted (or kept, for --keep-probe-copy) before it returns.
func runPending(env *Env, rep *Report, list []Rule, pending []pendingPart, opts ProbeOptions) {
	if len(pending) == 0 {
		return
	}
	slices.SortStableFunc(pending, func(a, b pendingPart) int { return cmp.Compare(a.order, b.order) })

	s := startProbeSession(env, opts)
	defer s.close()
	env.probe = s
	defer func() { env.probe = nil }()

	for _, p := range pending {
		part := list[p.rule].Parts[p.part]
		po := &rep.Rules[p.rule].Parts[p.part]
		if f := s.precondition(env); f != nil {
			po.Result, po.Findings = ResultUndecided, []Finding{*f}
			continue
		}
		if err := s.Reset(); err != nil {
			po.Result, po.Findings = ResultUndecided, []Finding{{Message: "cannot decide: cannot reset the probe copy: " + err.Error()}}
			continue
		}
		po.Result, po.Findings = decided(part.Run(env))
	}
}

// decided is a part's result, a pass when the check left it empty.
func decided(r PartResult) (Result, []Finding) {
	if r.Result == "" {
		return ResultPass, r.Findings
	}
	return r.Result, r.Findings
}

// settle gives every part of a rule that was not checked the rule's
// result, and the first part the finding that explains it.
func settle(rr RuleResult, result Result, findings ...Finding) RuleResult {
	rr.Result = result
	for i, part := range rr.Rule.Parts {
		po := PartOutcome{Kind: part.Kind, Result: result}
		if i == 0 {
			po.Findings = findings
		}
		rr.Parts = append(rr.Parts, po)
	}
	return rr
}

// waiverEnd is the moment a waiver stops being in force: the end of its
// until day in UTC.
func waiverEnd(w Waiver) time.Time {
	day := w.Until
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
}
