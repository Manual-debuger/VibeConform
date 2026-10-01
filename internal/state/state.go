// Package state reads .vibe/state.yaml, the machine-owned record of what
// VibeConform last applied to a repository. See
// docs/decisions/0002-desired-state.md and
// docs/specs/0006-reconcile-diff-v1.md. vibe sync writes it (see
// docs/specs/0008-vibe-sync-v1.md); every other command only reads it.
package state

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/atomicfile"
)

const (
	// stateDirName is the repository-relative directory holding
	// VibeConform's machine-owned files.
	stateDirName = ".vibe"
	// stateFilePath is the repository-relative location of the state file.
	stateFilePath = ".vibe/state.yaml"
)

// SchemaVersion is the current .vibe/state.yaml layout.
//
// 1 is the implicit original: a bare resources map, with no record of what
// produced it. Files written before spec 0019 carry no schema field and are
// read as schema 1.
//
// 2 adds provenance — which vibe wrote the file, against which standard —
// so audit can tell a repository that fell behind from a binary that did.
// See docs/specs/0019-drift-classification.md.
//
// 3 adds, for a structured-patch resource, whether VibeConform created the
// file and a hash per owned element, so VibeConform can own entries in a
// file it shares with its users. Generated resources are recorded exactly
// as in schema 2. See docs/specs/0026-optional-integrations.md.
//
// 4 turns resources: from a map keyed by path into a sorted list of
// records, each naming its path, its ownership, and for a managed section
// its section ID, so one file can hold several independently owned
// sections without encoding two identities into one key. Schemas 1 to 3
// still load. See docs/specs/0029-text-policy.md.
const SchemaVersion = 4

// The ownership names schema 4 records.
const (
	ownershipGenerated       = "generated"
	ownershipStructuredPatch = "structured-patch"
	ownershipManagedSection  = "managed-section"
)

// ResourceState is what was last recorded for one resolved resource.
type ResourceState struct {
	// SHA256 is the hex-encoded content hash last applied for this path.
	// Empty for a structured-patch resource, which records Elements.
	SHA256 string `yaml:"sha256,omitempty"`
	// Created is true when VibeConform created a structured-patch file, so
	// it may delete the file once nothing but its skeleton remains.
	Created bool `yaml:"created,omitempty"`
	// Elements maps each owned element of a structured-patch resource,
	// keyed "<array>/<identity>", to its last applied hash.
	Elements map[string]ElementState `yaml:"elements,omitempty"`
}

// SectionKey identifies one managed section: the file, and the section's
// ID within it.
type SectionKey struct {
	Path string
	ID   string
}

// SectionState is what was last recorded for one managed section.
type SectionState struct {
	// SHA256 is the hex-encoded hash of the section's content, the bytes
	// between its markers.
	SHA256 string
	// Created is true when VibeConform created the file for this section,
	// so it may delete the file once removing the section leaves it empty.
	Created bool
}

// ElementState is what was last recorded for one owned element.
type ElementState struct {
	// SHA256 is the hex-encoded hash of the element's canonical JSON.
	SHA256 string `yaml:"sha256"`
}

// State is the parsed form of .vibe/state.yaml: what was recorded for each
// resource, by repository-relative path, and for each managed section, by
// path and section ID.
//
// The provenance fields are absent from every file written before spec
// 0019. Absent means unknown, never a default worth acting on: an empty
// VibeVersion must not be compared against anything (see CompareWriters),
// because "no version recorded" and "version zero" are different claims and
// only one of them is true.
type State struct {
	// Schema is the layout version of the file this State was read from, or
	// SchemaVersion for one being written. Zero means a pre-0019 file.
	Schema int `yaml:"schema,omitempty"`
	// VibeVersion is the version string of the vibe binary that last wrote
	// this file. Empty for a pre-0019 file, and "dev" for any build
	// GoReleaser did not stamp — neither is orderable, and CompareWriters
	// is the only thing that should decide what to do about that.
	VibeVersion string `yaml:"vibe_version,omitempty"`
	// Standard is the "<name>/<version>" the file was last written against.
	//
	// Recorded but not yet read. Nothing compares it to vibe.yaml today; a
	// mismatch between the two is a real condition worth detecting, but
	// detecting it is not part of spec 0019. Do not assume a check exists.
	Standard string `yaml:"standard,omitempty"`

	// Resources holds the whole-file and structured-patch entries, keyed by
	// path.
	Resources map[string]ResourceState `yaml:"-"`
	// Sections holds the managed-section entries.
	Sections map[SectionKey]SectionState `yaml:"-"`
}

// record is one schema 4 entry in resources:.
type record struct {
	Path      string                  `yaml:"path"`
	SectionID string                  `yaml:"section_id,omitempty"`
	Ownership string                  `yaml:"ownership"`
	Created   bool                    `yaml:"created,omitempty"`
	SHA256    string                  `yaml:"sha256,omitempty"`
	Elements  map[string]ElementState `yaml:"elements,omitempty"`
}

