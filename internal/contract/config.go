package contract

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is a vibe.yaml version 2 configuration (spec 0051, "vibe.yaml"),
// validated against the repository it configures.
type Config struct {
	// Contract is the declared contract version and the one audited.
	Contract Versions
	// Platforms is a non-empty list of distinct platforms, in file order.
	Platforms []string
	// Agents is a list of distinct agents, in file order; may be empty.
	Agents []string
	// Components is the repository's components: declared, or detected
	// from the root's manifests when the configuration omits them.
	Components []Component
	// ComponentsDetected is true when Components was detected.
	ComponentsDetected bool
	// Entrypoints holds the declared entrypoints as written. Whether the
	// required ones are declared and well formed is V-04's to decide.
	Entrypoints Entrypoints
	// Waivers is the declared waivers, in file order. At most one per
	// rule, each for a SHOULD rule.
	Waivers []Waiver
}

// The platforms of contract 1.0.0.
const (
	PlatformLinux   = "linux"
	PlatformWindows = "windows"
	PlatformMacOS   = "macos"
)

// The agents of contract 1.0.0. Contract 1.0.0 has no Codex rules.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

// The component languages of contract 1.0.0.
const (
	LanguageGo         = "go"
	LanguagePython     = "python"
	LanguageTypeScript = "typescript"
)

var (
	platforms = []string{PlatformLinux, PlatformWindows, PlatformMacOS}
	agents    = []string{AgentClaude, AgentCodex}
	languages = []string{LanguageGo, LanguagePython, LanguageTypeScript}
)

// rootManifests maps each manifest that decides the root component's
// language, in the order errors name them.
var rootManifests = []struct{ file, language string }{
	{"go.mod", LanguageGo},
	{"pyproject.toml", LanguagePython},
	{"package.json", LanguageTypeScript},
}

// Component is one single-language part of the repository.
type Component struct {
	// Path is a clean, relative, slash-separated directory; "." is the
	// root.
	Path     string
	Language string
}

// The entrypoint names (spec 0051, "vibe.yaml").
const (
	EntrypointVerify     = "verify"
	EntrypointVerifyFast = "verify-fast"
	EntrypointFmt        = "fmt"
	EntrypointFmtCheck   = "fmt-check"
	EntrypointSetup      = "setup"
)

// RequiredEntrypoints lists the entrypoints V-04 requires, in report order.
var RequiredEntrypoints = []string{EntrypointVerify, EntrypointVerifyFast, EntrypointFmt, EntrypointFmtCheck}

var entrypointNames = append(slices.Clone(RequiredEntrypoints), EntrypointSetup)

// Entrypoint is one declared entrypoint.
type Entrypoint struct {
	// Declared is true when the configuration has the key, even with an
	// empty value.
	Declared bool
	// Commands holds the commands as written: one for a string, the list
	// for a list. A null value has none.
	Commands []string
}

// Entrypoints maps an entrypoint name to its declaration. A name that is
// not declared is absent.
type Entrypoints map[string]Entrypoint

// Waiver suspends one SHOULD rule until the end of a day (spec 0051,
// "vibe.yaml"; spec 0052, "Waivers").
type Waiver struct {
	Rule   string
	Reason string
	// Until is the last day the waiver is in force, at midnight UTC as
	// written (YYYY-MM-DD). The waiver is in force through the end of
	// that day in UTC.
	Until time.Time
}

// v1Fields says, for each field of the version 1 schema, where its content
// went in version 2 (spec 0052, Q23).
var v1Fields = map[string]string{
	"standard":     "contract replaces it: declare the contract version, such as contract: 1.0.0",
	"version":      "the contract version goes in contract, such as contract: 1.0.0",
	"integrations": "agents go in agents; editors and repository intelligence moved into the setup skill",
	"development":  "the docs layout and development workflow moved into the setup skill",
	"policy":       "repository policies such as line endings moved into the setup skill",
	"ci":           "contract 1.0.0 reads GitHub Actions workflows and needs no CI provider",
}

const v2FieldList = "contract, platforms, agents, components, entrypoints, waivers"

