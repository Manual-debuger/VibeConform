// Package monorepotooling provides prod-mono/v1's verification entry
// points: a Taskfile.yml per component with that language's tasks, the
// root Taskfile.yml that includes them and fans out to all of them, the
// lefthook configuration, and the agent guard. Mirrors
// internal/module/repotooling's role for prod-go/v1, resolved from
// vibe.yaml's components rather than from constants.
// See docs/specs/0025-prod-mono.md.
package monorepotooling

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
	"github.com/Manual-debuger/VibeConform/internal/module/pyrepotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/repotooling"
	"github.com/Manual-debuger/VibeConform/internal/module/tsrepotooling"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// The root files are templates, since their includes and fan-out are the
// component list. [[ ]] delimiters leave Task's own {{ }} templating
// untouched.
var (
	//go:embed templates/Taskfile.yml.tmpl
	rootTaskfileSrc string
	//go:embed templates/lefthook.yml.tmpl
	lefthookSrc string

	rootTaskfile = template.Must(template.New("Taskfile.yml").Delims("[[", "]]").Parse(rootTaskfileSrc))
	lefthook     = template.Must(template.New("lefthook.yml").Delims("[[", "]]").Parse(lefthookSrc))
)

// Component Taskfiles are the same for every component of a profile.
var (
	//go:embed templates/go.Taskfile.yml
	goTaskfile []byte
	//go:embed templates/ts.Taskfile.yml
	tsTaskfile []byte
	//go:embed templates/py.Taskfile.yml
	pyTaskfile []byte
)

// profile is what this module knows about one component language.
type profile struct {
	// taskfile is the component's Taskfile.yml.
	taskfile []byte
	// glob selects the files lefthook's pre-commit checks run for.
	glob string
	// guard is the single-language repo-tooling module whose guard, and
	// hook:guard command, a repository with this profile uses.
	guard module.Module
	// guardCommand runs the guard; it is that module's hook:guard command.
	guardCommand string
	// tools are the binaries the component's Taskfile calls.
	tools []module.Tool
}

var profiles = map[manifest.Profile]profile{
	manifest.ProfileGo: {
		taskfile:     goTaskfile,
		glob:         "*.go",
		guard:        repotooling.New(),
		guardCommand: "go run .claude/hooks/guard.go .claude/hooks/policy.json",
		tools: []module.Tool{
			{Name: "go", Why: "every Go component's build, test, vet, and module tasks", Version: []string{"env", "GOVERSION"}},
			{Name: "goimports", Why: "a Go component's task fmt and task fmt:check"},
			{Name: "govulncheck", Why: "a Go component's task security"},
		},
	},
	manifest.ProfileTS: {
		taskfile:     tsTaskfile,
		glob:         "*.{js,jsx,ts,tsx,json,css,md}",
		guard:        tsrepotooling.New(),
		guardCommand: "node .claude/hooks/guard.mjs .claude/hooks/policy.json",
		tools: []module.Tool{
			{Name: "pnpm", Why: "every TypeScript component's tasks, which run through pnpm exec", Version: []string{"--version"}},
		},
	},
	manifest.ProfilePy: {
		taskfile:     pyTaskfile,
		glob:         "*.py",
		guard:        pyrepotooling.New(),
		guardCommand: "uv run --no-project python .claude/hooks/guard.py .claude/hooks/policy.json",
		tools: []module.Tool{
			{Name: "uv", Why: "every Python component's tasks, which run through uv run", Version: []string{"--version"}},
		},
	},
}

// fanOut are the root tasks that run the same-named task in every
// component, in the order the root Taskfile lists them.
var fanOut = []struct{ Name, Desc string }{
	{"fmt", "Apply every component's formatter."},
	{"fmt:check", "Fail if any component's files are not formatted (CI-safe, no writes)."},
	{"lint", "Run every component's linters."},
	{"typecheck", "Type-check every component."},
	{"test", "Run every component's test suite."},
}

type monorepotoolingModule struct{}

// New returns the mono-repo-tooling module.
func New() module.Module {
	return monorepotoolingModule{}
}

func (monorepotoolingModule) Name() string {
	return "mono-repo-tooling"
}

