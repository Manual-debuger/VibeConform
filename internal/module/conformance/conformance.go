// Package conformance provides the vibe-conformance module: the only
// generated files that run vibe. It owns the Conformance workflow and the
// Taskfile holding task audit, so removing VibeConform from a repository is
// deleting files rather than editing shared ones. Language-neutral, and
// composed into every standard. See docs/specs/0022-conformance-isolation.md.
package conformance

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/conformance.yml
var conformanceWorkflow []byte

//go:embed templates/Taskfile.vibe.yml
var vibeTaskfile []byte

//go:embed templates/gitlab-ci.vibe.yml
var gitlabConformance []byte

// GitLabPath is the GitLab conformance job's file (spec 0038 §4).
const GitLabPath = ".gitlab-ci.vibe.yml"

// taskfileRemoval is the line of Taskfile.vibe.yml's header that names the
// CI file removed together with it, which depends on the CI provider.
const taskfileRemoval = "# .github/workflows/conformance.yml, vibe.yaml, and .vibe/ — removes\n"

type conformanceModule struct{}

// New returns the vibe-conformance module.
func New() module.Module {
	return conformanceModule{}
}

func (conformanceModule) Name() string {
	return "vibe-conformance"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
//
// The module deliberately implements no RequiredTools. Its one external
// need, go, matters only when vibe is not on PATH, and task audit explains
// that case itself when it happens.
//
// The conformance job follows ci.provider (spec 0038): the GitHub workflow,
// the GitLab include, or none. Taskfile.vibe.yml is always resolved, since
// task audit is what any CI system calls, and its header names the file
// deleted with it. Under github every byte is the template's.
func (conformanceModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	taskfile := vibeTaskfile
	var rs []resource.Resource
	switch provider := module.CIProvider(mctx); provider {
	case module.CIGitHub:
		rs = append(rs, resource.Resource{
			Path:      ".github/workflows/conformance.yml",
			Ownership: resource.Generated,
			Content:   conformanceWorkflow,
		})
	case module.CIGitLab, module.CINone:
		line := "# vibe.yaml and .vibe/ — removes\n"
		if provider == module.CIGitLab {
			rs = append(rs, resource.Resource{Path: GitLabPath, Ownership: resource.Generated, Content: gitlabConformance})
			line = "# " + GitLabPath + ", vibe.yaml, and .vibe/ — removes\n"
		}
		var err error
		if taskfile, err = module.ReplaceOnce(vibeTaskfile, "Taskfile.vibe.yml", "name the conformance file", taskfileRemoval, line); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown ci.provider %q", provider)
	}
	return append(rs, resource.Resource{
		Path:      "Taskfile.vibe.yml",
		Ownership: resource.Generated,
		Content:   taskfile,
	}), nil
}
