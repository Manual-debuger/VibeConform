// Package githubmono provides the prod-mono/v1 variant of the GitHub CI
// module: a workflow with one job per component in vibe.yaml, a
// dependency-update entry per component in its own ecosystem, and the
// language-neutral pull request template. Mirrors
// internal/module/ci/github's shape for prod-go/v1.
// See docs/specs/0025-prod-mono.md.
package githubmono

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/github"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/pins"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

var (
	//go:embed templates/ci.yml.tmpl
	ciSrc string
	//go:embed templates/dependabot.yml.tmpl
	dependabotSrc string

	ciWorkflow = pins.Parse("ci.yml", ciSrc)
	dependabot = pins.Parse("dependabot.yml", dependabotSrc)
)

// ecosystems are the dependabot package ecosystems per profile, the same
// ones the single-language standards use.
var ecosystems = map[manifest.Profile]string{
	manifest.ProfileGo: "gomod",
	manifest.ProfileTS: "npm",
	manifest.ProfilePy: "pip",
}

// PullRequestTemplatePath is where the shared pull request template lives.
const pullRequestTemplatePath = ".github/pull_request_template.md"

type githubmonoModule struct{}

// New returns the github-ci-mono module.
func New() module.Module {
	return githubmonoModule{}
}

func (githubmonoModule) Name() string {
	return "github-ci-mono"
}

// Resolve returns ci.yml, dependabot.yml, and the pull request template,
// in the order the other github-ci modules use, under ci.provider github
// (the default), and nothing otherwise (spec 0038).
func (githubmonoModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	if module.CIProvider(mctx) != module.CIGitHub {
		return nil, nil
	}
	components := module.ComponentsOf(mctx)
	if len(components) == 0 {
		return nil, errors.New("prod-mono needs at least one component in vibe.yaml")
	}
	has := func(p manifest.Profile) bool {
		return slices.ContainsFunc(components, func(c manifest.Component) bool { return c.Profile == p })
	}

	needs := make([]string, 0, len(components)+1)
	for _, c := range components {
		needs = append(needs, c.ID)
	}
	needs = append(needs, "workflows")

	ci, err := pins.Render(ciWorkflow, struct {
		Pins                pins.Table
		Components          []manifest.Component
		Needs               string
		HasGo, HasTS, HasPy bool
	}{
		Pins:       pins.Current,
		Components: components,
		Needs:      "[" + strings.Join(needs, ", ") + "]",
		HasGo:      has(manifest.ProfileGo),
		HasTS:      has(manifest.ProfileTS),
		HasPy:      has(manifest.ProfilePy),
	})
	if err != nil {
		return nil, err
	}

	type entry struct{ Path, Ecosystem string }
	entries := make([]entry, 0, len(components))
	for _, c := range components {
		entries = append(entries, entry{Path: c.Path, Ecosystem: ecosystems[c.Profile]})
	}
	deps, err := pins.Render(dependabot, struct{ Components []entry }{entries})
	if err != nil {
		return nil, err
	}

	prTemplate, err := pullRequestTemplate()
	if err != nil {
		return nil, err
	}

	return []resource.Resource{
		{Path: ".github/workflows/ci.yml", Ownership: resource.Generated, Content: ci},
		{Path: ".github/dependabot.yml", Ownership: resource.Generated, Content: deps},
		prTemplate,
	}, nil
}

// pullRequestTemplate reuses github-ci's, which is language-neutral, so
// there is one copy of it.
func pullRequestTemplate() (resource.Resource, error) {
	rs, err := github.New().Resolve(context.Background(), nil)
	if err != nil {
		return resource.Resource{}, fmt.Errorf("resolve github-ci: %w", err)
	}
	for _, r := range rs {
		if r.Path == pullRequestTemplatePath {
			return r, nil
		}
	}
	return resource.Resource{}, fmt.Errorf("github-ci resolves no %s", pullRequestTemplatePath)
}
