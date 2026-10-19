package contract

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// The CI model the C rules, D-03 and R-01 read, whichever provider the
// repository uses (spec 0055, "Which provider is read (Q1)"). env.CI()
// picks the provider once per audit: GitHub Actions when tracked
// .github/workflows/*.yml files exist, GitLab CI when there are none and
// .gitlab-ci.yml is tracked. Other providers cannot be decided, and no CI
// at all fails.
//
// The GitHub checks keep reading CI.Workflows, so that their results and
// wording stay those of contract 1.0.1. The GitLab checks read the
// provider-neutral parts: Jobs, Run and Includes.

// CIProvider is the CI provider the C rules read. The values are the
// --json tokens.
type CIProvider string

// The providers vibe reads.
const (
	CIGitHub CIProvider = "github_actions"
	CIGitLab CIProvider = "gitlab_ci"
)

// Name is the provider as the text report and findings word it.
func (p CIProvider) Name() string {
	switch p {
	case CIGitHub:
		return "GitHub Actions"
	case CIGitLab:
		return "GitLab CI"
	}
	return string(p)
}

// PipelineKind is one of the two questions C-01 and C-04 ask of a job:
// does it run for merge (pull) requests, and does it run on a push to the
// default branch?
type PipelineKind int

// The pipeline kinds.
const (
	// KindMergeRequest is a merge request (GitHub: pull request). On
	// GitLab a branch pipeline of the request's source branch counts too:
	// GitLab shows it on the merge request.
	KindMergeRequest PipelineKind = iota
	// KindDefaultPush is a push to the default branch.
	KindDefaultPush
)

// CI is the repository's CI as the rules read it.
type CI struct {
	Provider CIProvider
	// Dir is the place a finding about the CI as a whole names:
	// .github/workflows/ or .gitlab-ci.yml.
	Dir string
	// Broken holds a failure for each file that does not parse or resolve.
	// Every rule that reads the CI fails with them.
	Broken []Finding
	// Jobs is every job read, whatever its triggers or rules. On GitLab:
	// every job of .gitlab-ci.yml with every local include followed
	// (include rules ignored), and of each child pipeline the parent waits
	// for; templates (.name) are not jobs.
	Jobs []*CIJob
	// Includes are the GitLab includes not followed (project:, remote:,
	// component:, template:, artifact:), top-level and in trigger:include.
	Includes []CIInclude
	// Workflows are the GitHub workflows that parsed.
	Workflows []*Workflow

	runs map[PipelineKind]*CIRun
	// gh maps a GitHub job to its CIJob.
	gh map[*Job]*CIJob
}

// CIJob is one job.
type CIJob struct {
	// File defines the job.
	File string
	ID   string
	// Line is the job's line in File, from 1.
	Line int
	// Child is the GitLab child pipeline's files the job belongs to,
	// joined by ", ", or "" for the main pipeline.
	Child string
	// Node is the job as the provider runs it. On GitLab it is resolved:
	// extends:, default: and !reference applied, so image:, services:,
	// tags: and parallel: can be read from it.
	Node *yaml.Node
	// Commands are the commands that count as calling an entrypoint:
	// GitHub run: steps; GitLab before_script and script.
	Commands []Command
	// Scripts are every script of the job, with where each is: GitLab
	// after_script included (Counts false).
	Scripts []CIScript
	// Images are the GitLab job's image: and services: entries, after
	// resolution (default: included).
	Images []CIImage

	key string
}

// CIImage is one GitLab image: or services: entry.
type CIImage struct {
	File string
	Line int
	Key  CIImageKey
	// Name is the image reference, or "" when the entry has none that is a
	// string.
	Name string
}

// CIImageKey is the GitLab key an image entry is under.
type CIImageKey string

// The keys that name images.
const (
	ImageKey    CIImageKey = "image"
	ServicesKey CIImageKey = "services"
)

// Key identifies a job, whichever provider read it: a job of a CIRun has
// the key of the same job in CI.Jobs, although on GitLab the run holds
// its own copy, resolved for that pipeline kind.
func (j *CIJob) Key() string { return j.key }

// githubJobKey and gitlabJobKey are the keys of the two providers' jobs.
// A GitHub job ID is unique in its workflow file; a GitLab job name in
// its pipeline, the main one or a child.
func githubJobKey(file, id string) string  { return file + "\x00" + id }
func gitlabJobKey(child, id string) string { return child + "\x00" + id }

// where names a job for a finding.
func (j *CIJob) where() string {
	if j.Child != "" {
		return "job " + j.ID + " (child pipeline " + j.Child + ")"
	}
	return "job " + j.ID
}

// CIScript is one script of a job: a GitHub run: step, or one GitLab
// before_script, script or after_script line.
type CIScript struct {
	File string
	// Line is the script's first line in File, from 1.
	Line int
	// Where names it for a finding, such as "job test, step 2" or
	// "job test, script".
	Where string
	// Counts is false for a script whose failure does not fail the job
	// (GitLab after_script): it does not count as calling verify.
	Counts bool
	Script
}

// CIInclude is a GitLab include the audit does not follow.
type CIInclude struct {
	File string
	Line int
	// Kind is project, remote, component, template or artifact.
	Kind CIIncludeKind
	// Node is the include entry: a string or a map.
	Node *yaml.Node
}

// CIIncludeKind is the kind of a GitLab include: the key that names
// what it includes.
type CIIncludeKind string

