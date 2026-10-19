package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/guard"
	"github.com/Manual-debuger/VibeConform/internal/guardrule"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// checkSkill is the drift checker (spec 0054, "The drift test (#109)"):
// it reads a setup skill as a file system and returns every problem it
// finds, sorted, each naming the skill file it is in.
//
// It lives in the cli package's tests because it checks vibe against the
// real command tree, which only this package builds, and loads the
// reference vibe.yaml files through contract.Parse, which needs a Git
// work tree. internal/setupskill may import only internal/guard, and a
// test here adds nothing to the vibe binary or the package graph.
func checkSkill(skill fs.FS) []string {
	c := &skillChecker{skill: skill}
	_ = fs.WalkDir(skill, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			c.problem(p, "cannot be read: %v", err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(skill, p)
		if err != nil {
			c.problem(p, "cannot be read: %v", err)
			return nil
		}
		c.files = append(c.files, skillFile{path: p, text: string(data)})
		return nil
	})
	for _, f := range c.files {
		c.checkRuleIDs(f)
		c.checkSkillPaths(f)
		if strings.HasSuffix(f.path, ".md") {
			c.checkMarkdown(f)
		} else {
			c.checkConfigCommands(f)
		}
	}
	c.checkRulesGuidance()
	c.checkReferenceConfigs()
	c.checkReferencePolicies()
	c.checkReferenceHooks()
	c.checkUpgrade()
	sort.Strings(c.problems)
	return c.problems
}

type skillFile struct {
	path, text string
}

type skillChecker struct {
	skill    fs.FS
	files    []skillFile
	problems []string
}

func (c *skillChecker) problem(file, format string, args ...any) {
	c.problems = append(c.problems, file+": "+fmt.Sprintf(format, args...))
}

func (c *skillChecker) file(path string) (skillFile, bool) {
	for _, f := range c.files {
		if f.path == path {
			return f, true
		}
	}
	return skillFile{}, false
}

// ruleIDPattern is a contract rule ID's form (spec 0051).
var ruleIDPattern = regexp.MustCompile(`\b[VHCADR]-[0-9]{2}\b`)

// checkRuleIDs: every rule ID the file names is in the registry of the
// contract vibe implements.
func (c *skillChecker) checkRuleIDs(f skillFile) {
	for _, id := range uniqueMatches(ruleIDPattern, f.text) {
		if _, ok := contract.LookupRule(id); !ok {
			c.problem(f.path, "names rule %s, which contract %s does not have", id, contract.Implemented)
		}
	}
}

