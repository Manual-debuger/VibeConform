package module

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// graphified returns a repo-tooling template's Taskfile and lefthook.yml
// as a repository selecting graphify gets them, with (hooks) or without
// the agent-hook surface.
func graphified(t *testing.T, taskfilePath string, hooks bool) (taskfile, lefthook []byte) {
	t.Helper()
	tf, err := os.ReadFile(filepath.FromSlash(taskfilePath))
	if err != nil {
		t.Fatal(err)
	}
	lh, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.FromSlash(taskfilePath)), "lefthook.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !hooks {
		if tf, err = StripAgentHooks(tf); err != nil {
			t.Fatal(err)
		}
	}
	tf, lh, err = AddGraphify(tf, lh, hooks)
	if err != nil {
		t.Fatal(err)
	}
	return tf, lh
}

func parseTaskfile(t *testing.T, data []byte) (taskfileDoc, describedTaskfile) {
	t.Helper()
	var doc taskfileDoc
	var described describedTaskfile
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing Taskfile: %v\n%s", err, data)
	}
	if err := yaml.Unmarshal(data, &described); err != nil {
		t.Fatal(err)
	}
	return doc, described
}

// TestAddGraphifyShape pins spec 0035 §2 on every single-language
// template: a public graph:update task, the hook:context line only with
// the agent hooks, and Git hook jobs only under post-commit and
// post-checkout.
func TestAddGraphifyShape(t *testing.T) {
	for _, path := range repoToolingTaskfiles {
		for _, hooks := range []bool{true, false} {
			t.Run(path+map[bool]string{true: "/hooks", false: "/no-hooks"}[hooks], func(t *testing.T) {
				tf, lh := graphified(t, path, hooks)
				_, described := parseTaskfile(t, tf)

				update, ok := described.Tasks["graph:update"]
				switch {
				case !ok:
					t.Fatal("no graph:update task")
				case update.Desc == "":
					t.Error("graph:update has no desc; people run it")
				case len(update.Cmds) != 1 || !strings.Contains(update.Cmds[0].shell, "graphify update ."):
					t.Errorf("graph:update should run graphify update . in one script, has %+v", update.Cmds)
				}

				_, hasContext := described.Tasks["hook:context"]
				if hasContext != hooks {
					t.Fatalf("hook:context present = %v, want %v", hasContext, hooks)
				}
				if hooks {
					script := hookScript(t, described, "hook:context")
					if !strings.Contains(script, "Graphify: ") {
						t.Error("hook:context lacks the Graphify line")
					}
					// The line must pass the same allowlist as the rest
					// of session start: it reads, it never rebuilds.
					for _, word := range commandWords(script) {
						if !slices.Contains(contextCommands, word) {
							t.Errorf("hook:context runs %q", word)
						}
					}
					if strings.Contains(script, "task ") || strings.Contains(script, "graphify update") {
						t.Error("hook:context mentions running a task or a graph update")
					}
				}

				hooksDoc := parseLefthookHooks(t, lh)
				for _, hook := range []string{"post-commit", "post-checkout"} {
					if got := hooksDoc[hook].Commands["graphify-update"].Run; got != "task graph:update" {
						t.Errorf("%s graphify-update runs %q, want task graph:update", hook, got)
					}
				}
				for _, hook := range []string{"pre-commit", "pre-push"} {
					for name, c := range hooksDoc[hook].Commands {
						if strings.Contains(name+c.Run, "graph") {
							t.Errorf("%s runs %s (%q); the graph must never gate a commit or push", hook, name, c.Run)
						}
					}
				}
			})
		}
	}
}

// TestVerifyNeverRunsGraphify pins spec 0035 acceptance criterion 8:
// verify, verify-ci, and verify:fast stay independent of graphify when it
// is selected, so they pass on a machine without it.
func TestVerifyNeverRunsGraphify(t *testing.T) {
	for _, path := range repoToolingTaskfiles {
		t.Run(path, func(t *testing.T) {
			tf, _ := graphified(t, path, true)
			doc, _ := parseTaskfile(t, tf)
			for _, entry := range []string{"verify", "verify-ci", "verify:fast"} {
				for _, cmd := range doc.shellClosure(t, entry) {
					if strings.Contains(cmd, "graph") {
						t.Errorf("task %s reaches %q; verification never depends on graphify (spec 0035)", entry, cmd)
					}
				}
			}
		})
	}
}

func TestAddGraphifyNeedsItsAnchor(t *testing.T) {
	if _, _, err := AddGraphify([]byte("version: '3'\n"), []byte("pre-commit: {}\n"), true); err == nil {
		t.Error("no hook:context anchor, but no error")
	}
	if _, _, err := AddGraphify([]byte("version: '3'"), []byte("pre-commit: {}\n"), false); err == nil {
		t.Error("Taskfile without a final newline, but no error")
	}
}

// TestGraphUpdateSkipsWithoutGraphify runs the generated task with a PATH
// holding only task: it must say the graph was not updated, and exit 0,
// because the Git hooks that run it cannot undo a commit.
func TestGraphUpdateSkipsWithoutGraphify(t *testing.T) {
	task := taskOnPath(t)
	if p, err := exec.LookPath("graphify"); err == nil && filepath.Dir(p) == filepath.Dir(task) {
		t.Skip("graphify is installed next to task; cannot hide it from PATH")
	}
	tf, _ := graphified(t, repoToolingTaskfiles[0], false)
	var doc struct {
		Tasks map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(tf, &doc); err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(map[string]any{"version": "3", "tasks": map[string]any{"graph:update": doc.Tasks["graph:update"]}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), out, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", filepath.Dir(task))
	code, stdout, stderr := runTask(t, task, dir, "graph:update", "")
	if code != 0 {
		t.Fatalf("exit %d; a missing graphify must not fail the Git hook. stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "graphify: not on PATH; graph not updated") {
		t.Errorf("no skip line on stderr:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "graphify-out")); err == nil {
		t.Error("graphify-out/ created without graphify")
	}
}

// TestSelectedFailsClosed: an unknown selection selects no default-off
// integration, unlike WantsAgentHooks.
func TestSelectedFailsClosed(t *testing.T) {
	if Selected(nil, GraphifyIntegration) || Selected(&Context{}, GraphifyIntegration) {
		t.Error("an unknown selection selected graphify")
	}
	if !Selected(&Context{Integrations: []string{"claude", GraphifyIntegration}}, GraphifyIntegration) {
		t.Error("an explicit selection was not seen")
	}
}
