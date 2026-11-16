package contract

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// DetectionDepth is how many directories below the root DetectComponents
// looks for a component's manifest.
const DetectionDepth = 3

// detectionSkipped are directory names whose manifests belong to
// dependencies, not to the repository's components.
var detectionSkipped = []string{"node_modules", "vendor"}

// Detection is what DetectComponents finds in the tracked files.
type Detection struct {
	// Components are the components, sorted by path.
	Components []Component
	// Nested holds each directory with a manifest that lies inside one of
	// Components, sorted by path.
	Nested []NestedManifest
	// AuditDetectsSame is true when the audit detects exactly Components
	// from a vibe.yaml without a components key.
	AuditDetectsSame bool
}

// NestedManifest is a directory with a manifest inside another
// component. Components do not nest, so it is not a component of its own.
type NestedManifest struct {
	// Dir is the directory with the manifest.
	Dir string
	// Component is the component Dir lies inside.
	Component Component
}

// DetectComponents finds the components among the tracked files (slash
// separated, relative to the root), reading nothing but the list. A
// component is a directory up to DetectionDepth below the root, and not
// under a dependency directory, that holds a language's manifest and the
// files the language wants beside it. When a directory qualifies for more
// than one language, the first in the contract's language order wins. A
// directory inside another component is not one.
func DetectComponents(files []string) Detection {
	has := make(map[string]bool, len(files))
	for _, f := range files {
		has[f] = true
	}
	var d Detection
	for _, dir := range manifestDirs(files) {
		language := dirLanguage(dir, has)
		if language == "" {
			continue
		}
		c := Component{Path: dir, Language: language}
		if i := slices.IndexFunc(d.Components, func(o Component) bool { return within(dir, o.Path) }); i >= 0 {
			d.Nested = append(d.Nested, NestedManifest{Dir: dir, Component: d.Components[i]})
			continue
		}
		d.Components = append(d.Components, c)
	}
	if len(d.Components) == 1 && d.Components[0].Path == "." {
		language, err := rootLanguage(func(f string) bool { return has[f] })
		d.AuditDetectsSame = err == nil && language == d.Components[0].Language
	}
	return d
}

// manifestDirs returns, sorted, each directory that may hold a component
// and holds a file named like a language's manifest.
func manifestDirs(files []string) []string {
	manifests := map[string]bool{}
	for _, lang := range languageRegistry {
		manifests[lang.manifest] = true
	}
	dirs := map[string]bool{}
	for _, f := range files {
		if dir := path.Dir(f); manifests[path.Base(f)] && componentDir(dir) {
			dirs[dir] = true
		}
	}
	return slices.Sorted(maps.Keys(dirs))
}

// componentDir reports whether dir may hold a component: within
// DetectionDepth of the root and not under a dependency directory.
func componentDir(dir string) bool {
	if dir == "." {
		return true
	}
	parts := strings.Split(dir, "/")
	return len(parts) <= DetectionDepth && !slices.ContainsFunc(parts, func(p string) bool {
		return slices.Contains(detectionSkipped, p)
	})
}

// dirLanguage returns the first language, in the contract's order, whose
// manifest and companions are all in dir, or "" for none.
func dirLanguage(dir string, has map[string]bool) string {
	for _, lang := range languageRegistry {
		wants := append([]string{lang.manifest}, lang.companions...)
		if !slices.ContainsFunc(wants, func(f string) bool { return !has[joinRel(dir, f)] }) {
			return lang.id
		}
	}
	return ""
}

// rootLanguage decides the root component's language as the audit does
// for a vibe.yaml without a components key: from the root manifests that
// has reports tracked, exactly one of which must be there. The error says
// why the root has no language or more than one.
func rootLanguage(has func(file string) bool) (string, error) {
	var all, found []string
	language := ""
	for _, lang := range languageRegistry {
		all = append(all, lang.manifest)
		if has(lang.manifest) {
			found = append(found, lang.manifest)
			language = lang.id
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("the repository root has no tracked %s, so its language is unknown; "+
			"declare components with path and language", joinWords(all, " or "))
	case 1:
		return language, nil
	}
	return "", fmt.Errorf("the repository root has %s, so its language is ambiguous; "+
		"declare components with path and language", joinWords(found, " and "))
}

// ManifestPhrase names, for messages, the files that make a directory a
// component: "go.mod, pyproject.toml, or package.json beside a
// tsconfig.json".
func ManifestPhrase() string {
	items := make([]string, 0, len(languageRegistry))
	for _, lang := range languageRegistry {
		item := lang.manifest
		if len(lang.companions) > 0 {
			item += " beside a " + strings.Join(lang.companions, " and a ")
		}
		items = append(items, item)
	}
	return joinWords(items, ", or ")
}
