package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// repoToolingTaskfiles are the three templates that define what `task verify`
// means in a generated repository, one per language standard.
var repoToolingTaskfiles = []string{
	"repotooling/templates/Taskfile.yml",
	"tsrepotooling/templates/Taskfile.yml",
	"pyrepotooling/templates/Taskfile.yml",
}

// taskCmd is one entry in a Taskfile task's cmds list. Task allows either a
// bare shell string, a {task: other} reference, or a {cmd: "..."} map; the
// first two are what these templates use, the third is handled so a future
// rewrite doesn't silently slip past this test.
type taskCmd struct {
	shell string
	dep   string
}

func (c *taskCmd) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Decode(&c.shell)
	}
	var m struct {
		Task string `yaml:"task"`
		Cmd  string `yaml:"cmd"`
	}
	if err := n.Decode(&m); err != nil {
		return err
	}
	c.dep, c.shell = m.Task, m.Cmd
	return nil
}

type taskfileDoc struct {
	Tasks map[string]struct {
		Cmds []taskCmd `yaml:"cmds"`
	} `yaml:"tasks"`
}

// shellClosure returns every shell command reachable from the named task,
// following `- task:` references transitively.
func (d taskfileDoc) shellClosure(t *testing.T, root string) []string {
	t.Helper()

	var out []string
	seen := map[string]bool{}

	var walk func(name string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true

		task, ok := d.Tasks[name]
		if !ok {
			t.Fatalf("task %q is referenced but not defined", name)
		}
		for _, c := range task.Cmds {
			if c.dep != "" {
				walk(c.dep)
				continue
			}
			if c.shell != "" {
				out = append(out, c.shell)
			}
		}
	}
	walk(root)

	return out
}

// TestVerifyNeverInvokesVibe pins spec 0017's central invariant: in a generated
// repository, `task verify` and `task verify-ci` must exercise native language
// tooling only, so they succeed in a repository that has never installed
// VibeConform. Before spec 0017 they transitively ran `task audit`, which meant
// a `prod-ts`/`prod-py` adopter needed `vibe` on PATH — and `prod-go` only
// worked because this repository happens to vendor cmd/vibe.
//
// The per-module drift tests cannot catch a regression here. They compare the
// embedded template to the live file byte for byte, so re-adding `- task: audit`
// to verify and re-running `vibe sync` moves both copies together and leaves
// every repository reporting conformant.
func TestVerifyNeverInvokesVibe(t *testing.T) {
	for _, rel := range repoToolingTaskfiles {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.FromSlash(rel))
			if err != nil {
				t.Fatalf("reading template: %v", err)
			}

			var doc taskfileDoc
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("parsing template: %v", err)
			}

			for _, entry := range []string{"verify", "verify-ci"} {
				for _, cmd := range doc.shellClosure(t, entry) {
					if strings.Contains(cmd, "vibe") {
						t.Errorf("task %s reaches %q, which invokes vibe — "+
							"verify must depend on native language tooling only (spec 0017); "+
							"run VibeConform's own checks from task audit instead", entry, cmd)
					}
				}
			}
		})
	}
}

// TestRepoToolingTaskfilesNeverInvokeVibe pins spec 0022's isolation: every
// vibe invocation lives in Taskfile.vibe.yml, which the vibe-conformance
// module owns and which removing VibeConform deletes. The repo-tooling
// Taskfile only includes it, optionally, so it keeps working once that file
// is gone. Checking every task rather than only verify's closure is what
// makes "delete four files" the whole removal procedure.
//
// The audit task's own invariants — it must reach vibe, and only as a
// PATH-resolved binary or a pinned remote module (specs 0017, 0018) — moved
// with it to internal/module/conformance.
func TestRepoToolingTaskfilesNeverInvokeVibe(t *testing.T) {
	for _, rel := range repoToolingTaskfiles {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.FromSlash(rel))
			if err != nil {
				t.Fatalf("reading template: %v", err)
			}

			var doc taskfileDoc
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("parsing template: %v", err)
			}

			if _, ok := doc.Tasks["audit"]; ok {
				t.Error("defines an audit task; it belongs in Taskfile.vibe.yml (spec 0022)")
			}
			for name := range doc.Tasks {
				for _, cmd := range doc.shellClosure(t, name) {
					if strings.Contains(cmd, "vibe") {
						t.Errorf("task %s reaches %q, which invokes vibe; only "+
							"Taskfile.vibe.yml may (spec 0022)", name, cmd)
					}
				}
			}

			var inc struct {
				Includes map[string]struct {
					Taskfile string `yaml:"taskfile"`
					Optional bool   `yaml:"optional"`
					Flatten  bool   `yaml:"flatten"`
				} `yaml:"includes"`
			}
			if err := yaml.Unmarshal(raw, &inc); err != nil {
				t.Fatalf("parsing includes: %v", err)
			}
			vibe, ok := inc.Includes["vibe"]
			switch {
			case !ok:
				t.Error("does not include Taskfile.vibe.yml, so task audit is undefined")
			case vibe.Taskfile != "./Taskfile.vibe.yml":
				t.Errorf("vibe include points at %q, want ./Taskfile.vibe.yml", vibe.Taskfile)
			case !vibe.Optional:
				t.Error("vibe include is not optional; deleting Taskfile.vibe.yml would break every task")
			case !vibe.Flatten:
				t.Error("vibe include is not flattened; task audit would become task vibe:audit")
			}
		})
	}
}
