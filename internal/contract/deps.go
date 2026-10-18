package contract

import (
	"errors"
	"io/fs"
	"strings"
)

// "Has dependencies" decides D-01's no-lockfile case (spec 0053, D-01).

// errNoManifest is a component without its language's manifest. V-04's
// component checks report that; D-01 has nothing to decide.
var errNoManifest = errors.New("no manifest")

// hasDependencies reports whether a component's manifest declares
// dependencies, and names the manifest. Go: go.mod has a require
// directive. Python: pyproject.toml declares a non-empty
// [project].dependencies, [project].optional-dependencies,
// [dependency-groups] or [tool.poetry] dependency table, python itself
// not counted. TypeScript: package.json has a non-empty dependencies,
// devDependencies, optionalDependencies or peerDependencies.
func hasDependencies(env *Env, comp Component) (manifest string, deps bool, err error) {
	lang := languageTable[comp.Language]
	manifest = joinRel(comp.Path, lang.manifest)
	data, err := env.ReadFile(manifest)
	if errors.Is(err, fs.ErrNotExist) {
		return manifest, false, errNoManifest
	}
	if err != nil {
		return manifest, false, err
	}
	deps, err = lang.hasDependencies(manifest, data)
	return manifest, deps, err
}

func pyprojectHasDependencies(_ string, data []byte) (bool, error) {
	doc, err := parseTOML(string(data))
	if err != nil {
		return false, err
	}
	return pyprojectDeclaresDependencies(doc), nil
}

func pyprojectDeclaresDependencies(doc map[string]any) bool {
	project, _ := doc["project"].(map[string]any)
	if nonEmptyList(project["dependencies"]) {
		return true
	}
	if anyNonEmptyList(project["optional-dependencies"]) || anyNonEmptyList(doc["dependency-groups"]) {
		return true
	}
	tool, _ := doc["tool"].(map[string]any)
	poetry, _ := tool["poetry"].(map[string]any)
	if poetry == nil {
		return false
	}
	tables := []any{poetry["dependencies"], poetry["dev-dependencies"]}
	if groups, ok := poetry["group"].(map[string]any); ok {
		for _, g := range groups {
			if gm, ok := g.(map[string]any); ok {
				tables = append(tables, gm["dependencies"])
			}
		}
	}
	for _, t := range tables {
		m, _ := t.(map[string]any)
		for name := range m {
			if !strings.EqualFold(name, "python") {
				return true
			}
		}
	}
	return false
}

func nonEmptyList(v any) bool {
	l, ok := v.([]any)
	return ok && len(l) > 0
}

// anyNonEmptyList reports whether a table holds a non-empty list.
func anyNonEmptyList(v any) bool {
	m, _ := v.(map[string]any)
	for _, x := range m {
		if nonEmptyList(x) {
			return true
		}
	}
	return false
}
