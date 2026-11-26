package guard

import gr "github.com/Manual-debuger/VibeConform/internal/guardrule"

// RuleRefuses reports whether a baseline rule with the ID id refuses the
// simple command c in g's repository, by a command rule or as a write of
// one of c's paths, and whether such a rule is fixed: no exception lifts
// it. It reads the rules decideCommand applies (commandMatches), without
// the policy, so that a test can check the corpus's fixed and refuses
// against them (spec 0072, "Guard corpus").
func (g *Guard) RuleRefuses(id string, c gr.Command) (refused, fixed bool) {
	return refusedBy(id, g.commandMatches(&c))
}

// RuleRefusesEdit is RuleRefuses for an edit of path, with the rules
// decidePath applies.
func (g *Guard) RuleRefusesEdit(id, path string) (refused, fixed bool) {
	return refusedBy(id, g.pathMatches(pathRules, g.normalize(path), path, nil))
}

func refusedBy(id string, matches []ruleMatch) (refused, fixed bool) {
	for _, m := range matches {
		if m.finding.Rule == id {
			refused, fixed = true, fixed || m.fixed
		}
	}
	return refused, fixed
}
