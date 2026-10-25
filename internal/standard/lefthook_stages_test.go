package standard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

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
// every standard, with Graphify and without.
func TestLefthookHookStages(t *testing.T) {
	cases := lefthookCases(t)
	if len(cases) == 0 {
		t.Fatal("no standard resolves lefthook.yml")
	}
	// The setup skill's reference hooks are tested in internal/setupskill
	// (spec 0059).
	for name, content := range cases {
		if got := lefthookRuns(t, name, content, "pre-commit"); !slices.Equal(got, []string{preCommitRun}) {
			t.Errorf("%s: pre-commit runs %q, want only %q", name, got, preCommitRun)
		}
		if got := lefthookRuns(t, name, content, "pre-push"); !slices.Equal(got, []string{prePushRun}) {
			t.Errorf("%s: pre-push runs %q, want only %q", name, got, prePushRun)
		}
	}
}

// TestLefthookPassesH02 audits every generated lefthook.yml with contract
// rule H-02 (spec 0057, "The hooks name tools, not entrypoints"): the
// hooks call the entrypoints, so the rule passes.
func TestLefthookPassesH02(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	var h02 []contract.Rule
	for _, r := range contract.Rules() {
		if r.ID == "H-02" {
			h02 = append(h02, r)
		}
	}
	if len(h02) != 1 {
		t.Fatal("H-02 is not in the registry")
	}
	const config = "contract: 1.1.0\nplatforms: [linux, windows]\nagents: [claude]\nentrypoints:\n" +
		"  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"
	for name, content := range lefthookCases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			files := map[string]string{"lefthook.yml": string(content), "vibe.yaml": config, "go.mod": "module example.com/x\n"}
			for f, data := range files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}} {
				if out, err := exec.Command(git, append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			ctx := context.Background()
			repo, err := contract.OpenRepo(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := contract.Parse("vibe.yaml", []byte(config), repo)
			if err != nil {
				t.Fatal(err)
			}
			rep := contract.Audit(ctx, cfg, repo, contract.Options{Static: true, Rules: h02})
			if len(rep.Rules) != 1 || rep.Rules[0].Result != contract.ResultPass {
				t.Errorf("H-02 = %+v, want pass", rep.Rules)
			}
		})
	}
}
