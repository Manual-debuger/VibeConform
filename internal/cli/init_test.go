package cli

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// runInit runs vibe init in dir with the audit's repository seam, so that
// a fixture built without git lists its files the same way for both.
func runInit(t *testing.T, dir string) (string, error) {
	t.Helper()
	root := newRootCmd("test", contractSeams{openRepo: openFixtureRepo})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"init", "--repo-root", dir})
	err := root.Execute()
	return out.String(), err
}

// readVibeYAML returns the fixture's vibe.yaml.
func readVibeYAML(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "vibe.yaml"))
	if err != nil {
		t.Fatalf("vibe init wrote no vibe.yaml: %v", err)
	}
	return string(data)
}

// The undeclared-entrypoint finding of a rule that needs an entrypoint:
// it sends the reader to V-04.
const fixV04First = "fix V-04 first"

// The fixtures below swap the baseline's Go project for another layout.
var (
	withoutGo = []fixtureEdit{withoutFile("vibe.yaml"), withoutFile("go.mod"), withoutFile("fixture_test.go")}
	pyFiles   = []fixtureEdit{withFile("pyproject.toml", "[project]\nname = \"calc\"\nversion = \"0.1.0\"\n"), withFile("calc.py", "def add(a, b):\n    return a + b\n")}
	tsFiles   = []fixtureEdit{withFile("package.json", "{\"name\": \"calc\", \"private\": true}\n"), withFile("tsconfig.json", "{}\n"), withFile("calc.ts", "export const add = (a: number, b: number) => a + b;\n")}
)

func under(dir string, edits []fixtureEdit) []fixtureEdit {
	out := make([]fixtureEdit, len(edits))
	for i, e := range edits {
		e.path = dir + "/" + e.path
		out[i] = e
	}
	return out
}

// TestInitWritesAConfigTheAuditReads: on each fixture vibe init writes a
// version 2 vibe.yaml with the components it finds, which the audit's
// parser reads. The audit then fails V-04 for the four undeclared
// entrypoints, and every other failure only sends the reader to V-04
// (spec 0059, Desired Behavior 10).
func TestInitWritesAConfigTheAuditReads(t *testing.T) {
	t.Parallel()
	monorepo := slices.Concat(withoutGo,
		[]fixtureEdit{withFile("services/api/go.mod", "module example.com/api\n\ngo 1.24\n")},
		under("tools/py", pyFiles),
		under("web/app", tsFiles),
		// Not components: a package.json without tsconfig.json, manifests
		// of dependencies, a manifest below the depth init looks at, and a
		// manifest inside another component.
		[]fixtureEdit{
			dependabot("gomod directory /services/api", "pip directory /tools/py", "npm directory /web/app", "github-actions directory /"),
			withFile("docs/site/package.json", "{}\n"),
			withFile("node_modules/dep/package.json", "{}\n"),
			withFile("node_modules/dep/tsconfig.json", "{}\n"),
			withFile("third/vendor/x/go.mod", "module x\n"),
			withFile("a/b/c/d/go.mod", "module d\n"),
			withFile("services/api/tools/go.mod", "module tools\n"),
		})
	for _, c := range []struct {
		name          string
		edits         []fixtureEdit
		want          []contract.Component
		componentsKey bool
	}{
		{"go at the root", []fixtureEdit{withoutFile("vibe.yaml")}, []contract.Component{{Path: ".", Language: "go"}}, false},
		{"python", slices.Concat(withoutGo, pyFiles, []fixtureEdit{dependabot("pip directory /", "github-actions directory /")}), []contract.Component{{Path: ".", Language: "python"}}, false},
		{"typescript", slices.Concat(withoutGo, tsFiles, []fixtureEdit{dependabot("npm directory /", "github-actions directory /")}), []contract.Component{{Path: ".", Language: "typescript"}}, false},
		{"monorepo", monorepo, []contract.Component{
			{Path: "services/api", Language: "go"},
			{Path: "tools/py", Language: "python"},
			{Path: "web/app", Language: "typescript"},
		}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := contractFixture(t, c.edits...)
			out, err := runInit(t, dir)
			if err != nil {
				t.Fatalf("vibe init: %v\n%s", err, out)
			}
			written := readVibeYAML(t, dir)
			for _, want := range []string{"contract: " + contract.Implemented + "\n", "platforms: [linux, windows]\n", "agents: [claude]\n",
				"#   verify: ", "#   verify-fast: ", "#   fmt: ", "#   fmt-check: "} {
				if !strings.Contains(written, want) {
					t.Errorf("vibe.yaml lacks %q:\n%s", want, written)
				}
			}
			if got := strings.Contains(written, "\ncomponents:"); got != c.componentsKey {
				t.Errorf("components key written = %v, want %v:\n%s", got, c.componentsKey, written)
			}

			repo, err := openFixtureRepo(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := contract.Parse("vibe.yaml", []byte(written), repo)
			if err != nil {
				t.Fatalf("the written vibe.yaml does not parse: %v\n%s", err, written)
			}
			if !slices.Equal(cfg.Components, c.want) {
				t.Errorf("components = %v, want %v", cfg.Components, c.want)
			}

			rep, _ := auditJSON(t, dir, "--static")
			assertRule(t, rep, "V-04", "fail", "entrypoints.verify ", "entrypoints.verify-fast ", "entrypoints.fmt ", "entrypoints.fmt-check ")
			for _, r := range rep.Rules {
				if r.ID == "V-04" || r.Result == "pass" || r.Result == "not_applicable" || r.Result == "not_run" {
					continue
				}
				if len(r.Findings) == 0 {
					t.Errorf("%s = %s with no finding", r.ID, r.Result)
				}
				for _, f := range r.Findings {
					if !strings.Contains(f.Fix, fixV04First) {
						t.Errorf("%s = %s for another reason than V-04: %s %s (fix: %s)", r.ID, r.Result, f.File, f.Message, f.Fix)
					}
				}
			}
		})
	}
}

