package manifest

import "fmt"

// DocsDirs is where the docs layout puts each kind of document, as
// slash-separated paths from the repository root. Development and
// Operations are empty unless their directory is selected (spec 0033).
type DocsDirs struct {
	Specs, Architecture, Decisions string
	Development, Operations        string
}

// The keys that adopt an existing directory for one kind of document
// instead of the default (spec 0034).
const (
	DevelopmentSpecsDir        = "specs_dir"
	DevelopmentArchitectureDir = "architecture_dir"
	DevelopmentDecisionsDir    = "decisions_dir"
	DevelopmentDevelopmentDir  = "development_dir"
	DevelopmentOperationsDir   = "operations_dir"
)

// docsDir is one kind of document: its path key, its default, the value
// vibe.yaml gives the key, and whether the directory is in the layout.
type docsDir struct {
	key, def string
	set      *string
	in       bool
	// needs names the key that must be set for set to be allowed, other
	// than docs_layout.
	needs string
}

func (d *Development) docsDirs() []docsDir {
	return []docsDir{
		{key: DevelopmentSpecsDir, def: "docs/specs", set: d.SpecsDir, in: true},
		{key: DevelopmentArchitectureDir, def: "docs/architecture", set: d.ArchitectureDir, in: true},
		{key: DevelopmentDecisionsDir, def: "docs/decisions", set: d.DecisionsDir, in: true},
		{key: DevelopmentDevelopmentDir, def: "docs/development", set: d.DevelopmentDir, in: d.DocsDevelopment != nil,
			needs: DevelopmentDocsDevelopment},
		{key: DevelopmentOperationsDir, def: "docs/operations", set: d.OperationsDir, in: d.DocsOperations != nil,
			needs: DevelopmentDocsOperations},
	}
}

func (dd docsDir) path() string {
	if dd.set != nil {
		return *dd.set
	}
	return dd.def
}

// DocsDirs returns the directories of the layout d selects: each default,
// or the path vibe.yaml adopts instead. A nil *Development selects the
// defaults and neither optional directory.
func (d *Development) DocsDirs() DocsDirs {
	if d == nil {
		d = &Development{}
	}
	out := make([]string, 0, 5)
	for _, dd := range d.docsDirs() {
		if !dd.in {
			out = append(out, "")
			continue
		}
		out = append(out, dd.path())
	}
	return DocsDirs{Specs: out[0], Architecture: out[1], Decisions: out[2], Development: out[3], Operations: out[4]}
}

// AdoptedDirs lists the directories vibe.yaml adopts, by key, in key
// order: the ones that must already exist (spec 0034 §1).
func (d *Development) AdoptedDirs() []ScalarKey {
	if d == nil {
		return nil
	}
	var keys []ScalarKey
	for _, dd := range d.docsDirs() {
		if dd.set != nil {
			keys = append(keys, ScalarKey{Map: MapDevelopment, Key: dd.key})
		}
	}
	return keys
}

// DocsDir returns the path vibe.yaml gives the path key k, or "".
func (d *Development) DocsDir(key string) string {
	if d == nil {
		return ""
	}
	for _, dd := range d.docsDirs() {
		if dd.key == key && dd.set != nil {
			return *dd.set
		}
	}
	return ""
}

// validateDevelopment checks the path keys of spec 0034: each a clean
// relative path, each with its prerequisite, and no two directories of
// the layout overlapping. Whether a path exists is checked against the
// repository, not here.
func validateDevelopment(d *Development) error {
	if d == nil {
		return nil
	}
	dirs := d.docsDirs()
	for _, dd := range dirs {
		if dd.set == nil {
			continue
		}
		where := fmt.Sprintf("development.%s (%s)", dd.key, *dd.set)
		if *dd.set == "." {
			return fmt.Errorf("%s: the repository root cannot be a docs directory", where)
		}
		if err := validatePath(*dd.set); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if d.DocsLayout == nil {
			return fmt.Errorf("%s: requires development.%s", where, DevelopmentDocsLayout)
		}
		if dd.needs != "" && !dd.in {
			return fmt.Errorf("%s: requires development.%s: on", where, dd.needs)
		}
	}
	for i, a := range dirs {
		for _, b := range dirs[:i] {
			if !a.in || !b.in || (a.set == nil && b.set == nil) {
				continue
			}
			if within(a.path(), b.path()) || within(b.path(), a.path()) {
				return fmt.Errorf("development.%s (%s): overlaps development.%s (%s)", a.key, a.path(), b.key, b.path())
			}
		}
	}
	return nil
}
