package contract

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
	"gopkg.in/yaml.v3"
)

// The C rules and D-03 on GitLab CI (spec 0055, "The rules on GitLab
// CI"). C-01 and C-04 read the jobs that run for a pipeline kind, and the
// jobs skipped with the condition that left them out; C-02, C-05 and D-03
// read every job and the includes the audit does not follow. C-03 is not
// applicable (rules.go); R-01's CI part is in vibe_calls.go.

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

// pinnedDigest is an image reference pinned by digest.
var pinnedDigest = regexp.MustCompile(`@sha256:[0-9a-fA-F]{64}$`)

// fullSHA is a full 40-character commit SHA.
var fullSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// gitlabC02: every image: and services: entry of every job is pinned by
// digest, every include the audit does not follow is pinned (a template:
// include cannot be), and no script word ends in @latest. Every job
// counts, whatever its rules.
func gitlabC02(ci *CI) PartResult { //nolint:gocognit // baseline: pre-existing complexity
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
	// what is the value under the include's kind key: the project,
	// component, remote URL or template.
	what := value(string(inc.Kind))
	switch inc.Kind {
	case IncludeProject:
		ref := value("ref")
		if fullSHA.MatchString(ref) {
			return Finding{}, true
		}
		if ref == "" {
			ref = "(no ref: the default branch)"
		}
		return Finding{File: inc.File,
			Message: at("project %s is not pinned to a commit SHA: ref is %s", what, ref),
			Fix:     "set ref: to a full 40-character commit SHA, with the version in a comment (ref: <sha> # v1.2.3)"}, false
	case IncludeComponent:
		c := what
		if i := strings.LastIndex(c, "@"); i >= 0 && fullSHA.MatchString(c[i+1:]) {
			return Finding{}, true
		}
		return Finding{File: inc.File,
			Message: at("component %s is not pinned to a commit SHA", c),
			Fix:     "pin the component to a full 40-character commit SHA, with the version in a comment (component: <path>@<sha> # 1.2.0)"}, false
	case IncludeRemote:
		url := inc.Node.Value
		if inc.Node.Kind == yaml.MappingNode {
			url = what
			if value("integrity") != "" {
				return Finding{}, true
			}
		}
		return Finding{File: inc.File,
			Message: at("remote %s has no integrity:", url),
			Fix:     "add integrity: sha256-<base64 of the file's SHA-256 digest>, or copy the file into the repository and include it with local:"}, false
	case IncludeTemplate:
		return Finding{File: inc.File,
			Message: at("template %s cannot be pinned: GitLab serves the version of the instance", what),
			Fix:     "copy the template into the repository and include it with local:, or include the matching CI/CD component pinned to a commit SHA"}, false
	}
	// A dynamic child pipeline (artifact:) is generated by the pipeline
	// itself: there is nothing to pin.
	return Finding{}, true
}
