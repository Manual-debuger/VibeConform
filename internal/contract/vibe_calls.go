package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The places R-01's static part reads for a call of vibe (spec 0052,
// "Recognition rules", R-01): every declared entrypoint, every hook
// command, and every CI step but those of a job that satisfies C-04. The
// hook commands are read here on their own, as raw command strings; the
// H rules' hook-manager model is separate.

// vibeCall is one simple command whose first word is vibe.
type vibeCall struct {
	file  string
	where string
	cmd   Command
}

// callsVibe returns the commands in cmds whose first word is vibe.
func callsVibe(file, where string, cmds []Command) []vibeCall {
	var out []vibeCall
	for _, c := range cmds {
		if c.HasPrefix("vibe") {
			out = append(out, vibeCall{file, where, c})
		}
	}
	return out
}

// entrypointVibeCalls reads the declared entrypoints. An entrypoint V-04
// finds malformed is skipped: V-04 fails it.
func entrypointVibeCalls(env *Env) []vibeCall {
	var out []vibeCall
	for _, name := range entrypointNames {
		if _, ok := env.Config.Entrypoints[name]; !ok {
			continue
		}
		out = append(out, callsVibe(env.Config.File, "entrypoints."+name, env.entrypoint(name).commands)...)
	}
	return out
}

// hookVibeCalls reads every tracked hook command: lefthook (every run:,
// in lefthook.yml or .lefthook.yml and the tracked files it extends),
// pre-commit (the entry of each hook), husky (each script under .husky/)
// and Claude Code (each command hook in .claude/settings.json). A file
// that does not parse is a finding: R-01 cannot hold for it.
func hookVibeCalls(env *Env) ([]vibeCall, []Finding) {
	var calls []vibeCall
	var broken []Finding
	read := func(file string, parse func([]byte) ([]hookCommand, error)) {
		data, err := env.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err == nil {
			var hooks []hookCommand
			if hooks, err = parse(data); err == nil {
				for _, h := range hooks {
					cmds, perr := ParseCommands(h.command)
					if perr != nil {
						continue // a command that does not parse calls nothing
					}
					calls = append(calls, callsVibe(file, h.where, cmds)...)
				}
				return
			}
		}
		broken = append(broken, ParseFailure(file, err))
	}

	lefthook := []string{"lefthook.yml", ".lefthook.yml"}
	for i := 0; i < len(lefthook); i++ {
		file := lefthook[i]
		read(file, func(data []byte) ([]hookCommand, error) {
			hooks, extends, err := lefthookRuns(data)
			for _, e := range extends {
				if env.Repo.HasFile(e) && !slices.Contains(lefthook, e) {
					lefthook = append(lefthook, e)
				}
			}
			return hooks, err
		})
	}
	read(".pre-commit-config.yaml", preCommitEntries)
	for _, f := range env.Repo.FilesUnder(".husky") {
		if !strings.HasPrefix(f, ".husky/_/") {
			read(f, func(data []byte) ([]hookCommand, error) {
				return []hookCommand{{where: "script", command: string(data)}}, nil
			})
		}
	}
	read(".claude/settings.json", claudeHookCommands)
	return calls, broken
}

// hookCommand is one hook's command string and where it is.
type hookCommand struct {
	where   string
	command string
}

// lefthookRuns returns every run: string of a lefthook configuration,
// wherever it is nested, and the files it extends.
func lefthookRuns(data []byte) ([]hookCommand, []string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Content) == 0 {
		return nil, nil, nil
	}
	root := resolve(doc.Content[0])
	var extends []string
	if e := mapValue(root, "extends"); e != nil {
		extends = filterPatterns(e)
	}
	var out []hookCommand
	var walk func(n *yaml.Node, at string)
	walk = func(n *yaml.Node, at string) {
		n = resolve(n)
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i].Value, resolve(n.Content[i+1])
				if k == "run" && v.Kind == yaml.ScalarNode {
					out = append(out, hookCommand{where: fmt.Sprintf("line %d: %s", v.Line, at), command: v.Value})
					continue
				}
				walk(v, joinPath(at, k))
			}
		case yaml.SequenceNode:
			for i, c := range n.Content {
				walk(c, fmt.Sprintf("%s[%d]", at, i))
			}
		}
	}
	walk(root, "")
	return out, extends, nil
}

func joinPath(at, key string) string {
	if at == "" {
		return key
	}
	return at + "." + key
}

// preCommitEntries returns the entry of every hook of a pre-commit
// configuration's local repositories.
func preCommitEntries(data []byte) ([]hookCommand, error) {
	var cfg struct {
		Repos []struct {
			Repo  string `yaml:"repo"`
			Hooks []struct {
				ID    string `yaml:"id"`
				Entry string `yaml:"entry"`
			} `yaml:"hooks"`
		} `yaml:"repos"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	var out []hookCommand
	for _, r := range cfg.Repos {
		if r.Repo != "local" {
			continue
		}
		for _, h := range r.Hooks {
			if h.Entry != "" {
				out = append(out, hookCommand{where: "hook " + h.ID, command: h.Entry})
			}
		}
	}
	return out, nil
}

// claudeHookCommands returns every command hook of a Claude Code
// settings file.
func claudeHookCommands(data []byte) ([]hookCommand, error) {
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	events := make([]string, 0, len(s.Hooks))
	for e := range s.Hooks {
		events = append(events, e)
	}
	slices.Sort(events)
	var out []hookCommand
	for _, e := range events {
		for _, entry := range s.Hooks[e] {
			for _, h := range entry.Hooks {
				if h.Type == "command" && h.Command != "" {
					out = append(out, hookCommand{where: "hooks." + e, command: h.Command})
				}
			}
		}
	}
	return out, nil
}

// ciVibeCalls reads every CI step of every parsed workflow, except the
// jobs in exempt.
func ciVibeCalls(parsed []*Workflow, exempt []ciJob) []vibeCall {
	var out []vibeCall
	for _, wf := range parsed {
		for _, j := range wf.Jobs {
			if slices.ContainsFunc(exempt, func(e ciJob) bool { return e.job == j }) {
				continue
			}
			for _, s := range j.Steps {
				out = append(out, callsVibe(wf.File, fmt.Sprintf("line %d: job %s, step %d", s.Line, j.ID, s.Index+1), s.Run.Commands)...)
			}
		}
	}
	return out
}
