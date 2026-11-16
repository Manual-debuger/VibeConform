package contract

import (
	"context"
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

	order := runOrder(list)
	rep := &Report{Contract: cfg.Contract, Static: opts.Static}
	pending := map[partRef]bool{}
	for i, rule := range list {
		rr, parts := auditRule(env, rule, waivers, now, opts.Static)
		rep.Rules = append(rep.Rules, rr)
		for _, j := range parts {
			pending[partRef{rule: i, part: j}] = true
		}
	}
	ran := runPending(env, rep, list, order, pending, opts.Probe)
	for i := range rep.Rules {
		if rep.Rules[i].Result == "" {
			rep.Rules[i].Result = combine(rep.Rules[i].Parts)
		}
	}
	rep.GuardExceptions, rep.LocalAdjustments = guardListings(env)
	for _, r := range ran {
		rep.GuardExceptionUses = append(rep.GuardExceptionUses, r.result.guardUses...)
	}
	if ci, _, ok := env.CI(); ok {
		rep.CIProvider = ci.Provider
	}
	rep.Notes = env.notes
	rep.decide()
	return rep
}

// auditRule decides one rule: not applicable, then waived, stop the
// check; otherwise every static part is decided. It returns the indexes
// of the execution and probe parts left to run in the probe copy; the
// rule's result is set once they ran.
func auditRule(env *Env, rule Rule, waivers map[string]Waiver, now time.Time, staticOnly bool) (RuleResult, []int) {
	rr := RuleResult{Rule: rule, Required: env.Config.Requires(rule.ID)}
	if w, ok := waivers[rule.ID]; ok {
		rr.Waiver = &WaiverStatus{Reason: w.Reason, Until: w.Until, Expired: !now.Before(waiverEnd(w))}
	}

	if ok, why := rule.Applies(env); !ok {
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
			po.Result, po.Findings = decided(part.Run(env, Needed{}))
		default:
			pending = append(pending, j)
		}
		rr.Parts = append(rr.Parts, po)
	}
	return rr, pending
}

// runPending runs the pending execution and probe parts in order (see
// runOrder), in one probe copy built for them, each from a reset copy.
// Each part gets the results of the parts it needs that ran before it,
// and the calls of vibe the shim recorded while they ran. It returns
// every part that ran, in run order. The copy is deleted (or kept, for
// --keep-probe-copy) before it returns.
func runPending(env *Env, rep *Report, list []Rule, order []partRef, pending map[partRef]bool, opts ProbeOptions) []partRun {
	if len(pending) == 0 {
		return nil
	}

	s := startProbeSession(env, opts)
	defer s.close()
	env.probe = s
	defer func() { env.probe = nil }()

	// ran holds every part that ran, in run order, for one audit.
	var ran []partRun
	setupCalls := s.takeCalls()
	for _, p := range order {
		if !pending[p] {
			continue
		}
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
		key := keyOf(list, p)
		r := part.Run(env, Needed{self: key, needs: part.Needs, ran: ran, setupCalls: setupCalls})
		r.Result, r.Findings = decided(r)
		ran = append(ran, partRun{key: key, result: r, calls: s.takeCalls()})
		po.Result, po.Findings = r.Result, r.Findings
	}
	return ran
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
