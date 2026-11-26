package contract

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// The CI model the C rules, D-03 and R-01 read, whichever provider the
// repository uses (spec 0055, "Which provider is read (Q1)"). env.ciModel()
// picks the provider once per audit: GitHub Actions when tracked
// .github/workflows/*.yml files exist, GitLab CI when there are none and
// .gitlab-ci.yml is tracked. Other providers cannot be decided, and no CI
// at all fails.
//
// Each reader fills the whole model when the CI is read: every job with
// its commands, scripts and runner platforms, the default branch, and
// the run of each pipeline kind. The GitHub reader adds each job's uses:
// references and the parsed workflows; the GitLab reader each job's
// images and the includes it does not follow. Each reader also sets its provider's
// wording (ci_wording.go): the words of findings that differ by provider.
//
// C-01, C-02, C-04, C-05 and D-03 each have one implementation
// (checks_ci.go) that reads only the model and the wording, and none
// compares the provider (spec 0067). C-03 reads the GitHub workflows
// (ciModel.Workflows), which no other provider has; the registry makes it not
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

// pipelineKind is one of the two questions C-01 and C-04 ask of a job:
// does it run for merge (pull) requests, and does it run on a push to the
// default branch?
type pipelineKind int

// The pipeline kinds.
const (
	// kindMergeRequest is a merge request (GitHub: pull request). On
	// GitLab a branch pipeline of the request's source branch counts too:
	// GitLab shows it on the merge request.
	kindMergeRequest pipelineKind = iota
	// kindDefaultPush is a push to the default branch.
	kindDefaultPush
)

// ciModel is the repository's ciModel as the rules read it.
type ciModel struct {
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
	Jobs []*ciJob
	// Includes are the GitLab includes not followed (project:, remote:,
	// component:, template:, artifact:), top-level and in trigger:include.
	Includes []ciInclude
	// Workflows are the GitHub workflows that parsed.
	Workflows []*workflow
	// DefaultBranch is the branch the reader decided the runs for: the
	// target of refs/remotes/origin/HEAD, or else main or master, as the
	// provider's configuration suggests.
	DefaultBranch string

	// runs are the jobs that run for each pipeline kind, as the reader
	// decided them.
	runs map[pipelineKind]*ciRun
	// noteOnRun, when set, has the report note the default branch the
	// reader assumed, at the first Run. The GitHub reader sets it; the
	// GitLab reader notes the branch when it reads the configuration.
	noteOnRun func()
	// wording is the provider's words for findings (ci_wording.go).
	wording *ciWording
}

// ciJob is one job.
type ciJob struct {
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
	Commands []shellCommand
	// Scripts are every script of the job, with where each is: GitLab
	// after_script included (Counts false).
	Scripts []ciScript
	// Images are the GitLab job's image: and services: entries, after
	// resolution (default: included).
	Images []ciImage
	// Uses are the GitHub job's uses: references: the job's own (a
	// reusable workflow), then each step's (an action), in order. Local
	// ./ references are kept.
	Uses []ciUses
	// Platforms are the platforms the job runs on (platformLinux,
	// platformWindows, platformMacOS), for each matrix combination that
	// names one: GitHub runs-on with strategy: matrix expanded, GitLab
	// tags: with parallel: matrix expanded and the image rule.
	Platforms []string
	// UnknownRunners describe the runners whose platform is unknown, as
	// C-05 quotes them: on GitHub the runs-on labels as written, on GitLab
	// the job and its tags.
	UnknownRunners []string

	key string
}

// ciUses is one GitHub uses: reference.
type ciUses struct {
	File string
	// Line is the job's uses: line, or the step's line, from 1.
	Line int
	// Where names it for a finding: "job build" or "job build, step 2".
	Where string
	// Ref is the reference as written, such as actions/checkout@v4 or
	// ./.github/workflows/test.yml.
	Ref string
}

// ciImage is one GitLab image: or services: entry.
type ciImage struct {
	File string
	Line int
	Key  ciImageKey
	// Name is the image reference, or "" when the entry has none that is a
	// string.
	Name string
}

