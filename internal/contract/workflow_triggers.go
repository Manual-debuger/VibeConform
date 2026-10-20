package contract

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Triggers, branch filters, the default branch and local reusable
// workflows, as the C rules read them (spec 0052, "Recognition rules",
// GitHub workflows).

// The two events C-01 and C-04 read.
const (
	eventPullRequest = "pull_request"
	eventPush        = "push"
)

// prTypes are the pull_request activity types a types filter must keep
// for a workflow to count: GitHub's default set.
var prTypes = []string{"opened", "synchronize", "reopened"}

// fallbackBranches are the default branches assumed, in order, when the
// clone has no refs/remotes/origin/HEAD.
var fallbackBranches = []string{"main", "master"}

// eventNode returns a workflow's trigger for event: (filters, true) when
// the workflow is triggered by it. filters is nil for an event with no
// filters (on: push, on: [push], or push: with no value).
func eventNode(wf *Workflow, event string) (*yaml.Node, bool) {
	on := wf.On
	if on == nil {
		return nil, false
	}
	switch on.Kind {
	case yaml.ScalarNode:
		return nil, on.Value == event
	case yaml.SequenceNode:
		for _, n := range on.Content {
			if n = resolve(n); n.Kind == yaml.ScalarNode && n.Value == event {
				return nil, true
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(on.Content); i += 2 {
			if on.Content[i].Value == event {
				v := resolve(on.Content[i+1])
				if v.Kind == yaml.MappingNode {
					return v, true
				}
				return nil, true
			}
		}
	}
	return nil, false
}

// triggeredFor reports whether a workflow counts for event on branch: it
// is triggered by event, has no paths or paths-ignore filter, no tags
// filter without a branch filter, a branch filter that admits branch,
// and (for pull_request) a types filter, if any, that keeps opened,
// synchronize and reopened.
func triggeredFor(wf *Workflow, event, branch string) bool {
	filters, ok := eventNode(wf, event)
	if !ok {
		return false
	}
	if filters == nil {
		return true
	}
	if mapValue(filters, "paths") != nil || mapValue(filters, "paths-ignore") != nil {
		return false
	}
	branches, ignore := mapValue(filters, "branches"), mapValue(filters, "branches-ignore")
	if branches == nil && ignore == nil &&
		(mapValue(filters, "tags") != nil || mapValue(filters, "tags-ignore") != nil) {
		return false
	}
	if branches != nil && !admits(filterPatterns(branches), branch) {
		return false
	}
	if ignore != nil && ignores(filterPatterns(ignore), branch) {
		return false
	}
	if event == eventPullRequest {
		if types := mapValue(filters, "types"); types != nil {
			kept := filterPatterns(types)
			for _, t := range prTypes {
				if !slices.Contains(kept, t) {
					return false
				}
			}
		}
	}
	return true
}

// filterPatterns reads a filter: a list of strings or one string.
func filterPatterns(n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			if c = resolve(c); c.Kind == yaml.ScalarNode {
				out = append(out, c.Value)
			}
		}
		return out
	}
	return nil
}

// admits applies a branches filter: the last pattern that matches decides,
// a pattern starting with ! excludes, and a branch no pattern matches is
// not admitted.
func admits(patterns []string, branch string) bool {
	in := false
	for _, p := range patterns {
		if neg, ok := strings.CutPrefix(p, "!"); ok {
			if globMatch(neg, branch) {
				in = false
			}
			continue
		}
		if globMatch(p, branch) {
			in = true
		}
	}
	return in
}

// ignores applies a branches-ignore filter: any matching pattern ignores
// the branch.
func ignores(patterns []string, branch string) bool {
	for _, p := range patterns {
		if globMatch(p, branch) {
			return true
		}
	}
	return false
}

