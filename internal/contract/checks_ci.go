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
// reads the rest. The readers fill the model (ci_model.go): GitHub
// triggers and the default branch are in workflow_triggers.go, runners
// and matrices in workflow_runners.go; the GitLab pipeline model is in
// gitlab_ci.go and gitlab_rules.go, its runners in gitlab_runners.go.

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

// checkC03: every workflow has top-level permissions of read-all, {} or a
// map whose every value is read or none. It reads the GitHub workflows
// (CI.Workflows), which only the GitHub reader fills: on GitLab CI the
// rule is not applicable, which the registry decides before it runs
// (notOnGitLab).
func checkC03(env *Env) PartResult {
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	const fix = "set top-level `permissions: contents: read` (or `read-all`, or `{}`) and grant write scopes in the job that needs them"
	var findings []Finding
	for _, wf := range ci.Workflows {
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
					Message: fmt.Sprintf("line %d: top-level permissions grant %s", p.Line, joinWords(writes, " and ")),
					Fix:     "move " + joinWords(writes, " and ") + " into the permissions of the job that needs it; keep top-level scopes read or none"})
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

// checkC01: the jobs that run for merge (pull) requests, and the jobs
// that run on pushes to the default branch, each call every verify
// command. A skipped job that calls an uncalled command is explained.
func checkC01(env *Env) PartResult {
	verify, missing := env.Entrypoint(EntrypointVerify)
	if missing != nil {
		return Fail(*missing)
	}
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	w := ci.wording
	var findings []Finding
	for _, kind := range []PipelineKind{KindMergeRequest, KindDefaultPush} {
		what := w.kind(kind, ci.DefaultBranch)
		run := ci.Run(kind)
		explained := map[*CIJob]bool{}
		explain := func(c Command) {
			for _, s := range run.Skipped {
				if !explained[s.Job] && CallsAny(s.Job.Commands, c) {
					explained[s.Job] = true
					findings = append(findings, skipFinding(s, what))
				}
			}
		}
		if len(run.Jobs) == 0 {
			findings = append(findings, Finding{File: ci.Dir,
				Message: w.noJobRuns(what) + w.hint,
				Fix:     w.noJobRunsFix(what, quoteCommandList(verify))})
			for _, c := range verify {
				explain(c)
			}
			continue
		}
		for _, c := range Uncalled(verify, commandsOfJobs(run.Jobs)) {
			findings = append(findings, Finding{File: ci.Dir,
				Message: w.notCalled(what, c),
				Fix:     w.notCalledFix(what, c)})
			explain(c)
		}
	}
	return Merge(res, failIf(findings))
}

// skipFinding explains why a job that would count for what did not.
func skipFinding(s CISkip, what string) Finding {
	return Finding{File: s.File,
		Message: fmt.Sprintf("line %d: %s does not count for %s: %s", s.Line, s.Job.where(), what, s.Why),
		Fix:     s.Fix}
}

// quoteCommandList quotes an entrypoint's commands for a message.
func quoteCommandList(cmds []Command) string {
	q := make([]string, len(cmds))
	for i, c := range cmds {
		q[i] = "`" + c.String() + "`"
	}
	return joinWords(q, " and ")
}

// pinnedRef is a uses: reference pinned to a full commit SHA.
var pinnedRef = regexp.MustCompile(`@[0-9a-fA-F]{40}$`)

// checkC02: every reference the CI pins is pinned, and no script runs a
// word ending in @latest. It reads, in this order: the uses: references
// (a full 40-character commit SHA, except a local ./ path), the images
// (pinned by digest), the includes the audit does not follow (a
// template: include cannot be pinned) and the @latest words of every
// script. Every job counts, whatever its triggers or rules.
func checkC02(env *Env) PartResult {
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	findings := unpinnedUses(ci.Jobs)
	findings = append(findings, unpinnedImages(ci.Jobs)...)
	for _, inc := range ci.Includes {
		if f, ok := unpinnedInclude(inc); ok {
			findings = append(findings, f)
		}
	}
	findings = append(findings, latestWords(ci.Jobs)...)
	return Merge(res, failIf(findings))
}

// unpinnedUses are C-02's findings for the uses: references that are
// neither a local ./ path nor pinned to a full commit SHA.
func unpinnedUses(jobs []*CIJob) []Finding {
	const fix = "pin it to a full 40-character commit SHA, with the version in a comment (uses: owner/repo@<sha> # v1.2.3)"
	var findings []Finding
	for _, j := range jobs {
		for _, u := range j.Uses {
			if strings.HasPrefix(u.Ref, "./") || pinnedRef.MatchString(u.Ref) {
				continue
			}
			findings = append(findings, Finding{File: u.File,
				Message: fmt.Sprintf("line %d: %s uses %s, which is not pinned to a commit SHA", u.Line, u.Where, u.Ref),
				Fix:     fix})
		}
	}
	return findings
}