// TestInitRefusesAnExistingConfig: vibe init never overwrites vibe.yaml,
// of either version, and says why; a version 1 file gets the migration
// pointer.
func TestInitRefusesAnExistingConfig(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, content string
		want          []string
	}{
		{"version 2", "contract: 1.1.0\nplatforms: [linux]\nagents: []\n", []string{"vibe.yaml exists", "does not overwrite"}},
		{"version 1", "standard: prod-go\nversion: v1\n", []string{"vibe.yaml exists", "version 1", "does not overwrite", migrationGuidePath}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := contractFixture(t, withFile("vibe.yaml", c.content))
			before := treeHash(t, dir)
			out, err := runInit(t, dir)
			if code := ExitCode(err); code != exitError {
				t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if after := treeHash(t, dir); !maps.Equal(before, after) {
				t.Error("the refused init changed the repository")
			}
		})
	}
}

// TestInitRaceLostIsAnError: when the exclusive create finds a vibe.yaml
// that is gone again by the time init reads it, init wrote nothing and
// must say so with an error, not exit 0 (regression: refuseExisting
// returned nil for a missing file).
func TestInitRaceLostIsAnError(t *testing.T) {
	t.Parallel()
	err := createRefused(filepath.Join(t.TempDir(), "vibe.yaml"))
	if code := ExitCode(err); code != exitError {
		t.Fatalf("exit %d, want %d: %v", code, exitError, err)
	}
	if err == nil || !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("error = %v, want one saying nothing was written", err)
	}
}

// TestInitNeedsAComponent: with no manifest in the tracked files, vibe
// init says what it looked for and writes nothing, since the audit could
// not read a vibe.yaml without components.
func TestInitNeedsAComponent(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, withoutGo...)
	before := treeHash(t, dir)
	out, err := runInit(t, dir)
	if code := ExitCode(err); code != exitError {
		t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
	}
	for _, want := range []string{"go.mod", "pyproject.toml", "tsconfig.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if after := treeHash(t, dir); !maps.Equal(before, after) {
		t.Error("the failed init changed the repository")
	}
}

// TestInitDeclaresAnAmbiguousRoot: a root with two manifests is one
// component, of the first language init checks, and is declared, since
// the audit detects a root's language only from a single manifest.
func TestInitDeclaresAnAmbiguousRoot(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, slices.Concat([]fixtureEdit{withoutFile("vibe.yaml")}, tsFiles)...)
	if out, err := runInit(t, dir); err != nil {
		t.Fatalf("vibe init: %v\n%s", err, out)
	}
	if got, want := readVibeYAML(t, dir), "components:\n  - path: \".\"\n    language: go\n"; !strings.Contains(got, want) {
		t.Errorf("vibe.yaml lacks %q:\n%s", want, got)
	}
}
