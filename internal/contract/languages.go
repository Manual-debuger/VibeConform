package contract

import (
	"encoding/json"
	"path"
	"strings"

	"golang.org/x/mod/modfile"
)

// languageFacts is what the execution and probe parts know about a
// component language, in one place.
type languageFacts struct {
	// manifest is the file that declares the component's dependencies.
	manifest string
	// hasDependencies reads the manifest: whether it declares
	// dependencies (spec 0053, D-01).
	hasDependencies func(manifest string, data []byte) (bool, error)
	// lockfiles names the lockfiles D-01 recognizes, for a finding.
	lockfiles string
	// testFiles are the file name patterns that mark a directory as
	// holding tests (spec 0053, "Placement").
	testFiles []string
}

var languageTable = map[string]languageFacts{
	LanguageGo: {
		manifest:        "go.mod",
		hasDependencies: goModHasDependencies,
		lockfiles:       "go.sum",
		testFiles:       []string{"*_test.go"},
	},
	LanguagePython: {
		manifest:        "pyproject.toml",
		hasDependencies: pyprojectHasDependencies,
		lockfiles:       "uv.lock or poetry.lock",
		testFiles:       []string{"test_*.py", "*_test.py"},
	},
	LanguageTypeScript: {
		manifest:        "package.json",
		hasDependencies: packageJSONHasDependencies,
		lockfiles:       "pnpm-lock.yaml or package-lock.json",
		testFiles:       []string{"*.test.ts", "*.spec.ts", "*.test.tsx", "*.spec.tsx"},
	},
}

// isTestFile reports whether a file name marks its directory as holding
// tests of a language.
func isTestFile(language, name string) bool {
	for _, pattern := range languageTable[language].testFiles {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

// testFilePattern names a language's test files for a fix, such as "a
// test_*.py or *_test.py file".
func testFilePattern(language string) string {
	return "a " + orList(languageTable[language].testFiles) + " file"
}

// orList joins items as "a", "a or b" or "a, b or c".
func orList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

func goModHasDependencies(manifest string, data []byte) (bool, error) {
	f, err := modfile.ParseLax(manifest, data, nil)
	if err != nil {
		return false, err
	}
	return len(f.Require) > 0, nil
}

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