// pinnedDigest is an image reference pinned by digest.
var pinnedDigest = regexp.MustCompile(`@sha256:[0-9a-fA-F]{64}$`)

// unpinnedImages are C-02's findings for the images not pinned by digest:
// one for each image entry, which names every job that uses it.
func unpinnedImages(jobs []*CIJob) []Finding {
	type image struct {
		img  CIImage
		jobs []string
	}
	var images []*image
	byKey := map[string]*image{}
	for _, j := range jobs {
		for _, img := range j.Images {
			if pinnedDigest.MatchString(img.Name) {
				continue
			}
			k := fmt.Sprintf("%s\x00%d\x00%s\x00%s", img.File, img.Line, img.Key, img.Name)
			if byKey[k] == nil {
				byKey[k] = &image{img: img}
				images = append(images, byKey[k])
			}
			byKey[k].jobs = append(byKey[k].jobs, j.where())
		}
	}
	findings := make([]Finding, 0, len(images))
	for _, im := range images {
		name := im.img.Name
		if name == "" {
			name = "(no image name)"
		}
		findings = append(findings, Finding{File: im.img.File,
			Message: fmt.Sprintf("line %d: %s %s is not pinned by digest (used by %s)", im.img.Line, im.img.Key, name, strings.Join(im.jobs, ", ")),
			Fix:     "pin it by digest, keeping the tag for readers (name:tag@sha256:<64 hexadecimal characters>)"})
	}
	return findings
}

// latestWords are C-02's findings for the script words that end in
// @latest: one for each file, line and word.
func latestWords(jobs []*CIJob) []Finding {
	var findings []Finding
	seen := map[string]bool{}
	for _, j := range jobs {
		for _, s := range j.Scripts {
			for _, w := range s.Words() {
				k := fmt.Sprintf("%s\x00%d\x00%s", s.File, s.Line, w)
				if !strings.HasSuffix(w, "@latest") || seen[k] {
					continue
				}
				seen[k] = true
				w = guardrule.ShowExpansions(w)
				findings = append(findings, Finding{File: s.File,
					Message: fmt.Sprintf("line %d: %s runs %s, which is not pinned", s.Line, s.Where, w),
					Fix:     "pin " + strings.TrimSuffix(w, "@latest") + " to a version (@v1.2.3) instead of @latest"})
			}
		}
	}
	return findings
}

// checkC04: a job that runs for merge (pull) requests (reusable
// workflows and child pipelines followed) has a simple command that
// starts with the words vibe audit, and calls no verify command.
func checkC04(env *Env) PartResult {
	verify, missing := env.Entrypoint(EntrypointVerify)
	if missing != nil {
		return Fail(*missing)
	}
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	run := ci.Run(KindMergeRequest)
	if len(ciAuditJobs(run, verify)) > 0 {
		return res
	}
	w := ci.wording
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
			findings = append(findings, skipFinding(s, w.mergeRequests))
		}
	}
	if len(findings) == 0 {
		findings = append(findings, Finding{File: ci.Dir,
			Message: w.noAuditJob + w.hint,
			Fix:     w.noAuditJobFix})
	}
	return Merge(res, Fail(findings...))
}

// ciAuditJobs returns the jobs of a run that satisfy C-04: they run vibe
// audit (a simple command that counts as calling, starting with the words
// vibe audit) and call no verify command. R-01's CI part exempts them.
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

// checkC05: each verify command is called by a job that runs on a Linux
// runner and by one that runs on a Windows runner, after matrix
// expansion, among every job. The rule is not applicable without windows
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
	w := ci.wording
	var findings []Finding
	for _, c := range verify {
		r := runnersOf(ci, c)
		if len(r.where) == 0 {
			findings = append(findings, Finding{File: ci.Dir,
				Message: w.uncalledOnRunner(c),
				Fix:     w.uncalledOnRunnerFix(c)})
			continue
		}
		var lacking []string
		for _, p := range []string{PlatformLinux, PlatformWindows} {
			if !slices.Contains(r.platforms, p) {
				lacking = append(lacking, platformName(p))
			}
		}
		if len(lacking) == 0 {
			continue
		}
		msg := fmt.Sprintf("the verify command `%s` runs on no %s runner (called in %s)",
			c, joinWords(lacking, " or "), strings.Join(r.where, ", "))
		if len(r.unknown) > 0 {
			msg += "; runners of unknown platform: " + strings.Join(r.unknown, "; ")
		}
		findings = append(findings, Finding{File: r.file, Message: msg,
			Fix: w.lackingPlatformFix(c, joinWords(lacking, " and "))})
	}
	return Merge(res, failIf(findings))
}

