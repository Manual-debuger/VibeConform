package contract

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
	"gopkg.in/yaml.v3"
)

// The C rules and D-03 on GitLab CI (spec 0055, "The rules on GitLab
// CI"). C-01 and C-04 read the jobs that run for a pipeline kind, and the
// jobs skipped with the condition that left them out; C-02, C-05 and D-03
// read every job and the includes the audit does not follow. C-03 is not
// applicable (rules.go); R-01's CI part is in vibe_calls.go.

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

// pinnedDigest is an image reference pinned by digest.
var pinnedDigest = regexp.MustCompile(`@sha256:[0-9a-fA-F]{64}$`)

// fullSHA is a full 40-character commit SHA.
var fullSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// gitlabC02: every image: and services: entry of every job is pinned by
// digest, every include the audit does not follow is pinned (a template:
// include cannot be), and no script word ends in @latest. Every job
// counts, whatever its rules.
func gitlabC02(ci *CI) PartResult {
	var findings []Finding
	type image struct {
		img  CIImage
		jobs []string
	}
	var images []*image
	byKey := map[string]*image{}
	for _, j := range ci.Jobs {
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
	for _, im := range images {
		name := im.img.Name
		if name == "" {
			name = "(no image name)"
		}
		findings = append(findings, Finding{File: im.img.File,
			Message: fmt.Sprintf("line %d: %s %s is not pinned by digest (used by %s)", im.img.Line, im.img.Key, name, strings.Join(im.jobs, ", ")),
			Fix:     "pin it by digest, keeping the tag for readers (name:tag@sha256:<64 hexadecimal characters>)"})
	}
	for _, inc := range ci.Includes {
		if f, ok := includeFinding(inc); !ok {
			findings = append(findings, f)
		}
	}
	seen := map[string]bool{}
	for _, j := range ci.Jobs {
		for _, s := range j.Scripts {
			for _, w := range s.Words() {
				if !strings.HasSuffix(w, "@latest") {
					continue
				}
				if k := fmt.Sprintf("%s\x00%d\x00%s", s.File, s.Line, w); !seen[k] {
					seen[k] = true
					w = guardrule.ShowExpansions(w)
					findings = append(findings, Finding{File: s.File,
						Message: fmt.Sprintf("line %d: %s runs %s, which is not pinned", s.Line, s.Where, w),
						Fix:     "pin " + strings.TrimSuffix(w, "@latest") + " to a version (@v1.2.3) instead of @latest"})
				}
			}
		}
	}
	return failIf(findings)
}

// includeFinding decides whether an include the audit does not follow is
// pinned, and the finding when it is not.
func includeFinding(inc CIInclude) (Finding, bool) {
	at := func(format string, args ...any) string {
		return fmt.Sprintf("line %d: include ", inc.Line) + fmt.Sprintf(format, args...)
	}
	value := func(key string) string { return scalarOr(mapValue(inc.Node, key), "") }
	switch inc.Kind {
	case "project":
		ref := value("ref")
		if fullSHA.MatchString(ref) {
			return Finding{}, true
		}
		if ref == "" {
			ref = "(no ref: the default branch)"
		}
		return Finding{File: inc.File,
			Message: at("project %s is not pinned to a commit SHA: ref is %s", value("project"), ref),
			Fix:     "set ref: to a full 40-character commit SHA, with the version in a comment (ref: <sha> # v1.2.3)"}, false
	case "component":
		c := value("component")
		if i := strings.LastIndex(c, "@"); i >= 0 && fullSHA.MatchString(c[i+1:]) {
			return Finding{}, true
		}
		return Finding{File: inc.File,
			Message: at("component %s is not pinned to a commit SHA", c),
			Fix:     "pin the component to a full 40-character commit SHA, with the version in a comment (component: <path>@<sha> # 1.2.0)"}, false
	case "remote":
		url := inc.Node.Value
		if inc.Node.Kind == yaml.MappingNode {
			url = value("remote")
			if value("integrity") != "" {
				return Finding{}, true
			}
		}
		return Finding{File: inc.File,
			Message: at("remote %s has no integrity:", url),
			Fix:     "add integrity: sha256-<base64 of the file's SHA-256 digest>, or copy the file into the repository and include it with local:"}, false
	case "template":
		return Finding{File: inc.File,
			Message: at("template %s cannot be pinned: GitLab serves the version of the instance", value("template")),
			Fix:     "copy the template into the repository and include it with local:, or include the matching CI/CD component pinned to a commit SHA"}, false
	}
	// A dynamic child pipeline (artifact:) is generated by the pipeline
	// itself: there is nothing to pin.
	return Finding{}, true
}

// gitlabC05: each verify command is called by a job that runs on a Linux
// runner and by one that runs on a Windows runner, after parallel: matrix
// expansion, among every job.
func gitlabC05(ci *CI, verify []Command) PartResult {
	var findings []Finding
	for _, c := range verify {
		var platforms, unknown, where []string
		file := gitlabCIFile
		for _, j := range ci.Jobs {
			if !CallsAny(j.Commands, c) {
				continue
			}
			if len(where) == 0 {
				file = j.File
			}
			where = append(where, j.File+" "+j.where())
			ps, us := gitlabPlatforms(j)
			for _, p := range ps {
				if !slices.Contains(platforms, p) {
					platforms = append(platforms, p)
				}
			}
			unknown = append(unknown, us...)
		}
		if len(where) == 0 {
			findings = append(findings, Finding{File: file,
				Message: fmt.Sprintf("no job calls the verify command `%s`, so it runs on no runner", c),
				Fix:     fmt.Sprintf("call `%s` in a job on a Linux runner (tags: [linux], or no tags and an image:) and in one with tags: [windows]", c)})
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
			Fix: fmt.Sprintf("run `%s` on %s too: in a job whose tags name a runner of that platform, such as tags: [windows] "+
				"(saas-windows-medium-amd64 on GitLab.com), or with a parallel: matrix over the tags", c, joinWords(lacking, "and"))})
	}
	return failIf(findings)
}

