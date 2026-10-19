package contract

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
	"gopkg.in/yaml.v3"
)

// The static checks that read CI: C-01 to C-05, D-03 and R-01's static
// part (spec 0052, "Recognition rules"; spec 0055 for GitLab CI). Each
// starts from ciRead, which applies env.CI() and turns every CI file that
// does not parse or resolve into a failure of the rule; the rule then
// reads the rest. GitHub triggers and the default branch are in
// workflow_triggers.go, runners and matrices in workflow_runners.go; the
// GitLab pipeline model is in gitlab_ci.go and gitlab_rules.go.

// ciRead returns the CI for a rule that reads it. When ok is false the
// rule's result is res: no CI the audit reads. When ok is true, res holds
// a failure for each file that does not parse (or passes), and the rule
// merges its own result into it.
func ciRead(env *Env) (ci *CI, res PartResult, ok bool) {
	ci, res, ok = env.CI()
	if !ok {
		return nil, res, false
	}
	return ci, failIf(ci.Broken), true
}

// gitlabNotRead is the result of a rule this vibe does not read on GitLab
// CI yet.
func gitlabNotRead(res PartResult) PartResult {
	return Merge(res, Undecided(Finding{File: gitlabCIFile,
		Message: "this vibe does not read GitLab CI for this rule yet",
		Fix:     "wait for a vibe that reads it"}))
}