// RequiredTools reports task and lefthook, which every prod-mono
// repository runs, plus the binaries of each profile vibe.yaml declares:
// warning about uv in a repository with no Python would be a false alarm
// (docs/decisions/0012-manifest-components.md). actionlint is required
// because task verify runs workflows:lint.
func (monorepotoolingModule) RequiredTools(mctx *module.Context) []module.Tool {
	tools := []module.Tool{
		{Name: "task", Why: "every verification entry point Taskfile.yml defines", Version: []string{"--version"}},
		{Name: "lefthook", Why: "the pre-commit hooks lefthook.yml describes, which vibe sync registers", Version: []string{"version"}},
	}
	for _, p := range declared(module.ComponentsOf(mctx)) {
		tools = append(tools, profiles[p].tools...)
	}
	return append(tools, module.Tool{Name: "actionlint", Why: "task workflows:lint", Version: []string{"-version"}})
}

// Resolve returns the root Taskfile.yml, lefthook.yml, and guard, then
// each component's Taskfile.yml in vibe.yaml order.
func (monorepotoolingModule) Resolve(_ context.Context, mctx *module.Context) ([]resource.Resource, error) {
	components := module.ComponentsOf(mctx)
	if len(components) == 0 {
		return nil, errors.New("prod-mono needs at least one component in vibe.yaml")
	}
	used := declared(components)
	guardProfile := profiles[used[0]]

	type lefthookComponent struct {
		manifest.Component
		Glob string
	}
	data := struct {
		Components   []manifest.Component
		FanOut       any
		GuardCommand string
		HasGo        bool
		HasTS        bool
		HasPy        bool
	}{
		Components:   components,
		FanOut:       fanOut,
		GuardCommand: guardProfile.guardCommand,
		HasGo:        slices.Contains(used, manifest.ProfileGo),
		HasTS:        slices.Contains(used, manifest.ProfileTS),
		HasPy:        slices.Contains(used, manifest.ProfilePy),
	}

	taskfile, err := render(rootTaskfile, data)
	if err != nil {
		return nil, err
	}

	lh := make([]lefthookComponent, 0, len(components))
	for _, c := range components {
		lh = append(lh, lefthookComponent{Component: c, Glob: profiles[c.Profile].glob})
	}
	hooks, err := render(lefthook, struct{ Components []lefthookComponent }{lh})
	if err != nil {
		return nil, err
	}

	resources := []resource.Resource{
		{Path: "Taskfile.yml", Ownership: resource.Generated, Content: taskfile},
		{Path: "lefthook.yml", Ownership: resource.Generated, Content: hooks},
	}
	// The hook:* tasks and the guard follow the claude integration, as in
	// the single-language repo-tooling modules (spec 0026).
	if module.WantsAgentHooks(mctx) {
		guard, err := guardResource(guardProfile.guard)
		if err != nil {
			return nil, err
		}
		resources = append(resources, guard)
	} else if resources[0].Content, err = module.StripAgentHooks(taskfile); err != nil {
		return nil, err
	}
	for _, c := range components {
		resources = append(resources, resource.Resource{
			Path:      c.Path + "/Taskfile.yml",
			Ownership: resource.Generated,
			Content:   profiles[c.Profile].taskfile,
		})
	}
	return resources, nil
}

// declared lists the profiles components use, in manifest.Profiles order.
func declared(components []manifest.Component) []manifest.Profile {
	var used []manifest.Profile
	for _, p := range manifest.Profiles {
		if slices.ContainsFunc(components, func(c manifest.Component) bool { return c.Profile == p }) {
			used = append(used, p)
		}
	}
	return used
}

// guardResource takes the guard from the single-language repo-tooling
// module that owns it, so there is one copy of each guard.
func guardResource(m module.Module) (resource.Resource, error) {
	rs, err := m.Resolve(context.Background(), nil)
	if err != nil {
		return resource.Resource{}, fmt.Errorf("resolve %s: %w", m.Name(), err)
	}
	for _, r := range rs {
		if strings.HasPrefix(r.Path, ".claude/hooks/guard.") {
			return r, nil
		}
	}
	return resource.Resource{}, fmt.Errorf("%s resolves no .claude/hooks/guard.*", m.Name())
}

func render(t *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", t.Name(), err)
	}
	return buf.Bytes(), nil
}
