package contract

import (
	"fmt"
	"slices"
	"strings"
)

// Need is what an execution or probe part requires to have run before
// it (spec 0064): one named part, or every other execution and probe
// part of the audit. Static parts declare none.
type Need struct {
	part partKey
	// everyOther is the need on every other execution and probe part.
	everyOther bool
}

// NeedPart is the need on one part: rule id's part of kind.
func NeedPart(id string, kind PartKind) Need {
	return Need{part: partKey{rule: id, kind: kind}}
}

// NeedEveryOther is the need on every other execution and probe part of
// the audit. The part that declares it runs after all of them and sees
// all their results.
func NeedEveryOther() Need { return Need{everyOther: true} }

func (n Need) String() string {
	if n.everyOther {
		return "every other execution and probe part"
	}
	return n.part.String()
}

// partKey names a part by its rule ID and kind.
type partKey struct {
	rule string
	kind PartKind
}

func (k partKey) String() string { return k.rule + "/" + string(k.kind) }

// Needed is a part's read-only view of the results of the parts it
// declared it needs. Static parts get the empty view.
type Needed struct {
	self    partKey
	needs   []Need
	results map[partKey]PartResult
}

// Result returns a needed part's result and true, or false when the
// part did not run (its rule is not applicable or waived, the audit is
// static, or the probe copy's precondition failed) or the asking part
// did not declare it as a need.
func (n Needed) Result(id string, kind PartKind) (PartResult, bool) {
	k := partKey{rule: id, kind: kind}
	if !n.declares(k) {
		return PartResult{}, false
	}
	r, ok := n.results[k]
	return r, ok
}

func (n Needed) declares(k partKey) bool {
	for _, need := range n.needs {
		if need.everyOther && k != n.self || !need.everyOther && need.part == k {
			return true
		}
	}
	return false
}

// partRef is a part of the rule list by index.
type partRef struct {
	rule, part int
}

// runOrder validates the needs of the rule list and returns its
// execution and probe parts in the order they run: a stable topological
// order that always takes the first ready part in rule-list order, and
// in part order inside a rule. It panics, naming the part and its need,
// when a static part declares a need, a need names a part the list does
// not hold, or the needs form a cycle. Only a test's Options.Rules can
// reach the panic: a registry test keeps Rules() valid.
func runOrder(list []Rule) []partRef {
	parts := runParts(list)
	after := needEdges(list, parts)

	var order []partRef
	placed := map[partRef]bool{}
	for len(order) < len(parts) {
		i := slices.IndexFunc(parts, func(p partRef) bool { return !placed[p] && allPlaced(after[p], placed) })
		if i < 0 {
			panic("contract: the needs form a cycle: " + cycleText(list, parts, placed))
		}
		placed[parts[i]] = true
		order = append(order, parts[i])
	}
	return order
}

// runParts lists the execution and probe parts in rule-list and part
// order. It panics on a static part that declares a need.
func runParts(list []Rule) []partRef {
	var parts []partRef
	for i, rule := range list {
		for j, part := range rule.Parts {
			ref := partRef{rule: i, part: j}
			switch {
			case part.Kind != Static:
				parts = append(parts, ref)
			case len(part.Needs) > 0:
				panic(fmt.Sprintf("contract: %s needs %s, but a static part declares no needs", keyOf(list, ref), part.Needs[0]))
			}
		}
	}
	return parts
}

// needEdges maps each part to the parts it runs after. It panics on a
// need the list does not hold and on two parts that each need every
// other part.
func needEdges(list []Rule, parts []partRef) map[partRef][]partRef {
	after := map[partRef][]partRef{}
	var everyOther []partRef
	for _, p := range parts {
		for _, need := range list[p.rule].Parts[p.part].Needs {
			if need.everyOther {
				everyOther = append(everyOther, p)
				after[p] = append(after[p], slices.DeleteFunc(slices.Clone(parts), func(q partRef) bool { return q == p })...)
				continue
			}
			i := slices.IndexFunc(parts, func(q partRef) bool { return keyOf(list, q) == need.part })
			if i < 0 {
				panic(fmt.Sprintf("contract: %s needs %s, which is not in the rule list", keyOf(list, p), need))
			}
			after[p] = append(after[p], parts[i])
		}
	}
	if len(everyOther) > 1 {
		panic(fmt.Sprintf("contract: %s and %s each need %s, a cycle",
			keyOf(list, everyOther[0]), keyOf(list, everyOther[1]), NeedEveryOther()))
	}
	return after
}

func allPlaced(refs []partRef, placed map[partRef]bool) bool {
	for _, r := range refs {
		if !placed[r] {
			return false
		}
	}
	return true
}

// cycleText names each part left unplaced with its needs, such as
// "T-01/probe needs T-02/probe; T-02/probe needs T-01/probe".
func cycleText(list []Rule, parts []partRef, placed map[partRef]bool) string {
	var lines []string
	for _, p := range parts {
		if placed[p] {
			continue
		}
		for _, need := range list[p.rule].Parts[p.part].Needs {
			lines = append(lines, keyOf(list, p).String()+" needs "+need.String())
		}
	}
	return strings.Join(lines, "; ")
}

func keyOf(list []Rule, ref partRef) partKey {
	return partKey{rule: list[ref.rule].ID, kind: list[ref.rule].Parts[ref.part].Kind}
}
