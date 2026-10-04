package standard

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// resolveProvider resolves prod-mono's sample components with vibe.yaml's
// ci: map set to doc ("" for absent), in order.
func resolveProvider(t *testing.T, ci string) ([]resource.Resource, *module.Context) {
	t.Helper()
	s := mustLookup(t, "prod-mono")
	m := &manifest.Manifest{Standard: "prod-mono", Version: "v1"}
	if ci != "" {
		m.CI = &manifest.CI{Provider: &ci}
	}
	sel, err := s.Select(m)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	mctx := sampleContext(*s)
	mctx.Integrations, mctx.Policies = sel.Integrations, sel.Policies
	var out []resource.Resource
	for _, mod := range s.ModulesFor(sel) {
		rs, err := mod.Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatalf("resolving %s: %v", mod.Name(), err)
		}
		out = append(out, rs...)
	}
	return out, mctx
}

func paths(rs []resource.Resource) []string {
	var ps []string
	for _, r := range rs {
		ps = append(ps, r.Path)
	}
	return ps
}

// TestCIProviderDefaultIsGitHub pins spec 0038 acceptance criterion 1: an
// absent ci: and provider: github resolve the same resources, byte for
// byte and in order.
func TestCIProviderDefaultIsGitHub(t *testing.T) {
	absent, _ := resolveProvider(t, "")
	github, _ := resolveProvider(t, "github")
	if !slices.Equal(paths(absent), paths(github)) {
		t.Fatalf("paths differ:\n%v\n%v", paths(absent), paths(github))
	}
	for i := range absent {
		if string(absent[i].Content) != string(github[i].Content) {
			t.Errorf("%s differs", absent[i].Path)
		}
	}
	if !slices.Contains(paths(absent), ".github/workflows/ci.yml") {
		t.Error("the default resolves no ci.yml")
	}
}

// TestCIProviderSelection pins spec 0038 acceptance criterion 2's
// standard-level errors: the setting is offered by prod-mono only, and an
// unknown value lists the valid ones.
func TestCIProviderSelection(t *testing.T) {
	gitlab := "gitlab"
	for _, name := range []string{"prod-go", "prod-ts", "prod-py"} {
		s := mustLookup(t, name)
		_, err := s.Select(&manifest.Manifest{Standard: name, Version: "v1", CI: &manifest.CI{Provider: &gitlab}})
		want := "ci.provider (gitlab): " + name + "/v1 offers no provider setting"
		if err == nil || err.Error() != want {
			t.Errorf("%s: error %v, want %q", name, err, want)
		}
		if _, ok := s.Defaults().Policies[manifest.CIProvider]; ok {
			t.Errorf("%s: has a default ci.provider", name)
		}
	}
	s := mustLookup(t, "prod-mono")
	bad := "bitbucket"
	_, err := s.Select(&manifest.Manifest{Standard: "prod-mono", Version: "v1", CI: &manifest.CI{Provider: &bad}})
	if want := "ci.provider (bitbucket): unknown value (valid: github, gitlab, none)"; err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
	if got := s.Defaults().Policies[manifest.CIProvider]; got != "github" {
		t.Errorf("default provider %q, want github", got)
	}
}

