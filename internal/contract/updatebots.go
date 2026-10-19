package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/tailscale/hujson"
	"gopkg.in/yaml.v3"
)

// Update bots as D-02 sees them (spec 0052, "Recognition rules", Update
// bots): Dependabot and Renovate.

var dependabotFiles = []string{".github/dependabot.yml", ".github/dependabot.yaml"}

// dependabotEcosystems are the package-ecosystem values that cover a
// component's language.
var dependabotEcosystems = map[string][]string{
	LanguageGo:         {"gomod"},
	LanguagePython:     {"pip", "uv"},
	LanguageTypeScript: {"npm", "bun"},
}

// renovateFile is one place Renovate reads its configuration from. jwcc
// files are JSON5 forms, read as JSON with comments and trailing commas.
type renovateFile struct {
	file string
	jwcc bool
}

// renovateFiles are Renovate's configuration files in the order it looks
// for them; it reads the first it finds. A root package.json's renovate
// key comes after them.
var renovateFiles = []renovateFile{
	{"renovate.json", false}, {"renovate.json5", true},
	{".github/renovate.json", false}, {".github/renovate.json5", true},
	{".gitlab/renovate.json", false}, {".gitlab/renovate.json5", true},
	{".renovaterc", false}, {".renovaterc.json", false}, {".renovaterc.json5", true},
}

// renovateManagers are the Renovate managers that cover a component's
// language. enabledManagers leaves a component out when it names none
// of them.
var renovateManagers = map[string][]string{
	LanguageGo:         {"gomod"},
	LanguagePython:     {"pep621", "poetry", "pip_requirements", "pip-compile", "pipenv", "setup-cfg", "pip_setup"},
	LanguageTypeScript: {"npm", "bun"},
}

const githubActions = "github-actions"

// Renovate's GitLab CI managers (spec 0051, after the D table): gitlabci
// updates .gitlab-ci.yml, gitlabci-include its project: includes.
const (
	gitlabCIManager      = "gitlabci"
	gitlabIncludeManager = "gitlabci-include"
)

// updateBot is one update bot's configuration as D-02 judges it.
type updateBot struct {
	// File is the configuration file findings name.
	File string
	// Problem is set when the configuration cannot be judged: fail when it
	// does not parse, cannot decide when the audit cannot read it.
	Problem *PartResult
	// Uncovered maps a component's index to the finding that says the
	// bot does not cover it; CI holds the findings for the CI's own
	// dependencies (github-actions, or gitlabci on GitLab CI), and is
	// empty when the bot covers them.
	Uncovered map[int]Finding
	CI        []Finding
	// Automerge holds a finding per place that enables automerge.
	Automerge []Finding
}

// renovateFix is the fix for a GitLab CI repository without a Renovate
// configuration that D-02 accepts.
const renovateFix = "add renovate.json with Renovate covering each component's manager and gitlabci " +
	"(and gitlabci-include when you list enabledManagers and .gitlab-ci.yml has project: includes), without automerge"

// onGitLab reports whether the C rules read GitLab CI, with the model.
func onGitLab(env *Env) (*CI, bool) {
	ci, _, ok := env.CI()
	if ok && ci.Provider == CIGitLab {
		return ci, true
	}
	return nil, false
}

