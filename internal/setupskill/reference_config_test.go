package setupskill_test

import (
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// Reference configuration checks moved here from internal/module and
// internal/standard by spec 0059 (Desired Behavior 13, Seam 2).

// TestGolangciWaitsForTheLock pins spec 0056's Desired Behavior 9 for the
// Go reference: golangci-lint holds one lock per machine, and a second
// run that finds it held exits 3 with "parallel golangci-lint is running"
// unless the config sets run.allow-serial-runners. Agent hooks and
// parallel worktrees overlap routinely, so a second run must wait.
func TestGolangciWaitsForTheLock(t *testing.T) {
	var cfg struct {
		Run map[string]any `yaml:"run"`
	}
	if err := yaml.Unmarshal(readRef(t, "reference/lint/go/.golangci.yml"), &cfg); err != nil {
		t.Fatalf("parsing YAML: %v", err)
	}
	if got, ok := cfg.Run["allow-serial-runners"].(bool); !ok || !got {
		t.Errorf("run.allow-serial-runners = %v, want true", cfg.Run["allow-serial-runners"])
	}
}

// The Git hooks of spec 0057 (Desired Behavior 8–11): pre-commit runs the
// format check, pre-push clears Git's repository variables and runs the
// fast verification. Nothing else runs in either hook.
const (
	preCommitRun = "task fmt:check"
	prePushRun   = "unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR; task verify:fast"
)

// lefthookRuns returns the run: of every command of a hook.
func lefthookRuns(t *testing.T, name string, content []byte, hook string) []string {
	t.Helper()
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("%s: parsing lefthook.yml: %v", name, err)
	}
	var h struct {
		Commands map[string]struct {
			Run string `yaml:"run"`
		} `yaml:"commands"`
		Jobs    []any `yaml:"jobs"`
		Scripts any   `yaml:"scripts"`
	}
	node := doc[hook]
	if err := node.Decode(&h); err != nil {
		t.Fatalf("%s: parsing %s: %v", name, hook, err)
	}
	if len(h.Jobs) > 0 || h.Scripts != nil {
		t.Errorf("%s: %s has jobs or scripts beside its commands", name, hook)
	}
	var runs []string
	for _, c := range h.Commands {
		runs = append(runs, c.Run)
	}
	return runs
}

// TestLefthookHookStages pins spec 0057 acceptance criterion 3's shape for
// every reference lefthook.yml.
func TestLefthookHookStages(t *testing.T) {
	for _, stack := range []string{"go", "python", "typescript", "monorepo"} {
		name := "reference/hooks/" + stack + "/lefthook.yml"
		content := readRef(t, name)
		if got := lefthookRuns(t, name, content, "pre-commit"); !slices.Equal(got, []string{preCommitRun}) {
			t.Errorf("%s: pre-commit runs %q, want only %q", name, got, preCommitRun)
		}
		if got := lefthookRuns(t, name, content, "pre-push"); !slices.Equal(got, []string{prePushRun}) {
			t.Errorf("%s: pre-push runs %q, want only %q", name, got, prePushRun)
		}
	}
}