// Parse validates a version 2 configuration against the tracked files of
// repo. file names the configuration in errors. It reports the first
// error in document order as a *ConfigError; a required field that is
// missing comes after every field present. A contract version this vibe
// cannot audit is an *UnsupportedVersionError, decided before anything
// else, since a newer contract may have fields this vibe does not know.
func Parse(file string, data []byte, repo *Repo) (*Config, error) {
	top, err := decodeTop(file, data)
	if err != nil {
		return nil, err
	}
	if top == nil {
		top = &yaml.Node{Kind: yaml.MappingNode}
	}
	if v := field(top, "contract"); v != nil && isString(v) {
		if major, minor, ok := parseVersion(v.Value); ok && !supported(major, minor) {
			return nil, &UnsupportedVersionError{File: file, Declared: v.Value}
		}
	}

	p := &configParser{file: file, repo: repo, cfg: &Config{}}
	if err := p.top(top); err != nil {
		return nil, err
	}
	return p.cfg, nil
}

type configParser struct {
	file string
	repo *Repo
	cfg  *Config
}

func (p *configParser) fail(path, format string, args ...any) error {
	return &ConfigError{File: p.file, Path: path, Reason: fmt.Sprintf(format, args...)}
}

func (p *configParser) top(m *yaml.Node) error {
	seen := map[string]bool{}
	err := p.mapping(m, "", seen, func(key string, v *yaml.Node) error {
		switch key {
		case "contract":
			return p.contract(v)
		case "platforms":
			list, err := p.choices(key, v, platforms, false)
			p.cfg.Platforms = list
			return err
		case "agents":
			list, err := p.choices(key, v, agents, true)
			p.cfg.Agents = list
			return err
		case "components":
			return p.components(v)
		case "entrypoints":
			return p.entrypoints(v)
		case "waivers":
			return p.waivers(v)
		}
		if where, ok := v1Fields[key]; ok {
			return p.fail(key, "the version 2 schema has no %s field; %s", key, where)
		}
		return p.fail(key, "unknown field; the fields are %s", v2FieldList)
	})
	if err != nil {
		return err
	}

	for _, key := range []string{"contract", "platforms", "agents"} {
		if !seen[key] {
			return p.fail(key, "is required")
		}
	}
	if !seen["components"] {
		return p.detectComponents()
	}
	return nil
}

// mapping walks the pairs of a mapping node in document order. Every key
// must be a string and appear once; seen records the keys.
func (p *configParser) mapping(m *yaml.Node, prefix string, seen map[string]bool, each func(key string, v *yaml.Node) error) error {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := resolve(m.Content[i])
		at := prefix + k.Value
		if !isString(k) {
			return p.fail(at, "field names must be strings")
		}
		if seen[k.Value] {
			return p.fail(at, "appears more than once")
		}
		seen[k.Value] = true
		if err := each(k.Value, resolve(m.Content[i+1])); err != nil {
			return err
		}
	}
	return nil
}

func (p *configParser) contract(v *yaml.Node) error {
	if !isString(v) {
		return p.fail("contract", "must be a contract version X.Y.Z, such as %s", Implemented)
	}
	if _, _, ok := parseVersion(v.Value); !ok {
		return p.fail("contract", "%q is not a contract version X.Y.Z, such as %s", v.Value, Implemented)
	}
	// Parse already refused an unsupported version.
	p.cfg.Contract = Versions{Declared: v.Value, Audited: Implemented}
	return nil
}

// choices reads a list of distinct values from allowed. An empty list is
// valid only when mayBeEmpty.
func (p *configParser) choices(key string, v *yaml.Node, allowed []string, mayBeEmpty bool) ([]string, error) {
	options := strings.Join(allowed, ", ")
	if v.Kind != yaml.SequenceNode {
		return nil, p.fail(key, "must be a list of values from %s", options)
	}
	if len(v.Content) == 0 && !mayBeEmpty {
		return nil, p.fail(key, "must list at least one of %s", options)
	}
	list := []string{}
	for i, item := range v.Content {
		item = resolve(item)
		at := fmt.Sprintf("%s[%d]", key, i)
		if !isString(item) {
			return nil, p.fail(at, "must be one of %s", options)
		}
		if !slices.Contains(allowed, item.Value) {
			return nil, p.fail(at, "%q is not one of %s", item.Value, options)
		}
		if j := slices.Index(list, item.Value); j >= 0 {
			return nil, p.fail(at, "%s is listed twice (also %s[%d])", item.Value, key, j)
		}
		list = append(list, item.Value)
	}
	return list, nil
}

