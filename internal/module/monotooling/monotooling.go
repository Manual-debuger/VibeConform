// Package monotooling provides prod-mono/v1's language configuration: for
// each component in vibe.yaml, the files its profile's single-language
// tooling module generates (go-tooling, ts-tooling, python-tooling),
// re-rooted at the component's path. The content is those modules' own,
// so a component is configured byte for byte as a single-language
// repository would be. See docs/specs/0025-prod-mono.md.
package monotooling

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/gotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/pythontooling"
	"github.com/Manual-debuger/VibeConform/internal/module/tstooling"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

var tooling = map[manifest.Profile]module.Module{
	manifest.ProfileGo: gotooling.New(),
	manifest.ProfileTS: tstooling.New(),
	manifest.ProfilePy: pythontooling.New(),
}

type monotoolingModule struct{}

// New returns the mono-tooling module.
func New() module.Module {
	return monotoolingModule{}
}

func (monotoolingModule) Name() string {
	return "mono-tooling"
}

// RequiredTools is the union of what each declared profile's tooling
// module requires, in manifest.Profiles order.
func (monotoolingModule) RequiredTools(mctx *module.Context) []module.Tool {
	var tools []module.Tool
	for _, p := range manifest.Profiles {
		if !slices.ContainsFunc(module.ComponentsOf(mctx), func(c manifest.Component) bool { return c.Profile == p }) {
			continue
		}
		if r, ok := tooling[p].(module.ToolRequirer); ok {
			tools = append(tools, r.RequiredTools(nil)...)
		}
	}
	return tools
}

// Resolve returns each component's configuration files in vibe.yaml
// order, and within a component in its tooling module's order.
func (monotoolingModule) Resolve(ctx context.Context, mctx *module.Context) ([]resource.Resource, error) {
	components := module.ComponentsOf(mctx)
	if len(components) == 0 {
		return nil, errors.New("prod-mono needs at least one component in vibe.yaml")
	}
	var out []resource.Resource
	for _, c := range components {
		m, ok := tooling[c.Profile]
		if !ok {
			return nil, fmt.Errorf("component %s: no tooling for profile %q", c.ID, c.Profile)
		}
		rs, err := m.Resolve(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("component %s: resolve %s: %w", c.ID, m.Name(), err)
		}
		for _, r := range rs {
			r.Path = c.Path + "/" + r.Path
			out = append(out, r)
		}
	}
	return out, nil
}
