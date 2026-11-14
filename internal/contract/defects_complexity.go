package contract

import (
	"fmt"
	"strings"
)

// complexityBranches is the number of if statements in V-06's sample. A
// complexity limit of complexityBranches or less, of either metric,
// catches it.
const complexityBranches = 40

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
		head: "// Planted by vibe audit: a function of cyclomatic complexity 41 and\n" +
			"// cognitive complexity 40 (the complexity defect).\n\n" +
			"package {{package}}\n\n" +
			"// VibeProbeComplexity adds a constant to a running total for each\n" +
			"// threshold that n exceeds.\n" +
			"func VibeProbeComplexity(n int) int {\n\ttotal := 0\n",
		branch: "\tif n > %d {\n\t\ttotal += %d\n\t}\n",
		tail:   "\treturn total\n}\n",
	},
	LanguagePython: {
		name: "vibe_probe_complexity.py",
		head: "# Planted by vibe audit: a function of cyclomatic complexity 41 and\n" +
			"# cognitive complexity 40 (the complexity defect).\n" +
			"def vibe_probe_complexity(n: int) -> int:\n    total = 0\n",
		branch: "    if n > %d:\n        total += %d\n",
		tail:   "    return total\n",
	},
	LanguageTypeScript: {
		name: "vibe_probe_complexity.ts",
		head: "// Planted by vibe audit: a function of cyclomatic complexity 41 and\n" +
			"// cognitive complexity 40 (the complexity defect).\n" +
			"export function vibeProbeComplexity(n: number): number {\n  let total = 0\n",
		branch: "  if (n > %d) {\n    total += %d\n  }\n",
		tail:   "  return total\n}\n",
	},
}

// complexitySample is V-06's sample in a language (spec 0061, "V-06,
// complexity"): one function of one integer parameter that holds 40
// consecutive, unnested if statements, each adding a constant to a
// running total, and returns the total. Cyclomatic complexity 41,
// cognitive complexity 40, one return statement, no nesting and no
// duplicated branches.
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
