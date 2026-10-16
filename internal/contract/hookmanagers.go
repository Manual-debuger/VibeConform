package contract

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// Local Git hooks as the H rules see them (spec 0052, "Recognition
// rules", Hook managers). Only tracked files count. Three managers are
// recognized: lefthook, pre-commit and husky.

// The Git hooks the H rules read.
const (
	hookPreCommit = "pre-commit"
	hookPrePush   = "pre-push"
)

var gitHooks = []string{hookPreCommit, hookPrePush}

// hookManager is one recognized hook manager's committed configuration.
type hookManager struct {
	// Name is lefthook, pre-commit or husky.
	Name string
	// File is the configuration file findings name: lefthook.yml,
	// .pre-commit-config.yaml, or the first tracked file under .husky/.
	File string
	// Commands holds every simple command each Git hook runs.
	Commands map[string][]Command
	// HookFile names the file to change for a hook, when it differs from
	// File (husky keeps one script per hook).
	HookFile map[string]string
	// Broken holds a parse failure that makes the configuration
	// unusable: H-01 and H-02 fail with it.
	Broken []Finding
	// ScriptBroken holds parse failures of hook scripts. They fail H-02
	// only: the configuration installs, the hook does not run.
	ScriptBroken []Finding
}

// fileFor returns the file to change for a Git hook.
func (m *hookManager) fileFor(hook string) string {
	if f := m.HookFile[hook]; f != "" {
		return f
	}
	return m.File
}

// hookManagers returns the repository's recognized hook managers, in the
// order lefthook, pre-commit, husky.
func (e *Env) hookManagers() []*hookManager {
	var out []*hookManager
	if m := e.lefthook(); m != nil {
		out = append(out, m)
	}
	if m := e.preCommit(); m != nil {
		out = append(out, m)
	}
	if m := e.husky(); m != nil {
		out = append(out, m)
	}
	return out
}

// noHookManager is the finding for a repository with no recognized hook
// manager.
func noHookManager() Finding {
	return Finding{
		File:    "lefthook.yml",
		Message: "no recognized hook manager (lefthook, pre-commit, husky)",
		Fix: "commit the configuration of lefthook (lefthook.yml), pre-commit (.pre-commit-config.yaml) " +
			"or husky (.husky/)",
	}
}

// lefthookFiles are lefthook's main configuration files, in the order it
// looks for them.
var lefthookFiles = []string{"lefthook.yml", ".lefthook.yml"}

// lefthookLocal reports whether a file is lefthook's personal overlay,
// which the audit never reads.
func lefthookLocal(file string) bool {
	switch path.Base(file) {
	case "lefthook-local.yml", ".lefthook-local.yml":
		return true
	}
	return false
}

// lefthookHook is one Git hook of a lefthook configuration, merged across
// the main file and the files it extends.
type lefthookHook struct {
	// named holds commands and scripts by name: a later file replaces an
	// earlier one's entry of the same name.
	named map[string][]Command
	order []string
	// jobs are the commands of jobs: lists, appended per file.
	jobs []Command
}

func (h *lefthookHook) set(name string, cmds []Command) {
	if _, ok := h.named[name]; !ok {
		h.order = append(h.order, name)
	}
	h.named[name] = cmds
}

func (h *lefthookHook) commands() []Command {
	var out []Command
	for _, n := range h.order {
		out = append(out, h.named[n]...)
	}
	return append(out, h.jobs...)
}

