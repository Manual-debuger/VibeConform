package githubmono

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

var components = []manifest.Component{
	{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
	{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
	{ID: "worker-2", Path: "services/worker", Profile: manifest.ProfilePy},
}

func resolve(t *testing.T, cs []manifest.Component) []resource.Resource {
	t.Helper()
	rs, err := New().Resolve(context.Background(), &module.Context{Components: cs})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return rs
}

type step struct {
	Name             string            `yaml:"name"`
	If               string            `yaml:"if"`
	Run              string            `yaml:"run"`
	WorkingDirectory string            `yaml:"working-directory"`
	With             map[string]string `yaml:"with"`
}

type workflow struct {
	Env  map[string]string `yaml:"env"`
	Jobs map[string]struct {
		Needs []string `yaml:"needs"`
		Steps []step   `yaml:"steps"`
	} `yaml:"jobs"`
}

func parseCI(t *testing.T, cs []manifest.Component) (workflow, string) {
	t.Helper()
	rs := resolve(t, cs)
	if rs[0].Path != ".github/workflows/ci.yml" {
		t.Fatalf("first resource is %s", rs[0].Path)
	}
	var wf workflow
	if err := yaml.Unmarshal(rs[0].Content, &wf); err != nil {
		t.Fatalf("parsing ci.yml: %v\n%s", err, rs[0].Content)
	}
	return wf, string(rs[0].Content)
}

func TestResolveOrder(t *testing.T) {
	var got []string
	for _, r := range resolve(t, components) {
		got = append(got, r.Path)
	}
	want := []string{".github/workflows/ci.yml", ".github/dependabot.yml", ".github/pull_request_template.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths %v, want %v", got, want)
	}
}

func TestResolveNeedsComponents(t *testing.T) {
	if _, err := New().Resolve(context.Background(), nil); err == nil {
		t.Fatal("Resolve succeeded with no components")
	}
}

// TestJobPerComponent: every component has a job named by its id that
// installs its dependencies in its directory and runs its verify.
func TestJobPerComponent(t *testing.T) {
	wf, _ := parseCI(t, components)
	for _, c := range components {
		job, ok := wf.Jobs[c.ID]
		if !ok {
			t.Errorf("no job %s", c.ID)
			continue
		}
		runs := map[string]step{}
		for _, s := range job.Steps {
			runs[s.Run] = s
		}
		if _, ok := runs["task "+c.ID+":verify"]; !ok {
			t.Errorf("job %s never runs task %s:verify", c.ID, c.ID)
		}
		switch c.Profile {
		case manifest.ProfileGo:
			if _, ok := runs["task "+c.ID+":test:race"]; !ok {
				t.Errorf("go job %s never runs test:race", c.ID)
			}
		case manifest.ProfileTS:
			if s := runs["pnpm install --frozen-lockfile"]; s.WorkingDirectory != c.Path {
				t.Errorf("ts job %s installs in %q, want %q", c.ID, s.WorkingDirectory, c.Path)
			}
		case manifest.ProfilePy:
			if s := runs["uv sync"]; s.WorkingDirectory != c.Path {
				t.Errorf("py job %s installs in %q, want %q", c.ID, s.WorkingDirectory, c.Path)
			}
		}
	}
}

// TestGateNeedsEveryJob mirrors github-ci's check: a job the gate does not
// require could fail without failing CI.
func TestGateNeedsEveryJob(t *testing.T) {
	wf, raw := parseCI(t, components)
	gate := wf.Jobs["gate"]
	for name := range wf.Jobs {
		if name == "gate" {
			continue
		}
		found := false
		for _, need := range gate.Needs {
			found = found || need == name
		}
		if !found {
			t.Errorf("gate does not need job %q", name)
		}
		if !strings.Contains(raw, "needs."+name+".result") {
			t.Errorf("gate never checks needs.%s.result", name)
		}
	}
	if len(gate.Needs) != len(wf.Jobs)-1 {
		t.Errorf("gate needs %v, but ci.yml has %d other jobs", gate.Needs, len(wf.Jobs)-1)
	}
}

// TestCIWorkflowCarriesNoConformance pins spec 0022 for prod-mono.
func TestCIWorkflowCarriesNoConformance(t *testing.T) {
	_, raw := parseCI(t, components)
	if strings.Contains(raw, "vibe") || strings.Contains(raw, "conformance") {
		t.Error("ci.yml mentions vibe or conformance; both belong in conformance.yml (spec 0022)")
	}
}

// TestEnvFollowsProfiles: only the toolchain versions a repository uses.
func TestEnvFollowsProfiles(t *testing.T) {
	wf, _ := parseCI(t, components[2:])
	for _, k := range []string{"GO_VERSION", "TASK_VERSION", "PYTHON_VERSION"} {
		if _, ok := wf.Env[k]; !ok {
			t.Errorf("py-only ci.yml has no %s", k)
		}
	}
	for _, k := range []string{"NODE_VERSION", "GOLANGCI_LINT_VERSION"} {
		if _, ok := wf.Env[k]; ok {
			t.Errorf("py-only ci.yml sets %s", k)
		}
	}
}

// TestGoTestNeverReplaysCachedResults pins issue #64: setup-go restores
// GOCACHE, which also holds test results, so a Go component's CI needs
// -count=1 to run its tests rather than report cached passes.
func TestGoTestNeverReplaysCachedResults(t *testing.T) {
	wf, _ := parseCI(t, components)
	if got := wf.Env["GOFLAGS"]; got != "-count=1" {
		t.Errorf("ci.yml env GOFLAGS = %q, want -count=1", got)
	}
	wf, _ = parseCI(t, components[1:])
	if _, ok := wf.Env["GOFLAGS"]; ok {
		t.Error("a ci.yml with no Go component sets GOFLAGS")
	}
}

func TestDependabotPerComponent(t *testing.T) {
	var doc struct {
		Updates []struct {
			Ecosystem string `yaml:"package-ecosystem"`
			Directory string `yaml:"directory"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal(resolve(t, components)[1].Content, &doc); err != nil {
		t.Fatal(err)
	}
	type u struct{ eco, dir string }
	var got []u
	for _, x := range doc.Updates {
		got = append(got, u{x.Ecosystem, x.Directory})
	}
	want := []u{{"gomod", "/services/api"}, {"npm", "/apps/web"}, {"pip", "/services/worker"}, {"github-actions", "/"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("updates %v, want %v", got, want)
	}
}
