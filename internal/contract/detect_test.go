package contract_test

import (
	"slices"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// detectCase is a tracked file list and the detection vibe init gets
// from it.
type detectCase struct {
	name  string
	files []string
	want  contract.Detection
}

func runDetectCases(t *testing.T, cases []detectCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			files := slices.Clone(c.files)
			slices.Sort(files)
			got := contract.DetectComponents(files)
			if !slices.Equal(got.Components, c.want.Components) {
				t.Errorf("components = %v, want %v", got.Components, c.want.Components)
			}
			if !slices.Equal(got.Nested, c.want.Nested) {
				t.Errorf("nested = %v, want %v", got.Nested, c.want.Nested)
			}
			if got.AuditDetectsSame != c.want.AuditDetectsSame {
				t.Errorf("audit detects the same = %v, want %v", got.AuditDetectsSame, c.want.AuditDetectsSame)
			}
		})
	}
}

func detected(dir, language string) contract.Component {
	return contract.Component{Path: dir, Language: language}
}

func detectedComponents(c ...contract.Component) []contract.Component { return c }

// The layouts of vibe init's CLI suite before spec 0066, with the
// components vibe init wrote for each.
func TestDetectComponentsInitLayouts(t *testing.T) {
	t.Parallel()
	runDetectCases(t, []detectCase{
		{name: "go at the root", files: []string{"go.mod", "fixture_test.go"},
			want: contract.Detection{Components: detectedComponents(detected(".", contract.LanguageGo)), AuditDetectsSame: true}},
		{name: "python", files: []string{"pyproject.toml", "calc.py"},
			want: contract.Detection{Components: detectedComponents(detected(".", contract.LanguagePython)), AuditDetectsSame: true}},
		{name: "typescript", files: []string{"package.json", "tsconfig.json", "calc.ts"},
			want: contract.Detection{Components: detectedComponents(detected(".", contract.LanguageTypeScript)), AuditDetectsSame: true}},
		{name: "monorepo", files: []string{
			"services/api/go.mod",
			"tools/py/pyproject.toml", "tools/py/calc.py",
			"web/app/package.json", "web/app/tsconfig.json", "web/app/calc.ts",
			// Not components: a package.json without tsconfig.json,
			// manifests of dependencies, a manifest below the depth
			// detection looks at, and a manifest inside another component.
			"docs/site/package.json",
			"node_modules/dep/package.json", "node_modules/dep/tsconfig.json",
			"third/vendor/x/go.mod",
			"a/b/c/d/go.mod",
			"services/api/tools/go.mod",
		}, want: contract.Detection{
			Components: detectedComponents(detected("services/api", contract.LanguageGo), detected("tools/py", contract.LanguagePython), detected("web/app", contract.LanguageTypeScript)),
			Nested:     []contract.NestedManifest{{Dir: "services/api/tools", In: detected("services/api", contract.LanguageGo)}},
		}},
		{name: "nested manifests", files: []string{"go.mod", "tools/go.mod", "web/package.json", "web/tsconfig.json"},
			want: contract.Detection{
				Components: detectedComponents(detected(".", contract.LanguageGo)),
				Nested: []contract.NestedManifest{
					{Dir: "tools", In: detected(".", contract.LanguageGo)},
					{Dir: "web", In: detected(".", contract.LanguageGo)},
				},
				AuditDetectsSame: true,
			}},
		{name: "no manifest", files: []string{"README.md", "main.go"}, want: contract.Detection{}},
		{name: "ambiguous root", files: []string{"go.mod", "package.json", "tsconfig.json", "calc.ts"},
			want: contract.Detection{Components: detectedComponents(detected(".", contract.LanguageGo))}},
	})
}

// Detection looks DetectionDepth directories below the root and skips
// the directories of dependencies, at any level.
func TestDetectComponentsDepthAndSkippedDirectories(t *testing.T) {
	t.Parallel()
	if contract.DetectionDepth != 3 {
		t.Errorf("DetectionDepth = %d, want 3", contract.DetectionDepth)
	}
	runDetectCases(t, []detectCase{
		{name: "three directories deep", files: []string{"a/b/c/go.mod"},
			want: contract.Detection{Components: detectedComponents(detected("a/b/c", contract.LanguageGo))}},
		{name: "four directories deep", files: []string{"a/b/c/d/go.mod"}, want: contract.Detection{}},
		{name: "vendor", files: []string{"vendor/x/go.mod"}, want: contract.Detection{}},
		{name: "node_modules below a directory", files: []string{"web/node_modules/dep/package.json", "web/node_modules/dep/tsconfig.json"},
			want: contract.Detection{}},
		{name: "vendor as the last directory", files: []string{"svc/vendor/pyproject.toml"}, want: contract.Detection{}},
		{name: "a name that only contains vendor", files: []string{"vendored/go.mod"},
			want: contract.Detection{Components: detectedComponents(detected("vendored", contract.LanguageGo))}},
	})
}