// The include kinds GitLab reads, local first; only local ones are
// followed.
const (
	IncludeLocal     CIIncludeKind = "local"
	IncludeProject   CIIncludeKind = "project"
	IncludeRemote    CIIncludeKind = "remote"
	IncludeTemplate  CIIncludeKind = "template"
	IncludeComponent CIIncludeKind = "component"
	IncludeArtifact  CIIncludeKind = "artifact"
)

// CIRun is the jobs that run for one pipeline kind.
type CIRun struct {
	Jobs []*CIJob
	// Skipped are the jobs that do not run for the kind, each with why:
	// on GitLab the rule, condition or keyword that left it out.
	Skipped []CISkip
}

// CISkip is a job that does not run for a pipeline kind.
type CISkip struct {
	Job *CIJob
	// File and Line locate Why: the condition the finding quotes.
	File string
	Line int
	Why  string
}

// Run returns the jobs that run for kind. GitHub follows local reusable
// workflows with the caller's triggers; GitLab evaluates workflow:rules,
// rules: and only/except, and follows child pipelines the parent waits
// for, with the trigger job's rules as their outer condition.
func (ci *CI) Run(env *Env, kind PipelineKind) *CIRun {
	if r := ci.runs[kind]; r != nil {
		return r
	}
	// GitLab runs are decided when the configuration is read, for every
	// kind; GitHub ones here, on first use, as contract 1.0.1 did.
	if ci.Provider != CIGitHub || (kind != KindMergeRequest && kind != KindDefaultPush) {
		panic(fmt.Sprintf("contract: no %s run for pipeline kind %d", ci.Provider.Name(), kind))
	}
	event := eventPullRequest
	if kind == KindDefaultPush {
		event = eventPush
	}
	r := &CIRun{}
	for _, j := range jobsTriggered(ci.Workflows, event, defaultBranch(env, ci.Workflows)) {
		r.Jobs = append(r.Jobs, ci.gh[j.job])
	}
	if ci.runs == nil {
		ci.runs = map[PipelineKind]*CIRun{}
	}
	ci.runs[kind] = r
	return r
}

// ciState is env.CI()'s answer once worked out.
type ciState struct {
	ci  *CI
	res PartResult
	ok  bool
}

// CI reads the repository's CI once per audit. When ok is false there is
// no CI the audit reads and result is what a rule that needs it decides:
// cannot decide when another provider's CI is tracked, fail when there is
// none (spec 0055, Q1).
func (e *Env) CI() (ci *CI, result PartResult, ok bool) {
	if e.ci == nil {
		e.ci = e.readCI()
	}
	return e.ci.ci, e.ci.res, e.ci.ok
}

// gitLabCI reports whether the C rules read GitLab CI, with the model.
// Rules outside C that read differently on GitLab (C-03's applicability,
// D-02) ask it.
func (e *Env) gitLabCI() (*CI, bool) {
	if ci, _, ok := e.CI(); ok && ci.Provider == CIGitLab {
		return ci, true
	}
	return nil, false
}

func (e *Env) readCI() *ciState {
	ws := e.Workflows()
	if len(ws.Files) > 0 {
		return &ciState{ci: githubCI(ws), res: Pass(), ok: true}
	}
	if e.Repo.HasFile(gitlabCIFile) {
		return &ciState{ci: readGitLabCI(e), res: Pass(), ok: true}
	}
	if len(ws.OtherCI) > 0 {
		return &ciState{res: Undecided(Finding{
			File: ws.OtherCI[0],
			Message: fmt.Sprintf("contract %s reads GitHub Actions and GitLab CI only, and this repository's CI is %s",
				Implemented, joinWords(ws.OtherCI, "and")),
			Fix: "run CI in GitHub Actions under .github/workflows/ or in GitLab CI (.gitlab-ci.yml), or wait for a vibe that reads this provider",
		})}
	}
	return &ciState{res: Fail(Finding{
		File:    workflowDir + "/",
		Message: "the repository has no CI configuration",
		Fix:     "add a GitHub Actions workflow under .github/workflows/ or a GitLab CI configuration (.gitlab-ci.yml)",
	})}
}

// githubCI is the model of the GitHub workflows.
func githubCI(ws *Workflows) *CI {
	ci := &CI{Provider: CIGitHub, Dir: workflowDir + "/", gh: map[*Job]*CIJob{}}
	for _, wf := range ws.Files {
		if wf.Err != nil {
			ci.Broken = append(ci.Broken, ParseFailure(wf.File, wf.Err))
			continue
		}
		ci.Workflows = append(ci.Workflows, wf)
		for _, j := range wf.Jobs {
			cj := &CIJob{File: wf.File, ID: j.ID, Line: j.Node.Line, Node: j.Node, Commands: j.Commands(),
				key: githubJobKey(wf.File, j.ID)}
			for _, s := range j.Steps {
				if s.HasRun {
					cj.Scripts = append(cj.Scripts, CIScript{File: wf.File, Line: s.Line,
						Where: fmt.Sprintf("job %s, step %d", j.ID, s.Index+1), Counts: true, Script: s.Run})
				}
			}
			ci.Jobs = append(ci.Jobs, cj)
			ci.gh[j] = cj
		}
	}
	return ci
}

// commandsOfJobs returns every command of the jobs that counts as calling.
func commandsOfJobs(jobs []*CIJob) []Command {
	var out []Command
	for _, j := range jobs {
		out = append(out, j.Commands...)
	}
	return out
}
