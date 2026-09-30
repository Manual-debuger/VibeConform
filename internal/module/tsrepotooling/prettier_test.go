package tsrepotooling

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/module"
)

// Regression tests for spec 0026 §10: examples/typescript failed its own
// fmt:check on PR #43 because Prettier rewrites the layout of the elements
// the vscode integration owns. Every place the generated tooling runs
// Prettier must leave the selected editors' owned files alone.

var ownedByEditors = map[string][]string{
	"vscode": {".vscode/tasks.json", ".vscode/extensions.json"},
	"zed":    {".zed/tasks.json"},
}

func resolveFor(t *testing.T, integrations []string) (taskfile, lefthook []byte) {
	t.Helper()
	rs, err := New().Resolve(context.Background(), &module.Context{Integrations: integrations})
	if err != nil {
		t.Fatalf("Resolve(%v): %v", integrations, err)
	}
	for _, r := range rs {
		switch r.Path {
		case "Taskfile.yml":
			taskfile = r.Content
		case "lefthook.yml":
			lefthook = r.Content
		}
	}
	return taskfile, lefthook
}

type renderedTaskfile struct {
	Tasks map[string]struct {
		// A cmd is a shell string or a map such as {task: fmt}.
		Cmds []any `yaml:"cmds"`
	} `yaml:"tasks"`
}

type renderedLefthook struct {
	PreCommit struct {
		Commands map[string]struct {
			Glob    string   `yaml:"glob"`
			Exclude []string `yaml:"exclude"`
			Run     string   `yaml:"run"`
		} `yaml:"commands"`
	} `yaml:"pre-commit"`
}

