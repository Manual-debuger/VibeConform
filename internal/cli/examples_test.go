package cli

import (
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/reconcile"
)

// TestExamplesAreConformant is the drift alarm for the standards this
// repository cannot dogfood. VibeConform is a Go repository, so nothing here
// resolves ts-tooling or python-tooling, and their templates would otherwise
// ship with no live counterpart to prove them against — which is precisely
// what internal/module/gotooling's TestTemplateMatchesLiveFile exists to
// prevent for .golangci.yml.
//
// The committed example is the tool's own output rather than an assertion
// about a copy of it, so this fails whenever a template changes without
// examples/ being re-synced.
//
// It deliberately lives here rather than in Taskfile.yml or CI: both are
// resources of production/v1 and ship to every adopting repository, and an
// --repo-root examples/typescript line in either would leak this
// repository's layout into the standard. See
// docs/specs/0014-m2-milestone.md.
func TestExamplesAreConformant(t *testing.T) {
	for _, example := range []string{"typescript", "python"} {
		t.Run(example, func(t *testing.T) {
			root := filepath.Join("..", "..", "examples", example)

			p, err := buildPlan(root)
			if err != nil {
				t.Fatalf("planning %s: %v", root, err)
			}
			if len(p.Resources) == 0 {
				t.Fatalf("%s resolved no resources; the standard it declares is empty", root)
			}

			for _, rp := range p.Resources {
				if !rp.Supported {
					t.Errorf("%s: unsupported ownership %v", rp.Resource.Path, rp.Resource.Ownership)
					continue
				}
				if rp.Decision != reconcile.NoChange {
					t.Errorf("%s: %v, want NoChange — run: vibe sync --repo-root examples/%s",
						rp.Resource.Path, rp.Decision, example)
				}
			}
		})
	}
}

// TestExamplesCoverEveryLanguageStandard stops an example from being
// forgotten when a standard is added: a standard nothing syncs is a standard
// nothing checks.
func TestExamplesCoverEveryLanguageStandard(t *testing.T) {
	for _, example := range []string{"typescript", "python"} {
		root := filepath.Join("..", "..", "examples", example)
		p, err := buildPlan(root)
		if err != nil {
			t.Fatalf("planning %s: %v", root, err)
		}
		if want := "production-" + example; p.Standard.Name != want {
			t.Errorf("examples/%s declares %s, want %s", example, p.Standard.Name, want)
		}
	}
}
