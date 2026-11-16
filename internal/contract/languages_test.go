package contract

import (
	"slices"
	"testing"
)

// languageFact is one fact every language record must fill, and how to
// tell that a record leaves it empty.
type languageFact struct {
	name    string
	missing func(languageFacts) bool
}

// languageDependentKinds are the defect kinds every language plants with
// a sample of its own. The type-error kind is V-05's, which is Python's
// alone (spec 0066, Out of Scope), and the secret sample is shared.
var languageDependentKinds = []defectKind{
	defectFormat, defectStaticAnalysis, defectTest, defectBlanketSuppression, defectComplexity,
}

var languageFactsRequired = []languageFact{
	{"identifier", func(l languageFacts) bool { return l.id == "" }},
	{"display name", func(l languageFacts) bool { return l.name == "" }},
	{"manifest", func(l languageFacts) bool { return l.manifest == "" }},
	{"dependency reader", func(l languageFacts) bool { return l.hasDependencies == nil }},
	{"lockfile", func(l languageFacts) bool { return len(l.lockfiles) == 0 }},
	{"lockfile with a file, a check and a fix", func(l languageFacts) bool {
		return slices.ContainsFunc(l.lockfiles, func(lf lockfile) bool {
			return lf.file == "" || lf.fix == "" || len(lf.check) == 0 ||
				slices.ContainsFunc(lf.check, func(argv []string) bool { return len(argv) == 0 })
		})
	}},
	{"test file pattern", func(l languageFacts) bool { return len(l.testFiles) == 0 }},
	{"sample for each language-dependent defect kind", func(l languageFacts) bool {
		return slices.ContainsFunc(languageDependentKinds, func(kind defectKind) bool {
			s, ok := l.sample(kind)
			return !ok || s.Name == "" || s.Content == ""
		})
	}},
	{"shared secret sample", func(l languageFacts) bool {
		s, ok := l.sample(defectSecret)
		return !ok || s != secretSample
	}},
	{"complexity layout", func(l languageFacts) bool {
		c := l.complexity
		return c.name == "" || c.head == "" || c.branch == "" || c.tail == ""
	}},
	{"Dependabot ecosystem", func(l languageFacts) bool { return len(l.dependabotEcosystems) == 0 }},
	{"Renovate manager", func(l languageFacts) bool { return len(l.renovateManagers) == 0 }},
}

// Every language record fills every fact (spec 0066, "One internal
// test"), so that a half-added language cannot ship.
func TestLanguageRecordsAreComplete(t *testing.T) {
	t.Parallel()
	if len(languageRegistry) == 0 {
		t.Fatal("the language registry is empty")
	}
	for _, l := range languageRegistry {
		for _, fact := range languageFactsRequired {
			if fact.missing(l) {
				t.Errorf("language %q has no %s", l.id, fact.name)
			}
		}
	}
}

// Identifiers, manifests and lockfile names are unique across languages,
// so that a lookup or a file names one language.
func TestLanguageNamesAreUnique(t *testing.T) {
	t.Parallel()
	owner := map[string]string{}
	claim := func(kind, name, id string) {
		key := kind + " " + name
		if other, ok := owner[key]; ok {
			t.Errorf("%s %q is in both %q and %q", kind, name, other, id)
		}
		owner[key] = id
	}
	for _, l := range languageRegistry {
		claim("identifier", l.id, l.id)
		claim("file", l.manifest, l.id)
		for _, lf := range l.lockfiles {
			claim("file", lf.file, l.id)
		}
	}
}
