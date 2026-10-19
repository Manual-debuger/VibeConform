package cli

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// The reference implementations pass the static audit (spec 0054, "The
// drift test (#109)"): for each stack, a fixture repository assembled
// from the skill's reference files around a minimal component with one
// test has no failing MUST rule under vibe audit --static --json. The
// static audit runs no tool.

// minimalComponents are the source files of a minimal component with one
// test, by language, relative to the component's directory.
var minimalComponents = map[string]map[string]string{
	"go": {
		"go.mod":      "module example.com/app\n\ngo 1.24\n",
		"app.go":      "package app\n\n// Add returns a + b.\nfunc Add(a, b int) int { return a + b }\n",
		"app_test.go": "package app\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n",
	},
	"python": {
		"pyproject.toml":    "[project]\nname = \"app\"\nversion = \"0.1.0\"\nrequires-python = \">=3.12\"\n",
		"uv.lock":           "version = 1\nrequires-python = \">=3.12\"\n",
		"app/__init__.py":   "def add(a: int, b: int) -> int:\n    return a + b\n",
		"tests/test_app.py": "from app import add\n\n\ndef test_add() -> None:\n    assert add(1, 2) == 3\n",
	},
	"typescript": {
		"package.json":    "{\n  \"name\": \"app\",\n  \"private\": true,\n  \"type\": \"module\"\n}\n",
		"pnpm-lock.yaml":  "lockfileVersion: '9.0'\n",
		"src/add.ts":      "export function add(a: number, b: number): number {\n  return a + b;\n}\n",
		"src/add.test.ts": "import { add } from \"./add\";\n\nif (add(1, 2) !== 3) {\n  throw new Error(\"add(1, 2) !== 3\");\n}\n",
	},
}

// referenceRepo is the fixture repository of stack: every reference file
// the stack uses at its place, with the version placeholder rendered,
// and a minimal component for each component.
func referenceRepo(t *testing.T, stack string) map[string]string {
	t.Helper()
	rendered, err := setupskill.Render("v0.12.0-alpha.1")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, ref := range referenceFiles(setupskill.FS()) {
		if target, ok := referenceTarget(stack, ref); ok {
			files[target] = string(rendered[ref])
		}
	}
	if stack == "monorepo" {
		for lang, dir := range monorepoComponents {
			for name, content := range minimalComponents[lang] {
				files[dir+"/"+name] = content
			}
		}
	} else {
		maps.Copy(files, minimalComponents[stack])
	}
	return files
}

func TestReferenceImplementationsPassTheStaticAudit(t *testing.T) {
	for _, stack := range referenceStacks {
		t.Run(stack, func(t *testing.T) {
			var edits []fixtureEdit
			for name, content := range referenceRepo(t, stack) {
				edits = append(edits, withFile(name, content))
			}
			dir := fixtureFrom(t, t.TempDir(), edits...)
			res := runVibeAudit(t, "--static", "--json", "--repo-root", dir)
			var report struct {
				Rules []struct {
					ID       string          `json:"id"`
					Level    contract.Level  `json:"level"`
					Result   contract.Result `json:"result"`
					Findings []struct {
						File    string `json:"file"`
						Message string `json:"message"`
					} `json:"findings"`
				} `json:"rules"`
			}
			if err := json.Unmarshal([]byte(res.stdout), &report); err != nil {
				t.Fatalf("the report is not JSON: %v\nstdout: %s\nstderr: %s", err, res.stdout, res.stderr)
			}
			if len(report.Rules) == 0 {
				t.Fatalf("the report has no rules\nstdout: %s", res.stdout)
			}
			for _, r := range report.Rules {
				if r.Level == contract.Must && r.Result == contract.ResultFail {
					var msgs []string
					for _, f := range r.Findings {
						msgs = append(msgs, f.File+": "+f.Message)
					}
					t.Errorf("MUST rule %s fails: %s", r.ID, strings.Join(msgs, "; "))
				}
			}
		})
	}
}
