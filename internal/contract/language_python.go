package contract

import "strings"

// pythonLanguage is Python's record.
var pythonLanguage = languageFacts{
	id:              LanguagePython,
	name:            "Python",
	manifest:        "pyproject.toml",
	hasDependencies: pyprojectHasDependencies,
	lockfiles: []lockfile{
		{file: "uv.lock", check: [][]string{{"uv", "lock", "--check"}}, fix: "uv lock"},
		{file: "poetry.lock", check: [][]string{{"poetry", "check", "--lock"}}, fix: "poetry lock"},
	},
	testFiles: []string{"test_*.py", "*_test.py"},
	samples: map[defectKind]defectSample{
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
		defectSecret: secretSample,
	},
	complexity: complexityLayout{
		name: "vibe_probe_complexity.py",
		head: complexityHead("#") +
			"def vibe_probe_complexity(n: int) -> int:\n    total = 0\n",
		branch: "    if n > %d:\n        total += %d\n",
		tail:   "    return total\n",
	},
	dependabotEcosystems: []string{"pip", "uv"},
	renovateManagers:     []string{"pep621", "poetry", "pip_requirements", "pip-compile", "pipenv", "setup-cfg", "pip_setup"},
}

// pyprojectHasDependencies reports whether pyproject.toml declares a
// non-empty [project].dependencies, [project].optional-dependencies,
// [dependency-groups] or [tool.poetry] dependency table, python itself
// not counted.
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