func uniqueMatches(re *regexp.Regexp, s string) []string {
	var out []string
	for _, m := range re.FindAllString(s, -1) {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// skillPathPattern is a path into the skill: reference/… not preceded by
// a path or word character.
var skillPathPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])(reference/[A-Za-z0-9_./<>-]*)`)

// referenceStacks are the values of <stack> in a skill path.
var referenceStacks = []string{"go", "python", "typescript", "monorepo"}

// checkSkillPaths: every path into the skill exists, for every stack when
// it names <stack>. A path ending in / is a directory.
func (c *skillChecker) checkSkillPaths(f skillFile) {
	seen := map[string]bool{}
	for _, m := range skillPathPattern.FindAllStringSubmatch(f.text, -1) {
		p := strings.TrimRight(m[1], ".,:")
		if seen[p] {
			continue
		}
		seen[p] = true
		paths := []string{p}
		if strings.Contains(p, "<stack>") {
			paths = nil
			for _, s := range referenceStacks {
				paths = append(paths, strings.ReplaceAll(p, "<stack>", s))
			}
		}
		for _, q := range paths {
			if strings.ContainsAny(q, "<>") {
				c.problem(f.path, "names %s, whose placeholder is not <stack>", p)
				break
			}
			info, err := fs.Stat(c.skill, strings.TrimSuffix(q, "/"))
			switch {
			case err != nil:
				c.problem(f.path, "names %s, which the skill does not have", q)
			case strings.HasSuffix(q, "/") && !info.IsDir():
				c.problem(f.path, "names %s as a directory, but it is a file", q)
			}
		}
	}
}

// shellLanguages are the fenced code block languages read as commands.
// Another language, such as text, is output and is not checked.
var shellLanguages = []string{"", "sh", "bash", "shell", "console", "powershell", "pwsh"}

// codeSpanPattern is a Markdown code span.
var codeSpanPattern = regexp.MustCompile("`([^`]+)`")

// guardIDPattern is the form of a guard rule ID.
var guardIDPattern = regexp.MustCompile(`^[a-z]+(-[a-z]+)+$`)

// checkMarkdown checks the commands in a Markdown file's code spans and
// shell code blocks, the guard rule IDs of every paragraph about guard
// rule IDs, and the repository files its code spans name.
func (c *skillChecker) checkMarkdown(f skillFile) {
	var paragraph []string
	flush := func() {
		text := strings.Join(paragraph, " ")
		paragraph = nil
		spans := codeSpanPattern.FindAllStringSubmatch(text, -1)
		aboutGuardIDs := strings.Contains(text, "guard rule ID")
		for _, s := range spans {
			span := s[1]
			c.checkCommands(f.path, span)
			c.checkRepoFile(f.path, span)
			if aboutGuardIDs && guardIDPattern.MatchString(span) && !guardrule.IsContractID(span) {
				c.problem(f.path, "names guard rule ID %q, which is not one of contract %s's ten", span, contract.Implemented)
			}
		}
	}
	lines := strings.Split(f.text, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if lang, ok := strings.CutPrefix(line, "```"); ok {
			flush()
			var block []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				block = append(block, lines[i])
			}
			if slices.Contains(shellLanguages, strings.TrimSpace(lang)) {
				for _, l := range block {
					c.checkCommands(f.path, strings.TrimPrefix(strings.TrimSpace(l), "$ "))
				}
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			flush()
			if line != "" {
				paragraph = append(paragraph, line)
				flush()
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	flush()
}

// configCommandPattern is a command in a reference configuration file: a
// workflow's or Taskfile's run: or cmd: value, a Taskfile's list item, or
// a Claude Code hook's "command".
var configCommandPattern = regexp.MustCompile(`(?m)^\s*(?:-\s+)?(?:run|cmd):\s*(.+)$|"command":\s*"((?:[^"\\]|\\.)*)"`)

// checkConfigCommands checks the commands of a reference file that is not
// Markdown.
func (c *skillChecker) checkConfigCommands(f skillFile) {
	for _, m := range configCommandPattern.FindAllStringSubmatch(f.text, -1) {
		cmd := m[1]
		if m[2] != "" {
			if s, err := strconv.Unquote(`"` + m[2] + `"`); err == nil {
				cmd = s
			}
		}
		c.checkCommands(f.path, cmd)
	}
}

// commandSeparators split a command line into simple commands.
var commandSeparators = regexp.MustCompile(`&&|\|\||[;|{}()\n]`)

// checkCommands checks every simple command of text that runs vibe or
// vibe-guard.
func (c *skillChecker) checkCommands(file, text string) {
	for _, simple := range commandSeparators.Split(text, -1) {
		words := strings.Fields(simple)
		if len(words) == 0 {
			continue
		}
		args := shellArgs(words[1:])
		switch words[0] {
		case "vibe":
			if msg := checkVibeArgs(args); msg != "" {
				c.problem(file, "`%s`: %s", strings.TrimSpace(simple), msg)
			}
		case "vibe-guard":
			if msg := checkVibeGuardArgs(args); msg != "" {
				c.problem(file, "`%s`: %s", strings.TrimSpace(simple), msg)
			}
		}
	}
}

// shellArgs is words up to the first redirection, unquoted.
func shellArgs(words []string) []string {
	var out []string
	for _, w := range words {
		if redirection.MatchString(w) {
			break
		}
		out = append(out, strings.Trim(w, `'"`))
	}
	return out
}

// redirection is a redirection word: >file, 2>&1, </dev/null. A
// placeholder such as <ID> is not one.
var redirection = regexp.MustCompile(`^[0-9]*>|^<[^>]*$`)

var (
	vibeRootOnce sync.Once
	vibeRoot     *cobra.Command
)

// vibeCommandTree is the real vibe command tree, with cobra's default
// help and version flags and help command added as Execute adds them.
func vibeCommandTree() *cobra.Command {
	vibeRootOnce.Do(func() {
		vibeRoot = NewRootCmd("test")
		vibeRoot.InitDefaultHelpCmd()
		vibeRoot.InitDefaultVersionFlag()
		var walk func(*cobra.Command)
		walk = func(cmd *cobra.Command) {
			cmd.InitDefaultHelpFlag()
			for _, sub := range cmd.Commands() {
				walk(sub)
			}
		}
		walk(vibeRoot)
	})
	return vibeRoot
}

// checkVibeArgs checks vibe's arguments against the command tree: each
// word before the first positional argument names a subcommand, and
// every flag is one of the command's. It returns "" when they are valid.
func checkVibeArgs(args []string) string {
	cmd := vibeCommandTree()
	positional := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
			var flag = cmd.Flag(name)
			if !strings.HasPrefix(a, "--") {
				flag = cmd.Flags().ShorthandLookup(name)
				if flag == nil {
					flag = cmd.InheritedFlags().ShorthandLookup(name)
				}
			}
			if flag == nil {
				return fmt.Sprintf("%s has no flag %s", cmd.CommandPath(), a)
			}
			if !hasValue && flag.NoOptDefVal == "" {
				i++ // the flag's value
			}
			continue
		}
		if positional {
			continue
		}
		if sub, _, err := cmd.Find([]string{a}); err == nil && sub != cmd {
			cmd = sub
			continue
		}
		if cmd.HasSubCommands() && !cmd.Runnable() || cmd == vibeCommandTree() {
			return fmt.Sprintf("%q is not a command of %s", a, cmd.CommandPath())
		}
		positional = true
	}
	return ""
}

var (
	guardCommandsOnce sync.Once
	guardCommands     map[string]map[string]bool // command -> flag -> takes a value
)

// vibeGuardCommands is vibe-guard's real command set, read from the
// usage its help command prints.
func vibeGuardCommands() map[string]map[string]bool {
	guardCommandsOnce.Do(func() {
		var out bytes.Buffer
		guard.Main([]string{"help"}, guard.Env{
			Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out,
			Getenv: func(string) string { return "" }, Now: time.Now,
		}, "test")
		guardCommands = map[string]map[string]bool{}
		for _, line := range strings.Split(out.String(), "\n") {
			words := strings.Fields(line)
			if len(words) < 2 || words[0] != "vibe-guard" {
				continue
			}
			flags := map[string]bool{}
			for i := 2; i < len(words); i++ {
				w := strings.Trim(words[i], "[]")
				if strings.HasPrefix(w, "--") {
					flags[w] = i+1 < len(words) && strings.HasPrefix(words[i+1], "<")
				}
			}
			guardCommands[words[1]] = flags
		}
		// The aliases guard.Main accepts beside its usage.
		guardCommands["--version"] = map[string]bool{}
		guardCommands["help"] = map[string]bool{}
	})
	return guardCommands
}

// checkVibeGuardArgs checks vibe-guard's arguments against its command
// set. It returns "" when they are valid.
func checkVibeGuardArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	flags, ok := vibeGuardCommands()[args[0]]
	if !ok {
		return fmt.Sprintf("%q is not a command of vibe-guard", args[0])
	}
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, _, hasValue := strings.Cut(a, "=")
		takesValue, ok := flags[name]
		if !ok {
			return fmt.Sprintf("vibe-guard %s has no flag %s", args[0], name)
		}
		if takesValue && !hasValue {
			i++
		}
	}
	return ""
}

// repoFileExtensions are the extensions of the names a code span is read
// as a repository file by.
var repoFileExtensions = []string{".yml", ".yaml", ".json", ".toml", ".lock", ".sum", ".mod", ".md", ".js", ".cfg", ".ini"}

// repoFilePattern is a code span that names one repository file or
// directory: a relative path with no spaces, globs or placeholders.
var repoFilePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*/?$`)

// removedOrAvoided are the repository files the skill names although the
// contract does not read them and no reference implementation creates
// them: the template-sync system's files, which the migration removes or
// merges into one file (spec 0054, "The setup skill", Migrate), and the
// secret files the skill must not read. #112 removes the template-sync
// system, and its names leave this list with migrate.md.
var removedOrAvoided = []string{
	".vibe/", "Taskfile.vibe.yml", "Taskfile.local.yml", "lefthook.local.yml",
	".gitlab-ci.vibe.yml", ".gitlab-ci.local.yml", ".gitlab-ci.defaults.yml",
	".github/workflows/conformance.yml",
	"eslint.base.config.js", "tsconfig.base.json", "ruff.base.toml",
	".claude/skills/vibeconform/", ".agents/skills/vibeconform/",
	".env",
}

// checkRepoFile: a code span naming a repository file names a file the
// contract reads or a file a reference implementation creates. A bare
// name, such as ci.yml, may name a file in any directory. The optional
// content (reference/optional/) is outside the contract: each file there
// is the reference implementation of the files it names.
func (c *skillChecker) checkRepoFile(file, span string) {
	if strings.HasPrefix(file, "reference/optional/") || !repoFilePattern.MatchString(span) ||
		strings.HasPrefix(span, "reference/") || span == "SKILL.md" {
		return
	}
	isDir := strings.HasSuffix(span, "/")
	if !isDir && !slices.Contains(repoFileExtensions, pathpkg.Ext(span)) {
		return
	}
	if slices.Contains(removedOrAvoided, span) {
		return
	}
	name := strings.TrimSuffix(span, "/")
	for _, known := range knownRepoFiles() {
		if known == name || strings.HasPrefix(known, name+"/") ||
			!strings.Contains(name, "/") && strings.HasSuffix(known, "/"+name) {
			return
		}
	}
	c.problem(file, "names %s, which the contract does not read and no reference implementation creates", span)
}

var (
	knownRepoFilesOnce sync.Once
	knownRepoFilesList []string
)

// knownRepoFiles are the repository paths the contract reads (the string
// literals of internal/contract and internal/guardrule) and the paths
// the reference implementations and vibe init --skill create.
func knownRepoFiles() []string {
	knownRepoFilesOnce.Do(func() {
		for _, dir := range []string{"../contract", "../guardrule"} {
			knownRepoFilesList = append(knownRepoFilesList, goStringLiterals(dir)...)
		}
		for _, stack := range referenceStacks {
			for _, ref := range referenceFiles(setupskill.FS()) {
				if p, ok := referenceTarget(stack, ref); ok {
					knownRepoFilesList = append(knownRepoFilesList, p)
				}
			}
		}
		knownRepoFilesList = append(knownRepoFilesList, setupskill.Dirs...)
		for i, p := range knownRepoFilesList {
			knownRepoFilesList[i] = strings.TrimSuffix(p, "/")
		}
	})
	return knownRepoFilesList
}

// goStringLiterals returns the string literals of the non-test Go files in
// dir.
func goStringLiterals(dir string) []string {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		panic(err)
	}
	var out []string
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			panic(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					out = append(out, s)
				}
			}
			return true
		})
	}
	return out
}

// referenceFiles lists the files under reference/ in skill.
func referenceFiles(skill fs.FS) []string {
	var out []string
	_ = fs.WalkDir(skill, "reference", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// monorepoComponents are the components of reference/vibe-yaml/monorepo.yaml,
// by language.
var monorepoComponents = map[string]string{
	"go":         "services/api",
	"typescript": "apps/web",
	"python":     "services/worker",
}

// referenceTarget is where a repository of stack places the reference
// file ref (a skill path), or false when the stack does not use it.
func referenceTarget(stack, ref string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(ref, "reference/"), "/")
	switch {
	case len(parts) == 2 && parts[0] == "vibe-yaml":
		return "vibe.yaml", parts[1] == stack+".yaml"
	case len(parts) == 2 && parts[0] == "agents":
		switch parts[1] {
		case "settings.json":
			return ".claude/settings.json", true
		case "policy.json":
			return guardrule.PolicyFile, true
		}
		return parts[1], true
	case len(parts) == 2 && parts[0] == "ci":
		return ".github/" + parts[1], true
	case len(parts) == 3 && parts[0] == "ci" && parts[1] == stack:
		if parts[2] == "dependabot.yml" {
			return ".github/dependabot.yml", true
		}
		return ".github/workflows/" + parts[2], true
	case len(parts) == 3 && parts[0] == "hooks" && parts[1] == stack:
		return parts[2], true
	case len(parts) == 3 && parts[0] == "taskrunner" && parts[1] == stack:
		if lang, ok := strings.CutSuffix(parts[2], ".Taskfile.yml"); ok && stack == "monorepo" {
			dir, ok := monorepoComponents[lang]
			return dir + "/Taskfile.yml", ok
		}
		return parts[2], true
	case len(parts) == 3 && parts[0] == "lint":
		if stack == "monorepo" {
			dir, ok := monorepoComponents[parts[1]]
			return dir + "/" + parts[2], ok
		}
		return parts[2], parts[1] == stack
	}
	return "", false
}

// checkRulesGuidance: reference/rules.md has a heading for every rule of
// the registry, MUST and SHOULD, with the rule's level.
func (c *skillChecker) checkRulesGuidance() {
	const file = "reference/rules.md"
	f, ok := c.file(file)
	if !ok {
		c.problem(file, "is missing, so no rule has guidance")
		return
	}
	for _, r := range contract.Rules() {
		want := fmt.Sprintf("### %s (%s)", r.ID, r.Level)
		if !slices.ContainsFunc(strings.Split(f.text, "\n"), func(l string) bool { return strings.HasPrefix(l, want) }) {
			c.problem(file, "has no %q heading, so rule %s has no guidance", want, r.ID)
		}
	}
}

// manifestByLanguage is the root manifest of a component of a language.
var manifestByLanguage = map[string]string{"go": "go.mod", "python": "pyproject.toml", "typescript": "package.json"}

// checkReferenceConfigs: every reference vibe.yaml loads as a valid
// version 2 configuration, in a Git work tree holding a manifest for its
// stack's root component, or for each component it declares.
func (c *skillChecker) checkReferenceConfigs() {
	for _, f := range c.files {
		if pathpkg.Dir(f.path) != "reference/vibe-yaml" {
			continue
		}
		if err := loadReferenceConfig(f); err != nil {
			c.problem(f.path, "does not load as a version 2 vibe.yaml: %v", err)
		}
	}
}

func loadReferenceConfig(f skillFile) error {
	data := []byte(f.text)
	schema, err := contract.DetectSchema("vibe.yaml", data)
	if err != nil {
		return err
	}
	if schema != contract.SchemaV2 {
		return fmt.Errorf("it is not version 2 (no contract:)")
	}
	var decl struct {
		Components []struct {
			Path     string `yaml:"path"`
			Language string `yaml:"language"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &decl); err != nil {
		return err
	}
	files := map[string]string{}
	if m, ok := manifestByLanguage[strings.TrimSuffix(pathpkg.Base(f.path), ".yaml")]; ok && len(decl.Components) == 0 {
		files[m] = ""
	}
	for _, comp := range decl.Components {
		files[pathpkg.Join(comp.Path, manifestByLanguage[comp.Language])] = ""
	}
	dir, err := os.MkdirTemp("", "vibe-skill-config-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := writeGitTree(dir, files); err != nil {
		return err
	}
	repo, err := contract.OpenRepo(context.Background(), dir)
	if err != nil {
		return err
	}
	_, err = contract.Parse("vibe.yaml", data, repo)
	return err
}

