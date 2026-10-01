// Package manifest parses vibe.yaml, the human-owned desired-state
// declaration described in docs/architecture/overview.md: the standard a
// repository conforms to and, for a standard that takes them, the
// components it is made of (docs/decisions/0012-manifest-components.md).
package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest is the parsed form of vibe.yaml.
type Manifest struct {
	// Standard is the name of the versioned production standard this
	// repository conforms to, e.g. "prod-go".
	Standard string `yaml:"standard"`
	// Version pins the standard revision, e.g. "v1".
	Version string `yaml:"version"`
	// Components lists the single-language projects a monorepo standard
	// composes, in the order every report and fan-out task uses.
	Components []Component `yaml:"components,omitempty"`
	// Integrations selects the optional editor, agent, and
	// code-intelligence integrations; nil means the standard's defaults
	// (docs/decisions/0013-optional-integrations.md).
	Integrations *Integrations `yaml:"integrations,omitempty"`
	// Policy selects repository policies; nil means none, since every
	// policy is opt-in (docs/decisions/0014-managed-sections.md).
	Policy *Policy `yaml:"policy,omitempty"`
	// Development selects how people and agents work in the repository;
	// nil means nothing, since every setting is opt-in
	// (docs/decisions/0015-agents-md-workflow-section.md).
	Development *Development `yaml:"development,omitempty"`
}

// Policy is vibe.yaml's policy: map. Each key takes one value, and an
// absent key selects nothing.
type Policy struct {
	LineEndings *string `yaml:"line_endings,omitempty"`
}

// PolicyLineEndings is the line-ending policy's key.
const PolicyLineEndings = "line_endings"

// Development is vibe.yaml's development: map. Each key takes one value,
// and an absent key selects nothing.
type Development struct {
	Workflow   *string `yaml:"workflow,omitempty"`
	DocsLayout *string `yaml:"docs_layout,omitempty"`
}

// The development: map's keys.
const (
	DevelopmentWorkflow   = "workflow"
	DevelopmentDocsLayout = "docs_layout"
)

// The top-level maps whose keys each take one value.
const (
	MapPolicy      = "policy"
	MapDevelopment = "development"
)

// ScalarKey names one single-valued key: the map it is under and its key
// within it, e.g. development.workflow.
type ScalarKey struct {
	Map string
	Key string
}

// ScalarKeys lists every single-valued key in resolution order.
var ScalarKeys = []ScalarKey{
	{MapPolicy, PolicyLineEndings},
	{MapDevelopment, DevelopmentWorkflow},
	{MapDevelopment, DevelopmentDocsLayout},
}

// Scalar returns the value vibe.yaml gives k, or nil when it is absent.
func (m *Manifest) Scalar(k ScalarKey) *string {
	switch k.Map {
	case MapPolicy:
		return m.Policy.Get(k.Key)
	case MapDevelopment:
		return m.Development.Get(k.Key)
	}
	return nil
}

// Get returns the value vibe.yaml gives key, or nil when it is absent. A
// nil *Development has every key absent.
func (d *Development) Get(key string) *string {
	if d == nil {
		return nil
	}
	switch key {
	case DevelopmentWorkflow:
		return d.Workflow
	case DevelopmentDocsLayout:
		return d.DocsLayout
	}
	return nil
}

// Get returns the value vibe.yaml gives key, or nil when it is absent. A
// nil *Policy has every key absent.
func (p *Policy) Get(key string) *string {
	if p == nil {
		return nil
	}
	if key == PolicyLineEndings {
		return p.LineEndings
	}
	return nil
}

// Integrations is vibe.yaml's integrations: map. Each category is a
// pointer so that an absent category (nil: the standard's defaults) is
// distinguishable from an empty one (none).
type Integrations struct {
	Editors      *[]string `yaml:"editors,omitempty"`
	Agents       *[]string `yaml:"agents,omitempty"`
	Intelligence *[]string `yaml:"intelligence,omitempty"`
}

// The integration categories, in the order integrations resolve and
// report.
const (
	CategoryEditors      = "editors"
	CategoryAgents       = "agents"
	CategoryIntelligence = "intelligence"
)

// Categories lists every integration category in resolution order.
var Categories = []string{CategoryEditors, CategoryAgents, CategoryIntelligence}

// Get returns the names vibe.yaml selects for category, or nil when the
// category is absent. A nil *Integrations has every category absent.
func (in *Integrations) Get(category string) *[]string {
	if in == nil {
		return nil
	}
	switch category {
	case CategoryEditors:
		return in.Editors
	case CategoryAgents:
		return in.Agents
	case CategoryIntelligence:
		return in.Intelligence
	}
	return nil
}

