package setupskill_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// The reference CI files' pins, moved here from internal/module/ci/pins
// by spec 0059 (Desired Behavior 13, Seam 2). The pin table and the
// generated CI they were checked against go with internal/module, so the
// table's versions are held here, and each action must be pinned at one
// commit across the reference files.

// refPins are the toolchain versions the reference CI files name: the
// values of internal/module/ci/pins.Current when spec 0059 removed it.
// A bump changes the reference files and this table together.
var refPins = struct {
	Go, Task, GolangciLint, Goimports, Govulncheck, Actionlint, Node, Python string
}{
	Go:           "1.27.0",
	Task:         "v3.53.1",
	GolangciLint: "v2.13.2",
	Goimports:    "v0.51.0",
	Govulncheck:  "v1.8.0",
	Actionlint:   "v1.7.12",
	Node:         "22",
	Python:       "3.12",
}

const setupAction = "Manual-debuger/VibeConform"

var (
	// usesRE matches a uses: line pinned by commit SHA.
	usesRE = regexp.MustCompile(`(?m)uses:\s*([^@\s]+)@([0-9a-f]{40})`)
	// envRE matches a quoted version in a workflow's env: block.
	envRE = regexp.MustCompile(`(?m)^\s*([A-Z_]+_(?:VERSION|RELEASE)):\s*"([^"]*)"`)
	// installRE matches a go install with a literal version.
	installRE = regexp.MustCompile(`go install ([^@\s]+)@([^\s$]+)`)
	// setupTaskRE matches go-task/setup-task's version input.
	setupTaskRE = regexp.MustCompile(`uses:\s*go-task/setup-task@\S+[^\n]*\n\s+with:\n\s+version:\s*"?([^"\s]+)"?`)
)

// TestSkillCIMatchesPins: wherever the skill's reference CI files name a
// pinned tool, they name the version refPins holds, and every action is
// pinned at the same commit in each file, so a pin bump cannot leave one
// reference behind.
func TestSkillCIMatchesPins(t *testing.T) {
	env := map[string]string{
		"GO_VERSION":            refPins.Go,
		"TASK_VERSION":          refPins.Task,
		"GOLANGCI_LINT_VERSION": refPins.GolangciLint,
		"NODE_VERSION":          refPins.Node,
		"PYTHON_VERSION":        refPins.Python,
	}
	install := map[string]string{
		"github.com/go-task/task/v3/cmd/task":                    refPins.Task,
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint": refPins.GolangciLint,
		"golang.org/x/tools/cmd/goimports":                       refPins.Goimports,
		"golang.org/x/vuln/cmd/govulncheck":                      refPins.Govulncheck,
		"github.com/rhysd/actionlint/cmd/actionlint":             refPins.Actionlint,
	}

	skill := setupskill.FS()
	// The pattern matches .gitlab-ci.yml too: * matches a leading dot.
	files, err := fs.Glob(skill, "reference/ci/*/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 8 {
		t.Fatalf("found only %d reference CI files: %v", len(files), files)
	}
	// actions holds, per action, the commits and the files that pin it.
	actions := map[string]map[string][]string{}
	checked := map[string]int{}
	for _, file := range files {
		b, err := fs.ReadFile(skill, file)
		if err != nil {
			t.Fatal(err)
		}
		content := strings.ReplaceAll(string(b), "\r\n", "\n")
		for _, m := range usesRE.FindAllStringSubmatch(content, -1) {
			// The skill names the setup action by a placeholder commit
			// that the agent fills in from the release it installs.
			if m[1] == setupAction {
				continue
			}
			checked["actions"]++
			if actions[m[1]] == nil {
				actions[m[1]] = map[string][]string{}
			}
			actions[m[1]][m[2]] = append(actions[m[1]][m[2]], file)
		}
		for _, m := range envRE.FindAllStringSubmatch(content, -1) {
			// The GitLab references need an exact Node release (for
			// Chocolatey and nodejs.org) of the major the table pins.
			if m[1] == "NODE_RELEASE" {
				checked["env"]++
				if !strings.HasPrefix(m[2], refPins.Node+".") {
					t.Errorf("%s sets NODE_RELEASE to %q; the pin table says Node %q", file, m[2], refPins.Node)
				}
				continue
			}
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
			if want := strings.TrimPrefix(refPins.Task, "v"); m[1] != want {
				t.Errorf("%s sets up Task %s; the pin table says %s", file, m[1], want)
			}
		}
	}
	for action, commits := range actions {
		if len(commits) > 1 {
			t.Errorf("%s is pinned at %d commits: %v", action, len(commits), commits)
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
