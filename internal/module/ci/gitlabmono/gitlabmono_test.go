package gitlabmono

import (
	"context"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
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
	Inherit       map[string]any   `yaml:"inherit"`
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

	images := map[string]string{"api": "golang:${GO_VERSION}", "web": "node:${NODE_VERSION}", "worker": "ghcr.io/astral-sh/uv:python${PYTHON_VERSION}-bookworm"}
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
	// The conformance job, then the project's defaults (spec 0040 §2), each
	// only when its file exists.
	wantIncludes := []string{conformance.GitLabPath, DefaultsPath}
	if len(top.Include) != len(wantIncludes) {
		t.Errorf("include %+v, want %v", top.Include, wantIncludes)
	}
	for i, inc := range top.Include {
		if i >= len(wantIncludes) || inc.Local != wantIncludes[i] || len(inc.Rules) != 1 || !slices.Equal(inc.Rules[0].Exists, []string{inc.Local}) {
			t.Errorf("include %d: %+v, want %s with rules: exists on itself", i, inc, wantIncludes[i%len(wantIncludes)])
		}
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
	if local.Inherit["default"] != false {
		t.Errorf("local: inherit %v, want default: false so project defaults never reach the trigger job", local.Inherit)
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

// TestGoTestNeverReplaysCachedResults pins issue #64: the Go job caches
// GOCACHE, which also holds test results, so without -count=1 a restored
// cache makes go test report cached passes instead of running.
func TestGoTestNeverReplaysCachedResults(t *testing.T) {
	_, doc := resolvePipeline(t, components)
	for _, c := range components {
		var j struct {
			Variables map[string]string `yaml:"variables"`
		}
		n := doc[c.ID]
		if err := n.Decode(&j); err != nil {
			t.Fatal(err)
		}
		if got, want := j.Variables["GOFLAGS"], map[bool]string{true: "-count=1"}[c.Profile == manifest.ProfileGo]; got != want {
			t.Errorf("%s: GOFLAGS = %q, want %q", c.ID, got, want)
		}
	}
}

// contractKeys are the keys every required job declares (spec 0040 §1). A
// declared key beats the project's default:, so dropping one from a
// template would hand it to .gitlab-ci.defaults.yml.
var contractKeys = []string{"stage", "image", "needs", "rules", "allow_failure", "before_script", "script", "interruptible"}

// TestRequiredJobsDeclareContractKeys covers every component job and the
// conformance job, which the defaults also reach.
func TestRequiredJobsDeclareContractKeys(t *testing.T) {
	_, doc := resolvePipeline(t, components)
	rs, err := conformance.New().Resolve(context.Background(), gitlabContext(components))
	if err != nil {
		t.Fatal(err)
	}
	var vibeDoc map[string]yaml.Node
	for _, r := range rs {
		if r.Path == conformance.GitLabPath {
			if err := yaml.Unmarshal(r.Content, &vibeDoc); err != nil {
				t.Fatal(err)
			}
		}
	}
	jobs := map[string]yaml.Node{"conformance:audit": vibeDoc["conformance:audit"]}
	for _, c := range components {
		jobs[c.ID] = doc[c.ID]
	}
	for name, n := range jobs {
		var keys map[string]yaml.Node
		if err := n.Decode(&keys); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, k := range contractKeys {
			if _, ok := keys[k]; !ok {
				t.Errorf("%s does not declare %s, so a project default: could set it", name, k)
			}
		}
	}
}

// TestCachePolicy pins spec 0040 §3 and §4 (issue #65): each component
// job's cache policy is VIBE_CACHE_POLICY, pull-push unless overridden, and
// merge request pipelines turn it to pull only when the project opts in.
func TestCachePolicy(t *testing.T) {
	content, doc := resolvePipeline(t, components)
	for _, c := range components {
		var j struct {
			Cache struct {
				Policy string   `yaml:"policy"`
				Paths  []string `yaml:"paths"`
			} `yaml:"cache"`
		}
		n := doc[c.ID]
		if err := n.Decode(&j); err != nil {
			t.Fatal(err)
		}
		if j.Cache.Policy != "$VIBE_CACHE_POLICY" || !slices.Equal(j.Cache.Paths, []string{".ci-cache/" + c.ID + "/"}) {
			t.Errorf("%s: cache %+v", c.ID, j.Cache)
		}
	}

	var top struct {
		Variables map[string]string `yaml:"variables"`
		Workflow  struct {
			Rules []struct {
				If        string            `yaml:"if"`
				When      string            `yaml:"when"`
				Variables map[string]string `yaml:"variables"`
			} `yaml:"rules"`
		} `yaml:"workflow"`
	}
	if err := yaml.Unmarshal([]byte(content), &top); err != nil {
		t.Fatal(err)
	}
	if got := top.Variables["VIBE_CACHE_POLICY"]; got != "pull-push" {
		t.Errorf("VIBE_CACHE_POLICY default %q, want pull-push", got)
	}
	type rule struct{ ifc, when string }
	var got []rule
	for _, r := range top.Workflow.Rules {
		got = append(got, rule{r.If, r.When})
	}
	want := []rule{
		{`$CI_PIPELINE_SOURCE == "merge_request_event" && $VIBE_MR_CACHE_POLICY == "pull"`, ""},
		{`$CI_PIPELINE_SOURCE == "merge_request_event"`, ""},
		{`$CI_COMMIT_BRANCH && $CI_OPEN_MERGE_REQUESTS`, "never"},
		{`$CI_COMMIT_BRANCH`, ""},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("workflow rules %v, want %v", got, want)
	}
	for i, r := range top.Workflow.Rules {
		if wantVars := map[string]string{"VIBE_CACHE_POLICY": "pull"}; i == 0 && !reflect.DeepEqual(r.Variables, wantVars) || i > 0 && r.Variables != nil {
			t.Errorf("workflow rule %d sets %v", i, r.Variables)
		}
	}
}

// TestCheckDefaults pins the shape audit enforces on
// .gitlab-ci.defaults.yml (spec 0040 §2).
func TestCheckDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          []string // substrings, one per expected problem
	}{
		{"default only", "default:\n  tags: [linux]\n  retry: 1\n", nil},
		{"default with anything under it", "default:\n  image: alpine\n  before_script: [exit 0]\n", nil},
		{"job", "default:\n  tags: [linux]\napi:\n  allow_failure: true\n", []string{`"api"`}},
		{"variables and include", "variables:\n  X: y\ninclude:\n  - local: x.yml\n", []string{`"variables"`, `"include"`}},
		{"merge key", "<<: {default: {}}\n", []string{`"<<"`}},
		{"not a mapping", "- default\n", []string{"not a mapping"}},
		{"empty", "", []string{"empty"}},
		{"comments only", "# nothing yet\n", []string{"empty"}},
		{"invalid YAML", "default: [\n", []string{"not valid YAML"}},
		{"two documents", "default: {}\n---\napi: {}\n", []string{"more than one YAML document"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckDefaults([]byte(tc.content))
			if len(got) != len(tc.want) {
				t.Fatalf("problems %q, want %d matching %q", got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("problem %d %q does not mention %s", i, got[i], w)
				}
			}
		})
	}
}

// TestGuardsOnlyUnderGitLab: github and none never read the defaults file.
func TestGuardsOnlyUnderGitLab(t *testing.T) {
	g := New().(module.ProjectFileGuard)
	if fs := g.GuardedFiles(gitlabContext(components)); len(fs) != 1 || fs[0].Path != DefaultsPath {
		t.Errorf("gitlab: guarded %v, want %s", fs, DefaultsPath)
	}
	for _, mctx := range []*module.Context{
		nil,
		{Components: components},
		{Components: components, Policies: map[string]string{manifest.CIProvider: module.CIGitHub}},
		{Components: components, Policies: map[string]string{manifest.CIProvider: module.CINone}},
	} {
		if fs := g.GuardedFiles(mctx); fs != nil {
			t.Errorf("guarded %v outside gitlab", fs)
		}
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

// Both providers' pins come from internal/module/ci/pins, whose tests check
// them, so changing a provider never changes a toolchain (spec 0044).
