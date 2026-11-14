package contract

import (
	"encoding/base64"
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
	// defectBlanketSuppression is V-01's static-analysis defect under the
	// language's blanket suppressions (spec 0061, V-08).
	defectBlanketSuppression defectKind = "blanket suppression"
)

// The defect kinds of spec 0061.
const (
	// defectTypeError is V-05's kind. Only Python has a sample: in Go and
	// TypeScript a type error is a compile error, which V-01 shows.
	defectTypeError defectKind = "type error"
	// defectComplexity is V-06's kind.
	defectComplexity defectKind = "complexity"
	// defectSecret is V-07's kind.
	defectSecret defectKind = "secret"
)

// defectSample is one planted file. In Content, {{package}} stands for
// the package clause's name of the Go test file beside it; in a Go
// sample that is not a _test.go file, without a _test suffix (spec 0061,
// "The Go V-06 sample is a non-test file").
type defectSample struct {
	Name    string
	Content string
}

// v01Kinds lists V-01's and V-02's defect kinds in the order the probes
// plant them.
var v01Kinds = []defectKind{defectFormat, defectStaticAnalysis, defectTest}

// defectCatchers names, by defect kind, what an entrypoint must run to
// catch it. A probe's Fix lists the catchers of the kinds it missed.
var defectCatchers = map[defectKind]string{
	defectFormat:         "a format check",
	defectStaticAnalysis: "static analysis (a type check or linter)",
	defectTest:           "the tests",
	defectTypeError:      "a type checker",
	defectBlanketSuppression: "static analysis that a suppression comment naming no rule cannot switch off " +
		"(golangci-lint's nolintlint, Ruff's PGH004, eslint-comments' no-unlimited-disable)",
	defectComplexity: "a complexity limit of 40 or less (cyclomatic or cognitive)",
	defectSecret:     "a secret scanner over the working tree's files",
}

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
		defectComplexity: complexitySample(LanguageGo),
		defectSecret:     secretSample,
	},
	LanguagePython: {
		defectFormat: {Name: "vibe_probe_format.py", Content: "# Planted by vibe audit: an unformatted file (the format defect).\n" +
			"vibe_probe = [1,2 ,3]\n"},
		defectStaticAnalysis: {Name: "vibe_probe_static_analysis.py", Content: "# Planted by vibe audit: a reference to an undefined name (the static-analysis defect).\n" +
			"def vibe_probe() -> int:\n    return vibe_probe_undefined_name\n"},
		// V-08: Pyright honors # type: ignore, Ruff honors # noqa.
		defectBlanketSuppression: {Name: "vibe_probe_blanket_suppression.py", Content: "# Planted by vibe audit: a reference to an undefined name (the static-analysis defect),\n" +
			"# under blanket suppressions.\n" +
			"def vibe_probe() -> int:\n    return vibe_probe_undefined_name  # type: ignore  # noqa\n"},
		defectTest: {Name: "vibe_probe_test.py", Content: "# Planted by vibe audit: a test that fails (the test defect).\n" +
			"def test_vibe_probe() -> None:\n    assert False, \"planted by vibe audit: this test fails\"\n"},
		defectTypeError: {Name: "vibe_probe_type_error.py", Content: "# Planted by vibe audit: a function annotated -> int that returns a string (the type-error defect).\n" +
			"def vibe_probe_type_error() -> int:\n    return \"vibe audit\"\n"},
		defectComplexity: complexitySample(LanguagePython),
		defectSecret:     secretSample,
	},
	LanguageTypeScript: {
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
		defectComplexity: complexitySample(LanguageTypeScript),
		defectSecret:     secretSample,
	},
}

// secretSample is V-07's sample (spec 0061, "V-07, secret"): a plain
// text file holding a PEM private-key block with a fixed base64 body. A
// secret does not depend on the language, so every language has the same
// sample. The boundary lines are assembled here, at run time, so that no
// file in this repository holds a literal private-key block for secret
// scanners to report.
var secretSample = func() defectSample {
	boundary := func(word string) string {
		dashes := strings.Repeat("-", 5)
		return dashes + word + " PRIVATE " + "KEY" + dashes + "\n"
	}
	body := base64.StdEncoding.EncodeToString([]byte("Planted by vibe audit: this is not a key, it is the secret defect."))
	var b strings.Builder
	b.WriteString("Planted by vibe audit: a PEM private-key block (the secret defect). It is not a key.\n")
	b.WriteString(boundary("BEGIN"))
	for len(body) > 64 {
		b.WriteString(body[:64] + "\n")
		body = body[64:]
	}
	b.WriteString(body + "\n")
	b.WriteString(boundary("END"))
	return defectSample{Name: "vibe_probe_secret.txt", Content: b.String()}
}()

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

// defectFor places a defect kind's sample for the component's language
// in the component. It returns errNoTests when no directory qualifies,
// and an error when the sample cannot be built.
func defectFor(env *Env, comp Component, kind defectKind) (plantedDefect, error) {
	sample, ok := defectSamples[comp.Language][kind]
	if !ok {
		return plantedDefect{}, fmt.Errorf("this vibe has no %s sample for %s", kind, comp.Language)
	}
	return plantSample(env, comp, sample)
}

// plantSample places a sample in the first directory of a component that
// holds tests, with {{package}} filled in. It returns errNoTests when no
// directory qualifies.
func plantSample(env *Env, comp Component, sample defectSample) (plantedDefect, error) {
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
		pkg := f.Name.Name
		if !strings.HasSuffix(sample.Name, "_test.go") {
			// Only a _test.go file may declare the external test package.
			pkg = strings.TrimSuffix(pkg, "_test")
		}
		content = strings.ReplaceAll(content, "{{package}}", pkg)
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