// scripts joins a task's shell commands, skipping task calls.
func scripts(cmds []any) string {
	var out []string
	for _, c := range cmds {
		if s, ok := c.(string); ok {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

func TestPrettierLeavesOwnedEditorFilesAlone(t *testing.T) {
	for name, tc := range map[string]struct {
		integrations []string
		excluded     []string
	}{
		"vscode":            {[]string{"claude", "vscode"}, ownedByEditors["vscode"]},
		"zed":               {[]string{"claude", "zed"}, ownedByEditors["zed"]},
		"both":              {[]string{"claude", "vscode", "zed"}, append(append([]string{}, ownedByEditors["vscode"]...), ownedByEditors["zed"]...)},
		"vscode, no agents": {[]string{"vscode"}, ownedByEditors["vscode"]},
	} {
		t.Run(name, func(t *testing.T) {
			taskfile, lefthook := resolveFor(t, tc.integrations)

			var tf renderedTaskfile
			if err := yaml.Unmarshal(taskfile, &tf); err != nil {
				t.Fatalf("Taskfile.yml: %v", err)
			}
			for _, task := range []string{"fmt", "fmt:check"} {
				cmds := scripts(tf.Tasks[task].Cmds)
				for _, p := range tc.excluded {
					if !strings.Contains(cmds, `"!`+p+`"`) {
						t.Errorf("%s does not exclude %s:\n%s", task, p, cmds)
					}
				}
				if got := strings.Count(cmds, `"!`); got != len(tc.excluded) {
					t.Errorf("%s has %d exclusions, want %d:\n%s", task, got, len(tc.excluded), cmds)
				}
			}

			hookFormat, hasHooks := tf.Tasks["hook:format"]
			if hasHooks != module.WantsAgentHooks(&module.Context{Integrations: tc.integrations}) {
				t.Fatalf("hook:format present = %v with %v", hasHooks, tc.integrations)
			}
			if hasHooks {
				cmds := scripts(hookFormat.Cmds)
				for _, p := range tc.excluded {
					// Once on git diff, once on git ls-files.
					if got := strings.Count(cmds, "':(exclude)"+p+"'"); got != 2 {
						t.Errorf("hook:format excludes %s %d times, want 2:\n%s", p, got, cmds)
					}
				}
			}

			var lh renderedLefthook
			if err := yaml.Unmarshal(lefthook, &lh); err != nil {
				t.Fatalf("lefthook.yml: %v", err)
			}
			if got := lh.PreCommit.Commands["prettier"].Exclude; strings.Join(got, "\n") != strings.Join(tc.excluded, "\n") {
				t.Errorf("lefthook prettier exclude = %v, want %v", got, tc.excluded)
			}
		})
	}
}

// TestNoEditorKeepsTemplatesByteIdentical: without a selected editor the
// rendered files are the embedded templates, so no repository that does
// not opt in sees a change (spec 0026 "Behavior").
func TestNoEditorKeepsTemplatesByteIdentical(t *testing.T) {
	for name, integrations := range map[string][]string{
		"unknown selection": nil,
		"default agents":    {"claude", "codex"},
	} {
		t.Run(name, func(t *testing.T) {
			tf, lh := resolveFor(t, integrations)
			if !bytes.Equal(tf, taskfile) {
				t.Error("Taskfile.yml differs from the embedded template")
			}
			if !bytes.Equal(lh, lefthookConfig) {
				t.Error("lefthook.yml differs from the embedded template")
			}
		})
	}
}

func TestExcludeFromPrettierRefusesUnknownTemplates(t *testing.T) {
	paths := []string{".zed/tasks.json"}
	missing := bytes.ReplaceAll(taskfile, []byte(prettierGlob), []byte(`"src/**/*.ts"`))
	if _, err := excludeFromPrettierTaskfile(missing, paths, true); err == nil {
		t.Error("a Taskfile without the fmt glob was accepted")
	}
	duplicated := append(append([]byte{}, taskfile...), taskfile...)
	if _, err := excludeFromPrettierTaskfile(duplicated, paths, true); err == nil {
		t.Error("a Taskfile with every anchor twice was accepted")
	}
	if _, err := excludeFromPrettierLefthook(bytes.ReplaceAll(lefthookConfig, []byte("glob:"), []byte("files:")), paths); err == nil {
		t.Error("a lefthook.yml without the prettier glob was accepted")
	}
	if got, err := excludeFromPrettierTaskfile(missing, nil, true); err != nil || !bytes.Equal(got, missing) {
		t.Errorf("no paths must return the input unchanged; got err %v", err)
	}
}

// TestHookFormatSkipsOwnedFilesWithRealGit runs hook:format's file list
// with real git: an edited owned file is not listed, while the user's own
// .vscode/settings.json and ordinary sources still are.
func TestHookFormatSkipsOwnedFilesWithRealGit(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	task, err := exec.LookPath("task")
	if err != nil {
		t.Skip("task not on PATH")
	}
	tf, _ := resolveFor(t, []string{"claude", "vscode", "zed"})
	var rendered renderedTaskfile
	if err := yaml.Unmarshal(tf, &rendered); err != nil {
		t.Fatal(err)
	}
	script := scripts(rendered.Tasks["hook:format"].Cmds)
	start := strings.Index(script, "list() {")
	end := strings.Index(script, "\n}\n")
	if start < 0 || end < 0 {
		t.Fatalf("hook:format has no list function:\n%s", script)
	}
	listFn := script[start : end+2]

	dir := t.TempDir()
	indented := "        " + strings.ReplaceAll(listFn, "\n", "\n        ")
	probe := "version: '3'\ntasks:\n  list:\n    silent: true\n    cmds:\n      - |\n" + indented + "\n        list\n"
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(name string, args ...string) string {
		t.Helper()
		// #nosec G204 -- fixed arguments
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s %v: %v", name, args, err)
		}
		return string(out)
	}

	write("Taskfile.yml", probe)
	for _, p := range []string{".vscode/tasks.json", ".vscode/settings.json", ".zed/tasks.json", "src/a.ts"} {
		write(p, "{}\n")
	}
	run(git, "init", "-q", "-b", "main")
	run(git, "-c", "user.name=t", "-c", "user.email=t@example.com", "add", ".")
	run(git, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init")
	// Tracked and changed, plus untracked: both halves of list().
	write(".vscode/tasks.json", "[]\n")
	write(".vscode/settings.json", "[]\n")
	write(".zed/tasks.json", "[]\n")
	write("src/a.ts", "export {}\n")
	write(".vscode/extensions.json", "{}\n")
	write("src/b.ts", "export {}\n")

	listed := map[string]bool{}
	for _, f := range strings.Split(run(task, "list"), "\x00") {
		if f != "" {
			listed[f] = true
		}
	}
	for _, p := range []string{".vscode/settings.json", "src/a.ts", "src/b.ts"} {
		if !listed[p] {
			t.Errorf("%s is not listed; listed %v", p, listed)
		}
	}
	for _, p := range []string{".vscode/tasks.json", ".vscode/extensions.json", ".zed/tasks.json"} {
		if listed[p] {
			t.Errorf("owned file %s is listed; listed %v", p, listed)
		}
	}
}
