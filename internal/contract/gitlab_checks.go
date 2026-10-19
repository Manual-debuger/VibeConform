package contract

import "fmt"

// C-01 and C-04 on GitLab CI (spec 0055, "The rules on GitLab CI"). They
// read the neutral model: the jobs that run for a pipeline kind, and the
// jobs skipped with the condition that left them out.

// gitlabHint says what makes a GitLab job not count, for findings.
const gitlabHint = " (workflow:rules, rules: or only/except that leave the pipeline out, changes: or exists:, " +
	"and conditions the audit does not read make a job not count)"

// gitlabKinds are the two pipeline kinds C-01 reads, as findings word them.
func gitlabKinds(env *Env) []struct {
	kind PipelineKind
	what string
} {
	return []struct {
		kind PipelineKind
		what string
	}{
		{KindMergeRequest, "merge request pipelines"},
		{KindDefaultPush, "pushes to " + env.defaultBranch},
	}
}

// skipFinding explains why a job that would count did not.
func skipFinding(s CISkip, what string) Finding {
	return Finding{File: s.File,
		Message: fmt.Sprintf("line %d: %s does not count for %s: %s", s.Line, s.Job.where(), what, s.Why),
		Fix: "make the job run in " + what + " with conditions the audit reads " +
			"($CI_PIPELINE_SOURCE, $CI_MERGE_REQUEST_IID, $CI_COMMIT_BRANCH, $CI_COMMIT_TAG, $CI_COMMIT_REF_NAME, $CI_DEFAULT_BRANCH), " +
			"without changes: or exists:"}
}

// gitlabC01: jobs that run in merge request pipelines, and jobs that run
// on pushes to the default branch, each call every verify command.
func gitlabC01(env *Env, ci *CI, verify []Command) PartResult {
	var findings []Finding
	for _, k := range gitlabKinds(env) {
		run := ci.Run(env, k.kind)
		explained := map[*CIJob]bool{}
		explain := func(c Command) {
			for _, s := range run.Skipped {
				if !explained[s.Job] && CallsAny(s.Job.Commands, c) {
					explained[s.Job] = true
					findings = append(findings, skipFinding(s, k.what))
				}
			}
		}
		if len(run.Jobs) == 0 {
			findings = append(findings, Finding{File: gitlabCIFile,
				Message: "no job runs in " + k.what + gitlabHint,
				Fix:     fmt.Sprintf("add a job that runs in %s and calls %s", k.what, quoteCommandList(verify))})
			for _, c := range verify {
				explain(c)
			}
			continue
		}
		for _, c := range Uncalled(verify, commandsOfJobs(run.Jobs)) {
			findings = append(findings, Finding{File: gitlabCIFile,
				Message: fmt.Sprintf("no job that runs in %s calls the verify command `%s`", k.what, c),
				Fix:     fmt.Sprintf("add `%s` as its own command to the script of a job that runs in %s", c, k.what)})
			explain(c)
		}
	}
	return failIf(findings)
}

// ciAuditJobs returns the jobs of a run that satisfy C-04: they run vibe
// audit (a simple command that counts as calling, starting with the words
// vibe audit) and call no verify command.
func ciAuditJobs(run *CIRun, verify []Command) []*CIJob {
	var out []*CIJob
	for _, j := range run.Jobs {
		if !runsAuditCommand(j.Commands) {
			continue
		}
		calls := false
		for _, c := range verify {
			calls = calls || CallsAny(j.Commands, c)
		}
		if !calls {
			out = append(out, j)
		}
	}
	return out
}

func runsAuditCommand(cmds []Command) bool {
	for _, c := range cmds {
		if c.HasPrefix("vibe", "audit") {
			return true
		}
	}
	return false
}

// gitlabC04: a job that runs in merge request pipelines runs vibe audit
// and calls no verify command.
func gitlabC04(env *Env, ci *CI, verify []Command) PartResult {
	const what = "merge request pipelines"
	run := ci.Run(env, KindMergeRequest)
	if len(ciAuditJobs(run, verify)) > 0 {
		return Pass()
	}
	var findings []Finding
	for _, j := range run.Jobs {
		if !runsAuditCommand(j.Commands) {
			continue
		}
		for _, c := range verify {
			if CallsAny(j.Commands, c) {
				findings = append(findings, Finding{File: j.File,
					Message: fmt.Sprintf("%s runs vibe audit and also calls the verify command `%s`", j.where(), c),
					Fix:     "run vibe audit in a job of its own that does not call verify"})
			}
		}
	}
	for _, s := range run.Skipped {
		if runsAuditCommand(s.Job.Commands) {
			findings = append(findings, skipFinding(s, what))
		}
	}
	if len(findings) == 0 {
		findings = append(findings, Finding{File: gitlabCIFile,
			Message: "no job that runs in " + what + " runs vibe audit" + gitlabHint,
			Fix:     "add a job that runs in merge request pipelines, whose script runs vibe audit (any arguments) and that calls no verify command"})
	}
	return Fail(findings...)
}
