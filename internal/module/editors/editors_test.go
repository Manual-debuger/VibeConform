package editors_test

import (
	"context"
	"testing"

	"github.com/tailscale/hujson"

	"github.com/Manual-debuger/VibeConform/internal/jsonarray"
	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/editors/vscode"
	"github.com/Manual-debuger/VibeConform/internal/module/editors/zed"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// TestElementsAreWellFormed: every owned element parses, is identified by
// its declared ID, and every skeleton parses with its array present and
// empty — the invariants the structured-patch planner relies on.
func TestElementsAreWellFormed(t *testing.T) {
	mctx := &module.Context{Profiles: manifest.Profiles}
	for _, m := range []module.Module{vscode.New(), zed.New()} {
		rs, err := m.Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatalf("%s: %v", m.Name(), err)
		}
		if len(rs) == 0 {
			t.Fatalf("%s resolves nothing", m.Name())
		}
		for _, r := range rs {
			if r.Ownership != resource.StructuredPatch || r.Patch == nil {
				t.Fatalf("%s: %s is not a structured patch", m.Name(), r.Path)
			}
			doc, err := jsonarray.Parse(r.Patch.Skeleton, r.Patch.Array)
			if err != nil || doc.Len() != 0 {
				t.Errorf("%s: skeleton %q: %v, %d elements", r.Path, r.Patch.Skeleton, err, doc.Len())
			}
			seen := map[string]bool{}
			for _, e := range r.Patch.Elements {
				v, err := hujson.Parse(e.Value)
				if err != nil {
					t.Errorf("%s: element %s: %v", r.Path, e.Value, err)
					continue
				}
				if id, ok := jsonarray.Identity(v); !ok || id != e.ID {
					t.Errorf("%s: element %s identifies as %q, declared %q", r.Path, e.Value, id, e.ID)
				}
				if seen[e.ID] {
					t.Errorf("%s: duplicate element %q", r.Path, e.ID)
				}
				seen[e.ID] = true
			}
		}
	}
}

// TestVSCodeRecommendsOnlyDeclaredProfiles: recommendations are per
// declared language, and no language means no extensions.json at all.
func TestVSCodeRecommendsOnlyDeclaredProfiles(t *testing.T) {
	for name, tc := range map[string]struct {
		profiles []manifest.Profile
		want     []string
	}{
		"go":   {[]manifest.Profile{manifest.ProfileGo}, []string{"golang.go"}},
		"py":   {[]manifest.Profile{manifest.ProfilePy}, []string{"charliermarsh.ruff", "ms-python.python"}},
		"none": {nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			rs, err := vscode.New().Resolve(context.Background(), &module.Context{Profiles: tc.profiles})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range rs {
				if r.Path != ".vscode/extensions.json" {
					continue
				}
				for _, e := range r.Patch.Elements {
					got = append(got, e.ID)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("recommendations %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("recommendations %v, want %v", got, tc.want)
				}
			}
		})
	}
}