// file is the on-disk layout. Resources is a node because its shape
// depends on the schema: a mapping through schema 3, a list from 4.
type file struct {
	Schema      int       `yaml:"schema,omitempty"`
	VibeVersion string    `yaml:"vibe_version,omitempty"`
	Standard    string    `yaml:"standard,omitempty"`
	Resources   yaml.Node `yaml:"resources"`
}

// MarshalYAML writes s in the schema 4 layout, sorted by path then section
// ID so the file is deterministic.
func (s State) MarshalYAML() (any, error) {
	records := make([]record, 0, len(s.Resources)+len(s.Sections))
	for path, rs := range s.Resources {
		r := record{Path: path, Ownership: ownershipGenerated, SHA256: rs.SHA256}
		if rs.Elements != nil {
			r.Ownership, r.Created, r.Elements = ownershipStructuredPatch, rs.Created, rs.Elements
		}
		records = append(records, r)
	}
	for key, ss := range s.Sections {
		records = append(records, record{Path: key.Path, SectionID: key.ID, Ownership: ownershipManagedSection, Created: ss.Created, SHA256: ss.SHA256})
	}
	slices.SortFunc(records, func(a, b record) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.SectionID, b.SectionID))
	})

	var node yaml.Node
	if err := node.Encode(records); err != nil {
		return nil, err
	}
	return file{Schema: s.Schema, VibeVersion: s.VibeVersion, Standard: s.Standard, Resources: node}, nil
}

// UnmarshalYAML reads any schema. Through schema 3 resources: is a map
// keyed by path; from schema 4 it is a list of records. The node's kind
// tells them apart.
func (s *State) UnmarshalYAML(n *yaml.Node) error {
	var f file
	if err := n.Decode(&f); err != nil {
		return err
	}
	s.Schema, s.VibeVersion, s.Standard = f.Schema, f.VibeVersion, f.Standard
	s.Resources = map[string]ResourceState{}
	s.Sections = map[SectionKey]SectionState{}

	switch f.Resources.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		if f.Resources.Tag == "!!null" {
			return nil
		}
	case yaml.MappingNode:
		return f.Resources.Decode(&s.Resources)
	case yaml.SequenceNode:
		var records []record
		if err := f.Resources.Decode(&records); err != nil {
			return err
		}
		return s.addRecords(records)
	}
	return fmt.Errorf("line %d: resources must be a list", f.Resources.Line)
}

// addRecords files each schema 4 record under its ownership.
func (s *State) addRecords(records []record) error {
	seen := map[SectionKey]bool{}
	for i, r := range records {
		where := fmt.Sprintf("resources[%d] (%s)", i, r.Path)
		key := SectionKey{Path: r.Path, ID: r.SectionID}
		switch {
		case r.Path == "":
			return fmt.Errorf("resources[%d]: path is required", i)
		case seen[key]:
			return fmt.Errorf("%s: recorded twice", where)
		case r.SectionID != "" && r.Ownership != ownershipManagedSection:
			return fmt.Errorf("%s: section_id on a %s entry", where, r.Ownership)
		}
		seen[key] = true

		switch r.Ownership {
		case ownershipGenerated:
			s.Resources[r.Path] = ResourceState{SHA256: r.SHA256}
		case ownershipStructuredPatch:
			elements := r.Elements
			if elements == nil {
				elements = map[string]ElementState{}
			}
			s.Resources[r.Path] = ResourceState{Created: r.Created, Elements: elements}
		case ownershipManagedSection:
			if r.SectionID == "" {
				return fmt.Errorf("%s: a managed-section entry needs a section_id", where)
			}
			s.Sections[key] = SectionState{SHA256: r.SHA256, Created: r.Created}
		default:
			return fmt.Errorf("%s: unknown ownership %q", where, r.Ownership)
		}
	}
	return nil
}

// Load reads .vibe/state.yaml from repoRoot. A missing file is not an
// error: it returns an empty State, since no vibe sync has ever run.
//
// A file written before spec 0019 has no schema or provenance fields.
// Load accepts it unchanged and leaves those fields zero rather than
// inventing values for them — every existing repository's state file is
// one of these, and guessing would produce exactly the confident-but-wrong
// verdict spec 0019 exists to remove.
func Load(repoRoot string) (*State, error) {
	path := filepath.Join(repoRoot, stateFilePath)
	data, err := os.ReadFile(path) // #nosec G304 -- repoRoot is an operator-supplied CLI flag, same trust boundary as init.go's WriteFile target
	if errors.Is(err, os.ErrNotExist) {
		return &State{Resources: map[string]ResourceState{}, Sections: map[SectionKey]SectionState{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if s.Resources == nil {
		s.Resources = map[string]ResourceState{}
	}
	if s.Sections == nil {
		s.Sections = map[SectionKey]SectionState{}
	}
	return &s, nil
}

// Save writes s to .vibe/state.yaml under repoRoot, creating the .vibe
// directory if it does not exist. The write is atomic: an interrupted run
// leaves the previous state file intact rather than a truncated one, since
// a half-written state file would corrupt every subsequent reconciliation
// decision.
func Save(repoRoot string, s *State) error {
	if err := os.MkdirAll(filepath.Join(repoRoot, stateDirName), 0o750); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if err := atomicfile.Write(filepath.Join(repoRoot, stateFilePath), data, 0o600); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
