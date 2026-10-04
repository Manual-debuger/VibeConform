package gitlabmono

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/githubmono"
	"github.com/Manual-debuger/VibeConform/internal/module/conformance"
)

var components = []manifest.Component{
	{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
	{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
	{ID: "worker", Path: "services/worker", Profile: manifest.ProfilePy},
}

func gitlabContext(cs []manifest.Component) *module.Context {
	return &module.Context{Components: cs, Policies: map[string]string{manifest.CIProvider: module.CIGitLab}}
}

type job struct {
	Stage         string           `yaml:"stage"`
	Image         string           `yaml:"image"`
	Needs         *[]string        `yaml:"needs"`
	Rules         []map[string]any `yaml:"rules"`
	AllowFailure  *bool            `yaml:"allow_failure"`
	BeforeScript  *[]string        `yaml:"before_script"`
	Script        []string         `yaml:"script"`
	Interruptible *bool            `yaml:"interruptible"`
	Trigger       map[string]any   `yaml:"trigger"`
}

func resolvePipeline(t *testing.T, cs []manifest.Component) (string, map[string]yaml.Node) {
	t.Helper()
	rs, err := New().Resolve(context.Background(), gitlabContext(cs))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Path != PipelinePath || rs[1].Path != MergeRequestTemplatePath {
		t.Fatalf("resolved %v", rs)
	}
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(rs[0].Content, &doc); err != nil {
		t.Fatalf("parsing %s: %v\n%s", PipelinePath, err, rs[0].Content)
	}
	return string(rs[0].Content), doc
}

func decodeJob(t *testing.T, doc map[string]yaml.Node, name string) job {
	t.Helper()
	n, ok := doc[name]
	if !ok {
		t.Fatalf("no job %s", name)
	}
	var j job
	if err := n.Decode(&j); err != nil {
		t.Fatalf("job %s: %v", name, err)
	}
	return j
}

// TestResolvesNothingUnlessGitLab: the module is silent under the default
// and under none.
func TestResolvesNothingUnlessGitLab(t *testing.T) {
	for _, mctx := range []*module.Context{
		nil,
		{Components: components},
		{Components: components, Policies: map[string]string{manifest.CIProvider: module.CIGitHub}},
		{Components: components, Policies: map[string]string{manifest.CIProvider: module.CINone}},
	} {
		if rs, err := New().Resolve(context.Background(), mctx); err != nil || rs != nil {
			t.Errorf("resolved %v, %v", rs, err)
		}
	}
}

// TestPipelineStructure pins spec 0038 acceptance criterion 4.
func TestPipelineStructure(t *testing.T) {
	content, doc := resolvePipeline(t, components)

	images := map[string]string{"api": "golang:${GO_VERSION}", "web": "node:${NODE_VERSION}", "worker": "ghcr.io/astral-sh/uv:python${PYTHON_VERSION}-bookworm-slim"}
	for _, c := range components {
		j := decodeJob(t, doc, c.ID)
		if j.Image != images[c.ID] {
			t.Errorf("%s: image %q, want %q", c.ID, j.Image, images[c.ID])
		}
		if !slices.Contains(j.Script, "task "+c.ID+":verify") {
			t.Errorf("%s: script %v does not run task %s:verify", c.ID, j.Script, c.ID)
		}
		if race := slices.Contains(j.Script, "task "+c.ID+":test:race"); race != (c.Profile == manifest.ProfileGo) {
			t.Errorf("%s: runs test:race = %v", c.ID, race)
		}
		// The hardening keys (spec 0038 §4).
		if j.Stage != "test" || j.Needs == nil || len(*j.Needs) != 0 || len(j.Rules) != 1 || j.Rules[0]["when"] != "on_success" ||
			j.AllowFailure == nil || *j.AllowFailure || j.BeforeScript == nil || len(*j.BeforeScript) != 0 || j.Interruptible == nil || !*j.Interruptible {
			t.Errorf("%s: hardening keys missing or wrong: %+v", c.ID, j)
		}
		if c.Profile != manifest.ProfileGo && !slices.ContainsFunc(j.Script, func(s string) bool { return strings.Contains(s, "sha256sum -c") }) {
			t.Errorf("%s: Task archive is not checksum-verified", c.ID)
		}
	}

	var top struct {
		Workflow struct {
			Rules []map[string]any `yaml:"rules"`
		} `yaml:"workflow"`
		Include []struct {
			Local string `yaml:"local"`
			Rules []struct {
				Exists []string `yaml:"exists"`
			} `yaml:"rules"`
		} `yaml:"include"`
		Variables map[string]string `yaml:"variables"`
	}
	if err := yaml.Unmarshal([]byte(content), &top); err != nil {
		t.Fatal(err)
	}
	if len(top.Workflow.Rules) == 0 {
		t.Error("no workflow:rules")
	}
	if len(top.Include) != 1 || top.Include[0].Local != conformance.GitLabPath ||
		len(top.Include[0].Rules) != 1 || !slices.Equal(top.Include[0].Rules[0].Exists, []string{conformance.GitLabPath}) {
		t.Errorf("include %+v, want %s with rules: exists on itself", top.Include, conformance.GitLabPath)
	}
	if top.Variables["TASK_SHA256"] == "" {
		t.Error("TASK_VERSION is used outside a Go job but TASK_SHA256 is not set")
	}

	local := decodeJob(t, doc, "local")
	exists, _ := local.Rules[0]["exists"].([]any)
	if len(local.Rules) != 1 || len(exists) != 1 || exists[0] != LocalPath {
		t.Errorf("local: rules %v, want exists on %s", local.Rules, LocalPath)
	}
	if local.Trigger["strategy"] != "mirror" || local.AllowFailure == nil || *local.AllowFailure || local.Stage != "test" || local.Needs == nil || local.Interruptible == nil {
		t.Errorf("local: %+v", local)
	}
	inc, _ := local.Trigger["include"].([]any)
	if len(inc) != 1 || inc[0].(map[string]any)["local"] != LocalPath {
		t.Errorf("local: trigger include %v", local.Trigger["include"])
	}

	// One job per component, plus local: nothing else runs here.
	var jobs []string
	for k := range doc {
		if !slices.Contains([]string{"workflow", "include", "variables"}, k) {
			jobs = append(jobs, k)
		}
	}
	slices.Sort(jobs)
	if want := []string{"api", "local", "web", "worker"}; !slices.Equal(jobs, want) {
		t.Errorf("jobs %v, want %v", jobs, want)
	}
}

// TestPipelineNeverRunsVibe pins spec 0038 acceptance criterion 5 for
// .gitlab-ci.yml: its only mention of vibe is the include path.
func TestPipelineNeverRunsVibe(t *testing.T) {
	content, _ := resolvePipeline(t, components)
	for _, line := range strings.Split(content, "\n") {
		if rest := strings.ReplaceAll(line, conformance.GitLabPath, ""); regexp.MustCompile(`\bvibe\b`).MatchString(rest) || strings.Contains(line, "task audit") {
			t.Errorf("line mentions vibe or task audit: %q", line)
		}
	}
}

// TestPipelineOnlyNeededVersions: each toolchain version variable is set
// exactly when a component needs it.
func TestPipelineOnlyNeededVersions(t *testing.T) {
	content, _ := resolvePipeline(t, components[:1])
	for _, v := range []string{"NODE_VERSION", "PYTHON_VERSION", "TASK_SHA256"} {
		if strings.Contains(content, v) {
			t.Errorf("a Go-only pipeline sets %s", v)
		}
	}
}

// TestVersionsMatchGitHub keeps the GitLab pins equal to the GitHub
// workflow's, so changing a provider never changes a toolchain.
func TestVersionsMatchGitHub(t *testing.T) {
	gl, _ := resolvePipeline(t, components)
	rs, err := githubmono.New().Resolve(context.Background(), &module.Context{Components: components})
	if err != nil {
		t.Fatal(err)
	}
	gh := string(rs[0].Content)
	pin := regexp.MustCompile(`(?m)^\s+(GO_VERSION|TASK_VERSION|GOLANGCI_LINT_VERSION|NODE_VERSION|PYTHON_VERSION): (".*")$`)
	want := map[string]string{}
	for _, m := range pin.FindAllStringSubmatch(gh, -1) {
		want[m[1]] = m[2]
	}
	got := map[string]string{}
	for _, m := range pin.FindAllStringSubmatch(gl, -1) {
		got[m[1]] = m[2]
	}
	if len(want) != 5 || len(got) != 5 {
		t.Fatalf("pins: github %v, gitlab %v", want, got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: gitlab %s, github %s", k, got[k], v)
		}
	}
}
