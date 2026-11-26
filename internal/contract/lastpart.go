package contract

import (
	"fmt"
	"strings"
)

// partKey names a part by its rule ID and kind.
type partKey struct {
	rule string
	kind PartKind
}

func (k partKey) String() string { return k.rule + "/" + string(k.kind) }

// partRun is what the runner stores for a part that ran: its result,
// and the calls of vibe the shim recorded while it ran.
type partRun struct {
	key    partKey
	result PartResult
	calls  []shimCall
}

// Earlier is the last part's read-only view of every execution and
// probe part that ran before it (spec 0071).
type Earlier struct {
	// ran is every part that ran before the last part, in run order.
	ran []partRun
	// setupCalls are the calls of vibe setup made before any part ran.
	setupCalls []shimCall
}

// Result returns an earlier part's result and true, or false when the
// part did not run: its rule is not applicable or waived, or the probe
// copy's precondition failed.
func (e Earlier) Result(id string, kind PartKind) (PartResult, bool) {
	k := partKey{rule: id, kind: kind}
	for _, r := range e.ran {
		if r.key == k {
			return r.result, true
		}
	}
	return PartResult{}, false
}

// guardDeny returns A-02's verdict on its deny cases, or false when
// A-02's probe did not run.
func (e Earlier) guardDeny() (PartResult, bool) {
	r, ok := e.Result("A-02", Probe)
	if !ok || r.deny == nil {
		return PartResult{}, false
	}
	return *r.deny, true
}

// vibeCalls returns the calls of vibe setup made, then those made while
// the earlier parts ran, in run order.
func (e Earlier) vibeCalls() []shimCall {
	calls := append([]shimCall(nil), e.setupCalls...)
	for _, r := range e.ran {
		calls = append(calls, r.calls...)
	}
	return calls
}

// partRef is a part of the rule list by index.
type partRef struct {
	rule, part int
}

// lastPart validates the rule list and returns its last part, or false
// when it has none. It panics, naming the part, when a part sets both or
// neither of Run and RunLast, when a static part is the last part, or
// when two parts are last parts. Only a test's Options.Rules can reach
// the panic: a registry test keeps Rules() valid.
func lastPart(list []Rule) (partRef, bool) {
	var last []partRef
	for i, rule := range list {
		for j, part := range rule.Parts {
			ref := partRef{rule: i, part: j}
			key := keyOf(list, ref)
			switch {
			case part.Run != nil && part.RunLast != nil:
				panic(fmt.Sprintf("contract: %s sets both Run and RunLast; a part sets exactly one", key))
			case part.Run == nil && part.RunLast == nil:
				panic(fmt.Sprintf("contract: %s sets neither Run nor RunLast; a part sets exactly one", key))
			case part.RunLast != nil && part.Kind == Static:
				panic(fmt.Sprintf("contract: %s is a static part and sets RunLast; the last part is an execution or probe part", key))
			case part.RunLast != nil:
				last = append(last, ref)
			}
		}
	}
	switch len(last) {
	case 0:
		return partRef{}, false
	case 1:
		return last[0], true
	}
	names := make([]string, len(last))
	for i, ref := range last {
		names[i] = keyOf(list, ref).String()
	}
	panic("contract: a rule list has at most one last part, and these set RunLast: " + strings.Join(names, ", "))
}

func keyOf(list []Rule, ref partRef) partKey {
	return partKey{rule: list[ref.rule].ID, kind: list[ref.rule].Parts[ref.part].Kind}
}