func (p *configParser) components(v *yaml.Node) error {
	if v.Kind != yaml.SequenceNode {
		return p.fail("components", "must be a list of {path, language}")
	}
	if len(v.Content) == 0 {
		return p.fail("components", "must list at least one component; omit components to detect the root's language")
	}
	for i, item := range v.Content {
		item = resolve(item)
		at := fmt.Sprintf("components[%d]", i)
		if item.Kind != yaml.MappingNode {
			return p.fail(at, "must be a map with path and language")
		}
		var c Component
		seen := map[string]bool{}
		err := p.mapping(item, at+".", seen, func(key string, v *yaml.Node) error {
			switch key {
			case "path":
				path, err := p.componentPath(at+".path", v)
				c.Path = path
				return err
			case "language":
				if !isString(v) || !slices.Contains(languages, v.Value) {
					return p.fail(at+".language", "%s is not one of %s", describe(v), strings.Join(languages, ", "))
				}
				c.Language = v.Value
				return nil
			}
			return p.fail(at+"."+key, "unknown field; a component has path and language")
		})
		if err != nil {
			return err
		}
		for _, key := range []string{"path", "language"} {
			if !seen[key] {
				return p.fail(at+"."+key, "is required")
			}
		}
		p.cfg.Components = append(p.cfg.Components, c)
	}
	return nil
}

