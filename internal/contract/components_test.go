package contract_test

import (
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

func compSection(body string) contracttest.Section {
	return contracttest.Section{Key: "components", Body: "components:\n" + body}
}

// Declared components (spec 0052, "Components"; Q11).
func TestContractConfigComponents(t *testing.T) {
	t.Parallel()
	sub := contracttest.WithFile("svc/pyproject.toml", "[project]\nname = \"svc\"\n")
	web := contracttest.WithFile("web/package.json", "{}\n")
	runConfigLoadCases(t, []configLoadCase{
		{name: "valid", config: contracttest.ConfigWith(compSection("  - path: svc\n    language: python\n  - path: web\n    language: typescript\n")),
			edits: []contracttest.Edit{sub, web}},
		{name: "root is valid", config: contracttest.ConfigWith(compSection("  - path: .\n    language: go\n"))},
		{name: "nested path is valid", config: contracttest.ConfigWith(compSection("  - path: svc/api\n    language: python\n")),
			edits: []contracttest.Edit{contracttest.WithFile("svc/api/main.py", "")}},
		{name: "not a list", config: contracttest.ConfigWith(contracttest.Section{Key: "components", Body: "components: {path: svc}\n"}),
			want: []string{"vibe.yaml: components: ", "list"}},
		{name: "empty list", config: contracttest.ConfigWith(contracttest.Section{Key: "components", Body: "components: []\n"}),
			want: []string{"vibe.yaml: components: ", "omit"}},
		{name: "entry not a map", config: contracttest.ConfigWith(compSection("  - svc\n")),
			want: []string{"vibe.yaml: components[0]: ", "path and language"}},
		{name: "unknown field", config: contracttest.ConfigWith(compSection("  - path: svc\n    language: python\n    id: svc\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[0].id: unknown field"}},
		{name: "path missing", config: contracttest.ConfigWith(compSection("  - language: python\n")),
			want: []string{"vibe.yaml: components[0].path: is required"}},
		{name: "language missing", config: contracttest.ConfigWith(compSection("  - path: svc\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[0].language: is required"}},
		{name: "language unknown", config: contracttest.ConfigWith(compSection("  - path: svc\n    language: rust\n")),
			edits: []contracttest.Edit{sub}, want: []string{`vibe.yaml: components[0].language: "rust" is not one of go, python, typescript`}},
		{name: "path not clean", config: contracttest.ConfigWith(compSection("  - path: ./svc\n    language: python\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[0].path: ", "clean", `"svc"`}},
		{name: "path trailing slash", config: contracttest.ConfigWith(compSection("  - path: svc/\n    language: python\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[0].path: ", "clean"}},
		{name: "path absolute", config: contracttest.ConfigWith(compSection("  - path: /svc\n    language: python\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[0].path: ", "relative"}},
		{name: "path with a drive", config: contracttest.ConfigWith(compSection("  - path: C:/svc\n    language: python\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[0].path: ", "relative"}},
		{name: "path with backslashes", config: contracttest.ConfigWith(compSection("  - path: svc\\api\n    language: python\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[0].path: ", "slash"}},
		{name: "path outside", config: contracttest.ConfigWith(compSection("  - path: ../svc\n    language: python\n")),
			want: []string{"vibe.yaml: components[0].path: ", "inside the repository"}},
		{name: "path empty", config: contracttest.ConfigWith(compSection("  - path: \"\"\n    language: python\n")),
			want: []string{"vibe.yaml: components[0].path: "}},
		{name: "path does not exist", config: contracttest.ConfigWith(compSection("  - path: svc\n    language: python\n")),
			want: []string{"vibe.yaml: components[0].path: ", "svc", "tracked"}},
		{name: "path is a file", config: contracttest.ConfigWith(compSection("  - path: go.mod\n    language: go\n")),
			want: []string{"vibe.yaml: components[0].path: ", "tracked"}},
		{name: "path holds only untracked files", config: contracttest.ConfigWith(compSection("  - path: svc\n    language: python\n")),
			edits: []contracttest.Edit{contracttest.WithUntracked("svc/pyproject.toml", "")},
			want:  []string{"vibe.yaml: components[0].path: ", "tracked"}},
		{name: "path twice", config: contracttest.ConfigWith(compSection("  - path: svc\n    language: python\n  - path: svc\n    language: go\n")),
			edits: []contracttest.Edit{sub}, want: []string{"vibe.yaml: components[1].path: ", "components[0]"}},
		{name: "path inside another", config: contracttest.ConfigWith(compSection("  - path: svc\n    language: python\n  - path: svc/api\n    language: go\n")),
			edits: []contracttest.Edit{sub, contracttest.WithFile("svc/api/go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components[1].path: ", "inside components[0]"}},
		{name: "path contains another", config: contracttest.ConfigWith(compSection("  - path: svc/api\n    language: go\n  - path: .\n    language: python\n")),
			edits: []contracttest.Edit{contracttest.WithFile("svc/api/go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components[1].path: ", "contains components[0]"}},
	})
}

// Without components, the root's manifests decide the language.
func TestContractConfigComponentDetection(t *testing.T) {
	t.Parallel()
	noComponents := contracttest.ConfigWith()
	runConfigLoadCases(t, []configLoadCase{
		{name: "go.mod", config: noComponents},
		{name: "pyproject.toml", config: noComponents,
			edits: []contracttest.Edit{contracttest.WithoutFile("go.mod"), contracttest.WithFile("pyproject.toml", "[project]\n")}},
		{name: "package.json", config: noComponents,
			edits: []contracttest.Edit{contracttest.WithoutFile("go.mod"), contracttest.WithFile("package.json", "{}\n")}},
		{name: "no manifest", config: noComponents, edits: []contracttest.Edit{contracttest.WithoutFile("go.mod")},
			want: []string{"vibe.yaml: components: the repository root has no tracked go.mod, pyproject.toml or package.json, " +
				"so its language is unknown; declare components with path and language"}},
		{name: "two manifests", config: noComponents, edits: []contracttest.Edit{contracttest.WithFile("package.json", "{}\n")},
			want: []string{"vibe.yaml: components: the repository root has go.mod and package.json, " +
				"so its language is ambiguous; declare components with path and language"}},
		{name: "three manifests", config: noComponents,
			edits: []contracttest.Edit{contracttest.WithFile("package.json", "{}\n"), contracttest.WithFile("pyproject.toml", "[project]\n")},
			want: []string{"vibe.yaml: components: the repository root has go.mod, pyproject.toml and package.json, " +
				"so its language is ambiguous; declare components with path and language"}},
		// Only tracked files count (spec 0052, "Tracked files only").
		{name: "untracked manifest does not count", config: noComponents,
			edits: []contracttest.Edit{contracttest.WithUntracked("package.json", "{}\n")}},
		{name: "only an untracked manifest", config: noComponents,
			edits: []contracttest.Edit{contracttest.WithoutFile("go.mod"), contracttest.WithUntracked("go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components: ", "declare components"}},
		{name: "manifest below the root does not count", config: noComponents,
			edits: []contracttest.Edit{contracttest.WithoutFile("go.mod"), contracttest.WithFile("svc/go.mod", "module x\n")},
			want:  []string{"vibe.yaml: components: "}},
	})
}