// ciImageKey is the GitLab key an image entry is under.
type ciImageKey string

// The keys that name images.
const (
	imageKey    ciImageKey = "image"
	servicesKey ciImageKey = "services"
)

// Key identifies a job, whichever provider read it: a job of a ciRun has
// the key of the same job in ciModel.Jobs, although on GitLab the run holds
// its own copy, resolved for that pipeline kind.
func (j *ciJob) Key() string { return j.key }

// githubJobKey and gitlabJobKey are the keys of the two providers' jobs.
// A GitHub job ID is unique in its workflow file; a GitLab job name in
// its pipeline, the main one or a child.
func githubJobKey(file, id string) string  { return file + "\x00" + id }
func gitlabJobKey(child, id string) string { return child + "\x00" + id }

// where names a job for a finding.
func (j *ciJob) where() string {
	if j.Child != "" {
		return "job " + j.ID + " (child pipeline " + j.Child + ")"
	}
	return "job " + j.ID
}

// ciScript is one script of a job: a GitHub run: step, or one GitLab
// before_script, script or after_script line.
type ciScript struct {
	File string
	// Line is the script's first line in File, from 1.
	Line int
	// Where names it for a finding, such as "job test, step 2" or
	// "job test, script".
	Where string
	// Counts is false for a script whose failure does not fail the job
	// (GitLab after_script): it does not count as calling verify.
	Counts bool
	shellScript
}

// ciInclude is a GitLab include the audit does not follow.
type ciInclude struct {
	File string
	Line int
	// Kind is project, remote, component, template or artifact.
	Kind ciIncludeKind
	// Node is the include entry: a string or a map.
	Node *yaml.Node
}

// Target is the value under the include's kind key: the project,
// component, remote URL or template. It is "" when the entry is not a map
// or that value is not a string.
func (inc ciInclude) Target() string {
	return scalarOr(mapValue(inc.Node, string(inc.Kind)), "")
}

// ciIncludeKind is the kind of a GitLab include: the key that names
// what it includes.
type ciIncludeKind string

// The include kinds GitLab reads, local first; only local ones are
// followed.
const (
	includeLocal     ciIncludeKind = "local"
	includeProject   ciIncludeKind = "project"
	includeRemote    ciIncludeKind = "remote"
	includeTemplate  ciIncludeKind = "template"
	includeComponent ciIncludeKind = "component"
	includeArtifact  ciIncludeKind = "artifact"
)

// ciRun is the jobs that run for one pipeline kind.
type ciRun struct {
	Jobs []*ciJob
	// Skipped are the jobs that do not run for the kind, each with why:
	// on GitLab the rule, condition or keyword that left it out.
	Skipped []ciSkip
}

// ciSkip is a job that does not run for a pipeline kind.
type ciSkip struct {
	Job *ciJob
	// File and Line locate Why: the condition the finding quotes.
	File string
	Line int
	Why  string
	// Fix is the fix of a finding about the job: what makes it run for
	// the kind. Only the GitLab reader skips jobs; it words the fix.
	Fix string
}

// Run returns the jobs that run for kind, as the reader decided them when
// it read the CI. GitHub follows local reusable workflows with the
// caller's triggers; GitLab evaluates workflow:rules, rules: and
// only/except, and follows child pipelines the parent waits for, with the
// trigger job's rules as their outer condition. Both readers decide every
// pipeline kind, for ciModel.DefaultBranch. The first call makes the note the
// reader left for it, if any (noteOnRun).
func (ci *ciModel) Run(kind pipelineKind) *ciRun {
	if ci.noteOnRun != nil {
		ci.noteOnRun()
		ci.noteOnRun = nil
	}
	r := ci.runs[kind]
	if r == nil {
		panic(fmt.Sprintf("contract: no %s run for pipeline kind %d", ci.Provider.Name(), kind))
	}
	return r
}

// ciState is env.ciModel()'s answer once worked out.
type ciState struct {
	ci  *ciModel
	res PartResult
	ok  bool
}

