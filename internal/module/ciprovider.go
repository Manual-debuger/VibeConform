package module

import (
	"context"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// The values of vibe.yaml's ci.provider (docs/specs/0038-ci-provider.md).
const (
	CIGitHub = "github"
	CIGitLab = "gitlab"
	CINone   = "none"
)

// CIProvider returns the CI system the repository uses. Absent means
// GitHub, which is what every standard generated before ci.provider
// existed, so a nil context, one without policies, or one without the key
// resolves today's files. Unlike Selected, failing open is the compatible
// direction here.
func CIProvider(mctx *Context) string {
	if mctx == nil || mctx.Policies[manifest.CIProvider] == "" {
		return CIGitHub
	}
	return mctx.Policies[manifest.CIProvider]
}

// Setting is a catalog option that resolves nothing itself: core modules
// read its value from the context instead, so that pruning's trial
// selection finds every file the value would produce, through whichever
// module produces it.
type Setting struct{ name string }

// NewSetting returns a Setting module with the given name.
func NewSetting(name string) Setting {
	return Setting{name: name}
}

// Name returns the module's name.
func (s Setting) Name() string {
	return s.name
}

// Resolve returns nothing.
func (Setting) Resolve(context.Context, *Context) ([]resource.Resource, error) {
	return nil, nil
}
