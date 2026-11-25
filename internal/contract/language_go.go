package contract

import "golang.org/x/mod/modfile"

// goLanguage is Go's record.
var goLanguage = languageFacts{
	id:              LanguageGo,
	name:            "Go",
	manifest:        "go.mod",
	hasDependencies: goModHasDependencies,
	lockfiles: []lockfile{
		{
			file:  "go.sum",
			check: [][]string{{"go", "mod", "tidy"}, {"git", "diff", "--exit-code", "go.mod", "go.sum"}},
			fix:   "go mod tidy",
		},
	},
	testFiles: []string{"*_test.go"},
	samples: map[defectKind]defectSample{
		// Test files, so that they may use the package clause of an
		// external test package; gofmt rewrites the spacing, go vet
		// reports the Printf call.
		defectFormat: {Name: "vibe_probe_format_test.go", Content: "// Planted by vibe audit: an unformatted file (the format defect).\n\n" +
			"package {{package}}\n\nfunc  vibeProbeFormat()  {}\n"},
		defectStaticAnalysis: {Name: "vibe_probe_static_analysis_test.go", Content: "// Planted by vibe audit: a Printf call whose arguments do not match its format\n" +
			"// (the static-analysis defect).\n\n" +
			"package {{package}}\n\nimport \"fmt\"\n\n" +
			"func vibeProbeStaticAnalysis() { fmt.Printf(\"%d\\n\", \"vibe audit\") }\n"},
		// V-08: V-01's test file, its Printf call under a nolint
		// directive that names no linter. go vet ignores the directive;
		// golangci-lint honors it.
		defectBlanketSuppression: {Name: "vibe_probe_blanket_suppression_test.go", Content: "// Planted by vibe audit: a Printf call whose arguments do not match its format\n" +
			"// (the static-analysis defect), under a blanket suppression.\n\n" +
			"package {{package}}\n\nimport \"fmt\"\n\n" +
			"func vibeProbeStaticAnalysis() { fmt.Printf(\"%d\\n\", \"vibe audit\") } //nolint\n"},
		defectTest: {Name: "vibe_probe_test.go", Content: "// Planted by vibe audit: a test that fails (the test defect).\n\n" +
			"package {{package}}\n\nimport \"testing\"\n\n" +
			"func TestVibeProbe(t *testing.T) { t.Fatal(\"planted by vibe audit: this test fails\") }\n"},
		defectSecret: secretSample,
	},
	complexity: complexityLayout{
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
	dependabotEcosystems: []string{"gomod"},
	renovateManagers:     []string{"gomod"},
}

// goModHasDependencies reports whether go.mod has a require directive.
func goModHasDependencies(manifest string, data []byte) (bool, error) {
	f, err := modfile.ParseLax(manifest, data, nil)
	if err != nil {
		return false, err
	}
	return len(f.Require) > 0, nil
}
