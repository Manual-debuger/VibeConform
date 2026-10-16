package contract

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The static checks that read GitHub Actions workflows: C-01 to C-05,
// D-03 and R-01's static part (spec 0052, "Recognition rules"). Each
// starts from ciRead, which applies env.GitHubCI() and turns every
// workflow that does not parse into a failure of the rule; the rule then
// reads the workflows that parsed. Triggers and the default branch are in
// workflow_triggers.go, runners and matrices in workflow_runners.go.

// ciRead returns the parsed workflows for a rule that reads GitHub
// Actions. When ok is false the rule's result is res: no GitHub CI. When
// ok is true, res holds a failure for each workflow that does not parse
// (or passes), and the rule merges its own result into it.
func ciRead(env *Env) (parsed []*Workflow, ws *Workflows, res PartResult, ok bool) {
	ws, res, ok = env.GitHubCI()
	if !ok {
		return nil, ws, res, false
	}
	var broken []Finding
	for _, wf := range ws.Files {
		if wf.Err != nil {
			broken = append(broken, ParseFailure(wf.File, wf.Err))
			continue
		}
		parsed = append(parsed, wf)
	}
	if len(broken) > 0 {
		return parsed, ws, Fail(broken...), true
	}
	return parsed, ws, Pass(), true
}

// checkC03: every workflow has top-level permissions of read-all, {} or a
// map whose every value is read or none.
func checkC03(env *Env) PartResult {
	parsed, _, res, ok := ciRead(env)
	if !ok {
		return res
	}
	const fix = "set top-level `permissions: contents: read` (or `read-all`, or `{}`) and grant write scopes in the job that needs them"
	var findings []Finding
	for _, wf := range parsed {
		p := wf.Permissions
		switch {
		case p == nil:
			findings = append(findings, Finding{File: wf.File,
				Message: "has no top-level permissions, so it gets the repository's default, which may be write",
				Fix:     fix})
		case p.Kind == yaml.ScalarNode:
			if p.Value != "read-all" {
				findings = append(findings, Finding{File: wf.File,
					Message: fmt.Sprintf("line %d: top-level permissions are %s, not read-all", p.Line, p.Value),
					Fix:     fix})
			}
		case p.Kind == yaml.MappingNode:
			var writes []string
			for i := 0; i+1 < len(p.Content); i += 2 {
				v := resolve(p.Content[i+1])
				if v.Kind != yaml.ScalarNode || (v.Value != "read" && v.Value != "none") {
					writes = append(writes, p.Content[i].Value+": "+scalarOr(v, "(not a scope level)"))
				}
			}
			if len(writes) > 0 {
				findings = append(findings, Finding{File: wf.File,
					Message: fmt.Sprintf("line %d: top-level permissions grant %s", p.Line, joinWords(writes, "and")),
					Fix:     "move " + joinWords(writes, "and") + " into the permissions of the job that needs it; keep top-level scopes read or none"})
			}
		default:
			findings = append(findings, Finding{File: wf.File,
				Message: fmt.Sprintf("line %d: top-level permissions are neither a map nor read-all", p.Line),
				Fix:     fix})
		}
	}
	return Merge(res, failIf(findings))
}

// failIf fails with findings, or passes when there are none.
func failIf(findings []Finding) PartResult {
	if len(findings) > 0 {
		return Fail(findings...)
	}
	return Pass()
}

// scalarOr returns a scalar node's value, or alt for any other node.
func scalarOr(n *yaml.Node, alt string) string {
	if n != nil && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return alt
}

// checkC01: the workflows that count for pull requests, and those that
// count for pushes to the default branch, each call every verify command.
func checkC01(env *Env) PartResult {
	verify, missing := env.Entrypoint(EntrypointVerify)
	if missing != nil {
		return Fail(*missing)
	}
	parsed, _, res, ok := ciRead(env)
	if !ok {
		return res
	}
	branch := defaultBranch(env, parsed)
	var findings []Finding
	for _, ev := range []struct{ event, what string }{
		{eventPullRequest, "pull requests"},
		{eventPush, "pushes to " + branch},
	} {
		jobs := jobsTriggered(parsed, ev.event, branch)
		if len(jobs) == 0 {
			findings = append(findings, Finding{File: workflowDir + "/",
				Message: "no workflow runs on " + ev.what + triggerHint,
				Fix:     fmt.Sprintf("trigger a workflow that calls %s on %s", quoteCommandList(verify), ev.what)})
			continue
		}
		for _, c := range Uncalled(verify, commandsOf(jobs)) {
			findings = append(findings, Finding{File: workflowDir + "/",
				Message: fmt.Sprintf("no workflow that runs on %s calls the verify command `%s`", ev.what, c),
				Fix:     fmt.Sprintf("add a step that runs `%s` as its own command to a job that runs on %s", c, ev.what)})
		}
	}
	return Merge(res, failIf(findings))
}

// triggerHint says what makes a workflow not count, for findings.
const triggerHint = " (a paths filter, a tags filter without branches, a types filter without opened, " +
	"synchronize and reopened, or branch filters that leave the branch out make a workflow not count)"

// quoteCommandList quotes an entrypoint's commands for a message.
func quoteCommandList(cmds []Command) string {
	q := make([]string, len(cmds))
	for i, c := range cmds {
		q[i] = "`" + c.String() + "`"
	}
	return joinWords(q, "and")
}

func checkC02(_ *Env) PartResult { return notImplementedYet() }

func checkC04(_ *Env) PartResult { return notImplementedYet() }

func checkC05(_ *Env) PartResult { return notImplementedYet() }

func checkD03(_ *Env) PartResult { return notImplementedYet() }

func checkR01(_ *Env) PartResult { return notImplementedYet() }

var (
	_ = slices.Contains[[]string]
	_ = strings.Contains
)
