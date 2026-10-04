package module

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// lefthookTemplates are the lefthook.yml templates that are plain files;
// monorepotooling's is a text/template and is covered through its module.
var lefthookTemplates = []string{
	"repotooling/templates/lefthook.yml",
	"tsrepotooling/templates/lefthook.yml",
	"pyrepotooling/templates/lefthook.yml",
}

type lefthookHook struct {
	Commands map[string]struct {
		Run string `yaml:"run"`
	} `yaml:"commands"`
}

// lefthookDump is a lefthook config's hooks by name. The top-level
// extends: list (spec 0036) is not a hook and is left out.
type lefthookDump map[string]lefthookHook

// parseLefthookHooks parses a lefthook config into its hooks.
func parseLefthookHooks(t *testing.T, data []byte) lefthookDump {
	t.Helper()
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing lefthook config: %v\n%s", err, data)
	}
	hooks := lefthookDump{}
	for name, node := range doc {
		if name == "extends" {
			continue
		}
		var h lefthookHook
		if err := node.Decode(&h); err != nil {
			t.Fatalf("parsing lefthook hook %s: %v\n%s", name, err, data)
		}
		hooks[name] = h
	}
	return hooks
}

// TestLefthookLocalMerge pins spec 0036 acceptance criteria 4 and 5 against
// a real lefthook: without lefthook.local.yml the config is exactly the
// managed one, and with it the project's hooks and commands are merged in
// beside the managed commands.
func TestLefthookLocalMerge(t *testing.T) {
	lefthook, err := exec.LookPath("lefthook")
	if err != nil {
		t.Skip("lefthook not on PATH")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	for _, tmpl := range lefthookTemplates {
		t.Run(tmpl, func(t *testing.T) {
			data, err := os.ReadFile(tmpl)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if out, err := exec.Command(git, "-C", dir, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}
			if err := os.WriteFile(filepath.Join(dir, "lefthook.yml"), data, 0o644); err != nil {
				t.Fatal(err)
			}
			dump := func() lefthookDump {
				t.Helper()
				cmd := exec.Command(lefthook, "dump")
				cmd.Dir = dir
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("lefthook dump: %v\n%s", err, out)
				}
				return parseLefthookHooks(t, out)
			}

			managed := parseLefthookHooks(t, data)
			without := dump()
			if !slices.Equal(sortedKeys(without), sortedKeys(managed)) {
				t.Errorf("without lefthook.local.yml: hooks %v, want %v", sortedKeys(without), sortedKeys(managed))
			}

			local := "post-merge:\n  commands:\n    deps:\n      run: echo deps\npre-commit:\n  commands:\n    extra:\n      run: echo extra\n"
			if err := os.WriteFile(filepath.Join(dir, "lefthook.local.yml"), []byte(local), 0o644); err != nil {
				t.Fatal(err)
			}
			with := dump()
			if with["post-merge"].Commands["deps"].Run != "echo deps" {
				t.Errorf("post-merge from lefthook.local.yml not merged: %v", with["post-merge"])
			}
			if with["pre-commit"].Commands["extra"].Run != "echo extra" {
				t.Errorf("pre-commit command from lefthook.local.yml not merged")
			}
			for hook, h := range managed {
				for name, c := range h.Commands {
					if with[hook].Commands[name].Run != c.Run {
						t.Errorf("%s.%s: run = %q after merge, want %q", hook, name, with[hook].Commands[name].Run, c.Run)
					}
				}
			}
		})
	}
}

func sortedKeys(d lefthookDump) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
