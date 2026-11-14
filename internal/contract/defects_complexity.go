package contract

import (
	"fmt"
	"strings"
)

// complexityBranches is the number of if statements in V-06's sample:
// its cyclomatic complexity is complexityBranches+1 and its cognitive
// complexity complexityBranches.
const complexityBranches = 41

// complexityLimit is the largest complexity limit, of either metric, that
// catches V-06's sample. Complexity linters (gocyclo, gocognit, Ruff's
// C901, ESLint's complexity, sonarjs's cognitive-complexity) report a
// function whose complexity is above their limit, and the sample's lower
// metric, its cognitive complexity, is complexityBranches.
const complexityLimit = complexityBranches - 1

// complexityFunction names V-06's sample by its complexities, as the
// rule text states it.
var complexityFunction = fmt.Sprintf("a function of cyclomatic complexity %d and cognitive complexity %d",
	complexityBranches+1, complexityBranches)

// complexityCatches is the rule text's sentence on what catches V-06's
// sample.
var complexityCatches = fmt.Sprintf("A complexity limit of %d or less, of either metric, catches it.", complexityLimit)

// complexityHead is the first comment of V-06's sample, with the
// language's line comment marker.
func complexityHead(marker string) string {
	return fmt.Sprintf("%[1]s Planted by vibe audit: a function of cyclomatic complexity %[2]d and\n"+
		"%[1]s cognitive complexity %[3]d (the complexity defect).\n", marker, complexityBranches+1, complexityBranches)
}

// complexityLayout is how one language writes V-06's sample.
type complexityLayout struct {
	name, head, branch, tail string
}

// complexityLayouts holds the sample's layout by language. branch takes
// the threshold and the constant it adds.
var complexityLayouts = map[string]complexityLayout{
	LanguageGo: {
		// A non-test file, since repositories commonly relax lint for
		// test files (spec 0061, "The Go V-06 sample is a non-test file").
		name: "vibe_probe_complexity.go",
		head: complexityHead("//") + "\n" +
			"package {{package}}\n\n" +
			"// VibeProbeComplexity adds a constant to a running total for each\n" +
			"// threshold that n exceeds.\n" +
			"func VibeProbeComplexity(n int) int {\n\ttotal := 0\n",
		branch: "\tif n > %d {\n\t\ttotal += %d\n\t}\n",
		tail:   "\treturn total\n}\n",
	},
	LanguagePython: {
		name: "vibe_probe_complexity.py",
		head: complexityHead("#") +
			"def vibe_probe_complexity(n: int) -> int:\n    total = 0\n",
		branch: "    if n > %d:\n        total += %d\n",
		tail:   "    return total\n",
	},
	LanguageTypeScript: {
		name: "vibe_probe_complexity.ts",
		head: complexityHead("//") +
			"export function vibeProbeComplexity(n: number): number {\n  let total = 0\n",
		branch: "  if (n > %d) {\n    total += %d\n  }\n",
		tail:   "  return total\n}\n",
	},
}

// complexitySample is V-06's sample in a language (spec 0061, "V-06,
// complexity"): one function of one integer parameter that holds
// complexityBranches consecutive, unnested if statements, each adding a
// constant to a running total, and returns the total. One return
// statement, no nesting and no duplicated branches. The spec says 40 if
// statements; its implementation notes say why the sample has 41.
func complexitySample(lang string) defectSample {
	l := complexityLayouts[lang]
	var b strings.Builder
	b.WriteString(l.head)
	for i := range complexityBranches {
		fmt.Fprintf(&b, l.branch, i, i+2)
	}
	b.WriteString(l.tail)
	return defectSample{Name: l.name, Content: b.String()}
}