// commandRunners are the runners of the jobs that call one command.
type commandRunners struct {
	// file defines the first job that calls it.
	file string
	// where names each job that calls it, with its file.
	where []string
	// platforms are the distinct platforms of those jobs; unknown are
	// their runners of unknown platform, as C-05 quotes them.
	platforms, unknown []string
}

// runnersOf returns the runners of every job that calls c.
func runnersOf(ci *CI, c Command) commandRunners {
	var r commandRunners
	for _, j := range ci.Jobs {
		if !CallsAny(j.Commands, c) {
			continue
		}
		if len(r.where) == 0 {
			r.file = j.File
		}
		r.where = append(r.where, j.File+" "+j.where())
		for _, p := range j.Platforms {
			if !slices.Contains(r.platforms, p) {
				r.platforms = append(r.platforms, p)
			}
		}
		r.unknown = append(r.unknown, j.UnknownRunners...)
	}
	return r
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

// checkD03: some job's script (GitLab after_script included) runs a
// recognized scanner, or the pipeline includes GitLab's
// Dependency-Scanning template or dependency-scanning component.
func checkD03(env *Env) PartResult {
	ci, res, ok := ciRead(env)
	if !ok {
		return res
	}
	if runsScanner(ci) || includesDependencyScanning(ci) {
		return res
	}
	return Merge(res, Fail(Finding{File: ci.Dir, Message: ci.wording.noScanner, Fix: ci.wording.noScannerFix}))
}

// runsScanner reports whether a script of a job runs a recognized scanner.
func runsScanner(ci *CI) bool {
	for _, j := range ci.Jobs {
		for _, s := range j.Scripts {
			for _, c := range s.Commands {
				if slices.ContainsFunc(scanners, func(sc []string) bool { return c.HasPrefix(sc...) }) {
					return true
				}
			}
		}
	}
	return false
}

// dependencyScanningTemplate is GitLab's Dependency-Scanning template.
var dependencyScanningTemplate = regexp.MustCompile(`^(Jobs|Security)/Dependency-Scanning[^/]*\.gitlab-ci\.yml$`)

// includesDependencyScanning reports whether the pipeline includes
// GitLab's Dependency-Scanning template or its dependency-scanning
// component.
func includesDependencyScanning(ci *CI) bool {
	for _, inc := range ci.Includes {
		v := inc.Target()
		switch inc.Kind {
		case IncludeTemplate:
			if dependencyScanningTemplate.MatchString(v) {
				return true
			}
		case IncludeComponent:
			path, _, _ := strings.Cut(v, "@")
			if slices.Contains(strings.Split(path, "/"), "dependency-scanning") {
				return true
			}
		}
	}
	return false
}

// checkR01 is R-01's static part: no entrypoint, hook command or CI step
// has a simple command whose first word is vibe, except in a job that
// satisfies C-04, and except vibe --version in a CI job that calls no
// entrypoint (spec 0055). On GitLab CI every job's before_script, script
// and after_script are read. Without CI only the entrypoints and hook
// commands are read; a CI file that does not parse fails it.
func checkR01(env *Env) PartResult {
	calls := entrypointVibeCalls(env)
	hookCalls, res := hookVibeCalls(env)
	calls = append(calls, hookCalls...)
	if ci, ciRes, ok := ciRead(env); ok {
		res = Merge(res, ciRes)
		// With verify missing (V-04 fails), any vibe audit job that runs
		// for pull (merge) requests is the audit job.
		verify, _ := env.Entrypoint(EntrypointVerify)
		calls = append(calls, ciVibeCalls(env, ci, ciAuditJobs(ci.Run(KindMergeRequest), verify))...)
	}
	var findings []Finding
	for _, c := range calls {
		findings = append(findings, Finding{File: c.file,
			Message: fmt.Sprintf("%s: `%s` calls vibe, so it does not work without vibe", c.where, c.cmd),
			Fix:     "call the underlying tools directly instead of vibe; only the C-04 audit job (vibe audit, no verify) may call vibe"})
	}
	return Merge(res, failIf(findings))
}
