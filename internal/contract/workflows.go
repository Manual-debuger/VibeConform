package contract

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// GitHub Actions as the C rules, D-03 and R-01 see it (spec 0052,
// "Recognition rules"). This file reads the workflows into jobs and
// steps. Triggers, branch filters, the default branch, reusable-workflow
// following and runner expansion belong to the C rules and build on the
// raw nodes kept here (Workflow.On, Job.RunsOn, Job.Strategy, Job.Uses).

// workflowDir holds the GitHub Actions workflows. GitHub reads only the
// files directly in it.
const workflowDir = ".github/workflows"

// otherCIFiles are the CI configurations of providers contract 1.0.0 does
// not read. A path ending in "/" is a directory: any tracked file in it
// counts.
var otherCIFiles = []string{
	".gitlab-ci.yml", ".circleci/config.yml", "Jenkinsfile", "azure-pipelines.yml",
	".travis.yml", "bitbucket-pipelines.yml", ".buildkite/", ".drone.yml",
}

// Workflows is the repository's CI configuration.
type Workflows struct {
	// Files is every tracked .github/workflows/*.yml and *.yaml, sorted by
	// path. A file that does not parse is kept with Err set.
	Files []*Workflow
	// OtherCI lists the tracked CI files of other providers, in
	// otherCIFiles order.
	OtherCI []string
}

// Workflow is one GitHub Actions workflow file.
type Workflow struct {
	File string
	// Err is set when the file does not parse as a workflow: invalid
	// YAML, or jobs and steps that are not the maps and lists GitHub
	// reads. Every rule that needs the file fails with ParseFailure.
	Err error
	// Root is the workflow's top-level mapping.
	Root *yaml.Node
	// On is the top-level on: value, or nil. Triggers are the C rules'
	// to read.
	On *yaml.Node
	// Permissions is the top-level permissions: value, or nil when the
	// workflow has none.
	Permissions *yaml.Node
	// Jobs are the workflow's jobs in file order.
	Jobs []*Job
}

// Job is one job of a workflow.
type Job struct {
	ID   string
	Node *yaml.Node
	// Name is the job's name:, or "".
	Name string
	// Uses is a reusable-workflow job's uses: (./.github/workflows/… for a
	// local one), or "". Such a job has no steps.
	Uses string
	// UsesLine is uses:'s line, from 1.
	UsesLine int
	// RunsOn and Strategy are the job's runs-on: and strategy: values, or
	// nil. Runner and matrix expansion are C-05's to read.
	RunsOn   *yaml.Node
	Strategy *yaml.Node
	Steps    []*Step
}

// Step is one step of a job.
type Step struct {
	// Index is the step's position in the job, from 0.
	Index int
	// Line is the step's line in the workflow file, from 1.
	Line int
	Node *yaml.Node
	// Uses is the step's uses: (an action reference), or "".
	Uses string
	// HasRun is true when the step has a run: script.
	HasRun bool
	// Run is the parsed run: script when HasRun.
	Run Script
}

// Commands returns every simple command of a job's run: steps, in order.
// A step that does not parse contributes none.
func (j *Job) Commands() []Command {
	var out []Command
	for _, s := range j.Steps {
		out = append(out, s.Run.Commands...)
	}
	return out
}

// Workflows reads the repository's GitHub Actions workflows once per
// audit.
func (e *Env) Workflows() *Workflows {
	if e.workflows != nil {
		return e.workflows
	}
	ws := &Workflows{}
	for _, f := range e.Repo.Files() {
		if path.Dir(f) == workflowDir && (strings.HasSuffix(f, ".yml") || strings.HasSuffix(f, ".yaml")) {
			data, err := e.ReadFile(f)
			wf := &Workflow{File: f}
			if err != nil {
				wf.Err = err
			} else {
				wf.Err = wf.parse(data)
			}
			ws.Files = append(ws.Files, wf)
		}
	}
	for _, other := range otherCIFiles {
		if dir, ok := strings.CutSuffix(other, "/"); ok {
			if files := e.Repo.FilesUnder(dir); len(files) > 0 {
				ws.OtherCI = append(ws.OtherCI, other)
			}
			continue
		}
		if e.Repo.HasFile(other) {
			ws.OtherCI = append(ws.OtherCI, other)
		}
	}
	e.workflows = ws
	return ws
}

