package pins_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/ci/pins"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// The skill's reference CI files are written out, not rendered from the
// table: the setup skill must not import the template-sync packages
// (spec 0054). So the check lives here, on the table's side, and reads the
// skill's embedded files.

const setupAction = "Manual-debuger/VibeConform"

var (
	// usesRE matches a uses: line pinned by commit SHA.
	usesRE = regexp.MustCompile(`(?m)uses:\s*([^@\s]+)@([0-9a-f]{40})`)
	// envRE matches a quoted version in a workflow's env: block.
	envRE = regexp.MustCompile(`(?m)^\s*([A-Z_]+_VERSION):\s*"([^"]*)"`)
	// installRE matches a go install with a literal version.
	installRE = regexp.MustCompile(`go install ([^@\s]+)@([^\s$]+)`)
	// setupTaskRE matches go-task/setup-task's version input.
	setupTaskRE = regexp.MustCompile(`uses:\s*go-task/setup-task@\S+[^\n]*\n\s+with:\n\s+version:\s*"?([^"\s]+)"?`)
)

// skillPins returns, for each version the table holds, the name the skill's
// files give it: an env: variable or a go install path.
func skillPins(t pins.Table) (env, install map[string]string) {
	env = map[string]string{
		"GO_VERSION":            t.Go,
		"TASK_VERSION":          t.Task,
		"GOLANGCI_LINT_VERSION": t.GolangciLint,
		"NODE_VERSION":          t.Node,
		"PYTHON_VERSION":        t.Python,
	}
	install = map[string]string{
		"github.com/go-task/task/v3/cmd/task":                    t.Task,
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint": t.GolangciLint,
		"golang.org/x/tools/cmd/goimports":                       t.Goimports,
		"golang.org/x/vuln/cmd/govulncheck":                      t.Govulncheck,
		"github.com/rhysd/actionlint/cmd/actionlint":             t.Actionlint,
	}
	return env, install
}

// TestSkillCIMatchesPins: wherever the skill's reference CI files name a
// tool the table pins, or an action a generated CI file pins, they name the
// same version or commit, so a pin bump cannot leave the skill behind.
func TestSkillCIMatchesPins(t *testing.T) {
	// Every action the generated CI uses, with the commits it is pinned to.
	actions := map[string]map[string]bool{}
	for _, content := range renderAll(t) {
		for _, m := range usesRE.FindAllStringSubmatch(string(content), -1) {
			if actions[m[1]] == nil {
				actions[m[1]] = map[string]bool{}
			}
			actions[m[1]][m[2]] = true
		}
	}
	env, install := skillPins(pins.Current)

	skill := setupskill.FS()
	files, err := fs.Glob(skill, "reference/ci/*/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 8 {
		t.Fatalf("found only %d reference CI files: %v", len(files), files)
	}
	checked := map[string]int{}
	for _, file := range files {
		b, err := fs.ReadFile(skill, file)
		if err != nil {
			t.Fatal(err)
		}
		content := strings.ReplaceAll(string(b), "\r\n", "\n")
		for _, m := range usesRE.FindAllStringSubmatch(content, -1) {
			want, ok := actions[m[1]]
			// The skill names the setup action by a placeholder commit
			// that the agent fills in from the release it installs.
			if !ok || m[1] == setupAction {
				continue
			}
			checked["actions"]++
			if !want[m[2]] {
				t.Errorf("%s pins %s at %s; the generated CI pins it at %v", file, m[1], m[2], keysOf(want))
			}
		}
		for _, m := range envRE.FindAllStringSubmatch(content, -1) {
			want, ok := env[m[1]]
			if !ok {
				continue
			}
			checked["env"]++
			if m[2] != want {
				t.Errorf("%s sets %s to %q; the pin table says %q", file, m[1], m[2], want)
			}
		}
		for _, m := range installRE.FindAllStringSubmatch(content, -1) {
			want, ok := install[m[1]]
			if !ok {
				t.Errorf("%s installs %s, which the pin table does not pin", file, m[1])
				continue
			}
			checked["go install"]++
			if m[2] != want {
				t.Errorf("%s installs %s@%s; the pin table says %s", file, m[1], m[2], want)
			}
		}
		for _, m := range setupTaskRE.FindAllStringSubmatch(content, -1) {
			checked["setup-task"]++
			if m[1] != pins.Current.TaskSetup() {
				t.Errorf("%s sets up Task %s; the pin table says %s", file, m[1], pins.Current.TaskSetup())
			}
		}
	}
	// Each kind of reference is present, so a change to the files' form
	// cannot make the check pass by matching nothing.
	for _, kind := range []string{"actions", "env", "go install", "setup-task"} {
		if checked[kind] == 0 {
			t.Errorf("no %s reference in the skill's CI files was checked", kind)
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
