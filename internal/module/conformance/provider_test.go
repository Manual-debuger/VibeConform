package conformance

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

func resolveFor(t *testing.T, provider string) map[string][]byte {
	t.Helper()
	rs, err := New().Resolve(context.Background(), &module.Context{Policies: map[string]string{manifest.CIProvider: provider}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	var order []string
	for _, r := range rs {
		out[r.Path] = r.Content
		order = append(order, r.Path)
	}
	if order[len(order)-1] != "Taskfile.vibe.yml" {
		t.Errorf("%s: Taskfile.vibe.yml is not last: %v", provider, order)
	}
	return out
}

// TestProviderResources pins spec 0038's table for vibe-conformance: the
// GitHub workflow, the GitLab job, or none, then Taskfile.vibe.yml, whose
// header names the file deleted with it. Under github every byte is the
// template's.
func TestProviderResources(t *testing.T) {
	gh := resolveFor(t, module.CIGitHub)
	if len(gh) != 2 || !bytes.Equal(gh["Taskfile.vibe.yml"], vibeTaskfile) || !bytes.Equal(gh[".github/workflows/conformance.yml"], conformanceWorkflow) {
		t.Errorf("github: %v", keys(gh))
	}

	gl := resolveFor(t, module.CIGitLab)
	if len(gl) != 2 || !bytes.Equal(gl[GitLabPath], gitlabConformance) {
		t.Errorf("gitlab: %v", keys(gl))
	}
	if want := "# .gitlab-ci.vibe.yml, vibe.yaml, and .vibe/ — removes\n"; !strings.Contains(string(gl["Taskfile.vibe.yml"]), want) {
		t.Errorf("gitlab Taskfile.vibe.yml header lacks %q", want)
	}

	none := resolveFor(t, module.CINone)
	if len(none) != 1 {
		t.Errorf("none: %v", keys(none))
	}
	if want := "# vibe.yaml and .vibe/ — removes\n"; !strings.Contains(string(none["Taskfile.vibe.yml"]), want) {
		t.Errorf("none Taskfile.vibe.yml header lacks %q", want)
	}
	for provider, rs := range map[string]map[string][]byte{"gitlab": gl, "none": none} {
		for path, c := range rs {
			if bytes.Contains(c, []byte(".github/")) {
				t.Errorf("%s: %s names .github/", provider, path)
			}
		}
		// Only the header line differs from the template.
		if strings.Count(string(rs["Taskfile.vibe.yml"]), "\n") != strings.Count(string(vibeTaskfile), "\n") {
			t.Errorf("%s: Taskfile.vibe.yml changed by more than one line", provider)
		}
	}
	if _, err := New().Resolve(context.Background(), &module.Context{Policies: map[string]string{manifest.CIProvider: "bitbucket"}}); err == nil {
		t.Error("an unknown provider resolved")
	}
}

// TestGitLabJobIsPinnedAndSelfContained mirrors the GitHub workflow's test
// (spec 0022) for .gitlab-ci.vibe.yml: exactly one job, which runs task
// audit after the self-hosting probe and never installs vibe itself.
func TestGitLabJobIsPinnedAndSelfContained(t *testing.T) {
	if bytes.Contains(gitlabConformance, []byte("\r")) {
		t.Error("gitlab-ci.vibe.yml contains CR bytes")
	}
	var doc map[string]struct {
		Image        string            `yaml:"image"`
		Needs        *[]string         `yaml:"needs"`
		Rules        []map[string]any  `yaml:"rules"`
		AllowFailure *bool             `yaml:"allow_failure"`
		BeforeScript *[]string         `yaml:"before_script"`
		Script       []string          `yaml:"script"`
		Variables    map[string]string `yaml:"variables"`
	}
	if err := yaml.Unmarshal(gitlabConformance, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc) != 1 {
		t.Fatalf("%d top-level keys, want one job", len(doc))
	}
	j, ok := doc["conformance:audit"]
	if !ok {
		t.Fatal("no conformance:audit job")
	}
	last := j.Script[len(j.Script)-1]
	if !strings.HasSuffix(strings.TrimSpace(last), "task audit") || !strings.Contains(last, "[ -d ./cmd/vibe ]") || !strings.Contains(last, "go build") {
		t.Errorf("last script step does not run the self-hosting probe and then task audit:\n%s", last)
	}
	for _, s := range j.Script {
		if strings.Contains(s, "go install") && strings.Contains(s, "cmd/vibe") {
			t.Errorf("installs vibe itself: %q", s)
		}
	}
	if bytes.Contains(gitlabConformance, []byte("@latest")) {
		t.Error("installs something @latest")
	}
	if j.Needs == nil || len(*j.Needs) != 0 || j.AllowFailure == nil || *j.AllowFailure || j.BeforeScript == nil || len(j.Rules) != 1 {
		t.Errorf("hardening keys missing: %+v", j)
	}
	// The pins are the GitHub workflow's.
	for _, pin := range []string{`GO_VERSION: "1.27.0"`, `TASK_VERSION: "v3.53.1"`} {
		if !bytes.Contains(conformanceWorkflow, []byte(pin)) {
			t.Errorf("conformance.yml no longer pins %s; update gitlab-ci.vibe.yml with it", pin)
		}
	}
	if j.Image != "golang:1.27.0" || j.Variables["TASK_VERSION"] != "v3.53.1" {
		t.Errorf("image %q, TASK_VERSION %q", j.Image, j.Variables["TASK_VERSION"])
	}
}

func keys(m map[string][]byte) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