// ciModel reads the repository's ciModel once per audit. When ok is false there is
// no ciModel the audit reads and result is what a rule that needs it decides:
// cannot decide when another provider's ciModel is tracked, fail when there is
// none (spec 0055, Q1).
func (e *Env) ciModel() (ci *ciModel, result PartResult, ok bool) {
	if e.ci == nil {
		e.ci = e.readCI()
	}
	return e.ci.ci, e.ci.res, e.ci.ok
}

// gitLabCI reports whether the C rules read GitLab CI, with the model.
// Rules outside C that read differently on GitLab (C-03's applicability,
// D-02) ask it.
func (e *Env) gitLabCI() (*ciModel, bool) {
	if ci, _, ok := e.ciModel(); ok && ci.Provider == CIGitLab {
		return ci, true
	}
	return nil, false
}

func (e *Env) readCI() *ciState {
	ws := e.githubWorkflows()
	if len(ws.Files) > 0 {
		return &ciState{ci: githubCI(e, ws), res: Pass(), ok: true}
	}
	if e.repo.HasFile(gitlabCIFile) {
		return &ciState{ci: readGitLabCI(e), res: Pass(), ok: true}
	}
	if len(ws.OtherCI) > 0 {
		return &ciState{res: undecided(Finding{
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
func githubCI(env *Env, ws *workflowSet) *ciModel {
	ci := &ciModel{Provider: CIGitHub, Dir: workflowDir + "/", wording: githubWording}
	for _, wf := range ws.Files {
		if wf.Err != nil {
			ci.Broken = append(ci.Broken, parseFailure(wf.File, wf.Err))
			continue
		}
		ci.Workflows = append(ci.Workflows, wf)
		for _, j := range wf.Jobs {
			ci.Jobs = append(ci.Jobs, githubJob(wf, j))
		}
	}
	byKey := map[string]*ciJob{}
	for _, j := range ci.Jobs {
		byKey[j.Key()] = j
	}
	// GitHub notes an assumed branch when a rule first reads the runs
	// (C-01, C-04 or R-01), as contract 1.0.1 did.
	branch, assumed := githubDefaultBranch(env, ci.Workflows)
	if assumed {
		ci.noteOnRun = func() { noteAssumedBranch(env, branch) }
	}
	ci.DefaultBranch = branch
	ci.runs = map[pipelineKind]*ciRun{}
	for _, k := range []struct {
		kind  pipelineKind
		event string
	}{{kindMergeRequest, eventPullRequest}, {kindDefaultPush, eventPush}} {
		r := &ciRun{}
		for _, j := range jobsTriggered(ci.Workflows, k.event, branch) {
			r.Jobs = append(r.Jobs, byKey[githubJobKey(j.wf.File, j.job.ID)])
		}
		ci.runs[k.kind] = r
	}
	return ci
}

// githubJob is the model of one workflow job: its run: steps, its uses:
// references and its runner platforms.
func githubJob(wf *workflow, j *workflowJob) *ciJob {
	cj := &ciJob{File: wf.File, ID: j.ID, Line: j.Node.Line, Node: j.Node, Commands: j.Commands(),
		key: githubJobKey(wf.File, j.ID)}
	if j.Uses != "" {
		cj.Uses = append(cj.Uses, ciUses{File: wf.File, Line: j.UsesLine, Where: cj.where(), Ref: j.Uses})
	}
	for _, s := range j.Steps {
		where := fmt.Sprintf("%s, step %d", cj.where(), s.Index+1)
		if s.Uses != "" {
			cj.Uses = append(cj.Uses, ciUses{File: wf.File, Line: s.Line, Where: where, Ref: s.Uses})
		}
		if s.HasRun {
			cj.Scripts = append(cj.Scripts, ciScript{File: wf.File, Line: s.Line, Where: where, Counts: true, shellScript: s.Run})
		}
	}
	cj.Platforms, cj.UnknownRunners = runnerPlatforms(j)
	return cj
}

// commandsOfJobs returns every command of the jobs that counts as calling.
func commandsOfJobs(jobs []*ciJob) []shellCommand {
	var out []shellCommand
	for _, j := range jobs {
		out = append(out, j.Commands...)
	}
	return out
}