// lefthook reads lefthook.yml (or .lefthook.yml) and the tracked files
// its extends names. Later files override earlier ones by command name,
// as lefthook merges them. lefthook-local.yml is personal and not read.
func (e *Env) lefthook() *hookManager {
	var main string
	for _, f := range lefthookFiles {
		if e.Repo.HasFile(f) {
			main = f
			break
		}
	}
	if main == "" {
		return nil
	}
	m := &hookManager{Name: "lefthook", File: main, Commands: map[string][]Command{}}
	hooks := map[string]*lefthookHook{}
	for _, h := range gitHooks {
		hooks[h] = &lefthookHook{named: map[string][]Command{}}
	}

	root, err := e.readYAMLMap(main)
	if err != nil {
		m.Broken = append(m.Broken, ParseFailure(main, err))
		return m
	}
	files := []*yaml.Node{root}
	if ext := mapValue(root, "extends"); ext != nil {
		for _, name := range stringList(ext) {
			name = strings.TrimPrefix(path.Clean(name), "./")
			if lefthookLocal(name) || !e.Repo.HasFile(name) {
				continue
			}
			n, err := e.readYAMLMap(name)
			if err != nil {
				m.Broken = append(m.Broken, ParseFailure(name, err))
				continue
			}
			files = append(files, n)
		}
	}
	if len(m.Broken) > 0 {
		return m
	}
	for _, f := range files {
		for _, h := range gitHooks {
			e.addLefthookHook(m, hooks[h], h, mapValue(f, h))
		}
	}
	for _, h := range gitHooks {
		m.Commands[h] = hooks[h].commands()
	}
	return m
}

func (e *Env) addLefthookHook(m *hookManager, into *lefthookHook, hook string, n *yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	if cmds := mapValue(n, "commands"); cmds != nil && cmds.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(cmds.Content); i += 2 {
			into.set("command "+cmds.Content[i].Value, runCommands(mapValue(resolve(cmds.Content[i+1]), "run")))
		}
	}
	if scripts := mapValue(n, "scripts"); scripts != nil && scripts.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(scripts.Content); i += 2 {
			name := scripts.Content[i].Value
			into.set("script "+name, e.lefthookScript(m, hook, name, resolve(scripts.Content[i+1])))
		}
	}
	into.jobs = append(into.jobs, e.lefthookJobs(m, hook, mapValue(n, "jobs"))...)
}

// lefthookJobs returns the commands of a jobs: list, following nested
// groups.
func (e *Env) lefthookJobs(m *hookManager, hook string, jobs *yaml.Node) []Command {
	if jobs == nil || jobs.Kind != yaml.SequenceNode {
		return nil
	}
	var out []Command
	for _, j := range jobs.Content {
		j = resolve(j)
		if j.Kind != yaml.MappingNode {
			continue
		}
		out = append(out, runCommands(mapValue(j, "run"))...)
		if s := mapValue(j, "script"); s != nil && s.Kind == yaml.ScalarNode {
			out = append(out, e.lefthookScript(m, hook, s.Value, j)...)
		}
		if g := mapValue(j, "group"); g != nil {
			out = append(out, e.lefthookJobs(m, hook, mapValue(g, "jobs"))...)
		}
	}
	return out
}

// shellRunners are the runners whose lefthook scripts are read as POSIX
// shell.
var shellRunners = map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true}

// lefthookScript returns the commands of a lefthook script: the tracked
// file .lefthook/<hook>/<name>, read as shell when its runner is a shell.
// A script of another runner, or one that is not tracked, calls nothing.
func (e *Env) lefthookScript(m *hookManager, hook, name string, entry *yaml.Node) []Command {
	if r := mapValue(entry, "runner"); r != nil && r.Kind == yaml.ScalarNode {
		if words := strings.Fields(r.Value); len(words) > 0 && !shellRunners[path.Base(words[0])] {
			return nil
		}
	}
	file := path.Join(".lefthook", hook, name)
	data, err := e.ReadFile(file)
	if err != nil {
		return nil
	}
	cmds, err := ParseCommands(string(data))
	if err != nil {
		m.ScriptBroken = append(m.ScriptBroken, ParseFailure(file, err))
		return nil
	}
	return cmds
}

// runCommands parses a run: value. A run that does not parse as shell
// calls nothing.
func runCommands(n *yaml.Node) []Command {
	if n == nil || n.Kind != yaml.ScalarNode {
		return nil
	}
	cmds, err := ParseCommands(n.Value)
	if err != nil {
		return nil
	}
	return cmds
}

const preCommitFile = ".pre-commit-config.yaml"