// GitHubCI returns the workflows for a rule that reads GitHub Actions.
// When there are none, ok is false and result is what the rule decides:
// cannot decide when another provider's CI is tracked, fail when the
// repository has no CI at all (spec 0052, Q19).
func (e *Env) GitHubCI() (ws *Workflows, result PartResult, ok bool) {
	ws = e.Workflows()
	if len(ws.Files) > 0 {
		return ws, Pass(), true
	}
	if len(ws.OtherCI) > 0 {
		return ws, Undecided(Finding{
			File: ws.OtherCI[0],
			Message: fmt.Sprintf("contract %s reads GitHub Actions workflows only, and this repository's CI is %s",
				Implemented, joinWords(ws.OtherCI, "and")),
			Fix: "run CI in GitHub Actions under .github/workflows/, or wait for a vibe that reads this provider",
		}), false
	}
	return ws, Fail(Finding{
		File:    workflowDir + "/",
		Message: "the repository has no CI configuration",
		Fix:     "add a GitHub Actions workflow under .github/workflows/",
	}), false
}

func (wf *Workflow) parse(data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 {
		return errors.New("the file is empty")
	}
	root := resolve(doc.Content[0])
	if root.Kind != yaml.MappingNode {
		return errors.New("a workflow must be a map")
	}
	wf.Root = root
	wf.On = mapValue(root, "on")
	wf.Permissions = mapValue(root, "permissions")

	jobs := mapValue(root, "jobs")
	if jobs == nil {
		return nil
	}
	if jobs.Kind != yaml.MappingNode {
		return lineError(jobs, "jobs must be a map")
	}
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		job, err := parseJob(jobs.Content[i].Value, resolve(jobs.Content[i+1]))
		if err != nil {
			return err
		}
		wf.Jobs = append(wf.Jobs, job)
	}
	return nil
}

func parseJob(id string, n *yaml.Node) (*Job, error) {
	if n.Kind != yaml.MappingNode {
		return nil, lineError(n, "job "+id+" must be a map")
	}
	j := &Job{ID: id, Node: n, RunsOn: mapValue(n, "runs-on"), Strategy: mapValue(n, "strategy")}
	if v := mapValue(n, "name"); v != nil && v.Kind == yaml.ScalarNode {
		j.Name = v.Value
	}
	if v := mapValue(n, "uses"); v != nil {
		if v.Kind != yaml.ScalarNode {
			return nil, lineError(v, "job "+id+": uses must be a string")
		}
		j.Uses, j.UsesLine = v.Value, v.Line
	}
	steps := mapValue(n, "steps")
	if steps == nil {
		return j, nil
	}
	if steps.Kind != yaml.SequenceNode {
		return nil, lineError(steps, "job "+id+": steps must be a list")
	}
	for i, sn := range steps.Content {
		sn = resolve(sn)
		if sn.Kind != yaml.MappingNode {
			return nil, lineError(sn, fmt.Sprintf("job %s: step %d must be a map", id, i+1))
		}
		s := &Step{Index: i, Line: sn.Line, Node: sn}
		if v := mapValue(sn, "uses"); v != nil {
			if v.Kind != yaml.ScalarNode {
				return nil, lineError(v, fmt.Sprintf("job %s: step %d: uses must be a string", id, i+1))
			}
			s.Uses = v.Value
		}
		if v := mapValue(sn, "run"); v != nil {
			if v.Kind != yaml.ScalarNode {
				return nil, lineError(v, fmt.Sprintf("job %s: step %d: run must be a string", id, i+1))
			}
			s.HasRun = true
			s.Run = ParseRunStep(v.Value)
		}
		j.Steps = append(j.Steps, s)
	}
	return j, nil
}

// mapValue returns the value of key in a mapping node, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return resolve(m.Content[i+1])
		}
	}
	return nil
}

func lineError(n *yaml.Node, msg string) error {
	return fmt.Errorf("line %d: %s", n.Line, msg)
}