// checkD02: Dependabot or Renovate covers each component's ecosystem and
// github-actions, and enables no automerge. With both bots configured,
// either may cover a component, and automerge in either fails it. When
// the C rules read GitLab CI, only Renovate counts, and it must cover
// gitlabci instead of github-actions (spec 0055, D-02).
func checkD02(env *Env) PartResult {
	_, gitlab := onGitLab(env)
	var bots []*updateBot
	dependabotBot := dependabot(env)
	if dependabotBot != nil && !gitlab {
		bots = append(bots, dependabotBot)
	}
	b, pkgProblem := renovate(env)
	if b != nil {
		bots = append(bots, b)
	}
	if len(bots) == 0 {
		if pkgProblem != nil {
			return *pkgProblem
		}
		if gitlab {
			if dependabotBot != nil {
				return Fail(Finding{
					File:    dependabotBot.File,
					Message: "Dependabot does not run on GitLab, so it cannot satisfy D-02 when the CI is GitLab CI",
					Fix:     renovateFix,
				})
			}
			return Fail(Finding{
				File:    renovateFiles[0].file,
				Message: "no update bot (Renovate) is configured",
				Fix:     renovateFix,
			})
		}
		return Fail(Finding{
			File:    dependabotFiles[0],
			Message: "no update bot (Dependabot or Renovate) is configured",
			Fix: "add " + dependabotFiles[0] + " with an updates entry for each component's ecosystem " +
				"and one for github-actions, without automerge",
		})
	}

	var problems []PartResult
	for _, b := range bots {
		if b.Problem != nil {
			problems = append(problems, *b.Problem)
		}
	}
	if len(problems) > 0 {
		return Merge(problems...)
	}

	var findings []Finding
	for i := range env.Config.Components {
		if f, all := uncoveredByAll(bots, func(b *updateBot) (Finding, bool) { f, ok := b.Uncovered[i]; return f, ok }); all {
			findings = append(findings, f)
		}
	}
	if fs, all := uncoveredByAll(bots, func(b *updateBot) ([]Finding, bool) { return b.CI, len(b.CI) > 0 }); all {
		findings = append(findings, fs...)
	}
	for _, b := range bots {
		findings = append(findings, b.Automerge...)
	}
	if len(findings) > 0 {
		return Fail(findings...)
	}
	return Pass()
}

// uncoveredByAll reports whether no bot covers a requirement, with the
// first bot's finding.
func uncoveredByAll[T any](bots []*updateBot, uncovered func(*updateBot) (T, bool)) (T, bool) {
	var first, none T
	for i, b := range bots {
		f, ok := uncovered(b)
		if !ok {
			return none, false
		}
		if i == 0 {
			first = f
		}
	}
	return first, true
}

// dependabot reads .github/dependabot.yml (or .yaml). Automerge is a
// workflow that runs gh pr merge with --auto.
func dependabot(env *Env) *updateBot {
	var file string
	for _, f := range dependabotFiles {
		if env.Repo.HasFile(f) {
			file = f
			break
		}
	}
	if file == "" {
		return nil
	}
	b := &updateBot{File: file, Uncovered: map[int]Finding{}}
	updates, err := dependabotUpdates(env, file)
	if err != nil {
		res := Fail(ParseFailure(file, err))
		b.Problem = &res
		return b
	}
	for i, c := range env.Config.Components {
		ecosystems := dependabotEcosystems[c.Language]
		if !slices.ContainsFunc(updates, func(u dependabotUpdate) bool { return u.covers(ecosystems, c.Path) }) {
			b.Uncovered[i] = Finding{
				File:    file,
				Message: fmt.Sprintf("no updates entry for %s covers component %s", joinWords(ecosystems, "or"), c.Path),
				Fix: fmt.Sprintf("add an updates entry with package-ecosystem: %s and directory: %s",
					ecosystems[0], dependabotDir(c.Path)),
			}
		}
	}
	if !slices.ContainsFunc(updates, func(u dependabotUpdate) bool { return u.ecosystem == githubActions }) {
		b.CI = []Finding{{
			File:    file,
			Message: "no updates entry has package-ecosystem github-actions",
			Fix:     "add an updates entry with package-ecosystem: github-actions and directory: /",
		}}
	}
	b.Automerge = workflowAutomerge(env)
	return b
}

// dependabotUpdate is one entry of updates.
type dependabotUpdate struct {
	ecosystem string
	// directory is directory:, which Dependabot takes literally.
	directory string
	// directories are directories:, which may hold globs.
	directories []string
}

func dependabotUpdates(env *Env, file string) ([]dependabotUpdate, error) {
	root, err := env.readYAMLMap(file)
	if err != nil {
		return nil, err
	}
	list := mapValue(root, "updates")
	if list == nil {
		return nil, nil
	}
	if list.Kind != yaml.SequenceNode {
		return nil, lineError(list, "updates must be a list")
	}
	var out []dependabotUpdate
	for i, n := range list.Content {
		n = resolve(n)
		if n.Kind != yaml.MappingNode {
			return nil, lineError(n, fmt.Sprintf("updates[%d] must be a map", i))
		}
		var u dependabotUpdate
		if v := mapValue(n, "package-ecosystem"); v != nil && v.Kind == yaml.ScalarNode {
			u.ecosystem = v.Value
		}
		if v := mapValue(n, "directory"); v != nil && v.Kind == yaml.ScalarNode {
			u.directory = v.Value
		}
		if v := mapValue(n, "directories"); v != nil {
			u.directories = stringList(v)
		}
		out = append(out, u)
	}
	return out, nil
}