// Components do not nest: a directory with a manifest inside a
// component is reported with that component, however deep.
func TestDetectComponentsDoNotNest(t *testing.T) {
	t.Parallel()
	runDetectCases(t, []detectCase{
		{name: "two levels inside", files: []string{"svc/go.mod", "svc/a/b/pyproject.toml"},
			want: contract.Detection{
				Components: detectedComponents(detected("svc", contract.LanguageGo)),
				Nested:     []contract.NestedManifest{{Dir: "svc/a/b", In: detected("svc", contract.LanguageGo)}},
			}},
		{name: "siblings with a shared prefix", files: []string{"svc/go.mod", "svc-web/package.json", "svc-web/tsconfig.json"},
			want: contract.Detection{Components: detectedComponents(detected("svc", contract.LanguageGo), detected("svc-web", contract.LanguageTypeScript))}},
		{name: "a package.json without tsconfig.json is not nested", files: []string{"go.mod", "docs/package.json"},
			want: contract.Detection{Components: detectedComponents(detected(".", contract.LanguageGo)), AuditDetectsSame: true}},
	})
}

// When a directory has more than one manifest, the first language in
// the order Go, Python, TypeScript wins; TypeScript also wants a
// tsconfig.json beside package.json.
func TestDetectComponentsPriority(t *testing.T) {
	t.Parallel()
	runDetectCases(t, []detectCase{
		{name: "go before python", files: []string{"svc/pyproject.toml", "svc/go.mod"},
			want: contract.Detection{Components: detectedComponents(detected("svc", contract.LanguageGo))}},
		{name: "python before typescript", files: []string{"svc/package.json", "svc/tsconfig.json", "svc/pyproject.toml"},
			want: contract.Detection{Components: detectedComponents(detected("svc", contract.LanguagePython))}},
		{name: "package.json without tsconfig.json", files: []string{"svc/package.json"}, want: contract.Detection{}},
		{name: "tsconfig.json without package.json", files: []string{"svc/tsconfig.json"}, want: contract.Detection{}},
		{name: "tsconfig.json in another directory", files: []string{"svc/package.json", "tsconfig.json"}, want: contract.Detection{}},
	})
}

// The audit detects the components itself only when there is one, at
// the root, and the root has a single root manifest, of that language.
func TestDetectComponentsAuditDetectsSame(t *testing.T) {
	t.Parallel()
	runDetectCases(t, []detectCase{
		{name: "one component below the root", files: []string{"svc/go.mod"},
			want: contract.Detection{Components: detectedComponents(detected("svc", contract.LanguageGo))}},
		{name: "a root package.json without tsconfig.json beside go.mod", files: []string{"go.mod", "package.json"},
			want: contract.Detection{Components: detectedComponents(detected(".", contract.LanguageGo))}},
		{name: "python beside a package.json without tsconfig.json", files: []string{"pyproject.toml", "package.json"},
			want: contract.Detection{Components: detectedComponents(detected(".", contract.LanguagePython))}},
		{name: "manifests below the root do not count", files: []string{"pyproject.toml", "tools/go.mod"},
			want: contract.Detection{
				Components:       detectedComponents(detected(".", contract.LanguagePython)),
				Nested:           []contract.NestedManifest{{Dir: "tools", In: detected(".", contract.LanguagePython)}},
				AuditDetectsSame: true,
			}},
	})
}

// The phrase vibe init's error and help text name the manifests with.
func TestManifestPhrase(t *testing.T) {
	t.Parallel()
	if got, want := contract.ManifestPhrase(), "go.mod, pyproject.toml, or package.json beside a tsconfig.json"; got != want {
		t.Errorf("ManifestPhrase() = %q, want %q", got, want)
	}
}
