package contract

import (
	"context"
	"fmt"
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
	// stand in for parts this vibe does not implement yet.
	Rules []Rule
}

// Audit checks a repository against the configuration's contract and
// returns the report. It reads tracked files only and writes nothing.
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
	for _, rule := range list {
		rep.Rules = append(rep.Rules, auditRule(env, rule, waivers, now, opts.Static))
	}
	rep.GuardExceptions, rep.LocalAdjustments = guardListings(env)
	rep.Notes = env.notes
	rep.decide()
	return rep
}

// auditRule decides one rule: not applicable, then waived, stop the
// check; otherwise every part is decided and combined.
func auditRule(env *Env, rule Rule, waivers map[string]Waiver, now time.Time, staticOnly bool) RuleResult {
	rr := RuleResult{Rule: rule}
	if w, ok := waivers[rule.ID]; ok {
		rr.Waiver = &WaiverStatus{Reason: w.Reason, Until: w.Until, Expired: !now.Before(waiverEnd(w))}
	}

	if ok, why := rule.Applies(env.Config); !ok {
		rr.NotApplicable = why
		if rr.Waiver != nil {
			rr.Waiver.Unused = true
		}
		return settle(rr, ResultNotApplicable, Finding{File: env.Config.File, Message: "not applicable: " + why})
	}
	if rr.Waiver != nil && !rr.Waiver.Expired {
		return settle(rr, ResultWaived)
	}

	for _, part := range rule.Parts {
		rr.Parts = append(rr.Parts, runPart(env, part, staticOnly))
	}
	rr.Result = combine(rr.Parts)
	return rr
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

func runPart(env *Env, part Part, staticOnly bool) PartOutcome {
	po := PartOutcome{Kind: part.Kind}
	switch {
	case staticOnly && part.Kind != Static:
		po.Result = ResultNotRun
	case part.Run == nil:
		po.Result = ResultUndecided
		po.Findings = []Finding{{
			Message: fmt.Sprintf("the %s check is not implemented in this vibe; #%d adds it", part.Kind, part.Issue),
		}}
	default:
		r := part.Run(env)
		po.Result, po.Findings = r.Result, r.Findings
		if po.Result == "" {
			po.Result = ResultPass
		}
	}
	return po
}

// waiverEnd is the moment a waiver stops being in force: the end of its
// until day in UTC.
func waiverEnd(w Waiver) time.Time {
	day := w.Until
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
}