// checkC03: every workflow has top-level permissions of read-all, {} or a
// map whose every value is read or none.
func checkC03(env *Env) PartResult {
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	if ci.Provider == CIGitLab {
		return gitlabNotRead(res)
	}
	parsed := ci.Workflows
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
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	if ci.Provider == CIGitLab {
		return Merge(res, gitlabC01(env, ci, verify))
	}
	parsed := ci.Workflows
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

// pinnedRef is a uses: reference pinned to a full commit SHA.
var pinnedRef = regexp.MustCompile(`@[0-9a-fA-F]{40}$`)

// checkC02: every job-level and step-level uses: that is not a local ./
// path ends in @ and 40 hexadecimal characters, and no word of a run:
// step ends in @latest, in every workflow.
func checkC02(env *Env) PartResult {
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	if ci.Provider == CIGitLab {
		return gitlabNotRead(res)
	}
	parsed := ci.Workflows
	const pinFix = "pin it to a full 40-character commit SHA, with the version in a comment (uses: owner/repo@<sha> # v1.2.3)"
	var findings []Finding
	for _, wf := range parsed {
		for _, j := range wf.Jobs {
			if j.Uses != "" && !strings.HasPrefix(j.Uses, "./") && !pinnedRef.MatchString(j.Uses) {
				findings = append(findings, Finding{File: wf.File,
					Message: fmt.Sprintf("line %d: job %s uses %s, which is not pinned to a commit SHA", j.UsesLine, j.ID, j.Uses),
					Fix:     pinFix})
			}
			for _, s := range j.Steps {
				if s.Uses != "" && !strings.HasPrefix(s.Uses, "./") && !pinnedRef.MatchString(s.Uses) {
					findings = append(findings, Finding{File: wf.File,
						Message: fmt.Sprintf("line %d: job %s, step %d uses %s, which is not pinned to a commit SHA", s.Line, j.ID, s.Index+1, s.Uses),
						Fix:     pinFix})
				}
				if !s.HasRun {
					continue
				}
				for _, w := range s.Run.Words() {
					if strings.HasSuffix(w, "@latest") {
						w = guardrule.ShowExpansions(w)
						findings = append(findings, Finding{File: wf.File,
							Message: fmt.Sprintf("line %d: job %s, step %d runs %s, which is not pinned", s.Line, j.ID, s.Index+1, w),
							Fix:     "pin " + strings.TrimSuffix(w, "@latest") + " to a version (@v1.2.3) instead of @latest"})
					}
				}
			}
		}
	}
	return Merge(res, failIf(findings))
}

// checkC04: a job of a workflow that counts for pull requests (reusable
// workflows followed) has a step whose simple command starts with the
// words vibe audit, and none of its steps calls a verify command.
func checkC04(env *Env) PartResult {
	verify, missing := env.Entrypoint(EntrypointVerify)
	if missing != nil {
		return Fail(*missing)
	}
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	if ci.Provider == CIGitLab {
		return Merge(res, gitlabC04(env, ci, verify))
	}
	parsed := ci.Workflows
	branch := defaultBranch(env, parsed)
	if len(auditJobs(parsed, branch, verify)) > 0 {
		return res
	}
	var findings []Finding
	for _, j := range jobsTriggered(parsed, eventPullRequest, branch) {
		if !runsAudit(j.job) {
			continue
		}
		for _, c := range verify {
			if CallsAny(j.job.Commands(), c) {
				findings = append(findings, Finding{File: j.wf.File,
					Message: fmt.Sprintf("%s runs vibe audit and also calls the verify command `%s`", j.where(), c),
					Fix:     "run vibe audit in a job of its own that does not call verify"})
			}
		}
	}
	if len(findings) == 0 {
		findings = append(findings, Finding{File: workflowDir + "/",
			Message: "no job of a workflow that runs on pull requests runs vibe audit" + triggerHint,
			Fix:     "add a job, triggered on pull_request, whose step runs vibe audit (any arguments) and that does not call verify"})
	}
	return Merge(res, Fail(findings...))
}

// auditJobs returns the jobs that satisfy C-04: run by a workflow that
// counts for pull requests, with a step that runs vibe audit and no step
// that calls a verify command.
func auditJobs(parsed []*Workflow, branch string, verify []Command) []ciJob {
	var out []ciJob
	for _, j := range jobsTriggered(parsed, eventPullRequest, branch) {
		if !runsAudit(j.job) {
			continue
		}
		calls := false
		for _, c := range verify {
			calls = calls || CallsAny(j.job.Commands(), c)
		}
		if !calls {
			out = append(out, j)
		}
	}
	return out
}

// runsAudit reports whether a job has a simple command that starts with
// the words vibe audit.
func runsAudit(j *Job) bool {
	for _, c := range j.Commands() {
		if c.HasPrefix("vibe", "audit") {
			return true
		}
	}
	return false
}

// checkC05: each verify command is called by a job that runs on a Linux
// runner and by one that runs on a Windows runner, after matrix
// expansion, in any workflow. The rule is not applicable without windows
// in platforms; the registry decides that before it runs.
func checkC05(env *Env) PartResult {
	verify, missing := env.Entrypoint(EntrypointVerify)
	if missing != nil {
		return Fail(*missing)
	}
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	if ci.Provider == CIGitLab {
		return gitlabNotRead(res)
	}
	parsed := ci.Workflows
	var findings []Finding
	for _, c := range verify {
		var platforms, unknown, where []string
		file := workflowDir + "/"
		for _, wf := range parsed {
			for _, j := range wf.Jobs {
				if !CallsAny(j.Commands(), c) {
					continue
				}
				if len(where) == 0 {
					file = wf.File
				}
				where = append(where, wf.File+" job "+j.ID)
				ps, us := runnerPlatforms(j)
				for _, p := range ps {
					if !slices.Contains(platforms, p) {
						platforms = append(platforms, p)
					}
				}
				unknown = append(unknown, us...)
			}
		}
		if len(where) == 0 {
			findings = append(findings, Finding{File: file,
				Message: fmt.Sprintf("no workflow step calls the verify command `%s`, so it runs on no runner", c),
				Fix:     fmt.Sprintf("call `%s` in a job that runs on ubuntu-latest and windows-latest (a matrix over os)", c)})
			continue
		}
		var lacking []string
		for _, p := range []string{PlatformLinux, PlatformWindows} {
			if !slices.Contains(platforms, p) {
				lacking = append(lacking, platformName(p))
			}
		}
		if len(lacking) == 0 {
			continue
		}
		msg := fmt.Sprintf("the verify command `%s` runs on no %s runner (called in %s)",
			c, joinWords(lacking, "or"), strings.Join(where, ", "))
		if len(unknown) > 0 {
			msg += "; runners of unknown platform: " + strings.Join(unknown, "; ")
		}
		findings = append(findings, Finding{File: file, Message: msg,
			Fix: fmt.Sprintf("run `%s` on %s too, such as with a matrix over os: [ubuntu-latest, windows-latest] and runs-on: ${{ matrix.os }}",
				c, joinWords(lacking, "and"))})
	}
	return Merge(res, failIf(findings))
}

func platformName(p string) string {
	switch p {
	case PlatformLinux:
		return "Linux"
	case PlatformWindows:
		return "Windows"
	}
	return "macOS"
}

// scanners are the first words, or first two words, of the commands D-03
// recognizes as vulnerability scanners.
var scanners = [][]string{
	{"govulncheck"}, {"pip-audit"}, {"osv-scanner"}, {"trivy"}, {"pnpm", "audit"}, {"npm", "audit"},
}

// checkD03: some workflow step runs a recognized scanner.
func checkD03(env *Env) PartResult {
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	if ci.Provider == CIGitLab {
		return gitlabNotRead(res)
	}
	parsed := ci.Workflows
	for _, wf := range parsed {
		for _, j := range wf.Jobs {
			for _, c := range j.Commands() {
				for _, s := range scanners {
					if c.HasPrefix(s...) {
						return res
					}
				}
			}
		}
	}
	return Merge(res, Fail(Finding{File: workflowDir + "/",
		Message: "no workflow step runs a recognized vulnerability scanner (govulncheck, pip-audit, pnpm audit, npm audit, osv-scanner or trivy)",
		Fix:     "add a CI step that runs the scanner for the component's language, such as govulncheck ./... for Go, pip-audit for Python or pnpm audit for TypeScript"}))
}

// checkR01 is R-01's static part: no entrypoint, hook command or CI step
// has a simple command whose first word is vibe, except in a job that
// satisfies C-04. Without GitHub workflows only the entrypoints and hook
// commands are read; a workflow that does not parse fails it. GitLab CI
// is not read here yet: only its entrypoints and hook commands are.
func checkR01(env *Env) PartResult {
	calls := entrypointVibeCalls(env)
	hookCalls, res := hookVibeCalls(env)
	calls = append(calls, hookCalls...)
	if ci, ciRes, ok := ciRead(env); ok && ci.Provider == CIGitHub {
		parsed := ci.Workflows
		res = Merge(res, ciRes)
		// With verify missing (V-04 fails), any vibe audit job of a pull
		// request workflow is the audit job.
		verify, _ := env.Entrypoint(EntrypointVerify)
		calls = append(calls, ciVibeCalls(parsed, auditJobs(parsed, defaultBranch(env, parsed), verify))...)
	}
	var findings []Finding
	for _, c := range calls {
		findings = append(findings, Finding{File: c.file,
			Message: fmt.Sprintf("%s: `%s` calls vibe, so it does not work without vibe", c.where, c.cmd),
			Fix:     "call the underlying tools directly instead of vibe; only the C-04 audit job (vibe audit, no verify) may call vibe"})
	}
	return Merge(res, failIf(findings))
}
