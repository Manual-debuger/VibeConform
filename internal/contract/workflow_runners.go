package contract

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Runners as C-05 reads them (spec 0052, "Recognition rules", Runners):
// runs-on as a string, a list of labels or a group with labels; a
// ${{ matrix.<key> }} value expands to the values that key takes after
// include and exclude. A label ubuntu-* or linux is Linux, windows-* or
// windows is Windows, macos-* or macos is macOS; any other label or
// expression is an unknown platform.

// matrixRef is a runs-on label that is exactly one matrix value.
var matrixRef = regexp.MustCompile(`^\$\{\{\s*matrix\.([A-Za-z0-9_-]+)\s*\}\}$`)

// unknownValue stands for a matrix value that is not a scalar.
const unknownValue = "\x00"

// matrixCombo is one matrix combination: key to scalar value.
type matrixCombo map[string]string

// runnerPlatforms returns the platforms a job runs on, one entry per
// matrix combination, and the runners whose platform is unknown, as
// written.
func runnerPlatforms(j *Job) (platforms, unknown []string) {
	combos, matrixKnown := matrixCombos(j.Strategy)
	for _, combo := range combos {
		p, label := comboPlatform(j.RunsOn, combo, matrixKnown)
		if p == "" {
			if !slices.Contains(unknown, label) {
				unknown = append(unknown, label)
			}
			continue
		}
		if !slices.Contains(platforms, p) {
			platforms = append(platforms, p)
		}
	}
	return platforms, unknown
}

// comboPlatform decides the platform of runs-on in one matrix
// combination: the first label that names one. It returns "" and the
// runs-on text when none does.
func comboPlatform(runsOn *yaml.Node, combo matrixCombo, matrixKnown bool) (string, string) {
	if runsOn == nil {
		return "", "(no runs-on)"
	}
	var labels []string
	switch runsOn.Kind {
	case yaml.ScalarNode, yaml.SequenceNode:
		labels = filterPatterns(runsOn)
	case yaml.MappingNode:
		if l := mapValue(runsOn, "labels"); l != nil {
			labels = filterPatterns(l)
		}
	}
	var written []string
	for _, label := range labels {
		written = append(written, label)
		value := label
		if m := matrixRef.FindStringSubmatch(label); m != nil {
			v, ok := combo[m[1]]
			if !matrixKnown || !ok || v == unknownValue {
				continue
			}
			value = v
		}
		if p := labelPlatform(value); p != "" {
			return p, ""
		}
	}
	if len(written) == 0 {
		return "", "(runs-on with no labels)"
	}
	return "", strings.Join(written, ", ")
}

// labelPlatform maps a runner label to a platform, or "".
func labelPlatform(label string) string {
	l := strings.ToLower(label)
	switch {
	case strings.Contains(l, "${{"):
		return ""
	case l == "linux" || strings.HasPrefix(l, "ubuntu-"):
		return PlatformLinux
	case l == "windows" || strings.HasPrefix(l, "windows-"):
		return PlatformWindows
	case l == "macos" || strings.HasPrefix(l, "macos-"):
		return PlatformMacOS
	}
	return ""
}

// matrixCombos expands a job's strategy.matrix the way GitHub does: the
// product of the keys' lists, minus each combination an exclude entry
// matches, then each include entry merged into the combinations it does
// not contradict, or added as a combination of its own. known is false
// when the matrix is not a map of lists (an expression, such as
// fromJSON); its one combination then has no values.
func matrixCombos(strategy *yaml.Node) (combos []matrixCombo, known bool) {
	m := mapValue(strategy, "matrix")
	if m == nil {
		return []matrixCombo{{}}, true
	}
	if m.Kind != yaml.MappingNode {
		return []matrixCombo{{}}, false
	}
	var includes, excludes []matrixCombo
	combos = []matrixCombo{{}}
	var keys []string
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i].Value, resolve(m.Content[i+1])
		switch k {
		case "include", "exclude":
			entries, ok := comboList(v)
			if !ok {
				return []matrixCombo{{}}, false
			}
			if k == "include" {
				includes = entries
			} else {
				excludes = entries
			}
			continue
		}
		if v.Kind != yaml.SequenceNode {
			return []matrixCombo{{}}, false
		}
		keys = append(keys, k)
		var next []matrixCombo
		for _, c := range combos {
			for _, item := range v.Content {
				n := maps.Clone(c)
				n[k] = nodeValue(resolve(item))
				next = append(next, n)
			}
		}
		combos = next
	}
	if len(keys) == 0 {
		combos = nil
	}
	combos = slices.DeleteFunc(combos, func(c matrixCombo) bool {
		return slices.ContainsFunc(excludes, func(e matrixCombo) bool { return comboMatches(c, e) })
	})
	base := len(combos)
	for _, inc := range includes {
		merged := false
		for i := 0; i < base; i++ {
			if agrees(combos[i], inc, keys) {
				for k, v := range inc {
					combos[i][k] = v
				}
				merged = true
			}
		}
		if !merged {
			combos = append(combos, maps.Clone(inc))
		}
	}
	if len(combos) == 0 {
		combos = []matrixCombo{{}}
	}
	return combos, true
}

// comboList reads include: or exclude:, a list of maps.
func comboList(n *yaml.Node) ([]matrixCombo, bool) {
	if n.Kind != yaml.SequenceNode {
		return nil, false
	}
	var out []matrixCombo
	for _, item := range n.Content {
		item = resolve(item)
		if item.Kind != yaml.MappingNode {
			return nil, false
		}
		c := matrixCombo{}
		for i := 0; i+1 < len(item.Content); i += 2 {
			c[item.Content[i].Value] = nodeValue(resolve(item.Content[i+1]))
		}
		out = append(out, c)
	}
	return out, true
}

func nodeValue(n *yaml.Node) string {
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return unknownValue
}

// comboMatches reports whether an exclude entry matches a combination:
// every key of the entry has the combination's value.
func comboMatches(c, exclude matrixCombo) bool {
	for k, v := range exclude {
		if c[k] != v {
			return false
		}
	}
	return true
}

// agrees reports whether an include entry can merge into a combination:
// it changes none of the matrix's own keys.
func agrees(c, include matrixCombo, keys []string) bool {
	for k, v := range include {
		if slices.Contains(keys, k) && c[k] != v {
			return false
		}
	}
	return true
}
