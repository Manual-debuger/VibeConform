package contract

import "encoding/json"

// typeScriptLanguage is TypeScript's record.
var typeScriptLanguage = languageFacts{
	id:              LanguageTypeScript,
	name:            "TypeScript",
	manifest:        "package.json",
	hasDependencies: packageJSONHasDependencies,
	companions:      []string{"tsconfig.json"},
	lockfiles: []lockfile{
		{
			file:  "pnpm-lock.yaml",
			check: [][]string{{"pnpm", "install", "--frozen-lockfile", "--lockfile-only"}},
			fix:   "pnpm install",
		},
		{file: "package-lock.json", check: [][]string{{"npm", "ci", "--dry-run"}}, fix: "npm install"},
	},
	testFiles: []string{"*.test.ts", "*.spec.ts", "*.test.tsx", "*.spec.tsx"},
	samples: map[defectKind]defectSample{
		defectFormat: {Name: "vibe_probe_format.ts", Content: "// Planted by vibe audit: an unformatted file (the format defect).\n" +
			"export const vibeProbe = [1,2 ,3]\n"},
		defectStaticAnalysis: {Name: "vibe_probe_static_analysis.ts", Content: "// Planted by vibe audit: a reference to an undefined name (the static-analysis defect).\n" +
			"export const vibeProbe: number = vibeProbeUndefinedName;\n"},
		// V-08: ESLint honors eslint-disable, tsc honors @ts-ignore. No
		// semicolon, so that Prettier with semi: false (the setup skill's
		// configuration) finds nothing to change.
		defectBlanketSuppression: {Name: "vibe_probe_blanket_suppression.ts", Content: "// Planted by vibe audit: a reference to an undefined name (the static-analysis defect),\n" +
			"// under blanket suppressions.\n" +
			"/* eslint-disable */\n" +
			"// @ts-ignore\n" +
			"export const vibeProbe: number = vibeProbeUndefinedName\n"},
		defectTest: {Name: "vibe_probe.test.ts", Content: "// Planted by vibe audit: a test file that throws when it is loaded (the test defect).\n" +
			"export {};\n\nthrow new Error(\"planted by vibe audit: this test fails\");\n"},
		defectSecret: secretSample,
	},
	complexity: complexityLayout{
		name: "vibe_probe_complexity.ts",
		head: complexityHead("//") +
			"export function vibeProbeComplexity(n: number): number {\n  let total = 0\n",
		branch: "  if (n > %d) {\n    total += %d\n  }\n",
		tail:   "  return total\n}\n",
	},
	dependabotEcosystems: []string{"npm", "bun"},
	renovateManagers:     []string{"npm", "bun"},
}

// packageJSONHasDependencies reports whether package.json has a
// non-empty dependencies, devDependencies, optionalDependencies or
// peerDependencies.
func packageJSONHasDependencies(_ string, data []byte) (bool, error) {
	var pkg map[string]json.RawMessage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, err
	}
	for _, key := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
		var m map[string]any
		if raw, ok := pkg[key]; ok && json.Unmarshal(raw, &m) == nil && len(m) > 0 {
			return true, nil
		}
	}
	return false, nil
}
