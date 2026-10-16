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
//
// The H rules read the pre-commit and pre-push hooks. R-01 reads every
// Git hook the same model holds ("no hook command calls vibe"): a
// post-checkout hook that calls vibe breaks without vibe as much as a
// pre-push hook does. So the model reads every hook named in githooks(5),
// with the same files, merging and parsing for all of them.

// The Git hooks the H rules read.
const (
	hookPreCommit = "pre-commit"
	hookPrePush   = "pre-push"
)

var gitHooks = []string{hookPreCommit, hookPrePush}

// allGitHooks are the client and server hooks of githooks(5), in its
// order. A hook manager's entry under another name is not a Git hook.
var allGitHooks = []string{
	"applypatch-msg", "pre-applypatch", "post-applypatch", hookPreCommit, "pre-merge-commit",
	"prepare-commit-msg", "commit-msg", "post-commit", "pre-rebase", "post-checkout", "post-merge",
	hookPrePush, "pre-receive", "update", "proc-receive", "post-receive", "post-update",
	"reference-transaction", "push-to-checkout", "pre-auto-gc", "post-rewrite", "sendemail-validate",
	"fsmonitor-watchman", "p4-changelist", "p4-prepare-changelist", "p4-post-changelist", "p4-pre-submit",
	"post-index-change",
}

// hookManager is one recognized hook manager's committed configuration.
type hookManager struct {
	// Name is lefthook, pre-commit or husky.
	Name string
	// File is the configuration file findings name: lefthook.yml,
	// .pre-commit-config.yaml, or the first tracked file under .husky/.
	File string
	// Entries holds what each Git hook runs, by hook (pre-commit holds its
	// stages, manual included, under their names).
	Entries map[string][]hookEntry
	// HookFile names the file to change for a hook, when it differs from
	// File (husky keeps one script per hook).
	HookFile map[string]string
	// Broken holds a parse failure that makes the configuration
	// unusable: H-01 and H-02 fail with it.
	Broken []Finding
	// ScriptBroken holds parse failures of hook scripts, by hook. Those of
	// pre-commit and pre-push fail H-02 only: the configuration installs,
	// the hook does not run.
	ScriptBroken map[string][]Finding
}

// hookEntry is one command, script or job a Git hook runs.
type hookEntry struct {
	// File holds the entry: the configuration file it is written in, or
	// the script file it runs.
	File string
	// Where locates it in File, such as "line 4: pre-commit.commands.fmt".
	Where string
	// Commands are its simple commands.
	Commands []Command
}

// commands returns every simple command a Git hook runs.
func (m *hookManager) commands(hook string) []Command {
	var out []Command
	for _, en := range m.Entries[hook] {
		out = append(out, en.Commands...)
	}
	return out
}

func (m *hookManager) addEntry(hook string, en hookEntry) {
	m.Entries[hook] = append(m.Entries[hook], en)
}

func (m *hookManager) scriptBroken(hook string, f Finding) {
	if m.ScriptBroken == nil {
		m.ScriptBroken = map[string][]Finding{}
	}
	m.ScriptBroken[hook] = append(m.ScriptBroken[hook], f)
}

// fileFor returns the file to change for a Git hook.
func (m *hookManager) fileFor(hook string) string {
	if f := m.HookFile[hook]; f != "" {
		return f
	}
	return m.File
}

// hookManagers returns the repository's recognized hook managers, in the
// order lefthook, pre-commit, husky. They are read once per audit.
func (e *Env) hookManagers() []*hookManager {
	if e.hookMgrs == nil {
		ms := e.readHookManagers()
		e.hookMgrs = &ms
	}
	return *e.hookMgrs
}

func (e *Env) readHookManagers() []*hookManager {
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
	named map[string][]hookEntry
	order []string
	// jobs are the entries of jobs: lists, appended per file.
	jobs []hookEntry
}

func (h *lefthookHook) set(name string, entries []hookEntry) {
	if _, ok := h.named[name]; !ok {
		h.order = append(h.order, name)
	}
	h.named[name] = entries
}

func (h *lefthookHook) entries() []hookEntry {
	var out []hookEntry
	for _, n := range h.order {
		out = append(out, h.named[n]...)
	}
	return append(out, h.jobs...)
}

// lefthookFile is one lefthook configuration file, read.
type lefthookFile struct {
	name string
	root *yaml.Node
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
	m := &hookManager{Name: "lefthook", File: main, Entries: map[string][]hookEntry{}}

	root, err := e.readYAMLMap(main)
	if err != nil {
		m.Broken = append(m.Broken, ParseFailure(main, err))
		return m
	}
	files := []lefthookFile{{main, root}}
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
			files = append(files, lefthookFile{name, n})
		}
	}
	if len(m.Broken) > 0 {
		return m
	}
	for _, h := range allGitHooks {
		hook := &lefthookHook{named: map[string][]hookEntry{}}
		for _, f := range files {
			e.addLefthookHook(m, hook, f.name, h, mapValue(f.root, h))
		}
		if en := hook.entries(); len(en) > 0 {
			m.Entries[h] = en
		}
	}
	return m
}

