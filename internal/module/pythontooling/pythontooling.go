// Package pythontooling provides the Python half of prod-py/v1:
// ruff for lint and format, pyright for typecheck, mirroring gotooling's
// shape.
//
// Both configurations are standalone files rather than pyproject.toml
// sections. VibeConform owns whole files, and pyproject.toml also holds
// [project] metadata belonging to the repository — see
// docs/specs/0014-m2-milestone.md.
package pythontooling

import (
	"context"
	_ "embed"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

//go:embed templates/ruff.toml
var ruffConfig []byte

//go:embed templates/pyrightconfig.json
var pyrightConfig []byte

type pythontoolingModule struct{}

// New returns the python-tooling module.
func New() module.Module {
	return pythontoolingModule{}
}

func (pythontoolingModule) Name() string {
	return "python-tooling"
}

// Resolve returns this module's resources in a fixed order; see the
// github-ci module for why order is part of the contract.
//
// The module deliberately implements no RequiredTools: ruff and pyright are
// normally pinned per project and run through uv or a virtualenv, so
// checking PATH for them would warn on a perfectly healthy repository.
//
// Generated paths (spec 0037) become ruff's extend-exclude, so neither ruff
// format nor ruff check touches them; pyright still type-checks them.
// Without any, every file is the embedded template.
func (pythontoolingModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	ruff, err := excludeInRuff(ruffConfig, module.GeneratedOf(mctx))
	if err != nil {
		return nil, err
	}
	return []resource.Resource{
		{
			Path:      "ruff.toml",
			Ownership: resource.Generated,
			Content:   ruff,
		},
		{
			Path:      "pyrightconfig.json",
			Ownership: resource.Generated,
			Content:   pyrightConfig,
		},
	}, nil
}

// ruffAnchor is the last top-level key of ruff.toml before its first table;
// the generated keys go right after it, so they stay top-level.
const ruffAnchor = "line-length = 100\n"

// excludeInRuff adds extend-exclude with the generated paths, and
// force-exclude so the exclusion also holds for files passed explicitly,
// as lefthook and hook:format do. The grammar admits no quote or
// backslash, so the TOML strings need no escaping.
func excludeInRuff(config []byte, generated []string) ([]byte, error) {
	if len(generated) == 0 {
		return config, nil
	}
	quoted := make([]string, len(generated))
	for i, g := range generated {
		quoted[i] = `"` + g + `"`
	}
	keys := ruffAnchor +
		"\n# Generated code (vibe.yaml generated:): not formatted or linted, still\n" +
		"# type-checked. force-exclude applies it to files passed explicitly too.\n" +
		"extend-exclude = [" + strings.Join(quoted, ", ") + "]\n" +
		"force-exclude = true\n"
	return module.ReplaceOnce(config, "ruff.toml", "exclude generated paths", ruffAnchor, keys)
}