// componentPath checks a component's path: clean, relative, slash
// separated, a directory holding tracked files, distinct from and not
// nesting with the components before it.
func (p *configParser) componentPath(at string, v *yaml.Node) (string, error) {
	if !isString(v) || v.Value == "" {
		return "", p.fail(at, "must be a directory relative to the repository root, such as . or services/api")
	}
	dir := v.Value
	switch {
	case strings.Contains(dir, `\`):
		return "", p.fail(at, "%q must use forward slashes", dir)
	case strings.HasPrefix(dir, "/") || hasDrive(dir):
		return "", p.fail(at, "%q must be relative to the repository root", dir)
	case path.Clean(dir) != dir:
		return "", p.fail(at, "%q is not clean; write %q", dir, path.Clean(dir))
	case dir == ".." || strings.HasPrefix(dir, "../"):
		return "", p.fail(at, "%q must be inside the repository", dir)
	case !p.repo.HasDir(dir):
		return "", p.fail(at, "%q is not a directory with tracked files", dir)
	}
	for j, other := range p.cfg.Components {
		switch {
		case other.Path == dir:
			return "", p.fail(at, "%q is already components[%d]", dir, j)
		case within(dir, other.Path):
			return "", p.fail(at, "%q is inside components[%d] (%q); components do not nest", dir, j, other.Path)
		case within(other.Path, dir):
			return "", p.fail(at, "%q contains components[%d] (%q); components do not nest", dir, j, other.Path)
		}
	}
	return dir, nil
}

// within reports whether dir lies strictly inside parent.
func within(dir, parent string) bool {
	if parent == "." {
		return dir != "."
	}
	return strings.HasPrefix(dir, parent+"/")
}

func hasDrive(p string) bool {
	return len(p) >= 2 && p[1] == ':' && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z')
}

// detectComponents decides the root component's language from the tracked
// manifests at the root: exactly one must be there.
func (p *configParser) detectComponents() error {
	var found []string
	language := ""
	for _, m := range rootManifests {
		if p.repo.HasFile(m.file) {
			found = append(found, m.file)
			language = m.language
		}
	}
	all := make([]string, len(rootManifests))
	for i, m := range rootManifests {
		all[i] = m.file
	}
	switch len(found) {
	case 0:
		return p.fail("components", "the repository root has no tracked %s, so its language is unknown; "+
			"declare components with path and language", joinWords(all, "or"))
	case 1:
		p.cfg.Components = []Component{{Path: ".", Language: language}}
		p.cfg.ComponentsDetected = true
		return nil
	default:
		return p.fail("components", "the repository root has %s, so its language is ambiguous; "+
			"declare components with path and language", joinWords(found, "and"))
	}
}

func (p *configParser) entrypoints(v *yaml.Node) error {
	p.cfg.Entrypoints = Entrypoints{}
	if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
		return nil
	}
	if v.Kind != yaml.MappingNode {
		return p.fail("entrypoints", "must be a map from entrypoint to command, such as verify: task verify")
	}
	return p.mapping(v, "entrypoints.", map[string]bool{}, func(key string, v *yaml.Node) error {
		at := "entrypoints." + key
		if !slices.Contains(entrypointNames, key) {
			return p.fail(at, "unknown entrypoint; the entrypoints are %s", strings.Join(entrypointNames, ", "))
		}
		e := Entrypoint{Declared: true}
		switch {
		case v.Kind == yaml.ScalarNode && v.Tag == "!!null":
		case isString(v):
			e.Commands = []string{v.Value}
		case v.Kind == yaml.SequenceNode:
			for i, item := range v.Content {
				item = resolve(item)
				if !isString(item) {
					return p.fail(fmt.Sprintf("%s[%d]", at, i), "must be a command string")
				}
				e.Commands = append(e.Commands, item.Value)
			}
		default:
			return p.fail(at, "must be a command or a list of commands")
		}
		p.cfg.Entrypoints[key] = e
		return nil
	})
}

func (p *configParser) waivers(v *yaml.Node) error {
	if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
		return nil
	}
	if v.Kind != yaml.SequenceNode {
		return p.fail("waivers", "must be a list of {rule, reason, until}")
	}
	for i, item := range v.Content {
		item = resolve(item)
		at := fmt.Sprintf("waivers[%d]", i)
		if item.Kind != yaml.MappingNode {
			return p.fail(at, "must be a map with rule, reason and until")
		}
		var w Waiver
		seen := map[string]bool{}
		err := p.mapping(item, at+".", seen, func(key string, v *yaml.Node) error {
			switch key {
			case "rule":
				return p.waiverRule(at+".rule", v, &w)
			case "reason":
				if !isString(v) {
					return p.fail(at+".reason", "must be a string saying why the rule is waived")
				}
				if strings.TrimSpace(v.Value) == "" {
					return p.fail(at+".reason", "must not be empty: say why the rule is waived")
				}
				w.Reason = v.Value
				return nil
			case "until":
				day, err := time.Parse(time.DateOnly, v.Value)
				if v.Kind != yaml.ScalarNode || err != nil {
					return p.fail(at+".until", "%s is not a date YYYY-MM-DD", describe(v))
				}
				w.Until = day
				return nil
			}
			return p.fail(at+"."+key, "unknown field; a waiver has rule, reason and until")
		})
		if err != nil {
			return err
		}
		for _, key := range []string{"rule", "reason", "until"} {
			if !seen[key] {
				return p.fail(at+"."+key, "is required")
			}
		}
		p.cfg.Waivers = append(p.cfg.Waivers, w)
	}
	return nil
}

func (p *configParser) waiverRule(at string, v *yaml.Node, w *Waiver) error {
	r, ok := LookupRule(v.Value)
	if !isString(v) || !ok {
		return p.fail(at, "%s is not a rule of contract %s", describe(v), Implemented)
	}
	if r.Level != Should {
		return p.fail(at, "%s is a %s rule; only SHOULD rules can be waived", r.ID, r.Level)
	}
	for j, other := range p.cfg.Waivers {
		if other.Rule == r.ID {
			return p.fail(at, "%s already has a waiver (waivers[%d]); one waiver per rule", r.ID, j)
		}
	}
	w.Rule = r.ID
	return nil
}

// field returns the value of the first top-level key named key, or nil.
func field(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return resolve(m.Content[i+1])
		}
	}
	return nil
}

// isString reports whether n is a plain string scalar.
func isString(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!str"
}

// describe quotes a scalar's value for an error, and names any other node
// by its kind.
func describe(n *yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return fmt.Sprintf("%q", n.Value)
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "a map"
	default:
		return "this value"
	}
}

// joinWords joins words as "a, b and c" (or "or").
func joinWords(words []string, conj string) string {
	if len(words) <= 1 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " " + conj + " " + words[len(words)-1]
}
