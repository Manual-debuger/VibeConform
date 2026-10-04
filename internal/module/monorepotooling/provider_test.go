package monorepotooling

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

// TestRootFilesFollowProvider pins spec 0038 acceptance criterion 6 for
// mono-repo-tooling: under gitlab and none, verify runs only the
// components' verify, workflows:lint does not exist, actionlint is not
// required, and nothing reached from verify or verify-ci runs vibe.
func TestRootFilesFollowProvider(t *testing.T) {
	for _, provider := range []string{module.CIGitHub, module.CIGitLab, module.CINone} {
		t.Run(provider, func(t *testing.T) {
			mctx := &module.Context{Components: allProfiles, Policies: map[string]string{manifest.CIProvider: provider}}
			rs, err := New().Resolve(context.Background(), mctx)
			if err != nil {
				t.Fatal(err)
			}
			github := provider == module.CIGitHub
			for _, r := range rs {
				if r.Path != "Taskfile.yml" {
					continue
				}
				root := parse(t, r.Path, r.Content)
				if _, ok := root.Tasks["workflows:lint"]; ok != github {
					t.Errorf("workflows:lint defined = %v", ok)
				}
				var want []any
				for _, c := range allProfiles {
					want = append(want, map[string]any{"task": c.ID + ":verify"})
				}
				if github {
					want = append(want, map[string]any{"task": "workflows:lint"})
				}
				if got := root.Tasks["verify"].Cmds; !reflect.DeepEqual(got, want) {
					t.Errorf("verify cmds %v, want %v", got, want)
				}
				if !github && strings.Contains(string(r.Content), "actionlint") {
					t.Error("Taskfile.yml mentions actionlint")
				}
				if !reflect.DeepEqual(root.Tasks["verify-ci"].Cmds, []any{map[string]any{"task": "verify"}}) {
					t.Errorf("verify-ci cmds %v", root.Tasks["verify-ci"].Cmds)
				}
			}
			required := false
			for _, tool := range New().(module.ToolRequirer).RequiredTools(mctx) {
				required = required || tool.Name == "actionlint"
			}
			if required != github {
				t.Errorf("actionlint required = %v", required)
			}
		})
	}
}
