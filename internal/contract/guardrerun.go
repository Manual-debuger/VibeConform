package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The rerun (spec 0072, "The rerun"): under 2.0.0, a deny case that the
// guard let through and an exception matches runs again with every
// matching exception removed from the probe copy's policy.json. The
// exception counts only when the guard refuses the case without it.

// rerunFix is the fix for a deny case the guard lets through without the
// exception that matches it.
const rerunFix = "make the guard refuse it; an exception counts only when the guard refuses the case without it"

// caseRerun is one deny case's run without the exceptions that match it.
// err says why the case could not run again.
type caseRerun struct {
	removed []guardrule.Exception
	run     caseRun
	err     string
}

// rerunSet is the deny cases that remove the same exceptions.
type rerunSet struct {
	removed []guardrule.Exception
	idx     []int
}

// rerunWithoutExceptions reruns, by the index of runs, every deny case
// that the hooks ran and let through and that an exception matches.
// Cases that remove the same exceptions run together, with feedCases's
// grouping; the sets run one after another, because policy.json is one
// file. Afterwards the original policy.json is back in the probe copy,
// also on an error; when it cannot be written back, both verdicts are
// undecided.
func (t *corpusTally) rerunWithoutExceptions(runs []caseRun) map[int]*caseRerun {
	sets := t.rerunSets(runs)
	if len(sets) == 0 {
		return nil
	}
	reruns := map[int]*caseRerun{}
	fail := func(set *rerunSet, why string) {
		for _, i := range set.idx {
			reruns[i] = &caseRerun{removed: set.removed, err: why}
		}
	}
	run := func(set *rerunSet) {
		cases := make([]GuardCase, len(set.idx))
		for j, i := range set.idx {
			cases[j] = runs[i].c
		}
		without := "without " + exceptionsText(set.removed)
		out, err := feedCases(t.s, t.hooks, cases, "the PreToolUse hooks during "+t.rule+" "+without,
			func(c GuardCase) string { return t.rule + " with " + describeGuardCase(c) + " " + without }, runOpts{})
		if err != nil {
			fail(set, "cannot write the unlock marker: "+err.Error())
			return
		}
		for j, i := range set.idx {
			reruns[i] = &caseRerun{removed: set.removed, run: out[j]}
		}
	}
	p := filepath.Join(t.s.Root(), filepath.FromSlash(guardPolicyFile))
	if err := rewritePolicyPerSet(p, sets, run, fail); err != nil {
		t.undecidedCase(Finding{Message: "cannot decide: cannot restore " + guardPolicyFile + " in the probe copy: " + err.Error()})
	}
	return reruns
}

// rewritePolicyPerSet writes, for each set in turn, the policy.json at p
// without the set's exceptions, and calls run while it is written. A set
// whose policy cannot be read or written is not run: fail gets why.
// Afterwards the original file is back, also when a run fails or removes
// it; restoreErr says when it could not be written back.
func rewritePolicyPerSet(p string, sets []*rerunSet, run func(*rerunSet), fail func(set *rerunSet, why string)) (restoreErr error) {
	original, err := os.ReadFile(p) // #nosec G304 -- a path inside the probe copy
	if err != nil {
		for _, set := range sets {
			fail(set, "cannot read "+guardPolicyFile+" in the probe copy: "+err.Error())
		}
		return nil
	}
	defer func() {
		restoreErr = os.WriteFile(p, original, 0o600) // #nosec G703 -- a path inside the probe copy
	}()
	for _, set := range sets {
		policy, err := withoutExceptions(original, set.removed)
		if err == nil {
			err = os.WriteFile(p, policy, 0o600) // #nosec G703 -- a path inside the probe copy
		}
		if err != nil {
			fail(set, "cannot remove the exception from "+guardPolicyFile+" in the probe copy: "+err.Error())
			continue
		}
		run(set)
	}
	return nil
}

// rerunSets groups the deny cases to rerun by the exceptions they
// remove, in the order first seen.
func (t *corpusTally) rerunSets(runs []caseRun) []*rerunSet {
	var sets []*rerunSet
	byKey := map[string]*rerunSet{}
	for i, run := range runs {
		if !run.letThrough() {
			continue
		}
		ex := t.exceptionsFor(run.c)
		if len(ex) == 0 {
			continue
		}
		key := fmt.Sprintf("%q", ex)
		set := byKey[key]
		if set == nil {
			set = &rerunSet{removed: ex}
			byKey[key] = set
			sets = append(sets, set)
		}
		set.idx = append(set.idx, i)
	}
	return sets
}

// letThrough reports whether a deny case ran through some hook and none
// refused it or failed to end.
func (r caseRun) letThrough() bool {
	return r.c.Want == WantDeny && len(r.runs) > 0 && !r.refused() && r.undecided() == nil
}

// withoutExceptions is the policy.json data without the entries of
// exceptions that name a removed exception's rule and match. Every other
// entry stays.
func withoutExceptions(data []byte, removed []guardrule.Exception) ([]byte, error) {
	top, entries, err := policyEntries(data)
	if err != nil {
		return nil, err
	}
	kept := []json.RawMessage{}
	for _, en := range entries {
		if en.object && slices.ContainsFunc(removed, func(ex guardrule.Exception) bool {
			return ex.Rule == en.Rule && ex.Match == en.Match
		}) {
			continue
		}
		kept = append(kept, en.raw)
	}
	if top["exceptions"], err = json.Marshal(kept); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// exceptionsText names exceptions for a finding: `the exception rm-rf
// "rm -r -f build" in .claude/hooks/policy.json`.
func exceptionsText(exceptions []guardrule.Exception) string {
	var names []string
	for _, ex := range exceptions {
		names = append(names, fmt.Sprintf("%s %q", ex.Rule, ex.Match))
	}
	return plural(len(names), "the exception ", "the exceptions ") + strings.Join(names, ", ") + " in " + guardPolicyFile
}

// rerunCase judges a deny case an exception let through, under 2.0.0, by
// rerun, its run without the exceptions that match it.
func (t *corpusTally) rerunCase(run caseRun, ex []guardrule.Exception, rerun *caseRerun) {
	c := run.c
	if rerun == nil {
		rerun = &caseRerun{removed: ex, err: "the case did not run again"}
	}
	without := run.what() + " without " + exceptionsText(rerun.removed)
	switch {
	case rerun.err != "":
		t.undecidedCase(Finding{File: claudeSettingsFile, Message: "cannot decide: " + without + ": " + rerun.err})
	case rerun.run.refused():
		t.uses = append(t.uses, GuardExceptionUse{Rule: ex[0].Rule, Match: ex[0].Match, Case: c.Name})
	case rerun.run.undecided() != nil:
		t.undecidedCase(Finding{File: claudeSettingsFile,
			Message: "cannot decide: " + without + ": " + rerun.run.undecided().describe()})
	default:
		t.failCase(Finding{File: claudeSettingsFile,
			Message: fmt.Sprintf("%s should be refused (guard rule %s) without %s; %s",
				run.what(), c.Rule, exceptionsText(rerun.removed), rerun.run.exits()),
			Fix: rerunFix})
	}
}
