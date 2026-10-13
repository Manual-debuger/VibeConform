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
	"io"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/github"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/pins"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

var (
	//go:embed templates/gitlab-ci.yml.tmpl
	pipelineSrc string

	pipeline = pins.Parse(".gitlab-ci.yml", pipelineSrc)
)

// The paths this module writes, and the ones it names but never writes.
const (
	PipelinePath = ".gitlab-ci.yml"
	// MergeRequestTemplatePath is applied by default to new merge requests
	// on tiers that support default templates; elsewhere it is selectable.
	MergeRequestTemplatePath = ".gitlab/merge_request_templates/Default.md"
	// LocalPath is the project-owned child pipeline, never written.
	LocalPath = ".gitlab-ci.local.yml"
	// DefaultsPath is the project-owned default: for the generated jobs,
	// never written; GuardedFiles checks its shape.
	DefaultsPath = ".gitlab-ci.defaults.yml"
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

	content, err := pins.Render(pipeline, struct {
		Pins                pins.Table
		Components          []manifest.Component
		HasGo, HasTS, HasPy bool
		NeedsTaskArchive    bool
	}{
		Pins:             pins.Current,
		Components:       components,
		HasGo:            has(manifest.ProfileGo),
		HasTS:            has(manifest.ProfileTS),
		HasPy:            has(manifest.ProfilePy),
		NeedsTaskArchive: has(manifest.ProfileTS) || has(manifest.ProfilePy),
	})
	if err != nil {
		return nil, err
	}

	mr, err := mergeRequestTemplate()
	if err != nil {
		return nil, err
	}
	return []resource.Resource{
		{Path: PipelinePath, Ownership: resource.Generated, Content: content},
		{Path: MergeRequestTemplatePath, Ownership: resource.Generated, Content: mr},
	}, nil
}

// GuardedFiles names .gitlab-ci.defaults.yml under ci.provider gitlab, and
// nothing otherwise.
func (gitlabmonoModule) GuardedFiles(mctx *module.Context) []module.GuardedFile {
	if module.CIProvider(mctx) != module.CIGitLab {
		return nil
	}
	return []module.GuardedFile{{Path: DefaultsPath, Check: CheckDefaults}}
}

// CheckDefaults reports what makes content more than a default: block. Any
// other top-level key would merge into the generated jobs: a job of the same
// name could add allow_failure or variables, which the floor forbids. What
// is under default: is the project's; a job's own keyword wins over it
// anyway (docs/specs/0040-ci-floor-environment-seam.md §2).
func CheckDefaults(content []byte) []string {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return []string{"empty: the file must hold a default: mapping"}
		}
		return []string{fmt.Sprintf("not valid YAML: %v", err)}
	}
	var problems []string
	var next yaml.Node
	if err := dec.Decode(&next); !errors.Is(err, io.EOF) {
		problems = append(problems, "more than one YAML document: the file must be a single default: mapping")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return append(problems, "not a mapping: the file must hold a default: mapping")
	}
	for i := 0; i < len(root.Content); i += 2 {
		if key := root.Content[i].Value; key != "default" {
			problems = append(problems, fmt.Sprintf("top-level key %q: only default: may be set here", key))
		}
	}
	return problems
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
