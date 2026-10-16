package contract

import "slices"

// Level is how binding a rule is (spec 0051, "The verdict"): a failed
// MUST rule makes a repository not conformant; a failed SHOULD rule is
// reported and never changes the verdict.
type Level string

// The levels of contract 1.0.0.
const (
	Must   Level = "MUST"
	Should Level = "SHOULD"
)

// Category groups rules by the property they are about. It is the rule
// ID's letter.
type Category string

// The categories of contract 1.0.0, in report order.
const (
	CategoryVerification Category = "V"
	CategoryHooks        Category = "H"
	CategoryCI           Category = "C"
	CategoryAgents       Category = "A"
	CategoryDependencies Category = "D"
	CategoryRemovability Category = "R"
)

// Rule is one rule of contract 1.0.0 (spec 0051, "Rules").
type Rule struct {
	ID       string
	Level    Level
	Category Category
}

// rules is contract 1.0.0's rule table, in ID order within report order.
// 16 MUST and 9 SHOULD rules.
var rules = []Rule{
	{"V-01", Must, CategoryVerification},
	{"V-02", Must, CategoryVerification},
	{"V-03", Must, CategoryVerification},
	{"V-04", Must, CategoryVerification},
	{"H-01", Should, CategoryHooks},
	{"H-02", Should, CategoryHooks},
	{"C-01", Must, CategoryCI},
	{"C-02", Must, CategoryCI},
	{"C-03", Must, CategoryCI},
	{"C-04", Must, CategoryCI},
	{"C-05", Should, CategoryCI},
	{"A-01", Must, CategoryAgents},
	{"A-02", Must, CategoryAgents},
	{"A-03", Must, CategoryAgents},
	{"A-04", Must, CategoryAgents},
	{"A-05", Must, CategoryAgents},
	{"A-06", Should, CategoryAgents},
	{"A-07", Should, CategoryAgents},
	{"A-08", Must, CategoryAgents},
	{"D-01", Must, CategoryDependencies},
	{"D-02", Should, CategoryDependencies},
	{"D-03", Should, CategoryDependencies},
	{"R-01", Must, CategoryRemovability},
	{"R-02", Should, CategoryRemovability},
	{"R-03", Should, CategoryRemovability},
}

// Rules returns every rule of contract 1.0.0 in report order.
func Rules() []Rule {
	return slices.Clone(rules)
}

// LookupRule returns the rule with the given ID.
func LookupRule(id string) (Rule, bool) {
	i := slices.IndexFunc(rules, func(r Rule) bool { return r.ID == id })
	if i < 0 {
		return Rule{}, false
	}
	return rules[i], true
}

// RuleIDs returns every rule ID in report order.
func RuleIDs() []string {
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	return ids
}
