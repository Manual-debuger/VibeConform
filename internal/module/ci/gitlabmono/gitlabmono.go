// Package gitlabmono provides prod-mono/v1's GitLab CI: a pipeline with one
// job per component in vibe.yaml, an optional child pipeline for the
// project's own jobs, and the merge request template. It resolves nothing
// unless vibe.yaml selects ci.provider gitlab. Mirrors
// internal/module/ci/githubmono's shape. See docs/specs/0038-ci-provider.md.
package gitlabmono

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/github"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// [[ ]] delimiters leave GitLab's own $VAR and ${VAR} untouched.
var (
	//go:embed templates/gitlab-ci.yml.tmpl
	pipelineSrc string

	pipeline = template.Must(template.New(".gitlab-ci.yml").Delims("[[", "]]").Parse(pipelineSrc))
)

// The paths this module writes, and the ones it names but never writes.
const (
	PipelinePath = ".gitlab-ci.yml"
	// MergeRequestTemplatePath is applied by default to new merge requests
	// on tiers that support default templates; elsewhere it is selectable.
	MergeRequestTemplatePath = ".gitlab/merge_request_templates/Default.md"
	// LocalPath is the project-owned child pipeline, never written.
	LocalPath = ".gitlab-ci.local.yml"
)

// ReservedIDs are GitLab top-level keywords and reserved job names. A
// component with one of these ids would become that keyword in
// .gitlab-ci.yml rather than a job. They are checked only under gitlab,
// so a GitHub repository with such a component stays valid.
var ReservedIDs = []string{"default", "include", "stages", "variables", "workflow", "image", "services", "cache", "pages"}

const pullRequestTemplatePath = ".github/pull_request_template.md"

type gitlabmonoModule struct{}

// New returns the gitlab-ci-mono module.
func New() module.Module {
	return gitlabmonoModule{}
}

func (gitlabmonoModule) Name() string {
	return "gitlab-ci-mono"
}

// Resolve returns .gitlab-ci.yml and the merge request template under
// ci.provider gitlab, and nothing otherwise.
func (gitlabmonoModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	if module.CIProvider(mctx) != module.CIGitLab {
		return nil, nil
	}
	components := module.ComponentsOf(mctx)
	if len(components) == 0 {
		return nil, errors.New("prod-mono needs at least one component in vibe.yaml")
	}
	for i, c := range components {
		if slices.Contains(ReservedIDs, c.ID) {
			return nil, fmt.Errorf("components[%d] (%s): id %q is a GitLab CI keyword, so it cannot be a job name under ci.provider gitlab (reserved: %s)",
				i, c.ID, c.ID, strings.Join(ReservedIDs, ", "))
		}
	}
	has := func(p manifest.Profile) bool {
		return slices.ContainsFunc(components, func(c manifest.Component) bool { return c.Profile == p })
	}

	var buf bytes.Buffer
	err := pipeline.Execute(&buf, struct {
		Components          []manifest.Component
		HasGo, HasTS, HasPy bool
		NeedsTaskArchive    bool
	}{
		Components:       components,
		HasGo:            has(manifest.ProfileGo),
		HasTS:            has(manifest.ProfileTS),
		HasPy:            has(manifest.ProfilePy),
		NeedsTaskArchive: has(manifest.ProfileTS) || has(manifest.ProfilePy),
	})
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", PipelinePath, err)
	}

	mr, err := mergeRequestTemplate()
	if err != nil {
		return nil, err
	}
	return []resource.Resource{
		{Path: PipelinePath, Ownership: resource.Generated, Content: buf.Bytes()},
		{Path: MergeRequestTemplatePath, Ownership: resource.Generated, Content: mr},
	}, nil
}

// mergeRequestTemplate reuses github-ci's pull request template, which is
// language- and host-neutral, so there is one copy of it.
func mergeRequestTemplate() ([]byte, error) {
	rs, err := github.New().Resolve(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("resolve github-ci: %w", err)
	}
	for _, r := range rs {
		if r.Path == pullRequestTemplatePath {
			return r.Content, nil
		}
	}
	return nil, fmt.Errorf("github-ci resolves no %s", pullRequestTemplatePath)
}