// Profile is the language a component is written in.
type Profile string

// The profiles a component may declare: the language half of the
// single-language standards' names.
const (
	ProfileGo Profile = "go"
	ProfileTS Profile = "ts"
	ProfilePy Profile = "py"
)

// Profiles lists every valid profile, in the order a choice between them
// is made (for example, which runtime runs the agent guard).
var Profiles = []Profile{ProfileGo, ProfileTS, ProfilePy}

// Component is one single-language project inside a repository.
type Component struct {
	// ID names the component. It is a Task namespace and a CI job name.
	ID string `yaml:"id"`
	// Path is the component's directory, slash-separated and relative to
	// the repository root.
	Path string `yaml:"path"`
	// Profile is the component's language.
	Profile Profile `yaml:"profile"`
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// reservedIDs would collide, as a Task namespace or a CI job key, with a
// name the generated root Taskfile.yml or ci.yml already uses: its
// includes (local, vibe), its tasks' first segments, and CI's own jobs.
var reservedIDs = []string{
	"audit", "fmt", "gate", "hook", "lint", "local", "test",
	"typecheck", "verify", "verify-ci", "vibe", "workflows",
}

// Parse decodes a vibe.yaml document. Decoding is strict: an unknown key
// is an error, since a misspelled field would otherwise be silently
// ignored.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Standard == "" {
		return nil, fmt.Errorf("parse manifest: %q is required", "standard")
	}
	if err := validateComponents(m.Components); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := validateIntegrations(m.Integrations); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

func validateComponents(components []Component) error {
	ids := map[string]bool{}
	for i, c := range components {
		where := fmt.Sprintf("components[%d]", i)
		if !idPattern.MatchString(c.ID) {
			return fmt.Errorf("%s: id %q must match %s", where, c.ID, idPattern)
		}
		if slices.Contains(reservedIDs, c.ID) {
			return fmt.Errorf("%s: id %q is reserved (reserved: %s)", where, c.ID, strings.Join(reservedIDs, ", "))
		}
		if ids[c.ID] {
			return fmt.Errorf("%s: duplicate id %q", where, c.ID)
		}
		ids[c.ID] = true

		if !slices.Contains(Profiles, c.Profile) {
			return fmt.Errorf("%s (%s): profile %q must be one of %v", where, c.ID, c.Profile, Profiles)
		}

		if err := validatePath(c.Path); err != nil {
			return fmt.Errorf("%s (%s): %w", where, c.ID, err)
		}
		for _, other := range components[:i] {
			if within(c.Path, other.Path) || within(other.Path, c.Path) {
				return fmt.Errorf("%s (%s): path %q overlaps component %s at %q", where, c.ID, c.Path, other.ID, other.Path)
			}
		}
	}
	return nil
}

// validateIntegrations checks each selected name's form and uniqueness
// within its category. Whether a name exists is the standard's to say,
// since the catalog belongs to it.
func validateIntegrations(in *Integrations) error {
	for _, category := range Categories {
		names := in.Get(category)
		if names == nil {
			continue
		}
		seen := map[string]bool{}
		for i, name := range *names {
			where := fmt.Sprintf("integrations.%s[%d] (%s)", category, i, name)
			if !idPattern.MatchString(name) {
				return fmt.Errorf("%s: name must match %s", where, idPattern)
			}
			if seen[name] {
				return fmt.Errorf("%s: duplicate name", where)
			}
			seen[name] = true
		}
	}
	return nil
}

func validatePath(p string) error {
	switch {
	case p == "":
		return errors.New("path is required")
	case strings.Contains(p, `\`):
		return fmt.Errorf("path %q must use forward slashes", p)
	case path.IsAbs(p) || strings.Contains(p, ":"):
		return fmt.Errorf("path %q must be relative to the repository root", p)
	case path.Clean(p) != p:
		return fmt.Errorf("path %q must be clean (%q)", p, path.Clean(p))
	case p == ".":
		return errors.New(`path "." is the repository root; a component must be a subdirectory`)
	case p == ".." || strings.HasPrefix(p, "../"):
		return fmt.Errorf("path %q is outside the repository", p)
	}
	return nil
}

// within reports whether p is dir or inside it.
func within(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// New constructs a Manifest for the given standard and version.
func New(standard, version string) (*Manifest, error) {
	if standard == "" {
		return nil, fmt.Errorf("new manifest: %q is required", "standard")
	}
	return &Manifest{Standard: standard, Version: version}, nil
}

// Marshal encodes the manifest back into vibe.yaml form.
func (m *Manifest) Marshal() ([]byte, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	return data, nil
}