// covers reports whether the entry updates one of ecosystems in the
// component at dir (slash separated, "." for the root).
func (u dependabotUpdate) covers(ecosystems []string, dir string) bool {
	if !slices.Contains(ecosystems, u.ecosystem) {
		return false
	}
	want := pathSegments(dir)
	if u.directory != "" && slices.Equal(pathSegments(u.directory), want) {
		return true
	}
	for _, d := range u.directories {
		if globSegments(pathSegments(d), want) {
			return true
		}
	}
	return false
}

// dependabotDir writes a component path the way Dependabot's directory
// does: from the repository root, starting with /.
func dependabotDir(p string) string {
	if p == "." {
		return "/"
	}
	return "/" + p
}

// pathSegments splits a repository path into its directories; the root
// ("/", ".", "") has none.
func pathSegments(p string) []string {
	p = strings.Trim(path.Clean("/"+p), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// globSegments matches path segments against a pattern of segments, where
// ** matches any number of directories and each other segment is a
// path.Match pattern.
func globSegments(pattern, segs []string) bool {
	if len(pattern) == 0 {
		return len(segs) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if globSegments(pattern[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if ok, err := path.Match(pattern[0], segs[0]); err != nil || !ok {
		return false
	}
	return globSegments(pattern[1:], segs[1:])
}

// workflowAutomerge finds every workflow step that runs gh pr merge with
// --auto. A workflow that does not parse does not run, so it merges
// nothing.
func workflowAutomerge(env *Env) []Finding {
	var out []Finding
	for _, wf := range env.Workflows().Files {
		if wf.Err != nil {
			continue
		}
		for _, j := range wf.Jobs {
			for _, s := range j.Steps {
				for _, c := range s.Run.Commands {
					if c.HasPrefix("gh", "pr", "merge") && slices.ContainsFunc(c.Words, func(w Word) bool {
						return w.Literal && w.Text == "--auto"
					}) {
						out = append(out, Finding{
							File: wf.File,
							Message: fmt.Sprintf("job %s runs gh pr merge --auto (step at line %d), which merges update pull requests without review",
								j.ID, s.Line),
							Fix: "remove --auto from gh pr merge, so that dependency updates are reviewed before they merge",
						})
					}
				}
			}
		}
	}
	return out
}

// renovate reads the first Renovate configuration found, or a root
// package.json's renovate key. pkgProblem is set when package.json is
// tracked and does not parse, so its renovate key cannot be read.
func renovate(env *Env) (b *updateBot, pkgProblem *PartResult) {
	for _, rf := range renovateFiles {
		if !env.Repo.HasFile(rf.file) {
			continue
		}
		b := &updateBot{File: rf.file, Uncovered: map[int]Finding{}}
		cfg, res := readRenovateFile(env, rf)
		if res != nil {
			b.Problem = res
			return b, nil
		}
		judgeRenovate(env, b, cfg)
		return b, nil
	}

	const pkg = "package.json"
	data, err := env.ReadFile(pkg)
	if err != nil {
		return nil, nil
	}
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		res := Fail(ParseFailure(pkg, err))
		return nil, &res
	}
	cfg, ok := top["renovate"].(map[string]any)
	if !ok {
		return nil, nil
	}
	b = &updateBot{File: pkg, Uncovered: map[int]Finding{}}
	judgeRenovate(env, b, cfg)
	return b, nil
}

func readRenovateFile(env *Env, rf renovateFile) (map[string]any, *PartResult) {
	data, err := env.ReadFile(rf.file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		res := Undecided(Finding{File: rf.file, Message: fmt.Sprintf("cannot be read: %v", err), Fix: "make the file readable"})
		return nil, &res
	}
	if rf.jwcc {
		std, err := hujson.Standardize(data)
		if err != nil {
			res := Undecided(Finding{
				File:    rf.file,
				Message: fmt.Sprintf("cannot be read as JSON with comments and trailing commas (%v); the audit does not read other JSON5", err),
				Fix:     "write it without JSON5-only syntax such as unquoted keys or single quotes",
			})
			return nil, &res
		}
		data = std
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil || cfg == nil {
		if err == nil {
			err = errors.New("the top level must be a JSON object")
		}
		res := Fail(ParseFailure(rf.file, err))
		return nil, &res
	}
	return cfg, nil
}

// judgeRenovate decides coverage and automerge for a Renovate
// configuration. Coverage holds unless enabledManagers leaves out a
// component's manager or github-actions; on GitLab CI, gitlabci, and
// gitlabci-include when the pipeline has project: includes (the ones
// that manager updates). Automerge is any automerge:
// true, or an extends preset whose name contains "automerge", except
// Renovate's built-in presets that do not enable it (renovateNotAutomerge).
// Other presets are not resolved.
func judgeRenovate(env *Env, b *updateBot, cfg map[string]any) {
	if raw, ok := cfg["enabledManagers"].([]any); ok {
		var enabled []string
		for _, m := range raw {
			if s, ok := m.(string); ok {
				enabled = append(enabled, s)
			}
		}
		for i, c := range env.Config.Components {
			managers := renovateManagers[c.Language]
			if !slices.ContainsFunc(managers, func(m string) bool { return slices.Contains(enabled, m) }) {
				b.Uncovered[i] = Finding{
					File:    b.File,
					Message: fmt.Sprintf("enabledManagers leaves out %s, which component %s needs", joinWords(managers, "or"), c.Path),
					Fix:     fmt.Sprintf("add %s to enabledManagers", managers[0]),
				}
			}
		}
		if ci, gitlab := onGitLab(env); !gitlab {
			if !slices.Contains(enabled, githubActions) {
				b.CI = append(b.CI, Finding{
					File:    b.File,
					Message: "enabledManagers leaves out github-actions",
					Fix:     "add github-actions to enabledManagers",
				})
			}
		} else {
			if !slices.Contains(enabled, gitlabCIManager) {
				b.CI = append(b.CI, Finding{
					File:    b.File,
					Message: "enabledManagers leaves out gitlabci, which updates the images and components of " + gitlabCIFile,
					Fix:     "add gitlabci to enabledManagers",
				})
			}
			i := slices.IndexFunc(ci.Includes, func(in CIInclude) bool { return in.Kind == "project" })
			if i >= 0 && !slices.Contains(enabled, gitlabIncludeManager) {
				in := ci.Includes[i]
				b.CI = append(b.CI, Finding{
					File: b.File,
					Message: fmt.Sprintf("enabledManagers leaves out gitlabci-include, which updates includes such as the %s include at %s line %d",
						in.Kind, in.File, in.Line),
					Fix: "add gitlabci-include to enabledManagers",
				})
			}
		}
	}
	walkRenovate(cfg, "", func(at, why string) {
		b.Automerge = append(b.Automerge, Finding{
			File:    b.File,
			Message: why + " (" + at + ")",
			Fix:     "remove the automerge setting, so that dependency updates are reviewed before they merge",
		})
	})
}

// renovateNotAutomerge lists Renovate's built-in presets whose names
// contain "automerge" but that do not enable it: :automergeDisabled sets
// automerge to false, and the others only choose how automerge works.
// ":x" is short for "default:x". A third-party preset of the same name is
// not one of these and still counts.
var renovateNotAutomerge = map[string]bool{
	":automergeDisabled":                      true,
	"default:automergeDisabled":               true,
	":automergeBranch":                        true,
	"default:automergeBranch":                 true,
	":automergePr":                            true,
	"default:automergePr":                     true,
	":automergeRequireAllStatusChecks":        true,
	"default:automergeRequireAllStatusChecks": true,
}

// walkRenovate calls found for every place in a Renovate configuration
// that enables automerge, in a stable order.
func walkRenovate(v any, at string, found func(at, why string)) {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			sub := k
			if at != "" {
				sub = at + "." + k
			}
			switch {
			case k == "automerge" && x[k] == true:
				found(sub, "enables automerge")
			case k == "extends":
				presets, _ := x[k].([]any)
				for i, p := range presets {
					if s, ok := p.(string); ok && strings.Contains(strings.ToLower(s), "automerge") && !renovateNotAutomerge[s] {
						found(fmt.Sprintf("%s[%d]", sub, i), fmt.Sprintf("extends the automerge preset %q", s))
					}
				}
			default:
				walkRenovate(x[k], sub, found)
			}
		}
	case []any:
		for i, e := range x {
			walkRenovate(e, fmt.Sprintf("%s[%d]", at, i), found)
		}
	}
}
