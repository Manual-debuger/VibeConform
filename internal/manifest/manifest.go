// Package manifest parses vibe.yaml, the human-owned desired-state
// declaration described in docs/architecture/overview.md. Only the fields
// needed to identify a repository's standard are implemented at bootstrap
// time; module/component composition is added as those subsystems land.
package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Manifest is the parsed form of vibe.yaml.
type Manifest struct {
	// Standard is the name of the versioned production standard this
	// repository conforms to, e.g. "production".
	Standard string `yaml:"standard"`
	// Version pins the standard revision, e.g. "v1".
	Version string `yaml:"version"`
}

// Parse decodes a vibe.yaml document.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Standard == "" {
		return nil, fmt.Errorf("parse manifest: %q is required", "standard")
	}
	return &m, nil
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
