package guard

import gr "github.com/Manual-debuger/VibeConform/internal/guardrule"

// RuleRefuses reports whether a baseline rule with the ID id refuses the
// simple command c in g's repository, by a command rule or as a write of
// one of c's paths, and whether such a rule is fixed: no exception lifts
// it. It reads the rule table as decideCommand does, without the policy,
// so that a test can check the corpus's fixed and refuses against it
// (spec 0072, "Guard corpus").
func (g *Guard) RuleRefuses(id string, c gr.Command) (refused, fixed bool) {
	for _, r := range commandRulesFor(c.Shell) {
		if r.id == id && r.match(c) {
			refused, fixed = true, fixed || r.fixed
		}
	}
	for _, w := range c.Writes {
		if !w.Literal {
			continue
		}
		p := g.normalize(w.Text)
		for _, r := range shellWriteRules {
			if r.id == id && r.match(g, p) {
				refused, fixed = true, fixed || r.fixed
			}
		}
	}
	return refused, fixed
}

// RuleRefusesEdit is RuleRefuses for an edit of path.
func (g *Guard) RuleRefusesEdit(id, path string) (refused, fixed bool) {
	p := g.normalize(path)
	for _, r := range pathRules {
		if r.id == id && r.match(g, p) {
			refused, fixed = true, fixed || r.fixed
		}
	}
	return refused, fixed
}
