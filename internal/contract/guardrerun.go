package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
// that hooks ran and let through and that an exception matches under the
// 2.0.0 reading. Cases that remove the same exceptions run together, with
// feedCases's grouping; the sets run one after another, because
// policy.json is one file. Afterwards the original policy.json is back in
// the probe copy, also on an error; restoreErr says when it could not be
// written back.
func rerunWithoutExceptions(s *probeSession, hooks []settingsHook, runs []caseRun, exceptions []guardrule.Exception,
	rule string) (reruns map[int]*caseRerun, restoreErr error) {
	sets := rerunSets(s, runs, exceptions)
	if len(sets) == 0 {
		return nil, nil
	}
	reruns = map[int]*caseRerun{}
	fail := func(set *rerunSet, why string) {
		for _, i := range set.idx {
			reruns[i] = &caseRerun{removed: set.removed, err: why}
		}
	}
	p := filepath.Join(s.Root(), filepath.FromSlash(guardPolicyFile))
	original, err := os.ReadFile(p) // #nosec G304 -- a path inside the probe copy
	if err != nil {
		for _, set := range sets {
			fail(set, "cannot read "+guardPolicyFile+" in the probe copy: "+err.Error())
		}
		return reruns, nil
	}
	defer func() {
		if err := os.WriteFile(p, original, 0o600); err != nil { // #nosec G703 -- a path inside the probe copy
			restoreErr = err
		}
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
		cases := make([]GuardCase, len(set.idx))
		for j, i := range set.idx {
			cases[j] = runs[i].c
		}
		without := "without " + exceptionsText(set.removed)
		out, err := feedCases(s, hooks, cases, "the PreToolUse hooks during "+rule+" "+without,
			func(c GuardCase) string { return rule + " with " + describeGuardCase(c) + " " + without }, runOpts{})
		if err != nil {
			fail(set, "cannot write the unlock marker: "+err.Error())
			continue
		}
		for j, i := range set.idx {
			reruns[i] = &caseRerun{removed: set.removed, run: out[j]}
		}
	}
	return reruns, nil
}

// rerunSets groups the deny cases to rerun by the exceptions they
// remove, in the order first seen.
func rerunSets(s *probeSession, runs []caseRun, exceptions []guardrule.Exception) []*rerunSet {
	var sets []*rerunSet
	byKey := map[string]*rerunSet{}
	for i, run := range runs {
		if !run.letThrough() {
			continue
		}
		ex := exceptionsFor(exceptions, run.c, s, Contract2)
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
	top, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	list, err := (&guardPolicy{top: top}).exceptions()
	if err != nil {
		return nil, err
	}
	kept := []json.RawMessage{}
	for _, raw := range list {
		var ex map[string]any
		if json.Unmarshal(raw, &ex) == nil && ex != nil && removes(removed, stringField(ex, "rule"), stringField(ex, "match")) {
			continue
		}
		kept = append(kept, raw)
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

func removes(removed []guardrule.Exception, rule, match string) bool {
	for _, ex := range removed {
		if ex.Rule == rule && ex.Match == match {
			return true
		}
	}
	return false
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
// its rerun without the exceptions that match it.
func (t *corpusTally) rerunCase(run caseRun, ex []guardrule.Exception, rr *caseRerun) {
	c := run.c
	if rr == nil {
		rr = &caseRerun{removed: ex, err: "the case did not run again"}
	}
	without := run.what() + " without " + exceptionsText(rr.removed)
	switch {
	case rr.err != "":
		t.undecidedCase(Finding{File: claudeSettingsFile, Message: "cannot decide: " + without + ": " + rr.err})
	case rr.run.refused():
		t.uses = append(t.uses, GuardExceptionUse{Rule: ex[0].Rule, Match: ex[0].Match, Case: c.Name})
	case rr.run.undecided() != nil:
		t.undecidedCase(Finding{File: claudeSettingsFile,
			Message: "cannot decide: " + without + ": " + rr.run.undecided().describe()})
	default:
		t.failCase(Finding{File: claudeSettingsFile,
			Message: fmt.Sprintf("%s should be refused (guard rule %s) without %s; %s",
				run.what(), c.Rule, exceptionsText(rr.removed), rr.run.exits()),
			Fix: rerunFix})
	}
}
