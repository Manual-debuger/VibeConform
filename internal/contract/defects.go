package contract

import (
	"fmt"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strings"
)

// Planted defects (spec 0053, "Planted defects"): one fixed sample per
// language and defect kind, planted into the first directory of a
// component that holds tests. The samples are part of contract 1.0.0:
// changing one can change a verdict.

// defectKind is a kind of planted defect, as V-01 names it.
type defectKind string

// The defect kinds of spec 0051.
const (
	defectFormat         defectKind = "format"
	defectStaticAnalysis defectKind = "static analysis"
	defectTest           defectKind = "test"
)

// defectSample is one planted file. In Content, {{package}} stands for
// the package clause's name of the Go test file beside it.
type defectSample struct {
	Name    string
	Content string
}

// defectKinds lists the defect kinds in the order the probes plant them.
var defectKinds = []defectKind{defectFormat, defectStaticAnalysis, defectTest}

// defectSamples holds the samples by language and kind. Spec 0053
// ("Planted defects") quotes each one; change both together.
var defectSamples = map[string]map[defectKind]defectSample{
	LanguageGo: {
		// Test files, so that they may use the package clause of an
		// external test package; gofmt rewrites the spacing, go vet
		// reports the Printf call.
		defectFormat: {Name: "vibe_probe_format_test.go", Content: "// Planted by vibe audit: an unformatted file (the format defect).\n\n" +
			"package {{package}}\n\nfunc  vibeProbeFormat()  {}\n"},
		defectStaticAnalysis: {Name: "vibe_probe_static_analysis_test.go", Content: "// Planted by vibe audit: a Printf call whose arguments do not match its format\n" +
			"// (the static-analysis defect).\n\n" +
			"package {{package}}\n\nimport \"fmt\"\n\n" +
			"func vibeProbeStaticAnalysis() { fmt.Printf(\"%d\\n\", \"vibe audit\") }\n"},
		defectTest: {Name: "vibe_probe_test.go", Content: "// Planted by vibe audit: a test that fails (the test defect).\n\n" +
			"package {{package}}\n\nimport \"testing\"\n\n" +
			"func TestVibeProbe(t *testing.T) { t.Fatal(\"planted by vibe audit: this test fails\") }\n"},
	},
	LanguagePython: {
		defectFormat: {Name: "vibe_probe_format.py", Content: "# Planted by vibe audit: an unformatted file (the format defect).\n" +
			"vibe_probe = [1,2 ,3]\n"},
		defectStaticAnalysis: {Name: "vibe_probe_static_analysis.py", Content: "# Planted by vibe audit: a reference to an undefined name (the static-analysis defect).\n" +
			"def vibe_probe() -> int:\n    return vibe_probe_undefined_name\n"},
		defectTest: {Name: "vibe_probe_test.py", Content: "# Planted by vibe audit: a test that fails (the test defect).\n" +
			"def test_vibe_probe() -> None:\n    assert False, \"planted by vibe audit: this test fails\"\n"},
	},
	LanguageTypeScript: {
		defectFormat: {Name: "vibe_probe_format.ts", Content: "// Planted by vibe audit: an unformatted file (the format defect).\n" +
			"export const vibeProbe = [1,2 ,3]\n"},
		defectStaticAnalysis: {Name: "vibe_probe_static_analysis.ts", Content: "// Planted by vibe audit: a reference to an undefined name (the static-analysis defect).\n" +
			"export const vibeProbe: number = vibeProbeUndefinedName;\n"},
		defectTest: {Name: "vibe_probe.test.ts", Content: "// Planted by vibe audit: a test file that throws when it is loaded (the test defect).\n" +
			"export {};\n\nthrow new Error(\"planted by vibe audit: this test fails\");\n"},
	},
}

// isTestFile reports whether a file name marks its directory as holding
// tests of a language (spec 0053, "Placement").
func isTestFile(language, name string) bool {
	switch language {
	case LanguageGo:
		return strings.HasSuffix(name, "_test.go")
	case LanguagePython:
		return strings.HasSuffix(name, ".py") && (strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py"))
	case LanguageTypeScript:
		for _, suf := range []string{".test.ts", ".spec.ts", ".test.tsx", ".spec.tsx"} {
			if strings.HasSuffix(name, suf) {
				return true
			}
		}
	}
	return false
}

// plantDir returns the first directory, in sorted path order, inside a
// component that holds a test file of its language, and that test file.
// Directories under vendor/ or node_modules/ do not qualify. Components
// do not nest (the configuration refuses that), so every directory
// inside a component is the component's. ok is false when no directory
// does.
func plantDir(env *Env, comp Component) (dir, testFile string, ok bool) {
	var files []string
	if comp.Path == "." {
		files = env.Repo.Files()
	} else {
		files = env.Repo.FilesUnder(comp.Path)
	}
	best := ""
	for _, f := range files {
		d, name := path.Split(f)
		d = strings.TrimSuffix(d, "/")
		if d == "" {
			d = "."
		}
		if !isTestFile(comp.Language, name) || underVendor(d) {
			continue
		}
		if !ok || d < dir || (d == dir && f < best) {
			dir, best, ok = d, f, true
		}
	}
	return dir, best, ok
}

func underVendor(dir string) bool {
	parts := strings.Split(dir, "/")
	return slices.Contains(parts, "vendor") || slices.Contains(parts, "node_modules")
}

// plantedDefect is a sample placed in a component.
type plantedDefect struct {
	// Path is slash separated, relative to the copy's root.
	Path    string
	Content []byte
}

// errNoTests is the reason no defect can be planted in a component.
type errNoTests struct{ component string }

func (e errNoTests) Error() string { return "component " + e.component + " has no tests" }

// defectFor places a defect kind's sample in a component. It returns
// errNoTests when no directory qualifies, and an error when the sample
// cannot be built.
func defectFor(env *Env, comp Component, kind defectKind) (plantedDefect, error) {
	sample, ok := defectSamples[comp.Language][kind]
	if !ok {
		return plantedDefect{}, fmt.Errorf("this vibe has no %s sample for %s", kind, comp.Language)
	}
	dir, testFile, ok := plantDir(env, comp)
	if !ok {
		return plantedDefect{}, errNoTests{comp.Path}
	}
	content := sample.Content
	if strings.Contains(content, "{{package}}") {
		src, err := env.ReadFile(testFile)
		if err != nil {
			return plantedDefect{}, err
		}
		f, err := parser.ParseFile(token.NewFileSet(), testFile, src, parser.PackageClauseOnly)
		if err != nil {
			return plantedDefect{}, fmt.Errorf("%s: cannot read its package clause: %w", testFile, err)
		}
		content = strings.ReplaceAll(content, "{{package}}", f.Name.Name)
	}
	p := sample.Name
	if dir != "." {
		p = dir + "/" + sample.Name
	}
	if env.Repo.HasFile(p) {
		return plantedDefect{}, fmt.Errorf("%s already exists, so the sample cannot be planted there", p)
	}
	return plantedDefect{Path: p, Content: []byte(content)}, nil
}