// writeGitTree writes files (slash paths) under dir and makes it a Git
// repository that tracks them all.
func writeGitTree(dir string, files map[string]string) error {
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"-c", "core.autocrlf=false", "add", "-A"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w\n%s", args, err, out)
		}
	}
	return nil
}

// checkReferencePolicies: every reference policy.json loads as a valid
// version 2 policy.
func (c *skillChecker) checkReferencePolicies() {
	for _, f := range c.files {
		if pathpkg.Base(f.path) != "policy.json" {
			continue
		}
		if _, err := guardrule.ParsePolicy([]byte(f.text)); err != nil {
			c.problem(f.path, "is not a valid version 2 guard policy: %v", err)
		}
	}
}

// checkReferenceHooks: the reference PreToolUse and Stop commands are the
// reference guard's, for the placeholder version.
func (c *skillChecker) checkReferenceHooks() {
	const file = "reference/agents/settings.json"
	f, ok := c.file(file)
	if !ok {
		c.problem(file, "is missing")
		return
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(f.text), &cfg); err != nil {
		c.problem(file, "is not JSON: %v", err)
		return
	}
	pre, stop := cfg.Hooks["PreToolUse"], cfg.Hooks["Stop"]
	if len(pre) != 1 || len(pre[0].Hooks) != 1 || pre[0].Matcher != guard.ReferencePreToolUseMatcher ||
		pre[0].Hooks[0].Command != guard.ReferencePreToolUse(setupskill.VersionPlaceholder) {
		c.problem(file, "PreToolUse is not the reference guard's matcher and command")
	}
	if len(stop) != 1 || len(stop[0].Hooks) != 1 || stop[0].Hooks[0].Command != guard.ReferenceStop {
		c.problem(file, "Stop is not the reference guard's command")
	}
}

// checkUpgrade: the upgrade flow asks a person to run vibe unlock before a
// MAJOR upgrade changes the guardrails (A-08).
func (c *skillChecker) checkUpgrade() {
	const file = "reference/upgrade.md"
	f, ok := c.file(file)
	if !ok || !strings.Contains(f.text, "`vibe unlock`") {
		c.problem(file, "does not name `vibe unlock`")
	}
}
