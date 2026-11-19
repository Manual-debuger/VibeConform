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
// Each reader fills the whole model when the CI is read: every job with
// its commands, scripts and runner platforms, and the run of each
// pipeline kind. The GitHub reader adds each job's uses: references and
// the parsed workflows; the GitLab reader each job's images and the
// includes it does not follow. Each reader also sets its provider's
// wording (ci_wording.go): the words of findings that differ by provider.
//
// C-01, C-02, C-04, C-05 and D-03 each have one implementation
// (checks_ci.go) that reads only the model and the wording, and none
// compares the provider (spec 0067). C-03 reads the GitHub workflows
// (CI.Workflows), which no other provider has; the registry makes it not
// applicable on GitLab CI. R-01's CI part (vibe_calls.go) reads the model
// too.

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

	// runs are the jobs that run for each pipeline kind, as the reader
	// decided them.
	runs map[PipelineKind]*CIRun
	// wording is the provider's words for findings (ci_wording.go).
	wording *ciWording
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
	// Uses are the GitHub job's uses: references: the job's own (a
	// reusable workflow), then each step's (an action), in order. Local
	// ./ references are kept.
	Uses []CIUses
	// Platforms are the platforms the job runs on (PlatformLinux,
	// PlatformWindows, PlatformMacOS), for each matrix combination that
	// names one: GitHub runs-on with strategy: matrix expanded, GitLab
	// tags: with parallel: matrix expanded and the image rule.
	Platforms []string
	// UnknownRunners describe the runners whose platform is unknown, as
	// C-05 quotes them: on GitHub the runs-on labels as written, on GitLab
	// the job and its tags.
	UnknownRunners []string

	key string
}

// CIUses is one GitHub uses: reference.
type CIUses struct {
	File string
	// Line is the job's uses: line, or the step's line, from 1.
	Line int
	// Where names it for a finding: "job build" or "job build, step 2".
	Where string
	// Ref is the reference as written, such as actions/checkout@v4 or
	// ./.github/workflows/test.yml.
	Ref string
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

// Run returns the jobs that run for kind, as the reader decided them when
// it read the CI. GitHub follows local reusable workflows with the
// caller's triggers; GitLab evaluates workflow:rules, rules: and
// only/except, and follows child pipelines the parent waits for, with the
// trigger job's rules as their outer condition. Both readers decide every
// pipeline kind.
func (ci *CI) Run(kind PipelineKind) *CIRun {
	r := ci.runs[kind]
	if r == nil {
		panic(fmt.Sprintf("contract: no %s run for pipeline kind %d", ci.Provider.Name(), kind))
	}
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
		return &ciState{ci: githubCI(e, ws), res: Pass(), ok: true}
	}
	if e.Repo.HasFile(gitlabCIFile) {
		return &ciState{ci: readGitLabCI(e), res: Pass(), ok: true}
	}
	if len(ws.OtherCI) > 0 {
		return &ciState{res: Undecided(Finding{
			File: ws.OtherCI[0],
			Message: fmt.Sprintf("contract %s reads GitHub Actions and GitLab CI only, and this repository's CI is %s",
				Implemented, joinWords(ws.OtherCI, " and ")),
			Fix: "run CI in GitHub Actions under .github/workflows/ or in GitLab CI (.gitlab-ci.yml), or wait for a vibe that reads this provider",
		})}
	}
	return &ciState{res: Fail(Finding{
		File:    workflowDir + "/",
		Message: "the repository has no CI configuration",
		Fix:     "add a GitHub Actions workflow under .github/workflows/ or a GitLab CI configuration (.gitlab-ci.yml)",
	})}
}

// githubCI is the model of the GitHub workflows. A run is the jobs the
// trigger walk finds for its event on the default branch; a GitHub run
// has no skipped jobs.
func githubCI(env *Env, ws *Workflows) *CI {
	ci := &CI{Provider: CIGitHub, Dir: workflowDir + "/", wording: githubWording}
	for _, wf := range ws.Files {
		if wf.Err != nil {
			ci.Broken = append(ci.Broken, ParseFailure(wf.File, wf.Err))
			continue
		}
		ci.Workflows = append(ci.Workflows, wf)
		for _, j := range wf.Jobs {
			ci.Jobs = append(ci.Jobs, githubJob(wf, j))
		}
	}
	byKey := map[string]*CIJob{}
	for _, j := range ci.Jobs {
		byKey[j.Key()] = j
	}
	branch := defaultBranch(env, ci.Workflows)
	ci.runs = map[PipelineKind]*CIRun{}
	for _, k := range []struct {
		kind  PipelineKind
		event string
	}{{KindMergeRequest, eventPullRequest}, {KindDefaultPush, eventPush}} {
		r := &CIRun{}
		for _, j := range jobsTriggered(ci.Workflows, k.event, branch) {
			r.Jobs = append(r.Jobs, byKey[githubJobKey(j.wf.File, j.job.ID)])
		}
		ci.runs[k.kind] = r
	}
	return ci
}

// githubJob is the model of one workflow job: its run: steps, its uses:
// references and its runner platforms.
func githubJob(wf *Workflow, j *Job) *CIJob {
	cj := &CIJob{File: wf.File, ID: j.ID, Line: j.Node.Line, Node: j.Node, Commands: j.Commands(),
		key: githubJobKey(wf.File, j.ID)}
	if j.Uses != "" {
		cj.Uses = append(cj.Uses, CIUses{File: wf.File, Line: j.UsesLine, Where: cj.where(), Ref: j.Uses})
	}
	for _, s := range j.Steps {
		where := fmt.Sprintf("%s, step %d", cj.where(), s.Index+1)
		if s.Uses != "" {
			cj.Uses = append(cj.Uses, CIUses{File: wf.File, Line: s.Line, Where: where, Ref: s.Uses})
		}
		if s.HasRun {
			cj.Scripts = append(cj.Scripts, CIScript{File: wf.File, Line: s.Line, Where: where, Counts: true, Script: s.Run})
		}
	}
	cj.Platforms, cj.UnknownRunners = runnerPlatforms(j)
	return cj
}

// commandsOfJobs returns every command of the jobs that counts as calling.
func commandsOfJobs(jobs []*CIJob) []Command {
	var out []Command
	for _, j := range jobs {
		out = append(out, j.Commands...)
	}
	return out
}