// preCommit reads .pre-commit-config.yaml: the hooks of repo: local, each
// a command of its entry's words and its args, in the stages it declares,
// or default_stages, or pre-commit. Legacy stage names commit and push
// count as pre-commit and pre-push.
func (e *Env) preCommit() *hookManager {
	if !e.Repo.HasFile(preCommitFile) {
		return nil
	}
	m := &hookManager{Name: "pre-commit", File: preCommitFile, Commands: map[string][]Command{}}
	root, err := e.readYAMLMap(preCommitFile)
	if err == nil {
		err = checkPreCommitShape(root)
	}
	if err != nil {
		m.Broken = append(m.Broken, ParseFailure(preCommitFile, err))
		return m
	}
	defaults := []string{hookPreCommit}
	if d := mapValue(root, "default_stages"); d != nil {
		defaults = stringList(d)
	}
	for _, repo := range mapValue(root, "repos").Content {
		repo = resolve(repo)
		if r := mapValue(repo, "repo"); r == nil || r.Value != "local" {
			continue
		}
		hooks := mapValue(repo, "hooks")
		if hooks == nil {
			continue
		}
		for _, h := range hooks.Content {
			h = resolve(h)
			cmd, ok := preCommitCommand(h)
			if !ok {
				continue
			}
			stages := defaults
			if s := mapValue(h, "stages"); s != nil {
				stages = stringList(s)
			}
			for _, st := range stages {
				switch st {
				case "commit":
					st = hookPreCommit
				case "push":
					st = hookPrePush
				}
				m.Commands[st] = append(m.Commands[st], cmd)
			}
		}
	}
	return m
}

func checkPreCommitShape(root *yaml.Node) error {
	repos := mapValue(root, "repos")
	if repos == nil || repos.Kind != yaml.SequenceNode {
		return errors.New("repos must be a list")
	}
	for i, r := range repos.Content {
		r = resolve(r)
		if r.Kind != yaml.MappingNode {
			return lineError(r, fmt.Sprintf("repos[%d] must be a map", i))
		}
		if h := mapValue(r, "hooks"); h != nil && h.Kind != yaml.SequenceNode {
			return lineError(h, fmt.Sprintf("repos[%d].hooks must be a list", i))
		}
	}
	return nil
}

// preCommitCommand is a local hook's command: its entry's words followed
// by its args. pre-commit splits entry into words and runs no shell, so
// an entry that is not one simple command calls nothing.
func preCommitCommand(h *yaml.Node) (Command, bool) {
	entry := mapValue(h, "entry")
	if entry == nil || entry.Kind != yaml.ScalarNode {
		return Command{}, false
	}
	cmd, err := ParseSimpleCommand(entry.Value)
	if err != nil {
		return Command{}, false
	}
	if args := mapValue(h, "args"); args != nil {
		for _, a := range stringList(args) {
			cmd.Words = append(cmd.Words, Word{Text: a, Literal: true})
		}
	}
	return cmd, true
}

const huskyDir = ".husky"

// husky recognizes any tracked file under .husky/ and reads the
// pre-commit and pre-push scripts as shell.
func (e *Env) husky() *hookManager {
	files := e.Repo.FilesUnder(huskyDir)
	if len(files) == 0 {
		return nil
	}
	m := &hookManager{Name: "husky", File: files[0], Commands: map[string][]Command{}, HookFile: map[string]string{}}
	for _, h := range gitHooks {
		file := huskyDir + "/" + h
		m.HookFile[h] = file
		data, err := e.ReadFile(file)
		if err != nil {
			continue
		}
		cmds, err := ParseCommands(string(data))
		if err != nil {
			m.ScriptBroken = append(m.ScriptBroken, ParseFailure(file, err))
			continue
		}
		m.Commands[h] = cmds
	}
	return m
}

// readYAMLMap reads a tracked YAML file whose top level must be a map.
// An empty file is an empty map.
func (e *Env) readYAMLMap(file string) (*yaml.Node, error) {
	data, err := e.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode}, nil
	}
	root := resolve(doc.Content[0])
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("the top level must be a map")
	}
	return root, nil
}

// stringList reads a YAML string or list of strings.
func stringList(n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			if c = resolve(c); c.Kind == yaml.ScalarNode {
				out = append(out, c.Value)
			}
		}
		return out
	}
	return nil
}
