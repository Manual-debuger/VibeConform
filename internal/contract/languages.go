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
	// companions are the files DetectComponents also wants beside the
	// manifest before a directory is a component of the language.
	companions []string
	// lockfiles are the lockfiles D-01 recognizes for the language, in
	// the order D-01 checks them (spec 0051, D-01).
	lockfiles []lockfile
	// testFiles are the file name patterns that mark a directory as
	// holding tests (spec 0053, "Placement").
	testFiles []string
}

// lockfile is one lockfile D-01 recognizes: its file name, the commands
// that check it in the component's directory, run in order, and the
// command that updates it, which the stale-lockfile fix names.
type lockfile struct {
	file  string
	check [][]string
	fix   string
}

var languageTable = map[string]languageFacts{
	LanguageGo: {
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
	},
	LanguagePython: {
		manifest:        "pyproject.toml",
		hasDependencies: pyprojectHasDependencies,
		lockfiles: []lockfile{
			{file: "uv.lock", check: [][]string{{"uv", "lock", "--check"}}, fix: "uv lock"},
			{file: "poetry.lock", check: [][]string{{"poetry", "check", "--lock"}}, fix: "poetry lock"},
		},
		testFiles: []string{"test_*.py", "*_test.py"},
	},
	LanguageTypeScript: {
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
	},
}

// allLockfiles are every language's lockfiles, in language order and
// then in each language's order. D-01 checks a component's directory for
// each of them, whatever the component's language.
func allLockfiles() []lockfile {
	var all []lockfile
	for _, language := range languages {
		all = append(all, languageTable[language].lockfiles...)
	}
	return all
}

// lockfileNames names a language's lockfiles for a finding, such as
// "uv.lock or poetry.lock".
func lockfileNames(language string) string {
	var names []string
	for _, lf := range languageTable[language].lockfiles {
		names = append(names, lf.file)
	}
	return orList(names)
}

// lockfileCheckText is D-01's sentence for every lockfile's check, in
// D-01's order: "For `X`: `cmd`." for one command, "For `X`: run `a`,
// then `b`." for more.
func lockfileCheckText() string {
	var sentences []string
	for _, lf := range allLockfiles() {
		commands := make([]string, len(lf.check))
		for i, argv := range lf.check {
			commands[i] = "`" + strings.Join(argv, " ") + "`"
		}
		if len(commands) == 1 {
			sentences = append(sentences, "For `"+lf.file+"`: "+commands[0]+".")
			continue
		}
		sentences = append(sentences, "For `"+lf.file+"`: run "+strings.Join(commands, ", then ")+".")
	}
	return strings.Join(sentences, " ")
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
