package contract

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// languageFacts is the language record of one component language:
// everything vibe knows about it (spec 0066, "One record per language"). Each language's
// record is in a source file of its own, with its samples and its
// dependency reader, and languageRegistry lists the records.
type languageFacts struct {
	// id is the language's identifier, the configuration's language
	// value.
	id string
	// name is the language's display name, as --explain prints it.
	name string
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
	// samples are the language's planted defect samples by kind (spec
	// 0053, "Planted defects"), except V-06's, which complexity lays
	// out. Spec 0053 quotes each one; change both together.
	samples map[defectKind]defectSample
	// complexity is how the language writes V-06's sample.
	complexity complexityLayout
	// dependabotEcosystems are the package-ecosystem values that cover
	// the language (D-02). A fix names the first.
	dependabotEcosystems []string
	// renovateManagers are the Renovate managers that cover the language
	// (D-02). enabledManagers leaves a component out when it names none
	// of them. A fix names the first.
	renovateManagers []string
}

// sample is the language's planted sample of a defect kind, and whether
// it has one. A record without a complexity layout has no V-06 sample.
func (l languageFacts) sample(kind defectKind) (defectSample, bool) {
	if kind == defectComplexity {
		return complexitySample(l.complexity), l.complexity != complexityLayout{}
	}
	s, ok := l.samples[kind]
	return s, ok
}

// lockfile is one lockfile D-01 recognizes: its file name, the commands
// that check it in the component's directory, run in order, and the
// command that updates it, which the stale-lockfile fix names.
type lockfile struct {
	file  string
	check [][]string
	fix   string
}

// languageRegistry holds the language records in the contract's language
// order. Every list that depends on that order is derived from it: the
// valid language values, the root manifests and detection's priority,
// D-01's lockfile order and the order --explain prints the samples in.
var languageRegistry = []languageFacts{goLanguage, pythonLanguage, typeScriptLanguage}

// lookupLanguage returns the record of the language with this
// identifier, and whether there is one.
func lookupLanguage(id string) (languageFacts, bool) {
	i := slices.IndexFunc(languageRegistry, func(l languageFacts) bool { return l.id == id })
	if i < 0 {
		return languageFacts{}, false
	}
	return languageRegistry[i], true
}

// language returns the record of the component's language. Parse admits
// only the identifiers of languageRegistry, and DetectComponents writes
// only those, so a component without a record is a bug in vibe: language
// panics rather than hand back an empty record.
func (c Component) language() languageFacts {
	l, ok := lookupLanguage(c.Language)
	if !ok {
		panic(fmt.Sprintf("contract: component %s has the language %q, which has no record", c.Path, c.Language))
	}
	return l
}

// languageIDs are the valid language values, in language order.
func languageIDs() []string {
	ids := make([]string, len(languageRegistry))
	for i, l := range languageRegistry {
		ids[i] = l.id
	}
	return ids
}

// allLockfiles are every language's lockfiles, in language order and
// then in each language's order. D-01 checks a component's directory for
// each of them, whatever the component's language.
func allLockfiles() []lockfile {
	var all []lockfile
	for _, l := range languageRegistry {
		all = append(all, l.lockfiles...)
	}
	return all
}

// lockfileNames names the language's lockfiles for a finding, such as
// "uv.lock or poetry.lock".
func (l languageFacts) lockfileNames() string {
	var names []string
	for _, lf := range l.lockfiles {
		names = append(names, lf.file)
	}
	return joinWords(names, " or ")
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
// tests of the language.
func (l languageFacts) isTestFile(name string) bool {
	for _, pattern := range l.testFiles {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

// testFilePattern names the language's test files for a fix, such as "a
// test_*.py or *_test.py file".
func (l languageFacts) testFilePattern() string {
	return "a " + joinWords(l.testFiles, " or ") + " file"
}