// globMatch matches a GitHub filter pattern: * matches any characters but
// /, ** any characters, ? and + repeat the preceding character, [ ]
// is a character class, and \ escapes.
func globMatch(pattern, s string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?', '+':
			b.WriteByte(c)
		case '[':
			end := strings.IndexByte(pattern[i:], ']')
			if end < 0 {
				b.WriteString(`\[`)
				continue
			}
			b.WriteString(pattern[i : i+end+1])
			i += end
		case '\\':
			if i+1 < len(pattern) {
				b.WriteString(regexp.QuoteMeta(pattern[i+1 : i+2]))
				i++
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(s)
}

// defaultBranch is the branch C-01, C-04 and R-01 read the triggers for:
// the target of refs/remotes/origin/HEAD (read once, by OpenRepo), or
// else main or master: the one more of the workflows' pull_request and
// push triggers count for, main on a tie. In that case the report says
// which it assumed. parsed is ciRead's list, the same for every rule, so
// the answer is decided once per audit.
func defaultBranch(env *Env, parsed []*Workflow) string {
	if env.defaultBranch == "" {
		env.defaultBranch = decideDefaultBranch(env, parsed)
	}
	return env.defaultBranch
}

func decideDefaultBranch(env *Env, parsed []*Workflow) string {
	if b := env.Repo.originHead; b != "" {
		return b
	}
	branch, best := fallbackBranches[0], -1
	for _, b := range fallbackBranches {
		if n := countTriggered(parsed, b); n > best {
			branch, best = b, n
		}
	}
	noteAssumedBranch(env, branch)
	return branch
}

// noteAssumedBranch tells the report which default branch the CI rules
// assumed.
func noteAssumedBranch(env *Env, branch string) {
	env.Note("the clone has no refs/remotes/origin/HEAD, so the CI rules assume the default branch is %s", branch)
}

// countTriggered counts the workflow triggers (pull_request or push) that
// count for branch.
func countTriggered(parsed []*Workflow, branch string) int {
	n := 0
	for _, wf := range parsed {
		for _, ev := range []string{eventPullRequest, eventPush} {
			if triggeredFor(wf, ev, branch) {
				n++
			}
		}
	}
	return n
}

// ciJob is a job with the workflow file that defines it.
type ciJob struct {
	wf  *Workflow
	job *Job
}

// where names a job for a finding.
func (j ciJob) where() string { return "job " + j.job.ID }

// jobsRun returns the jobs a workflow runs: its own, with each job that
// uses a local reusable workflow (./.github/workflows/…) replaced by the
// jobs of that workflow, followed recursively. A remote reusable workflow
// is not followed; its job has no steps and calls nothing.
func jobsRun(parsed []*Workflow, wf *Workflow) []ciJob {
	byFile := map[string]*Workflow{}
	for _, w := range parsed {
		byFile[w.File] = w
	}
	seen := map[string]bool{}
	var out []ciJob
	var walk func(w *Workflow)
	walk = func(w *Workflow) {
		if seen[w.File] {
			return
		}
		seen[w.File] = true
		for _, j := range w.Jobs {
			if callee, ok := localWorkflow(j.Uses); ok {
				if cw := byFile[callee]; cw != nil {
					walk(cw)
				}
				continue
			}
			out = append(out, ciJob{w, j})
		}
	}
	walk(wf)
	return out
}

// localWorkflow returns the repository path of a local reusable workflow
// reference (./.github/workflows/x.yml).
func localWorkflow(uses string) (string, bool) {
	rest, ok := strings.CutPrefix(uses, "./")
	if !ok {
		return "", false
	}
	p := path.Clean(rest)
	return p, path.Dir(p) == workflowDir
}

// jobsTriggered returns, once each, the jobs run by the workflows that
// count for event on branch, reusable workflows followed with the
// caller's triggers.
func jobsTriggered(parsed []*Workflow, event, branch string) []ciJob {
	seen := map[*Job]bool{}
	var out []ciJob
	for _, wf := range parsed {
		if !triggeredFor(wf, event, branch) {
			continue
		}
		for _, j := range jobsRun(parsed, wf) {
			if !seen[j.job] {
				seen[j.job] = true
				out = append(out, j)
			}
		}
	}
	return out
}

// commandsOf returns every simple command of the jobs' run: steps.
func commandsOf(jobs []ciJob) []Command {
	var out []Command
	for _, j := range jobs {
		out = append(out, j.job.Commands()...)
	}
	return out
}
