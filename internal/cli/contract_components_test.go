package cli

import "testing"

func components(body string) section { return section{"components", "components:\n" + body} }

// Declared components (spec 0052, "Components"; Q11).
func TestContractConfigComponents(t *testing.T) {
	sub := withFile("svc/pyproject.toml", "[project]\nname = \"svc\"\n")
	web := withFile("web/package.json", "{}\n")
	runConfigCases(t, []configCase{
		{name: "valid", config: configWith(components("  - path: svc\n    language: python\n  - path: web\n    language: typescript\n")),
			edits: []fixtureEdit{sub, web}},
		{name: "root is valid", config: configWith(components("  - path: .\n    language: go\n"))},
		{name: "nested path is valid", config: configWith(components("  - path: svc/api\n    language: python\n")),
			edits: []fixtureEdit{withFile("svc/api/main.py", "")}},
		{name: "not a list", config: configWith(section{"components", "components: {path: svc}\n"}),
			want: []string{"vibe.yaml: components: ", "list"}},
		{name: "empty list", config: configWith(section{"components", "components: []\n"}),
			want: []string{"vibe.yaml: components: ", "omit"}},
		{name: "entry not a map", config: configWith(components("  - svc\n")),
			want: []string{"vibe.yaml: components[0]: ", "path and language"}},
		{name: "unknown field", config: configWith(components("  - path: svc\n    language: python\n    id: svc\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].id: unknown field"}},
		{name: "path missing", config: configWith(components("  - language: python\n")),
			want: []string{"vibe.yaml: components[0].path: is required"}},
		{name: "language missing", config: configWith(components("  - path: svc\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].language: is required"}},
		{name: "language unknown", config: configWith(components("  - path: svc\n    language: rust\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].language: ", "rust", "go, python, typescript"}},
		{name: "path not clean", config: configWith(components("  - path: ./svc\n    language: python\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].path: ", "clean", `"svc"`}},
		{name: "path trailing slash", config: configWith(components("  - path: svc/\n    language: python\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].path: ", "clean"}},
		{name: "path absolute", config: configWith(components("  - path: /svc\n    language: python\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].path: ", "relative"}},
		{name: "path with a drive", config: configWith(components("  - path: C:/svc\n    language: python\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].path: ", "relative"}},
		{name: "path with backslashes", config: configWith(components("  - path: svc\\api\n    language: python\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[0].path: ", "slash"}},
		{name: "path outside", config: configWith(components("  - path: ../svc\n    language: python\n")),
			want: []string{"vibe.yaml: components[0].path: ", "inside the repository"}},
		{name: "path empty", config: configWith(components("  - path: \"\"\n    language: python\n")),
			want: []string{"vibe.yaml: components[0].path: "}},
		{name: "path does not exist", config: configWith(components("  - path: svc\n    language: python\n")),
			want: []string{"vibe.yaml: components[0].path: ", "svc", "tracked"}},
		{name: "path is a file", config: configWith(components("  - path: go.mod\n    language: go\n")),
			want: []string{"vibe.yaml: components[0].path: ", "tracked"}},
		{name: "path holds only untracked files", config: configWith(components("  - path: svc\n    language: python\n")),
			edits: []fixtureEdit{withUntracked("svc/pyproject.toml", "")},
			want:  []string{"vibe.yaml: components[0].path: ", "tracked"}},
		{name: "path twice", config: configWith(components("  - path: svc\n    language: python\n  - path: svc\n    language: go\n")),
			edits: []fixtureEdit{sub}, want: []string{"vibe.yaml: components[1].path: ", "components[0]"}},
		{name: "path inside another", config: configWith(components("  - path: svc\n    language: python\n  - path: svc/api\n    language: go\n")),
			edits: []fixtureEdit{sub, withFile("svc/api/go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components[1].path: ", "inside components[0]"}},
		{name: "path contains another", config: configWith(components("  - path: svc/api\n    language: go\n  - path: .\n    language: python\n")),
			edits: []fixtureEdit{withFile("svc/api/go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components[1].path: ", "contains components[0]"}},
	})
}

// Without components, the root's manifests decide the language.
func TestContractConfigComponentDetection(t *testing.T) {
	noComponents := configWith()
	runConfigCases(t, []configCase{
		{name: "go.mod", config: noComponents},
		{name: "pyproject.toml", config: noComponents,
			edits: []fixtureEdit{withoutFile("go.mod"), withFile("pyproject.toml", "[project]\n")}},
		{name: "package.json", config: noComponents,
			edits: []fixtureEdit{withoutFile("go.mod"), withFile("package.json", "{}\n")}},
		{name: "no manifest", config: noComponents, edits: []fixtureEdit{withoutFile("go.mod")},
			want: []string{"vibe.yaml: components: ", "go.mod, pyproject.toml or package.json", "declare components"}},
		{name: "two manifests", config: noComponents, edits: []fixtureEdit{withFile("package.json", "{}\n")},
			want: []string{"vibe.yaml: components: ", "go.mod and package.json", "declare components"}},
		{name: "three manifests", config: noComponents,
			edits: []fixtureEdit{withFile("package.json", "{}\n"), withFile("pyproject.toml", "[project]\n")},
			want:  []string{"vibe.yaml: components: ", "go.mod, pyproject.toml and package.json"}},
		// Only tracked files count (spec 0052, "Tracked files only").
		{name: "untracked manifest does not count", config: noComponents,
			edits: []fixtureEdit{withUntracked("package.json", "{}\n")}},
		{name: "only an untracked manifest", config: noComponents,
			edits: []fixtureEdit{withoutFile("go.mod"), withUntracked("go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components: ", "declare components"}},
		{name: "manifest below the root does not count", config: noComponents,
			edits: []fixtureEdit{withoutFile("go.mod"), withFile("svc/go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components: "}},
	})
}
