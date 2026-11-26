package setupskill

import (
	"slices"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// The placement manifest (spec 0073): where a repository built from the
// references places each reference file. The drift checker, the static
// audit of the references and the hooks test read it.

// The CI providers a reference repository uses (spec 0055): GitHub
// Actions with Dependabot, or GitLab CI with Renovate.
const (
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"
)

// StackMonorepo is the stack whose components are MonorepoComponents.
const StackMonorepo = "monorepo"

// Stacks returns the stacks a reference repository can have: the values
// of <stack> in a skill path. Each call returns a new slice.
func Stacks() []string { return []string{"go", "python", "typescript", StackMonorepo} }

// Providers returns the CI providers a reference repository can use.
// Each call returns a new slice.
func Providers() []string { return []string{ProviderGitHub, ProviderGitLab} }

// Component is one component of the reference monorepo
// (reference/vibe-yaml/monorepo.yaml).
type Component struct {
	// Language is the component's language, also the directory of its
	// lint references.
	Language string
	// Dir is the component's directory, relative to the root.
	Dir string
	// ID is the name the root reference Taskfile includes it under.
	ID string
}

// Taskfile is the reference file that the component's Taskfile.yml is
// built from.
func (c Component) Taskfile() string {
	return "reference/taskrunner/" + StackMonorepo + "/" + c.Language + ".Taskfile.yml"
}

// MonorepoComponents returns the components of the reference monorepo.
// Each call returns a new slice.
func MonorepoComponents() []Component {
	return []Component{
		{Language: "go", Dir: "services/api", ID: "api"},
		{Language: "typescript", Dir: "apps/web", ID: "web"},
		{Language: "python", Dir: "services/worker", ID: "worker"},
	}
}

// monorepoComponent is the component of the reference monorepo in
// language.
func monorepoComponent(language string) (Component, bool) {
	i := slices.IndexFunc(MonorepoComponents(), func(c Component) bool { return c.Language == language })
	if i < 0 {
		return Component{}, false
	}
	return MonorepoComponents()[i], true
}

// gitlabOnly are the reference CI files only a GitLab repository uses,
// at the root.
var gitlabOnly = []string{".gitlab-ci.yml", "renovate.json"}

// placer places a reference file of one directory of reference/: sub is
// the subdirectory ("" for a file directly in it) and name the file.
type placer func(stack, provider, sub, name string) (string, bool)

// placers are the placers by directory of reference/. A file in another
// directory, or directly in reference/, is used by no repository.
var placers = map[string]placer{
	"agents":     placeAgents,
	"ci":         placeCI,
	"hooks":      placeStackFile,
	"lint":       placeLint,
	"taskrunner": placeTaskfile,
	"vibe-yaml":  placeConfig,
}

// Place returns where a repository of stack on provider places the
// reference file ref, a slash path in the skill such as
// reference/ci/go/ci.yml, or false when that repository does not use it.
// A GitHub repository takes the workflows and dependabot.yml; a GitLab
// repository takes .gitlab-ci.yml, renovate.json and the pull request
// template as a merge request template.
func Place(stack, provider, ref string) (string, bool) {
	rest, ok := strings.CutPrefix(ref, "reference/")
	if !ok || !slices.Contains(Stacks(), stack) || !slices.Contains(Providers(), provider) {
		return "", false
	}
	parts := strings.Split(rest, "/")
	var sub string
	switch len(parts) {
	case 2:
	case 3:
		sub = parts[1]
	default:
		return "", false
	}
	place, ok := placers[parts[0]]
	if !ok {
		return "", false
	}
	if p, ok := place(stack, provider, sub, parts[len(parts)-1]); ok {
		return p, true
	}
	return "", false
}

func placeAgents(_, _, sub, name string) (string, bool) {
	switch {
	case sub != "":
		return "", false
	case name == "settings.json":
		return ".claude/settings.json", true
	case name == "policy.json":
		return guard.PolicyFile, true
	}
	return name, true
}

func placeConfig(stack, _, sub, name string) (string, bool) {
	return "vibe.yaml", sub == "" && name == stack+".yaml"
}

func placeStackFile(stack, _, sub, name string) (string, bool) {
	return name, sub == stack
}

func placeCI(stack, provider, sub, name string) (string, bool) {
	switch {
	case sub != "" && sub != stack:
		return "", false
	case slices.Contains(gitlabOnly, name):
		return name, provider == ProviderGitLab
	case sub == "" && name == "pull_request_template.md" && provider == ProviderGitLab:
		return ".gitlab/merge_request_templates/Default.md", true
	case provider != ProviderGitHub:
		return "", false
	case sub == "":
		return ".github/" + name, true
	case name == "dependabot.yml":
		return ".github/dependabot.yml", true
	}
	return ".github/workflows/" + name, true
}

func placeTaskfile(stack, _, sub, name string) (string, bool) {
	if sub != stack {
		return "", false
	}
	if lang, ok := strings.CutSuffix(name, ".Taskfile.yml"); ok && stack == StackMonorepo {
		c, ok := monorepoComponent(lang)
		return c.Dir + "/Taskfile.yml", ok
	}
	return name, true
}

// placeLint places a lint reference of the language sub: at the root of
// a repository of that stack, or in the monorepo component of that
// language.
func placeLint(stack, _, sub, name string) (string, bool) {
	if stack != StackMonorepo {
		return name, sub != "" && sub == stack
	}
	c, ok := monorepoComponent(sub)
	return c.Dir + "/" + name, ok
}
