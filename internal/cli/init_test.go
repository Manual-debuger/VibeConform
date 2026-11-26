package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// runInit runs vibe init in dir with the audit's repository seam, so that
// a fixture built without git lists its files the same way for both.
func runInit(t *testing.T, dir string) (string, error) {
	t.Helper()
	root := newRootCmd("test", cliSeams{openRepo: contracttest.OpenRepo})
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
	withoutGo = []contracttest.Edit{contracttest.WithoutFile("vibe.yaml"), contracttest.WithoutFile("go.mod"), contracttest.WithoutFile("fixture_test.go")}
	pyFiles   = []contracttest.Edit{contracttest.WithFile("pyproject.toml", "[project]\nname = \"calc\"\nversion = \"0.1.0\"\n"), contracttest.WithFile("calc.py", "def add(a, b):\n    return a + b\n")}
	tsFiles   = []contracttest.Edit{contracttest.WithFile("package.json", "{\"name\": \"calc\", \"private\": true}\n"), contracttest.WithFile("tsconfig.json", "{}\n"), contracttest.WithFile("calc.ts", "export const add = (a: number, b: number) => a + b;\n")}
)

func under(dir string, edits []contracttest.Edit) []contracttest.Edit {
	return contracttest.Under(dir, edits...)
}

// monorepo is a Go, a Python and a TypeScript component below the root,
// beside manifests that are not components: a package.json without
// tsconfig.json, manifests of dependencies, a manifest below the depth
// init looks at, and a manifest inside another component.
var monorepo = slices.Concat(withoutGo,
	[]contracttest.Edit{contracttest.WithFile("services/api/go.mod", "module example.com/api\n\ngo 1.24\n")},
	under("tools/py", pyFiles),
	under("web/app", tsFiles),
	[]contracttest.Edit{
		contracttest.Dependabot("gomod directory /services/api", "pip directory /tools/py", "npm directory /web/app", "github-actions directory /"),
		contracttest.WithFile("docs/site/package.json", "{}\n"),
		contracttest.WithFile("node_modules/dep/package.json", "{}\n"),
		contracttest.WithFile("node_modules/dep/tsconfig.json", "{}\n"),
		contracttest.WithFile("third/vendor/x/go.mod", "module x\n"),
		contracttest.WithFile("a/b/c/d/go.mod", "module d\n"),
		contracttest.WithFile("services/api/tools/go.mod", "module tools\n"),
	})

// TestInitWritesAConfigTheAuditReads: on each fixture vibe init writes a
// version 2 vibe.yaml with the components it finds, which the audit's
// parser reads. The audit then fails V-04 for the four undeclared
// entrypoints, and every other failure only sends the reader to V-04
// (spec 0059, Desired Behavior 10).
func TestInitWritesAConfigTheAuditReads(t *testing.T) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	t.Parallel()
	for _, c := range []struct {
		name          string
		edits         []contracttest.Edit
		want          []contract.Component
		componentsKey bool
	}{
		{"go at the root", []contracttest.Edit{contracttest.WithoutFile("vibe.yaml")}, []contract.Component{{Path: ".", Language: "go"}}, false},
		{"python", slices.Concat(withoutGo, pyFiles, []contracttest.Edit{contracttest.Dependabot("pip directory /", "github-actions directory /")}), []contract.Component{{Path: ".", Language: "python"}}, false},
		{"typescript", slices.Concat(withoutGo, tsFiles, []contracttest.Edit{contracttest.Dependabot("npm directory /", "github-actions directory /")}), []contract.Component{{Path: ".", Language: "typescript"}}, false},
		{"monorepo", monorepo, []contract.Component{
			{Path: "services/api", Language: "go"},
			{Path: "tools/py", Language: "python"},
			{Path: "web/app", Language: "typescript"},
		}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := contracttest.Fixture(t, contracttest.Baseline, c.edits...)
			out, err := runInit(t, dir)
			if err != nil {
				t.Fatalf("vibe init: %v\n%s", err, out)
			}
			written := readVibeYAML(t, dir)
			for _, want := range []string{"contract: " + contract.Initial + "\n", "platforms: [linux, windows]\n", "agents: [claude]\n",
				"#   verify: ", "#   verify-fast: ", "#   fmt: ", "#   fmt-check: "} {
				if !strings.Contains(written, want) {
					t.Errorf("vibe.yaml lacks %q:\n%s", want, written)
				}
			}
			if got := strings.Contains(written, "\ncomponents:"); got != c.componentsKey {
				t.Errorf("components key written = %v, want %v:\n%s", got, c.componentsKey, written)
			}

			repo, err := contracttest.OpenRepo(context.Background(), dir)
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

			rep := auditJSON(t, dir, "--static")
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

// TestInitPrintsWhatItWrote: vibe init prints the file it wrote, each
// component, each manifest it skipped and the next step, byte for byte as
// before spec 0066.
func TestInitPrintsWhatItWrote(t *testing.T) {
	t.Parallel()
	const next = "next: declare the entrypoints in vibe.yaml; until then vibe audit fails V-04\n"
	for _, c := range []struct {
		name  string
		edits []contracttest.Edit
		want  string
	}{
		{"go at the root", []contracttest.Edit{contracttest.WithoutFile("vibe.yaml")},
			"wrote vibe.yaml\ncomponent . (go)\n" + next},
		{"monorepo", monorepo,
			"wrote vibe.yaml\n" +
				"component services/api (go)\n" +
				"component tools/py (python)\n" +
				"component web/app (typescript)\n" +
				"skipped services/api/tools: inside the component services/api (go); components do not nest\n" +
				next},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := contracttest.Fixture(t, contracttest.Baseline, c.edits...)
			out, err := runInit(t, dir)
			if err != nil {
				t.Fatalf("vibe init: %v\n%s", err, out)
			}
			if out != c.want {
				t.Errorf("output =\n%s\nwant\n%s", out, c.want)
			}
		})
	}
}