// TestCIProviderResourceSets pins spec 0038 acceptance criteria 3 and 6:
// gitlab swaps the four .github/ paths for the GitLab files, none drops
// them, every other resource is the same path, and neither names .github/
// anywhere or keeps workflows:lint and actionlint.
func TestCIProviderResourceSets(t *testing.T) {
	githubOnly := []string{".github/workflows/ci.yml", ".github/dependabot.yml", ".github/pull_request_template.md", ".github/workflows/conformance.yml"}
	base, _ := resolveProvider(t, "github")
	var common []string
	for _, p := range paths(base) {
		if !slices.Contains(githubOnly, p) {
			common = append(common, p)
		}
	}
	for provider, extra := range map[string][]string{
		"gitlab": {".gitlab-ci.yml", ".gitlab/merge_request_templates/Default.md", ".gitlab-ci.vibe.yml"},
		"none":   nil,
	} {
		t.Run(provider, func(t *testing.T) {
			rs, mctx := resolveProvider(t, provider)
			var rest, got []string
			for _, p := range paths(rs) {
				if slices.Contains(extra, p) {
					got = append(got, p)
				} else {
					rest = append(rest, p)
				}
			}
			if !slices.Equal(got, extra) {
				t.Errorf("provider files %v, want %v in that order", got, extra)
			}
			if !slices.Equal(rest, common) {
				t.Errorf("other paths %v, want %v", rest, common)
			}
			for _, r := range rs {
				if strings.Contains(string(r.Content), ".github/") {
					t.Errorf("%s names a .github/ path", r.Path)
				}
				if r.Path == "Taskfile.yml" {
					if _, ok := taskfileTasks(t, r)["workflows:lint"]; ok {
						t.Error("Taskfile.yml defines workflows:lint")
					}
					if strings.Contains(string(r.Content), "workflows:lint") || strings.Contains(string(r.Content), "actionlint") {
						t.Error("Taskfile.yml still mentions workflows:lint or actionlint")
					}
				}
			}
			for _, mod := range s(t).ModulesFor(Selection{Integrations: mctx.Integrations, Policies: mctx.Policies}) {
				if tr, ok := mod.(module.ToolRequirer); ok {
					for _, tool := range tr.RequiredTools(mctx) {
						if tool.Name == "actionlint" {
							t.Errorf("%s requires actionlint", mod.Name())
						}
					}
				}
			}
		})
	}
}

func s(t *testing.T) *Standard {
	t.Helper()
	return mustLookup(t, "prod-mono")
}

// TestCIProviderConformanceIsolated pins spec 0038 acceptance criterion 5
// for every provider: only Taskfile.vibe.yml, conformance.yml and
// .gitlab-ci.vibe.yml run vibe or task audit, and nothing installs
// vibe@latest.
func TestCIProviderConformanceIsolated(t *testing.T) {
	allowed := []string{"Taskfile.vibe.yml", ".github/workflows/conformance.yml", ".gitlab-ci.vibe.yml"}
	for _, provider := range []string{"github", "gitlab", "none"} {
		rs, _ := resolveProvider(t, provider)
		for _, r := range rs {
			c := string(r.Content)
			if strings.Contains(c, "vibe@latest") {
				t.Errorf("%s: %s installs vibe@latest", provider, r.Path)
			}
			if slices.Contains(allowed, r.Path) {
				continue
			}
			if strings.Contains(c, "task audit") && !strings.HasSuffix(r.Path, ".md") {
				for _, line := range strings.Split(c, "\n") {
					if strings.Contains(line, "task audit") && !strings.HasPrefix(strings.TrimSpace(line), "#") && !strings.Contains(line, "desc:") && !strings.Contains(line, "\"task audit\"") {
						t.Errorf("%s: %s runs task audit: %q", provider, r.Path, line)
					}
				}
			}
		}
	}
}

// TestCIProviderReservedIDs pins spec 0038 acceptance criterion 7: under
// gitlab a component named after a GitLab keyword is an error naming both;
// under github the same components resolve.
func TestCIProviderReservedIDs(t *testing.T) {
	st := mustLookup(t, "prod-mono")
	for _, id := range []string{"default", "include", "stages", "variables", "workflow", "image", "services", "cache", "pages"} {
		for _, provider := range []string{"github", "gitlab"} {
			mctx := &module.Context{
				Components: []manifest.Component{{ID: id, Path: "svc", Profile: manifest.ProfileGo}},
				Policies:   map[string]string{manifest.CIProvider: provider},
			}
			var err error
			for _, mod := range st.Modules {
				if _, e := mod.Resolve(context.Background(), mctx); e != nil && err == nil {
					err = e
				}
			}
			switch {
			case provider == "github" && err != nil:
				t.Errorf("github, id %s: %v", id, err)
			case provider == "gitlab" && (err == nil || !strings.Contains(err.Error(), "components[0] ("+id+")") || !strings.Contains(err.Error(), "GitLab CI keyword")):
				t.Errorf("gitlab, id %s: error %v", id, err)
			}
		}
	}
}