// glVariable is a variable in a tag: $NAME or ${NAME}.
var glVariable = regexp.MustCompile(`\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))`)

// gitlabPlatforms returns the platforms a job runs on, one per parallel:
// matrix combination: a tag containing windows, linux or macos (ignoring
// case) names it, and a job with no tags and an image: is Linux. unknown
// describes each combination that names no platform.
func gitlabPlatforms(j *CIJob) (platforms, unknown []string) {
	tags := mapValue(j.Node, "tags")
	image := mapValue(j.Node, "image") != nil
	for _, combo := range parallelCombos(mapValue(j.Node, "parallel")) {
		var named, words []string
		if tags != nil && tags.Kind == yaml.SequenceNode {
			for _, t := range tags.Content {
				tag := glVariable.ReplaceAllStringFunc(scalarOr(t, ""), func(v string) string {
					m := glVariable.FindStringSubmatch(v)
					if val, ok := combo[m[1]+m[2]]; ok {
						return val
					}
					return v
				})
				words = append(words, tag)
				l := strings.ToLower(tag)
				for _, p := range []string{PlatformWindows, PlatformLinux, PlatformMacOS} {
					if strings.Contains(l, p) && !slices.Contains(named, p) {
						named = append(named, p)
					}
				}
			}
		}
		switch {
		case len(named) > 0:
			platforms = append(platforms, named...)
		case len(words) == 0 && tags != nil && tags.Kind != yaml.SequenceNode:
			unknown = append(unknown, j.where()+": tags that are not a list")
		case len(words) == 0 && image:
			platforms = append(platforms, PlatformLinux)
		case len(words) == 0:
			unknown = append(unknown, j.where()+": no tags and no image")
		default:
			unknown = append(unknown, j.where()+": tags ["+strings.Join(words, ", ")+"]")
		}
	}
	return platforms, unknown
}

// parallelCombos expands parallel: matrix: each entry is the product of
// its variables' values. Without a matrix there is one combination with
// no values.
func parallelCombos(parallel *yaml.Node) []map[string]string {
	matrix := mapValue(parallel, "matrix")
	if matrix == nil || matrix.Kind != yaml.SequenceNode {
		return []map[string]string{{}}
	}
	var out []map[string]string
	for _, entry := range matrix.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		combos := []map[string]string{{}}
		for i := 0; i+1 < len(entry.Content); i += 2 {
			name := entry.Content[i].Value
			var next []map[string]string
			for _, v := range listItems(entry.Content[i+1]) {
				for _, c := range combos {
					n := maps.Clone(c)
					n[name] = scalarOr(v, "")
					next = append(next, n)
				}
			}
			combos = next
		}
		out = append(out, combos...)
	}
	if len(out) == 0 {
		return []map[string]string{{}}
	}
	return out
}

// dependencyScanningTemplate is GitLab's Dependency-Scanning template.
var dependencyScanningTemplate = regexp.MustCompile(`^(Jobs|Security)/Dependency-Scanning[^/]*\.gitlab-ci\.yml$`)

// gitlabD03: some job's script (after_script included) runs a recognized
// scanner, or the pipeline includes GitLab's Dependency-Scanning template
// or dependency-scanning component.
func gitlabD03(ci *CI) PartResult {
	for _, j := range ci.Jobs {
		for _, s := range j.Scripts {
			for _, c := range s.Commands {
				for _, sc := range scanners {
					if c.HasPrefix(sc...) {
						return Pass()
					}
				}
			}
		}
	}
	for _, inc := range ci.Includes {
		v := scalarOr(mapValue(inc.Node, inc.Kind), "")
		switch inc.Kind {
		case "template":
			if dependencyScanningTemplate.MatchString(v) {
				return Pass()
			}
		case "component":
			path, _, _ := strings.Cut(v, "@")
			if slices.Contains(strings.Split(path, "/"), "dependency-scanning") {
				return Pass()
			}
		}
	}
	return Fail(Finding{File: gitlabCIFile,
		Message: "no job runs a recognized vulnerability scanner (govulncheck, pip-audit, pnpm audit, npm audit, osv-scanner or trivy), " +
			"and the pipeline includes neither GitLab's Dependency-Scanning template nor its dependency-scanning component",
		Fix: "add a job whose script runs the scanner for the component's language, such as govulncheck ./... for Go, pip-audit for Python " +
			"or pnpm audit for TypeScript, or include the dependency-scanning CI/CD component pinned to a commit SHA"})
}