// failingWriter fails each write that starts with prefix and takes every
// other write.
type failingWriter struct{ prefix string }

var errOutput = errors.New("output closed")

func (w failingWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p), w.prefix) {
		return 0, errOutput
	}
	return len(p), nil
}

// TestInitOutputErrors: when printing fails after vibe.yaml is written,
// init returns the error, prefixed with "init: " for every line but the
// last, the next step, whose error it returns as it is, as before spec
// 0066.
func TestInitOutputErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ prefix, want string }{
		{"wrote ", "init: " + errOutput.Error()},
		{"component ", "init: " + errOutput.Error()},
		{"next: ", errOutput.Error()},
	} {
		t.Run(c.prefix, func(t *testing.T) {
			t.Parallel()
			dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithoutFile("vibe.yaml"))
			root := newRootCmd("test", cliSeams{openRepo: contracttest.OpenRepo})
			root.SetOut(failingWriter{c.prefix})
			root.SetErr(io.Discard)
			root.SetArgs([]string{"init", "--repo-root", dir})
			err := root.Execute()
			if !errors.Is(err, errOutput) || err.Error() != c.want {
				t.Errorf("error = %v, want %q", err, c.want)
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
			dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", c.content))
			before := contracttest.TreeHash(t, dir)
			out, err := runInit(t, dir)
			if code := ExitCode(err); code != exitError {
				t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if after := contracttest.TreeHash(t, dir); !maps.Equal(before, after) {
				t.Error("the refused init changed the repository")
			}
		})
	}
}

// TestInitSaysWhyANestedManifestIsNoComponent: a manifest inside another
// component is not a component, since the contract forbids nesting, and
// init prints one line per such directory saying so.
func TestInitSaysWhyANestedManifestIsNoComponent(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithoutFile("vibe.yaml"),
		contracttest.WithFile("tools/go.mod", "module tools\n"),
		contracttest.WithFile("web/package.json", "{}\n"), contracttest.WithFile("web/tsconfig.json", "{}\n"))
	out, err := runInit(t, dir)
	if err != nil {
		t.Fatalf("vibe init: %v\n%s", err, out)
	}
	for _, want := range []string{
		"skipped tools: inside the component . (go); components do not nest\n",
		"skipped web: inside the component . (go); components do not nest\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(readVibeYAML(t, dir), "\ncomponents:") {
		t.Errorf("the nested manifests became components:\n%s", readVibeYAML(t, dir))
	}
}

// TestInitRaceLostIsAnError: when the exclusive create finds a vibe.yaml
// that is gone again by the time init reads it, init wrote nothing and
// must say so with an error, not exit 0 (regression: refuseExisting
// returned nil for a missing file).
func TestInitRaceLostIsAnError(t *testing.T) {
	t.Parallel()
	err := createRefused(t.TempDir())
	if code := ExitCode(err); code != exitError {
		t.Fatalf("exit %d, want %d: %v", code, exitError, err)
	}
	if err == nil || !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("error = %v, want one saying nothing was written", err)
	}
}

// TestInitNeedsAComponent: with no manifest in the tracked files, vibe
// init says what it looked for, byte for byte as before spec 0066, and
// writes nothing, since the audit could not read a vibe.yaml without
// components.
func TestInitNeedsAComponent(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, withoutGo...)
	before := contracttest.TreeHash(t, dir)
	out, err := runInit(t, dir)
	if code := ExitCode(err); code != exitError {
		t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
	}
	const want = "init: found no component in the tracked files: no go.mod, pyproject.toml, " +
		"or package.json beside a tsconfig.json within 3 directories of the root; " +
		"commit the project's manifest and run vibe init again"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	if after := contracttest.TreeHash(t, dir); !maps.Equal(before, after) {
		t.Error("the failed init changed the repository")
	}
}

// TestInitHelpNamesTheManifests: vibe init --help names the manifests
// with the same phrase as the "found no component" error (spec 0066).
func TestInitHelpNamesTheManifests(t *testing.T) {
	t.Parallel()
	root := newRootCmd("test", cliSeams{openRepo: contracttest.OpenRepo})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"init", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("vibe init --help: %v", err)
	}
	if want := "(go.mod, pyproject.toml, or package.json beside a tsconfig.json)"; !strings.Contains(out.String(), want) {
		t.Errorf("help lacks %q:\n%s", want, out.String())
	}
}

// TestInitDeclaresAnAmbiguousRoot: a root with two manifests is one
// component, of the first language init checks, and is declared, since
// the audit detects a root's language only from a single manifest.
func TestInitDeclaresAnAmbiguousRoot(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, slices.Concat([]contracttest.Edit{contracttest.WithoutFile("vibe.yaml")}, tsFiles)...)
	if out, err := runInit(t, dir); err != nil {
		t.Fatalf("vibe init: %v\n%s", err, out)
	}
	if got, want := readVibeYAML(t, dir), "components:\n  - path: \".\"\n    language: go\n"; !strings.Contains(got, want) {
		t.Errorf("vibe.yaml lacks %q:\n%s", want, got)
	}
}