func (e *Env) addLefthookHook(m *hookManager, into *lefthookHook, file, hook string, n *yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	if cmds := mapValue(n, "commands"); cmds != nil && cmds.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(cmds.Content); i += 2 {
			name := cmds.Content[i].Value
			into.set("command "+name, runEntry(file, hook+".commands."+name, mapValue(resolve(cmds.Content[i+1]), "run")))
		}
	}
	if scripts := mapValue(n, "scripts"); scripts != nil && scripts.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(scripts.Content); i += 2 {
			name := scripts.Content[i].Value
			into.set("script "+name, e.lefthookScript(m, hook, name, resolve(scripts.Content[i+1])))
		}
	}
	into.jobs = append(into.jobs, e.lefthookJobs(m, file, hook, hook+".jobs", mapValue(n, "jobs"))...)
}

// lefthookJobs returns the entries of a jobs: list, following nested
// groups.
func (e *Env) lefthookJobs(m *hookManager, file, hook, at string, jobs *yaml.Node) []hookEntry {
	if jobs == nil || jobs.Kind != yaml.SequenceNode {
		return nil
	}
	var out []hookEntry
	for i, j := range jobs.Content {
		j = resolve(j)
		if j.Kind != yaml.MappingNode {
			continue
		}
		jobAt := fmt.Sprintf("%s[%d]", at, i)
		out = append(out, runEntry(file, jobAt, mapValue(j, "run"))...)
		if s := mapValue(j, "script"); s != nil && s.Kind == yaml.ScalarNode {
			out = append(out, e.lefthookScript(m, hook, s.Value, j)...)
		}
		if g := mapValue(j, "group"); g != nil {
			out = append(out, e.lefthookJobs(m, file, hook, jobAt+".group.jobs", mapValue(g, "jobs"))...)
		}
	}
	return out
}

// shellRunners are the runners whose lefthook scripts are read as POSIX
// shell.
var shellRunners = map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true}

// lefthookScript returns the entry of a lefthook script: the tracked file
// .lefthook/<hook>/<name>, read as shell when its runner is a shell. A
// script of another runner, or one that is not tracked, calls nothing.
func (e *Env) lefthookScript(m *hookManager, hook, name string, entry *yaml.Node) []hookEntry {
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
		m.scriptBroken(hook, ParseFailure(file, err))
		return nil
	}
	return []hookEntry{{File: file, Where: hook + " script", Commands: cmds}}
}

// runEntry is the entry of a run: value at "at" in file. A run that does
// not parse as shell calls nothing.
func runEntry(file, at string, n *yaml.Node) []hookEntry {
	if n == nil || n.Kind != yaml.ScalarNode {
		return nil
	}
	cmds, err := ParseCommands(n.Value)
	if err != nil {
		return nil
	}
	return []hookEntry{{File: file, Where: fmt.Sprintf("line %d: %s", n.Line, at), Commands: cmds}}
}

const preCommitFile = ".pre-commit-config.yaml"

// preCommitLegacyStages are pre-commit's old stage names and the Git
// hooks they stand for.
var preCommitLegacyStages = map[string]string{
	"commit":       hookPreCommit,
	"merge-commit": "pre-merge-commit",
	"push":         hookPrePush,
}

// preCommit reads .pre-commit-config.yaml: the hooks of repo: local, each
// a command of its entry's words and its args, in the stages it declares,
// or default_stages, or pre-commit. Legacy stage names commit,
// merge-commit and push count as pre-commit, pre-merge-commit and
// pre-push.
func (e *Env) preCommit() *hookManager {
	if !e.Repo.HasFile(preCommitFile) {
		return nil
	}
	m := &hookManager{Name: "pre-commit", File: preCommitFile, Entries: map[string][]hookEntry{}}
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
			en := hookEntry{File: preCommitFile, Where: fmt.Sprintf("line %d: hook %s", h.Line, scalarOr(mapValue(h, "id"), "")), Commands: []Command{cmd}}
			stages := defaults
			if s := mapValue(h, "stages"); s != nil {
				stages = stringList(s)
			}
			for _, st := range stages {
				if hook, ok := preCommitLegacyStages[st]; ok {
					st = hook
				}
				m.addEntry(st, en)
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

// husky recognizes any tracked file under .husky/ and reads the script of
// each Git hook, .husky/<hook>, as shell. Other files there (husky's own
// .husky/_/, helpers a hook sources) are not hooks: husky runs only the
// scripts named for a Git hook.
func (e *Env) husky() *hookManager {
	files := e.Repo.FilesUnder(huskyDir)
	if len(files) == 0 {
		return nil
	}
	m := &hookManager{Name: "husky", File: files[0], Entries: map[string][]hookEntry{}, HookFile: map[string]string{}}
	for _, h := range gitHooks {
		m.HookFile[h] = huskyDir + "/" + h
	}
	for _, h := range allGitHooks {
		file := huskyDir + "/" + h
		data, err := e.ReadFile(file)
		if err != nil {
			continue
		}
		cmds, err := ParseCommands(string(data))
		if err != nil {
			m.scriptBroken(h, ParseFailure(file, err))
			continue
		}
		m.addEntry(h, hookEntry{File: file, Where: h + " script", Commands: cmds})
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
