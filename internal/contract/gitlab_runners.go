package contract

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Runners of a GitLab job as the reader fills them in (spec 0055, "The
// rules on GitLab CI", C-05): a tag containing windows, linux or macos
// (ignoring case) names the platform, a $VARIABLE in a tag takes each
// value of a parallel: matrix, and a job with no tags and an image: runs
// on Linux.

// glVariable is a variable in a tag: $NAME or ${NAME}.
var glVariable = regexp.MustCompile(`\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))`)

// gitlabPlatforms returns the platforms a resolved job runs on, one per
// parallel: matrix combination, and describes each combination that names
// no platform.
func gitlabPlatforms(j *CIJob) (platforms, unknown []string) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	tags := mapValue(j.Node, "tags")
	image := mapValue(j.Node, string(ImageKey)) != nil
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
